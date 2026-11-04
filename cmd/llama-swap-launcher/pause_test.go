package main

import (
	"io"
	"os"
	"testing"

	"github.com/alron/llama-swap-mac-launcher/internal/paths"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// launch is the app as it starts with prefs p, in dir: it reads a pause
// kept on disk (TODO item 59).
func launch(dir string, p prefs.Prefs) *app {
	a := &app{
		paths: paths.Paths{SupportDir: dir},
		sup:   supervisor.New(supervisor.Options{Log: io.Discard}),
		prefs: p,
	}
	a.restorePause()
	return a
}

// wantPause sets whether a pause is wanted, as Pause All, Resume All or a
// pause seen in llama-swap's web UI do.
func (a *app) wantPause(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pauseWanted = on
	a.recordPause()
}

func (a *app) setPrefs(p prefs.Prefs) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prefs = p
	a.recordPause()
}

func paused(a *app) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pauseWanted
}

// A pause outlasts quitting the app only with both Tweaks boxes ticked,
// and a resume, or unticking either, forgets it.
func TestPauseKeptAcrossLaunches(t *testing.T) {
	dir := t.TempDir()
	file := paths.Paths{SupportDir: dir}.PauseFile()
	exists := func() bool {
		_, err := os.Stat(file)
		return err == nil
	}
	keep := prefs.Defaults()
	keep.KeepPausedOnAppRestart = true
	off := false
	notAcrossRestarts := keep
	notAcrossRestarts.KeepPausedOnRestart = &off

	a := launch(dir, keep)
	if paused(a) || exists() {
		t.Fatal("a first launch is paused")
	}
	a.wantPause(true)
	if !exists() {
		t.Fatal("a pause wasn't recorded")
	}
	before, _ := os.ReadFile(file)
	a = launch(dir, keep)
	if !paused(a) {
		t.Error("the pause didn't outlast the app's restart")
	}
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Errorf("the pause's time changed at launch: %q, then %q", before, after)
	}
	a.wantPause(false)
	if exists() {
		t.Error("Resume left the file")
	}
	if a = launch(dir, keep); paused(a) {
		t.Error("paused again after a resume")
	}

	// Unticking either box, in Settings or by hand, forgets it.
	for _, p := range []prefs.Prefs{prefs.Defaults(), notAcrossRestarts} {
		a = launch(dir, keep)
		a.wantPause(true)
		a.setPrefs(p)
		if exists() {
			t.Errorf("%+v: still recorded", p)
		}
		a.setPrefs(keep) // ticked again while paused: recorded again
		if !exists() {
			t.Errorf("%+v, then ticked again while paused: not recorded", p)
		}
		if a = launch(dir, p); paused(a) || exists() {
			t.Errorf("%+v: a launch kept the pause (file there: %v)", p, exists())
		}
	}

	// Before the launch has read it, nothing removes it.
	a = launch(dir, keep)
	a.wantPause(true)
	early := &app{paths: paths.Paths{SupportDir: dir}, sup: supervisor.New(supervisor.Options{Log: io.Discard}), prefs: prefs.Defaults()}
	early.wantPause(false)
	early.setPrefs(prefs.Defaults())
	if !exists() {
		t.Error("removed before the launch read it")
	}
}
