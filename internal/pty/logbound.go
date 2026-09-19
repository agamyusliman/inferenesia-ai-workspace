package pty

import (
	"os"
	"sync"
)

// DefaultMaxLogBytes bounds the on-disk out.log for detached sessions so a
// spammy process cannot grow disk (or memory when re-reading) without limit
// (VAL-TERM-006 companion for detached path).
const DefaultMaxLogBytes = 1 << 20 // 1 MiB

// boundedLogWriter writes to a file and, when size exceeds max, keeps only the
// newest keepTail bytes. Safe for concurrent writes from a single writer loop.
type boundedLogWriter struct {
	mu       sync.Mutex
	f        *os.File
	path     string
	max      int
	keepTail int
	written  int64 // approximate size (reset after truncate)
}

func newBoundedLogWriter(path string, max int) (*boundedLogWriter, error) {
	if max <= 0 {
		max = DefaultMaxLogBytes
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	st, _ := f.Stat()
	var size int64
	if st != nil {
		size = st.Size()
	}
	return &boundedLogWriter{
		f:        f,
		path:     path,
		max:      max,
		keepTail: max / 2,
		written:  size,
	}, nil
}

func (w *boundedLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.f.Write(p)
	w.written += int64(n)
	if w.written > int64(w.max) {
		_ = w.compactLocked()
	}
	return n, err
}

func (w *boundedLogWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Sync()
}

func (w *boundedLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}

func (w *boundedLogWriter) compactLocked() error {
	_ = w.f.Sync()
	_ = w.f.Close()
	data, err := os.ReadFile(w.path)
	if err != nil {
		// Reopen best-effort.
		w.f, _ = os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		return err
	}
	if len(data) > w.keepTail {
		data = data[len(data)-w.keepTail:]
	}
	if err := os.WriteFile(w.path, data, 0o600); err != nil {
		w.f, _ = os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	w.f = f
	w.written = int64(len(data))
	return nil
}
