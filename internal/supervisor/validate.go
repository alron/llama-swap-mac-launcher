package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	// The app's own files in the config folder are checked in a copy of it,
	// so the real folder only ever gets files that passed; its messages
	// name the real folder.
	shown := func(s string) string { return s }
	if len(spec.ConfigFiles) > 0 {
		tmp, err := stageConfigDir(spec)
		if err != nil {
			return fmt.Errorf("can't check the config folder: %w", err)
		}
		defer os.RemoveAll(tmp)
		spec.Args = withConfigDir(spec.Args, tmp)
		shown = func(s string) string { return strings.ReplaceAll(s, tmp, spec.ConfigDir) }
	}
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
	msg := shown(strings.TrimSpace(string(out)))
	if strings.Contains(msg, "flag provided but not defined: -validate") {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidConfig, msg)
}
