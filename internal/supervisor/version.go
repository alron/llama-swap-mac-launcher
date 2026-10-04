package supervisor

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// BinaryVersion asks the llama-swap at binary for its version and returns
// it as llama-swap writes it, such as "v262 (079c35a)", or "" if it can't
// tell. Running the binary needs no API key, unlike llama-swap's
// /api/version, and works even when llama-swap then fails to start.
//
// llama-swap's wording has changed between releases ("Version: v260 …",
// then "version: v262 …"), so this looks for the version itself, not the
// words around it.
func BinaryVersion(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "-version").CombinedOutput() // #nosec G204 -- the user's chosen llama-swap, run with -version
	if err != nil {
		return ""
	}
	return parseVersion(string(out))
}

// versionPattern matches a release number, then an optional commit in
// brackets: "v262 (079c35a)".
var versionPattern = regexp.MustCompile(`\bv\d+\b(?: \([0-9a-f]+\))?`)

var numberPattern = regexp.MustCompile(`^v(\d+)`)

func parseVersion(out string) string {
	return versionPattern.FindString(out)
}

// VersionNumber returns the release number in a version from
// BinaryVersion (262 for "v262 (079c35a)"), or 0 if there isn't one.
func VersionNumber(v string) int {
	m := numberPattern.FindStringSubmatch(v)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1]) // the pattern only matches digits
	return n
}
