package main

// The markUpdatedModels preference: a model whose server program changed on
// disk after it loaded (after "brew upgrade llama.cpp", say) keeps running
// the old build until it's unloaded. The app checks every minute and marks
// such models in the menu and in llsl status.

import (
	"context"
	"net/url"
	"path/filepath"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// checkUpdatesEvery is how often the app looks. Upgrades are rare, and
// each look is an lsof and a file check per running model.
const checkUpdatesEvery = time.Minute

func (a *app) watchUpdatedModels() {
	t := time.NewTicker(checkUpdatesEvery)
	defer t.Stop()
	for range t.C {
		updated := a.findUpdatedModels()
		a.mu.Lock()
		changed := len(updated) != len(a.updated)
		for id := range updated {
			changed = changed || !a.updated[id]
		}
		a.updated = updated
		a.mu.Unlock()
		if changed {
			for id := range updated {
				a.sup.Logf("%s's server program has changed on disk since it loaded; unload it to use the new one", id)
			}
			a.render()
		}
	}
}

// forgetStopped clears the mark of models that have stopped: loaded again,
// they run the program now on disk. Without this, the mark would linger
// until the next check.
func (a *app) forgetStopped(snap health.Snapshot) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range snap.Models {
		if !m.Loaded() {
			delete(a.updated, m.ID)
		}
	}
}

// findUpdatedModels returns the running local models whose server program
// has changed since they loaded, or none when the preference is off.
func (a *app) findUpdatedModels() map[string]bool {
	updated := map[string]bool{}
	p := a.currentPrefs()
	m := a.monitor()
	if !p.MarkUpdatedModels || m == nil || !m.Snapshot().Healthy {
		return updated
	}
	peer := map[string]bool{}
	for _, mod := range m.Snapshot().Models {
		peer[mod.ID] = mod.PeerID != ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	running, err := m.Running(ctx)
	if err != nil {
		return updated
	}
	dir := filepath.Dir(p.ConfigPath()) // where llama-swap runs, for a relative program path
	for id, proxy := range running {
		u, err := url.Parse(proxy)
		if err != nil || peer[id] {
			continue
		}
		host := u.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			continue // served elsewhere; nothing here to check
		}
		port := u.Port()
		if port == "" {
			continue
		}
		if changed, _, err := supervisor.ServerChanged(port, dir); err == nil && changed {
			updated[id] = true
		}
	}
	return updated
}
