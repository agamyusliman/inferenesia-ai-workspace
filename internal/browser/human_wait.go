package browser

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// Default human-like wait range (milliseconds) applied around browser actions
// that mimic a user (navigate, wait_human). VAL-BRW-006.
const (
	DefaultHumanWaitMinMs = 80
	DefaultHumanWaitMaxMs = 350
)

// HumanWaitConfig controls randomized human-like delays.
// Min/Max are inclusive millisecond bounds. Zero-delay is never used when Enabled.
type HumanWaitConfig struct {
	// Enabled applies randomized delays (default true).
	Enabled bool
	// MinMs is the lower bound of the delay range (default DefaultHumanWaitMinMs).
	MinMs int
	// MaxMs is the upper bound of the delay range (default DefaultHumanWaitMaxMs).
	MaxMs int
}

// DefaultHumanWait returns the product default wait config.
func DefaultHumanWait() HumanWaitConfig {
	return HumanWaitConfig{
		Enabled: true,
		MinMs:   DefaultHumanWaitMinMs,
		MaxMs:   DefaultHumanWaitMaxMs,
	}
}

// Normalize clamps invalid ranges so Max >= Min and both are positive when Enabled.
func (c HumanWaitConfig) Normalize() HumanWaitConfig {
	out := c
	if out.MinMs <= 0 {
		out.MinMs = DefaultHumanWaitMinMs
	}
	if out.MaxMs <= 0 {
		out.MaxMs = DefaultHumanWaitMaxMs
	}
	if out.MaxMs < out.MinMs {
		out.MaxMs = out.MinMs
	}
	return out
}

// Duration returns a randomized delay in [MinMs, MaxMs]. When disabled, returns 0.
// Never returns a zero delay when Enabled (VAL-BRW-006: not zero-delay).
func (c HumanWaitConfig) Duration(rng *rand.Rand) time.Duration {
	c = c.Normalize()
	if !c.Enabled {
		return 0
	}
	span := c.MaxMs - c.MinMs
	var n int
	if span <= 0 {
		n = c.MinMs
	} else if rng != nil {
		n = c.MinMs + rng.Intn(span+1)
	} else {
		n = c.MinMs + rand.Intn(span+1)
	}
	if n < 1 {
		n = 1
	}
	return time.Duration(n) * time.Millisecond
}

// Waiter applies human-like delays. Thread-safe.
type Waiter struct {
	mu  sync.Mutex
	cfg HumanWaitConfig
	rng *rand.Rand
	// last is the most recent sleep duration (for tests).
	last time.Duration
	// sleepFn is overridable in tests (default time.Sleep / context wait).
	sleepFn func(ctx context.Context, d time.Duration) error
}

// NewWaiter builds a Waiter with the given config (defaults applied when zero).
func NewWaiter(cfg HumanWaitConfig) *Waiter {
	if cfg == (HumanWaitConfig{}) {
		// Completely zero value → product default (enabled).
		cfg = DefaultHumanWait()
	} else {
		if cfg.MinMs == 0 {
			cfg.MinMs = DefaultHumanWaitMinMs
		}
		if cfg.MaxMs == 0 {
			cfg.MaxMs = DefaultHumanWaitMaxMs
		}
	}
	cfg = cfg.Normalize()
	return &Waiter{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		sleepFn: func(ctx context.Context, d time.Duration) error {
			if d <= 0 {
				return nil
			}
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

// Config returns a copy of the current wait config.
func (w *Waiter) Config() HumanWaitConfig {
	if w == nil {
		return DefaultHumanWait()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cfg
}

// SetConfig replaces the wait config (normalized).
func (w *Waiter) SetConfig(cfg HumanWaitConfig) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.cfg = cfg.Normalize()
	w.mu.Unlock()
}

// LastDelay returns the most recent applied delay (tests / status detail).
func (w *Waiter) LastDelay() time.Duration {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

// Wait sleeps a randomized human-like duration (VAL-BRW-006).
// Returns the actual delay applied.
func (w *Waiter) Wait(ctx context.Context) (time.Duration, error) {
	if w == nil {
		// Fail-safe: still apply a small default delay so callers never zero-delay
		// when they expected human wait.
		d := DefaultHumanWait().Duration(nil)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(d):
			return d, nil
		}
	}
	w.mu.Lock()
	cfg := w.cfg
	rng := w.rng
	sleepFn := w.sleepFn
	w.mu.Unlock()

	d := cfg.Duration(rng)
	if err := sleepFn(ctx, d); err != nil {
		return d, err
	}
	w.mu.Lock()
	w.last = d
	w.mu.Unlock()
	return d, nil
}
