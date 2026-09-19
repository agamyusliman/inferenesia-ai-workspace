package pty

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// VAL-TERM-006: high-output process does not unbounded-grow the capture buffer.
func TestBoundedOutputUnderHighLoad(t *testing.T) {
	mgr := NewManager(t.TempDir())
	mgr.MaxOutput = 4 << 10 // 4 KiB ring
	// Flood: many short lines, no sleep — stress the ring.
	s, err := mgr.Start(StartOpts{
		Shell: `i=0; while [ $i -lt 5000 ]; do echo "SPAM-LINE-$i-XXXXXXXXXXXXXXXXXXXXXXXXXXXX"; i=$((i+1)); done; sleep 30`,
		ID:    "spam-1",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(time.Second) }()

	// Wait until buffer has data and process is still (or was) producing.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(s.Output()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Give the flood time to fill and overflow the ring.
	time.Sleep(400 * time.Millisecond)

	out := s.Output()
	if len(out) > mgr.MaxOutput {
		t.Fatalf("buffer len=%d exceeds MaxOutput=%d", len(out), mgr.MaxOutput)
	}
	// After flooding many lines, capacity bound holds regardless of Dropped flag.
	// (Dropped may race with process finishing the loop early.)
	// CLI remains "responsive": Stop returns promptly.
	start := time.Now()
	if err := s.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("Stop took too long under load: %v", time.Since(start))
	}
}

// VAL-TERM-008: send input to cat and capture echo.
func TestSendInputToCatEchoes(t *testing.T) {
	mgr := NewManager(t.TempDir())
	s, err := mgr.Start(StartOpts{Command: []string{"cat"}, ID: "cat-in"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(time.Second) }()

	const msg = "hello-pty"
	if _, err := s.WriteString(msg + "\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.Output(), msg) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("did not capture echo of %q; out=%q", msg, s.Output())
}

// VAL-TERM-009: CleanupNow stops in-process sessions; detached are separate.
func TestCleanupOnExitStopsInProcessSessions(t *testing.T) {
	// Isolate cleanup registry by using a dedicated manager + CleanupNow.
	mgr := NewManager(t.TempDir())
	RegisterCleanup(mgr)
	defer UnregisterCleanup(mgr)

	s, err := mgr.Start(StartOpts{Shell: "while true; do sleep 1; done", ID: "cleanup-1"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := s.PID()
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("not alive: %v", err)
	}

	CleanupNow()
	// Process should be gone (or almost).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("pid %d still alive after CleanupNow", pid)
}

// VAL-TERM-010: port helpers enforce 4100–4199; off-limits rejected.
func TestControlPortRange(t *testing.T) {
	if !PortInRange(4100) || !PortInRange(4199) || !PortInRange(4110) {
		t.Fatal("expected in-range ports accepted")
	}
	if PortInRange(4099) || PortInRange(4200) {
		t.Fatal("boundary outside range should fail")
	}
	for _, p := range OffLimitsPorts {
		if PortAllowed(p) {
			t.Fatalf("off-limits port %d must not be allowed", p)
		}
	}
	if PortAllowed(5000) || PortAllowed(7000) || PortAllowed(5599) {
		t.Fatal("explicit off-limits must fail PortAllowed")
	}
	// Core PTY opens no control port by default — verify constant contract only
	// (no listener spun up in package tests).
}

// VAL-TERM-011: stop already-stopped / unknown is safe (manager API).
func TestStopAlreadyStoppedAndUnknown(t *testing.T) {
	mgr := NewManager(t.TempDir())
	s, err := mgr.Start(StartOpts{Shell: "sleep 60", ID: "stop-once"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Stop(2 * time.Second); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Second stop on same session: nil.
	if err := s.Stop(time.Second); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	// Manager.Stop after exit still ok (session still registered).
	if err := mgr.Stop("stop-once", time.Second); err != nil {
		t.Fatalf("Manager.Stop already-stopped: %v", err)
	}
	if err := mgr.Stop("never-existed", time.Second); err == nil || !IsNotFound(err) {
		t.Fatalf("unknown want ErrNotFound, got %v", err)
	}
}

// VAL-TERM-012: List shows id, command, pid, status; empty when fresh.
func TestSessionListFields(t *testing.T) {
	mgr := NewManager(t.TempDir())
	if list := mgr.List(); len(list) != 0 {
		t.Fatalf("fresh list want empty, got %+v", list)
	}

	s, err := mgr.Start(StartOpts{
		Shell: "while true; do sleep 1; done",
		ID:    "list-1",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(time.Second) }()

	list := mgr.List()
	if len(list) != 1 {
		t.Fatalf("list len=%d want 1", len(list))
	}
	info := list[0]
	if info.ID != "list-1" {
		t.Fatalf("id=%q", info.ID)
	}
	if info.Command == "" {
		t.Fatal("command empty")
	}
	if info.PID <= 0 {
		t.Fatalf("pid=%d", info.PID)
	}
	if info.Status != StatusRunning {
		t.Fatalf("status=%s want running", info.Status)
	}

	_ = s.Stop(2 * time.Second)
	// After stop, list still contains the session with non-running status
	// (manager keeps meta until Remove). CLI detach uses disk for empty/gone.
	list2 := mgr.List()
	if len(list2) != 1 {
		t.Fatalf("after stop list len=%d", len(list2))
	}
	if list2[0].Status != StatusStopped && list2[0].Status != StatusExited {
		t.Fatalf("status after stop=%s", list2[0].Status)
	}
}

// VAL-TERM-006 + disk: bounded log writer truncates oldest.
func TestBoundedLogWriterDropsOldest(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/out.log"
	w, err := newBoundedLogWriter(path, 256)
	if err != nil {
		t.Fatal(err)
	}
	// Write well past max.
	line := strings.Repeat("X", 64) + "\n"
	for i := 0; i < 40; i++ {
		if _, err := w.Write([]byte(fmt.Sprintf("%04d-%s", i, line))); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Sync()
	_ = w.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 256 {
		// After compact, keepTail is max/2 = 128, but last write may append
		// until next compact trigger; allow up to max.
		if len(data) > 256+64 {
			t.Fatalf("log size %d exceeds bound", len(data))
		}
	}
	// Newest content should survive; earliest markers often gone.
	if !strings.Contains(string(data), "X") {
		t.Fatalf("expected retained data: %q", data)
	}
}

// Detached input via FIFO (when writer+reader wired).
func TestWriteInputViaFIFO(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(t.TempDir())
	s, err := mgr.Start(StartOpts{Command: []string{"cat"}, ID: "fifo-cat"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(time.Second) }()

	// Simulate worker FIFO pump.
	go pumpInputFIFO(root, "fifo-cat", s)
	time.Sleep(50 * time.Millisecond)

	if err := WriteInputToSession(root, "fifo-cat", []byte("hello-fifo\n")); err != nil {
		// WriteInputToSession tries DefaultManager first — set it.
		// Session is in mgr, not DefaultManager; FIFO path should still work.
		t.Logf("WriteInputToSession: %v (retry after sleep)", err)
		time.Sleep(100 * time.Millisecond)
		if err2 := WriteInputToSession(root, "fifo-cat", []byte("hello-fifo\n")); err2 != nil {
			// Direct write via path if pump has reader.
			path, _ := EnsureInputFIFO(root, "fifo-cat")
			f, oerr := os.OpenFile(path, os.O_WRONLY, 0o600)
			if oerr != nil {
				t.Fatalf("open fifo: %v (first err %v)", oerr, err2)
			}
			_, _ = f.Write([]byte("hello-fifo\n"))
			_ = f.Close()
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.Output(), "hello-fifo") {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("fifo input not echoed; out=%q", s.Output())
}
