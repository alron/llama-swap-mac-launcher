package supervisor

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests use /bin/sh and sleep as stand-ins for llama-swap.

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newTestSupervisor(t *testing.T, policy Policy) (*Supervisor, *syncBuffer) {
	log := &syncBuffer{}
	s := New(Options{
		Log:         log,
		PidFile:     filepath.Join(t.TempDir(), "llama-swap.pid"),
		StopTimeout: 300 * time.Millisecond,
		Policy:      policy,
	})
	t.Cleanup(s.Stop)
	return s, log
}

func shSpec(script string) Spec {
	return Spec{Binary: "/bin/sh", Args: []string{"-c", script}}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartStop(t *testing.T) {
	s, _ := newTestSupervisor(t, DefaultPolicy)
	if err := s.Start(shSpec("exec sleep 60")); err != nil {
		t.Fatal(err)
	}
	st := s.Status()
	if st.State != Running || st.Pid == 0 {
		t.Fatalf("after Start: %+v", st)
	}
	e, _ := lookup(st.Pid)
	if _, err := os.Stat(s.opts.PidFile); err != nil {
		t.Errorf("no pidfile while running: %v", err)
	}

	s.Stop()
	if st := s.Status(); st.State != Stopped {
		t.Errorf("after Stop: %+v", st)
	}
	if alive(e.proc) {
		t.Error("process still running after Stop")
	}
	if _, err := os.Stat(s.opts.PidFile); !os.IsNotExist(err) {
		t.Error("pidfile left behind after Stop")
	}
}

func TestCrashesRestartThenGiveUp(t *testing.T) {
	policy := Policy{
		Backoff:     10 * time.Millisecond,
		MaxBackoff:  20 * time.Millisecond,
		MaxCrashes:  3,
		Window:      time.Minute,
		StableAfter: time.Hour,
	}
	s, log := newTestSupervisor(t, policy)
	if err := s.Start(shSpec("exit 3")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Failed", func() bool { return s.Status().State == Failed })
	if n := strings.Count(log.String(), "started llama-swap"); n != 3 {
		t.Errorf("started %d times, want 3", n)
	}
	if msg := s.Status().Message; !strings.Contains(msg, "exit status 3") {
		t.Errorf("message %q doesn't give the exit status", msg)
	}
	if errors.Is(s.Status().Err, ErrNotStarted) {
		t.Error("crashing counts as not starting")
	}
}

func TestStopKillsLeftovers(t *testing.T) {
	childPidFile := filepath.Join(t.TempDir(), "child")
	s, log := newTestSupervisor(t, DefaultPolicy)
	// A stand-in that ignores SIGTERM, so it gets SIGKILLed and leaves its
	// child behind, like llama-swap leaving a llama-server. The child starts
	// before the trap, so it doesn't inherit ignoring SIGTERM.
	script := fmt.Sprintf("sleep 60 & echo $! > %s; trap '' TERM; wait", childPidFile)
	if err := s.Start(shSpec(script)); err != nil {
		t.Fatal(err)
	}
	var child procEntry
	waitFor(t, "the child to start", func() bool {
		b, _ := os.ReadFile(childPidFile)
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			return false
		}
		var ok bool
		child, ok = lookup(pid)
		return ok
	})

	s.Stop()
	if alive(child.proc) {
		t.Error("child outlived Stop")
	}
	if !strings.Contains(log.String(), "left running") {
		t.Errorf("log doesn't mention the leftover:\n%s", log)
	}
}

func TestCrashStopsLeftovers(t *testing.T) {
	childPidFile := filepath.Join(t.TempDir(), "child")
	// A long backoff, so the stand-in isn't restarted during the test.
	policy := DefaultPolicy
	policy.Backoff, policy.MaxBackoff = time.Hour, time.Hour
	s, log := newTestSupervisor(t, policy)
	// Like llama-swap dying from SIGKILL with a llama-server running: the
	// child is orphaned, and only the periodic snapshot knows about it.
	script := fmt.Sprintf("sleep 60 & echo $! > %s; sleep 2.5; kill -KILL $$", childPidFile)
	if err := s.Start(shSpec(script)); err != nil {
		t.Fatal(err)
	}
	var child procEntry
	waitFor(t, "the child to start", func() bool {
		b, _ := os.ReadFile(childPidFile)
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			return false
		}
		var ok bool
		child, ok = lookup(pid)
		return ok
	})

	waitFor(t, "the restart to be scheduled", func() bool { return s.Status().State == Waiting })
	if alive(child.proc) {
		t.Error("orphaned child still running after the crash")
	}
	if !strings.Contains(log.String(), "left running") {
		t.Errorf("log doesn't mention the leftover:\n%s", log)
	}
}

func TestStartFailsWhenPortTaken(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	s, _ := newTestSupervisor(t, DefaultPolicy)
	spec := shSpec("exec sleep 60")
	spec.Listen = l.Addr().String()
	if err := s.Start(spec); err == nil {
		t.Fatal("started although the port is taken")
	}
	if st := s.Status(); st.State != Failed || !errors.Is(st.Err, ErrNotStarted) {
		t.Errorf("status %+v, want Failed with ErrNotStarted", st)
	}
}

func TestCleanupLeftovers(t *testing.T) {
	// Pretend a previous run of the app left this running.
	cmd := exec.Command("/bin/sh", "-c", "sleep 60 & wait")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	var kids []proc
	waitFor(t, "sleep to start", func() bool {
		kids, _ = descendants(cmd.Process.Pid)
		return len(kids) == 1
	})
	e, _ := lookup(cmd.Process.Pid)
	pidFile := filepath.Join(t.TempDir(), "llama-swap.pid")
	if err := writePidFile(pidFile, e, nil); err != nil {
		t.Fatal(err)
	}

	s := New(Options{PidFile: pidFile, StopTimeout: time.Second})
	s.CleanupLeftovers()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("leftover wasn't stopped")
	}
	if alive(kids[0]) {
		t.Error("leftover's child is still running")
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("pidfile not removed")
	}
}

func TestCleanupIgnoresRecycledPid(t *testing.T) {
	// Our own pid, but a different start time: a pid that has been reused.
	pidFile := filepath.Join(t.TempDir(), "llama-swap.pid")
	e, _ := lookup(os.Getpid())
	e.Start++
	writePidFile(pidFile, e, nil)

	s := New(Options{PidFile: pidFile})
	s.CleanupLeftovers() // would kill this test if it got it wrong

	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("pidfile not removed")
	}
}
