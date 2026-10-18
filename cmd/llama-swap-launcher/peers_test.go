package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

func TestEscapeModel(t *testing.T) {
	for id, want := range map[string]string{
		"gemma4-e2b-q4":     "gemma4-e2b-q4",
		"studio/gemma4":     "studio/gemma4",
		"odd?name#x":        "odd%3Fname%23x",
		"peer/with space/m": "peer/with%20space/m",
	} {
		if got := escapeModel(id); got != want {
			t.Errorf("escapeModel(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestLlamaSwapError(t *testing.T) {
	for body, want := range map[string]string{
		`{"src":"llama-swap","error":{"message":"peer proxy error: dial tcp 127.0.0.1:1: connect: connection refused","code":"bad_gateway"}}`: "peer proxy error: dial tcp 127.0.0.1:1: connect: connection refused",
		"plain text\n": "plain text",
	} {
		if got := llamaSwapError([]byte(body)); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

// With LLAMA_SWAP_BIN set: a real llama-swap with three peers, one that
// answers, one that refuses connections, and one whose host never answers.
func TestCheckPeerReal(t *testing.T) {
	bin := os.Getenv("LLAMA_SWAP_BIN")
	if bin == "" {
		t.Skip("set LLAMA_SWAP_BIN to run against a real llama-swap")
	}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "OK") }))
	defer peer.Close()
	config := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(config, fmt.Appendf(nil, `models:
  local:
    cmd: /bin/sleep 600 ${PORT}
peers:
  good:
    proxy: %s
    models: [m]
  refused:
    proxy: http://127.0.0.1:1
    models: [m]
  silent:
    proxy: http://10.255.255.1:8080
    models: [m]
`, peer.URL), 0o600)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	swap := exec.Command(bin, "-config", config, "-listen", addr)
	if err := swap.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		swap.Process.Signal(syscall.SIGTERM)
		swap.Wait()
	})
	st := supervisor.Status{Listen: addr}
	deadline := time.Now().Add(5 * time.Second)
	for checkPeer(context.Background(), st, "good/m") != nil && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	for model, want := range map[string]string{
		"good/m":    "",
		"refused/m": "connection refused",
		"silent/m":  "didn't answer within",
	} {
		start := time.Now()
		err := checkPeer(context.Background(), st, model)
		took := time.Since(start)
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v", model, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: got %v, want %q", model, err, want)
		case took > peerTimeout+time.Second:
			t.Errorf("%s took %s", model, took)
		}
	}
}

func TestFetchPeerModels(t *testing.T) {
	var gotAuth, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotKey = r.Header.Get("Authorization"), r.Header.Get("x-api-key")
		switch {
		case r.URL.Path != "/api/v1/models":
			http.NotFound(w, r)
		case gotKey != "sk-peer":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			// As another llama-swap answers: its own peers' models are marked.
			w.Write([]byte(`{"object":"list","data":[
				{"id":"zeta-7b","meta":{"llamaswap":{"type":"model"}}},
				{"id":"farther/far-1","meta":{"llamaswap":{"peerID":"farther","type":"peer"}}},
				{"id":"Alpha-31b"},
				{"id":"org/beta"},
				{"id":"zeta-7b"}]}`))
		}
	}))
	defer srv.Close()

	ids, err := fetchPeerModels(context.Background(), srv.URL+"/api/", "sk-peer")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Alpha-31b", "org/beta", "zeta-7b"}; !slices.Equal(ids, want) {
		t.Errorf("got %q, want %q", ids, want)
	}
	if gotAuth != "Bearer sk-peer" {
		t.Errorf("Authorization: %q", gotAuth)
	}

	if _, err := fetchPeerModels(context.Background(), srv.URL+"/api", ""); err == nil || !strings.Contains(err.Error(), "wants an API key") {
		t.Errorf("without a key: got %v", err)
	}
	if _, err := fetchPeerModels(context.Background(), srv.URL+"/api", "wrong"); err == nil || !strings.Contains(err.Error(), "didn't accept the key") {
		t.Errorf("with the wrong key: got %v", err)
	}
	if _, err := fetchPeerModels(context.Background(), srv.URL+"/elsewhere", ""); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("not there: got %v", err)
	}
}
