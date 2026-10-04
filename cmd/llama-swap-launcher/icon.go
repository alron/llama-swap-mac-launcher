package main

import (
	"fyne.io/systray"

	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

// menuBarLook is how the menu bar's "L→S" shows a state: the letters stay
// in the menu bar's normal colour, and the arrow is normal when llama-swap
// is working, grey while something is in progress, and red when something
// needs attention. Everything is faded when llama-swap is stopped.
type menuBarLook struct {
	letters, arrow macos.Color
}

func lookFor(st supervisor.Status, snap health.Snapshot) menuBarLook {
	normal := func(arrow macos.Color) menuBarLook { return menuBarLook{macos.ColorNormal, arrow} }
	switch st.State {
	case supervisor.Stopped:
		return menuBarLook{macos.ColorFaded, macos.ColorFaded}
	case supervisor.Failed:
		return normal(macos.ColorAlert)
	case supervisor.Stopping, supervisor.Waiting:
		return normal(macos.ColorBusy)
	}
	// Running.
	switch {
	case snap.Healthy && len(snap.Faulted()) > 0:
		return normal(macos.ColorAlert)
	case snap.Healthy && modelChanging(snap):
		return normal(macos.ColorBusy)
	case snap.Healthy:
		return normal(macos.ColorNormal)
	case snap.Problem != "":
		return normal(macos.ColorAlert) // refusing the app, such as for its API key
	case snap.WasHealthy:
		return normal(macos.ColorAlert) // not responding
	}
	return normal(macos.ColorBusy) // starting
}

// modelChanging reports whether a local model is loading or unloading.
func modelChanging(snap health.Snapshot) bool {
	for _, m := range snap.Models {
		if m.Loaded() && m.State != "ready" {
			return true
		}
	}
	return false
}

// showMenuBar updates the menu-bar title when its look changes. If the
// menu-bar item can't be found, it falls back to plain text, with a symbol
// for the arrow's colour. It needs renderMu held.
func (a *app) showMenuBar(look menuBarLook) {
	if a.shownLook != nil && *a.shownLook == look {
		return
	}
	if !macos.SetMenuBarTitle("L", "S", look.letters, look.arrow) {
		title := "L→S"
		switch look.arrow {
		case macos.ColorBusy:
			title += " …"
		case macos.ColorAlert:
			title += " ⚠"
		}
		systray.SetTitle(title)
	}
	a.shownLook = &look
}
