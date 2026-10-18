package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// TestManagedPeersReal runs an actual llama-swap with the peers the app
// writes, skipped unless LLAMA_SWAP_BIN names the binary:
//
//	LLAMA_SWAP_BIN=/opt/homebrew/bin/llama-swap go test ./cmd/llama-swap-launcher -run ManagedPeersReal -v
//
// llama-swap must read peers.yaml, offer the peer's models as name/model,
// send the peer the key from the secret variable, and refuse a peer that
// the user's own config defines too.
func TestManagedPeersReal(t *testing.T) {
	bin := os.Getenv("LLAMA_SWAP_BIN")
	if bin == "" {
		t.Skip("set LLAMA_SWAP_BIN to run against a real llama-swap")
	}
	var mu sync.Mutex
	var got []string // the keys the stand-in peer was sent
	// A peer that takes 3 s to start answering, against a 1 s timeout.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.Write([]byte(`{}`))
	}))
	defer slow.Close()
	one := 1
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Get("Authorization")+" | "+r.Header.Get("x-api-key"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`))
	}))
	defer peer.Close()

	dir := t.TempDir()
	config := filepath.Join(dir, "config.yaml")
	os.WriteFile(config, []byte("models:\n  local:\n    cmd: /bin/sleep 600 ${PORT}\n"), 0o600)
	managed := filepath.Join(dir, "llama-swap.d")
	os.Mkdir(managed, 0o700)
	p := prefs.Prefs{
		ConfigDirMode: prefs.ConfigDirManaged,
		SecretEnv:     []string{"STUDIO_KEY"},
		Peers: []prefs.Peer{
			{Name: "studio", URL: peer.URL, KeyVariable: "STUDIO_KEY", Models: []string{"big-model", "org/other"}},
			{Name: "slow", URL: slow.URL, Models: []string{"m"}, Timeouts: prefs.PeerTimeouts{ResponseHeader: &one}},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, data := range managedFiles(p) {
		os.WriteFile(filepath.Join(managed, name), data, 0o600)
	}
	env := append(os.Environ(), "STUDIO_KEY=sk-studio")

	validate := func(cfg string) (string, error) {
		cmd := exec.Command(bin, "-config", cfg, "-config-dir", managed, "-validate")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := validate(config); err != nil {
		t.Fatalf("the managed peers don't validate: %v\n%s", err, out)
	}
	// The same peer in the user's config as well: llama-swap's own error.
	twice := filepath.Join(dir, "twice.yaml")
	os.WriteFile(twice, []byte("peers:\n  studio:\n    proxy: http://h\n    models: [x]\n"), 0o600)
	if out, err := validate(twice); err == nil || !strings.Contains(out, `duplicate peers "studio"`) {
		t.Errorf("a peer in both: got %v\n%s", err, out)
	}

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	swap := exec.Command(bin, "-config", config, "-config-dir", managed, "-listen", addr)
	swap.Env = env
	if err := swap.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		swap.Process.Signal(syscall.SIGTERM)
		swap.Wait()
	})
	base := "http://" + addr
	var ids []string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		resp, err := http.Get(base + "/v1/models")
		if err != nil {
			continue
		}
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&list)
		resp.Body.Close()
		ids = ids[:0]
		for _, m := range list.Data {
			ids = append(ids, m.ID)
		}
		break
	}
	for _, want := range []string{"studio/big-model", "studio/org/other", "local"} {
		if !slices.Contains(ids, want) {
			t.Errorf("/v1/models lacks %s: %v", want, ids)
		}
	}
	resp, err := http.Post(base+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"studio/big-model","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	start := time.Now()
	resp, err = http.Post(base+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"slow/m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if took := time.Since(start); resp.StatusCode == http.StatusOK || took > 2500*time.Millisecond {
		t.Errorf("the slow peer: HTTP %d after %s, want a failure after about 1 s", resp.StatusCode, took)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(got, "Bearer sk-studio | sk-studio") {
		t.Errorf("the peer was sent %q, want the secret variable's key", got)
	}
}

// TestManagedCORSReal runs an actual llama-swap with the security.yaml the
// app writes (skipped unless LLAMA_SWAP_BIN is set, as above): listed pages
// are allowed, together with those in the user's own config, others
// aren't, and the switches and cache time apply. A switch the user's
// config sets differently is llama-swap's own error.
func TestManagedCORSReal(t *testing.T) {
	bin := os.Getenv("LLAMA_SWAP_BIN")
	if bin == "" {
		t.Skip("set LLAMA_SWAP_BIN to run against a real llama-swap")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yaml")
	os.WriteFile(config, []byte("security:\n  cors:\n    allowedOrigins: [\"http://mine.example\"]\n"+
		"models:\n  m:\n    cmd: /bin/sleep 600 ${PORT}\n"), 0o600)
	managed := filepath.Join(dir, "llama-swap.d")
	os.Mkdir(managed, 0o700)
	age := 600
	p := prefs.Prefs{ConfigDirMode: prefs.ConfigDirManaged, CORS: prefs.CORS{
		Restrict: true, AllowedOrigins: []string{"http://localhost:3000"}, AllowPrivateNetwork: true, MaxAge: &age,
	}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, data := range managedFiles(p) {
		os.WriteFile(filepath.Join(managed, name), data, 0o600)
	}

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	swap := exec.Command(bin, "-config", config, "-config-dir", managed, "-listen", addr)
	if err := swap.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		swap.Process.Signal(syscall.SIGTERM)
		swap.Wait()
	})
	preflight := func(origin string) http.Header {
		req, _ := http.NewRequest(http.MethodOptions, "http://"+addr+"/v1/chat/completions", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Private-Network", "true")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil
		}
		resp.Body.Close()
		return resp.Header
	}
	for deadline := time.Now().Add(10 * time.Second); preflight("http://localhost:3000") == nil && time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)
	}
	h := preflight("http://localhost:3000")
	if h.Get("Access-Control-Allow-Origin") != "http://localhost:3000" || h.Get("Access-Control-Max-Age") != "600" ||
		h.Get("Access-Control-Allow-Private-Network") != "true" {
		t.Errorf("a listed page: %v", h)
	}
	if h := preflight("http://mine.example"); h.Get("Access-Control-Allow-Origin") != "http://mine.example" {
		t.Errorf("the user's own page: %v", h)
	}
	if h := preflight("http://elsewhere.example"); h.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("a page not listed: %v", h)
	}

	clash := filepath.Join(dir, "clash.yaml")
	os.WriteFile(clash, []byte("security:\n  cors:\n    allowedOrigins: [\"http://mine.example\"]\n    allowPrivateNetwork: false\n"), 0o600)
	out, err := exec.Command(bin, "-config", clash, "-config-dir", managed, "-validate").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "security.yaml") {
		t.Errorf("a switch set differently in the user's config: %v\n%s", err, out)
	}
}
