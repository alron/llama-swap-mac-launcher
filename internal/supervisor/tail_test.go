package supervisor

import (
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTail(t *testing.T) {
	var tl tail
	fmt.Fprint(&tl, "one\ntw")
	fmt.Fprint(&tl, "o\r\nthr")
	if got, want := tl.last(10), []string{"one", "two", "thr"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := tl.last(1), []string{"thr"}; !slices.Equal(got, want) {
		t.Errorf("last(1): got %q, want %q", got, want)
	}

	for i := range outputLines + 5 {
		fmt.Fprintf(&tl, "line %d\n", i)
	}
	got := tl.last(outputLines + 50)
	if len(got) != outputLines || got[len(got)-1] != fmt.Sprintf("line %d", outputLines+4) {
		t.Errorf("kept %d lines, ending %q", len(got), got[len(got)-1])
	}

	fmt.Fprintln(&tl, strings.Repeat("x", 3*maxLineLen))
	if got := tl.last(1)[0]; len(got) != maxLineLen {
		t.Errorf("a long line kept %d bytes, want %d", len(got), maxLineLen)
	}

	tl.reset()
	if got := tl.last(10); len(got) != 0 {
		t.Errorf("after reset: %q", got)
	}
}

// After crashing until it gives up, the output is the last attempt's.
func TestRecentOutputAfterCrashes(t *testing.T) {
	policy := Policy{Backoff: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond, MaxCrashes: 3, Window: time.Minute, StableAfter: time.Hour}
	s, _ := newTestSupervisor(t, policy)
	count := filepath.Join(t.TempDir(), "count")
	script := fmt.Sprintf(`n=$(cat %[1]s 2>/dev/null || echo 0); n=$((n+1)); echo $n > %[1]s
echo "run $n starting"; echo "run $n: something went wrong" >&2; exit 3`, count)
	if err := s.Start(shSpec(script)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Failed", func() bool { return s.Status().State == Failed })
	want := []string{"run 3 starting", "run 3: something went wrong"}
	if got := s.RecentOutput(10); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A failure before llama-swap runs shows no output, not an earlier run's.
func TestRecentOutputClearedAtLaunch(t *testing.T) {
	s, _ := newTestSupervisor(t, DefaultPolicy)
	if err := s.Start(shSpec("echo from the first run; exec sleep 60")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "output", func() bool { return len(s.RecentOutput(10)) > 0 })
	s.Stop()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	spec := shSpec("exec sleep 60")
	spec.Listen = l.Addr().String()
	if s.Start(spec) == nil {
		t.Fatal("started although the port is taken")
	}
	if got := s.RecentOutput(10); len(got) != 0 {
		t.Errorf("output after a failure before launch: %q", got)
	}
}
