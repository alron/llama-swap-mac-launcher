package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A running llama-swap's descendants go into the pidfile, for the next
// launch of the app should this one crash.
func TestPidFileRecordsDescendants(t *testing.T) {
	s, _ := newTestSupervisor(t, DefaultPolicy)
	if err := s.Start(shSpec("sleep 61 & exec sleep 60")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the child in the pidfile", func() bool {
		_, children, err := readPidFile(s.opts.PidFile)
		return err == nil && len(children) == 1 && children[0].name == "sleep"
	})
	s.Stop()
	if _, err := os.Stat(s.opts.PidFile); !os.IsNotExist(err) {
		t.Error("pidfile left behind after Stop")
	}
}

// When llama-swap died with the app, its orphaned children are stopped at
// the next launch: the pidfile names them.
func TestCleanupStopsOrphans(t *testing.T) {
	// Pretend a previous run of the app left this: a llama-swap with a
	// child, then the app crashed and took llama-swap with it.
	cmd := exec.Command("/bin/sh", "-c", "sleep 61 & exec sleep 60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var kids []proc
	waitFor(t, "the child to start", func() bool {
		kids, _ = descendants(cmd.Process.Pid)
		return len(kids) == 1
	})
	t.Cleanup(func() { signalAll(kids, 9) })
	root, _ := lookup(cmd.Process.Pid)
	pidFile := filepath.Join(t.TempDir(), "llama-swap.pid")
	if err := writePidFile(pidFile, root, named(kids)); err != nil {
		t.Fatal(err)
	}
	cmd.Process.Kill()
	cmd.Wait()
	if !alive(kids[0]) {
		t.Fatal("the child should outlive its parent for this test")
	}

	log := &syncBuffer{}
	s := New(Options{Log: log, PidFile: pidFile, StopTimeout: time.Second})
	s.CleanupLeftovers()
	if alive(kids[0]) {
		t.Error("the orphan is still running")
	}
	if !strings.Contains(log.String(), "a previous llama-swap left running") {
		t.Errorf("log: %q", log.String())
	}
}

// A recorded child whose pid now belongs to another process is left alone.
func TestCleanupIgnoresRecycledChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "llama-swap.pid")
	gone := procEntry{proc: proc{Pid: 1, Start: 1}, name: "llama-swap"} // long gone
	me, _ := lookup(os.Getpid())
	other := me
	other.Start++ // our pid, but another start time
	renamed := me
	renamed.name = "llama-server" // our pid and start, but another name
	writePidFile(pidFile, gone, []procEntry{other, renamed})

	s := New(Options{PidFile: pidFile})
	s.CleanupLeftovers() // would kill this test if it got it wrong
}

// A pidfile from before descendants were recorded still reads.
func TestReadOldPidFile(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "llama-swap.pid")
	os.WriteFile(pidFile, []byte("36098 1791146073750776 llama-swap\n"), 0o600)
	e, children, err := readPidFile(pidFile)
	if err != nil || e.Pid != 36098 || e.name != "llama-swap" || len(children) != 0 {
		t.Errorf("got %+v %v %v", e, children, err)
	}
}
