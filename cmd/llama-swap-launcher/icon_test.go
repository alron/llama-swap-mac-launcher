package main

import (
	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

func TestLookFor(t *testing.T) {
	running := supervisor.Status{State: supervisor.Running}
	ready := health.Model{ID: "m", State: "ready"}
	tests := []struct {
		name string
		st   supervisor.Status
		snap health.Snapshot
		want menuBarLook
	}{
		{"stopped", supervisor.Status{State: supervisor.Stopped}, health.Snapshot{}, menuBarLook{macos.ColorFaded, macos.ColorFaded}},
		{"failed", supervisor.Status{State: supervisor.Failed}, health.Snapshot{}, menuBarLook{macos.ColorNormal, macos.ColorAlert}},
		{"restarting", supervisor.Status{State: supervisor.Waiting}, health.Snapshot{}, menuBarLook{macos.ColorNormal, macos.ColorBusy}},
		{"starting", running, health.Snapshot{}, menuBarLook{macos.ColorNormal, macos.ColorBusy}},
		{"ready", running, health.Snapshot{Healthy: true, WasHealthy: true, Models: []health.Model{ready}}, menuBarLook{macos.ColorNormal, macos.ColorNormal}},
		{"model loading", running, health.Snapshot{Healthy: true, WasHealthy: true, Models: []health.Model{{ID: "m", State: "starting"}}}, menuBarLook{macos.ColorNormal, macos.ColorBusy}},
		{"peer model doesn't count", running, health.Snapshot{Healthy: true, WasHealthy: true, Models: []health.Model{{ID: "p/m", State: "starting", PeerID: "p"}}}, menuBarLook{macos.ColorNormal, macos.ColorNormal}},
		{"GPU fault", running, health.Snapshot{Healthy: true, WasHealthy: true, Models: []health.Model{{ID: "m", State: "ready", Faulted: true}}}, menuBarLook{macos.ColorNormal, macos.ColorAlert}},
		{"not responding", running, health.Snapshot{WasHealthy: true}, menuBarLook{macos.ColorNormal, macos.ColorAlert}},
		{"refusing the app", running, health.Snapshot{Problem: "wants an API key"}, menuBarLook{macos.ColorNormal, macos.ColorAlert}},
	}
	for _, tt := range tests {
		if got := lookFor(tt.st, tt.snap); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestDescribeRefusal(t *testing.T) {
	running := supervisor.Status{State: supervisor.Running}
	snap := health.Snapshot{Problem: "llama-swap wants an API key"}
	if got := describe(running, snap); got != "running" {
		t.Errorf("state %q, want running", got)
	}
	if got := message(running, snap); got != snap.Problem {
		t.Errorf("message %q, want the problem", got)
	}
	// The supervisor's own reason comes first.
	waiting := supervisor.Status{State: supervisor.Waiting, Message: "exited; restarting in 1s"}
	if got := message(waiting, snap); got != waiting.Message {
		t.Errorf("message %q, want the supervisor's", got)
	}
}

func TestWrap(t *testing.T) {
	for _, tt := range []struct {
		s     string
		width int
		max   int
		want  []string
	}{
		{"", 10, 3, nil},
		{"short", 10, 3, []string{"short"}},
		{"one two three four", 9, 3, []string{"one two", "three", "four"}},
		{"a /very/long/path/name here", 8, 4, []string{"a", "/very/lo", "ng/path/", "name…"}}, // "here" left out
		{"one two three four five six", 9, 2, []string{"one two", "three…"}},
		{"exactly nine", 9, 1, []string{"exactly…"}},
		{"a /very/long/path", 8, 4, []string{"a", "/very/lo", "ng/path"}},
	} {
		if got := wrap(tt.s, tt.width, tt.max); !slices.Equal(got, tt.want) {
			t.Errorf("wrap(%q, %d, %d) = %q, want %q", tt.s, tt.width, tt.max, got, tt.want)
		}
	}
	// The real messages fit.
	for _, msg := range []string{
		"llama-swap wants an API key (its config sets apiKeys). Add the key in Settings… → Secrets",
		"llama-swap (pid 27889), run by another copy of Llama Swap Launcher (pid 27882), is already listening on 127.0.0.1:8080. Stop llama-swap in that copy of the app, or choose another listen address in Settings…",
	} {
		if got := wrap(msg, detailWidth, detailLines); strings.HasSuffix(got[len(got)-1], "…") && !strings.HasSuffix(msg, "…") {
			t.Errorf("%q doesn't fit: %q", msg, got)
		}
	}
}

func TestFaultLine(t *testing.T) {
	now := time.Date(2026, 10, 4, 15, 0, 0, 0, time.Local)
	today := &control.Fault{Model: "gemma4-31b-q8", At: time.Date(2026, 10, 4, 14, 32, 0, 0, time.Local)}
	if got := faultLine(today, now); got != "Last GPU fault: gemma4-31b-q8 at 14:32" {
		t.Errorf("today: %q", got)
	}
	earlier := &control.Fault{Model: "m", At: time.Date(2026, 10, 3, 9, 5, 0, 0, time.Local), AutoUnloaded: true}
	if got := faultLine(earlier, now); got != "Last GPU fault: m Oct 3 at 09:05, unloaded automatically" {
		t.Errorf("yesterday: %q", got)
	}
}
