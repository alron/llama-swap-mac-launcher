package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

func TestFormRoundTrip(t *testing.T) {
	want := prefs.Prefs{
		Binary:                    "/opt/homebrew/bin/llama-swap",
		Config:                    "/x/llama-swap.yaml",
		Listen:                    "0.0.0.0:8080",
		Args:                      []string{"-log-prefix", "/x/My Logs"},
		WatchConfig:               true,
		ConfigDirMode:             prefs.ConfigDirOwn,
		ConfigDir:                 "~/llama-swap/config.d",
		ManageAPIKeys:             true,
		Env:                       map[string]string{"HF_TOKEN": "a=b", "B": ""},
		AutoStart:                 true,
		HealthCheckSeconds:        30,
		HealthCheckSkipWhenActive: false,
		UnloadOnGPUFault:          true,
		APIKeys:                   []string{"ADMIN", "CLAUDE"},
		AppAPIKey:                 "CLAUDE",
		CORS:                      prefs.CORS{Restrict: true, AllowedOrigins: []string{"http://localhost:3000"}, AllowPrivateNetwork: true, MaxAge: seconds(600)},
		SecretEnv:                 []string{"GITHUB_TOKEN"},
		Peers: []prefs.Peer{
			{Name: "studio", URL: "http://192.168.1.20:8080", KeyVariable: "GITHUB_TOKEN", Models: []string{"a", "org/b"}},
			{Name: "open", URL: "https://example.com/api", Models: []string{"x"}, Timeouts: prefs.PeerTimeouts{ResponseHeader: seconds(600), KeepAlive: seconds(0)}},
			{Name: "away", URL: "http://192.168.1.30:8080", Models: []string{"y"}, Disabled: true},
		},
		LauncherLog:      "/tmp/launcher.log",
		LlamaSwapLog:     prefs.LogToFile,
		LlamaSwapLogFile: "/tmp/llama-swap.log",
	}
	wantSecrets := map[string]string{"LLSL_ADMIN": "sk-one", "LLSL_CLAUDE": "sk-two", "GITHUB_TOKEN": "gh x y"}
	got, secrets, err := prefsFrom(formFrom(want, wantSecrets))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if !reflect.DeepEqual(secrets, wantSecrets) {
		t.Errorf("secrets: got %v, want %v", secrets, wantSecrets)
	}
}

func TestPrefsFromKeys(t *testing.T) {
	form := macos.PreferencesForm{
		APIKeys:   []macos.SecretField{{Name: " ADMIN ", Value: " sk-a\n"}, {Name: "B", Value: "sk-b"}},
		AppAPIKey: "GONE", // the chosen key was removed: the first is used
	}
	p, secrets, err := prefsFrom(form)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.APIKeys, []string{"ADMIN", "B"}) || secrets["LLSL_ADMIN"] != "sk-a" || p.AppKey() != "ADMIN" {
		t.Errorf("got keys %v %v, app key %q", p.APIKeys, secrets, p.AppKey())
	}
}

func TestPrefsFromDefaultsAndBlanks(t *testing.T) {
	got, _, err := prefsFrom(macos.PreferencesForm{
		Args: "\n  -log-prefix  \n\n",
		Env:  []macos.SecretField{{Name: " A ", Value: "1"}, {Name: "B", Value: " as typed "}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Args, []string{"-log-prefix"}) || got.Env["A"] != "1" || got.Env["B"] != " as typed " || len(got.Env) != 2 {
		t.Errorf("got args %q, env %v", got.Args, got.Env)
	}
	if got.HealthCheckSeconds != 0 || got.HealthCheckInterval() != prefs.DefaultHealthCheckSeconds*1e9 {
		t.Errorf("an empty interval should mean the default, got %d", got.HealthCheckSeconds)
	}
}

func TestPrefsFromErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		form macos.PreferencesForm
		want string
	}{
		{"env without a name", envForm(macos.SecretField{Name: "A", Value: "1"}, macos.SecretField{Value: "x"}), "row 2"},
		{"env twice", envForm(macos.SecretField{Name: "A", Value: "1"}, macos.SecretField{Name: "A", Value: "2"}), "listed twice"},
		{"env with =", envForm(macos.SecretField{Name: "A=B", Value: "1"}), "valid variable name"},
		{"interval not a number", macos.PreferencesForm{HealthCheckSeconds: "fast"}, "whole number"},
		{"interval zero", macos.PreferencesForm{HealthCheckSeconds: "0"}, "whole number"},
		{"bad listen", macos.PreferencesForm{Listen: "localhost"}, "listen"},
		{"env with the key prefix", envForm(macos.SecretField{Name: "LLSL_X", Value: "1"}), "kept for API keys"},
		{"key without a name", keysForm(macos.SecretField{Value: "sk"}), "row 1"},
		{"key name in lower case", keysForm(macos.SecretField{Name: "admin", Value: "sk"}), "LLSL_admin can only"},
		{"key twice", keysForm(macos.SecretField{Name: "A", Value: "sk"}, macos.SecretField{Name: "A", Value: "sk2"}), "twice"},
		{"key without a value", keysForm(macos.SecretField{Name: "A"}), "LLSL_A has no key"},
		{"key with a space", keysForm(macos.SecretField{Name: "A", Value: "sk a"}), "spaces"},
		{"secret without a name", varsForm(macos.SecretField{Value: "x"}), "row 1"},
		{"secret with =", varsForm(macos.SecretField{Name: "A=B", Value: "x"}), "valid variable name"},
		{"secret with the key prefix", varsForm(macos.SecretField{Name: "LLSL_X", Value: "x"}), "kept for API keys"},
		{"secret without a value", varsForm(macos.SecretField{Name: "HF_TOKEN"}), "HF_TOKEN has no value"},
		{"secret twice", varsForm(macos.SecretField{Name: "A", Value: "x"}, macos.SecretField{Name: "A", Value: "y"}), "twice"},
		{"secret also plain", macos.PreferencesForm{Env: []macos.SecretField{{Name: "A", Value: "1"}}, SecretEnv: []macos.SecretField{{Name: "A", Value: "x"}}}, "Environment on the General tab too"},
		{"log with a relative path", macos.PreferencesForm{LauncherLog: "logs/x.log"}, "isn't a whole path"},
		{"log in a missing folder", macos.PreferencesForm{LauncherLog: "/no/such/folder/x.log"}, "isn't there"},
		{"log is a folder", macos.PreferencesForm{LauncherLog: "/tmp"}, "is a folder"},
		{"log twice", macos.PreferencesForm{LauncherLog: "/tmp/x.log", LlamaSwapLog: "file", LlamaSwapLogFile: "/tmp/x.log"}, "launcher log too"},
	} {
		_, _, err := prefsFrom(tt.form)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got %v, want an error mentioning %q", tt.name, err, tt.want)
		}
		// Problems with keys, secrets and logs bring the dialog back on
		// their tab.
		isSecret := strings.HasPrefix(tt.name, "key") || strings.HasPrefix(tt.name, "secret")
		if errors.As(err, new(secretError)) != isSecret {
			t.Errorf("%s: secretError is %v, want %v", tt.name, !isSecret, isSecret)
		}
		if isLog := strings.HasPrefix(tt.name, "log"); errors.As(err, new(logError)) != isLog {
			t.Errorf("%s: logError is %v, want %v", tt.name, !isLog, isLog)
		}
	}
}

func keysForm(keys ...macos.SecretField) macos.PreferencesForm {
	return macos.PreferencesForm{APIKeys: keys}
}

func varsForm(vars ...macos.SecretField) macos.PreferencesForm {
	return macos.PreferencesForm{SecretEnv: vars}
}

func envForm(env ...macos.SecretField) macos.PreferencesForm {
	return macos.PreferencesForm{Env: env}
}

func TestLlamaSwapChanged(t *testing.T) {
	p := prefs.Defaults()
	health := p
	health.HealthCheckSeconds = 10
	if llamaSwapChanged(p, health) {
		t.Error("a health-check change doesn't need llama-swap restarted")
	}
	env := p
	env.Env = map[string]string{"A": "1"}
	if !llamaSwapChanged(p, env) {
		t.Error("an env change needs llama-swap restarted")
	}
	keys := p
	keys.APIKeys = []string{"A", "B"}
	if !llamaSwapChanged(p, keys) {
		t.Error("a new key needs llama-swap restarted")
	}
	appKey := keys
	appKey.AppAPIKey = "B"
	if !llamaSwapChanged(keys, appKey) {
		t.Error("changing the key the app uses needs llama-swap restarted")
	}
	peer := p
	peer.Peers = []prefs.Peer{{Name: "studio", URL: "http://h", Models: []string{"a"}}}
	off := peer
	off.Peers = []prefs.Peer{{Name: "studio", URL: "http://h", Models: []string{"a"}, Disabled: true}}
	if !llamaSwapChanged(peer, off) {
		t.Error("turning a peer off needs llama-swap restarted")
	}
}

// "~" works in the binary and config fields, as in the log fields: the
// preferences keep it as typed, and every use expands it.
func TestTildePaths(t *testing.T) {
	home, _ := os.UserHomeDir()
	config := filepath.Join(home, ".lsl-tilde-test.yaml")
	if err := os.WriteFile(config, nil, 0o600); err != nil {
		t.Skip("can't write in the home folder:", err)
	}
	defer os.Remove(config)
	p := prefs.Prefs{Config: "~/.lsl-tilde-test.yaml"}
	if err := checkPaths(p, t.TempDir()); err != nil {
		t.Errorf("checkPaths: %v", err)
	}
	if p.ConfigPath() != config {
		t.Errorf("ConfigPath() = %q, want %q", p.ConfigPath(), config)
	}
}

// Problems with peers bring the dialog back on the Peers tab.
func TestPrefsFromPeerErrors(t *testing.T) {
	ok := macos.PeerField{Name: "other", URL: "http://h:8080", Models: []string{"a"}}
	for _, tt := range []struct {
		name string
		peer macos.PeerField
		want string
	}{
		{"no name", macos.PeerField{URL: "http://h", Models: []string{"a"}}, "give the peer a name"},
		{"no URL", macos.PeerField{Name: "studio", Models: []string{"a"}}, "give studio's URL"},
		{"nothing ticked", macos.PeerField{Name: "studio", URL: "http://h"}, "tick at least one"},
		{"bad URL", macos.PeerField{Name: "studio", URL: "h:8080", Models: []string{"a"}}, "isn't an http://"},
		{"key gone", macos.PeerField{Name: "studio", URL: "http://h", KeyVariable: "GONE", Models: []string{"a"}}, "isn't one of the secret variables"},
		{"timeout not a number", macos.PeerField{Name: "studio", URL: "http://h", Models: []string{"a"}, ResponseHeader: "ten"}, "isn't a whole number of seconds"},
		{"timeout negative", macos.PeerField{Name: "studio", URL: "http://h", Models: []string{"a"}, Connect: "-5"}, "isn't a whole number of seconds"},
		{"key with ${", macos.PeerField{Name: "studio", URL: "http://h", KeyVariable: "SPLICE", Models: []string{"a"}}, "has ${ in it"},
	} {
		_, _, err := prefsFrom(macos.PreferencesForm{Peers: []macos.PeerField{ok, tt.peer},
			SecretEnv: []macos.SecretField{{Name: "SPLICE", Value: "x${env.OTHER}"}}})
		if !errors.As(err, new(peerError)) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got %v, want a peer error with %q", tt.name, err, tt.want)
		}
	}
	if _, _, err := prefsFrom(macos.PreferencesForm{Peers: []macos.PeerField{ok}}); err != nil {
		t.Errorf("a good peer: %v", err)
	}
	// A peer that's off isn't written, so its key's variable may be gone,
	// or have ${ in it; the rest is still checked.
	for _, off := range []macos.PeerField{
		{Name: "studio", URL: "http://h", KeyVariable: "GONE", Models: []string{"a"}, Disabled: true},
		{Name: "studio", URL: "http://h", KeyVariable: "SPLICE", Models: []string{"a"}, Disabled: true},
	} {
		if _, _, err := prefsFrom(macos.PreferencesForm{Peers: []macos.PeerField{ok, off},
			SecretEnv: []macos.SecretField{{Name: "SPLICE", Value: "x${env.OTHER}"}}}); err != nil {
			t.Errorf("a peer that's off, key %s: %v", off.KeyVariable, err)
		}
	}
	if _, _, err := prefsFrom(macos.PreferencesForm{Peers: []macos.PeerField{{Name: "studio", URL: "h:8080", Models: []string{"a"}, Disabled: true}}}); err == nil {
		t.Error("a peer that's off with a bad URL was accepted")
	}
}

// A timeout shown at, or typed as, llama-swap's default is left to it.
func TestPeerTimeoutsFromForm(t *testing.T) {
	f := macos.PeerField{Name: "a", URL: "http://h", Models: []string{"x"}, ResponseHeader: "600", Connect: "30", KeepAlive: " "}
	p, _, err := prefsFrom(macos.PreferencesForm{Peers: []macos.PeerField{f}})
	if err != nil {
		t.Fatal(err)
	}
	to := p.Peers[0].Timeouts
	if to.ResponseHeader == nil || *to.ResponseHeader != 600 || to.Connect != nil || to.KeepAlive != nil {
		t.Errorf("got %+v", to)
	}
	if shown := formFrom(p, nil).Peers[0]; shown.ResponseHeader != "600" || shown.Connect != "30" || shown.KeepAlive != "30" {
		t.Errorf("shown as %+v", shown)
	}
}

// Problems with the web pages bring the dialog back on the Security tab,
// and a preflight cache left at llama-swap's default is left to it.
func TestPrefsFromCORS(t *testing.T) {
	p, _, err := prefsFrom(macos.PreferencesForm{CORSRestrict: true, CORSOrigins: "http://localhost:3000\n\n https://app.example.com ", CORSMaxAge: "86400"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.CORS.AllowedOrigins, []string{"http://localhost:3000", "https://app.example.com"}) || p.CORS.MaxAge != nil {
		t.Errorf("got %+v", p.CORS)
	}
	for _, f := range []macos.PreferencesForm{
		{CORSRestrict: true},
		{CORSRestrict: true, CORSOrigins: "http://localhost:3000/"},
		{CORSRestrict: true, CORSOrigins: "http://localhost:3000", CORSMaxAge: "a day"},
	} {
		if _, _, err := prefsFrom(f); !errors.As(err, new(corsError)) {
			t.Errorf("%+v: got %v, want a Security tab error", f, err)
		}
	}
}

// The app's managed folder can't be chosen as the user's own, however it's
// written.
func TestOwnFolderIsntTheManagedOne(t *testing.T) {
	managed := t.TempDir()
	other := t.TempDir()
	for _, tt := range []struct {
		dir  string
		want bool
	}{{managed, false}, {managed + "/.", false}, {other, true}} {
		p := prefs.Prefs{ConfigDirMode: prefs.ConfigDirOwn, ConfigDir: tt.dir}
		if err := checkPaths(p, managed); (err == nil) != tt.want {
			t.Errorf("%s: got %v", tt.dir, err)
		}
	}
}
