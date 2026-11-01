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
// tell, and when it was built, such as "2026-10-03T06:29:19Z", or "".
// Running the binary needs no API key, unlike llama-swap's /api/version,
// and works even when llama-swap then fails to start.
//
// llama-swap's wording has changed between releases ("Version: v260 …",
// then "version: v262 …"), so this looks for the version itself, not the
// words around it.
func BinaryVersion(binary string) (version, built string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "-version").CombinedOutput() // #nosec G204 -- the user's chosen llama-swap, run with -version
	if err != nil {
		return "", ""
	}
	return parseVersion(string(out))
}

// versionPattern matches a release number, then what git adds to a build
// from source (commits since the release, the commit, and whether the tree
// was changed), then an optional commit in brackets: "v262 (079c35a)",
// "v269-2-g581d29 (581d29c)".
var versionPattern = regexp.MustCompile(`\bv\d+(?:-\d+-g[0-9a-f]+)?(?:-dirty)?\b(?: \([0-9a-f]+\))?`)

// builtPattern matches the build time after the version: "built at
// 2026-10-03T06:29:19Z".
var builtPattern = regexp.MustCompile(`\bbuilt at (\d{4}-\d\d-\d\dT[0-9:.]+(?:Z|[+-]\d\d:\d\d))`)

var numberPattern = regexp.MustCompile(`^v(\d+)`)

func parseVersion(out string) (version, built string) {
	version = versionPattern.FindString(out)
	if m := builtPattern.FindStringSubmatch(out); m != nil {
		built = m[1]
	}
	return version, built
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
