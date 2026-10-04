package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		Args:                      []string{"-watch-config", "-log-prefix", "/x/My Logs"},
		Env:                       map[string]string{"HF_TOKEN": "a=b", "B": ""},
		AutoStart:                 true,
		HealthCheckSeconds:        30,
		HealthCheckSkipWhenActive: false,
		UnloadOnGPUFault:          true,
		APIKeys:                   []string{"ADMIN", "CLAUDE"},
		AppAPIKey:                 "CLAUDE",
		SecretEnv:                 []string{"GITHUB_TOKEN"},
		LauncherLog:               "/tmp/launcher.log",
		LlamaSwapLog:              prefs.LogToFile,
		LlamaSwapLogFile:          "/tmp/llama-swap.log",
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
		Args: "\n  -watch-config  \n\n",
		Env:  "\r\nA=1\r\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Args, []string{"-watch-config"}) || got.Env["A"] != "1" || len(got.Env) != 1 {
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
		{"env without =", macos.PreferencesForm{Env: "A=1\nHF_TOKEN"}, "line 2"},
		{"interval not a number", macos.PreferencesForm{HealthCheckSeconds: "fast"}, "whole number"},
		{"interval zero", macos.PreferencesForm{HealthCheckSeconds: "0"}, "whole number"},
		{"bad listen", macos.PreferencesForm{Listen: "localhost"}, "listen"},
		{"env with the key prefix", macos.PreferencesForm{Env: "LLSL_X=1"}, "kept for API keys"},
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
		{"secret also plain", macos.PreferencesForm{Env: "A=1", SecretEnv: []macos.SecretField{{Name: "A", Value: "x"}}}, "Environment on the General tab too"},
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
	if err := checkPaths(p); err != nil {
		t.Errorf("checkPaths: %v", err)
	}
	if p.ConfigPath() != config {
		t.Errorf("ConfigPath() = %q, want %q", p.ConfigPath(), config)
	}
}
