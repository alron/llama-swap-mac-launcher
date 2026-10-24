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
		Binary:        "/opt/homebrew/bin/llama-swap",
		Config:        "/x/llama-swap.yaml",
		Listen:        "127.0.0.1:9090",
		Args:          []string{"--log-format=json"},
		WatchConfig:   true,
		ConfigDirMode: ConfigDirManaged,
		ConfigDir:     "~/llama-swap/config.d",
		ManageAPIKeys: true,
		Env:           map[string]string{"HF_TOKEN": "t"},
		AutoStart:     false,
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
		{"ordinary args", Prefs{Args: []string{"--log-level=debug", "value"}}, true},
		{"-watch-config in args", Prefs{Args: []string{"-watch-config"}}, false},
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
		{"own config folder", Prefs{ConfigDirMode: ConfigDirOwn, ConfigDir: "~/llama-swap/config.d"}, true},
		{"own config folder, not given", Prefs{ConfigDirMode: ConfigDirOwn}, false},
		{"own config folder, relative", Prefs{ConfigDirMode: ConfigDirOwn, ConfigDir: "config.d"}, false},
		{"managed config folder", Prefs{ConfigDirMode: ConfigDirManaged, ManageAPIKeys: true}, true},
		{"folder kept while off", Prefs{ConfigDirMode: ConfigDirOff, ConfigDir: "config.d"}, true},
		{"unknown config folder mode", Prefs{ConfigDirMode: "shared"}, false},
		{"peer", Prefs{SecretEnv: []string{"PEER_KEY"}, Peers: []Peer{{Name: "studio", URL: "http://192.168.1.20:8080", KeyVariable: "PEER_KEY", Models: []string{"a", "org/b"}}}}, true},
		{"peer without a key", Prefs{Peers: []Peer{{Name: "openrouter.ai", URL: "https://openrouter.ai/api", Models: []string{"x"}}}}, true},
		{"peer name with a slash", Prefs{Peers: []Peer{{Name: "a/b", URL: "http://h", Models: []string{"x"}}}}, false},
		{"peer name with a space", Prefs{Peers: []Peer{{Name: "my peer", URL: "http://h", Models: []string{"x"}}}}, false},
		{"peer twice", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"x"}}, {Name: "a", URL: "http://g", Models: []string{"y"}}}}, false},
		{"peer key not a secret variable", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", KeyVariable: "GONE", Models: []string{"x"}}}}, false},
		{"peer that's off, its key gone", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", KeyVariable: "GONE", Models: []string{"x"}, Disabled: true}}}, true},
		{"peer that's off, its URL bad", Prefs{Peers: []Peer{{Name: "a", URL: "h:8080", Models: []string{"x"}, Disabled: true}}}, false},
		{"peer that's off, its name taken", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"x"}}, {Name: "a", URL: "http://g", Models: []string{"y"}, Disabled: true}}}, false},
		{"peer key variable llama-swap can't name", Prefs{SecretEnv: []string{"my.key"}, Peers: []Peer{{Name: "a", URL: "http://h", KeyVariable: "my.key", Models: []string{"x"}}}}, false},
		{"peer without models", Prefs{Peers: []Peer{{Name: "a", URL: "http://h"}}}, false},
		{"peer tailcat URL", Prefs{Peers: []Peer{{Name: "a", URL: "tailcat://tcTOKEN", Models: []string{"x"}}}}, false},
		{"peer URL without a host", Prefs{Peers: []Peer{{Name: "a", URL: "http://", Models: []string{"x"}}}}, false},
		{"peer URL with a variable", Prefs{Peers: []Peer{{Name: "a", URL: "http://${env.HOST}", Models: []string{"x"}}}}, false},
		{"peer model twice", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"x", "x"}}}}, false},
		{"peer timeouts", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"x"}, Timeouts: PeerTimeouts{ResponseHeader: seconds(600), Connect: seconds(0)}}}}, true},
		{"peer timeout negative", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"x"}, Timeouts: PeerTimeouts{KeepAlive: seconds(-1)}}}}, false},
		{"cors", Prefs{CORS: CORS{Restrict: true, AllowedOrigins: []string{"http://localhost:3000", "https://app.example.com"}, AllowCredentials: true, MaxAge: seconds(600)}}, true},
		{"cors off, origins kept", Prefs{CORS: CORS{AllowedOrigins: []string{"http://localhost:3000"}}}, true},
		{"cors restricting nothing", Prefs{CORS: CORS{Restrict: true}}, false},
		{"cors star", Prefs{CORS: CORS{Restrict: true, AllowedOrigins: []string{"*"}}}, false},
		{"cors origin with a slash", Prefs{CORS: CORS{Restrict: true, AllowedOrigins: []string{"http://localhost:3000/"}}}, false},
		{"cors origin with a path", Prefs{CORS: CORS{Restrict: true, AllowedOrigins: []string{"https://app.example.com/ui"}}}, false},
		{"cors origin without a scheme", Prefs{CORS: CORS{Restrict: true, AllowedOrigins: []string{"localhost:3000"}}}, false},
		{"cors origin twice", Prefs{CORS: CORS{Restrict: true, AllowedOrigins: []string{"http://a.example", "http://A.example"}}}, false},
		{"cors maxAge negative", Prefs{CORS: CORS{Restrict: true, AllowedOrigins: []string{"http://a.example"}, MaxAge: seconds(-1)}}, false},
		{"peer URL with a password", Prefs{Peers: []Peer{{Name: "a", URL: "http://user:pass@h", Models: []string{"x"}}}}, false},
		{"peer URL with a user name", Prefs{Peers: []Peer{{Name: "a", URL: "https://user@h", Models: []string{"x"}}}}, false},
		{"peer model with DEL", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"x\x7f"}}}}, false},
		{"peer model with a C1 control", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"x\u0090"}}}}, false},
		{"peer model with a tab", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"a\tb"}}}}, false},
		{"peer model too long", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{strings.Repeat("m", MaxModelID+1)}}}}, false},
		{"peer model, long but not too long, and odd", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{strings.Repeat("é", MaxModelID), "- a: b #c [d] {e} 'f' \"g\""}}}}, true},
		{"peer model with a variable", Prefs{Peers: []Peer{{Name: "a", URL: "http://h", Models: []string{"${env.X}"}}}}, false},
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

func TestLoadLiftsWatchConfig(t *testing.T) {
	for _, tt := range []struct {
		name, file string
		watch      bool
		args       []string
		err        string
	}{
		{"old style", `{"args": ["-watch-config", "-x"]}`, true, []string{"-x"}, ""},
		{"two dashes", `{"args": ["--watch-config"]}`, true, []string{}, ""},
		{"with a value", `{"args": ["-watch-config=false"]}`, false, []string{}, ""},
		{"the last counts", `{"args": ["-watch-config", "-watch-config=0"]}`, false, []string{}, ""},
		{"agrees with the setting", `{"watchConfig": true, "args": ["-watch-config"]}`, true, []string{}, ""},
		{"setting alone", `{"watchConfig": true}`, true, []string{}, ""},
		{"disagrees with the setting", `{"watchConfig": false, "args": ["-watch-config"]}`, false, nil, "keep only watchConfig"},
		{"value on the same line", `{"args": ["-watch-config true"]}`, false, nil, "use the watchConfig setting"},
		{"not a boolean", `{"args": ["-watch-config=yes"]}`, false, nil, "use the watchConfig setting"},
	} {
		path := filepath.Join(t.TempDir(), "prefs.json")
		os.WriteFile(path, []byte(tt.file), 0o600)
		p, err := Load(path)
		switch {
		case tt.err != "":
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%s: got %v, want an error with %q", tt.name, err, tt.err)
			}
		case err != nil:
			t.Errorf("%s: %v", tt.name, err)
		case p.WatchConfig != tt.watch || !reflect.DeepEqual(p.Args, tt.args):
			t.Errorf("%s: got watchConfig %v, args %q; want %v, %q", tt.name, p.WatchConfig, p.Args, tt.watch, tt.args)
		}
	}
}

func TestConfigDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, tt := range []struct {
		p       Prefs
		dir     string
		manages bool
	}{
		{Defaults(), "", false},
		{Prefs{ConfigDir: "/x", ManageAPIKeys: true}, "", false},
		{Prefs{ConfigDirMode: ConfigDirOwn, ConfigDir: "~/c", ManageAPIKeys: true}, filepath.Join(home, "c"), false},
		{Prefs{ConfigDirMode: ConfigDirManaged, ConfigDir: "/x"}, "/managed", false},
		{Prefs{ConfigDirMode: ConfigDirManaged, ManageAPIKeys: true}, "/managed", true},
	} {
		if dir := tt.p.ConfigDirPath("/managed"); dir != tt.dir || tt.p.ManagesAPIKeys() != tt.manages {
			t.Errorf("%+v: got %q, manages %v; want %q, %v", tt.p, dir, tt.p.ManagesAPIKeys(), tt.dir, tt.manages)
		}
	}
}

func seconds(n int) *int { return &n }

// A peer's timeouts round-trip, and one left at the default isn't stored.
func TestPeerTimeoutsJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	p := Defaults()
	p.Peers = []Peer{{Name: "a", URL: "http://h", Models: []string{"x"}, Timeouts: PeerTimeouts{ResponseHeader: seconds(600)}}}
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"timeouts": {
        "responseHeader": 600
      }`) {
		t.Errorf("saved as:\n%s", b)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if to := got.Peers[0].Timeouts; to.ResponseHeader == nil || *to.ResponseHeader != 600 || to.Connect != nil || to.KeepAlive != nil {
		t.Errorf("loaded %+v", to)
	}
}
