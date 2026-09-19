package pty

import (
	"sync"
)

// ringBuffer is a fixed-capacity byte buffer that drops oldest data when full.
// Used for PTY output capture so a chatty process cannot grow unbounded.
type ringBuffer struct {
	mu   sync.Mutex
	buf  []byte
	max  int
	// dropped is true once capacity was exceeded at least once.
	dropped bool
}

func newRingBuffer(max int) *ringBuffer {
	if max <= 0 {
		max = DefaultMaxOutput
	}
	return &ringBuffer{
		buf: make([]byte, 0, min(max, 4096)),
		max: max,
	}
}

// Write appends p, dropping oldest bytes when over capacity.
func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	// If single write exceeds max, keep only the tail of p.
	if len(p) >= r.max {
		r.buf = append(r.buf[:0], p[len(p)-r.max:]...)
		r.dropped = true
		return len(p), nil
	}
	need := len(r.buf) + len(p) - r.max
	if need > 0 {
		r.buf = append(r.buf[:0], r.buf[need:]...)
		r.dropped = true
	}
	r.buf = append(r.buf, p...)
	return len(p), nil
}

// String returns the current contents.
func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// Bytes returns a copy of current contents.
func (r *ringBuffer) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, len(r.buf))
	copy(out, r.buf)
	return out
}

// Len returns current byte length.
func (r *ringBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buf)
}

// Cap returns the max capacity.
func (r *ringBuffer) Cap() int {
	return r.max
}

// Dropped reports whether any oldest data was discarded.
func (r *ringBuffer) Dropped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
