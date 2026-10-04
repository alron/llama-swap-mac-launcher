package supervisor

import (
	"os"
	"testing"
)

func TestParseVersion(t *testing.T) {
	for out, want := range map[string]string{
		"Version: v260 (fcefa7b), built at 2026-09-26T15:25:50Z\n": "v260 (fcefa7b)", // v260's wording
		"version: v262 (079c35a), built at 2026-10-03T06:29:19Z\n": "v262 (079c35a)", // v262's
		"version: v263\n": "v263",
		"flag provided but not defined: -version\n": "",
		"": "",
	} {
		if got := parseVersion(out); got != want {
			t.Errorf("parseVersion(%q) = %q, want %q", out, got, want)
		}
	}
}

func TestVersionNumber(t *testing.T) {
	for v, want := range map[string]int{"v262 (079c35a)": 262, "v7": 7, "": 0, "dev": 0} {
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
	v := BinaryVersion(bin)
	t.Logf("%s reports %q", bin, v)
	if VersionNumber(v) == 0 {
		t.Errorf("no version from %s", bin)
	}
}
