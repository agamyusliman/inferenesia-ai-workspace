package pty

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// VAL-TERM-001: start long-running process; pid running; non-blocking return.
func TestStartLongRunningDoesNotBlock(t *testing.T) {
	mgr := NewManager(t.TempDir())
	start := time.Now()
	s, err := mgr.Start(StartOpts{Shell: "while true; do sleep 1; done"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(time.Second) }()

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Start blocked for %v (want <2s)", elapsed)
	}
	if s.PID() <= 0 {
		t.Fatal("expected positive pid")
	}
	if s.Status() != StatusRunning {
		t.Fatalf("status=%s want running", s.Status())
	}
	if !s.Running() {
		t.Fatal("process should be alive")
	}
	// Confirm via kill 0.
	if err := syscall.Kill(s.PID(), 0); err != nil {
		t.Fatalf("pid %d not running: %v", s.PID(), err)
	}
}

// VAL-TERM-002: streaming markers appear over time.
func TestCaptureStreamingMarkers(t *testing.T) {
	mgr := NewManager(t.TempDir())
	marker := fmt.Sprintf("MARK-%d", time.Now().UnixNano())
	shell := fmt.Sprintf("while true; do echo %s; sleep 0.2; done", marker)
	s, err := mgr.Start(StartOpts{Shell: shell})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(time.Second) }()

	// Wait until we have at least 2 markers.
	deadline := time.Now().Add(3 * time.Second)
	var out string
	for time.Now().Before(deadline) {
		out = s.Output()
		if strings.Count(out, marker) >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	count := strings.Count(out, marker)
	if count < 2 {
		t.Fatalf("expected ≥2 markers over time, got %d; out=%q", count, out)
	}
}

// VAL-TERM-003: stop kills process cleanly; no orphan.
func TestStopKillsProcess(t *testing.T) {
	mgr := NewManager(t.TempDir())
	s, err := mgr.Start(StartOpts{Shell: "while true; do sleep 1; done"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := s.PID()
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("before stop: pid not alive: %v", err)
	}

	if err := s.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Allow reaping.
	s.WaitTimeout(2 * time.Second)

	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("after stop: pid %d still alive", pid)
	}
	st := s.Status()
	if st != StatusStopped && st != StatusExited {
		t.Fatalf("status=%s want stopped|exited", st)
	}
}

// VAL-TERM-004: exit code captured when process exits on its own.
func TestExitCodeCaptured(t *testing.T) {
	mgr := NewManager(t.TempDir())
	s, err := mgr.Start(StartOpts{Shell: "echo done; exit 7"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.WaitTimeout(3 * time.Second) {
		t.Fatal("process did not exit in time")
	}
	code, ok := s.ExitCode()
	if !ok {
		t.Fatal("exit code not set")
	}
	if code != 7 {
		t.Fatalf("exit_code=%d want 7; output=%q", code, s.Output())
	}
	if s.Status() != StatusExited && s.Status() != StatusStopped {
		t.Fatalf("status=%s", s.Status())
	}
	if !strings.Contains(s.Output(), "done") {
		t.Fatalf("output missing done: %q", s.Output())
	}
}

// VAL-TERM-005: two concurrent sessions independent.
func TestMultipleSessionsIndependent(t *testing.T) {
	mgr := NewManager(t.TempDir())
	markerA := "ALPHA-" + fmt.Sprintf("%d", time.Now().UnixNano())
	markerB := "BETA-" + fmt.Sprintf("%d", time.Now().UnixNano())

	a, err := mgr.Start(StartOpts{
		Shell: fmt.Sprintf("while true; do echo %s; sleep 0.15; done", markerA),
		ID:    "sess-a",
	})
	if err != nil {
		t.Fatalf("Start A: %v", err)
	}
	b, err := mgr.Start(StartOpts{
		Shell: fmt.Sprintf("while true; do echo %s; sleep 0.15; done", markerB),
		ID:    "sess-b",
	})
	if err != nil {
		t.Fatalf("Start B: %v", err)
	}
	defer func() {
		_ = a.Stop(time.Second)
		_ = b.Stop(time.Second)
	}()

	if a.PID() == b.PID() {
		t.Fatal("pids must be distinct")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(a.Output(), markerA) && strings.Contains(b.Output(), markerB) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	outA := a.Output()
	outB := b.Output()
	if !strings.Contains(outA, markerA) {
		t.Fatalf("A missing its marker; out=%q", outA)
	}
	if strings.Contains(outA, markerB) {
		t.Fatalf("A leaked B's marker; out=%q", outA)
	}
	if !strings.Contains(outB, markerB) {
		t.Fatalf("B missing its marker; out=%q", outB)
	}
	if strings.Contains(outB, markerA) {
		t.Fatalf("B leaked A's marker; out=%q", outB)
	}

	// Stop A; B must remain running.
	if err := a.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop A: %v", err)
	}
	a.WaitTimeout(2 * time.Second)
	if err := syscall.Kill(a.PID(), 0); err == nil {
		t.Fatal("A still alive after stop")
	}
	if !b.Running() {
		t.Fatal("B stopped when A was stopped (interference)")
	}
	if err := syscall.Kill(b.PID(), 0); err != nil {
		t.Fatalf("B not alive after A stop: %v", err)
	}

	list := mgr.List()
	if len(list) != 2 {
		t.Fatalf("list len=%d want 2", len(list))
	}
}

func TestRingBufferDropsOldest(t *testing.T) {
	r := newRingBuffer(16)
	_, _ = r.Write([]byte("abcdefghijklmnopXXXX")) // >16
	got := r.String()
	if len(got) > 16 {
		t.Fatalf("len=%d want ≤16", len(got))
	}
	if !r.Dropped() {
		t.Fatal("expected dropped=true")
	}
	// Tail should be the end of the write.
	if !strings.HasSuffix(got, "XXXX") && !strings.Contains(got, "XXXX") {
		// after oversize single write we keep last max bytes of that write
		if got != "efghijklmnopXXXX"[len("efghijklmnopXXXX")-16:] {
			// Accept any 16-byte suffix of the written content.
			full := "abcdefghijklmnopXXXX"
			want := full[len(full)-16:]
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		}
	}
}

func TestWriteInputEcho(t *testing.T) {
	// Optional path used by advanced feature; keep core coverage light.
	mgr := NewManager(t.TempDir())
	s, err := mgr.Start(StartOpts{Command: []string{"cat"}})
	if err != nil {
		t.Fatalf("Start cat: %v", err)
	}
	defer func() { _ = s.Stop(time.Second) }()

	msg := "hello-pty\n"
	if _, err := s.WriteString(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.Output(), "hello-pty") {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("did not see echo; out=%q", s.Output())
}

func TestManagerStopUnknown(t *testing.T) {
	mgr := NewManager(t.TempDir())
	err := mgr.Stop("no-such", time.Second)
	if !IsNotFound(err) {
		t.Fatalf("err=%v want not found", err)
	}
}

// Detach path: worker + stop across processes.
func TestDetachStartStopAndExitCode(t *testing.T) {
	root := t.TempDir()
	cliPath := findCLIBinary(t)
	if cliPath == "" {
		// Fallback: in-process Manager already covers VAL-TERM-001..005.
		// Detached path covered when CLI binary exists.
		t.Skip("CLI binary not built; skipping detach integration")
	}

	t.Setenv(PersistRootEnv, root)
	// Start sleep loop via DetachStart using CLI as worker binary.
	meta, err := DetachStart(root, cliPath, StartOpts{
		Shell: "while true; do echo DETACH-MARK; sleep 0.3; done",
		ID:    "detach-1",
	})
	if err != nil {
		t.Fatalf("DetachStart: %v", err)
	}
	if meta.PID <= 0 {
		t.Fatalf("pid not set: %+v", meta)
	}
	if !ProcessAlive(meta.PID) {
		t.Fatalf("process not alive: %+v", meta)
	}

	// Wait for stream markers in out.log
	deadline := time.Now().Add(4 * time.Second)
	var logOut string
	for time.Now().Before(deadline) {
		logOut, _ = ReadOutputFile(root, meta.ID, 0)
		if strings.Contains(logOut, "DETACH-MARK") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(logOut, "DETACH-MARK") {
		t.Fatalf("detached output missing marker: %q", logOut)
	}

	stopped, err := DetachStop(root, meta.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("DetachStop: %v", err)
	}
	if ProcessAlive(meta.PID) {
		t.Fatalf("pid still alive after stop: %d", meta.PID)
	}
	if stopped.Status != StatusStopped && stopped.Status != StatusExited {
		t.Fatalf("status=%s", stopped.Status)
	}

	// Exit code path via detach: exit 7
	meta2, err := DetachStart(root, cliPath, StartOpts{
		Shell: "echo done-detach; exit 7",
		ID:    "detach-exit7",
	})
	if err != nil {
		t.Fatalf("DetachStart exit7: %v", err)
	}
	deadline = time.Now().Add(4 * time.Second)
	var got SessionMeta
	for time.Now().Before(deadline) {
		got, _ = LoadMeta(root, meta2.ID)
		if got.ExitCode != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got.ExitCode == nil || *got.ExitCode != 7 {
		t.Fatalf("exit code want 7, got %+v; out=%q", got, mustRead(root, meta2.ID))
	}
}

func mustRead(root, id string) string {
	s, _ := ReadOutputFile(root, id, 0)
	return s
}

func findCLIBinary(t *testing.T) string {
	t.Helper()
	// Prefer repo bin/inferenesia
	candidates := []string{
		filepath.Join("..", "..", "bin", "inferenesia"),
	}
	if wd, err := os.Getwd(); err == nil {
		// walk up to module root
		dir := wd
		for i := 0; i < 6; i++ {
			candidates = append(candidates, filepath.Join(dir, "bin", "inferenesia"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}
