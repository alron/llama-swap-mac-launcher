// Package wired holds what the app and its root helper share about the
// GPU's wired memory limit (TODO item 51): iogpu.wired_limit_mb, its
// bounds, the reserve Settings suggests, and the names and code signing
// requirements by which each finds and trusts the other.
package wired

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	// ReserveMB is always left for macOS: the helper refuses less,
	// whatever it's sent.
	ReserveMB = 8 * 1024
	// SmallMB: on a Mac this size or smaller, a limit is refused.
	SmallMB = 32 * 1024
	// MinMB is the least limit set: a tiny one, a typo say, could starve
	// the window server of GPU memory.
	MinMB = 8 * 1024
)

// Check enforces the bounds on a limit of mb for a Mac of totalMB: 0 is
// macOS's default; otherwise at least MinMB, leaving at least ReserveMB
// for macOS, on a Mac of more than SmallMB.
func Check(mb, totalMB int64) error {
	switch {
	case mb == 0:
		return nil
	case mb < 0:
		return fmt.Errorf("%d MB isn't a limit", mb)
	case totalMB <= SmallMB:
		return fmt.Errorf("this Mac has %d GB; the limit is only changed on Macs with more than %d GB", totalMB>>10, SmallMB>>10)
	case mb < MinMB:
		return fmt.Errorf("%d MB is less than the least the app sets, %d GB", mb, MinMB>>10)
	case mb > totalMB-ReserveMB:
		return fmt.Errorf("%d MB would leave macOS less than %d GB of the %d GB", mb, ReserveMB>>10, totalMB>>10)
	}
	return nil
}

// SuggestedReserveGB is how much Settings suggests leaving for macOS on a
// Mac of totalGB, and warns below: a size between two rows takes the row
// below it (384 GB: 24). The maintainer's table, 2026-10-26.
func SuggestedReserveGB(totalGB int64) int64 {
	for _, row := range [][2]int64{{512, 32}, {256, 24}, {192, 20}, {128, 16}, {96, 12}} {
		if totalGB >= row[0] {
			return row[1]
		}
	}
	return 8
}

// Label is the helper's launchd label and Mach service, for the app with
// bundleID: the dev and release apps get separate helpers.
func Label(bundleID string) string { return bundleID + ".wired-limit" }

// Plist is the helper's LaunchDaemon plist, in the app's
// Contents/Library/LaunchDaemons.
func Plist(bundleID string) string { return Label(bundleID) + ".plist" }

// LogPath is the helper's log: its plist's StandardErrorPath.
func LogPath(bundleID string) string { return "/Library/Logs/" + Label(bundleID) + ".log" }

// StoredPath is where the helper keeps the limit it applies at every
// start: root's to write, anyone's to read.
func StoredPath(bundleID string) string {
	return filepath.Join("/Library/Application Support", Label(bundleID), "wired-limit-mb")
}

// Stored reads the limit the helper keeps for the app with bundleID; 0
// when there's none. It refuses a symlink, or anything but a plain file,
// and never repeats what it found: the helper reads it as root, and logs
// to a file anyone can read.
func Stored(bundleID string) (int64, error) { return stored(StoredPath(bundleID)) }

func stored(path string) (int64, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) // #nosec G304 -- the helper's own file, from the bundle ID
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return 0, fmt.Errorf("%s isn't a plain file", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, 64))
	if err != nil {
		return 0, err
	}
	mb, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || mb < 0 {
		return 0, fmt.Errorf("%s doesn't hold a limit", path)
	}
	return mb, nil
}

// requirement is a code signing requirement for code with identifier,
// signed by team with a Developer ID Application certificate: the form
// codesign gives the app as its designated requirement. Without the two
// markers (the Developer ID intermediate, and the Developer ID Application
// leaf), any Apple-issued certificate of the team would do, an Apple
// Development one included, which is a weaker secret (a security review,
// 2026-11-05).
func requirement(identifier, team string) string {
	return fmt.Sprintf(`anchor apple generic and identifier %q and certificate 1[field.1.2.840.113635.100.6.2.6] `+
		`and certificate leaf[field.1.2.840.113635.100.6.1.13] and certificate leaf[subject.OU] = %q`, identifier, team)
}

// AppRequirement is what the helper requires of its callers: the app.
func AppRequirement(bundleID, team string) string { return requirement(bundleID, team) }

// HelperRequirement is what the app requires of the helper answering.
func HelperRequirement(bundleID, team string) string { return requirement(Label(bundleID), team) }
