package wired

import "testing"

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
	if got := AppRequirement("com.my-wang.llama-swap-launcher", "P56KW9H72P"); got !=
		`anchor apple generic and identifier "com.my-wang.llama-swap-launcher" and certificate leaf[subject.OU] = "P56KW9H72P"` {
		t.Errorf("app: %s", got)
	}
	if got := HelperRequirement("com.my-wang.llama-swap-launcher.dev", "P56KW9H72P"); got !=
		`anchor apple generic and identifier "com.my-wang.llama-swap-launcher.dev.wired-limit" and certificate leaf[subject.OU] = "P56KW9H72P"` {
		t.Errorf("helper: %s", got)
	}
}
