package main

// The Launch at Login menu item. macOS holds the setting (System Settings →
// General → Login Items), so the checkbox always reflects what macOS says
// rather than a preference of ours.

import (
	"fyne.io/systray"

	"github.com/alron/llama-swap-mac-launcher/internal/macos"
)

// watchMenuOpens refreshes the checkbox each time the menu opens, since it
// can be changed in System Settings at any time. systray's send on
// TrayOpenedCh doesn't block, so this goroutine must be the only reader and
// stay quick.
func (a *app) watchMenuOpens() {
	for range systray.TrayOpenedCh {
		a.renderMu.Lock()
		a.renderLoginItem()
		a.renderMu.Unlock()
	}
}

// renderLoginItem needs renderMu held.
func (a *app) renderLoginItem() {
	enable(a.loginItem, !a.inDialog.Load()) // see render
	switch macos.LoginItem() {
	case macos.LoginItemEnabled:
		a.loginItem.SetTitle("Launch at Login")
		a.loginItem.Check()
	case macos.LoginItemRequiresApproval:
		// macOS 27 reports an item switched off in System Settings as not
		// registered, so this is rarely seen; the header documents it.
		a.loginItem.SetTitle("Launch at Login (turned off in System Settings)")
		a.loginItem.Uncheck()
	default:
		a.loginItem.SetTitle("Launch at Login")
		a.loginItem.Uncheck()
	}
}

func (a *app) toggleLoginItem() {
	defer a.render()
	switch macos.LoginItem() {
	case macos.LoginItemEnabled:
		if err := macos.SetLoginItem(false); err != nil {
			a.problem("Couldn't turn off Launch at Login", err)
			return
		}
		a.sup.Logf("launch at login turned off")
	case macos.LoginItemRequiresApproval:
		// Registered, but switched off in System Settings, which is the
		// only place it can be switched back on.
		a.askForLoginApproval()
	default:
		if err := macos.SetLoginItem(true); err != nil {
			a.problem("Couldn't turn on Launch at Login", err)
			return
		}
		a.sup.Logf("launch at login turned on")
		if macos.LoginItem() == macos.LoginItemRequiresApproval {
			a.askForLoginApproval()
		}
	}
}

func (a *app) askForLoginApproval() {
	if macos.Confirm("Allow Llama Swap Launcher to open at login",
		"It's listed in System Settings → General → Login Items, but switched off there. "+
			"Turn it on there to have it open when you log in.",
		"Open Login Items", "Cancel") {
		macos.OpenLoginItemsSettings()
	}
}
