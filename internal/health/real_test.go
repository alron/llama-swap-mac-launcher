package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// TestAgainstRealLlamaSwap runs the monitor against an actual llama-swap,
// to catch API changes in llama-swap upgrades. It's skipped unless
// LLAMA_SWAP_BIN names the binary:
//
//	LLAMA_SWAP_BIN=/opt/homebrew/bin/llama-swap go test ./internal/health -run Real -v
//
// Its one model is a stand-in: a shell script that serves /health with
// Python's http.server and, a few seconds after starting, prints the GPU
// fault marker the way a failing llama-server would.
func TestAgainstRealLlamaSwap(t *testing.T) {
	bin := os.Getenv("LLAMA_SWAP_BIN")
	if bin == "" {
		t.Skip("set LLAMA_SWAP_BIN to run against a real llama-swap")
	}

	dir := t.TempDir()
	www := filepath.Join(dir, "www")
	os.Mkdir(www, 0o700)
	os.WriteFile(filepath.Join(www, "health"), []byte("OK\n"), 0o600)
	script := filepath.Join(dir, "faulty.sh")
	os.WriteFile(script, []byte(`cd "$(dirname "$0")/www"
(sleep 4; echo "E ggml_metal: `+FaultMarker+`, recreate the backend to recover") &
exec /usr/bin/python3 -m http.server "$1" --bind 127.0.0.1
`), 0o700)
	config := filepath.Join(dir, "config.yaml")
	os.WriteFile(config, fmt.Appendf(nil, "models:\n  faulty:\n    cmd: /bin/sh %s ${PORT}\n", script), 0o600)

	// A pass against a newer llama-swap means TestedVersion can move up.
	if v := supervisor.BinaryVersion(bin); supervisor.VersionNumber(v) > TestedVersion {
		t.Cleanup(func() {
			if !t.Failed() {
				t.Logf("passed against llama-swap %s: raise TestedVersion (v%d) to match", v, TestedVersion)
			}
		})
	}

	addr := freeAddr(t)
	swap := exec.Command(bin, "-config", config, "-listen", addr)
	if err := swap.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		swap.Process.Signal(syscall.SIGTERM)
		swap.Wait()
	})

	base := "http://" + addr
	m := New(base)
	m.HealthEvery = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	waitFor(t, "llama-swap to be healthy", func() bool { return m.Snapshot().Healthy })

	// Any request for the model makes llama-swap start it.
	resp, err := http.Get(base + "/upstream/faulty/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	waitFor(t, "the model to be ready", func() bool { return modelState(m, "faulty") == "ready" })
	waitFor(t, "the fault to be noticed", func() bool { return len(m.Snapshot().Faulted()) == 1 })

	if err := m.Unload(ctx, "faulty"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the model to stop", func() bool { return modelState(m, "faulty") == "stopped" })
	if f := m.Snapshot().Faulted(); len(f) != 0 {
		t.Errorf("still faulted after unloading: %v", f)
	}
}

func modelState(m *Monitor, id string) string {
	for _, mod := range m.Snapshot().Models {
		if mod.ID == id {
			return mod.State
		}
	}
	return ""
}

func freeAddr(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// With apiKeys in the config, taken from a variable as the app passes
// them, the monitor needs the key: with it, it sees llama-swap and its
// models; without it, it doesn't connect.
func TestRealLlamaSwapWithAPIKey(t *testing.T) {
	bin := os.Getenv("LLAMA_SWAP_BIN")
	if bin == "" {
		t.Skip("set LLAMA_SWAP_BIN to run against a real llama-swap")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yaml")
	os.WriteFile(config, []byte("apiKeys:\n  - \"${env.LLSL_TEST}\"\nmodels:\n  m:\n    cmd: /bin/sleep 600 ${PORT}\n"), 0o600)

	addr := freeAddr(t)
	swap := exec.Command(bin, "-config", config, "-listen", addr)
	swap.Env = append(os.Environ(), "LLSL_TEST=sk-test-key")
	if err := swap.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		swap.Process.Signal(syscall.SIGTERM)
		swap.Wait()
	})

	run := func(key string) *Monitor {
		m := New("http://" + addr)
		m.HealthEvery = 200 * time.Millisecond
		m.APIKey = key
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go m.Run(ctx)
		return m
	}
	with := run("sk-test-key")
	waitFor(t, "the monitor with the key to see llama-swap", func() bool {
		return with.Snapshot().Healthy && modelState(with, "m") == "stopped"
	})
	without := run("")
	time.Sleep(time.Second) // long enough to connect, were it allowed to
	if s := without.Snapshot(); s.Healthy || len(s.Models) > 0 || !strings.Contains(s.Problem, "wants an API key") {
		t.Errorf("without the key: %+v", s)
	}
}
