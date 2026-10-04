package main

// The app's logs. The launcher log gets the app's own notes (the
// [launcher] lines) and its stderr, where Go writes a crash's stack trace.
// llama-swap's output goes, as the preferences say, into the launcher log
// too, into its own file (exactly as llama-swap wrote it, so a log
// collector can read it as it is), or nowhere. Changes apply at once: the
// writers switch between lines, so no line is split between files.

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/alron/llama-swap-mac-launcher/internal/logfile"
	"github.com/alron/llama-swap-mac-launcher/internal/prefs"
)

// Each log is rotated at maxLogSize, keeping keepLogs old files.
const (
	maxLogSize = 10 << 20
	keepLogs   = 5
)

type logs struct {
	dir    string          // the app's log folder, for the default paths
	notes  *logfile.Switch // the launcher log
	output *logfile.Switch // llama-swap's output, wherever it goes

	mu       sync.Mutex
	launcher *logfile.Rotating
	own      *logfile.Rotating // llama-swap's own log, with prefs.LogToFile
	mode     string
}

func newLogs(dir string) *logs {
	return &logs{dir: dir, notes: logfile.NewSwitch(io.Discard), output: logfile.NewSwitch(io.Discard)}
}

// apply opens the logs p asks for and switches to them. A log that can't
// be opened (its folder is gone, say) is reported, and the one before it
// kept.
func (l *logs) apply(p prefs.Prefs) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	launcherPath, ownPath := p.LogPaths(l.dir)
	var problems []error

	if l.launcher == nil || l.launcher.Path() != launcherPath {
		r, err := logfile.Open(launcherPath, maxLogSize, keepLogs)
		if err != nil {
			problems = append(problems, fmt.Errorf("the launcher log: %w", err))
		} else {
			r.OnOpen(pointStderrAt)
			l.notes.Set(r)
			old := l.launcher
			l.launcher = r
			if old != nil {
				_ = old.Close() // nothing writes to it any more
			}
		}
	}

	l.mode = p.LogMode()
	var keepOwn *logfile.Rotating
	switch l.mode {
	case prefs.LogToFile:
		if l.own != nil && l.own.Path() == ownPath {
			keepOwn = l.own
		} else if r, err := logfile.Open(ownPath, maxLogSize, keepLogs); err == nil {
			keepOwn = r
		} else {
			problems = append(problems, fmt.Errorf("llama-swap's log: %w", err))
			keepOwn = l.own // keep writing where it was, if anywhere
		}
		if keepOwn != nil {
			l.output.Set(keepOwn)
		} else {
			l.output.Set(l.launcher)
		}
	case prefs.LogOff:
		l.output.Set(io.Discard) // the supervisor still keeps the last lines
	default:
		l.output.Set(l.launcher)
	}
	if l.own != nil && l.own != keepOwn {
		_ = l.own.Close() // nothing writes to it any more
	}
	l.own = keepOwn

	if len(problems) > 0 {
		return fmt.Errorf("couldn't open %w", joinErrors(problems))
	}
	return nil
}

// paths returns the launcher log, and the file llama-swap's output goes to
// ("" when it isn't saved).
func (l *logs) paths() (launcher, output string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.launcher != nil {
		launcher = l.launcher.Path()
	}
	switch {
	case l.mode == prefs.LogOff:
	case l.mode == prefs.LogToFile && l.own != nil:
		output = l.own.Path()
	default:
		output = launcher
	}
	return launcher, output
}

func (l *logs) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range []*logfile.Rotating{l.launcher, l.own} {
		if r != nil {
			_ = r.Close() // best effort at exit
		}
	}
}

// pointStderrAt makes f the app's stderr, so anything written there, such
// as a crash's stack trace, lands in the current launcher log. When
// LaunchServices starts the app, stderr otherwise goes nowhere.
func pointStderrAt(f *os.File) {
	_ = unix.Dup2(int(f.Fd()), 2) // best effort: without it, stderr just goes nowhere
}

func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("%w; and %w", errs[0], joinErrors(errs[1:]))
}
