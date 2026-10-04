// Package paths derives the app's per-user file locations from its bundle
// ID, so a dev build and an installed release never share files.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	// SupportDir holds the preferences, the pidfile and the control socket.
	SupportDir string
	// LogDir holds the logs, unless the preferences put them elsewhere.
	LogDir string
}

func New(bundleID string) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		SupportDir: filepath.Join(home, "Library", "Application Support", bundleID),
		LogDir:     filepath.Join(home, "Library", "Logs", bundleID),
	}, nil
}

// Create makes the directories, readable only by the user.
func (p Paths) Create() error {
	for _, dir := range []string{p.SupportDir, p.LogDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// Socket is the control socket. Its name is short because a socket's whole
// path is limited to 103 bytes, and this folder's path already takes about
// 80 plus the username.
func (p Paths) Socket() string  { return filepath.Join(p.SupportDir, "ctl") }
func (p Paths) Prefs() string   { return filepath.Join(p.SupportDir, "prefs.json") }
func (p Paths) PidFile() string { return filepath.Join(p.SupportDir, "llama-swap.pid") }

// OldSocket is where versions up to 0.1.2 put the control socket. llsl only
// checks it to recognise an older app still running after an upgrade.
func (p Paths) OldSocket() string { return filepath.Join(p.SupportDir, "control.sock") }
