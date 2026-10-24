package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
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

// A file that already has the content isn't written again, so a llama-swap
// watching the folder doesn't reload, and unload its models, for nothing.
func TestWriteConfigFilesLeavesUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.yaml")
	spec := Spec{ConfigDir: dir, ConfigFiles: map[string][]byte{"peers.yaml": []byte(header + "peers: {}\n")}}
	if err := writeConfigFiles(spec); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	os.Chtimes(path, past, past)
	if err := writeConfigFiles(spec); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); !info.ModTime().Equal(past) {
		t.Errorf("the same content was written again: modified %v", info.ModTime())
	}
	spec.ConfigFiles["peers.yaml"] = []byte(header + "peers: {a: {}}\n")
	if err := writeConfigFiles(spec); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.ModTime().Equal(past) {
		t.Error("new content wasn't written")
	}
}

// UpdateConfigFiles writes new files for a running llama-swap that watches
// its config, without restarting it, and a restart after a crash uses
// them; one that doesn't watch is left alone, and with none running
// there's nothing to do.
func TestUpdateConfigFiles(t *testing.T) {
	policy := Policy{Backoff: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond, MaxCrashes: 3, Window: time.Minute, StableAfter: time.Hour}
	s, _ := newTestSupervisor(t, policy)
	dir := filepath.Join(t.TempDir(), "llama-swap.d")
	path := filepath.Join(dir, "api-keys.yaml")
	spec := func(content string, watch, keys bool) Spec {
		sp := shSpec("sleep 30")
		sp.ConfigDir, sp.WatchConfig, sp.KeysRequired = dir, watch, keys
		sp.ConfigFiles = map[string][]byte{"api-keys.yaml": []byte(header + content)}
		return sp
	}
	read := func() string { b, _ := os.ReadFile(path); return strings.TrimPrefix(string(b), header) }

	if err := s.UpdateConfigFiles(spec("none", true, false)); err != nil || read() != "" {
		t.Errorf("with nothing running: %v, file %q", err, read())
	}

	if err := s.Start(spec("one", true, false)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Running", func() bool { return s.Status().State == Running })
	pid := s.Status().Pid
	if err := s.UpdateConfigFiles(spec("two", true, true)); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); read() != "two" || !st.KeysRequired || st.Pid != pid || st.State != Running {
		t.Errorf("after the update: file %q, status %+v (pid was %d)", read(), st, pid)
	}
	// A crash restart writes the updated files, not those it started with.
	os.WriteFile(path, []byte(header+"stale"), 0o600)
	syscall.Kill(pid, syscall.SIGKILL)
	waitFor(t, "a restart", func() bool { st := s.Status(); return st.State == Running && st.Pid != pid })
	if read() != "two" {
		t.Errorf("the restart wrote %q, want the updated files", read())
	}

	if err := s.Restart(spec("three", false, false)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Running", func() bool { return s.Status().State == Running })
	if err := s.UpdateConfigFiles(spec("four", false, false)); !errors.Is(err, ErrNotWatching) || read() != "three" {
		t.Errorf("not watching: got %v, file %q", err, read())
	}
}
