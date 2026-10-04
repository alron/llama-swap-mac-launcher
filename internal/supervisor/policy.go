package supervisor

import "time"

// Policy decides how the supervisor reacts when llama-swap exits on its own.
type Policy struct {
	// Backoff is the delay before the first restart. It doubles with each
	// crash in the current streak, up to MaxBackoff.
	Backoff    time.Duration
	MaxBackoff time.Duration
	// After MaxCrashes crashes within Window, the supervisor gives up
	// instead of restarting.
	MaxCrashes int
	Window     time.Duration
	// A run that lasted at least StableAfter clears the crash history.
	StableAfter time.Duration
}

var DefaultPolicy = Policy{
	Backoff:     time.Second,
	MaxBackoff:  time.Minute,
	MaxCrashes:  5,
	Window:      5 * time.Minute,
	StableAfter: time.Minute,
}

// crashes tracks recent crashes to apply a Policy.
type crashes struct {
	policy Policy
	times  []time.Time
}

// record notes a crash at now, of a run that lasted ran. It returns how
// long to wait before restarting, or ok false to give up.
func (c *crashes) record(now time.Time, ran time.Duration) (delay time.Duration, ok bool) {
	if ran >= c.policy.StableAfter {
		c.times = nil
	}
	c.times = append(c.times, now)
	cutoff := now.Add(-c.policy.Window)
	for len(c.times) > 0 && c.times[0].Before(cutoff) {
		c.times = c.times[1:]
	}
	if len(c.times) >= c.policy.MaxCrashes {
		return 0, false
	}
	delay = c.policy.Backoff << (len(c.times) - 1)
	if delay <= 0 || delay > c.policy.MaxBackoff {
		delay = c.policy.MaxBackoff
	}
	return delay, true
}

func (c *crashes) reset() { c.times = nil }
