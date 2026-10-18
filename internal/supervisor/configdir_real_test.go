package supervisor

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigDirReal checks the managed config folder against an actual
// llama-swap, skipped unless LLAMA_SWAP_BIN names the binary:
//
//	LLAMA_SWAP_BIN=/opt/homebrew/bin/llama-swap go test ./internal/supervisor -run ConfigDirReal -v
//
// llama-swap must skip a file that's only the note, validation in the
// folder's copy must catch an unset variable and name the real folder, and
// a llama-swap started with the folder must require the key it lists.
func TestConfigDirReal(t *testing.T) {
	bin := os.Getenv("LLAMA_SWAP_BIN")
	if bin == "" {
		t.Skip("set LLAMA_SWAP_BIN to run against a real llama-swap")
	}
	base := t.TempDir()
	config := filepath.Join(base, "config.yaml")
	os.WriteFile(config, []byte("models:\n  m:\n    cmd: /bin/sleep 600 ${PORT}\n"), 0o600)
	dir := filepath.Join(base, "llama-swap.d")
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	note := "# Written by the app.\n"
	spec := func(file string, env ...string) Spec {
		return Spec{
			Binary:      bin,
			Args:        []string{"-config", config, "-listen", addr, "-config-dir", dir},
			Env:         append(os.Environ(), env...),
			Dir:         base,
			Listen:      addr,
			ConfigDir:   dir,
			ConfigFiles: map[string][]byte{"api-keys.yaml": []byte(note + file)},
		}
	}

	if err := Validate(spec("")); err != nil {
		t.Errorf("a file that's only the note: %v", err)
	}
	err := Validate(spec("apiKeys:\n  - \"${env.LLSL_UNSET}\"\n"))
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), filepath.Join(dir, "api-keys.yaml")) ||
		!strings.Contains(err.Error(), "LLSL_UNSET") {
		t.Errorf("an unset variable: got %v, want it reported in %s", err, dir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("validating touched the real folder: %v", err)
	}

	withKey := spec("apiKeys:\n  - \"${env.LLSL_TEST}\"\n", "LLSL_TEST=sk-test")
	if err := Validate(withKey); err != nil {
		t.Fatalf("a key that's set: %v", err)
	}
	s, _ := newTestSupervisor(t, Policy{})
	if err := s.Start(withKey); err != nil {
		t.Fatal(err)
	}
	get := func(key string) int {
		req, _ := http.NewRequest("GET", "http://"+addr+"/api/version", nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	waitFor(t, "llama-swap to answer", func() bool { return get("sk-test") == 200 })
	if code := get(""); code != 401 {
		t.Errorf("without the key: got %d, want 401", code)
	}
	if info, err := os.Stat(filepath.Join(dir, "api-keys.yaml")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the managed file: %v, %v", info, err)
	}
}
