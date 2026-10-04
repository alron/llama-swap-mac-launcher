package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// validateTimeout bounds llama-swap -validate, which only parses a file.
var validateTimeout = 30 * time.Second

// ErrInvalidConfig marks a config that llama-swap's -validate rejected.
var ErrInvalidConfig = errors.New("the llama-swap config isn't valid")

// Validate checks spec's config with llama-swap's -validate flag, which
// parses it and exits without starting anything. Checking before a restart
// means a broken config doesn't take down the llama-swap that's working.
//
// It returns nil if the config is valid, and also when it can't tell: an
// older llama-swap without -validate, or a binary that won't run at all
// (starting it then reports the real problem).
func Validate(spec Spec) error {
	ctx, cancel := context.WithTimeout(context.Background(), validateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, spec.Binary, append(slices.Clone(spec.Args), "-validate")...) // #nosec G204 -- the user's chosen llama-swap, run with -validate
	cmd.Env = spec.Env
	cmd.Dir = spec.Dir
	out, err := cmd.CombinedOutput()
	// Killed at the deadline, it looks like an invalid config's exit; say
	// what actually happened.
	if ctx.Err() != nil {
		return fmt.Errorf("llama-swap -validate didn't finish within %s, so the config couldn't be checked", validateTimeout)
	}
	var exit *exec.ExitError
	if err == nil || !errors.As(err, &exit) {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(msg, "flag provided but not defined: -validate") {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidConfig, msg)
}
