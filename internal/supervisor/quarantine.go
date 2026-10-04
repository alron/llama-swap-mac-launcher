package supervisor

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// ErrQuarantined marks failures that are probably Gatekeeper refusing to
// run a downloaded, un-notarized llama-swap.
var ErrQuarantined = errors.New("blocked by Gatekeeper")

// quarantined reports whether path (following symlinks) carries the
// com.apple.quarantine attribute that browsers add to downloads.
func quarantined(path string) bool {
	_, err := unix.Getxattr(path, "com.apple.quarantine", nil)
	return err == nil
}

// looksBlocked reports whether an exit looks like Gatekeeper refusing to
// run the binary: killed by SIGKILL almost immediately.
func looksBlocked(ps *os.ProcessState, ran time.Duration) bool {
	ws, ok := ps.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL && ran < 3*time.Second
}

func quarantineError(path string, cause error) error {
	return fmt.Errorf("%w: %s was downloaded from the internet and macOS won't run it (%v). "+
		"If you trust it, remove the quarantine with: xattr -d com.apple.quarantine %q",
		ErrQuarantined, path, cause, path)
}
