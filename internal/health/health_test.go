package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSwap imitates the parts of llama-swap's API the monitor uses.
type fakeSwap struct {
	events   chan string // raw SSE chunks; "" ends the current stream
	healthy  atomic.Bool
	checks   atomic.Int32 // /health requests
	checkUA  atomic.Value // the last one's User-Agent
	refusals atomic.Int32 // log stream requests refused with 400

	mu        sync.Mutex
	logs      map[string]chan string
	watchedBy map[string]string // model ID -> query string of its log request
	unloaded  []string
}

func newFakeSwap(t *testing.T) (*fakeSwap, *httptest.Server) {
	f := &fakeSwap{
		events:    make(chan string, 10),
		logs:      make(map[string]chan string),
		watchedBy: make(map[string]string),
	}
	f.healthy.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		f.checks.Add(1)
		f.checkUA.Store(r.UserAgent())
		if !f.healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case chunk := <-f.events:
				if chunk == "" {
					return
				}
				fmt.Fprint(w, chunk)
				w.(http.Flusher).Flush()
			}
		}
	})
	mux.HandleFunc("GET /logs/stream/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if strings.HasPrefix(id, "refused") {
			f.refusals.Add(1)
			http.Error(w, "invalid logger", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.watchedBy[id] = r.URL.RawQuery
		ch := f.logChan(id)
		f.mu.Unlock()
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case line := <-ch:
				fmt.Fprintln(w, line)
				w.(http.Flusher).Flush()
			}
		}
	})
	mux.HandleFunc("POST /api/models/unload/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.unloaded = append(f.unloaded, r.PathValue("id"))
		f.mu.Unlock()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

// logChan needs f.mu held.
func (f *fakeSwap) logChan(id string) chan string {
	if f.logs[id] == nil {
		f.logs[id] = make(chan string, 10)
	}
	return f.logs[id]
}

func (f *fakeSwap) sendLog(id, line string) {
	f.mu.Lock()
	ch := f.logChan(id)
	f.mu.Unlock()
	ch <- line
}

func (f *fakeSwap) watching(id string) (query string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	query, ok = f.watchedBy[id]
	return query, ok
}

// modelStatus builds an event the way llama-swap sends it: the data field
// is JSON encoded as a string.
func modelStatus(models ...map[string]string) string {
	data, _ := json.Marshal(models)
	ev, _ := json.Marshal(map[string]string{"type": "modelStatus", "data": string(data)})
	return "event:message\ndata:" + string(ev) + "\n\n"
}

func model(id, state, peer string) map[string]string {
	return map[string]string{"id": id, "state": state, "peerID": peer}
}

func startMonitor(t *testing.T, url string) *Monitor {
	return startMonitorSkipping(t, url, false)
}

func startMonitorSkipping(t *testing.T, url string, skip bool) *Monitor {
	m := New(url)
	m.HealthEvery = 50 * time.Millisecond
	m.SkipWhenActive = skip
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	return m
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestModelsAndHealth(t *testing.T) {
	f, srv := newFakeSwap(t)
	f.events <- modelStatus(model("a", "ready", ""), model("b", "stopped", ""), model("peer/c", "ready", "peer"))
	m := startMonitor(t, srv.URL)

	waitFor(t, "3 models, healthy", func() bool {
		s := m.Snapshot()
		return s.Healthy && s.WasHealthy && len(s.Models) == 3
	})
	var loaded []string
	for _, mod := range m.Snapshot().Models {
		if mod.Loaded() {
			loaded = append(loaded, mod.ID)
		}
	}
	if !slices.Equal(loaded, []string{"a"}) {
		t.Errorf("loaded = %v, want [a]: stopped and peer models don't count", loaded)
	}
}

func TestFaultDetectedAndCleared(t *testing.T) {
	f, srv := newFakeSwap(t)
	f.events <- modelStatus(model("a", "ready", ""))
	m := startMonitor(t, srv.URL)

	waitFor(t, "a's log to be watched", func() bool { _, ok := f.watching("a"); return ok })
	if q, _ := f.watching("a"); q != "no-history" {
		t.Errorf("log requested with %q; want no-history, so old faults aren't reported", q)
	}
	f.sendLog("a", "0.42.000.001 E ggml_metal: backend is in error state, recreate the backend to recover")
	waitFor(t, "the fault", func() bool { return len(m.Snapshot().Faulted()) == 1 })

	f.events <- modelStatus(model("a", "stopped", ""))
	waitFor(t, "the fault to clear", func() bool { return len(m.Snapshot().Faulted()) == 0 })
}

func TestUnhealthyWhenHealthFails(t *testing.T) {
	f, srv := newFakeSwap(t)
	m := startMonitor(t, srv.URL)
	waitFor(t, "healthy", func() bool { return m.Snapshot().Healthy })

	f.healthy.Store(false)
	waitFor(t, "unhealthy", func() bool { return !m.Snapshot().Healthy })
	if !m.Snapshot().WasHealthy {
		t.Error("WasHealthy should stay true, to tell 'not responding' from 'starting'")
	}
}

func TestChecksIdentifyThemselves(t *testing.T) {
	f, srv := newFakeSwap(t)
	m := startMonitor(t, srv.URL)
	waitFor(t, "healthy", func() bool { return m.Snapshot().Healthy })
	if ua := f.checkUA.Load(); ua != CheckUserAgent {
		t.Errorf("health check User-Agent %q, want %q so the app can filter its log lines", ua, CheckUserAgent)
	}
}

func TestSkipsChecksWhileActive(t *testing.T) {
	f, srv := newFakeSwap(t)
	m := startMonitorSkipping(t, srv.URL, true)
	waitFor(t, "healthy", func() bool { return m.Snapshot().Healthy })

	// Events every 20ms, against checks due every 50ms: all but possibly
	// the first are skipped.
	before := f.checks.Load()
	for range 15 {
		f.events <- modelStatus(model("a", "ready", ""))
		time.Sleep(20 * time.Millisecond)
	}
	if n := f.checks.Load() - before; n > 1 {
		t.Errorf("%d checks while events were arriving, want at most 1", n)
	}

	// Once events stop, the checks resume.
	after := f.checks.Load()
	waitFor(t, "checks to resume", func() bool { return f.checks.Load() >= after+2 })
}

// logRecorder collects what the monitor logs.
type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (l *logRecorder) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logRecorder) contains(s string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		if strings.Contains(line, s) {
			return true
		}
	}
	return false
}

func TestRefusedLogStreamIsReportedAndBackedOff(t *testing.T) {
	f, srv := newFakeSwap(t)
	f.events <- modelStatus(model("refused-model", "ready", ""))
	var log logRecorder
	m := New(srv.URL)
	m.HealthEvery = 50 * time.Millisecond
	m.Logf = log.logf
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	waitFor(t, "the refusal to be logged", func() bool { return log.contains("HTTP 400") })
	// Retries after 1s, then 2s, 4s...: in 2.5s, at most 3 requests.
	time.Sleep(2500 * time.Millisecond)
	if n := f.refusals.Load(); n > 3 {
		t.Errorf("%d requests for a refused log stream in 2.5s; it should back off", n)
	}
}

func TestOverlongEventIsReportedThenRecovered(t *testing.T) {
	old := maxLine
	maxLine = 4 << 10
	t.Cleanup(func() { maxLine = old })

	f, srv := newFakeSwap(t)
	var log logRecorder
	m := New(srv.URL)
	m.HealthEvery = 50 * time.Millisecond
	m.Logf = log.logf
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	f.events <- "data:" + strings.Repeat("x", 8<<10) + "\n\n"
	waitFor(t, "the over-long line to be logged", func() bool { return log.contains("token too long") })
	// The fake's handler for the dropped connection may still take an event
	// off the channel and lose it, so keep sending until one arrives.
	waitFor(t, "events after reconnecting", func() bool {
		select {
		case f.events <- modelStatus(model("a", "ready", "")):
		default:
		}
		time.Sleep(50 * time.Millisecond)
		return len(m.Snapshot().Models) == 1
	})
}

func TestReconnectsAfterStreamEnds(t *testing.T) {
	f, srv := newFakeSwap(t)
	f.events <- modelStatus(model("a", "ready", ""))
	m := startMonitor(t, srv.URL)
	waitFor(t, "first snapshot", func() bool { return len(m.Snapshot().Models) == 1 })

	f.events <- "" // end the stream
	f.events <- modelStatus(model("a", "ready", ""), model("b", "starting", ""))
	waitFor(t, "snapshot after reconnecting", func() bool { return len(m.Snapshot().Models) == 2 })
}

func TestUnload(t *testing.T) {
	f, srv := newFakeSwap(t)
	m := New(srv.URL)
	if err := m.Unload(context.Background(), "gemma4-e2b-q8"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.unloaded, []string{"gemma4-e2b-q8"}) {
		t.Errorf("unloaded %v", f.unloaded)
	}
	if err := m.UnloadAll(context.Background()); err == nil {
		t.Error("no error for a 404 from unload-all")
	}
}

// A refused event stream is reported, logged once however often the
// monitor retries, and cleared once the stream connects.
func TestRefusedStream(t *testing.T) {
	for _, tt := range []struct {
		status int
		key    string
		want   string
	}{
		{http.StatusUnauthorized, "", "wants an API key"},
		{http.StatusUnauthorized, "sk-wrong", "refused the app's API key"},
		{http.StatusNotFound, "", "older than v252"},
		{http.StatusInternalServerError, "", "HTTP 500"},
	} {
		var refuse atomic.Bool
		refuse.Store(true)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/events" && refuse.Load() {
				http.Error(w, "no", tt.status)
				return
			}
			if r.URL.Path == "/api/events" {
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}
		}))
		var logged atomic.Int32
		m := New(srv.URL)
		m.APIKey = tt.key
		m.Logf = func(string, ...any) { logged.Add(1) }
		ctx, cancel := context.WithCancel(context.Background())
		go m.Run(ctx)
		waitFor(t, "the problem", func() bool { return strings.Contains(m.Snapshot().Problem, tt.want) })
		time.Sleep(time.Second) // a few retries
		if n := logged.Load(); n != 1 {
			t.Errorf("HTTP %d: logged %d times, want once", tt.status, n)
		}
		refuse.Store(false)
		waitFor(t, "the problem to clear", func() bool { return m.Snapshot().Problem == "" })
		cancel()
		srv.Close()
	}
}

// The monitor follows llama-swap's inflight events: a snapshot on connect,
// then upserts and removes.
func TestInFlight(t *testing.T) {
	f, srv := newFakeSwap(t)
	m := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	inflight := func(data string) string {
		b, _ := json.Marshal(map[string]string{"type": "inflight", "data": data})
		return "event:message\ndata:" + string(b) + "\n\n"
	}
	ids := func(rs []Request) string {
		var s []string
		for _, r := range rs {
			s = append(s, r.ID+":"+r.Model)
		}
		return strings.Join(s, ",")
	}
	f.events <- inflight(`{"operation":"snapshot","requests":[{"id":"1","model":"a","req_path":"/v1/chat/completions","timestamp":"2026-10-04T13:47:08-07:00"}]}`)
	waitFor(t, "the snapshot", func() bool { return ids(m.Snapshot().InFlight) == "1:a" })
	f.events <- inflight(`{"operation":"upsert","request":{"id":"2","model":"b","req_path":"/v1/completions","timestamp":"2026-10-04T13:47:09-07:00"}}`)
	waitFor(t, "the upsert", func() bool { return ids(m.Snapshot().InFlight) == "1:a,2:b" })
	if got := ids(m.Snapshot().Busy("b")); got != "2:b" {
		t.Errorf("Busy(b) = %q", got)
	}
	f.events <- inflight(`{"operation":"remove","id":"1"}`)
	waitFor(t, "the remove", func() bool { return ids(m.Snapshot().InFlight) == "2:b" })
	// A dropped stream forgets them: they can't be known until the next
	// snapshot.
	f.events <- ""
	waitFor(t, "the reconnect to forget", func() bool { return len(m.Snapshot().InFlight) == 0 })
}

// An activity event names a finished request; the monitor looks up its
// speed in llama-swap's request log, and forgets it when the model stops.
func TestTokensPerSecond(t *testing.T) {
	events := make(chan string, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/events":
			w.(http.Flusher).Flush()
			for {
				select {
				case <-r.Context().Done():
					return
				case e := <-events:
					fmt.Fprint(w, e)
					w.(http.Flusher).Flush()
				}
			}
		case "/api/metrics/activity":
			fmt.Fprint(w, `{"data":[
				{"id":8,"model":"m","tokens":{"output_tokens":0,"tokens_per_second":0}},
				{"id":7,"model":"m","tokens":{"output_tokens":120,"tokens_per_second":118.66}}]}`)
		case "/health":
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	event := func(typ, data string) string {
		b, _ := json.Marshal(map[string]string{"type": typ, "data": data})
		return "data:" + string(b) + "\n\n"
	}
	m := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	speed := func() float64 {
		for _, mod := range m.Snapshot().Models {
			if mod.ID == "m" {
				return mod.TokensPerSecond
			}
		}
		return -1
	}
	events <- event("modelStatus", `[{"id":"m","state":"ready"}]`)
	waitFor(t, "the model", func() bool { return speed() == 0 })
	events <- event("activity", `{"id":7}`)
	waitFor(t, "the speed", func() bool { return speed() == 118.66 })
	events <- event("activity", `{"id":8}`) // generated nothing: no change
	time.Sleep(200 * time.Millisecond)
	if got := speed(); got != 118.66 {
		t.Errorf("after a request that generated nothing: %v", got)
	}
	events <- event("modelStatus", `[{"id":"m","state":"stopped"}]`)
	waitFor(t, "the speed to be forgotten", func() bool { return speed() == 0 })
}

// Only the app's own health checks are left out of its log: a client that
// sends the app's User-Agent with any other request is kept.
func TestIsCheckLine(t *testing.T) {
	for line, want := range map[string]bool{
		`[INFO] Request 127.0.0.1 "GET /health HTTP/1.1" 200 2 "llama-swap-launcher-healthcheck" 36µs`:                       true,
		`[INFO] Request 127.0.0.1 "GET /upstream/studio/gemma4/health HTTP/1.1" 200 2 "llama-swap-launcher-healthcheck" 1ms`: true,
		`[INFO] Request 127.0.0.1 "POST /api/models/unload HTTP/1.1" 200 13 "llama-swap-launcher-healthcheck" 36µs`:          false,
		`[INFO] Request 10.0.0.9 "GET /v1/models HTTP/1.1" 200 900 "llama-swap-launcher-healthcheck" 1ms`:                    false,
		`[INFO] Request 127.0.0.1 "GET /health HTTP/1.1" 200 2 "curl/8.7.1" 20µs`:                                            false,
		`[INFO] Request 127.0.0.1 "GET /healthz?x=llama-swap-launcher-healthcheck HTTP/1.1" 404 9 "x" 2µs`:                   false,
	} {
		if got := IsCheckLine([]byte(line)); got != want {
			t.Errorf("IsCheckLine(%q) = %v, want %v", line, got, want)
		}
	}
}

// The app's client never takes a proxy from the environment: its requests
// carry the API key.
func TestNewClientIgnoresProxies(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.invalid:3128")
	tr, ok := NewClient().Transport.(*http.Transport)
	if !ok || tr.Proxy != nil {
		t.Fatalf("the client's transport has a proxy function")
	}
	if m := New("http://192.168.1.20:8080"); m.client.Transport.(*http.Transport).Proxy != nil {
		t.Error("the monitor's client has a proxy function")
	}
}

// Once connected, the monitor asks llama-swap without a key for something
// that needs one, and records whether it answered.
func TestProbeOpen(t *testing.T) {
	for _, keyed := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if keyed && r.Header.Get("Authorization") != "Bearer sk-k" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if r.URL.Path == "/api/events" {
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}
		}))
		m := New(srv.URL)
		m.APIKey = "sk-k"
		ctx, cancel := context.WithCancel(context.Background())
		go m.Run(ctx)
		waitFor(t, "the probe", func() bool { return m.Snapshot().Healthy })
		time.Sleep(300 * time.Millisecond)
		if got := m.Snapshot().Open; got == keyed {
			t.Errorf("llama-swap asking for keys %v: Open = %v", keyed, got)
		}
		cancel()
		srv.Close()
	}
}
