package main

// What llsl can ask the app to do, through the control socket.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// codeError attaches a control.Code* to an error.
type codeError struct {
	code string
	err  error
}

func (e codeError) Error() string { return e.err.Error() }
func (e codeError) Unwrap() error { return e.err }

func codeOf(err error) string {
	var ce codeError
	if errors.As(err, &ce) {
		return ce.code
	}
	return control.CodeError
}

// handle answers one control request. Unlike the menu's actions, it
// reports problems in its response instead of showing alerts, since the
// caller is often a script or an agent.
func (a *app) handle(ctx context.Context, req control.Request) control.Response {
	if req.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
		defer cancel()
	}
	var err error
	switch req.Cmd {
	case "status":
	case "start", "restart":
		err = a.controlStart(ctx, req)
	case "stop":
		if err = a.refuseIfBusy(req); err == nil {
			a.sup.Logf("stop requested by llsl")
			a.sup.Stop()
		}
	case "unload":
		err = a.controlUnload(ctx, req)
	case "output":
		// For llsl logs when llama-swap's output isn't saved.
		return control.Response{OK: true, Output: a.recentOutput(min(max(req.Lines, 1), 100))}
	default:
		err = codeError{control.CodeUsage, fmt.Errorf("unknown command %q", req.Cmd)}
	}
	resp := control.Response{OK: err == nil, Status: a.controlStatus()}
	if req.Cmd == "status" {
		// Only for status: it takes a request per peer, up to peerTimeout.
		st := a.sup.Status()
		if snap, ok := a.healthFor(st.Pid); ok && snap.Healthy {
			resp.Status.Peers = a.checkPeers(ctx, st, snap)
		}
	}
	if err != nil {
		resp.Code, resp.Error = codeOf(err), err.Error()
		// llama-swap's own last words, from memory, whichever log (if
		// any) has them. A config that didn't validate never ran, and a
		// refusal changed nothing, so they'd say nothing.
		switch resp.Code {
		case control.CodeInvalidConfig, control.CodeUsage, control.CodeBusy:
		default:
			resp.Output = a.recentOutput(20)
		}
	}
	return resp
}

func (a *app) controlStart(ctx context.Context, req control.Request) error {
	if req.Cmd == "restart" {
		if err := a.refuseIfBusy(req); err != nil {
			return err
		}
	}
	spec, err := a.prepare()
	if err != nil {
		if errors.Is(err, supervisor.ErrInvalidConfig) {
			return codeError{control.CodeInvalidConfig, fmt.Errorf("%w. llama-swap was left as it was", err)}
		}
		return codeError{control.CodeStartFailed, err}
	}
	a.sup.Logf("%s requested by llsl", req.Cmd)
	if req.Cmd == "restart" {
		err = a.sup.Restart(spec)
	} else {
		err = a.sup.Start(spec) // does nothing if it's already running
	}
	if err != nil {
		return codeError{control.CodeStartFailed, err}
	}
	if !req.Wait && req.Load == "" {
		return nil
	}
	if err := a.waitReady(ctx); err != nil {
		return err
	}
	if req.Load != "" {
		return a.loadModel(ctx, req.Load)
	}
	return nil
}

// waitReady waits until llama-swap answers, or has failed.
func (a *app) waitReady(ctx context.Context) error {
	for {
		st := a.sup.Status()
		switch st.State {
		case supervisor.Failed, supervisor.Waiting, supervisor.Stopped:
			msg := st.Message
			if msg == "" {
				msg = "llama-swap is " + st.State.String()
			}
			return codeError{control.CodeStartFailed, errors.New(msg)}
		case supervisor.Running:
			snap, ok := a.healthFor(st.Pid)
			if ok && snap.Healthy {
				return nil
			}
			// A refusal, such as for the API key, won't fix itself.
			if ok && snap.Problem != "" {
				return codeError{control.CodeStartFailed, errors.New(snap.Problem)}
			}
		}
		select {
		case <-ctx.Done():
			return codeError{control.CodeTimeout, errors.New("timed out waiting for llama-swap to be ready")}
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// loadModel makes llama-swap load model and waits until it's ready. Any
// request for a model makes llama-swap start it, and holds the request
// until the model's server answers; its /health is a cheap one. Only a
// model of this Mac can be loaded this way: a peer's /health is answered by
// the peer's llama-swap, so it would report success without loading
// anything.
func (a *app) loadModel(ctx context.Context, model string) error {
	st := a.sup.Status()
	m, ok := a.findModel(ctx, st.Pid, model)
	switch {
	case !ok:
		return codeError{control.CodeUsage, fmt.Errorf("there's no model %q in llama-swap's config", model)}
	case m.PeerID != "":
		return codeError{control.CodeUsage, fmt.Errorf("%s is a model of the peer %s, which loads it itself when a request needs it. "+
			"-load only loads this Mac's models", model, m.PeerID)}
	}
	a.sup.Logf("loading %s, as requested by llsl", model)
	req, err := upstreamRequest(ctx, st, model, "/health")
	if err != nil {
		return codeError{control.CodeUsage, err}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return codeError{control.CodeTimeout, fmt.Errorf("timed out waiting for %s to load", model)}
		}
		return codeError{control.CodeLoadFailed, fmt.Errorf("loading %s: %w", model, err)}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	_ = resp.Body.Close() // already read what's needed
	// llama-swap's own word decides, not the status code: not every kind
	// of upstream has a /health.
	if a.modelReady(ctx, st.Pid, model) {
		return nil
	}
	return codeError{control.CodeLoadFailed,
		fmt.Errorf("%s didn't load: HTTP %d: %s", model, resp.StatusCode, llamaSwapError(body))}
}

// findModel looks model up in llama-swap's list. Right after llama-swap
// starts, the list may not have arrived yet, so it waits briefly for it.
func (a *app) findModel(ctx context.Context, pid int, model string) (health.Model, bool) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		snap, _ := a.healthFor(pid)
		for _, m := range snap.Models {
			if m.ID == model {
				return m, true
			}
		}
		if (len(snap.Models) > 0 && snap.Healthy) || time.Now().After(deadline) || ctx.Err() != nil {
			return health.Model{}, false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// modelReady waits briefly for llama-swap's events to report model ready.
func (a *app) modelReady(ctx context.Context, pid int, model string) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if snap, ok := a.healthFor(pid); ok {
			for _, m := range snap.Models {
				if m.ID == model && m.State == "ready" {
					return true
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// refuseIfBusy refuses stop, restart and unload while llama-swap is
// serving requests they'd cut short, unless req.Force is set: an agent
// restarting llama-swap shouldn't kill another's long completion. When the
// app can't see llama-swap (hung, say), it can't tell, and lets them
// through: stopping a stuck llama-swap must always work. The menu doesn't
// ask; it's the unconditional way.
func (a *app) refuseIfBusy(req control.Request) error {
	st := a.sup.Status()
	snap, ok := a.healthFor(st.Pid)
	if req.Force || st.State != supervisor.Running || !ok || !snap.Healthy {
		return nil
	}
	var busy []health.Request
	switch {
	case req.Cmd == "unload" && !req.All:
		busy = snap.Busy(req.Model)
	case req.Cmd == "unload":
		// Requests for a peer's models don't care about this Mac's.
		peer := map[string]bool{}
		for _, m := range snap.Models {
			peer[m.ID] = m.PeerID != ""
		}
		for _, r := range snap.Busy("") {
			if !peer[r.Model] {
				busy = append(busy, r)
			}
		}
	default:
		busy = snap.Busy("")
	}
	if len(busy) == 0 {
		return nil
	}
	return codeError{control.CodeBusy, fmt.Errorf("llama-swap is serving %s. Nothing was done; use -force to %s anyway",
		describeRequests(busy), req.Cmd)}
}

// describeRequests lists requests in flight, the oldest first: "2 requests
// (gemma4-31b-q8 for 1m12s, qwen3-8b for 4s)".
func describeRequests(rs []health.Request) string {
	var parts []string
	for i, r := range rs {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more", len(rs)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%s for %s", r.Model, time.Since(r.Started).Round(time.Second)))
	}
	noun := "request"
	if len(rs) != 1 {
		noun = "requests"
	}
	return fmt.Sprintf("%d %s (%s)", len(rs), noun, strings.Join(parts, ", "))
}

func (a *app) controlUnload(ctx context.Context, req control.Request) error {
	if err := a.refuseIfBusy(req); err != nil {
		return err
	}
	m := a.monitor()
	if m == nil {
		return codeError{control.CodeError, errors.New("llama-swap isn't running")}
	}
	var err error
	switch {
	case req.All:
		a.sup.Logf("unloading all models, as requested by llsl")
		err = m.UnloadAll(ctx)
	case req.Model != "":
		a.sup.Logf("unloading %s, as requested by llsl", req.Model)
		err = m.Unload(ctx, req.Model)
	default:
		return codeError{control.CodeUsage, errors.New("name a model to unload, or ask for all")}
	}
	if err != nil {
		return codeError{control.CodeError, err}
	}
	return nil
}

// healthFor returns the health snapshot for the llama-swap process pid, if
// the current monitor is watching that process.
func (a *app) healthFor(pid int) (health.Snapshot, bool) {
	a.mu.Lock()
	m, monPid := a.mon, a.monPid
	a.mu.Unlock()
	if m == nil || monPid != pid {
		return health.Snapshot{}, false
	}
	return m.Snapshot(), true
}

func (a *app) controlStatus() *control.Status {
	launcherLog, output := a.logs.paths()
	st := a.sup.Status()
	snap, _ := a.healthFor(st.Pid)
	p := a.currentPrefs()
	bin := p.BinaryPath()
	if bin == "" {
		home, _ := os.UserHomeDir()
		bin = supervisor.FindBinary("llama-swap", supervisor.SearchDirs(home))
	}
	s := &control.Status{
		AppPid:      os.Getpid(),
		AppVersion:  version,
		BundleID:    bundleID,
		State:       describe(st, snap),
		Pid:         st.Pid,
		Listen:      st.Listen,
		Version:     st.Version,
		Message:     message(st, snap),
		Binary:      bin,
		Config:      p.ConfigPath(),
		LogFile:     output,
		LauncherLog: launcherLog,
		Models:      []control.Model{},
	}
	a.mu.Lock()
	s.LastGPUFault = a.lastFault
	a.mu.Unlock()
	if st.State == supervisor.Running {
		for _, m := range snap.Models {
			if m.Loaded() {
				a.mu.Lock()
				updated := a.updated[m.ID]
				a.mu.Unlock()
				s.Models = append(s.Models, control.Model{ID: m.ID, State: m.State, Faulted: m.Faulted,
					LastTokensPerSecond: math.Round(m.TokensPerSecond*10) / 10, Updated: updated})
			}
		}
	}
	return s
}
