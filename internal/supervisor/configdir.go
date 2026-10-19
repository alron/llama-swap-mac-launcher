package supervisor

// The files the app manages in llama-swap's config folder (-config-dir):
// written just before llama-swap starts, after any old one has stopped, so
// a llama-swap watching the folder never reads a file naming variables it
// wasn't started with; and checked first in a copy of the folder, so a
// file that fails the check is never left in the real one.

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// writeConfigFiles writes spec.ConfigFiles into spec.ConfigDir, creating
// the folder if need be. It only replaces a file the app wrote: one whose
// first line is the new content's, since every managed file starts with a
// line saying who writes it.
func writeConfigFiles(spec Spec) error {
	if len(spec.ConfigFiles) == 0 {
		return nil
	}
	if err := os.MkdirAll(spec.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("can't create the config folder: %w", err)
	}
	for _, name := range slices.Sorted(maps.Keys(spec.ConfigFiles)) {
		data := spec.ConfigFiles[name]
		path := filepath.Join(spec.ConfigDir, name)
		if old, err := os.ReadFile(path); err == nil && !bytes.Equal(firstLine(old), firstLine(data)) { // #nosec G304 -- a file in the app's managed folder
			return fmt.Errorf("won't replace %s: the app didn't write it. Move it out of the folder", path)
		}
		// Written beside it and renamed, so llama-swap never reads half a
		// file; it only reads names ending .yaml or .yml.
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return fmt.Errorf("can't write %s: %w", path, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp) // best effort
			return fmt.Errorf("can't write %s: %w", path, err)
		}
	}
	return nil
}

func firstLine(b []byte) []byte {
	line, _, _ := bytes.Cut(b, []byte("\n"))
	return line
}

// stageConfigDir copies the config folder's YAML files into a new temporary
// folder, with spec.ConfigFiles in place of the app's own, for -validate to
// check what llama-swap will read. The caller removes it.
func stageConfigDir(spec Spec) (string, error) {
	tmp, err := os.MkdirTemp("", "llama-swap-config-")
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(spec.ConfigDir)
	if err != nil && !os.IsNotExist(err) {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	files := maps.Clone(spec.ConfigFiles)
	for _, e := range entries {
		name := e.Name()
		// What llama-swap reads: the folder's own *.yaml and *.yml files.
		if e.IsDir() || !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			continue
		}
		if _, ours := files[name]; ours {
			continue
		}
		data, err := os.ReadFile(filepath.Join(spec.ConfigDir, name)) // #nosec G304 -- a file in llama-swap's config folder
		if err != nil {
			// A symlink to nothing, or to a folder: llama-swap would fail
			// on it too, so the check fails here rather than the start.
			_ = os.RemoveAll(tmp) // best effort
			return "", fmt.Errorf("llama-swap would read %s, and it can't be read: %w", filepath.Join(spec.ConfigDir, name), err)
		}
		files[name] = data
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0o600); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
	}
	return tmp, nil
}

// withConfigDir returns args with -config-dir's value replaced by dir.
func withConfigDir(args []string, dir string) []string {
	out := slices.Clone(args)
	for i := 0; i+1 < len(out); i++ {
		if out[i] == "-config-dir" {
			out[i+1] = dir
			return out
		}
	}
	return append(out, "-config-dir", dir)
}
