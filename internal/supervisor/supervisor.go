// Package supervisor runs llama-swap as a direct child of the app, restarts
// it when it crashes, and stops it together with everything it started.
//
// llama-swap must be a direct child, started with exec, so that macOS
// attributes its Local Network access to the app (see CLAUDE.md).
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type State int

const (
	Stopped  State = iota
	Running        // the process is up; whether it's healthy is the health poller's job
	Stopping       // waiting for it to exit
	Waiting        // it crashed; a restart is scheduled
	Failed         // it couldn't start or kept crashing, and needs the user
)

func (s State) String() string {
	switch s {
	case Stopped:
		return "stopped"
	case Running:
		return "running"
	case Stopping:
		return "stopping"
	case Waiting:
		return "restarting"
	case Failed:
		return "failed"
	}
	return fmt.Sprintf("State(%d)", int(s))
}

type Status struct {
	State State
	Pid   int
	// Message says why it's in this state, when that's worth saying.
	Message string
	// Err is the failure behind a Failed state.
	Err error
	// Listen is where the running llama-swap listens.
	Listen string
	// Version is the running llama-swap's, from its Spec.
	Version string
	// APIKey is the key for requests to the running llama-swap, from its
	// Spec. It's a secret: never show it.
	APIKey string
}

// Spec is everything needed to start llama-swap.
type Spec struct {
	Binary string
	Args   []string
	Env    []string
	Dir    string
	// Listen is llama-swap's listen address. If set, starting fails when
	// something already answers there, such as a llama-swap started
	// elsewhere.
	Listen string
	// Version is the binary's version (BinaryVersion), if known, for the
	// status and the log.
	Version string
	// APIKey is the key the app's own requests to this llama-swap send,
	// if its config sets apiKeys. The supervisor only hands it on, in the
	// status, so requests use the key this llama-swap was started with.
	APIKey string
}

type Options struct {
	// Log receives the supervisor's own notes, and llama-swap's output
	// unless Output is set.
	Log io.Writer
	// Output receives llama-swap's output. It defaults to Log.
	Output io.Writer
	// PidFile records the running llama-swap, so CleanupLeftovers can find
	// it if the app crashes.
	PidFile string
	// StopTimeout is how long llama-swap gets to exit after SIGTERM before
	// it's killed.
	StopTimeout time.Duration
	Policy      Policy
}

type Supervisor struct {
	opts    Options
	changed chan struct{}

	ops sync.Mutex // serialises Start, Stop, Restart and scheduled restarts; taken before mu

	output tail // the end of llama-swap's output since the last launch

	mu      sync.Mutex
	status  Status
	spec    Spec
	run     *run // the current process, or nil
	want    bool // whether llama-swap should be running
	crashes crashes
	timer   *time.Timer // pending restart after a crash
}

// run is one llama-swap process.
type run struct {
	cmd     *exec.Cmd
	proc    proc
	started time.Time
	exited  chan struct{} // closed once the process has been reaped
	done    chan struct{} // closed once its leftovers are cleaned up too

	treeMu sync.Mutex
	tree   []proc // llama-swap's descendants, as last seen
	over   bool   // llama-swap has exited, so the pidfile is no longer written

	entry   procEntry // llama-swap, for the pidfile
	pidFile string
	logf    func(format string, args ...any)
}

// watchTree keeps r.tree current while llama-swap runs, so that if it
// dies without stopping its model servers (a crash, or SIGKILL), they can
// still be found: by then they've been reparented to launchd.
func (r *run) watchTree() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.exited:
			return
		case <-t.C:
			r.snapshotTree()
		}
	}
}

// snapshotTree records llama-swap's current descendants. A snapshot taken
// as llama-swap died would miss its reparented children, so it's kept only
// if llama-swap was still alive afterwards.
func (r *run) snapshotTree() {
	tree, err := descendants(r.proc.Pid)
	if err != nil || !alive(r.proc) {
		return
	}
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	if r.over {
		return // wait has removed the pidfile; don't bring it back
	}
	changed := !slices.Equal(tree, r.tree)
	r.tree = tree
	// The pidfile gets the descendants too, for the next launch of the app
	// should this one crash.
	if changed && r.pidFile != "" {
		if err := writePidFile(r.pidFile, r.entry, named(tree)); err != nil {
			r.logf("can't update the pidfile: %v", err)
		}
	}
}

func (r *run) lastTree() []proc {
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	return r.tree
}

func New(opts Options) *Supervisor {
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	if opts.Output == nil {
		opts.Output = opts.Log
	}
	if opts.StopTimeout == 0 {
		opts.StopTimeout = 15 * time.Second
	}
	if opts.Policy == (Policy{}) {
		opts.Policy = DefaultPolicy
	}
	return &Supervisor{
		opts:    opts,
		changed: make(chan struct{}, 1),
		crashes: crashes{policy: opts.Policy},
	}
}

// Changed receives a value whenever the status may have changed. Values
// coalesce, so call Status after each one.
func (s *Supervisor) Changed() <-chan struct{} { return s.changed }

// RecentOutput returns up to n of the last lines llama-swap wrote, oldest
// first, since it was last launched (the supervisor keeps 100). After a
// failure, they usually say why; a failure before llama-swap ran leaves
// none.
func (s *Supervisor) RecentOutput(n int) []string {
	return s.output.last(n)
}

func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Start starts llama-swap unless it's already running. It clears the crash
// history, so it also retries after Failed.
func (s *Supervisor) Start(spec Spec) error {
	s.ops.Lock()
	defer s.ops.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil {
		return nil
	}
	return s.startLocked(spec)
}

// Restart stops llama-swap if it's running, then starts it with spec.
func (s *Supervisor) Restart(spec Spec) error {
	s.ops.Lock()
	defer s.ops.Unlock()
	s.stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked(spec)
}

// Stop stops llama-swap and everything it started: SIGTERM, then SIGKILL
// after StopTimeout, then any descendants that outlived it.
func (s *Supervisor) Stop() {
	s.ops.Lock()
	defer s.ops.Unlock()
	s.stop()
}

// startLocked needs ops and mu held.
func (s *Supervisor) startLocked(spec Spec) error {
	s.cancelTimer()
	s.crashes.reset()
	s.want = true
	s.spec = spec
	return s.launch()
}

// ErrNotStarted marks a failure before llama-swap ran at all, such as its
// port being taken or its binary not running, as opposed to llama-swap
// exiting.
var ErrNotStarted = errors.New("llama-swap didn't start")

// notStarted marks err as ErrNotStarted, keeping its message.
type notStarted struct{ error }

func (e notStarted) Is(target error) bool { return target == ErrNotStarted }
func (e notStarted) Unwrap() error        { return e.error }

// launch starts the process. It needs mu held.
func (s *Supervisor) launch() error {
	spec := s.spec
	s.output.reset() // so a failure never shows an earlier run's output
	if err := checkPortFree(spec.Listen); err != nil {
		return s.fail(notStarted{err})
	}

	cmd := exec.Command(spec.Binary, spec.Args...) // #nosec G204 -- running the user's chosen llama-swap is the app's purpose
	cmd.Env = spec.Env
	cmd.Dir = spec.Dir
	// One writer for both, so os/exec copies them through one pipe and
	// their lines don't interleave mid-line.
	out := io.MultiWriter(s.opts.Output, &s.output)
	cmd.Stdout = out
	cmd.Stderr = out
	// If something llama-swap started still holds the output pipe after
	// llama-swap exits, don't wait for it forever.
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		if quarantined(spec.Binary) {
			err = quarantineError(spec.Binary, err)
		}
		return s.fail(notStarted{fmt.Errorf("can't start %s: %w", spec.Binary, err)})
	}

	pid := cmd.Process.Pid
	e, ok := lookup(pid)
	if !ok {
		e = procEntry{proc: proc{Pid: pid}}
	}
	r := &run{
		cmd:     cmd,
		proc:    e.proc,
		started: time.Now(),
		exited:  make(chan struct{}),
		done:    make(chan struct{}),
		entry:   e,
		pidFile: s.opts.PidFile,
		logf:    s.Logf,
	}
	s.run = r
	if err := writePidFile(s.opts.PidFile, e, nil); err != nil {
		s.Logf("can't write pidfile: %v", err)
	}
	name := "llama-swap"
	if spec.Version != "" {
		name += " " + spec.Version
	}
	s.Logf("started %s (pid %d): %s", name, pid, strings.Join(cmd.Args, " "))
	s.setStatus(Status{State: Running, Pid: pid, Listen: spec.Listen, Version: spec.Version, APIKey: spec.APIKey})
	go r.watchTree()
	go s.wait(r)
	return nil
}

// wait reaps r, stops anything it left running, and decides what happens
// next.
func (s *Supervisor) wait(r *run) {
	err := r.cmd.Wait()
	close(r.exited)
	r.treeMu.Lock()
	r.over = true
	r.treeMu.Unlock()
	// Before any restart: a leftover model server still holds its memory.
	if left := living(r.lastTree()); len(left) > 0 {
		// Say llama-swap is gone first: that's what stops the app's
		// requests to it, which carry the API key and would otherwise go
		// on, during the cleanup, to whatever listens on its port next.
		s.mu.Lock()
		if s.status.State == Running {
			s.setStatus(Status{State: Stopping, Message: "llama-swap exited; stopping what it left running"})
		}
		s.mu.Unlock()
		s.Logf("stopping %d process(es) llama-swap left running: pid %s", len(left), pids(left))
		terminate(left, 5*time.Second)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	defer close(r.done)
	s.run = nil
	_ = os.Remove(s.opts.PidFile) // stale either way; a leftover one is checked before use
	ran := time.Since(r.started)
	why := describeExit(r.cmd.ProcessState, err)

	if !s.want {
		s.Logf("llama-swap stopped (%s)", why)
		s.setStatus(Status{State: Stopped})
		return
	}

	s.Logf("llama-swap exited unexpectedly after %s (%s)", ran.Round(time.Second), why)
	if r.cmd.ProcessState != nil && looksBlocked(r.cmd.ProcessState, ran) && quarantined(s.spec.Binary) {
		_ = s.fail(quarantineError(s.spec.Binary, errors.New(why))) // fail records it in the status
		return
	}
	delay, ok := s.crashes.record(time.Now(), ran)
	if !ok {
		_ = s.fail(fmt.Errorf("llama-swap keeps exiting (last time: %s), so it won't be restarted again. Check the log", why)) // fail records it in the status
		return
	}
	s.Logf("restarting in %s", delay)
	s.setStatus(Status{State: Waiting, Message: fmt.Sprintf("exited (%s); restarting in %s", why, delay)})
	s.timer = time.AfterFunc(delay, s.restartAfterCrash)
}

func (s *Supervisor) restartAfterCrash() {
	s.ops.Lock()
	defer s.ops.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timer = nil
	if s.want && s.run == nil {
		_ = s.launch() // failures are recorded in the status
	}
}

// stop needs ops held.
func (s *Supervisor) stop() {
	s.mu.Lock()
	s.want = false
	s.cancelTimer()
	r := s.run
	if r == nil {
		if s.status.State != Stopped {
			s.setStatus(Status{State: Stopped})
		}
		s.mu.Unlock()
		return
	}
	s.setStatus(Status{State: Stopping, Pid: r.proc.Pid})
	s.mu.Unlock()

	// Refresh the snapshot of llama-swap's descendants right before
	// signalling it, so wait can stop any it leaves behind.
	r.snapshotTree()
	_ = r.cmd.Process.Signal(syscall.SIGTERM) // if it has already exited, wait sees that
	select {
	case <-r.exited:
	case <-time.After(s.opts.StopTimeout):
		s.Logf("llama-swap didn't exit within %s of SIGTERM; killing it", s.opts.StopTimeout)
		_ = r.cmd.Process.Kill() // if it has already exited, wait sees that
	}
	<-r.done
}

// cancelTimer needs mu held.
func (s *Supervisor) cancelTimer() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// fail records a failure that needs the user. It needs mu held.
func (s *Supervisor) fail(err error) error {
	s.want = false
	s.Logf("%v", err)
	s.setStatus(Status{State: Failed, Message: err.Error(), Err: err})
	return err
}

// setStatus needs mu held.
func (s *Supervisor) setStatus(st Status) {
	s.status = st
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Logf writes a timestamped note from the launcher into llama-swap's log.
func (s *Supervisor) Logf(format string, args ...any) {
	fmt.Fprintf(s.opts.Log, "%s [launcher] %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// CleanupLeftovers stops a llama-swap left running by a previous run of the
// app that crashed before stopping it, along with everything it started.
// Call it once at startup, before Start.
func (s *Supervisor) CleanupLeftovers() {
	old, children, err := readPidFile(s.opts.PidFile)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	defer os.Remove(s.opts.PidFile)
	if err != nil {
		s.Logf("ignoring unreadable pidfile: %v", err)
		return
	}
	// Usually llama-swap died with the app, when its output pipe broke,
	// leaving its model servers running on their own; the pidfile names
	// them. Checking start times and names means a pid that's since been
	// reused is left alone.
	left := stillRunning(children)
	if running := stillRunning([]procEntry{old}); len(running) > 0 {
		tree, _ := descendants(old.Pid)
		s.Logf("stopping llama-swap left over from a previous run (pid %d)", old.Pid)
		terminate(running, s.opts.StopTimeout)
		left = append(left, tree...)
	}
	if left = living(left); len(left) > 0 {
		slices.SortFunc(left, func(a, b proc) int { return a.Pid - b.Pid })
		left = slices.Compact(left)
		s.Logf("stopping %d process(es) a previous llama-swap left running: pid %s", len(left), pids(left))
		terminate(left, 5*time.Second)
	}
}

// checkPortFree fails if something already accepts connections at addr.
// It dials rather than binding, because binding a specific address can
// succeed even when another process listens on the wildcard address.
func checkPortFree(addr string) error {
	if addr == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad listen address %q: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 500*time.Millisecond)
	if err != nil {
		return nil
	}
	_ = conn.Close() // only probing whether something answers
	return portTaken(addr, listeners(port))
}

// portTaken says what's listening on addr, as far as it's known, and what
// to do about it.
func portTaken(addr string, holders []procEntry) error {
	if len(holders) == 0 {
		// lsof only sees this user's processes.
		return fmt.Errorf("something is already listening on %s; is another llama-swap running?", addr)
	}
	var who []string
	app := false
	for _, e := range holders {
		w, byApp := describeHolder(e)
		who = append(who, w)
		app = app || byApp
	}
	verb, advice := "is", fmt.Sprintf("Stop it (kill %d)", holders[0].Pid)
	if len(holders) > 1 {
		verb, advice = "are", "Stop them"
	}
	if app {
		advice = "Stop llama-swap in that copy of the app"
	}
	return fmt.Errorf("%s %s already listening on %s. %s, or choose another listen address in Settings…",
		strings.Join(who, " and "), verb, addr, advice)
}

// describeHolder names a process holding the port, and for a llama-swap,
// what started it. It reports whether that was another copy of this app.
func describeHolder(e procEntry) (string, bool) {
	who := fmt.Sprintf("%s (pid %d)", e.name, e.Pid)
	if e.name != "llama-swap" {
		return who, false
	}
	// The kernel keeps only the first 16 bytes of a name, so the app's
	// llama-swap-launcher is "llama-swap-launc". The dev and release apps
	// share it, and both default to the same port.
	if parent, ok := lookup(e.ppid); ok && strings.HasPrefix(parent.name, "llama-swap-l") {
		return fmt.Sprintf("%s, run by another copy of Llama Swap Launcher (pid %d),", who, parent.Pid), true
	}
	return who + ", not started by this app,", false
}

// listeners returns this user's processes listening on TCP port, as lsof
// finds them, or none if it can't tell.
func listeners(port string) []procEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// -t prints only pids; lsof exits 1 when it finds none.
	out, _ := exec.CommandContext(ctx, "/usr/sbin/lsof", "-t", "-nP", "-iTCP:"+port, "-sTCP:LISTEN").Output() // #nosec G204 -- fixed binary; port is from the listen address, already split by net.SplitHostPort
	var found []procEntry
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil {
			if e, ok := lookup(pid); ok {
				found = append(found, e)
			}
		}
	}
	return found
}

func describeExit(ps *os.ProcessState, err error) string {
	if ps == nil {
		if err != nil {
			return err.Error()
		}
		return "unknown"
	}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return fmt.Sprintf("signal %d, %s", int(ws.Signal()), ws.Signal())
	}
	return fmt.Sprintf("exit status %d", ps.ExitCode())
}
