package main

import (
	"strings"
	"testing"

	"github.com/alron/llama-swap-mac-launcher/internal/macos"
)

const gb = 1024 // MB

// A 96 GB Mac, as the development Mac is: macOS's default about 78 GB.
func mac96(helper macos.LoginItemStatus, limit, stored int64) wiredState {
	s := wiredState{totalMB: 96 * gb, limitMB: limit, storedMB: stored, helper: helper, running: helper == macos.LoginItemEnabled}
	if limit == 0 {
		s.defaultMB = 79626
	}
	return s
}

// What a Save does, from what's in effect and what was saved. Only a
// change to the limit is confirmed; a helper that changes nothing is
// just removed.
func TestWiredPlan(t *testing.T) {
	const (
		off      = macos.LoginItemNotRegistered
		enabled  = macos.LoginItemEnabled
		approval = macos.LoginItemRequiresApproval
	)
	waiting := mac96(approval, 0, 0)
	waiting.pendingMB = 84 * gb
	unusable := mac96(off, 0, 0)
	unusable.unusable = "Only on Macs with more than 32 GB"
	stalled := mac96(enabled, 80*gb, 80*gb) // allowed, but launchd isn't running it
	stalled.running = false
	for _, tt := range []struct {
		name string
		s    wiredState
		want wiredWant
		do   wiredAction
	}{
		{"off, and off", mac96(off, 0, 0), wiredWant{}, wiredNothing},
		{"turned on", mac96(off, 0, 0), wiredWant{true, 84 * gb}, wiredSet},
		{"in effect", mac96(enabled, 84*gb, 84*gb), wiredWant{true, 84 * gb}, wiredNothing},
		{"changed", mac96(enabled, 84*gb, 84*gb), wiredWant{true, 80 * gb}, wiredSet},
		{"changed back by something else", mac96(enabled, 70*gb, 84*gb), wiredWant{true, 84 * gb}, wiredSet},
		{"allowed, nothing set yet", mac96(enabled, 0, 0), wiredWant{true, 84 * gb}, wiredSet},
		{"turned off", mac96(enabled, 84*gb, 84*gb), wiredWant{}, wiredOff},
		{"turned off, set back by something else", mac96(enabled, 0, 84*gb), wiredWant{}, wiredOff},
		// Nothing of the app's in effect: no confirmation, and someone
		// else's limit is left alone.
		{"allowed with nothing, turned off", mac96(enabled, 0, 0), wiredWant{}, wiredDrop},
		{"someone else's, helper allowed, turned off", mac96(enabled, 70*gb, 0), wiredWant{}, wiredDrop},
		{"never allowed, turned off", mac96(approval, 0, 0), wiredWant{}, wiredDrop},
		{"waiting, same again", waiting, wiredWant{true, 84 * gb}, wiredRetry}, // no confirmation: it was confirmed
		{"waiting, another", waiting, wiredWant{true, 80 * gb}, wiredSet},
		{"waiting, turned off", waiting, wiredWant{}, wiredDrop},
		{"not allowed, nothing waiting, on", mac96(approval, 0, 0), wiredWant{true, 84 * gb}, wiredSet},
		{"someone else's, left off", mac96(off, 70*gb, 0), wiredWant{}, wiredNothing},
		{"unusable", unusable, wiredWant{true, 84 * gb}, wiredNothing},
		// Allowed but not running: a Save starts it again, even for the
		// limit in effect; turning off still confirms, then sets the default.
		{"not running, same again", stalled, wiredWant{true, 80 * gb}, wiredSet},
		{"not running, turned off", stalled, wiredWant{}, wiredOff},
	} {
		if got := wiredPlan(tt.s, tt.want); got != tt.do {
			t.Errorf("%s: %d, want %d", tt.name, got, tt.do)
		}
	}
}

// Settings' line says what's in effect, and notes a limit the app didn't
// set.
func TestWiredDescribe(t *testing.T) {
	const (
		off      = macos.LoginItemNotRegistered
		enabled  = macos.LoginItemEnabled
		approval = macos.LoginItemRequiresApproval
	)
	waiting := mac96(approval, 0, 0)
	waiting.pendingMB = 84 * gb
	for _, tt := range []struct {
		s    wiredState
		want string
	}{
		{mac96(off, 0, 0), "Now macOS's default, about 78 GB."},
		{mac96(enabled, 84*gb, 84*gb), "In effect: 84 GB, set again by the app's helper at every startup."},
		{waiting, "Waiting for you to allow the app's helper in System Settings"},
		{mac96(approval, 0, 0), "The app's helper isn't allowed to run"},
		{mac96(approval, 80*gb, 80*gb), "The app's helper isn't allowed to run, so it won't set 80 GB at startup: allow it again"},
		{mac96(enabled, 0, 0), "The app's helper is allowed, but has no limit to set"},
		{mac96(enabled, 70*gb, 84*gb), "Now 70 GB, set by something other than the app"},
		{mac96(off, 84*gb, 84*gb), "Now 84 GB, set by the app's helper, which has since been turned off"},
		{mac96(off, 230000, 0), "Now 224.6 GB, set by something other than the app"},
		{stalledState(), "macOS has the app's helper allowed, but isn't running it"},
	} {
		if got := tt.s.describe(); !strings.HasPrefix(got, tt.want) {
			t.Errorf("%+v: %q, want it to start %q", tt.s, got, tt.want)
		}
	}
}

// The field, read and checked as the helper checks it.
func TestWiredFrom(t *testing.T) {
	s := mac96(macos.LoginItemNotRegistered, 0, 0)
	for _, tt := range []struct {
		box  bool
		text string
		want wiredWant
		ok   bool
	}{
		{false, "anything", wiredWant{}, true},
		{true, "84", wiredWant{true, 84 * gb}, true},
		{true, " 88 GB ", wiredWant{true, 88 * gb}, true},
		{true, "89", wiredWant{}, false},
		{true, "7", wiredWant{}, false},
		{true, "8", wiredWant{true, 8 * gb}, true},
		{true, "84.5", wiredWant{}, false},
		{true, "", wiredWant{}, false},
		{true, "-84", wiredWant{}, false},
		{true, "99999999999999", wiredWant{}, false},
	} {
		got, err := wiredFrom(macos.PreferencesForm{WiredLimit: tt.box, WiredLimitGB: tt.text}, s)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("%v %q: %+v, %v", tt.box, tt.text, got, err)
		}
		if err != nil {
			if _, ok := err.(tweakError); !ok {
				t.Errorf("%q: %T, want a tweakError, so the dialog comes back on Tweaks", tt.text, err)
			}
		}
	}
	// Where it can't be changed, the greyed box comes back as it went in.
	s.unusable = "no"
	if got, err := wiredFrom(macos.PreferencesForm{WiredLimit: true, WiredLimitGB: "84"}, s); err != nil || got != (wiredWant{}) {
		t.Errorf("unusable: %+v, %v", got, err)
	}
}

// The field starts at what the app is waiting to set, what it sets,
// someone else's limit, or the suggestion.
func TestFillWiredForm(t *testing.T) {
	waiting := mac96(macos.LoginItemRequiresApproval, 0, 0)
	waiting.pendingMB = 80 * gb
	for _, tt := range []struct {
		s     wiredState
		box   bool
		field string
	}{
		{mac96(macos.LoginItemNotRegistered, 0, 0), false, "84"},
		{mac96(macos.LoginItemEnabled, 86*gb, 86*gb), true, "86"},
		{waiting, true, "80"},
		{mac96(macos.LoginItemNotRegistered, 70*gb, 0), false, "70"},
	} {
		var f macos.PreferencesForm
		fillWiredForm(&f, tt.s)
		if f.WiredLimit != tt.box || f.WiredLimitGB != tt.field {
			t.Errorf("%+v: box %v, field %q", tt.s, f.WiredLimit, f.WiredLimitGB)
		}
	}
	var f macos.PreferencesForm
	fillWiredForm(&f, mac96(macos.LoginItemNotRegistered, 0, 0))
	if f.WiredTotalGB != 96 || f.WiredSuggestedGB != 84 || f.WiredMinGB != 8 || f.WiredMaxGB != 88 || !f.WiredUsable {
		t.Errorf("bounds: %+v", f)
	}
}

func stalledState() wiredState {
	s := mac96(macos.LoginItemEnabled, 80*gb, 80*gb)
	s.running = false
	return s
}

// A helper never registered is reported as not found (no record of it
// yet): that's not registered, unless its plist really is missing.
func TestNeverRegistered(t *testing.T) {
	for _, tt := range []struct {
		st       macos.LoginItemStatus
		inBundle bool
		want     macos.LoginItemStatus
	}{
		{macos.LoginItemNotFound, true, macos.LoginItemNotRegistered},
		{macos.LoginItemNotFound, false, macos.LoginItemNotFound},
		{macos.LoginItemEnabled, true, macos.LoginItemEnabled},
		{macos.LoginItemRequiresApproval, true, macos.LoginItemRequiresApproval},
		{macos.LoginItemNotRegistered, false, macos.LoginItemNotRegistered},
	} {
		if got := neverRegistered(tt.st, tt.inBundle); got != tt.want {
			t.Errorf("%d, in the bundle %v: %d, want %d", tt.st, tt.inBundle, got, tt.want)
		}
	}
}
