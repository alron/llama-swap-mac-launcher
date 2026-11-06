package main

// Restarting into a newer version installed over the running app (TODO
// item 65). Homebrew's upgrade moves the running app's bundle aside and
// puts the new one in its place, but can't reopen the app without
// Automation access, so the app watches its own path and restarts itself:
// a Quit as usual, then the new copy opened through Launch Services once
// the old one has exited (its control socket is the single-instance lock).

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
	"github.com/alron/llama-swap-mac-launcher/internal/update"
)

const (
	// installPoll is how often the app looks at its own bundle on disk.
	installPoll = 10 * time.Second
	// settleTime is how long a new copy must have been there before the app
	// restarts into it: Homebrew still has steps after moving it in, such
	// as passing on the old copy's Gatekeeper approval.
	settleTime = 15 * time.Second
	// noticeUpdated is the notification after an update restart, and
	// noticeInstalled the one for a newer version installed that the app
	// won't restart into by itself; clicking it restarts.
	noticeUpdated   = "update-restarted"
	noticeInstalled = "update-installed"
	// homebrewHelp is the README's Homebrew instructions, linked from the
	// Tweaks tab when Homebrew doesn't manage this copy.
	homebrewHelp = homepage + "#with-homebrew"
)

// installed is what the app has seen at its own path: a newer version, if
// one is there, since when, and whether it's restarting.
type installed struct {
	mu         sync.Mutex
	version    string // a newer version on disk where the app runs, or ""
	seen       time.Time
	restarting bool
	refused    string // the version whose signature last failed, logged once
	// restarted is the marker the previous copy left, if this launch is
	// an update restart.
	restarted *update.Restarted
}

// shouldRestart decides, for a newer version seen at the app's own path,
// whether to restart into it now: with Restart on update ticked, for a
// copy Homebrew manages, once the copy has settled, while llama-swap has
// no requests in flight (the maintainer: wait until idle), and with no
// dialog open, whose edits a Quit would lose.
func shouldRestart(newer bool, seenFor time.Duration, on, homebrew, idle, dialog bool) bool {
	return newer && on && homebrew && seenFor >= settleTime && idle && !dialog
}

// notifyAfterRestart reports whether Notify after an update restart is in
// effect: ticked, with Restart on update on, Homebrew managing the copy,
// and Native notifications on, the conditions under which Settings leaves
// it enabled.
func notifyAfterRestart(p prefs.Prefs, homebrew bool) bool {
	return p.NotifyOnUpdateRestart && p.RestartsOnUpdate() && homebrew && p.Notifies()
}

// takeRestarted reads the marker the previous copy left when it restarted
// into this one, before the pause is restored and notices start.
func (a *app) takeRestarted() {
	if r, ok := update.TakeRestarted(a.paths.RestartedFile()); ok {
		a.install.restarted = &r
	}
}

// pausedByUpdate reports whether this launch is an update restart from a
// copy whose inference was paused: kept, whatever item 59's box says.
func (a *app) pausedByUpdate() bool {
	return a.install.restarted != nil && a.install.restarted.Paused
}

// startInstallWatch reads the app's own signing requirement, while the
// bundle on disk is still the one running, says so if this launch is an
// update restart, and looks at the bundle every installPoll.
func (a *app) startInstallWatch() {
	if err := macos.CaptureSelfRequirement(); err != nil {
		a.sup.Logf("can't check a new copy's signature, so won't restart into one by itself: %v", err)
	}
	if r := a.install.restarted; r != nil {
		a.sup.Logf("restarted into %s, installed over %s", version, r.From)
		// Only while the box is in effect, as Settings shows it: ticked and
		// not greyed out, so Restart on update on, for a copy Homebrew
		// manages (a greyed box keeps its tick, but doesn't count).
		if notifyAfterRestart(a.currentPrefs(), a.updates.homebrew) {
			a.postEvent(noticeUpdated, notice{
				title: "Llama Swap Launcher restarted into " + version,
				body:  "It was updated from " + r.From + ", and restarted itself.",
			})
		}
	}
	bundle := bundlePath()
	go func() {
		for range time.Tick(installPoll) {
			a.lookAtInstall(bundle)
		}
	}()
}

// lookAtInstall reads the version on disk at the app's path, and restarts
// into it when that's due.
func (a *app) lookAtInstall(bundle string) {
	cur, okCur := update.ParseVersion(version)
	v, err := update.InstalledVersion(bundle)
	newer := okCur && err == nil && v.Newer(cur) // missing mid-upgrade, or the same: nothing yet
	now := time.Now()
	a.install.mu.Lock()
	changed := false
	switch {
	case newer && a.install.version != v.String():
		a.install.version, a.install.seen, changed = v.String(), now, true
	case !newer && a.install.version != "":
		a.install.version, changed = "", true
	}
	seenFor := now.Sub(a.install.seen)
	a.install.mu.Unlock()
	p := a.currentPrefs()
	auto := p.RestartsOnUpdate() && a.updates.homebrew
	if changed {
		if newer {
			a.sup.Logf("version %s is installed over this one (%s)", v, version)
			// Not restarting by itself: say so, once per version (the
			// maintainer); a click restarts, as the menu's line does.
			if !auto {
				a.postEvent(noticeInstalled, notice{
					title: "Llama Swap Launcher " + v.String() + " is installed",
					body:  "This is still " + version + ". Click to restart into the new version.",
				})
			}
		} else {
			macos.RemoveNotification(noticeInstalled)
		}
		a.render()
	}
	if shouldRestart(newer, seenFor, p.RestartsOnUpdate(), a.updates.homebrew, a.idle(), a.inDialog.Load()) {
		go a.restartInto(v, false)
	}
}

// idle reports whether llama-swap has no requests in flight, or isn't
// running.
func (a *app) idle() bool {
	st := a.sup.Status()
	if st.State != supervisor.Running {
		return true
	}
	snap, ok := a.healthFor(st.Pid)
	return !ok || len(snap.Busy("")) == 0
}

// installedNewer is the newer version installed over the running app, if
// any.
func (a *app) installedNewer() string {
	a.install.mu.Lock()
	defer a.install.mu.Unlock()
	return a.install.version
}

// restartInto quits and reopens the app as version v, once its signature
// checks out. manual is the menu's Restart into Version…, which says why
// it can't.
func (a *app) restartInto(v update.Version, manual bool) {
	a.install.mu.Lock()
	if a.install.restarting {
		a.install.mu.Unlock()
		return
	}
	a.install.restarting = true
	a.install.mu.Unlock()
	fail := func(why string) {
		a.install.mu.Lock()
		a.install.restarting = false
		logIt := manual || a.install.refused != v.String()
		a.install.refused = v.String()
		a.install.mu.Unlock()
		if logIt {
			a.sup.Logf("can't restart into version %s: %s", v, why)
		}
		if manual {
			macos.Alert("Can't restart into version "+v.String(), sentence(why)+".")
		}
	}
	bundle := bundlePath()
	// A half-copied bundle, or one not signed as this app is, is never
	// opened by itself; it's looked at again next time.
	if err := macos.MatchesSelf(bundle); err != nil {
		fail("its signature doesn't check out: " + err.Error())
		return
	}
	a.mu.Lock()
	paused := a.pauseWanted
	a.mu.Unlock()
	if err := (update.Restarted{From: version, To: v.String(), Paused: paused}).Save(a.paths.RestartedFile()); err != nil {
		a.sup.Logf("couldn't note the update restart: %v", err)
	}
	if err := reopenAfterExit(bundle); err != nil {
		_ = os.Remove(a.paths.RestartedFile()) // best effort: no restart after all
		fail("couldn't arrange to open it again: " + err.Error())
		return
	}
	a.sup.Logf("restarting into version %s, installed over this one (%s)", v, version)
	a.quit()
}

// reopenAfterExit starts a detached shell that waits for this process to
// exit (up to two minutes), then opens bundle through Launch Services, as
// a login or a click in the Finder would, with a plain environment.
func reopenAfterExit(bundle string) error {
	const script = `pid=$1; app=$2; i=0
while kill -0 "$pid" 2>/dev/null && [ "$i" -lt 600 ]; do sleep 0.2; i=$((i+1)); done
exec /usr/bin/open "$app"`
	cmd := exec.Command("/bin/sh", "-c", script, "sh", strconv.Itoa(os.Getpid()), bundle) // #nosec G204 -- a fixed script; the arguments are this app's pid and bundle
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // outlives the app
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
