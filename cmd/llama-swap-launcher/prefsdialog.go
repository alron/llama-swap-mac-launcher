package main

// The Preferences dialog: turning the preferences into the dialog's text
// fields and back, checking what was entered, and applying it.

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// openPreferences shows the Preferences dialog, unless it's already open.
// While it, or an alert it led to, is up, the menu offers only Quit (see
// render), so nothing else changes the preferences or llama-swap meanwhile.
// tab is the tab to show first: "general", "secrets" or "logs".
func (a *app) openPreferences(tab string) {
	if !a.inDialog.CompareAndSwap(false, true) {
		return
	}
	a.render()
	restart := a.editPreferences(tab)
	a.inDialog.Store(false)
	a.render()
	if restart {
		a.restartLlamaSwap() // a new process gets a new monitor, with the new health settings
	}
}

// editPreferences shows the dialog until it's cancelled, or what was
// entered checks out and is saved. Entries that don't check out get an
// alert saying why, then the dialog comes back with them and the reason.
// It reports whether the user chose to restart llama-swap with the result.
func (a *app) editPreferences(tab string) (restart bool) {
	if err := a.reloadPrefs(); err != nil {
		macos.Alert("Can't open Settings",
			sentence(err.Error())+". Fix it in a text editor, or delete the file to start from the defaults.")
		return false
	}
	old := a.currentPrefs()
	oldSecrets, err := readSecrets(old)
	if err != nil {
		macos.Alert("Can't open Settings", sentence(err.Error()))
		return false
	}
	home, _ := os.UserHomeDir()
	form := formFrom(old, oldSecrets)
	form.Tab = tab
	form.DefaultLauncherLog, form.DefaultLlamaSwapLog = prefs.Prefs{}.LogPaths(a.paths.LogDir)
	form.FoundBinary = supervisor.FindBinary("llama-swap", supervisor.SearchDirs(home))
	form.File = a.paths.Prefs()

	for {
		edited, result, err := macos.Preferences(form)
		if err != nil {
			a.problem("The Settings window failed", err)
			return false
		}
		if result == macos.PreferencesCancel {
			return false
		}
		p, secrets, err := prefsFrom(edited)
		if err == nil {
			err = checkPaths(p)
		}
		if err != nil {
			form = edited
			form.Error = sentence(err.Error())
			form.Tab = "general"
			switch {
			case errors.As(err, new(secretError)):
				form.Tab = "secrets"
			case errors.As(err, new(logError)):
				form.Tab = "logs"
			}
			macos.Alert("These preferences can't be saved", form.Error)
			continue
		}
		// Opening llama-swap to the network without a key is worth a
		// second look, when this Save is what does it.
		if opensWithoutKey(old, p) && !macos.Confirm("Open llama-swap to the network without an API key?",
			"Listening on "+p.ListenAddr()+", llama-swap can be reached from other machines, and with no API key, "+
				"anyone who can reach it, on this network or any other this Mac joins, can use it: run and unload "+
				"models, and read its logs. To require a key, add one under Secrets, and list it in llama-swap's "+
				`config as apiKeys: ["${env.`+prefs.KeyPrefix+`NAME}"].`,
			"Save Anyway", "Back") {
			form = edited
			form.Error = ""
			form.Tab = "general"
			continue
		}
		// The secrets first: preferences naming one the keychain lacks
		// would stop llama-swap from starting.
		if err := saveSecrets(oldSecrets, secrets); err != nil {
			a.problem("Couldn't save the secrets", err)
			return false
		}
		if err := p.Save(a.paths.Prefs()); err != nil {
			a.problem("Couldn't save the preferences", err)
			return false
		}
		a.sup.Logf("preferences saved")
		a.mu.Lock()
		a.prefs = p
		a.mu.Unlock()
		// The logs move at once; the writers switch between lines.
		if err := a.logs.apply(p); err != nil {
			a.problem("A log couldn't be opened", err)
		}
		a.render() // the Logs submenu
		return a.applyPrefs(old, p, !maps.Equal(oldSecrets, secrets))
	}
}

// applyPrefs puts changed settings into effect. If anything llama-swap
// runs with changed, it offers to restart a running llama-swap and reports
// whether the user wants that; if only the health settings did, it
// restarts the health monitor.
func (a *app) applyPrefs(old, p prefs.Prefs, secretsChanged bool) (restart bool) {
	st := a.sup.Status().State
	if st != supervisor.Running && st != supervisor.Waiting {
		return false // the next start uses the new settings
	}
	if (llamaSwapChanged(old, p) || secretsChanged) &&
		macos.Confirm("Restart llama-swap with the new settings?",
			"It keeps running with the old ones until it restarts.", "Restart Now", "Later") {
		return true
	}
	if old.HealthCheckSeconds != p.HealthCheckSeconds || old.HealthCheckSkipWhenActive != p.HealthCheckSkipWhenActive {
		a.restartMonitor()
	}
	return false
}

// llamaSwapChanged reports whether a setting llama-swap runs with changed.
// The key the app uses counts: llama-swap's keys come from its environment,
// so the app keeps using the one it started llama-swap with.
func llamaSwapChanged(old, p prefs.Prefs) bool {
	return old.Binary != p.Binary || old.Config != p.Config || old.ListenAddr() != p.ListenAddr() ||
		!slices.Equal(old.Args, p.Args) || !maps.Equal(old.Env, p.Env) ||
		!slices.Equal(old.APIKeys, p.APIKeys) || old.AppKey() != p.AppKey() ||
		!slices.Equal(old.SecretEnv, p.SecretEnv)
}

// secretError and logError are problems on the Secrets and Logs tabs, so
// the dialog comes back there.
type (
	secretError struct{ error }
	logError    struct{ error }
)

// formFrom shows p, with its secrets' values (by variable), as the
// dialog's fields.
func formFrom(p prefs.Prefs, secrets map[string]string) macos.PreferencesForm {
	keys := []macos.SecretField{}
	for _, name := range p.APIKeys {
		keys = append(keys, macos.SecretField{Name: name, Value: secrets[prefs.KeyPrefix+name]})
	}
	vars := []macos.SecretField{}
	for _, name := range p.SecretEnv {
		vars = append(vars, macos.SecretField{Name: name, Value: secrets[name]})
	}
	var env []string
	for _, name := range slices.Sorted(maps.Keys(p.Env)) {
		env = append(env, name+"="+p.Env[name])
	}
	seconds := ""
	if p.HealthCheckSeconds > 0 {
		seconds = strconv.Itoa(p.HealthCheckSeconds)
	}
	return macos.PreferencesForm{
		Binary:                    p.Binary,
		Config:                    p.Config,
		Listen:                    p.Listen,
		Args:                      strings.Join(p.Args, "\n"),
		Env:                       strings.Join(env, "\n"),
		HealthCheckSeconds:        seconds,
		HealthCheckSkipWhenActive: p.HealthCheckSkipWhenActive,
		UnloadOnGPUFault:          p.UnloadOnGPUFault,
		MarkUpdatedModels:         p.MarkUpdatedModels,
		AutoStart:                 p.AutoStart,
		APIKeys:                   keys,
		AppAPIKey:                 p.AppKey(),
		SecretEnv:                 vars,
		LauncherLog:               p.LauncherLog,
		LlamaSwapLog:              p.LogMode(),
		LlamaSwapLogFile:          p.LlamaSwapLogFile,
		KeyPrefix:                 prefs.KeyPrefix,
		Tab:                       "general",
	}
}

// prefsFrom reads the dialog's fields back into preferences and the
// secrets' values (by variable), and checks them.
func prefsFrom(f macos.PreferencesForm) (prefs.Prefs, map[string]string, error) {
	p, err := generalFrom(f)
	if err != nil {
		return p, nil, err
	}
	secrets := map[string]string{}
	p.APIKeys = []string{}
	for i, k := range f.APIKeys {
		name, value := strings.TrimSpace(k.Name), strings.TrimSpace(k.Value)
		v := prefs.KeyPrefix + name
		switch {
		case name == "":
			return p, nil, secretError{fmt.Errorf("API keys, row %d: give the key a name", i+1)}
		case !prefs.ValidKeyName(name):
			return p, nil, secretError{fmt.Errorf("API keys: %s can only have capital letters, digits and underscores after %s", v, prefs.KeyPrefix)}
		case secrets[v] != "":
			return p, nil, secretError{fmt.Errorf("API keys: %s is listed twice", v)}
		case value == "":
			return p, nil, secretError{fmt.Errorf("API keys: %s has no key. Type or paste one, or use Generate", v)}
		case strings.ContainsFunc(value, unicode.IsSpace):
			return p, nil, secretError{fmt.Errorf("API keys: %s's key has spaces in it", v)}
		}
		secrets[v] = value
		p.APIKeys = append(p.APIKeys, name)
	}
	if _, ok := secrets[prefs.KeyPrefix+f.AppAPIKey]; ok && f.AppAPIKey != "" {
		p.AppAPIKey = f.AppAPIKey
	}
	p.SecretEnv = []string{}
	for i, sv := range f.SecretEnv {
		name, value := strings.TrimSpace(sv.Name), strings.TrimSpace(sv.Value)
		_, plain := p.Env[name]
		switch {
		case name == "":
			return p, nil, secretError{fmt.Errorf("secret variables, row %d: give the variable a name", i+1)}
		case !prefs.ValidVariable(name):
			return p, nil, secretError{fmt.Errorf("secret variables: %q isn't a valid variable name", name)}
		case strings.HasPrefix(name, prefs.KeyPrefix):
			return p, nil, secretError{fmt.Errorf("secret variables: %s starts with %s, which is kept for API keys", name, prefs.KeyPrefix)}
		case plain:
			return p, nil, secretError{fmt.Errorf("secret variables: %s is in Environment on the General tab too; keep one", name)}
		case secrets[name] != "":
			return p, nil, secretError{fmt.Errorf("secret variables: %s is listed twice", name)}
		case value == "":
			return p, nil, secretError{fmt.Errorf("secret variables: %s has no value", name)}
		}
		secrets[name] = value
		p.SecretEnv = append(p.SecretEnv, name)
	}
	if err := checkLogs(p); err != nil {
		return p, nil, logError{err}
	}
	return p, secrets, p.Validate()
}

// checkLogs checks the Logs tab's paths: whole paths, in folders that
// exist, and two different files.
func checkLogs(p prefs.Prefs) error {
	for _, f := range []struct{ name, path string }{
		{"launcher log", p.LauncherLog},
		{"llama-swap log", p.LlamaSwapLogFile},
	} {
		if f.path == "" {
			continue
		}
		full := prefs.ExpandHome(f.path)
		if !filepath.IsAbs(full) {
			return fmt.Errorf("%s: %q isn't a whole path, such as ~/Logs/%s; Choose… gives one", f.name, f.path, filepath.Base(full))
		}
		if info, err := os.Stat(filepath.Dir(full)); err != nil || !info.IsDir() {
			return fmt.Errorf("%s: the folder %s isn't there", f.name, filepath.Dir(full))
		}
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			return fmt.Errorf("%s: %s is a folder; give the file's name too", f.name, f.path)
		}
	}
	if launcher, own := p.LogPaths("/"); p.LogMode() == prefs.LogToFile && filepath.Clean(launcher) == filepath.Clean(own) {
		return errors.New("llama-swap log: it's the launcher log too. Give llama-swap's output a file of its own, or put it in the launcher log")
	}
	return nil
}

// generalFrom reads the General tab's fields into preferences.
func generalFrom(f macos.PreferencesForm) (prefs.Prefs, error) {
	p := prefs.Prefs{
		Binary:                    strings.TrimSpace(f.Binary),
		Config:                    strings.TrimSpace(f.Config),
		Listen:                    strings.TrimSpace(f.Listen),
		Args:                      []string{},
		Env:                       map[string]string{},
		AutoStart:                 f.AutoStart,
		HealthCheckSkipWhenActive: f.HealthCheckSkipWhenActive,
		UnloadOnGPUFault:          f.UnloadOnGPUFault,
		MarkUpdatedModels:         f.MarkUpdatedModels,
		LauncherLog:               strings.TrimSpace(f.LauncherLog),
		LlamaSwapLog:              f.LlamaSwapLog,
		LlamaSwapLogFile:          strings.TrimSpace(f.LlamaSwapLogFile),
	}
	for _, line := range lines(f.Args) {
		p.Args = append(p.Args, strings.TrimSpace(line))
	}
	for i, line := range lines(f.Env) {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return p, fmt.Errorf("environment, line %d: %q needs to be NAME=value", i+1, line)
		}
		p.Env[strings.TrimSpace(name)] = value
	}
	if s := strings.TrimSpace(f.HealthCheckSeconds); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return p, fmt.Errorf("health check every: %q isn't a whole number of seconds", s)
		}
		p.HealthCheckSeconds = n
	}
	return p, nil
}

// sentence capitalizes s's first letter, for showing an error on its own.
func sentence(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

// lines returns the non-blank lines of s.
func lines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimRight(line, "\r"); strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// checkPaths checks that the binary and config, if set, are there.
func checkPaths(p prefs.Prefs) error {
	if p.Binary != "" && !supervisor.IsExecutable(p.BinaryPath()) {
		return fmt.Errorf("llama-swap binary: %s isn't an executable file", p.Binary)
	}
	if p.Config != "" {
		if info, err := os.Stat(p.ConfigPath()); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("config file: %s isn't a file that can be read", p.Config)
		}
	}
	return nil
}
