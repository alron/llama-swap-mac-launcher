package supervisor

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const header = "# Written by the app\n"

func TestWriteConfigFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "llama-swap.d")
	spec := Spec{ConfigDir: dir, ConfigFiles: map[string][]byte{"api-keys.yaml": []byte(header + "apiKeys: [a]\n")}}
	if err := writeConfigFiles(spec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "api-keys.yaml")
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("folder: %v, %v", info.Mode(), err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file: %v, %v", info.Mode(), err)
	}

	// Its own file, it replaces.
	spec.ConfigFiles["api-keys.yaml"] = []byte(header + "apiKeys: [b]\n")
	if err := writeConfigFiles(spec); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "[b]") {
		t.Errorf("not replaced: %q", b)
	}

	// Someone else's, it leaves alone.
	os.WriteFile(path, []byte("apiKeys: [mine]\n"), 0o600)
	if err := writeConfigFiles(spec); err == nil || !strings.Contains(err.Error(), "didn't write it") {
		t.Errorf("got %v, want a refusal", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "apiKeys: [mine]\n" {
		t.Errorf("someone else's file changed: %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left something behind: %v", entries)
	}
}

// Validate checks the app's files in a copy of the folder: llama-swap
// sees them with the folder's other YAML files, the real folder isn't
// touched, and the message names the real folder.
func TestValidateStagesConfigDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "user.yaml"), []byte("logLevel: info\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not config\n"), 0o600)
	bin := filepath.Join(t.TempDir(), "llama-swap")
	os.WriteFile(bin, []byte(`#!/bin/sh
while [ $# -gt 0 ]; do [ "$1" = -config-dir ] && dir=$2; shift; done
ls "$dir" >&2
cat "$dir/api-keys.yaml" >&2
echo "config $dir/api-keys.yaml: something's wrong" >&2
exit 1
`), 0o755)
	spec := Spec{
		Binary:      bin,
		Args:        []string{"-config", "x.yaml", "-config-dir", dir},
		ConfigDir:   dir,
		ConfigFiles: map[string][]byte{"api-keys.yaml": []byte(header + "apiKeys: [new]\n")},
	}
	err := Validate(spec)
	if err == nil {
		t.Fatal("the stand-in llama-swap failed, but Validate didn't")
	}
	msg := err.Error()
	for _, want := range []string{"user.yaml", "api-keys.yaml", "[new]", "config " + dir + "/api-keys.yaml: something's wrong"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "notes.txt") || strings.Contains(msg, os.TempDir()+"llama-swap-config-") {
		t.Errorf("message shows a non-YAML file or the copy's path: %s", msg)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("the real folder changed: %v", entries)
	}
}

func TestWithConfigDir(t *testing.T) {
	args := []string{"-config", "x.yaml", "-config-dir", "/real", "-watch-config"}
	if got := withConfigDir(args, "/tmp/copy"); !slices.Equal(got, []string{"-config", "x.yaml", "-config-dir", "/tmp/copy", "-watch-config"}) {
		t.Errorf("got %q", got)
	}
	if args[3] != "/real" {
		t.Error("changed the caller's args")
	}
	if got := withConfigDir([]string{"-config", "x.yaml"}, "/tmp/copy"); !slices.Equal(got, []string{"-config", "x.yaml", "-config-dir", "/tmp/copy"}) {
		t.Errorf("got %q", got)
	}
}

// A YAML name in the folder that can't be read fails the check, as it
// would fail llama-swap's start.
func TestValidateUnreadableConfigFile(t *testing.T) {
	dir := t.TempDir()
	os.Symlink(filepath.Join(dir, "gone.yaml"), filepath.Join(dir, "dangling.yaml"))
	bin := filepath.Join(t.TempDir(), "llama-swap")
	os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	err := Validate(Spec{Binary: bin, Args: []string{"-config-dir", dir}, ConfigDir: dir,
		ConfigFiles: map[string][]byte{"api-keys.yaml": []byte(header)}})
	if err == nil || !strings.Contains(err.Error(), "dangling.yaml") {
		t.Errorf("got %v, want an error naming dangling.yaml", err)
	}
}
