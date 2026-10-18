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
	managed := prefs.Prefs{APIKeys: []string{"ADMIN"}, ConfigDirMode: prefs.ConfigDirManaged, ManageAPIKeys: true}
	for _, tt := range []struct {
		name string
		p    prefs.Prefs
		st   supervisor.Status
		snap health.Snapshot
		want string
	}{
		{"keys not in the config", keys, running("127.0.0.1:8080"), open, "isn't asking for API keys"},
		{"keys not in the config: says how to require them", keys, running("127.0.0.1:8080"), open, "Turn on Require these keys"},
		{"managed keys, not restarted yet", managed, running("127.0.0.1:8080"), open, "Restart it"},
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

func TestStopsRequiringKeys(t *testing.T) {
	required := prefs.Prefs{Listen: "0.0.0.0:8080", APIKeys: []string{"A"}, ConfigDirMode: prefs.ConfigDirManaged, ManageAPIKeys: true}
	off := required
	off.ManageAPIKeys = false
	ownFolder := required
	ownFolder.ConfigDirMode = prefs.ConfigDirOwn
	noKeys := required
	noKeys.APIKeys = nil
	local := required
	local.Listen = "127.0.0.1:8080"
	localOff := off
	localOff.Listen = "127.0.0.1:8080"
	for _, tt := range []struct {
		name   string
		old, p prefs.Prefs
		want   bool
	}{
		{"unticked", required, off, true},
		{"folder no longer managed", required, ownFolder, true},
		{"keys removed", required, noKeys, true},
		{"no change", required, required, false},
		{"turned on", off, required, false},
		{"only on this Mac", local, localOff, false},
	} {
		if got := stopsRequiringKeys(tt.old, tt.p); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestManagedFiles(t *testing.T) {
	p := prefs.Prefs{APIKeys: []string{"ADMIN", "CLAUDE"}, ConfigDirMode: prefs.ConfigDirManaged, ManageAPIKeys: true}
	got := string(managedFiles(p)["api-keys.yaml"])
	want := managedHeader + "apiKeys:\n  - \"${env.LLSL_ADMIN}\"\n  - \"${env.LLSL_CLAUDE}\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	peers := prefs.Prefs{ConfigDirMode: prefs.ConfigDirManaged, Peers: []prefs.Peer{
		{Name: "studio", URL: "http://192.168.1.20:8080", KeyVariable: "STUDIO_KEY", Models: []string{"gemma", "org/model \"q\""}},
		{Name: "open", URL: "https://example.com/api", Models: []string{"x"}},
	}}
	wantPeers := managedHeader + `peers:
  "studio":
    proxy: "http://192.168.1.20:8080"
    apiKey: "${env.STUDIO_KEY}"
    models:
      - "gemma"
      - "org/model \"q\""
  "open":
    proxy: "https://example.com/api"
    models:
      - "x"
`
	if got := string(managedFiles(peers)["peers.yaml"]); got != wantPeers {
		t.Errorf("peers: got\n%s\nwant\n%s", got, wantPeers)
	}
	unmanaged := peers
	unmanaged.ConfigDirMode = prefs.ConfigDirOwn
	if got := string(managedFiles(unmanaged)["peers.yaml"]); got != managedHeader {
		t.Errorf("peers without the managed folder: got %q, want only the note", got)
	}
	// Not managed, or no keys: only the note, so the setting turns off.
	for _, q := range []prefs.Prefs{
		{APIKeys: []string{"ADMIN"}, ConfigDirMode: prefs.ConfigDirManaged},
		{ConfigDirMode: prefs.ConfigDirManaged, ManageAPIKeys: true},
	} {
		if got := string(managedFiles(q)["api-keys.yaml"]); got != managedHeader {
			t.Errorf("%+v: got %q, want only the note", q, got)
		}
	}
}
