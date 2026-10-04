// Package prefs stores the app's settings as a JSON file. The app's
// Preferences dialog edits it, and so can a person or a script: the file is
// meant to be readable, and the dialog's Open File… button opens it in a
// text editor. It's reread whenever llama-swap starts.
package prefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DefaultListen = "127.0.0.1:8080"

type Prefs struct {
	// Binary is the llama-swap executable. Empty means look in the usual
	// install locations.
	Binary string `json:"binary"`
	// Config is llama-swap's config file.
	Config string `json:"config"`
	// Listen is passed to llama-swap as -listen, and is where the app
	// expects to find it.
	Listen string `json:"listen"`
	// Args are extra llama-swap arguments, such as "-watch-config".
	Args []string `json:"args"`
	// Env adds to, or overrides, the environment llama-swap starts with.
	Env map[string]string `json:"env"`
	// AutoStart starts llama-swap when the app launches.
	AutoStart bool `json:"autoStart"`
	// HealthCheckSeconds is how often the app checks that llama-swap still
	// answers, which is how it notices a hang. Each check is a request that
	// llama-swap logs if its config logs HTTP requests. 0 means the default.
	HealthCheckSeconds int `json:"healthCheckSeconds"`
	// HealthCheckSkipWhenActive skips a check when llama-swap has sent
	// events since the last one, which already shows it's alive.
	HealthCheckSkipWhenActive bool `json:"healthCheckSkipWhenActive"`
	// UnloadOnGPUFault unloads a model as soon as its GPU backend fails,
	// rather than asking, for runs nobody is watching. Every request to
	// such a model fails until it's unloaded.
	UnloadOnGPUFault bool `json:"unloadOnGpuFault"`
	// MarkUpdatedModels marks a loaded model whose server program has
	// changed on disk since it loaded (after "brew upgrade", say): it runs
	// the old build until it's unloaded.
	MarkUpdatedModels bool `json:"markUpdatedModels"`
	// APIKeys names the llama-swap API keys the app keeps, without
	// KeyPrefix: "ADMIN" reaches llama-swap as the variable LLSL_ADMIN, which
	// its config uses as "${env.LLSL_ADMIN}". The values are in the login
	// keychain, never here.
	APIKeys []string `json:"apiKeys"`
	// AppAPIKey names the key the app sends on its own requests to
	// llama-swap. Empty means the first of APIKeys.
	AppAPIKey string `json:"appApiKey"`
	// SecretEnv names environment variables for llama-swap, like Env, whose
	// values are secret (HF_TOKEN, say): they're in the login keychain,
	// never here.
	SecretEnv []string `json:"secretEnv"`
	// LauncherLog is the app's own log: its notes (the [launcher] lines),
	// its crash reports, and llama-swap's output when LlamaSwapLog is
	// LogToLauncher. Empty means launcher.log in the app's log folder.
	LauncherLog string `json:"launcherLog"`
	// LlamaSwapLog is where llama-swap's output goes: LogToLauncher,
	// LogToFile (LlamaSwapLogFile) or LogOff. Empty means LogToLauncher.
	LlamaSwapLog string `json:"llamaSwapLog"`
	// LlamaSwapLogFile is llama-swap's own log, with LogToFile. Empty means
	// llama-swap.log in the app's log folder.
	LlamaSwapLogFile string `json:"llamaSwapLogFile"`
}

// Where llama-swap's output goes (LlamaSwapLog).
const (
	LogToLauncher = "launcher" // into the launcher log, among the app's notes
	LogToFile     = "file"     // its own file, exactly as llama-swap writes it
	LogOff        = "off"      // not saved; the app keeps its last lines in memory
)

// The logs' default names, in the app's log folder.
const (
	DefaultLauncherLog  = "launcher.log"
	DefaultLlamaSwapLog = "llama-swap.log"
)

// LogMode returns LlamaSwapLog, with empty meaning LogToLauncher.
func (p Prefs) LogMode() string {
	if p.LlamaSwapLog == "" {
		return LogToLauncher
	}
	return p.LlamaSwapLog
}

// LogPaths returns the launcher log and llama-swap's own log, with the
// defaults in dir for those not set, and "~/" expanded.
func (p Prefs) LogPaths(dir string) (launcher, llamaSwap string) {
	return logPath(p.LauncherLog, dir, DefaultLauncherLog), logPath(p.LlamaSwapLogFile, dir, DefaultLlamaSwapLog)
}

func logPath(path, dir, name string) string {
	if path == "" {
		return filepath.Join(dir, name)
	}
	return ExpandHome(path)
}

// BinaryPath and ConfigPath return Binary and Config with "~/" expanded.
// The preferences keep paths as they were typed, "~" included, so every
// use goes through these.
func (p Prefs) BinaryPath() string { return ExpandHome(p.Binary) }
func (p Prefs) ConfigPath() string { return ExpandHome(p.Config) }

// ExpandHome turns a leading "~/" into the home folder.
func ExpandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}

// KeyPrefix starts the name of every variable that carries an API key to
// llama-swap. It's reserved for them, so a key can't replace a variable
// llama-swap or its model servers need, such as PATH.
const KeyPrefix = "LLSL_"

// AppKey returns the name of the API key the app uses, or "" if there are
// none.
func (p Prefs) AppKey() string {
	if p.AppAPIKey != "" {
		return p.AppAPIKey
	}
	if len(p.APIKeys) > 0 {
		return p.APIKeys[0]
	}
	return ""
}

// validKeyName is what follows KeyPrefix: capital letters, digits and
// underscores, as environment variables conventionally use.
var validKeyName = regexp.MustCompile(`^[A-Z0-9_]+$`)

// ValidKeyName reports whether name can follow KeyPrefix.
func ValidKeyName(name string) bool { return validKeyName.MatchString(name) }

const DefaultHealthCheckSeconds = 60

func Defaults() Prefs {
	return Prefs{
		Listen:                    DefaultListen,
		Args:                      []string{},
		Env:                       map[string]string{},
		APIKeys:                   []string{},
		LlamaSwapLog:              LogToLauncher,
		SecretEnv:                 []string{},
		AutoStart:                 true,
		HealthCheckSeconds:        DefaultHealthCheckSeconds,
		HealthCheckSkipWhenActive: true,
	}
}

// HealthCheckInterval is HealthCheckSeconds as a duration, or the default
// if it isn't positive.
func (p Prefs) HealthCheckInterval() time.Duration {
	if p.HealthCheckSeconds <= 0 {
		return DefaultHealthCheckSeconds * time.Second
	}
	return time.Duration(p.HealthCheckSeconds) * time.Second
}

// Load reads the file at path. A missing file, or a missing key, gets the
// defaults. An unknown key is an error rather than being ignored, since it's
// almost always a misspelling of a real one, which would otherwise silently
// keep its default.
func Load(path string) (Prefs, error) {
	p := Defaults()
	b, err := os.ReadFile(path) // #nosec G304 -- the app's own preferences file
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Validate reports the first setting that can't be used as it is.
func (p Prefs) Validate() error {
	if p.Listen != "" {
		// The host may be empty: ":8080" means every interface, as it does
		// for llama-swap.
		_, port, err := net.SplitHostPort(p.Listen)
		n, perr := strconv.Atoi(port)
		if err != nil || perr != nil || n < 1 || n > 65535 {
			return fmt.Errorf("listen: %q isn't an address and port, such as 127.0.0.1:8080", p.Listen)
		}
	}
	for _, arg := range p.Args {
		if why := reservedFlags[flagName(arg)]; why != "" {
			return fmt.Errorf("args: %s %s", strings.TrimSpace(arg), why)
		}
	}
	if p.HealthCheckSeconds < 0 {
		return fmt.Errorf("healthCheckSeconds: %d is negative; use 0 for the default", p.HealthCheckSeconds)
	}
	for name := range p.Env {
		if err := checkVariable(name); err != nil {
			return fmt.Errorf("env: %w", err)
		}
	}
	secret := map[string]bool{}
	for _, name := range p.SecretEnv {
		if err := checkVariable(name); err != nil {
			return fmt.Errorf("secretEnv: %w", err)
		}
		if _, ok := p.Env[name]; ok {
			return fmt.Errorf("secretEnv: %s is in env too; keep one", name)
		}
		if secret[name] {
			return fmt.Errorf("secretEnv: %s is listed twice", name)
		}
		secret[name] = true
	}
	seen := map[string]bool{}
	for _, name := range p.APIKeys {
		switch {
		case !validKeyName.MatchString(name):
			return fmt.Errorf("apiKeys: %q can only have capital letters, digits and underscores", name)
		case seen[name]:
			return fmt.Errorf("apiKeys: %s is listed twice", name)
		}
		seen[name] = true
	}
	switch p.LogMode() {
	case LogToLauncher, LogToFile, LogOff:
	default:
		return fmt.Errorf("llamaSwapLog: %q isn't one of %q, %q or %q", p.LlamaSwapLog, LogToLauncher, LogToFile, LogOff)
	}
	// Where the logs are is checked when they're opened: a log on a disk
	// that isn't connected mustn't make the whole file unreadable.
	for key, path := range map[string]string{"launcherLog": p.LauncherLog, "llamaSwapLogFile": p.LlamaSwapLogFile} {
		if path != "" && !filepath.IsAbs(ExpandHome(path)) {
			return fmt.Errorf("%s: %q isn't a full path, such as ~/Logs/llama-swap.log", key, path)
		}
	}
	if launcher, own := p.LogPaths("/"); p.LogMode() == LogToFile && filepath.Clean(launcher) == filepath.Clean(own) {
		return fmt.Errorf("llamaSwapLogFile: it's the launcher log too; give llama-swap's output its own file, or put it in the launcher log")
	}
	if p.AppAPIKey != "" && !seen[p.AppAPIKey] {
		return fmt.Errorf("appApiKey: %q isn't one of the API keys", p.AppAPIKey)
	}
	return nil
}

// reservedFlags are llama-swap flags that can't go in Args, and why. The
// app runs llama-swap with -config and -listen itself, and Go's flag
// package takes a flag's last value, so a second one would make
// llama-swap listen where the app isn't watching, or run a config the app
// didn't validate. The rest would leave llama-swap unreachable or exiting.
var reservedFlags = map[string]string{
	"config":        "is set by the app, from the config file preference",
	"listen":        "is set by the app, from the listen address preference",
	"config-dir":    "isn't supported: the app validates and watches the one config file",
	"tls-cert-file": "isn't supported: the app watches llama-swap over plain HTTP, so it couldn't follow it serving HTTPS",
	"tls-key-file":  "isn't supported: the app watches llama-swap over plain HTTP, so it couldn't follow it serving HTTPS",
	"validate":      "would make llama-swap check its config and exit at once",
	"version":       "would make llama-swap print its version and exit at once",
}

// flagName returns the flag name in arg, as Go's flag package reads it:
// "listen" for -listen, --listen and -listen=:8080, and for "-listen :8080"
// written on one line. It returns "" for an argument that isn't a flag.
func flagName(arg string) string {
	arg = strings.TrimSpace(arg)
	name, ok := strings.CutPrefix(arg, "-")
	if !ok {
		return ""
	}
	name = strings.TrimPrefix(name, "-")
	if i := strings.IndexAny(name, "= \t"); i >= 0 {
		name = name[:i]
	}
	return name
}

// checkVariable checks a name for an environment variable from env or
// secretEnv.
func checkVariable(name string) error {
	if !ValidVariable(name) {
		return fmt.Errorf("%q isn't a valid variable name", name)
	}
	if strings.HasPrefix(name, KeyPrefix) {
		return fmt.Errorf("%s starts with %s, which is kept for API keys", name, KeyPrefix)
	}
	return nil
}

// ValidVariable reports whether name can name an environment variable:
// not empty, and without "=" or spaces.
func ValidVariable(name string) bool {
	return name != "" && !strings.ContainsAny(name, "= \t\n")
}

// Save writes the file atomically, so a crash can't leave it half-written.
func (p Prefs) Save(path string) error {
	b, err := json.MarshalIndent(p, "", "  ") // #nosec G117 -- APIKeys holds only the keys' names; their values are in the keychain
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ListenAddr is Listen, or the default if it's empty.
func (p Prefs) ListenAddr() string {
	if p.Listen == "" {
		return DefaultListen
	}
	return p.Listen
}
