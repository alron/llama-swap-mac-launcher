package wired

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bounds, whatever the helper is sent: 0 is macOS's default; otherwise
// at least 8 GB left for macOS, and only on a Mac of more than 32 GB.
func TestCheck(t *testing.T) {
	const gb = 1024
	for _, tt := range []struct {
		mb, total int64
		ok        bool
	}{
		{0, 16 * gb, true}, // macOS's default, anywhere
		{86 * gb, 96 * gb, true},
		{88 * gb, 96 * gb, true},    // exactly 8 GB left
		{88*gb + 1, 96 * gb, false}, // less than 8 GB left
		{224 * gb, 256 * gb, true},  // the M5 Ultra's 224 GB
		{249 * gb, 256 * gb, false}, // 7 GB left
		{24 * gb, 32 * gb, false},   // a 32 GB Mac: refused outright
		{30 * gb, 36 * gb, false},   // 6 GB left on a 36 GB Mac
		{28 * gb, 36 * gb, true},
		{-1, 96 * gb, false},
		{1, 96 * gb, false},        // a typo, say
		{8*gb - 1, 96 * gb, false}, // under the floor
		{8 * gb, 96 * gb, true},
	} {
		if err := Check(tt.mb, tt.total); (err == nil) != tt.ok {
			t.Errorf("%d MB on a %d MB Mac: %v", tt.mb, tt.total, err)
		}
	}
}

// The maintainer's table, and a size between two rows taking the row below.
func TestSuggestedReserve(t *testing.T) {
	for total, want := range map[int64]int64{36: 8, 48: 8, 64: 8, 96: 12, 128: 16, 192: 20, 256: 24, 384: 24, 512: 32, 1024: 32} {
		if got := SuggestedReserveGB(total); got != want {
			t.Errorf("%d GB: %d, want %d", total, got, want)
		}
	}
}

// The helper and the app trust each other by bundle ID and team.
func TestRequirements(t *testing.T) {
	// Developer ID only: the intermediate's and the leaf's markers, as in
	// codesign's designated requirement.
	const devID = ` and certificate 1[field.1.2.840.113635.100.6.2.6] and certificate leaf[field.1.2.840.113635.100.6.1.13]`
	if got := AppRequirement("com.my-wang.llama-swap-launcher", "P56KW9H72P"); got !=
		`anchor apple generic and identifier "com.my-wang.llama-swap-launcher"`+devID+` and certificate leaf[subject.OU] = "P56KW9H72P"` {
		t.Errorf("app: %s", got)
	}
	if got := HelperRequirement("com.my-wang.llama-swap-launcher.dev", "P56KW9H72P"); got !=
		`anchor apple generic and identifier "com.my-wang.llama-swap-launcher.dev.wired-limit"`+devID+` and certificate leaf[subject.OU] = "P56KW9H72P"` {
		t.Errorf("helper: %s", got)
	}
}

// The stored limit: a plain file holding a number; a symlink, a folder or
// anything else is refused, without repeating what it held.
func TestStored(t *testing.T) {
	dir := t.TempDir()
	path := func(name string) string { return filepath.Join(dir, name) }
	read := func(p string) (int64, error) { return stored(p) }
	if mb, err := read(path("none")); mb != 0 || err != nil {
		t.Errorf("missing: %d, %v", mb, err)
	}
	_ = os.WriteFile(path("limit"), []byte("86016\n"), 0o644)
	if mb, err := read(path("limit")); mb != 86016 || err != nil {
		t.Errorf("limit: %d, %v", mb, err)
	}
	_ = os.WriteFile(path("secret"), []byte("hunter2\n"), 0o600)
	if _, err := read(path("secret")); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("not a number: %v", err)
	}
	_ = os.Symlink(path("limit"), path("link"))
	if _, err := read(path("link")); err == nil {
		t.Error("a symlink was followed")
	}
	_ = os.Mkdir(path("folder"), 0o755)
	if _, err := read(path("folder")); err == nil {
		t.Error("a folder was read")
	}
}
