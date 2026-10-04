package main

import (
	"strings"
	"testing"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

func TestExposed(t *testing.T) {
	for listen, want := range map[string]bool{
		"127.0.0.1:8080": false, "localhost:8080": false, "[::1]:8080": false,
		":8080": true, "0.0.0.0:8080": true, "[::]:8080": true, "192.168.1.20:8080": true, "studio.local:8080": true,
	} {
		if got := exposed(listen); got != want {
			t.Errorf("exposed(%q) = %v, want %v", listen, got, want)
		}
	}
}

func TestOpenWarning(t *testing.T) {
	open := health.Snapshot{Healthy: true, Open: true}
	running := func(listen string) supervisor.Status {
		return supervisor.Status{State: supervisor.Running, Listen: listen}
	}
	keys := prefs.Prefs{APIKeys: []string{"ADMIN"}}
	for _, tt := range []struct {
		name string
		p    prefs.Prefs
		st   supervisor.Status
		snap health.Snapshot
		want string
	}{
		{"keys not in the config", keys, running("127.0.0.1:8080"), open, "isn't asking for API keys"},
		{"open to the network", prefs.Prefs{}, running("0.0.0.0:8080"), open, "open to the network"},
		{"loopback without keys", prefs.Prefs{}, running("127.0.0.1:8080"), open, ""},
		{"asking for keys", keys, running("0.0.0.0:8080"), health.Snapshot{Healthy: true}, ""},
		{"not running", keys, supervisor.Status{State: supervisor.Stopped}, open, ""},
	} {
		got := openWarning(tt.p, tt.st, tt.snap)
		if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestOpensWithoutKey(t *testing.T) {
	local := prefs.Prefs{Listen: "127.0.0.1:8080"}
	open := prefs.Prefs{Listen: "0.0.0.0:8080"}
	keyed := prefs.Prefs{Listen: "0.0.0.0:8080", APIKeys: []string{"A"}}
	for _, tt := range []struct {
		name   string
		old, p prefs.Prefs
		want   bool
	}{
		{"opening it", local, open, true},
		{"removing the last key", keyed, open, true},
		{"already open", open, open, false},
		{"with a key", local, keyed, false},
		{"closing it", open, local, false},
	} {
		if got := opensWithoutKey(tt.old, tt.p); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
