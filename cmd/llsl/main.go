// Command llsl controls llama-swap through Llama Swap Launcher, for
// scripts, agents and anyone in a terminal.
//
// llsl never starts llama-swap itself: it asks the app to, because macOS
// attributes llama-swap's Local Network access to its parent, and that has
// to be the app (see CLAUDE.md). When the app isn't running, llsl launches
// it through LaunchServices (`open -b`), never by running the app's
// executable, which would make llsl's caller responsible instead.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/alron/llama-swap-mac-launcher/internal/control"
	"github.com/alron/llama-swap-mac-launcher/internal/paths"
)

const usage = `usage: llsl <command> [flags]

Controls llama-swap through Llama Swap Launcher.

commands:
  status                   show llama-swap's state, loaded models and peers
  start   [-wait] [-load MODEL] [-timeout D]
                           start llama-swap, launching the app if needed
  restart [-wait] [-load MODEL] [-timeout D] [-force]
                           restart llama-swap, launching the app if needed.
                           The config is validated first; a bad one leaves
                           llama-swap running as it was.
  stop    [-force]         stop llama-swap and everything it started
  unload  MODEL | -all [-force]
                           unload a model, or every model
  pause   [-force]         pause inference: unload every local model, and
                           turn requests for them away until resume
                           (llama-swap v268 or later)
  resume                   let requests through again
  logs    [-n N] [-f] [-launcher]
                           print the end of llama-swap's log, or follow it;
                           with -launcher, the launcher log
  check-update             ask GitHub, through the app, whether a newer
                           release of Llama Swap Launcher is out

flags:
  -wait      return once llama-swap is ready, or has failed
  -load M    also load model M and wait until it's ready (implies -wait)
  -timeout D how long -wait and -load may take (default 10m)
  -json      print the result as JSON (all but logs)
  -force     restart, stop, unload or pause even while llama-swap is
             serving requests; without it they refuse (exit 7)

exit status:
  0  ok; for status, llama-swap is ready and no model's GPU backend has failed
  1  failed; for status, llama-swap isn't ready or a model has failed
  2  usage error
  3  the app isn't running, or couldn't be launched
  4  the config isn't valid; llama-swap was left as it was
  5  the model didn't load
  6  timed out
  7  busy: llama-swap is serving requests, so nothing was done; see -force
  8  paused: for status, inference is paused; for -load, it couldn't load
     because of that
  9  for check-update, a newer release is out
`

const (
	exitOK = iota
	exitFailed
	exitUsage
	exitNoApp
	exitInvalidConfig
	exitLoadFailed
	exitTimeout
	exitBusy
	exitPaused
	exitUpdate
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}

	id := findBundleID()
	p, err := paths.New(id)
	if err != nil {
		fmt.Fprintln(stderr, "llsl:", err)
		return exitFailed
	}
	c := &cli{id: id, paths: p, stdout: stdout, stderr: stderr}

	switch cmd {
	case "status", "stop", "pause", "resume":
		return c.simple(cmd, args)
	case "start", "restart":
		return c.start(cmd, args)
	case "unload":
		return c.unload(args)
	case "logs":
		return c.logs(args)
	case "check-update":
		return c.checkUpdate(args)
	}
	fmt.Fprintf(stderr, "llsl: unknown command %q\n\n%s", cmd, usage)
	return exitUsage
}

type cli struct {
	id     string
	paths  paths.Paths
	stdout io.Writer
	stderr io.Writer
}

func (c *cli) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	fs.Usage = func() { fmt.Fprint(c.stderr, usage) }
	return fs
}

// simple runs status, stop, pause and resume, which never launch the app.
func (c *cli) simple(cmd string, args []string) int {
	fs := c.flags(cmd)
	asJSON := fs.Bool("json", false, "")
	force := new(bool)
	if cmd == "stop" || cmd == "pause" {
		force = fs.Bool("force", false, "")
	}
	if fs.Parse(args) != nil || fs.NArg() > 0 {
		return exitUsage
	}
	resp, err := c.call(control.Request{Cmd: cmd, Force: *force}, 30*time.Second)
	if errors.Is(err, control.ErrNotRunning) {
		switch cmd {
		case "stop":
			fmt.Fprintln(c.stderr, "llsl: the app isn't running, so neither is llama-swap")
			return exitOK
		case "pause", "resume":
			fmt.Fprintln(c.stderr, "llsl: the app isn't running, so neither is llama-swap")
			return exitNoApp
		}
		if *asJSON {
			c.printJSON(control.Response{Code: "not_running", Error: err.Error()})
		} else {
			fmt.Fprintln(c.stdout, "app:        not running")
		}
		return exitNoApp
	}
	return c.report(resp, err, *asJSON, cmd == "status")
}

// checkUpdate asks the app to check GitHub for a newer release now, as
// the menu's Check for Updates… does (TODO item 64). It never launches the
// app: the app makes the request, with its own User-Agent, and keeps what
// it found.
func (c *cli) checkUpdate(args []string) int {
	fs := c.flags("check-update")
	asJSON := fs.Bool("json", false, "")
	if fs.Parse(args) != nil || fs.NArg() > 0 {
		return exitUsage
	}
	resp, err := c.call(control.Request{Cmd: "check-update"}, 45*time.Second)
	if err != nil {
		fmt.Fprintln(c.stderr, "llsl:", err)
		if errors.Is(err, control.ErrNotRunning) || errors.Is(err, errOldApp) {
			return exitNoApp
		}
		return exitFailed
	}
	if *asJSON {
		c.printJSON(resp)
	}
	if !resp.OK {
		fmt.Fprintln(c.stderr, "llsl:", clean(resp.Error))
		return exitFailed
	}
	if resp.Update == nil {
		fmt.Fprintln(c.stderr, "llsl: the app didn't say what it found; it may be older than llsl")
		return exitFailed
	}
	if !*asJSON {
		printUpdate(c.stdout, resp.Update)
	}
	if resp.Update.Available {
		return exitUpdate
	}
	return exitOK
}

// printUpdate says what a check found, as status's update line does.
func printUpdate(w io.Writer, u *control.Update) {
	latest, current := clean(u.Latest), clean(u.Current)
	if u.Installed != "" {
		fmt.Fprintf(w, "update:     %s is installed over this copy, which is still %s until the app restarts\n", clean(u.Installed), current)
		return
	}
	if !u.Available {
		fmt.Fprintf(w, "update:     none; %s is the newest release (this is %s)\n", latest, current)
		return
	}
	fmt.Fprintf(w, "update:     %s is available (this is %s): %s\n", latest, current, clean(u.URL))
	if u.Homebrew {
		fmt.Fprintln(w, "            update with: brew upgrade --cask llama-swap-launcher")
	}
}

func (c *cli) start(cmd string, args []string) int {
	fs := c.flags(cmd)
	wait := fs.Bool("wait", false, "")
	load := fs.String("load", "", "")
	timeout := fs.Duration("timeout", 10*time.Minute, "")
	asJSON := fs.Bool("json", false, "")
	force := new(bool)
	if cmd == "restart" {
		force = fs.Bool("force", false, "")
	}
	if fs.Parse(args) != nil || fs.NArg() > 0 {
		return exitUsage
	}
	req := control.Request{Cmd: cmd, Wait: *wait || *load != "", Load: *load, TimeoutMs: timeout.Milliseconds(), Force: *force}
	resp, err := c.callLaunching(req, *timeout+time.Minute)
	return c.report(resp, err, *asJSON, false)
}

func (c *cli) unload(args []string) int {
	fs := c.flags("unload")
	all := fs.Bool("all", false, "")
	asJSON := fs.Bool("json", false, "")
	force := fs.Bool("force", false, "")
	if fs.Parse(args) != nil {
		return exitUsage
	}
	var model string
	if fs.NArg() > 0 { // flags may also follow the model
		model = fs.Arg(0)
		if fs.Parse(fs.Args()[1:]) != nil || fs.NArg() > 0 {
			return exitUsage
		}
	}
	if (model == "") == !*all {
		fmt.Fprint(c.stderr, "llsl: name one model to unload, or use -all\n")
		return exitUsage
	}
	resp, err := c.call(control.Request{Cmd: "unload", Model: model, All: *all, Force: *force}, 2*time.Minute)
	return c.report(resp, err, *asJSON, false)
}

// version is llsl's, compiled in by the Makefile. It comes with the app,
// so it should match the running app's.
var version = "dev"

// warnVersion notes on stderr when the running app isn't the version this
// llsl came with. That happens after an update while the old app is still
// running, and the old app may not understand everything a new llsl asks.
func (c *cli) warnVersion(s *control.Status) {
	if s == nil || s.AppVersion == "" || version == "dev" || s.AppVersion == version {
		return
	}
	fmt.Fprintf(c.stderr, "llsl: note: the running app is version %s, but this llsl is %s. "+
		"They should match; usually the app was updated while the old one kept running, so quit it and open it again\n",
		s.AppVersion, version)
}

// errOldApp means the app that's running is a version from before the
// control socket was renamed, which this llsl can't reach.
var errOldApp = errors.New("an older version of the app is still running, and this llsl can't reach it. " +
	"Quit it from the menu bar; `llsl start`, or opening the app, then starts the new one")

// call sends req to the app. When nothing answers, it checks for an older
// app at the old socket name, so llsl can say so rather than report the app
// as not running (and, for start, try to launch it, which would only bring
// the old one forward).
func (c *cli) call(req control.Request, timeout time.Duration) (control.Response, error) {
	resp, err := control.Call(c.paths.Socket(), req, timeout)
	if errors.Is(err, control.ErrNotRunning) {
		if _, oldErr := control.Call(c.paths.OldSocket(), control.Request{Cmd: "status"}, 2*time.Second); oldErr == nil {
			return resp, errOldApp
		}
	}
	return resp, err
}

// callLaunching sends req, launching the app first if it isn't running.
func (c *cli) callLaunching(req control.Request, timeout time.Duration) (control.Response, error) {
	resp, err := c.call(req, timeout)
	if !errors.Is(err, control.ErrNotRunning) {
		return resp, err
	}
	if err := c.launchApp(); err != nil {
		return resp, err
	}
	return control.Call(c.paths.Socket(), req, timeout)
}

// launchApp launches the app through LaunchServices, so the app, not llsl's
// caller, is responsible for it, and waits for it to answer.
func (c *cli) launchApp() error {
	fmt.Fprintf(c.stderr, "llsl: launching the app (%s)\n", c.id)
	out, err := exec.Command("/usr/bin/open", "-g", "-b", c.id).CombinedOutput() // #nosec G204 -- fixed binary; c.id is a bundle ID
	if err != nil {
		return fmt.Errorf("%w: couldn't launch it: %s", control.ErrNotRunning, strings.TrimSpace(string(out)))
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := control.Call(c.paths.Socket(), control.Request{Cmd: "status"}, 2*time.Second); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("%w: launched it, but it didn't answer within 20s", control.ErrNotRunning)
}

// report prints a response and returns the exit status for it.
func (c *cli) report(resp control.Response, err error, asJSON, isStatus bool) int {
	if err != nil {
		fmt.Fprintln(c.stderr, "llsl:", err)
		if errors.Is(err, control.ErrNotRunning) || errors.Is(err, errOldApp) {
			return exitNoApp
		}
		return exitFailed
	}
	c.warnVersion(resp.Status)
	code := exitCode(resp, isStatus)
	if asJSON {
		c.printJSON(resp)
	} else if resp.Status != nil {
		printStatus(c.stdout, resp.Status)
	}
	if !resp.OK {
		fmt.Fprintln(c.stderr, "llsl:", clean(resp.Error))
		// The log shows why llama-swap or a model failed. A config that
		// didn't validate never ran, so the log wouldn't say anything.
		if len(resp.Output) > 0 {
			fmt.Fprintf(c.stderr, "--- llama-swap's last %d lines:\n", len(resp.Output))
			for _, line := range resp.Output {
				fmt.Fprintln(c.stderr, line)
			}
		}
	}
	return code
}

func exitCode(resp control.Response, isStatus bool) int {
	if !resp.OK {
		switch resp.Code {
		case control.CodeUsage:
			return exitUsage
		case control.CodeInvalidConfig:
			return exitInvalidConfig
		case control.CodeLoadFailed:
			return exitLoadFailed
		case control.CodeTimeout:
			return exitTimeout
		case control.CodeBusy:
			return exitBusy
		case control.CodePaused:
			return exitPaused
		}
		return exitFailed
	}
	if isStatus && resp.Status != nil {
		if resp.Status.Paused {
			return exitPaused
		}
		if resp.Status.State != "ready" {
			return exitFailed
		}
		for _, m := range resp.Status.Models {
			if m.Faulted {
				return exitFailed
			}
		}
	}
	return exitOK
}

// clean makes text from elsewhere safe to print: control characters, such
// as an escape sequence that redraws the terminal or a newline that forges
// a line of output, show as \x1b-style escapes. (-json escapes them
// already; logs print as they are, as cat would.)
func clean(s string) string {
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) && r != '\t' {
			fmt.Fprintf(&b, "\\x%02x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cleaned returns a copy of s with every text field cleaned: the state,
// the messages, model and peer IDs, and a peer's error all come from
// outside llsl.
func cleaned(s *control.Status) *control.Status {
	c := *s
	for _, f := range []*string{&c.AppVersion, &c.BundleID, &c.State, &c.Listen, &c.Version, &c.Message,
		&c.Warning, &c.Binary, &c.Config, &c.ConfigDir, &c.LogFile, &c.LauncherLog} {
		*f = clean(*f)
	}
	c.Models = slices.Clone(s.Models)
	for i := range c.Models {
		c.Models[i].ID, c.Models[i].State = clean(c.Models[i].ID), clean(c.Models[i].State)
	}
	c.Peers = slices.Clone(s.Peers)
	for i := range c.Peers {
		c.Peers[i].ID, c.Peers[i].Error = clean(c.Peers[i].ID), clean(c.Peers[i].Error)
	}
	if s.LastGPUFault != nil {
		f := *s.LastGPUFault
		f.Model = clean(f.Model)
		c.LastGPUFault = &f
	}
	if s.Update != nil {
		u := *s.Update
		u.Current, u.Latest, u.URL = clean(u.Current), clean(u.Latest), clean(u.URL)
		c.Update = &u
	}
	if s.GPUMemory != nil {
		g := *s.GPUMemory
		g.Helper, g.Unavailable = clean(g.Helper), clean(g.Unavailable)
		c.GPUMemory = &g
	}
	return &c
}

func printStatus(w io.Writer, s *control.Status) {
	s = cleaned(s)
	fmt.Fprintf(w, "app:        running (pid %d, %s %s)\n", s.AppPid, s.BundleID, s.AppVersion)
	line := "llama-swap: " + s.State
	switch {
	case s.Pid != 0 && s.Listen != "" && s.Version != "":
		line += fmt.Sprintf(" (pid %d, %s, %s)", s.Pid, s.Listen, strings.Fields(s.Version)[0])
	case s.Pid != 0 && s.Listen != "":
		line += fmt.Sprintf(" (pid %d, %s)", s.Pid, s.Listen)
	case s.Pid != 0:
		line += fmt.Sprintf(" (pid %d)", s.Pid)
	}
	fmt.Fprintln(w, line)
	if s.Message != "" {
		fmt.Fprintln(w, "            "+s.Message)
	}
	if s.Warning != "" {
		fmt.Fprintln(w, "warning:    "+s.Warning)
	}
	fmt.Fprintln(w, "config:     "+orNone(s.Config))
	if s.ConfigDir != "" {
		folder := s.ConfigDir
		if s.ManagedConfig {
			folder += " (managed by the app)"
		}
		fmt.Fprintln(w, "folder:     "+folder)
	}
	if s.APIKeysRequired {
		fmt.Fprintln(w, "api keys:   required, as the app manages them")
	}
	fmt.Fprintln(w, "binary:     "+orNone(s.Binary))
	if len(s.Models) == 0 {
		fmt.Fprintln(w, "models:     none loaded")
	}
	for i, m := range s.Models {
		label := "            "
		if i == 0 {
			label = "models:     "
		}
		state := modelState(m)
		if m.LastTokensPerSecond > 0 && !m.Faulted {
			state += fmt.Sprintf(", last request %.0f tok/s", m.LastTokensPerSecond)
		}
		if m.Updated {
			state += ", server program updated since it loaded: unload it to use the new build"
		}
		fmt.Fprintf(w, "%s%s (%s)\n", label, m.ID, state)
	}
	for i, p := range s.Peers {
		label := "            "
		if i == 0 {
			label = "peers:      "
		}
		state := "ok"
		if !p.OK {
			state = "NOT ANSWERING: " + p.Error
		}
		fmt.Fprintf(w, "%s%s (%s)\n", label, p.ID, state)
	}
	if f := s.LastGPUFault; f != nil {
		auto := ""
		if f.AutoUnloaded {
			auto = ", unloaded automatically"
		}
		fmt.Fprintf(w, "last fault: %s's GPU backend, %s%s\n", f.Model, f.At.Local().Format("2006-01-02 15:04:05"), auto)
	}
	if s.GPUMemory != nil {
		fmt.Fprintln(w, "gpu memory: "+gpuMemory(s.GPUMemory))
	}
	switch {
	case s.LogFile == "":
		fmt.Fprintln(w, "log:        llama-swap's output isn't saved (llsl logs shows its last lines)")
	case s.LogFile != s.LauncherLog && s.LauncherLog != "":
		fmt.Fprintln(w, "log:        "+s.LogFile)
	}
	if s.LauncherLog != "" {
		fmt.Fprintln(w, "launcher:   "+s.LauncherLog)
	}
	if s.Update != nil {
		printUpdate(w, s.Update)
	}
}

// gpuMemory says what the GPU's memory limit is, and whether the app set
// it (TODO item 51). Only Settings changes it.
func gpuMemory(g *control.GPUMemory) string {
	of := " of " + gb(g.TotalMB)
	var line string
	switch {
	case g.LimitMB == 0 && g.DefaultMB > 0:
		line = fmt.Sprintf("macOS's default, about %.0f GB%s", float64(g.DefaultMB)/1024, of)
	case g.LimitMB == 0:
		line = "macOS's default (the Mac has " + gb(g.TotalMB) + ")"
	case g.SetByApp:
		line = gb(g.LimitMB) + of + ", set by the app"
	default:
		line = gb(g.LimitMB) + of + ", not set by the app"
	}
	switch {
	case g.PendingMB != 0:
		line += "; " + gb(g.PendingMB) + " once the app's helper is allowed in System Settings (if it's allowed and nothing happens, Save in Settings again)"
	case g.Helper == "requires approval":
		line += "; the app's helper waits to be allowed in System Settings"
	case g.Helper == "enabled" && !g.Running:
		line += "; the app's helper is allowed but not running (Save in Settings starts it again)"
	case g.Helper == "enabled" && g.StoredMB != 0 && g.StoredMB != g.LimitMB:
		line += "; the app sets " + gb(g.StoredMB) + " at the next startup"
	}
	return line
}

// gb is a size in MB, in GB: whole, or to a tenth.
func gb(mb int64) string {
	if mb%1024 == 0 {
		return fmt.Sprintf("%d GB", mb>>10)
	}
	return fmt.Sprintf("%.1f GB", float64(mb)/1024)
}

func modelState(m control.Model) string {
	switch {
	case m.Faulted:
		return "GPU BACKEND FAILED; unload it to recover"
	case m.State == "starting":
		return "loading"
	case m.State == "stopping":
		return "unloading"
	}
	return m.State
}

func orNone(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

func (c *cli) printJSON(resp control.Response) {
	b, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Fprintln(c.stdout, string(b))
}
