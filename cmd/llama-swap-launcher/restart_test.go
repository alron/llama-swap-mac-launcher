package main

import (
	"testing"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// A newer copy installed over the app is restarted into only with the box
// on, for a copy Homebrew manages, once it has settled, while llama-swap is
// idle, and with no dialog open (TODO item 65).
func TestShouldRestart(t *testing.T) {
	ok := func(mod func(*[6]bool, *time.Duration)) bool {
		b := [6]bool{true, true, true, true, true, false} // newer, on, homebrew, idle, (unused), dialog
		d := settleTime
		mod(&b, &d)
		return shouldRestart(b[0], d, b[1], b[2], b[3], b[5])
	}
	if !ok(func(*[6]bool, *time.Duration) {}) {
		t.Error("everything right, and no restart")
	}
	for name, mod := range map[string]func(*[6]bool, *time.Duration){
		"nothing newer":       func(b *[6]bool, _ *time.Duration) { b[0] = false },
		"the box off":         func(b *[6]bool, _ *time.Duration) { b[1] = false },
		"not Homebrew's copy": func(b *[6]bool, _ *time.Duration) { b[2] = false },
		"requests in flight":  func(b *[6]bool, _ *time.Duration) { b[3] = false },
		"a dialog open":       func(b *[6]bool, _ *time.Duration) { b[5] = true },
		"not settled":         func(_ *[6]bool, d *time.Duration) { *d = settleTime - time.Second },
	} {
		if ok(mod) {
			t.Errorf("%s: restarted", name)
		}
	}
}

// Restart on update is on by default and written only when off; the
// notification after one is off by default and written only when on.
func TestRestartBoxesFromForm(t *testing.T) {
	f := formFrom(prefs.Defaults(), nil)
	if !f.RestartOnUpdate || f.NotifyOnUpdateRestart {
		t.Errorf("shown %v and %v by default", f.RestartOnUpdate, f.NotifyOnUpdateRestart)
	}
	for _, on := range []bool{true, false} {
		f.RestartOnUpdate, f.NotifyOnUpdateRestart = on, on
		p, _, err := prefsFrom(f)
		if err != nil || p.RestartsOnUpdate() != on || (p.RestartOnUpdate == nil) != on || p.NotifyOnUpdateRestart != on {
			t.Errorf("ticked %v: restart %v (stored %v), notify %v, %v", on, p.RestartsOnUpdate(), p.RestartOnUpdate, p.NotifyOnUpdateRestart, err)
		}
		if g := formFrom(p, nil); g.RestartOnUpdate != on || g.NotifyOnUpdateRestart != on {
			t.Errorf("ticked %v: shown otherwise again", on)
		}
	}
}

// The notification after an update restart counts only while Settings
// leaves its box enabled: a greyed box keeps its tick, but doesn't post.
func TestNotifyAfterRestart(t *testing.T) {
	off := false
	on := prefs.Defaults()
	on.NotifyOnUpdateRestart = true
	if !notifyAfterRestart(on, true) {
		t.Error("everything on, and no notification")
	}
	noRestart, noNotices, unticked := on, on, on
	noRestart.RestartOnUpdate = &off
	noNotices.Notifications = &off
	unticked.NotifyOnUpdateRestart = false
	for name, tt := range map[string]struct {
		p        prefs.Prefs
		homebrew bool
	}{
		"Restart on update off (the maintainer's case)": {noRestart, true},
		"Native notifications off":                      {noNotices, true},
		"not Homebrew's copy":                           {on, false},
		"unticked":                                      {unticked, true},
	} {
		if notifyAfterRestart(tt.p, tt.homebrew) {
			t.Errorf("%s: notified", name)
		}
	}
}
