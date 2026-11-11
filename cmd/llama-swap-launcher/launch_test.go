package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// At launch the app waits for the files llama-swap starts with that the
// preferences name (TODO item 91): not a binary it finds itself, nor its
// own managed folder.
func TestLaunchFiles(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	for _, tt := range []struct {
		name string
		p    prefs.Prefs
		want []string
	}{
		// A fresh install names neither: the app finds llama-swap itself,
		// and Settings asks for a config, so nothing is waited for.
		{"a fresh install", prefs.Defaults(), nil},
		{"on a pool", prefs.Prefs{Binary: "/Volumes/pool/llama-swap", Config: "/Volumes/pool/config.yaml"},
			[]string{"/Volumes/pool/llama-swap", "/Volumes/pool/config.yaml"}},
		{"binary found by the app", prefs.Prefs{Config: "~/llama-swap.yaml"}, []string{filepath.Join(home, "llama-swap.yaml")}},
		{"own folder", prefs.Prefs{Binary: "/b", Config: "/c", ConfigDirMode: prefs.ConfigDirOwn, ConfigDir: "/Volumes/pool/conf.d"},
			[]string{"/b", "/c", "/Volumes/pool/conf.d"}},
		{"managed folder", prefs.Prefs{Binary: "/b", Config: "/c", ConfigDirMode: prefs.ConfigDirManaged}, []string{"/b", "/c"}},
		{"own folder, none named", prefs.Prefs{Binary: "/b", Config: "/c", ConfigDirMode: prefs.ConfigDirOwn}, []string{"/b", "/c"}},
	} {
		if got := launchFiles(tt.p); !slices.Equal(got, tt.want) {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// The menu and llsl status name the state, with what it waits for.
func TestDescribePending(t *testing.T) {
	st := supervisor.Status{State: supervisor.Pending, Message: "waiting for /Volumes/pool/llama-swap to appear"}
	if got := describe(st, health.Snapshot{}); got != "waiting for its files" {
		t.Errorf("describe: %q", got)
	}
	if got := message(st, health.Snapshot{}); got != st.Message {
		t.Errorf("message: %q", got)
	}
	if !up(st) {
		t.Error("not up, so the menu wouldn't offer Stop")
	}
}
