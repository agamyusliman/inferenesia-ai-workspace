package browser

import (
	"sync"
	"time"
)

// MaxStatusHistory caps retained BrowserStatus events for the desktop panel.
const MaxStatusHistory = 64

// TimedStatus is a Status with a wall-clock timestamp for lifecycle UI.
type TimedStatus struct {
	Status
	At time.Time `json:"at"`
}

// StatusRing keeps a ring buffer of BrowserStatus events (VAL-BRW-008).
// Thread-safe; used by Manager and core Service for the desktop panel.
type StatusRing struct {
	mu   sync.Mutex
	buf  []TimedStatus
	max  int
	last Status
}

// NewStatusRing builds a ring with capacity (default MaxStatusHistory).
func NewStatusRing(capacity int) *StatusRing {
	if capacity <= 0 {
		capacity = MaxStatusHistory
	}
	return &StatusRing{max: capacity, buf: make([]TimedStatus, 0, capacity)}
}

// Push records a status event.
func (r *StatusRing) Push(st Status) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ts := TimedStatus{Status: st, At: time.Now().UTC()}
	r.last = st
	r.buf = append(r.buf, ts)
	if len(r.buf) > r.max {
		// drop oldest
		r.buf = append([]TimedStatus(nil), r.buf[len(r.buf)-r.max:]...)
	}
}

// Snapshot returns a copy of history (oldest → newest) and last status.
func (r *StatusRing) Snapshot() (history []TimedStatus, last Status) {
	if r == nil {
		return nil, Status{State: StateIdle}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	history = append([]TimedStatus(nil), r.buf...)
	last = r.last
	if last.State == "" {
		last.State = StateIdle
	}
	return history, last
}

// Last returns the most recent status.
func (r *StatusRing) Last() Status {
	if r == nil {
		return Status{State: StateIdle}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last.State == "" {
		return Status{State: StateIdle}
	}
	return r.last
}

// Clear empties history.
func (r *StatusRing) Clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.buf = r.buf[:0]
	r.last = Status{State: StateIdle}
	r.mu.Unlock()
}
