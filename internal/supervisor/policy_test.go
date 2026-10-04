package supervisor

import (
	"testing"
	"time"
)

func TestCrashBackoffThenGiveUp(t *testing.T) {
	c := crashes{policy: DefaultPolicy}
	now := time.Now()
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second} {
		got, ok := c.record(now.Add(time.Duration(i)*time.Second), time.Second)
		if !ok || got != want {
			t.Fatalf("crash %d: got %v, %v; want %v, true", i+1, got, ok, want)
		}
	}
	if _, ok := c.record(now.Add(5*time.Second), time.Second); ok {
		t.Error("5th quick crash didn't give up")
	}
}

func TestStableRunClearsHistory(t *testing.T) {
	c := crashes{policy: DefaultPolicy}
	now := time.Now()
	c.record(now, time.Second)
	c.record(now, time.Second)
	if got, _ := c.record(now, 2*time.Minute); got != time.Second {
		t.Errorf("after a stable run: delay %v, want %v", got, time.Second)
	}
}

func TestOldCrashesExpire(t *testing.T) {
	c := crashes{policy: DefaultPolicy}
	now := time.Now()
	for range 4 {
		c.record(now, time.Second)
	}
	if _, ok := c.record(now.Add(10*time.Minute), time.Second); !ok {
		t.Error("gave up although the earlier crashes were outside the window")
	}
}

func TestBackoffCapped(t *testing.T) {
	c := crashes{policy: Policy{Backoff: time.Second, MaxBackoff: 3 * time.Second, MaxCrashes: 10, Window: time.Hour, StableAfter: time.Hour}}
	var got time.Duration
	for range 5 {
		got, _ = c.record(time.Now(), 0)
	}
	if got != 3*time.Second {
		t.Errorf("got %v, want the 3s cap", got)
	}
}
