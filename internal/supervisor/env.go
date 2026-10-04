package supervisor

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SearchDirs lists where llama-swap, and the tools its config runs, are
// usually installed. A GUI app's PATH includes none of them.
func SearchDirs(home string) []string {
	return []string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		filepath.Join(home, "go", "bin"),
	}
}

var systemPath = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}

// FindBinary returns the first executable called name in dirs, or "".
func FindBinary(name string, dirs []string) string {
	for _, dir := range dirs {
		if p := filepath.Join(dir, name); IsExecutable(p) {
			return p
		}
	}
	return ""
}

// IsExecutable reports whether path is a regular file with an execute bit.
func IsExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// inheritedVars are the only variables taken from the app's own
// environment. The rest of the app's environment depends on how the app
// was launched (`open` from a shell passes the shell's, Finder and login
// launches get launchd's bare one), so passing it on would make
// llama-swap behave differently depending on who started the app.
var inheritedVars = []string{"HOME", "USER", "LOGNAME", "SHELL", "TMPDIR"}

// BuildEnv returns the environment to start llama-swap with: a few
// variables from inherited, a PATH with binDir and SearchDirs ahead of the
// system directories, then extra, which can override anything, PATH
// included.
func BuildEnv(inherited []string, home, binDir string, extra map[string]string) []string {
	env := make(map[string]string)
	for _, kv := range inherited {
		if k, v, ok := strings.Cut(kv, "="); ok && slices.Contains(inheritedVars, k) {
			env[k] = v
		}
	}
	if env["HOME"] == "" {
		env["HOME"] = home
	}

	var dirs []string
	if binDir != "" {
		dirs = append(dirs, binDir)
	}
	dirs = append(dirs, SearchDirs(home)...)
	dirs = append(dirs, systemPath...)
	var path []string
	for _, d := range dirs {
		if !slices.Contains(path, d) {
			path = append(path, d)
		}
	}
	env["PATH"] = strings.Join(path, ":")

	maps.Copy(env, extra)

	out := make([]string, 0, len(env))
	for _, k := range slices.Sorted(maps.Keys(env)) {
		out = append(out, k+"="+env[k])
	}
	return out
}
