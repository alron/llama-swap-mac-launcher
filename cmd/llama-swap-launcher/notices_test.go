package main

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// Which of llama-swap's states post which notices (TODO item 60), and
// which are time-sensitive. Pauses, a llama-swap starting or stopping, and
// a healthy one post none.
func TestStandingNotices(t *testing.T) {
	running := supervisor.Status{State: supervisor.Running}
	ready := health.Snapshot{Healthy: true, WasHealthy: true}
	faulted := health.Snapshot{Healthy: true, WasHealthy: true, Models: []health.Model{
		{ID: "a", State: "ready", Faulted: true},
		{ID: "b", State: "ready"},
		{ID: "c", State: "ready", Faulted: true},
	}}
	for _, tt := range []struct {
		name       string
		st         supervisor.Status
		snap       health.Snapshot
		autoUnload bool
		want       map[string]bool // id: time-sensitive
	}{
		{"ready", running, ready, false, map[string]bool{}},
		{"paused", running, health.Snapshot{Healthy: true, WasHealthy: true, CanPause: true, Paused: true}, false, map[string]bool{}},
		{"starting", running, health.Snapshot{}, false, map[string]bool{}},
		{"stopped", supervisor.Status{State: supervisor.Stopped}, health.Snapshot{}, false, map[string]bool{}},
		{"waiting to restart", supervisor.Status{State: supervisor.Waiting, Message: "exited (x); restarting in 1s"}, health.Snapshot{}, false, map[string]bool{}},
		{"failed", supervisor.Status{State: supervisor.Failed, Message: "it kept crashing"}, health.Snapshot{}, false,
			map[string]bool{noticeFailed: true}},
		{"not responding", running, health.Snapshot{WasHealthy: true}, false, map[string]bool{noticeNotResponding: true}},
		{"refused", running, health.Snapshot{Problem: "llama-swap refused the app's API key"}, false, map[string]bool{noticeRefused: false}},
		{"GPU faults", running, faulted, false, map[string]bool{noticeFaultPrefix + "a": true, noticeFaultPrefix + "c": true}},
		// Unloaded at once instead: an event, not a standing notice.
		{"GPU faults, unloaded automatically", running, faulted, true, map[string]bool{}},
	} {
		got := standingNotices(tt.st, tt.snap, tt.autoUnload)
		ids := slices.Sorted(maps.Keys(got))
		if want := slices.Sorted(maps.Keys(tt.want)); !slices.Equal(ids, want) {
			t.Errorf("%s: notices %v, want %v", tt.name, ids, want)
			continue
		}
		for id, n := range got {
			if n.timeSensitive != tt.want[id] {
				t.Errorf("%s: %s time-sensitive %v", tt.name, id, n.timeSensitive)
			}
			if n.title == "" || n.body == "" {
				t.Errorf("%s: %s has an empty title or body: %+v", tt.name, id, n)
			}
			if (n.category == faultCategory) != (len(id) > len(noticeFaultPrefix) && id[:len(noticeFaultPrefix)] == noticeFaultPrefix) {
				t.Errorf("%s: %s has category %q; only GPU faults get Unload", tt.name, id, n.category)
			}
			// Only problems that can pass in a moment wait out the grace
			// (TODO item 89); a failure and a GPU fault are definite.
			if graced := id == noticeNotResponding || id == noticeRefused; n.grace != graced {
				t.Errorf("%s: %s grace %v, want %v", tt.name, id, n.grace, graced)
			}
		}
	}
}

// A problem marked grace is posted only once it has lasted noticeGrace,
// and one that ends first posts nothing: a config reload drops the event
// stream for a moment, which flashed "llama-swap isn't responding", sound
// and all (TODO item 89). Others are posted at once.
func TestDue(t *testing.T) {
	hang := notice{title: "llama-swap isn't responding", body: "b", timeSensitive: true, grace: true}
	failed := notice{title: "llama-swap stopped with a problem", body: "b", timeSensitive: true}
	since := map[string]time.Time{}
	t0 := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration, want map[string]notice) (map[string]notice, time.Duration) {
		t.Helper()
		return due(want, since, t0.Add(d))
	}

	// A reload: the stream drops and is back a quarter of a second later.
	if shown, next := at(0, map[string]notice{noticeNotResponding: hang}); len(shown) != 0 || next != noticeGrace {
		t.Fatalf("at once: %v, next %v", shown, next)
	}
	if shown, next := at(250*time.Millisecond, map[string]notice{}); len(shown) != 0 || next != 0 || len(since) != 0 {
		t.Fatalf("reconnected: %v, next %v, since %v", shown, next, since)
	}

	// A hang: held back, then posted once the grace is over, and the time
	// left is counted from when it began.
	at(time.Minute, map[string]notice{noticeNotResponding: hang})
	if shown, next := at(time.Minute+4*time.Second, map[string]notice{noticeNotResponding: hang}); len(shown) != 0 || next != 6*time.Second {
		t.Fatalf("4s in: %v, next %v", shown, next)
	}
	if shown, next := at(time.Minute+noticeGrace, map[string]notice{noticeNotResponding: hang}); shown[noticeNotResponding] != hang || next != 0 {
		t.Fatalf("10s in: %v, next %v", shown, next)
	}

	// A failure is posted at once, beside a problem still in its grace.
	shown, next := at(2*time.Minute, map[string]notice{noticeFailed: failed})
	if len(shown) != 1 || shown[noticeFailed] != failed || next != 0 {
		t.Fatalf("failed: %v, next %v", shown, next)
	}
	shown, next = at(3*time.Minute, map[string]notice{noticeFailed: failed, noticeNotResponding: hang})
	if len(shown) != 1 || next != noticeGrace {
		t.Fatalf("failed, and a hang beginning: %v, next %v", shown, next)
	}
}

// The failure's notice says how it failed, as its alert does.
func TestFailedNoticeTitle(t *testing.T) {
	for err, want := range map[error]string{
		supervisor.ErrQuarantined:                            "macOS blocked llama-swap",
		supervisor.ErrNotStarted:                             "llama-swap couldn't start",
		fmt.Errorf("crash loop"):                             "llama-swap stopped with a problem",
		fmt.Errorf("wrapped: %w", supervisor.ErrQuarantined): "macOS blocked llama-swap",
	} {
		n := standingNotices(supervisor.Status{State: supervisor.Failed, Err: err}, health.Snapshot{}, false)[noticeFailed]
		if n.title != want {
			t.Errorf("%v: %q, want %q", err, n.title, want)
		}
	}
}

// A restart's notice says why it exited, from the supervisor's message.
func TestRestartedNotice(t *testing.T) {
	for waiting, want := range map[string]string{
		"exited (signal: killed); restarting in 1s": "It exited (signal: killed) unexpectedly, and the app started it again.",
		"": "It exited unexpectedly, and the app started it again.",
	} {
		if got := restartedNotice(waiting).body; got != want {
			t.Errorf("%q: %q, want %q", waiting, got, want)
		}
	}
}

// Native notifications is on by default, written only when it's off, and
// round-trips through the dialog.
func TestNotificationsFromForm(t *testing.T) {
	f := formFrom(prefs.Defaults(), nil)
	if !f.Notifications {
		t.Error("shown unticked by default")
	}
	for _, on := range []bool{true, false} {
		f.Notifications = on
		p, _, err := prefsFrom(f)
		if err != nil || p.Notifies() != on || (p.Notifications == nil) != on {
			t.Errorf("ticked %v: got %v (stored %v), %v", on, p.Notifies(), p.Notifications, err)
		}
		if formFrom(p, nil).Notifications != on {
			t.Errorf("ticked %v: shown otherwise again", on)
		}
	}
}
