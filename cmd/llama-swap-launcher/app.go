package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/health"
	"github.com/alron/llama-swap-mac-launcher/internal/logfile"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/paths"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
	"github.com/alron/llama-swap-mac-launcher/internal/supervisor"
)

type app struct {
	paths paths.Paths
	logs  *logs
	sup   *supervisor.Supervisor
	ctl   *net.UnixListener // the control socket, if it could be opened
	// startupErr and logErr are problems found before the menu existed,
	// shown once it does.
	startupErr, logErr error

	mu    sync.Mutex
	prefs prefs.Prefs
	// The health monitor for the current llama-swap process; see models.go.
	mon       *health.Monitor
	monCancel context.CancelFunc
	monPid    int
	alerted   map[string]bool // faulted models already reported
	lastFault *control.Fault  // the latest GPU fault since the app started
	updated   map[string]bool // models whose server program changed since they loaded

	status               *systray.MenuItem
	details              []*systray.MenuItem // the status's explanation, wrapped over several lines
	faultItem            *systray.MenuItem   // the last GPU fault, once there's been one
	noModels, unloadAll  *systray.MenuItem
	models               []*modelSlot
	start, stop, restart *systray.MenuItem
	openUI               *systray.MenuItem
	// The Logs submenu. The llama-swap log's items show only while its
	// output has its own file.
	logsMenu                         *systray.MenuItem
	openLauncherLog, showLauncherLog *systray.MenuItem
	openOwnLog, showOwnLog           *systray.MenuItem
	logSettings, collectItem         *systray.MenuItem
	aboutItem, prefsItem             *systray.MenuItem
	quitItem                         *systray.MenuItem

	loginItem *systray.MenuItem
	shownLook *menuBarLook // the menu-bar look last shown, guarded by renderMu

	// inDialog is set while a dialog flow is open (Preferences, and any
	// alert it led to, or Collect Diagnostics…). The menu then offers only
	// Quit, so nothing opens a second one or changes things underneath.
	inDialog atomic.Bool

	renderMu     sync.Mutex // render runs from both status and health updates
	quitOnce     sync.Once
	shutdownOnce sync.Once
}

func newApp(p paths.Paths, lg *logs) *app {
	return &app{
		paths: p,
		logs:  lg,
		sup: supervisor.New(supervisor.Options{
			Log: lg.notes,
			// The monitor's own health checks would otherwise be a line
			// in the log every interval, when the config logs requests.
			Output:  logfile.DropLines(lg.output, health.IsCheckLine),
			PidFile: p.PidFile(),
		}),
		prefs: prefs.Defaults(),
	}
}

// onReady builds the menu. systray calls it on its own goroutine.
func (a *app) onReady() {
	systray.SetTooltip("Llama Swap Launcher")

	a.status = disabledItem("llama-swap: stopped")
	// systray can only append items, so the explanation's lines are made
	// now and shown as needed.
	for range detailLines {
		item := disabledItem("")
		item.Hide()
		a.details = append(a.details, item)
	}
	a.faultItem = disabledItem("")
	a.faultItem.Hide()
	systray.AddSeparator()
	// systray can only append items, so the model list is a fixed set of
	// slots, shown and hidden as models load and unload.
	a.noModels = disabledItem("No models loaded")
	for range maxModelSlots {
		a.models = append(a.models, a.newModelSlot())
	}
	a.unloadAll = systray.AddMenuItem("Unload All Models", "")
	systray.AddSeparator()
	a.start = systray.AddMenuItem("Start", "")
	a.stop = systray.AddMenuItem("Stop", "")
	a.restart = systray.AddMenuItem("Restart", "")
	systray.AddSeparator()
	a.logsMenu = systray.AddMenuItem("Logs", "")
	a.openLauncherLog = a.logsMenu.AddSubMenuItem("Open Launcher Log", "")
	a.showLauncherLog = a.logsMenu.AddSubMenuItem("Show Launcher Log in Finder", "")
	a.openOwnLog = a.logsMenu.AddSubMenuItem("Open llama-swap Log", "")
	a.showOwnLog = a.logsMenu.AddSubMenuItem("Show llama-swap Log in Finder", "")
	a.logsMenu.AddSeparator()
	a.logSettings = a.logsMenu.AddSubMenuItem("Log Settings…", "")
	a.logsMenu.AddSeparator()
	a.collectItem = a.logsMenu.AddSubMenuItem("Collect Diagnostics…", "")
	a.openUI = systray.AddMenuItem("Open llama-swap UI", "")
	systray.AddSeparator()
	a.aboutItem = systray.AddMenuItem("About Llama Swap Launcher", "")
	a.prefsItem = systray.AddMenuItem("Settings…", "")
	a.loginItem = systray.AddMenuItemCheckbox("Launch at Login", "", false)
	systray.AddSeparator()
	a.quitItem = systray.AddMenuItem("Quit", "")
	// Without this, no menu item (not even Quit) works while a dialog is open.
	if !macos.MenuWorksDuringDialogs() {
		a.sup.Logf("the menu won't work while a dialog is open: systray's menu target wasn't found")
	}

	err := a.reloadPrefs()
	a.render()
	go a.watchStatus()
	go a.handleClicks()
	go a.watchMenuOpens()
	go a.watchUpdatedModels()

	// Not on this goroutine: systray waits for onReady to return before it
	// can quit, and an alert would block it.
	go func() {
		if a.startupErr != nil {
			a.problem("The control socket isn't available", a.startupErr)
		}
		if a.logErr != nil {
			a.problem("A log couldn't be opened", a.logErr)
		}
		switch {
		case err != nil:
			a.problem("Couldn't read the preferences", err)
		case a.currentPrefs().AutoStart:
			a.startLlamaSwap()
		}
	}()
}

// onExit runs when macOS terminates the app, at logout for instance.
// systray's own Quit doesn't call it: that just makes systray.Run return,
// and main calls shutdown then.
func (a *app) onExit() {
	a.shutdown()
}

// shutdown closes the control socket (removing its file), then stops
// llama-swap. It's safe to call more than once.
func (a *app) shutdown() {
	a.shutdownOnce.Do(func() {
		if a.ctl != nil {
			_ = a.ctl.Close() // best effort at exit; it removes the socket file
		}
		a.sup.Stop()
	})
}

func (a *app) quit() {
	a.quitOnce.Do(func() {
		// Close any open dialog first. systray quits by stopping AppKit's
		// event loop, and while a dialog is open that stops only the
		// dialog's, so the app would keep running. If one won't close,
		// shut down and exit directly: Quit has to quit.
		if !macos.CloseDialogs(5 * time.Second) {
			a.sup.Logf("a dialog wouldn't close; exiting directly")
			a.shutdown()
			os.Exit(0)
		}
		a.sup.Stop()
		systray.Quit()
	})
}

func (a *app) handleClicks() {
	for {
		select {
		case <-a.start.ClickedCh:
			go a.startLlamaSwap()
		case <-a.stop.ClickedCh:
			go a.sup.Stop()
		case <-a.restart.ClickedCh:
			go a.restartLlamaSwap()
		case <-a.unloadAll.ClickedCh:
			go a.unloadAllModels()
		case <-a.openLauncherLog.ClickedCh:
			launcher, _ := a.logs.paths()
			a.open(launcher)
		case <-a.showLauncherLog.ClickedCh:
			launcher, _ := a.logs.paths()
			a.reveal(launcher)
		case <-a.openOwnLog.ClickedCh:
			_, own := a.logs.paths()
			a.open(own)
		case <-a.showOwnLog.ClickedCh:
			_, own := a.logs.paths()
			a.reveal(own)
		case <-a.logSettings.ClickedCh:
			go a.openPreferences("logs")
		case <-a.collectItem.ClickedCh:
			go a.collectDiagnostics()
		case <-a.openUI.ClickedCh:
			a.open(uiURL(a.currentPrefs().ListenAddr()))
		case <-a.aboutItem.ClickedCh:
			go a.showAbout()
		case <-a.prefsItem.ClickedCh:
			go a.openPreferences("general")
		case <-a.loginItem.ClickedCh:
			go a.toggleLoginItem()
		case <-a.quitItem.ClickedCh:
			go a.quit()
			return
		}
	}
}

func (a *app) startLlamaSwap() {
	spec, err := a.prepare()
	if err != nil {
		a.problem("Can't start llama-swap", err)
		return
	}
	_ = a.sup.Start(spec) // failures show up through the status
}

// restartLlamaSwap validates the config first, so a broken config leaves
// the running llama-swap alone.
func (a *app) restartLlamaSwap() {
	spec, err := a.prepare()
	if err != nil {
		a.problem("Can't restart llama-swap", fmt.Errorf("%w. llama-swap was left running as it was", err))
		return
	}
	_ = a.sup.Restart(spec) // failures show up through the status
}

// prepare builds the spec and validates the config.
func (a *app) prepare() (supervisor.Spec, error) {
	spec, err := a.spec()
	if err != nil {
		return spec, err
	}
	return spec, supervisor.Validate(spec)
}

// spec builds llama-swap's command line from the preferences. It rereads
// the preferences file first, so hand edits apply.
func (a *app) spec() (supervisor.Spec, error) {
	if err := a.reloadPrefs(); err != nil {
		return supervisor.Spec{}, err
	}
	p := a.currentPrefs()
	// Hand edits of the log settings apply now too. A log that can't be
	// opened mustn't stop llama-swap: the one before it is kept.
	if err := a.logs.apply(p); err != nil {
		a.sup.Logf("%v; keeping the log as it was", err)
		go a.problem("A log couldn't be opened", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return supervisor.Spec{}, err
	}

	bin := p.BinaryPath()
	if bin == "" {
		bin = supervisor.FindBinary("llama-swap", supervisor.SearchDirs(home))
	}
	switch {
	case bin == "":
		return supervisor.Spec{}, errors.New("llama-swap isn't in any of the usual places. Set its location in Settings…")
	case !supervisor.IsExecutable(bin):
		return supervisor.Spec{}, fmt.Errorf("%s isn't an executable file. Set llama-swap's location in Settings…", bin)
	case p.Config == "":
		return supervisor.Spec{}, errors.New("no llama-swap config is set. Choose one in Settings…")
	}
	config := p.ConfigPath()
	if _, err := os.Stat(config); err != nil {
		return supervisor.Spec{}, fmt.Errorf("can't read the config: %w", err)
	}

	v := supervisor.BinaryVersion(bin)
	switch n := supervisor.VersionNumber(v); {
	case n > health.TestedVersion:
		a.sup.Logf("llama-swap %s is newer than v%d, the last version this app was tested with. "+
			"If the menu or llsl misbehave, that may be why", v, health.TestedVersion)
	case n > 0 && n < health.MinimumVersion:
		a.sup.Logf("llama-swap %s is older than v%d, the oldest this app works with. "+
			"Parts of the menu and llsl may not work", v, health.MinimumVersion)
	}

	secrets, appKey, err := secretEnv(p)
	if err != nil {
		return supervisor.Spec{}, err
	}
	env := map[string]string{}
	maps.Copy(env, p.Env)
	maps.Copy(env, secrets) // can't clash: the preferences can't name a variable twice

	listen := p.ListenAddr()
	args := append([]string{"-config", config, "-listen", listen}, p.Args...)
	return supervisor.Spec{
		Binary:  bin,
		Args:    args,
		Env:     supervisor.BuildEnv(os.Environ(), home, filepath.Dir(bin), env),
		Dir:     filepath.Dir(config),
		Listen:  listen,
		Version: v,
		APIKey:  appKey,
	}, nil
}

func (a *app) currentPrefs() prefs.Prefs {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prefs
}

func (a *app) reloadPrefs() error {
	p, err := prefs.Load(a.paths.Prefs())
	if err != nil {
		return fmt.Errorf("the preferences file has an error: %w", err)
	}
	a.mu.Lock()
	a.prefs = p
	a.mu.Unlock()
	return nil
}

func (a *app) watchStatus() {
	last := a.sup.Status().State
	for range a.sup.Changed() {
		st := a.sup.Status()
		a.syncMonitor(st)
		a.render()
		if st.State == supervisor.Failed && last != supervisor.Failed {
			title := "llama-swap stopped with a problem"
			switch {
			case errors.Is(st.Err, supervisor.ErrQuarantined):
				title = "macOS blocked llama-swap"
			case errors.Is(st.Err, supervisor.ErrNotStarted):
				title = "llama-swap couldn't start"
			}
			go macos.AlertWithOutput(title, st.Message, a.recentOutput(10))
		}
		last = st.State
	}
}

// render updates the whole menu from the supervisor's status and the
// health monitor's snapshot. Both can change at any time, so it always
// reads both.
func (a *app) render() {
	a.renderMu.Lock()
	defer a.renderMu.Unlock()
	st := a.sup.Status()
	snap := a.snapshot()

	a.showMenuBar(lookFor(st, snap))
	text := "llama-swap: " + describe(st, snap)
	if st.Pid != 0 {
		text += fmt.Sprintf(" (pid %d)", st.Pid)
	}
	a.status.SetTitle(text)
	a.showDetail(message(st, snap))
	a.mu.Lock()
	fault := a.lastFault
	a.mu.Unlock()
	if fault != nil {
		a.faultItem.SetTitle(faultLine(fault, time.Now()))
		a.faultItem.Show()
	}
	// Everything but Quit waits while Preferences is open.
	idle := !a.inDialog.Load()
	a.renderModels(st, snap, idle)
	a.renderLoginItem()

	up := st.State == supervisor.Running || st.State == supervisor.Waiting
	enable(a.start, idle && (st.State == supervisor.Stopped || st.State == supervisor.Failed))
	enable(a.stop, idle && up)
	enable(a.restart, idle && up)
	enable(a.logsMenu, idle)
	// llama-swap's own log, only while its output has a file of its own.
	launcher, own := a.logs.paths()
	for _, item := range []*systray.MenuItem{a.openOwnLog, a.showOwnLog} {
		if own != "" && own != launcher {
			item.Show()
		} else {
			item.Hide()
		}
	}
	enable(a.openUI, idle && st.State == supervisor.Running && snap.Healthy)
	enable(a.aboutItem, idle)
	enable(a.prefsItem, idle)
}

// showAbout shows the About panel, with llama-swap's version while it runs.
func (a *app) showAbout() {
	var credits []string
	if st := a.sup.Status(); st.State == supervisor.Running && st.Version != "" {
		credits = append(credits, "llama-swap "+st.Version)
	}
	if buildDate != "" {
		credits = append(credits, "Built "+buildDate)
	}
	credits = append(credits, "MIT License")
	macos.ShowAbout(version, credits, homepage)
}

// recentOutput returns up to n of llama-swap's last output lines, leaving
// out the app's own health checks, as its log does.
func (a *app) recentOutput(n int) []string {
	lines := slices.DeleteFunc(a.sup.RecentOutput(n+20), func(l string) bool {
		return health.IsCheckLine([]byte(l))
	})
	return lines[max(0, len(lines)-n):]
}

// describe says what llama-swap is doing, in a few words.
func describe(st supervisor.Status, snap health.Snapshot) string {
	if st.State != supervisor.Running {
		return st.State.String()
	}
	switch {
	case snap.Healthy:
		return "ready"
	case snap.Problem != "":
		return "running" // but the app can't follow it; message says why
	case snap.WasHealthy:
		return "not responding"
	}
	return "starting"
}

// message explains the state, when that's worth saying: the supervisor's
// reason, or why llama-swap refused the app.
func message(st supervisor.Status, snap health.Snapshot) string {
	if st.Message == "" && st.State == supervisor.Running {
		return snap.Problem
	}
	return st.Message
}

// problem shows err in the menu and in an alert.
func (a *app) problem(title string, err error) {
	a.sup.Logf("%s: %v", title, err)
	a.showDetail(err.Error())
	macos.Alert(title, err.Error())
}

// reveal shows a file in the Finder, selected, so it can be dragged into a
// terminal for its path.
func (a *app) reveal(path string) {
	if err := exec.Command("/usr/bin/open", "-R", path).Run(); err != nil { // #nosec G204 -- fixed binary; path is one of the app's logs
		a.sup.Logf("can't show %s in the Finder: %v", path, err)
	}
}

// open hands a file or URL to LaunchServices, which opens it in the
// default app.
func (a *app) open(target string) {
	if err := exec.Command("/usr/bin/open", target).Run(); err != nil { // #nosec G204 -- fixed binary; target is the app's log file or llama-swap's UI URL
		a.sup.Logf("can't open %s: %v", target, err)
	}
}

func uiURL(listen string) string { return "http://" + localAddr(listen) + "/ui" }

// localAddr turns a listen address into one to connect to: a wildcard host
// means this machine.
func localAddr(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return prefs.DefaultListen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func disabledItem(title string) *systray.MenuItem {
	item := systray.AddMenuItem(title, "")
	item.Disable()
	return item
}

func enable(item *systray.MenuItem, on bool) {
	if on {
		item.Enable()
	} else {
		item.Disable()
	}
}

// detailLines and detailWidth bound the status's explanation in the menu:
// menu items don't wrap, and one long item makes the whole menu that wide.
const detailLines, detailWidth = 4, 64

// showDetail shows msg under the status, wrapped over the detail lines, or
// hides them if it's empty. The full text is also each line's tooltip.
func (a *app) showDetail(msg string) {
	msg = strings.ToValidUTF8(msg, "\uFFFD") // it can quote llama-swap's output; see macos.cString
	lines := wrap(msg, detailWidth, len(a.details))
	for i, item := range a.details {
		if i < len(lines) {
			item.SetTitle(lines[i])
			item.SetTooltip(msg)
			item.Show()
		} else {
			item.Hide()
		}
	}
}

// wrap breaks s into lines of at most width characters, at spaces where it
// can, and keeps at most max of them, ending the last with "…" if any text
// is left out.
func wrap(s string, width, max int) []string {
	var lines []string
	var line []rune
	for _, word := range strings.Fields(s) {
		w := []rune(word)
		if len(line) > 0 && len(line)+1+len(w) > width {
			lines = append(lines, string(line))
			line = nil
		}
		for len(w) > width { // a word too long for a line, such as a path
			lines = append(lines, string(w[:width]))
			w = w[width:]
		}
		if len(line) > 0 {
			line = append(line, ' ')
		}
		line = append(line, w...)
	}
	if len(line) > 0 {
		lines = append(lines, string(line))
	}
	if len(lines) > max {
		lines = lines[:max]
		last := []rune(lines[max-1])
		lines[max-1] = string(last[:min(len(last), width-1)]) + "…"
	}
	return lines
}
