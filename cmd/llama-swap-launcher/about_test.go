package main

import (
	"slices"
	"testing"

	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

func TestAboutCredits(t *testing.T) {
	running := func(v, built string) supervisor.Status {
		return supervisor.Status{State: supervisor.Running, Version: v, Built: built}
	}
	for _, tt := range []struct {
		name  string
		built string
		st    supervisor.Status
		want  []string
	}{
		{"a release", "2026-11-02", running("v269 (9fbc47c)", "2026-11-01T00:58:35Z"),
			[]string{"Built 2026-11-02", "Running: llama-swap v269", "llama-swap built 2026-11-01", "", "MIT License"}},
		{"a build from source, built in another time zone", "2026-11-02", running("v269-2-g581d29 (581d29c)", "2026-10-31T20:36:54-07:00"),
			[]string{"Built 2026-11-02", "Running: llama-swap v269-2-g581d29", "llama-swap built 2026-11-01", "", "MIT License"}},
		{"no build time", "2026-11-02", running("v263", ""),
			[]string{"Built 2026-11-02", "Running: llama-swap v263", "", "MIT License"}},
		{"llama-swap stopped", "2026-11-02", supervisor.Status{State: supervisor.Stopped, Version: "v269 (9fbc47c)"},
			[]string{"Built 2026-11-02", "", "MIT License"}},
		{"running, version unknown", "2026-11-02", running("", "2026-11-01T00:58:35Z"),
			[]string{"Built 2026-11-02", "", "MIT License"}},
		{"a plain go build, stopped", "", supervisor.Status{},
			[]string{"MIT License"}},
	} {
		if got := aboutCredits(tt.built, tt.st); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
