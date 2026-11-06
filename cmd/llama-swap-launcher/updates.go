package main

// Checking GitHub for a newer release (TODO item 64): daily with Settings'
// Check for updates daily box, and whenever the menu's Check for Updates…
// or llsl check-update asks. The app only says so and links to the
// release's page; it never downloads or installs anything.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/update"
)

const noticeUpdate = "update-available"

// updates is what the app knows of releases: the state kept beside the
// preferences, whether Homebrew manages this copy, and a lock so two
// checks never run at once.
type updates struct {
	mu       sync.Mutex
	state    update.State
	homebrew bool
	checking sync.Mutex
	wake     chan struct{}
}

// bundlePath is the app bundle the running executable is in.
func bundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(exe))) // …/X.app/Contents/MacOS/exe
}

// startUpdates reads what the last check found, and checks daily while
// the preferences ask. The first check waits a minute after launch, so the
// network is up after a login.
func (a *app) startUpdates() {
	a.updates.state = update.Load(a.paths.UpdateFile())
	a.updates.homebrew = update.HomebrewManaged(update.Caskroom, bundlePath())
	a.updates.wake = make(chan struct{}, 1)
	go func() {
		// A Save that turns the daily check on ends the wait: the user is
		// there, so the network is up.
		select {
		case <-a.updates.wake:
		case <-time.After(time.Minute):
		}
		for {
			if a.currentPrefs().CheckForUpdates {
				a.updates.mu.Lock()
				due := a.updates.state.Due(time.Now())
				a.updates.mu.Unlock()
				if due {
					_, _ = a.checkForUpdate(false) // logged; the menu shows what it found
				}
			}
			select {
			case <-a.updates.wake:
			case <-time.After(10 * time.Minute): // waking from sleep, it catches up within this
			}
		}
	}()
}

// updatesChanged looks again whether a check is due: after a Save that
// turned the daily check on, say.
func (a *app) updatesChanged() {
	select {
	case a.updates.wake <- struct{}{}:
	default:
	}
}

// checkForUpdate asks GitHub for the newest release, keeps what it found,
// and, for a daily check finding a newer one, posts a notification once
// per release. A manual check (the menu, llsl) is answered by its caller.
func (a *app) checkForUpdate(manual bool) (control.Update, error) {
	a.updates.checking.Lock()
	defer a.updates.checking.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	latest, err := update.Latest(ctx, health.NewClient(), update.LatestURL)
	notifies := a.currentPrefs().Notifies() // read before taking a.updates.mu, never under it

	a.updates.mu.Lock()
	s := &a.updates.state
	s.Checked = time.Now()
	if err != nil {
		s.Failed = err.Error()
	} else {
		s.Failed, s.Latest = "", latest.String()
	}
	newer, available := s.Available(version)
	post := available && !manual && s.Notified != newer.String() && notifies
	if post {
		s.Notified = newer.String()
	}
	saveErr := s.Save(a.paths.UpdateFile())
	a.updates.mu.Unlock()

	switch {
	case err != nil:
		a.sup.Logf("couldn't check for updates: %v; trying again in %s", err, update.Retry)
	case available:
		a.sup.Logf("checked for updates: %s is available (this is %s)", newer, version)
	default:
		a.sup.Logf("checked for updates: %s is the newest release (this is %s)", latest, version)
	}
	if saveErr != nil {
		a.sup.Logf("couldn't keep the update check's result: %v", saveErr)
	}
	if post {
		a.postEvent(noticeUpdate, a.updateNotice(newer))
	}
	a.render()
	return a.updateInfo(), err
}

// updateInfo is what the app knows now, for llsl and the menu.
func (a *app) updateInfo() control.Update {
	installed := a.installedNewer() // before a.updates.mu, never under it
	a.updates.mu.Lock()
	defer a.updates.mu.Unlock()
	u := control.Update{Current: version, Latest: a.updates.state.Latest, Checked: a.updates.state.Checked,
		Homebrew: a.updates.homebrew, Installed: installed}
	if v, ok := a.updates.state.Available(version); ok {
		u.Available, u.URL = true, update.ReleaseURL(v)
	}
	return u
}

// brewUpgrade is how a copy Homebrew manages is updated.
const brewUpgrade = "brew upgrade --cask " + update.Cask

func (a *app) updateNotice(v update.Version) notice {
	body := "Click to open its release page."
	if a.updates.homebrew {
		body = "Update it with " + brewUpgrade + ". Click for its release notes."
	}
	return notice{title: "Llama Swap Launcher " + v.String() + " is available", body: body}
}

// checkForUpdatesNow is the menu's Check for Updates…: it checks at once,
// whether or not the daily check is on, and says what it found.
func (a *app) checkForUpdatesNow() {
	if !a.inDialog.CompareAndSwap(false, true) {
		return
	}
	macos.ActivateForUser() // first, while the click still counts
	a.render()
	defer func() {
		a.inDialog.Store(false)
		a.render()
	}()
	u, err := a.checkForUpdate(true)
	switch {
	case err != nil:
		macos.Alert("Couldn't check for updates", sentence(err.Error())+".")
	case u.Available:
		a.offerUpdate(u)
	case u.Latest == version:
		macos.Alert("Llama Swap Launcher is up to date", version+" is the newest release.")
	default: // a build newer than any release
		macos.Alert("Llama Swap Launcher is up to date", "This is "+version+"; the newest release is "+u.Latest+".")
	}
}

// showUpdate is the menu's Version … Is Available…: what the last check
// found, with no new request.
func (a *app) showUpdate() {
	if !a.inDialog.CompareAndSwap(false, true) {
		return
	}
	macos.ActivateForUser()
	a.render()
	defer func() {
		a.inDialog.Store(false)
		a.render()
	}()
	if u := a.updateInfo(); u.Available {
		a.offerUpdate(u)
	}
}

// offerUpdate says a newer release is out, how to update this copy, and
// offers its page.
func (a *app) offerUpdate(u control.Update) {
	detail := fmt.Sprintf("This is %s. Download the new version from its release page, quit Llama Swap Launcher, "+
		"and replace it in Applications.", version)
	if u.Homebrew {
		detail = fmt.Sprintf("This is %s, installed by Homebrew: update it with %s in Terminal. "+
			"The release page has what changed.", version, brewUpgrade)
	}
	if macos.Confirm("Llama Swap Launcher "+u.Latest+" is available", detail, "Open Release Page", "Later") {
		a.open(u.URL)
	}
}

// openUpdate is a click on the update's notification: its release page,
// if it's still newer than this copy.
func (a *app) openUpdate() {
	if u := a.updateInfo(); u.Available {
		a.open(u.URL)
	}
}

// updateMenuTitle is the menu's line for a newer version: installed over
// this copy (TODO item 65), or a release GitHub has; "" for none.
func (a *app) updateMenuTitle() string {
	if v := a.installedNewer(); v != "" {
		return "Restart into Version " + v
	}
	if u := a.updateInfo(); u.Available {
		return "Version " + u.Latest + " Is Available…"
	}
	return ""
}

// updateItemClicked is a click on that line: restart into the installed
// version now, or say what the release check found.
func (a *app) updateItemClicked() {
	if s := a.installedNewer(); s != "" {
		if v, ok := update.ParseVersion(s); ok {
			a.restartInto(v, true)
		}
		return
	}
	a.showUpdate()
}
