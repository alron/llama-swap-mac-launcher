package supervisor

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBuildEnv(t *testing.T) {
	inherited := []string{
		"HOME=/Users/u",
		"USER=u",
		"PATH=/some/shell/path",
		"TMUX=/tmp/tmux-501/default",
		"TMPDIR=/var/folders/x/T/",
	}
	env := BuildEnv(inherited, "/Users/u", "/custom/bin", map[string]string{"HF_TOKEN": "t"})
	want := []string{
		"HF_TOKEN=t",
		"HOME=/Users/u",
		"PATH=/custom/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/Users/u/go/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"TMPDIR=/var/folders/x/T/",
		"USER=u",
	}
	if !slices.Equal(env, want) {
		t.Errorf("got\n%q\nwant\n%q", env, want)
	}
}

func TestBuildEnvExtraOverridesPathAndFillsHome(t *testing.T) {
	env := BuildEnv(nil, "/Users/u", "/opt/homebrew/bin", map[string]string{"PATH": "/mine"})
	if !slices.Contains(env, "PATH=/mine") || !slices.Contains(env, "HOME=/Users/u") {
		t.Errorf("got %q", env)
	}
}

func TestBuildEnvDoesNotRepeatBinDir(t *testing.T) {
	env := BuildEnv(nil, "/Users/u", "/opt/homebrew/bin", nil)
	want := "PATH=/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/Users/u/go/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	if !slices.Contains(env, want) {
		t.Errorf("got %q", env)
	}
}

func TestFindBinary(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(a, "llama-swap"), []byte("not executable"), 0o644)
	os.WriteFile(filepath.Join(b, "llama-swap"), []byte("#!/bin/sh\n"), 0o755)
	if got := FindBinary("llama-swap", []string{a, b}); got != filepath.Join(b, "llama-swap") {
		t.Errorf("got %q", got)
	}
	if got := FindBinary("missing", []string{a, b}); got != "" {
		t.Errorf("got %q for a missing binary", got)
	}
}
