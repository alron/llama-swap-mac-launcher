package main

// The model list, health monitoring and GPU-fault handling.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// maxModelSlots is how many loaded models the menu lists. With matrix
// routing, llama-swap usually keeps only a handful resident.
const maxModelSlots = 8

// modelSlot is the menu line for one loaded model, with an Unload submenu.
type modelSlot struct {
	item   *systray.MenuItem
	unload *systray.MenuItem

	mu sync.Mutex
	id string // the model shown, or "" when hidden
}

func (a *app) newModelSlot() *modelSlot {
	s := &modelSlot{item: systray.AddMenuItem("", "")}
	s.unload = s.item.AddSubMenuItem("Unload", "Stop this model; llama-swap loads it again on the next request")
	s.item.Hide()
	go func() {
		for range s.unload.ClickedCh {
			if id := s.model(); id != "" {
				go a.unloadModel(id)
			}
		}
	}()
	return s
}

func (s *modelSlot) model() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

func (s *modelSlot) show(m health.Model, more int, idle, updated bool) {
	s.mu.Lock()
	s.id = m.ID
	s.mu.Unlock()
	title := modelLine(m)
	if updated && !m.Faulted {
		title += " — outdated build, unload to update"
	}
	if more > 0 {
		title += fmt.Sprintf("  (and %d more)", more)
	}
	s.item.SetTitle(title)
	enable(s.unload, idle && m.State != "stopping")
	s.item.Show()
}

func (s *modelSlot) hide() {
	s.mu.Lock()
	s.id = ""
	s.mu.Unlock()
	s.item.Hide()
}

func modelLine(m health.Model) string {
	switch {
	case m.Faulted:
		return "⚠ " + m.ID + " — GPU backend failed"
	case m.State == "ready" && m.TokensPerSecond > 0:
		// "last": it's the latest request's average, not a live reading.
		return fmt.Sprintf("● %s — last %.0f tok/s", m.ID, m.TokensPerSecond)
	case m.State == "ready":
		return "● " + m.ID
	case m.State == "starting":
		return "◌ " + m.ID + " — loading"
	case m.State == "stopping":
		return "◌ " + m.ID + " — unloading"
	}
	return "◌ " + m.ID + " — " + m.State
}

func (a *app) renderModels(st supervisor.Status, snap health.Snapshot, idle bool) {
	running := st.State == supervisor.Running
	var loaded []health.Model
	if running {
		for _, m := range snap.Models {
			if m.Loaded() {
				loaded = append(loaded, m)
			}
		}
	}
	for i, s := range a.models {
		if i >= len(loaded) {
			s.hide()
			continue
		}
		more := 0
		if i == len(a.models)-1 {
			more = len(loaded) - len(a.models)
		}
		a.mu.Lock()
		updated := a.updated[loaded[i].ID]
		a.mu.Unlock()
		s.show(loaded[i], more, idle, updated)
	}
	if running && snap.Healthy && len(loaded) == 0 {
		a.noModels.Show()
	} else {
		a.noModels.Hide()
	}
	if running {
		a.unloadAll.Show()
	} else {
		a.unloadAll.Hide()
	}
	enable(a.unloadAll, idle && len(loaded) > 0)
}

// syncMonitor keeps a health monitor running while llama-swap is: a new one
// for each llama-swap process, so nothing carries over from the last one.
func (a *app) syncMonitor(st supervisor.Status) {
	a.mu.Lock()
	defer a.mu.Unlock()
	running := st.State == supervisor.Running && st.Listen != ""
	if running && a.mon != nil && a.monPid == st.Pid {
		return
	}
	if a.monCancel != nil {
		a.monCancel()
		a.mon, a.monCancel, a.monPid = nil, nil, 0
	}
	a.alerted = make(map[string]bool)
	if !running {
		return
	}
	m := health.New("http://" + localAddr(st.Listen))
	m.HealthEvery = a.prefs.HealthCheckInterval()
	m.SkipWhenActive = a.prefs.HealthCheckSkipWhenActive
	m.Logf = a.sup.Logf
	m.APIKey = st.APIKey // the key this llama-swap was started with
	ctx, cancel := context.WithCancel(context.Background())
	a.mon, a.monCancel, a.monPid = m, cancel, st.Pid
	go m.Run(ctx)
	go a.watchHealth(ctx, m)
}

// restartMonitor replaces the running llama-swap's health monitor, so it
// picks up changed health-check preferences.
func (a *app) restartMonitor() {
	a.mu.Lock()
	a.monPid = 0 // makes syncMonitor start a new one
	a.mu.Unlock()
	a.syncMonitor(a.sup.Status())
	a.render()
}

func (a *app) watchHealth(ctx context.Context, m *health.Monitor) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.Changed():
			a.forgetStopped(m.Snapshot())
			a.render()
			a.reportFaults(m.Snapshot())
		}
	}
}

func (a *app) monitor() *health.Monitor {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mon
}

func (a *app) snapshot() health.Snapshot {
	if m := a.monitor(); m != nil {
		return m.Snapshot()
	}
	return health.Snapshot{}
}

// reportFaults tells the user, once, about each model whose GPU backend has
// failed, and offers to unload it.
func (a *app) reportFaults(snap health.Snapshot) {
	current := make(map[string]bool)
	var fresh []health.Model
	a.mu.Lock()
	for _, m := range snap.Faulted() {
		current[m.ID] = true
		if !a.alerted[m.ID] {
			a.alerted[m.ID] = true
			fresh = append(fresh, m)
		}
	}
	for id := range a.alerted {
		if !current[id] {
			delete(a.alerted, id)
		}
	}
	a.mu.Unlock()

	auto := a.currentPrefs().UnloadOnGPUFault
	for _, m := range fresh {
		a.sup.Logf("%s: GPU backend failed (its log says %q); requests to it fail until it's unloaded", m.ID, health.FaultMarker)
		a.mu.Lock()
		a.lastFault = &control.Fault{Model: m.ID, At: time.Now(), AutoUnloaded: auto}
		a.mu.Unlock()
		a.render() // the menu's "Last GPU fault" line
		if auto {
			// The unloadOnGpuFault preference: for runs nobody is watching.
			a.sup.Logf("unloading %s automatically, as the preferences ask", m.ID)
			go a.unloadModel(m.ID)
			continue
		}
		go func() {
			if macos.Confirm(m.ID+"'s GPU backend has failed",
				"Requests to this model will fail until it's unloaded. llama-swap loads it again, "+
					"in a new process, on the next request for it.",
				"Unload "+m.ID, "Later") {
				a.unloadModel(m.ID)
			}
		}()
	}
}

// faultLine describes the last GPU fault for the menu, with the time of
// day (and the date, if it wasn't today): the menu only changes when
// something else does, so "2h ago" would go stale.
func faultLine(f *control.Fault, now time.Time) string {
	when := f.At.Format("15:04")
	if y1, m1, d1 := f.At.Date(); y1 != now.Year() || m1 != now.Month() || d1 != now.Day() {
		when = f.At.Format("Jan 2") + " at " + when
	} else {
		when = "at " + when
	}
	line := fmt.Sprintf("Last GPU fault: %s %s", f.Model, when)
	if f.AutoUnloaded {
		line += ", unloaded automatically"
	}
	return line
}

func (a *app) unloadModel(id string) {
	m := a.monitor()
	if m == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a.sup.Logf("unloading %s", id)
	if err := m.Unload(ctx, id); err != nil {
		a.problem("Couldn't unload "+id, err)
	}
}

func (a *app) unloadAllModels() {
	m := a.monitor()
	if m == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a.sup.Logf("unloading all models")
	if err := m.UnloadAll(ctx); err != nil {
		a.problem("Couldn't unload the models", err)
	}
}
