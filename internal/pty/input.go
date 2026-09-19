package pty

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// InputPath is the named FIFO used to send stdin to a detached session.
func InputPath(root, id string) string {
	return filepath.Join(SessionDir(root, id), "in.fifo")
}

// EnsureInputFIFO creates the session input FIFO if missing (mode 0600 best-effort).
func EnsureInputFIFO(root, id string) (string, error) {
	path := InputPath(root, id)
	if err := os.MkdirAll(SessionDir(root, id), 0o700); err != nil {
		return "", err
	}
	// If a regular file exists from an older version, remove it.
	if st, err := os.Stat(path); err == nil {
		if st.Mode()&os.ModeNamedPipe == 0 {
			_ = os.Remove(path)
		} else {
			return path, nil
		}
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		// Already exists (race) is fine.
		if !os.IsExist(err) {
			return "", fmt.Errorf("pty fifo: %w", err)
		}
	}
	return path, nil
}

// WriteInputToSession sends data to a running session.
// Prefer in-process Manager when the session is local; otherwise write the FIFO.
func WriteInputToSession(root, id string, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("pty: empty input")
	}
	// In-process first.
	if m := DefaultManager(); m != nil {
		if s := m.Get(id); s != nil {
			_, err := s.WriteInput(data)
			return err
		}
	}
	// Detached: ensure FIFO and write.
	path, err := EnsureInputFIFO(root, id)
	if err != nil {
		return err
	}
	// Prefer O_RDWR so open does not require a peer (writers may use WRONLY once
// the worker holds RDWR). Retry briefly if the pump has not opened yet.
	deadline := time.Now().Add(2 * time.Second)
	var f *os.File
	for {
		f, err = os.OpenFile(path, os.O_RDWR, 0o600)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("pty: open input fifo for %s: %w (is the session running?)", id, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

// pumpInputFIFO reads the FIFO forever and writes chunks into the PTY session.
// Blocks until the session is done or the reader loop ends. Must run in a goroutine.
//
// Uses O_RDWR so open does not deadlock waiting for a peer on platforms where
// O_RDONLY/O_WRONLY on an empty FIFO block (common classic FIFO trap).
func pumpInputFIFO(root, id string, s *Session) {
	path, err := EnsureInputFIFO(root, id)
	if err != nil {
		return
	}
	// Hold a long-lived RDWR handle so writers can open O_WRONLY without ENXIO
	// or blocking forever before a reader exists.
	hold, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer hold.Close()

	buf := make([]byte, 4096)
	for {
		if s.Status() != StatusRunning {
			return
		}
		// Set a short read deadline via non-blocking-ish loop: Read on RDWR FIFO
		// blocks until data arrives. Use select with session done when possible.
		// Poll: if session done, exit; else try read with temporary deadline by
		// running read in place (acceptable for worker process).
		_ = hold.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, rerr := hold.Read(buf)
		if n > 0 {
			_, _ = s.WriteInput(buf[:n])
		}
		if rerr != nil {
			if s.Status() != StatusRunning {
				return
			}
			// timeout / temporary: continue
			if ne, ok := rerr.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			if rerr == io.EOF {
				// Peer closed; keep reading (hold stays open).
				time.Sleep(20 * time.Millisecond)
				continue
			}
			// Other errors: brief backoff.
			time.Sleep(50 * time.Millisecond)
		}
	}
}
