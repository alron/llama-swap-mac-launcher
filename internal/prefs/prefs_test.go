package prefs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, Defaults()) {
		t.Errorf("got %+v, want defaults", p)
	}
}

func TestLoadFillsMissingKeysWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	os.WriteFile(path, []byte(`{"config": "/x/llama-swap.yaml"}`), 0o600)
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Config != "/x/llama-swap.yaml" || p.ListenAddr() != DefaultListen || !p.AutoStart {
		t.Errorf("got %+v", p)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	want := Prefs{
		Binary:    "/opt/homebrew/bin/llama-swap",
		Config:    "/x/llama-swap.yaml",
		Listen:    "127.0.0.1:9090",
		Args:      []string{"-watch-config"},
		Env:       map[string]string{"HF_TOKEN": "t"},
		AutoStart: false,
	}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	os.WriteFile(path, []byte(`{"healthCheckSecond": 30}`), 0o600)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "healthCheckSecond") {
		t.Errorf("got %v, want an error naming the misspelled key", err)
	}
}

func TestValidate(t *testing.T) {
	for _, tt := range []struct {
		name string
		p    Prefs
		ok   bool
	}{
		{"defaults", Defaults(), true},
		{"all interfaces", Prefs{Listen: ":8080"}, true},
		{"no port", Prefs{Listen: "127.0.0.1"}, false},
		{"port not a number", Prefs{Listen: "127.0.0.1:http"}, false},
		{"port out of range", Prefs{Listen: "127.0.0.1:70000"}, false},
		{"negative interval", Prefs{HealthCheckSeconds: -1}, false},
		{"env name with =", Prefs{Env: map[string]string{"A=B": "c"}}, false},
		{"env name empty", Prefs{Env: map[string]string{"": "c"}}, false},
		{"env ok", Prefs{Env: map[string]string{"HF_TOKEN": "x=y"}}, true},
		{"env with the key prefix", Prefs{Env: map[string]string{"LLSL_X": "y"}}, false},
		{"keys", Prefs{APIKeys: []string{"ADMIN", "CLAUDE_2"}, AppAPIKey: "CLAUDE_2"}, true},
		{"key in lower case", Prefs{APIKeys: []string{"admin"}}, false},
		{"key with the prefix's dash", Prefs{APIKeys: []string{"A-B"}}, false},
		{"key empty", Prefs{APIKeys: []string{""}}, false},
		{"key twice", Prefs{APIKeys: []string{"A", "A"}}, false},
		{"app key not listed", Prefs{APIKeys: []string{"A"}, AppAPIKey: "B"}, false},
		{"secret variables", Prefs{SecretEnv: []string{"HF_TOKEN", "http_proxy"}}, true},
		{"ordinary args", Prefs{Args: []string{"-watch-config", "--log-level=debug", "value"}}, true},
		{"-listen in args", Prefs{Args: []string{"-listen", "0.0.0.0:9000"}}, false},
		{"--config= in args", Prefs{Args: []string{"--config=/x.yaml"}}, false},
		{"-listen with its value on one line", Prefs{Args: []string{"-listen 0.0.0.0:9000"}}, false},
		{"-config-dir in args", Prefs{Args: []string{"-config-dir", "/x"}}, false},
		{"TLS in args", Prefs{Args: []string{"-tls-cert-file=/x.pem"}}, false},
		{"-version in args", Prefs{Args: []string{"-version"}}, false},
		{"-listen-tailcat in args", Prefs{Args: []string{"-listen-tailcat=:9000"}}, false},
		{"a value that looks like a flag name", Prefs{Args: []string{"listen"}}, true},
		{"log modes", Prefs{LlamaSwapLog: LogOff}, true},
		{"unknown log mode", Prefs{LlamaSwapLog: "syslog"}, false},
		{"log paths", Prefs{LauncherLog: "~/Logs/l.log", LlamaSwapLog: LogToFile, LlamaSwapLogFile: "/var/tmp/ls.log"}, true},
		{"relative log path", Prefs{LauncherLog: "logs/l.log"}, false},
		{"both logs one file", Prefs{LauncherLog: "/tmp/x.log", LlamaSwapLog: LogToFile, LlamaSwapLogFile: "/tmp/../tmp/x.log"}, false},
		{"one file, but not used", Prefs{LauncherLog: "/tmp/x.log", LlamaSwapLogFile: "/tmp/x.log"}, true},
		{"secret variable with =", Prefs{SecretEnv: []string{"A=B"}}, false},
		{"secret variable with the key prefix", Prefs{SecretEnv: []string{"LLSL_A"}}, false},
		{"secret variable twice", Prefs{SecretEnv: []string{"A", "A"}}, false},
		{"secret variable in env too", Prefs{Env: map[string]string{"A": "1"}, SecretEnv: []string{"A"}}, false},
	} {
		if err := tt.p.Validate(); (err == nil) != tt.ok {
			t.Errorf("%s: got %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
}

func TestLoadBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	os.WriteFile(path, []byte(`{"config": `), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("no error for truncated JSON")
	}
}

func TestAppKey(t *testing.T) {
	for _, tt := range []struct {
		p    Prefs
		want string
	}{
		{Prefs{}, ""},
		{Prefs{APIKeys: []string{"A", "B"}}, "A"},
		{Prefs{APIKeys: []string{"A", "B"}, AppAPIKey: "B"}, "B"},
	} {
		if got := tt.p.AppKey(); got != tt.want {
			t.Errorf("%+v: got %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestLogPaths(t *testing.T) {
	home, _ := os.UserHomeDir()
	l, s := Prefs{}.LogPaths("/logs")
	if l != "/logs/launcher.log" || s != "/logs/llama-swap.log" {
		t.Errorf("defaults: %q %q", l, s)
	}
	l, s = Prefs{LauncherLog: "~/a.log", LlamaSwapLogFile: "/b.log"}.LogPaths("/logs")
	if l != filepath.Join(home, "a.log") || s != "/b.log" {
		t.Errorf("set: %q %q", l, s)
	}
}
