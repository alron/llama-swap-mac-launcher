package main

// The GPU's wired memory limit (TODO item 51): iogpu.wired_limit_mb, which
// lets the GPU wire more of the Mac's memory, so bigger models fit. It needs
// root, so the app's helper (cmd/wired-limit) sets it: launchd runs it as
// root once the user allows it in System Settings, and it applies the limit
// again at every startup. Settings' Tweaks tab turns it on and off, and
// every change is confirmed first by the person at the Mac, with Touch ID,
// an Apple Watch or their password.
//
// The limit is kept only by the helper, in a file only root can write,
// never in the app's preferences: a value that anything running as the
// user could write would be applied without anyone confirming it.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/macos"
	"github.com/alron/llama-swap-mac-launcher/internal/wired"
)

// wiredApprovalWait is how long the app watches for its helper to be
// allowed in System Settings, after a Save that turns the limit on.
const wiredApprovalWait = 5 * time.Minute

// loginItems is where the helper is allowed, as the dialogs name it.
const loginItems = "System Settings → General → Login Items & Extensions"

// wiredState is the limit as it stands.
type wiredState struct {
	totalMB  int64 // the Mac's memory
	limitMB  int64 // iogpu.wired_limit_mb now; 0 is macOS's default
	storedMB int64 // what the helper sets at every start; 0 for none
	// defaultMB is macOS's default, as Metal reports it while the limit is
	// 0; 0 when unknown.
	defaultMB int64
	helper    macos.LoginItemStatus // the helper's SMAppService status, corrected (helperState)
	running   bool                  // the helper answers, as root
	// pendingMB is a limit confirmed in Settings, waiting for the helper to
	// be allowed.
	pendingMB int64
	// unusable says why the limit can't be changed from this copy of the
	// app, on this Mac; "" when it can.
	unusable string
	// storedErr is why the helper's file couldn't be read.
	storedErr error
}

// wiredFlow is the app's part: a limit waiting for the helper's approval,
// and macOS's default, once read.
type wiredFlow struct {
	mu        sync.Mutex
	pendingMB int64
	cancel    context.CancelFunc // ends the wait
	wait      int                // counts waits, so an old one doesn't clear a newer one's
	macOSMB   int64              // macOS's default, from Metal at launch; 0 when unknown
}

func (w *wiredFlow) pending() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pendingMB
}

// stop ends a wait for the helper's approval, if there is one.
func (w *wiredFlow) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
	w.pendingMB, w.cancel = 0, nil
}

// noteDefault reads macOS's default limit, at launch: Metal reports the
// limit as the GPU's working set, which is macOS's default while
// iogpu.wired_limit_mb is 0. Metal fixes the figure when the process
// first uses the GPU, and keeps it whatever the limit becomes: an app
// launched with the limit at 80 GB went on reporting 80 GB after it was
// set back to 0 (2026-11-05). So it's read once, now, and only trusted if
// the limit is 0; otherwise macOS's default goes without a number.
func (w *wiredFlow) noteDefault() {
	if limit, err := unix.SysctlUint32("iogpu.wired_limit_mb"); err == nil && limit == 0 {
		mb := macos.GPUWorkingSetMB()
		w.mu.Lock()
		w.macOSMB = mb
		w.mu.Unlock()
	}
}

// defaultMB is macOS's default limit, as read at launch; 0 when unknown.
func (w *wiredFlow) defaultMB() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.macOSMB
}

// helperState is the helper's SMAppService status, and whether it's
// running: answering, as root. Whether it answers is what counts, since
// the status misleads both ways (macOS 27, 2026-11-05):
//   - While the app runs, the status doesn't follow an approval given in
//     System Settings: it stays "requires approval" until the app's next
//     register or unregister, or its next launch, though launchd has
//     started the helper. One that answers has been allowed.
//   - "Enabled" while launchd isn't running it, and "not registered"
//     though Background Task Management has it enabled: registering checks
//     whether the helper may start before BTM has recorded the
//     registration, so after the helper was switched off in System
//     Settings, or unregistered just before, it was refused ("Job is not
//     allowed to bootstrap"), and BTM had it enabled half a second later,
//     with nothing to start it. registerHelper deals with that.
//
// Asking costs one local XPC round trip, and fails at once when launchd
// has no such job.
func (a *app) helperState() (macos.LoginItemStatus, bool) {
	st := macos.HelperStatus(wired.Plist(bundleID))
	if (st == macos.LoginItemEnabled || st == macos.LoginItemRequiresApproval) && teamID != "" {
		if r, err := a.askHelper("info", 0); err == nil && r.UID == 0 {
			return macos.LoginItemEnabled, true
		}
	}
	return st, false
}

// wiredState reads the limit, the helper's, and what the app is waiting
// for. Reading needs no root.
func (a *app) wiredState() wiredState {
	s := wiredState{pendingMB: a.wired.pending()}
	s.helper, s.running = a.helperState()
	total, terr := unix.SysctlUint64("hw.memsize")
	limit, lerr := unix.SysctlUint32("iogpu.wired_limit_mb")
	s.totalMB, s.limitMB = int64(total>>20), int64(limit)
	s.storedMB, s.storedErr = wired.Stored(bundleID)
	if s.storedErr == nil && s.storedMB != 0 && terr == nil {
		// Root's file, checked again by the helper before it's applied;
		// here, so the diagnostics flag one out of bounds.
		if err := wired.Check(s.storedMB, int64(total>>20)); err != nil {
			s.storedErr = fmt.Errorf("the stored limit is out of bounds: %w", err)
		}
	}
	switch {
	case terr != nil || lerr != nil:
		s.unusable = "This Mac has no GPU memory limit to change (iogpu.wired_limit_mb)."
	case s.totalMB <= wired.SmallMB:
		s.unusable = fmt.Sprintf("Only on Macs with more than %d GB of memory; this one has %d GB.", wired.SmallMB>>10, s.totalMB>>10)
	case teamID == "":
		s.unusable = "Needs a copy of the app signed with a Developer ID; this one is signed ad hoc."
	case s.helper == macos.LoginItemNotFound:
		s.unusable = "The app's helper is missing from it: install the app again."
	}
	if s.limitMB == 0 && lerr == nil {
		s.defaultMB = a.wired.defaultMB()
	}
	return s
}

// managed says the app is setting the limit, or about to: its helper is
// registered, or a limit waits for it to be allowed.
func (s wiredState) managed() bool {
	return s.helper == macos.LoginItemEnabled || s.helper == macos.LoginItemRequiresApproval || s.pendingMB != 0
}

// setByApp says the limit now is the one the app's helper set.
func (s wiredState) setByApp() bool { return s.limitMB != 0 && s.limitMB == s.storedMB }

// suggestedMB is the limit Settings suggests: the Mac's memory less the
// reserve for its size.
func (s wiredState) suggestedMB() int64 {
	return s.totalMB - wired.SuggestedReserveGB(s.totalMB>>10)<<10
}

// now says what the limit is: macOS's default, or a size.
func (s wiredState) now() string {
	switch {
	case s.limitMB != 0:
		return gbText(s.limitMB)
	case s.defaultMB != 0:
		return fmt.Sprintf("macOS's default, about %.0f GB", float64(s.defaultMB)/1024)
	}
	return "macOS's default"
}

// describe is Settings' line under the limit: what's in effect, and what
// the app is doing about it.
func (s wiredState) describe() string {
	enabled := s.helper == macos.LoginItemEnabled
	switch {
	case s.unusable != "":
		return s.unusable
	case s.pendingMB != 0:
		return fmt.Sprintf("Waiting for you to allow the app's helper in %s; once it's allowed, the app sets %s. If "+
			"it's switched on there and nothing happens, Save here again. Now: %s.", loginItems, gbText(s.pendingMB), s.now())
	case s.helper == macos.LoginItemRequiresApproval && s.storedMB != 0:
		// Switched off in System Settings: allowed again, it sets its own.
		return fmt.Sprintf("The app's helper isn't allowed to run, so it won't set %s at startup: allow it again in %s, "+
			"and it sets it at once. Now: %s.", gbText(s.storedMB), loginItems, s.now())
	case s.helper == macos.LoginItemRequiresApproval:
		return fmt.Sprintf("The app's helper isn't allowed to run: allow it in %s, then Save here to set the limit. Now: %s.", loginItems, s.now())
	case enabled && !s.running:
		return fmt.Sprintf("macOS has the app's helper allowed, but isn't running it, so it isn't setting the limit. "+
			"Save here to start it again. Now: %s.", s.now())
	case enabled && s.storedMB == 0:
		return fmt.Sprintf("The app's helper is allowed, but has no limit to set: Save to set one. Now: %s.", s.now())
	case enabled && s.setByApp():
		return fmt.Sprintf("In effect: %s, set again by the app's helper at every startup.", gbText(s.storedMB))
	case enabled:
		return fmt.Sprintf("Now %s, set by something other than the app, such as a LaunchDaemon of your own. The app's "+
			"helper sets %s at the next startup; remove the other, so they don't both set it.", s.now(), gbText(s.storedMB))
	case s.limitMB == 0:
		return "Now " + s.now() + "."
	case s.setByApp():
		return fmt.Sprintf("Now %s, set by the app's helper, which has since been turned off; macOS's default returns "+
			"when the Mac restarts.", s.now())
	}
	return fmt.Sprintf("Now %s, set by something other than the app, such as a LaunchDaemon of your own. To have the "+
		"app set it instead, tick the box, and remove the other, so they don't both set it at startup.", s.now())
}

// gbText is a limit in GB: whole, or to a tenth for one set in MB by
// something else.
func gbText(mb int64) string {
	if mb%1024 == 0 {
		return fmt.Sprintf("%d GB", mb>>10)
	}
	return fmt.Sprintf("%.1f GB", float64(mb)/1024)
}

// fillWiredForm shows the limit on the Tweaks tab: the box ticked while
// the app manages it, and the field with the limit it's waiting to set,
// the one it sets, someone else's (to start from), or the suggestion.
func fillWiredForm(f *macos.PreferencesForm, s wiredState) {
	mb := s.suggestedMB()
	switch {
	case s.pendingMB != 0:
		mb = s.pendingMB
	case s.storedMB != 0:
		mb = s.storedMB
	case s.limitMB != 0:
		mb = s.limitMB
	}
	f.WiredLimit = s.managed()
	f.WiredLimitGB = strconv.FormatInt(mb>>10, 10)
	f.WiredUsable = s.unusable == ""
	f.WiredState = s.describe()
	f.WiredTotalGB = s.totalMB >> 10
	f.WiredSuggestedGB = s.suggestedMB() >> 10
	f.WiredMinGB = wired.MinMB >> 10
	f.WiredMaxGB = (s.totalMB - wired.ReserveMB) >> 10
}

// wiredWant is the limit as Settings was saved: off, or on at mb.
type wiredWant struct {
	on bool
	mb int64
}

// tweakError is a problem on the Tweaks tab, so the dialog comes back
// there.
type tweakError struct{ error }

// wiredFrom reads the limit from the dialog, checked against the bounds
// the helper enforces too. Where it can't be changed, the box was greyed
// out, and it's left as it is.
func wiredFrom(f macos.PreferencesForm, s wiredState) (wiredWant, error) {
	if s.unusable != "" {
		return wiredWant{on: s.managed(), mb: s.storedMB}, nil
	}
	if !f.WiredLimit {
		return wiredWant{}, nil
	}
	text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(f.WiredLimitGB), "GB"))
	gb, err := strconv.ParseInt(text, 10, 64)
	if err != nil || gb <= 0 || gb > math.MaxInt32>>10 {
		return wiredWant{}, tweakError{fmt.Errorf("the GPU's memory limit: %q isn't a whole number of GB", f.WiredLimitGB)}
	}
	if err := wired.Check(gb<<10, s.totalMB); err != nil {
		return wiredWant{}, tweakError{fmt.Errorf("the GPU's memory limit can be from %d to %d GB on this Mac, leaving macOS at least %d GB",
			wired.MinMB>>10, (s.totalMB-wired.ReserveMB)>>10, wired.ReserveMB>>10)}
	}
	return wiredWant{on: true, mb: gb << 10}, nil
}

// wiredAction is what a Save needs done.
type wiredAction int

const (
	wiredNothing wiredAction = iota
	// wiredSet confirms, then sets the limit, registering the helper first
	// if need be.
	wiredSet
	// wiredOff confirms, then has the helper set macOS's default and
	// forget its limit, and unregisters it.
	wiredOff
	// wiredRetry tries again to start a helper waiting to be allowed, for
	// the limit already confirmed: saving again while waiting.
	wiredRetry
	// wiredDrop unregisters a helper that changes nothing now: not
	// allowed, or with no limit of its own. No confirmation, since the
	// limit stays as it is.
	wiredDrop
)

// wiredPlan decides what a Save with want needs, from the state as it
// stands.
func wiredPlan(s wiredState, want wiredWant) wiredAction {
	enabled := s.helper == macos.LoginItemEnabled
	switch {
	case s.unusable != "":
		return wiredNothing
	case want.on && s.pendingMB == want.mb && !s.running:
		return wiredRetry // already confirmed, and waiting
	case want.on && enabled && s.running && s.storedMB == want.mb && s.limitMB == want.mb:
		return wiredNothing // in effect
	case want.on:
		return wiredSet
	case enabled && s.storedMB != 0:
		return wiredOff
	case s.managed():
		return wiredDrop
	}
	return wiredNothing
}

// applyWired puts the limit Settings was saved with into effect, after the
// rest of the Save. A change to the limit is confirmed by the person at the
// Mac first; declined, or if it can't be confirmed, nothing changes, and an
// alert says so. Nothing retries: the next Save asks again.
func (a *app) applyWired(want wiredWant) {
	s := a.wiredState() // again: the helper may have been allowed while the dialog was open
	action := wiredPlan(s, want)
	switch action {
	case wiredNothing:
		return
	case wiredRetry:
		a.sup.Logf("GPU memory limit: saved again while waiting for the app's helper; trying again to start it")
		a.wired.stop() // setWired waits again, if it must
		a.setWired(want.mb, s, true)
		return
	case wiredDrop:
		a.wired.stop()
		if a.removeHelper() && s.setByApp() {
			// It can't be reached to set macOS's default, but its limit is
			// still in effect: it lasts until the Mac restarts.
			macos.Alert("The GPU's memory limit stays at "+gbText(s.limitMB)+" until the Mac restarts",
				"The app's helper isn't allowed to run, so it couldn't return the limit to macOS's default now. It has "+
					"been removed, so it won't set the limit again: macOS's default returns when the Mac restarts.")
		}
		return
	}
	reason, change := "turn off the GPU's raised memory limit, returning it to macOS's default", "macOS's default"
	if action == wiredSet {
		reason, change = "set the GPU's memory limit to "+gbText(want.mb), gbText(want.mb)
	}
	if err := macos.ConfirmOwner(reason); err != nil {
		a.sup.Logf("GPU memory limit: the change to %s wasn't confirmed: %v", change, err)
		why := "It wasn't confirmed"
		if !errors.Is(err, macos.ErrNotConfirmed) {
			why = "It couldn't be confirmed (" + err.Error() + ")"
		}
		macos.Alert("The GPU's memory limit wasn't changed",
			fmt.Sprintf("%s, so it's still %s. The other settings were saved. To change the limit, Save it in Settings again.", why, s.now()))
		return
	}
	a.sup.Logf("GPU memory limit: the change to %s was confirmed", change)
	a.wired.stop()
	if action == wiredOff {
		a.turnOffWired()
		return
	}
	// Repaired only if macOS has it allowed but it isn't running: one
	// waiting to be allowed needs the user first, so it's waited for at
	// once, and saving again while waiting repairs it.
	a.setWired(want.mb, s, s.helper == macos.LoginItemEnabled)
}

// setWired sets the limit through the helper, registering it first if it
// isn't running, and with repair, repairing it if a register doesn't start
// it (registerHelper). A helper waiting to be allowed is waited for
// (waitForHelper).
func (a *app) setWired(mb int64, s wiredState, repair bool) {
	if !s.running {
		status, running, err := a.registerHelper(repair)
		switch {
		case running:
		case status == macos.LoginItemRequiresApproval:
			a.waitForHelper(mb)
			return
		case status == macos.LoginItemEnabled:
			a.problem("Couldn't start the app's helper", errors.New("macOS has the app's helper allowed, but didn't start it. "+
				"Saving again in a few seconds may work"))
			return
		default:
			if err == nil {
				err = fmt.Errorf("macOS reports it as %s", helperStatusName(status))
			}
			a.problem("Couldn't start the app's helper", fmt.Errorf("macOS didn't start it (%w). Saving again in a few seconds may work", err))
			return
		}
	}
	if err := a.sendWired(mb); err != nil {
		a.wiredProblem(err)
	}
}

// notKept is a limit the helper applied but couldn't keep for the next
// startup: in effect until the Mac restarts.
type notKept struct{ error }

// wiredProblem reports a limit that couldn't be set, or was set but not
// kept.
func (a *app) wiredProblem(err error) {
	if errors.As(err, new(notKept)) {
		a.problem("The GPU's memory limit lasts only until the Mac restarts", err)
		return
	}
	a.problem("Couldn't set the GPU's memory limit", err)
}

// registerHelper registers the helper, for launchd to start it, and waits
// for it to answer. It returns the helper's status then, and whether it's
// running. Registering returns an error while the helper waits to be
// allowed: that's the user's to do, not a failure.
//
// With repair, a helper that a register doesn't start is unregistered and
// registered again, up to four times. Background Task Management keeps a
// copy of the app's record in the daemons' store, and launchd starts the
// helper only if that copy has the app allowed. Switching the helper off in
// System Settings disallows it there, but switching it back on updates
// only the user's copy, and every register is refused ("Job is not allowed
// to bootstrap"; 2026-11-05, six in a row). After an unregister, a register
// or two later BTM brings the daemons' copy into step, a second or so
// after the register that set it going, which was itself refused; the
// register after that starts the helper. Every row of System Settings' list
// of background items flickers meanwhile, so it's only done when the user
// saves, never on its own. Without repair, a helper that has never been
// allowed is registered once, and macOS's request to allow it is posted
// once.
func (a *app) registerHelper(repair bool) (status macos.LoginItemStatus, running bool, err error) {
	plist := wired.Plist(bundleID)
	register := func() {
		err = macos.RegisterHelper(plist, true)
		// Up to about 2 seconds: a probe of a job launchd can't start takes
		// its whole timeout (askHelper), so it's a deadline, not a count.
		for deadline := time.Now().Add(2 * time.Second); ; {
			// Waiting to be allowed goes straight to waitForHelper, which
			// goes on looking, unless it's being repaired.
			if status, running = a.helperState(); running || !repair && status == macos.LoginItemRequiresApproval || time.Now().After(deadline) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		a.sup.Logf("GPU memory limit: registered the app's helper (%s); it's %s, %s", orOK(err), helperStatusName(status), runningText(running))
	}
	register()
	if running || !repair {
		return status, running, err
	}
	uerr := macos.RegisterHelper(plist, false)
	a.sup.Logf("GPU memory limit: the app's helper didn't start; unregistered it (%s), to register it again", orOK(uerr))
	for try := 0; try < 4 && !running; try++ {
		register()
	}
	return status, running, err
}

func runningText(running bool) string {
	if running {
		return "running"
	}
	return "not running"
}

// waitForHelper opens System Settings where the helper is allowed, and
// watches for it, for up to wiredApprovalWait, then sets mb.
func (a *app) waitForHelper(mb int64) {
	ctx, cancel := context.WithTimeout(context.Background(), wiredApprovalWait)
	a.wired.mu.Lock()
	a.wired.pendingMB, a.wired.cancel = mb, cancel
	a.wired.wait++
	wait := a.wired.wait
	a.wired.mu.Unlock()
	a.sup.Logf("GPU memory limit: waiting up to %s for the app's helper to be allowed in System Settings", wiredApprovalWait)
	macos.OpenLoginItemsSettings()
	go func() {
		defer func() {
			cancel()
			a.wired.mu.Lock()
			if a.wired.wait == wait { // not replaced or stopped since
				a.wired.pendingMB, a.wired.cancel = 0, nil
			}
			a.wired.mu.Unlock()
		}()
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					status, _ := a.helperState()
					a.sup.Logf("GPU memory limit: the app's helper didn't start within %s (it's %s); %s not set", wiredApprovalWait, helperStatusName(status), gbText(mb))
					why := fmt.Sprintf("The app's helper wasn't allowed to run within %.0f minutes. Once it's allowed in %s, "+
						"Save the limit in Settings again.", wiredApprovalWait.Minutes(), loginItems)
					if status == macos.LoginItemEnabled {
						why = fmt.Sprintf("macOS has the app's helper allowed, but didn't start it within %.0f minutes. Save the "+
							"limit in Settings again to try again.", wiredApprovalWait.Minutes())
					}
					macos.Alert("The GPU's memory limit wasn't set", why)
				}
				return
			case <-tick.C:
			}
			status, running := a.helperState()
			if !running && (status == macos.LoginItemEnabled || status == macos.LoginItemRequiresApproval) {
				continue // not yet
			}
			switch status {
			case macos.LoginItemEnabled:
				a.sup.Logf("GPU memory limit: the app's helper was allowed")
				if err := a.sendWired(mb); err != nil {
					a.wiredProblem(err)
					return
				}
				a.tellWired("The GPU's memory limit is set",
					fmt.Sprintf("The GPU may now use up to %s of the Mac's %s. The app's helper sets it again at every startup.",
						gbText(mb), gbText(a.wiredState().totalMB)))
				return
			case macos.LoginItemNotRegistered, macos.LoginItemNotFound:
				a.sup.Logf("GPU memory limit: the app's helper was removed while waiting to be allowed; %s not set", gbText(mb))
				return
			}
		}
	}()
}

// tellWired reports a limit set after the wait: as a notification, if
// they're on and shown, otherwise in an alert.
func (a *app) tellWired(title, body string) {
	if a.notifying() {
		a.postEvent("gpu-memory", notice{title: title, body: body})
		return
	}
	macos.Alert(title, body)
}

// sendWired asks the helper to set mb, retrying for about 15 seconds
// while it can't be reached: launchd may still be starting it, just
// allowed. It checks what comes back: from root, with the limit in effect.
func (a *app) sendWired(mb int64) error {
	var err error
	deadline := time.Now().Add(15 * time.Second)
	for try := 0; try == 0 || time.Now().Before(deadline); try++ {
		if try > 0 {
			time.Sleep(time.Second)
		}
		var r macos.HelperReply
		r, err = a.askHelper("set", mb)
		if err != nil {
			continue // not reached: try again
		}
		switch {
		case r.UID != 0:
			return fmt.Errorf("the app's helper is running as user %d, not as root", r.UID)
		case !r.OK && r.Limit == mb && mb != 0:
			return notKept{errors.New(sentence(r.Error))}
		case !r.OK:
			return errors.New(sentence(r.Error))
		case r.Limit != mb:
			return fmt.Errorf("the app's helper set %s, but macOS reports %s", gbText(mb), gbText(r.Limit))
		case mb == 0:
			a.sup.Logf("GPU memory limit: back to macOS's default")
		default:
			a.sup.Logf("GPU memory limit: set to %d MB (%s), and kept by the app's helper for every startup", mb, gbText(mb))
		}
		return nil
	}
	return fmt.Errorf("the app's helper couldn't be reached: %w", err)
}

// askHelper sends the helper a request, accepting an answer only from the
// helper in this app, signed by its team. A running helper answers in
// milliseconds; one launchd can't start never does, so "info" (asked by
// Settings, llsl status and the diagnostics) waits 3 seconds, and "set",
// which may have to wait for launchd to start the helper, 10.
func (a *app) askHelper(op string, mb int64) (macos.HelperReply, error) {
	timeout := 3 * time.Second
	if op == "set" {
		timeout = 10 * time.Second
	}
	return macos.HelperRequest(wired.Label(bundleID), wired.HelperRequirement(bundleID, teamID), op, mb, timeout)
}

// turnOffWired has the helper set macOS's default and forget its limit,
// then unregisters it. Unregistered, it can't set one at the next startup,
// so even if the first step fails, macOS's default returns then.
func (a *app) turnOffWired() {
	// As a limit is set: retried while launchd may be slow to start the
	// helper, and checked (from root, with macOS's default now), so a
	// timeout doesn't leave the old limit stored for the next start.
	err := a.sendWired(0)
	uerr := macos.RegisterHelper(wired.Plist(bundleID), false)
	if uerr == nil {
		a.sup.Logf("GPU memory limit: removed the app's helper")
	}
	switch {
	case err != nil && uerr != nil:
		a.problem("Couldn't turn off the GPU's memory limit", fmt.Errorf(
			"the app's helper couldn't set macOS's default (%v), nor be removed (%v). Remove it in %s", err, uerr, loginItems))
	case err != nil:
		a.problem("Couldn't turn off the GPU's memory limit now", fmt.Errorf(
			"the app's helper couldn't set macOS's default: %w. It has been removed, so macOS's default returns when the Mac restarts", err))
	case uerr != nil:
		a.problem("Couldn't remove the app's helper", fmt.Errorf(
			"the limit is back to macOS's default, but the helper is still registered (%w). Remove it in %s", uerr, loginItems))
	}
}

// removeHelper unregisters a helper that can't change anything now: not
// allowed to run, or with no limit of its own. It reports whether it did.
func (a *app) removeHelper() bool {
	if err := macos.RegisterHelper(wired.Plist(bundleID), false); err != nil {
		a.problem("Couldn't remove the app's helper", fmt.Errorf("%w. Remove it in %s", err, loginItems))
		return false
	}
	a.sup.Logf("GPU memory limit: removed the app's helper, which couldn't change the limit now")
	return true
}

func helperStatusName(s macos.LoginItemStatus) string {
	switch s {
	case macos.LoginItemNotRegistered:
		return "not registered"
	case macos.LoginItemEnabled:
		return "enabled"
	case macos.LoginItemRequiresApproval:
		return "requires approval"
	case macos.LoginItemNotFound:
		return "not found"
	}
	return fmt.Sprintf("status %d", s)
}

func orOK(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

// control is the limit for llsl status and the diagnostics.
func (s wiredState) control() *control.GPUMemory {
	return &control.GPUMemory{
		TotalMB:     s.totalMB,
		LimitMB:     s.limitMB,
		DefaultMB:   s.defaultMB,
		StoredMB:    s.storedMB,
		Helper:      helperStatusName(s.helper),
		Running:     s.running,
		PendingMB:   s.pendingMB,
		SetByApp:    s.setByApp(),
		Unavailable: s.unusable,
	}
}
