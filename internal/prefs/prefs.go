// Package prefs stores the app's settings as a JSON file. The app's
// Settings dialog edits it, and so can a person or a script: the file is
// meant to be readable in a text editor. It's reread whenever llama-swap
// starts.
package prefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
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
	// Args are extra llama-swap arguments, for flags newer than the app.
	// The dialog doesn't show them; every flag the app knows is a setting.
	Args []string `json:"args"`
	// WatchConfig runs llama-swap with -watch-config, so it reloads its
	// config when the file, or the config folder, changes.
	WatchConfig bool `json:"watchConfig"`
	// ConfigDirMode is whether llama-swap also reads a folder of config
	// files (-config-dir): ConfigDirOff, ConfigDirOwn (ConfigDir) or
	// ConfigDirManaged (the app's own folder, where it writes the settings
	// it manages). Empty means ConfigDirOff.
	ConfigDirMode string `json:"configDirMode"`
	// ConfigDir is the folder for ConfigDirOwn. The app never writes there.
	ConfigDir string `json:"configDir"`
	// ManageAPIKeys makes llama-swap require the API keys, by listing them
	// in its apiKeys through a file in the managed config folder. It applies
	// only with ConfigDirManaged.
	ManageAPIKeys bool `json:"manageApiKeys"`
	// Peers are the peers the app manages for llama-swap: other llama-swaps,
	// or servers with the same API, whose models it serves too. They're
	// written to its peers in the managed config folder, so they apply only
	// with ConfigDirManaged.
	Peers []Peer `json:"peers"`
	// CORS is which web pages may use llama-swap from a browser, written to
	// its security.cors in the managed config folder, so it applies only
	// with ConfigDirManaged.
	CORS CORS `json:"cors"`
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

// Peer is one of llama-swap's peers, as the app manages it.
type Peer struct {
	// Name is llama-swap's name for it; its models are Name/model.
	Name string `json:"name"`
	// URL is where llama-swap sends its requests: http:// or https://.
	URL string `json:"url"`
	// KeyVariable names the secret variable (SecretEnv) holding its API key,
	// which llama-swap sends with each request. Empty means none.
	KeyVariable string `json:"keyVariable"`
	// Models are the peer's models that llama-swap offers.
	Models []string `json:"models"`
	// Timeouts are those llama-swap's requests to the peer use instead of
	// its defaults.
	Timeouts PeerTimeouts `json:"timeouts"`
}

// PeerTimeouts are the peer timeouts the app offers, in seconds, by
// llama-swap's names: three of its six, those that matter day to day. A
// nil one keeps llama-swap's default (DefaultResponseHeader and the
// rest), so it isn't written, and a peer follows if llama-swap changes
// it. 0 means no limit, which llama-swap advises against.
type PeerTimeouts struct {
	// ResponseHeader is how long llama-swap waits for the peer to start
	// answering: a slow machine, or a long prompt at a large context,
	// needs more.
	ResponseHeader *int `json:"responseHeader,omitempty"`
	// Connect is how long opening a connection to the peer may take.
	Connect *int `json:"connect,omitempty"`
	// KeepAlive is the connection's TCP keep-alive.
	KeepAlive *int `json:"keepalive,omitempty"`
}

// llama-swap's own defaults for the peer timeouts (v265), which the
// Settings dialog shows.
const (
	DefaultResponseHeader = 60
	DefaultConnect        = 30
	DefaultKeepAlive      = 30
)

// ManagesPeers reports whether the app writes Peers into llama-swap's
// config: there are some, and the managed folder is in use.
func (p Prefs) ManagesPeers() bool {
	return len(p.Peers) > 0 && p.ConfigDirSetting() == ConfigDirManaged
}

// validPeerName is a peer name that's safe in a variable-free YAML key and
// in the "peer/model" names llama-swap gives its models: letters, digits,
// dots, dashes and underscores, starting with a letter or digit.
var validPeerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// configVariable is a variable name llama-swap fills in as ${env.NAME}.
var configVariable = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// checkPeers checks Peers, against the secret variables for their keys.
func (p Prefs) checkPeers() error {
	secret := map[string]bool{}
	for _, name := range p.SecretEnv {
		secret[name] = true
	}
	seen := map[string]bool{}
	for _, peer := range p.Peers {
		switch {
		case !validPeerName.MatchString(peer.Name):
			return fmt.Errorf("peers: %q can only have letters, digits, dots, dashes and underscores", peer.Name)
		case seen[peer.Name]:
			return fmt.Errorf("peers: %s is listed twice", peer.Name)
		case peer.KeyVariable != "" && !secret[peer.KeyVariable]:
			return fmt.Errorf("peers: %s's key is %s, which isn't one of the secret variables", peer.Name, peer.KeyVariable)
		case peer.KeyVariable != "" && !configVariable.MatchString(peer.KeyVariable):
			return fmt.Errorf("peers: %s's key is %s, but llama-swap's config can only use variables named with letters, digits and underscores", peer.Name, peer.KeyVariable)
		case len(peer.Models) == 0:
			return fmt.Errorf("peers: %s has no models; llama-swap needs at least one", peer.Name)
		}
		seen[peer.Name] = true
		u, err := url.Parse(peer.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.Contains(peer.URL, "${") {
			return fmt.Errorf("peers: %s's URL %q isn't an http:// or https:// address", peer.Name, peer.URL)
		}
		for _, t := range []struct {
			name string
			v    *int
		}{{"wait for a reply", peer.Timeouts.ResponseHeader}, {"connect", peer.Timeouts.Connect}, {"keep-alive", peer.Timeouts.KeepAlive}} {
			if t.v != nil && *t.v < 0 {
				return fmt.Errorf("peers: %s's %s timeout is negative; use 0 for no limit", peer.Name, t.name)
			}
		}
		models := map[string]bool{}
		for _, m := range peer.Models {
			switch {
			case strings.TrimSpace(m) != m || m == "" || strings.ContainsAny(m, "\n\r") || strings.Contains(m, "${"):
				return fmt.Errorf("peers: %s has a model named %q, which llama-swap can't use", peer.Name, m)
			case models[m]:
				return fmt.Errorf("peers: %s lists %s twice", peer.Name, m)
			}
			models[m] = true
		}
	}
	return nil
}

// CORS is the part of llama-swap's security.cors the app manages, with
// llama-swap's names. Restrict off writes none of it, which leaves
// llama-swap's default: any web page may use it. llama-swap requires
// allowedOrigins for the rest, so they apply only with Restrict.
type CORS struct {
	// Restrict limits browser access to AllowedOrigins.
	Restrict bool `json:"restrict"`
	// AllowedOrigins are the web pages that may use llama-swap, each
	// scheme://host[:port], as a browser sends it. They're kept while
	// Restrict is off.
	AllowedOrigins []string `json:"allowedOrigins"`
	// AllowCredentials lets those pages send cookies and Authorization.
	AllowCredentials bool `json:"allowCredentials"`
	// AllowPrivateNetwork answers Chrome's Private Network Access check,
	// letting those pages, from public sites, reach llama-swap on this
	// network.
	AllowPrivateNetwork bool `json:"allowPrivateNetwork"`
	// MaxAge is how long, in seconds, a browser may keep a preflight's
	// answer. Nil keeps llama-swap's default, DefaultCORSMaxAge.
	MaxAge *int `json:"maxAge,omitempty"`
}

// DefaultCORSMaxAge is llama-swap's own default for cors.maxAge (v265),
// which the Settings dialog shows.
const DefaultCORSMaxAge = 86400

// ManagesCORS reports whether the app writes CORS into llama-swap's
// config: it restricts, and the managed folder is in use.
func (p Prefs) ManagesCORS() bool {
	return p.CORS.Restrict && p.ConfigDirSetting() == ConfigDirManaged
}

// checkCORS checks CORS as llama-swap would (its security.go): origins
// are scheme://host[:port] with nothing after, and restricting needs one.
func (p Prefs) checkCORS() error {
	seen := map[string]bool{}
	for _, o := range p.CORS.AllowedOrigins {
		u, err := url.Parse(o)
		switch {
		case o == "*":
			return errors.New(`cors: "*" means any web page; turn restricting off instead`)
		case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || strings.Contains(o, "${"):
			return fmt.Errorf("cors: %q isn't a web page's origin, such as http://localhost:3000, with nothing after the host and port", o)
		case seen[strings.ToLower(o)]:
			return fmt.Errorf("cors: %s is listed twice", o)
		}
		seen[strings.ToLower(o)] = true
	}
	if p.CORS.Restrict && len(p.CORS.AllowedOrigins) == 0 {
		return errors.New("cors: list at least one web page, or turn restricting off")
	}
	if p.CORS.MaxAge != nil && *p.CORS.MaxAge < 0 {
		return errors.New("cors: maxAge is negative")
	}
	return nil
}

// Whether llama-swap reads a config folder (ConfigDirMode).
const (
	ConfigDirOff     = "off"     // no -config-dir
	ConfigDirOwn     = "own"     // the user's folder, ConfigDir
	ConfigDirManaged = "managed" // the app's folder, with the settings it manages
)

// ConfigDirSetting returns ConfigDirMode, with empty meaning ConfigDirOff.
func (p Prefs) ConfigDirSetting() string {
	if p.ConfigDirMode == "" {
		return ConfigDirOff
	}
	return p.ConfigDirMode
}

// ConfigDirPath returns the folder llama-swap reads as -config-dir, given
// the app's managed folder, or "" for none.
func (p Prefs) ConfigDirPath(managed string) string {
	switch p.ConfigDirSetting() {
	case ConfigDirOwn:
		return ExpandHome(p.ConfigDir)
	case ConfigDirManaged:
		return managed
	}
	return ""
}

// ManagesAPIKeys reports whether the app writes the API keys into
// llama-swap's apiKeys: ManageAPIKeys, with the managed folder.
func (p Prefs) ManagesAPIKeys() bool {
	return p.ManageAPIKeys && p.ConfigDirSetting() == ConfigDirManaged
}

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
		ConfigDirMode:             ConfigDirOff,
		SecretEnv:                 []string{},
		Peers:                     []Peer{},
		CORS:                      CORS{AllowedOrigins: []string{}},
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
	var set struct {
		WatchConfig *bool `json:"watchConfig"`
	}
	_ = json.Unmarshal(b, &set) // the strict decode above already read it
	if err := p.liftWatchConfig(set.WatchConfig); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// liftWatchConfig moves a -watch-config in Args, from before it was a
// setting, into WatchConfig, in any form Go's flag package reads: -flag,
// --flag, -flag=value. Like that package, the last one counts. set is the
// watchConfig key's value if the file has one; if it disagrees, the file
// says two things, and that's an error rather than a guess.
func (p *Prefs) liftWatchConfig(set *bool) error {
	var kept []string
	found, value := false, false
	for _, arg := range p.Args {
		if flagName(arg) != "watch-config" {
			kept = append(kept, arg)
			continue
		}
		rest := strings.TrimLeft(strings.TrimSpace(arg), "-")[len("watch-config"):]
		v := true
		if rest != "" {
			b, err := strconv.ParseBool(strings.TrimPrefix(rest, "="))
			if !strings.HasPrefix(rest, "=") || err != nil {
				return fmt.Errorf("args: %q: use the watchConfig setting (true or false) instead", strings.TrimSpace(arg))
			}
			v = b
		}
		found, value = true, v
	}
	if !found {
		return nil
	}
	if set != nil && *set != value {
		return fmt.Errorf("watchConfig is %v, but args has -watch-config=%v; keep only watchConfig", *set, value)
	}
	if kept == nil {
		kept = []string{}
	}
	p.Args, p.WatchConfig = kept, value
	return nil
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
	switch p.ConfigDirSetting() {
	case ConfigDirOff, ConfigDirManaged:
	case ConfigDirOwn:
		// Whether it's there is checked when llama-swap starts, like the
		// logs: a folder on a disk that isn't connected mustn't make the
		// whole file unreadable.
		if p.ConfigDir == "" {
			return fmt.Errorf("configDir: give the folder, with configDirMode %q", ConfigDirOwn)
		}
		if !filepath.IsAbs(ExpandHome(p.ConfigDir)) {
			return fmt.Errorf("configDir: %q isn't a full path, such as ~/llama-swap/config.d", p.ConfigDir)
		}
	default:
		return fmt.Errorf("configDirMode: %q isn't one of %q, %q or %q", p.ConfigDirMode, ConfigDirOff, ConfigDirOwn, ConfigDirManaged)
	}
	if err := p.checkPeers(); err != nil {
		return err
	}
	if err := p.checkCORS(); err != nil {
		return err
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
	"config":         "is set by the app, from the config file preference",
	"listen":         "is set by the app, from the listen address preference",
	"config-dir":     "is set by the app, from the config folder setting",
	"watch-config":   "is a setting: watchConfig, or Reload the config when it changes in Settings",
	"tls-cert-file":  "isn't supported: the app watches llama-swap over plain HTTP, so it couldn't follow it serving HTTPS",
	"tls-key-file":   "isn't supported: the app watches llama-swap over plain HTTP, so it couldn't follow it serving HTTPS",
	"listen-tailcat": "isn't supported: a second listener, which the app can't watch or check for API keys",
	"validate":       "would make llama-swap check its config and exit at once",
	"version":        "would make llama-swap print its version and exit at once",
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
