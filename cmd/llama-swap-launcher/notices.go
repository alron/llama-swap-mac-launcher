package main

// macOS notifications for problems that need a person (TODO item 60).
//
// Standing notices follow llama-swap's state: one is posted when its problem
// begins and removed when it ends (watchNotices). A problem that can pass in
// a moment waits out a grace period first, so a config reload doesn't flash
// one. Events, such as a restart after a crash, are posted once, as they
// happen. While Settings' Native
// notifications box is ticked and macOS shows the app's notifications,
// they replace the alerts for llama-swap failing and a GPU fault; clicking
// one of those opens its alert.

import (
	"errors"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
	"github.com/alron/llama-swap-mac-launcher/internal/update"
)

// A notice is one notification. Its id, the key it's kept under, makes a
// new one replace the last of its kind.
type notice struct {
	title, body string
	// category gives it buttons: faultCategory, the GPU fault's Unload.
	category string
	// timeSensitive gets it through a Focus that allows the app.
	timeSensitive bool
	// grace holds it back until its problem has lasted noticeGrace: one
	// that can pass in a moment, such as the event stream dropping while
	// llama-swap reloads its config (TODO item 89).
	grace bool
}

// noticeGrace is how long a problem marked grace must last to be posted.
// A config reload drops the event stream for about a quarter of a second;
// a real hang lasts, and hang detection already takes up to the
// health-check interval plus 3 seconds.
const noticeGrace = 10 * time.Second

// The notices' ids, and their kinds' prefixes.
const (
	noticeFailed        = "failed"
	noticeNotResponding = "not-responding"
	noticeRefused       = "refused"
	noticeFaultPrefix   = "gpu-fault:"    // and the model's ID
	noticeUnloaded      = "gpu-unloaded:" // and the model's ID
	noticeRestarted     = "restarted"
	faultCategory       = "gpu-fault" // notify.m's, with the Unload button
)

// standingNotices returns the notices llama-swap's state calls for, by id.
// Each is shown while its problem lasts. With autoUnload, a faulted model
// is unloaded at once, so it gets an event instead (unloadedNotice).
func standingNotices(st supervisor.Status, snap health.Snapshot, autoUnload bool) map[string]notice {
	n := map[string]notice{}
	if st.State == supervisor.Failed {
		n[noticeFailed] = notice{title: failureTitle(st), body: orElse(st.Message, "Open the menu for more."), timeSensitive: true}
	}
	if st.State != supervisor.Running {
		return n
	}
	switch {
	case describe(st, snap) == "not responding":
		n[noticeNotResponding] = notice{
			title: "llama-swap isn't responding",
			body: "It stopped answering the app's health checks, so requests to it may hang. " +
				"If it doesn't recover, Restart it from the menu.",
			timeSensitive: true,
			grace:         true,
		}
	case snap.Problem != "":
		n[noticeRefused] = notice{title: "Llama Swap Launcher can't follow llama-swap", body: sentence(snap.Problem) + ".", grace: true}
	}
	if !autoUnload {
		for _, m := range snap.Faulted() {
			n[noticeFaultPrefix+m.ID] = notice{
				title:         m.ID + "'s GPU backend has failed",
				body:          "Requests to it fail until it's unloaded. llama-swap loads it again, in a new process, on the next request for it.",
				category:      faultCategory,
				timeSensitive: true,
			}
		}
	}
	return n
}

// unloadedNotice is the event for a faulted model unloaded automatically.
func unloadedNotice(model string) notice {
	return notice{
		title: model + "'s GPU backend failed",
		body:  "It was unloaded automatically, as Settings ask. llama-swap loads it again, in a new process, on the next request for it.",
	}
}

// restartedNotice is the event for llama-swap started again after an
// unexpected exit; waiting is the supervisor's message while it waited,
// such as "exited (signal: killed); restarting in 1s".
func restartedNotice(waiting string) notice {
	why, _, _ := strings.Cut(waiting, ";")
	if why = strings.TrimSpace(why); why == "" {
		why = "exited"
	}
	return notice{title: "llama-swap restarted", body: "It " + why + " unexpectedly, and the app started it again."}
}

// failureTitle names how llama-swap failed, for its alert and notice.
func failureTitle(st supervisor.Status) string {
	switch {
	case errors.Is(st.Err, supervisor.ErrQuarantined):
		return "macOS blocked llama-swap"
	case errors.Is(st.Err, supervisor.ErrNotStarted):
		return "llama-swap couldn't start"
	}
	return "llama-swap stopped with a problem"
}

func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// notices is what the app has posted: the standing notices shown now, and
// a wake-up for watchNotices.
type notices struct {
	mu    sync.Mutex
	shown map[string]notice
	wake  chan struct{}
}

// startNotices sets up notifications at launch: it clears any left from an
// earlier run, asks macOS for permission the first time (never at a
// problem, the worst moment for the question), and follows llama-swap's
// state and the clicks.
func (a *app) startNotices() {
	a.notices.shown = map[string]notice{}
	a.notices.wake = make(chan struct{}, 1)
	macos.SetUpNotifications()
	macos.RemoveAllNotifications()
	if a.currentPrefs().Notifies() {
		go a.askForNotifications()
	}
	go a.watchNotices()
	go a.watchNoticeClicks()
}

// askForNotifications asks macOS for permission, which shows its prompt
// only if it's never been asked.
func (a *app) askForNotifications() {
	if macos.NotificationSettings().Authorization != "not determined" {
		return
	}
	ok, err := macos.AuthorizeNotifications()
	a.sup.Logf("notifications: macOS allows them: %v%s", ok, errSuffix(err))
}

func errSuffix(err error) string {
	if err == nil {
		return ""
	}
	return " (" + err.Error() + ")"
}

// noticesChanged wakes watchNotices; render calls it with every change.
func (a *app) noticesChanged() {
	if a.notices.wake == nil {
		return // before startNotices
	}
	select {
	case a.notices.wake <- struct{}{}:
	default: // it's already due to look
	}
}

// watchNotices keeps the standing notices in step with llama-swap's state:
// it posts those that begin or change, once past any grace, and removes
// those that end. A problem within its grace sends no change when it simply
// lasts, so a timer wakes it when the grace is over.
func (a *app) watchNotices() {
	since := map[string]time.Time{}
	var timer *time.Timer
	for range a.notices.wake {
		p := a.currentPrefs()
		want := map[string]notice{}
		if p.Notifies() {
			want = standingNotices(a.sup.Status(), a.snapshot(), p.UnloadOnGPUFault)
		}
		want, next := due(want, since, time.Now())
		if timer != nil {
			timer.Stop()
		}
		if next > 0 {
			timer = time.AfterFunc(next, a.noticesChanged)
		}
		a.notices.mu.Lock()
		shown := maps.Clone(a.notices.shown)
		a.notices.mu.Unlock()
		for id := range shown {
			if _, ok := want[id]; !ok {
				macos.RemoveNotification(id)
				delete(shown, id)
			}
		}
		for id, n := range want {
			if old, ok := shown[id]; ok && old == n {
				continue
			}
			if err := macos.Notify(id, n.title, n.body, n.category, n.timeSensitive); err != nil {
				a.sup.Logf("notifications: couldn't post %q: %v", n.title, err)
			}
			shown[id] = n
		}
		a.notices.mu.Lock()
		a.notices.shown = shown
		a.notices.mu.Unlock()
	}
}

// due returns the notices wanted now, less those whose grace hasn't run
// out, and how long until the first of those is due (0 for none). since
// holds when each graced notice was first wanted; due adds to it, and drops
// those no longer wanted, so a problem that comes back waits again.
func due(want map[string]notice, since map[string]time.Time, now time.Time) (map[string]notice, time.Duration) {
	for id := range since {
		if _, ok := want[id]; !ok {
			delete(since, id)
		}
	}
	shown := map[string]notice{}
	var next time.Duration
	for id, n := range want {
		if !n.grace {
			shown[id] = n
			continue
		}
		first, ok := since[id]
		if !ok {
			first, since[id] = now, now
		}
		if left := noticeGrace - now.Sub(first); left > 0 {
			if next == 0 || left < next {
				next = left
			}
			continue
		}
		shown[id] = n
	}
	return shown, next
}

// postEvent posts a one-off notice, if notifications are on.
func (a *app) postEvent(id string, n notice) {
	if !a.currentPrefs().Notifies() {
		return
	}
	if err := macos.Notify(id, n.title, n.body, n.category, n.timeSensitive); err != nil {
		a.sup.Logf("notifications: couldn't post %q: %v", n.title, err)
	}
}

// notifying reports whether notifications replace the alerts now: the box
// is ticked, and macOS shows them. If macOS doesn't, the alerts stay, so a
// problem is never left unseen.
func (a *app) notifying() bool {
	return a.currentPrefs().Notifies() && macos.NotificationSettings().Shown()
}

// notificationsSaved follows the Native notifications box after a Save:
// ticked, macOS is asked (if it never has been); unticked, the app's
// notifications are cleared from Notification Centre.
func (a *app) notificationsSaved(old, p bool) {
	switch {
	case p && !old:
		go a.askForNotifications()
	case old && !p:
		a.notices.mu.Lock()
		a.notices.shown = map[string]notice{}
		a.notices.mu.Unlock()
		macos.RemoveAllNotifications()
	}
	a.noticesChanged()
}

// watchNoticeClicks answers clicks on the app's notifications: Unload on a
// GPU fault's, and a click on a failure's or a fault's opens its alert, if
// the problem's still there.
func (a *app) watchNoticeClicks() {
	for {
		r := macos.NextNotificationResponse()
		switch {
		case r.ID == noticeUpdate:
			go a.openUpdate()
		case r.ID == noticeInstalled:
			if v, ok := update.ParseVersion(a.installedNewer()); ok {
				go a.restartInto(v, true)
			}
		case r.ID == noticeFailed:
			if st := a.sup.Status(); st.State == supervisor.Failed {
				go macos.AlertWithOutput(failureTitle(st), st.Message, a.recentOutput(10))
			}
		case strings.HasPrefix(r.ID, noticeFaultPrefix):
			model := strings.TrimPrefix(r.ID, noticeFaultPrefix)
			if r.Action == "unload" {
				go a.unloadModel(model)
			} else if a.stillFaulted(model) {
				go a.offerUnload(model)
			}
		}
	}
}

func (a *app) stillFaulted(model string) bool {
	for _, m := range a.snapshot().Faulted() {
		if m.ID == model {
			return true
		}
	}
	return false
}
