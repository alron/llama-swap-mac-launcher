package main

// The model list, health monitoring and GPU-fault handling.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
	go a.watchHealth(ctx, m, st.Pid)
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

func (a *app) watchHealth(ctx context.Context, m *health.Monitor, pid int) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.Changed():
			if snap := m.Snapshot(); snap.CanPause {
				a.followPause(ctx, m, pid, snap.Paused)
			}
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
			go a.postEvent(noticeUnloaded+m.ID, unloadedNotice(m.ID))
			continue
		}
		// With notifications showing, its notice (with Unload) replaces the
		// alert; clicking the notice opens the alert (notices.go).
		if !a.notifying() {
			go a.offerUnload(m.ID)
		}
	}
}

// offerUnload asks whether to unload a model whose GPU backend has failed.
func (a *app) offerUnload(model string) {
	if macos.Confirm(model+"'s GPU backend has failed",
		"Requests to this model will fail until it's unloaded. llama-swap loads it again, "+
			"in a new process, on the next request for it.",
		"Unload "+model, "Later") {
		a.unloadModel(model)
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

// followPause keeps pauseWanted in step with what llama-swap pid says, so
// a pause or resume made elsewhere (llama-swap's web UI, say) is adopted.
// The exception is the first word from a llama-swap the app has started
// since inference was paused: with KeepPausedOnRestart, it's paused again.
func (a *app) followPause(ctx context.Context, m *health.Monitor, pid int, paused bool) {
	a.mu.Lock()
	again := pauseAgain(a.pauseWanted, paused, pid != a.pausePid, a.prefs.KeepsPause())
	a.pausePid = pid
	if !again {
		a.pauseWanted = paused
		a.recordPause()
	}
	a.mu.Unlock()
	if again {
		a.sup.Logf("pausing inference again, as it was before llama-swap started")
		go func() {
			if err := m.Pause(ctx); err != nil && ctx.Err() == nil {
				a.sup.Logf("couldn't pause inference again: %v", err)
			}
		}()
	}
}

// pauseAgain reports whether to pause a llama-swap that says it isn't
// paused: only one just started (fresh), while a pause was wanted, and with
// the preference on.
func pauseAgain(wanted, paused, fresh, keep bool) bool {
	return wanted && !paused && fresh && keep
}

// recordPause keeps pauseWanted on disk for the app's next launch (TODO
// item 59): the pause file is there while a pause is wanted and the
// preferences keep it across the app's restarts, and gone otherwise.
// Nothing is written or removed until restorePause has read it at launch.
// Call with a.mu held.
func (a *app) recordPause() {
	if !a.pauseRestored {
		return
	}
	file := a.paths.PauseFile()
	if !a.pauseWanted || !a.prefs.KeepsPauseOnAppRestart() {
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			a.sup.Logf("couldn't remove %s, so the app's next launch may pause inference again: %v", file, err)
		}
		return
	}
	if _, err := os.Stat(file); err == nil {
		return // already there, with the time the pause began
	}
	// The time is for anyone looking; only the file being there counts.
	when := []byte(time.Now().Format(time.RFC3339) + "\n")
	if err := os.WriteFile(file, when, 0o600); err != nil {
		a.sup.Logf("couldn't write %s, so the pause won't outlast quitting the app: %v", file, err)
	}
}

// restorePause, at launch, takes up a pause recorded before the app last
// quit, if the preferences still keep one: the first llama-swap the app
// starts is then paused again, as after a restart of llama-swap
// (followPause). The preferences must be loaded first.
func (a *app) restorePause() {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := os.Stat(a.paths.PauseFile())
	a.pauseWanted = err == nil && a.prefs.KeepsPauseOnAppRestart()
	a.pauseRestored = true
	if a.pauseWanted {
		a.sup.Logf("inference was paused when the app last quit; llama-swap is paused again when it starts")
	}
	a.recordPause() // removes a file the preferences no longer keep
}

// inferencePaused reports whether llama-swap's inference is paused, or is
// about to be again, after a start the app just made. A llama-swap known to
// be too old to pause won't be, whatever was wanted (by a pause kept from
// before the app last quit, say, with a newer llama-swap).
func (a *app) inferencePaused(snap health.Snapshot) bool {
	if snap.Paused {
		return true
	}
	if n := supervisor.VersionNumber(a.sup.Status().Version); n > 0 && n < health.PauseVersion {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pauseWanted && a.prefs.KeepsPause()
}

// togglePause is Pause All and Resume All in the menu.
func (a *app) togglePause() {
	m := a.monitor()
	if m == nil {
		return
	}
	snap := m.Snapshot()
	if !snap.CanPause {
		return
	}
	if err := a.setPaused(m, !snap.Paused, ""); err != nil {
		if snap.Paused {
			a.problem("Couldn't resume inference", err)
		} else {
			a.problem("Couldn't pause inference", err)
		}
	}
}

// setPaused pauses or resumes inference, and remembers which, for the
// next llama-swap the app starts. Pausing waits until every local model
// has stopped.
func (a *app) setPaused(m *health.Monitor, pause bool, why string) error {
	a.mu.Lock()
	a.pauseWanted = pause
	a.recordPause()
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if pause {
		a.sup.Logf("pausing inference%s", why)
		return m.Pause(ctx)
	}
	a.sup.Logf("resuming inference%s", why)
	return m.Resume(ctx)
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
