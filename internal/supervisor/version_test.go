package supervisor

import (
	"os"
	"testing"
)

func TestParseVersion(t *testing.T) {
	for out, want := range map[string][2]string{
		"Version: v260 (fcefa7b), built at 2026-09-26T15:25:50Z\n":         {"v260 (fcefa7b)", "2026-09-26T15:25:50Z"}, // v260's wording
		"version: v262 (079c35a), built at 2026-10-03T06:29:19Z\n":         {"v262 (079c35a)", "2026-10-03T06:29:19Z"}, // v262's
		"version: v269-2-g581d29 (581d29c), built at 2026-11-01T03:36:54Z": {"v269-2-g581d29 (581d29c)", "2026-11-01T03:36:54Z"},
		"version: v270-12-g581d29c-dirty (581d29c)\n":                      {"v270-12-g581d29c-dirty (581d29c)", ""},
		"version: v269-dirty, built at 2026-11-01T03:36:54.5+02:00\n":      {"v269-dirty", "2026-11-01T03:36:54.5+02:00"},
		"version: v269-rc1\n":                       {"v269", ""},
		"version: v263\n":                           {"v263", ""},
		"flag provided but not defined: -version\n": {"", ""},
		"built at sometime\n":                       {"", ""},
		"":                                          {"", ""},
	} {
		if v, built := parseVersion(out); v != want[0] || built != want[1] {
			t.Errorf("parseVersion(%q) = %q, %q, want %q, %q", out, v, built, want[0], want[1])
		}
	}
}

func TestVersionNumber(t *testing.T) {
	for v, want := range map[string]int{"v262 (079c35a)": 262, "v269-2-g581d29 (581d29c)": 269, "v7": 7, "": 0, "dev": 0} {
		if got := VersionNumber(v); got != want {
			t.Errorf("VersionNumber(%q) = %d, want %d", v, got, want)
		}
	}
}

// With LLAMA_SWAP_BIN set, BinaryVersion runs the real llama-swap.
func TestBinaryVersionReal(t *testing.T) {
	bin := os.Getenv("LLAMA_SWAP_BIN")
	if bin == "" {
		t.Skip("set LLAMA_SWAP_BIN to run against a real llama-swap")
	}
	v, built := BinaryVersion(bin)
	t.Logf("%s reports %q, built %q", bin, v, built)
	if VersionNumber(v) == 0 {
		t.Errorf("no version from %s", bin)
	}
	if built == "" {
		t.Errorf("no build time from %s", bin)
	}
}
