package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeValidator writes a stand-in llama-swap whose -validate prints msg and
// exits with code.
func fakeValidator(t *testing.T, msg string, code string) Spec {
	bin := filepath.Join(t.TempDir(), "llama-swap")
	script := "#!/bin/sh\necho '" + msg + "' >&2\nexit " + code + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return Spec{Binary: bin, Args: []string{"-config", "x.yaml", "-listen", "127.0.0.1:8080"}}
}

func TestValidate(t *testing.T) {
	if err := Validate(fakeValidator(t, "config is valid: 1 model(s)", "0")); err != nil {
		t.Errorf("valid config: %v", err)
	}

	err := Validate(fakeValidator(t, "config validation failed: model x: bad", "1"))
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "model x: bad") {
		t.Errorf("invalid config: got %v", err)
	}

	if err := Validate(fakeValidator(t, "flag provided but not defined: -validate", "2")); err != nil {
		t.Errorf("llama-swap without -validate: got %v, want nil", err)
	}

	if err := Validate(Spec{Binary: "/nonexistent/llama-swap"}); err != nil {
		t.Errorf("binary that won't run: got %v, want nil (Start reports it)", err)
	}
}

// A -validate that hangs is reported as a timeout, not as an invalid
// config (its killed exit would otherwise look like one).
func TestValidateTimeout(t *testing.T) {
	defer func(d time.Duration) { validateTimeout = d }(validateTimeout)
	validateTimeout = 200 * time.Millisecond
	bin := filepath.Join(t.TempDir(), "llama-swap")
	os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 10\n"), 0o755)
	err := Validate(Spec{Binary: bin})
	if err == nil || errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "didn't finish") {
		t.Errorf("got %v, want a timeout that isn't ErrInvalidConfig", err)
	}
}
