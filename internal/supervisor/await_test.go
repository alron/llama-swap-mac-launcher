package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// AwaitFiles waits, as Pending, for files on a disk still mounting at
// launch (TODO item 91), and reports whether they appeared.
func TestAwaitFiles(t *testing.T) {
	awaitEvery = 10 * time.Millisecond
	t.Cleanup(func() { awaitEvery = time.Second })
	dir := t.TempDir()
	there := filepath.Join(dir, "there")
	os.WriteFile(there, nil, 0o600)
	later := filepath.Join(dir, "pool", "llama-swap")

	t.Run("all there", func(t *testing.T) {
		s, _ := newTestSupervisor(t, DefaultPolicy)
		if !s.AwaitFiles([]string{there}, time.Second) || s.Status().State != Stopped {
			t.Errorf("with everything there: %+v", s.Status())
		}
	})

	t.Run("appears", func(t *testing.T) {
		s, log := newTestSupervisor(t, DefaultPolicy)
		done := make(chan bool)
		go func() { done <- s.AwaitFiles([]string{there, later}, 5*time.Second) }()
		waitFor(t, "Pending", func() bool { return s.Status().State == Pending })
		if st := s.Status(); !strings.Contains(st.Message, later) || st.State.String() != "waiting for its files" {
			t.Errorf("while waiting: %+v, %q", st, st.State)
		}
		os.MkdirAll(filepath.Dir(later), 0o700)
		os.WriteFile(later, nil, 0o600)
		if !<-done {
			t.Fatal("not reported as there")
		}
		if !strings.Contains(log.String(), "is there now") {
			t.Errorf("log:\n%s", log)
		}
		// Still Pending until the caller starts it, or stops.
		s.Stop()
		if st := s.Status(); st.State != Stopped {
			t.Errorf("after Stop: %+v", st)
		}
		os.RemoveAll(filepath.Dir(later))
	})

	t.Run("times out", func(t *testing.T) {
		s, _ := newTestSupervisor(t, DefaultPolicy)
		if s.AwaitFiles([]string{later}, 100*time.Millisecond) {
			t.Fatal("reported as there")
		}
		st := s.Status()
		if st.State != Failed || !errors.Is(st.Err, ErrNotStarted) || !strings.Contains(st.Message, later) || !strings.Contains(st.Message, "mounted") {
			t.Errorf("after the timeout: %+v", st)
		}
	})

	t.Run("stopped", func(t *testing.T) {
		s, _ := newTestSupervisor(t, DefaultPolicy)
		done := make(chan bool)
		go func() { done <- s.AwaitFiles([]string{later}, time.Minute) }()
		waitFor(t, "Pending", func() bool { return s.Status().State == Pending })
		s.Stop()
		if <-done {
			t.Error("reported as there after Stop")
		}
		if st := s.Status(); st.State != Stopped {
			t.Errorf("after Stop: %+v", st)
		}
	})

	t.Run("started instead", func(t *testing.T) {
		s, _ := newTestSupervisor(t, DefaultPolicy)
		done := make(chan bool)
		go func() { done <- s.AwaitFiles([]string{later}, time.Minute) }()
		waitFor(t, "Pending", func() bool { return s.Status().State == Pending })
		if err := s.Start(shSpec("exec sleep 60")); err != nil {
			t.Fatal(err)
		}
		if <-done {
			t.Error("reported as there after Start")
		}
		if st := s.Status(); st.State != Running {
			t.Errorf("after Start: %+v", st)
		}
	})
}
