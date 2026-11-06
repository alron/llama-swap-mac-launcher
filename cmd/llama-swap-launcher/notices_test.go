package main

import (
	"fmt"
	"maps"
	"slices"
	"testing"

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
		}
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
