// Package health watches a running llama-swap through its HTTP API: whether
// it answers, which models it has, and whether a model's GPU backend has
// failed.
//
// Model states come from llama-swap's event stream (/api/events), which
// sends a snapshot on connect and then every change, so nothing is polled
// except a periodic /health check.
package health

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// TestedVersion is the llama-swap release this monitor was last checked
// against, by TestAgainstRealLlamaSwap. The monitor depends on llama-swap's
// API, so the app notes in its log when it runs a newer llama-swap: after
// an upgrade breaks something, that note is the first clue. Bump it
// whenever that test passes against a newer release.
const TestedVersion = 262

// MinimumVersion is the oldest llama-swap release the app is known to work
// with: v252 added -validate, and older ones lack parts of the API too.
// The app notes in its log when it runs an older one.
const MinimumVersion = 252

// FaultMarker is what a llama-server logs when a GPU error has left its
// backend unusable. From then on every completion against that model fails
// with HTTP 500 while /health still answers, until the model's process is
// replaced.
const FaultMarker = "backend is in error state"

type Model struct {
	ID    string
	Name  string
	State string // llama-swap's: "stopped", "starting", "ready", "stopping", ...
	// PeerID is set for models served by another llama-swap.
	PeerID string
	// Faulted means the model's log reported FaultMarker since it became
	// ready.
	Faulted bool
	// TokensPerSecond is how fast the model generated in its most recent
	// request since it loaded, or 0 if it hasn't generated anything yet. A
	// model that fell back to running on the CPU shows here as a fraction
	// of its usual speed.
	TokensPerSecond float64
}

// Loaded reports whether the model is using (or about to use) this
// machine's resources.
func (m Model) Loaded() bool { return m.PeerID == "" && m.State != "stopped" }

type Snapshot struct {
	// Healthy means the event stream is connected and /health answers.
	Healthy bool
	// WasHealthy means Healthy has been true at least once, which tells
	// "not responding" apart from "still starting".
	WasHealthy bool
	Models     []Model
	// Problem says why llama-swap refused the event stream, such as a
	// missing API key, or "" if it hasn't. The app can't follow llama-swap
	// until it's fixed.
	Problem string
	// InFlight are the requests llama-swap is serving, oldest first. Only
	// model requests (/v1/…) count, not the app's own.
	InFlight []Request
}

// Request is a request llama-swap is serving.
type Request struct {
	ID      string
	Model   string
	Path    string
	Started time.Time
}

// Busy returns the requests in flight for model, or for any model if model
// is "".
func (s Snapshot) Busy(model string) []Request {
	var out []Request
	for _, r := range s.InFlight {
		if model == "" || r.Model == model {
			out = append(out, r)
		}
	}
	return out
}

// Faulted returns the models whose backend has failed.
func (s Snapshot) Faulted() []Model {
	var out []Model
	for _, m := range s.Models {
		if m.Faulted {
			out = append(out, m)
		}
	}
	return out
}

// maxLine is the longest line the monitor reads from llama-swap's streams.
// Events carry whole model lists and request headers; a longer line stops
// that connection, which is then reopened. A variable so tests can lower it.
var maxLine = 8 << 20

// CheckUserAgent is the User-Agent of the monitor's /health checks, so
// their lines can be kept out of the app's copy of llama-swap's log.
const CheckUserAgent = "llama-swap-launcher-healthcheck"

// checkRequest matches the request in llama-swap's access-log line for one
// of the app's own checks: its /health, or a peer's through /upstream.
var checkRequest = regexp.MustCompile(`"GET /(?:upstream/[^ "]+/)?health HTTP/`)

// IsCheckLine reports whether line is llama-swap's access-log line for one
// of the app's own checks, which the app keeps out of its copy of the log:
// a GET of a /health, sent with CheckUserAgent, which llama-swap quotes
// after the status:
//
//	[INFO] Request 127.0.0.1 "GET /health HTTP/1.1" 200 2 "llama-swap-launcher-healthcheck" 36µs
//
// The User-Agent alone isn't enough: any client can send it, and would
// then be left out of the log whatever it asked for.
func IsCheckLine(line []byte) bool {
	return bytes.Contains(line, []byte(`"`+CheckUserAgent+`"`)) && checkRequest.Match(line)
}

type Monitor struct {
	base    string
	client  *http.Client
	changed chan struct{}

	// HealthEvery is how often /health is checked. Set before Run.
	HealthEvery time.Duration
	// SkipWhenActive skips a check when events have arrived since the last
	// one: llama-swap sending them shows it's alive. Set before Run.
	SkipWhenActive bool
	// Logf, if set, receives problems worth recording, such as a stream
	// that failed. Set before Run.
	Logf func(format string, args ...any)
	// APIKey, if set, goes with every request, for a llama-swap whose
	// config sets apiKeys. Set before Run.
	APIKey string

	mu        sync.Mutex
	problem   string // see Snapshot.Problem
	refusal   int    // the last refusal's HTTP status, so each is logged once
	inflight  map[string]Request
	speed     map[string]float64 // tokens per second, by model; see Model.TokensPerSecond
	connected bool
	healthOK  bool
	was       bool
	lastEvent time.Time
	models    []Model
	faults    map[string]bool
	watchers  map[string]context.CancelFunc
}

// NewClient returns an HTTP client for the app's requests to llama-swap.
// It ignores proxy settings (HTTP_PROXY and the like), which the app can
// inherit from the shell that opened it: the requests carry the API key,
// so they go straight to llama-swap, never through a proxy. (Go already
// skips the proxy for loopback, but listen can be a LAN address.)
func NewClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return &http.Client{Transport: t}
}

// New returns a monitor for the llama-swap at base, such as
// "http://127.0.0.1:8080".
func New(base string) *Monitor {
	return &Monitor{
		base:        strings.TrimSuffix(base, "/"),
		client:      NewClient(),
		changed:     make(chan struct{}, 1),
		HealthEvery: time.Minute,
		faults:      make(map[string]bool),
		watchers:    make(map[string]context.CancelFunc),
	}
}

// Changed receives a value whenever the snapshot may have changed. Values
// coalesce, so call Snapshot after each one.
func (m *Monitor) Changed() <-chan struct{} { return m.changed }

func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Snapshot{Healthy: m.connected && m.healthOK, WasHealthy: m.was, Problem: m.problem}
	for _, r := range m.inflight {
		s.InFlight = append(s.InFlight, r)
	}
	slices.SortFunc(s.InFlight, func(a, b Request) int { return a.Started.Compare(b.Started) })
	for _, mod := range m.models {
		mod.Faulted = m.faults[mod.ID]
		mod.TokensPerSecond = m.speed[mod.ID]
		s.Models = append(s.Models, mod)
	}
	return s
}

// Run watches llama-swap until ctx is cancelled, reconnecting whenever the
// event stream drops.
func (m *Monitor) Run(ctx context.Context) {
	go m.pollHealth(ctx)
	backoff := 250 * time.Millisecond
	for ctx.Err() == nil {
		if m.streamEvents(ctx) {
			backoff = 250 * time.Millisecond
		}
		m.update(func() { m.connected, m.inflight = false, nil }) // in flight: unknown until the next snapshot
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}
	m.update(func() {
		for id, cancel := range m.watchers {
			cancel()
			delete(m.watchers, id)
		}
	})
}

// streamEvents reads /api/events until it ends. It reports whether it
// connected at all.
func (m *Monitor) streamEvents(ctx context.Context) bool {
	req, err := m.request(ctx, http.MethodGet, "/api/events")
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		m.refused(resp)
		return false
	}
	m.update(func() { m.connected, m.problem, m.refusal = true, "", 0 })

	sc := newScanner(resp.Body)
	for sc.Scan() {
		if data, ok := strings.CutPrefix(sc.Text(), "data:"); ok {
			m.mu.Lock()
			m.lastEvent = time.Now()
			m.mu.Unlock()
			m.handleEvent(ctx, data)
		}
	}
	// Scan stops both at the end of the stream and on an error, such as a
	// line longer than maxLine. Run reconnects either way; only an error is
	// worth reporting.
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		m.logf("llama-swap's event stream failed: %v; reconnecting", err)
	}
	return true
}

// handleEvent handles one event. Its data field is itself JSON, encoded
// as a string.
func (m *Monitor) handleEvent(ctx context.Context, raw string) {
	var ev struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if json.Unmarshal([]byte(raw), &ev) != nil {
		return
	}
	switch ev.Type {
	case "inflight":
		m.handleInFlight(ev.Data)
		return
	case "activity":
		go m.recordSpeed(ctx, ev.Data)
		return
	case "modelStatus":
	default:
		return
	}
	var list []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		State  string `json:"state"`
		PeerID string `json:"peerID"`
	}
	if json.Unmarshal([]byte(ev.Data), &list) != nil {
		return
	}
	models := make([]Model, len(list))
	for i, l := range list {
		models[i] = Model{ID: l.ID, Name: l.Name, State: l.State, PeerID: l.PeerID}
	}
	m.update(func() {
		// A model's speed is its own run's: forget it once it stops, so a
		// reload that's slower (on the CPU, say) can't hide behind it.
		for _, mod := range models {
			if mod.State == "stopped" {
				delete(m.speed, mod.ID)
			}
		}
		m.models = models
		m.syncWatchers(ctx)
	})
}

// recordSpeed looks up a finished request's generation speed. The activity
// event only names the request (its id); llama-swap's request log,
// /api/metrics/activity (newest first), has the token counts.
func (m *Monitor) recordSpeed(ctx context.Context, data string) {
	var ev struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal([]byte(data), &ev) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := m.request(ctx, http.MethodGet, "/api/metrics/activity?limit=10")
	if err != nil {
		return
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var log struct {
		Data []struct {
			ID     int64  `json:"id"`
			Model  string `json:"model"`
			Tokens struct {
				Output          int     `json:"output_tokens"`
				TokensPerSecond float64 `json:"tokens_per_second"`
			} `json:"tokens"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&log) != nil {
		return
	}
	for _, r := range log.Data {
		// Requests that generated nothing (embeddings, failures) say nothing
		// about the model's speed.
		if r.ID == ev.ID && r.Tokens.Output > 0 && r.Tokens.TokensPerSecond > 0 {
			m.update(func() {
				if m.speed == nil {
					m.speed = map[string]float64{}
				}
				m.speed[r.Model] = r.Tokens.TokensPerSecond
			})
			return
		}
	}
}

// handleInFlight applies an inflight event: a snapshot of every request in
// flight (sent on connect), one request starting or progressing (upsert),
// or one finishing (remove).
func (m *Monitor) handleInFlight(data string) {
	type request struct {
		ID        string    `json:"id"`
		Model     string    `json:"model"`
		Path      string    `json:"req_path"`
		Timestamp time.Time `json:"timestamp"`
	}
	var ev struct {
		Operation string    `json:"operation"`
		Requests  []request `json:"requests"` // snapshot
		Request   request   `json:"request"`  // upsert
		ID        string    `json:"id"`       // remove
	}
	if json.Unmarshal([]byte(data), &ev) != nil {
		return
	}
	add := func(r request) {
		m.inflight[r.ID] = Request{ID: r.ID, Model: r.Model, Path: r.Path, Started: r.Timestamp}
	}
	// Not through update: nothing the menu shows depends on these, and a
	// busy llama-swap sends them several times a request.
	m.mu.Lock()
	defer m.mu.Unlock()
	switch ev.Operation {
	case "snapshot":
		m.inflight = map[string]Request{}
		for _, r := range ev.Requests {
			add(r)
		}
	case "upsert":
		if m.inflight == nil {
			m.inflight = map[string]Request{}
		}
		add(ev.Request)
	case "remove":
		delete(m.inflight, ev.ID)
	}
}

// syncWatchers watches the log of every local model that's ready, and
// stops watching (and forgets any fault of) every other. It needs mu held.
func (m *Monitor) syncWatchers(ctx context.Context) {
	ready := make(map[string]bool)
	for _, mod := range m.models {
		if mod.PeerID == "" && mod.State == "ready" {
			ready[mod.ID] = true
		}
	}
	for id, cancel := range m.watchers {
		if !ready[id] {
			cancel()
			delete(m.watchers, id)
			delete(m.faults, id)
		}
	}
	for id := range ready {
		if _, ok := m.watchers[id]; !ok {
			wctx, cancel := context.WithCancel(ctx)
			m.watchers[id] = cancel
			go m.watchModelLog(wctx, id)
		}
	}
}

// watchModelLog follows one model's output for FaultMarker until ctx ends,
// reconnecting when the stream drops. Errors back off, so a stream
// llama-swap refuses isn't requested every second.
func (m *Monitor) watchModelLog(ctx context.Context, id string) {
	backoff := time.Second
	for {
		err := m.followModelLog(ctx, id)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			m.logf("watching %s's log for GPU faults: %v; retrying in %s", id, err, backoff)
			backoff = min(backoff*2, time.Minute)
		} else {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// followModelLog reads one connection to a model's log stream until it
// ends. It asks for no history, so a fault from an earlier run of the model
// isn't reported again.
func (m *Monitor) followModelLog(ctx context.Context, id string) error {
	req, err := m.request(ctx, http.MethodGet, "/logs/stream/"+url.PathEscape(id)+"?no-history")
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	sc := newScanner(resp.Body)
	for sc.Scan() {
		if strings.Contains(sc.Text(), FaultMarker) {
			m.update(func() {
				if m.watchers[id] != nil {
					m.faults[id] = true
				}
			})
		}
	}
	// As in streamEvents: nil at the end of the stream, or the error that
	// stopped Scan. Reconnecting skips past a line longer than maxLine.
	return sc.Err()
}

// newScanner reads lines of up to maxLine bytes. The starting buffer has to
// be smaller than that: Scanner's limit is the larger of the two.
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, min(64<<10, maxLine)), maxLine)
	return sc
}

func (m *Monitor) logf(format string, args ...any) {
	if m.Logf != nil {
		m.Logf(format, args...)
	}
}

func (m *Monitor) pollHealth(ctx context.Context) {
	var last time.Time
	for {
		if !m.canSkipCheck(last) {
			ok := m.checkHealth(ctx)
			m.update(func() { m.healthOK = ok })
		}
		last = time.Now()
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.checkInterval()):
		}
	}
}

// canSkipCheck reports whether the check due now can be skipped: only if
// SkipWhenActive is set, llama-swap was fine last time, and events have
// arrived since the last check at last.
func (m *Monitor) canSkipCheck(last time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.SkipWhenActive && m.connected && m.healthOK && m.lastEvent.After(last)
}

// checkInterval checks often until llama-swap first answers, so "starting"
// turns into "ready" promptly.
func (m *Monitor) checkInterval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.was && !m.healthOK {
		return 250 * time.Millisecond
	}
	return m.HealthEvery
}

func (m *Monitor) checkHealth(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := m.request(ctx, http.MethodGet, "/health")
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", CheckUserAgent)
	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body) // drained so the connection can be reused
	_ = resp.Body.Close()                 // nothing left to read
	return resp.StatusCode == http.StatusOK
}

// Running returns the models llama-swap has running, with the address
// it proxies each to (from /running).
func (m *Monitor) Running(ctx context.Context) (map[string]string, error) {
	req, err := m.request(ctx, http.MethodGet, "/running")
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/running: HTTP %d", resp.StatusCode)
	}
	var r struct {
		Running []struct {
			Model string `json:"model"`
			Proxy string `json:"proxy"`
		} `json:"running"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, x := range r.Running {
		out[x.Model] = x.Proxy
	}
	return out, nil
}

// Unload stops one model. llama-swap starts it again, in a new process, on
// the next request for it.
func (m *Monitor) Unload(ctx context.Context, id string) error {
	return m.post(ctx, "/api/models/unload/"+url.PathEscape(id))
}

// UnloadAll stops every model.
func (m *Monitor) UnloadAll(ctx context.Context) error {
	return m.post(ctx, "/api/models/unload")
}

// refused records why llama-swap refused the event stream, and logs it the
// first time it answers with that status.
func (m *Monitor) refused(resp *http.Response) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	problem := streamProblem(resp.StatusCode, m.APIKey != "")
	m.mu.Lock()
	first := m.refusal != resp.StatusCode
	m.refusal = resp.StatusCode
	m.mu.Unlock()
	if first && m.Logf != nil {
		m.Logf("%s (/api/events: HTTP %d: %s)", problem, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	m.update(func() { m.problem = problem })
}

// streamProblem says why llama-swap refused the event stream with status,
// as the menu and llsl show it.
func streamProblem(status int, sentKey bool) string {
	switch {
	case status == http.StatusUnauthorized && !sentKey:
		return "llama-swap wants an API key (its config sets apiKeys). Add the key in Settings… → Secrets"
	case status == http.StatusUnauthorized:
		return "llama-swap refused the app's API key. Check that the key the app uses is one its config lists"
	case status == http.StatusNotFound:
		return fmt.Sprintf("llama-swap has no event stream (/api/events). It may be older than v%d, the oldest the app works with", MinimumVersion)
	}
	return fmt.Sprintf("llama-swap's event stream answered HTTP %d", status)
}

// request makes a request to llama-swap, with the API key if there is one.
// llama-swap also takes the key as Basic auth's password or x-api-key;
// Bearer is what OpenAI-style clients send.
func (m *Monitor) request(ctx context.Context, method, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, m.base+path, nil)
	if err == nil && m.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.APIKey)
	}
	return req, err
}

func (m *Monitor) post(ctx context.Context, path string) error {
	req, err := m.request(ctx, http.MethodPost, path)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// update applies f under the lock and signals Changed. It also records
// whether llama-swap has been healthy, whichever of the two conditions
// became true last.
func (m *Monitor) update(f func()) {
	m.mu.Lock()
	f()
	if m.connected && m.healthOK {
		m.was = true
	}
	m.mu.Unlock()
	select {
	case m.changed <- struct{}{}:
	default:
	}
}
