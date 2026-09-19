package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitFileWatcherRoot waits until the async watch attach finished for want root.
func waitFileWatcherRoot(t *testing.T, svc *Service, want string) {
	t.Helper()
	wantAbs, err := filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if eval, err := filepath.EvalSymlinks(wantAbs); err == nil {
		wantAbs = eval
	}
	wantAbs = filepath.Clean(wantAbs)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		fw := svc.fileWatcher
		svc.mu.Unlock()
		if fw != nil {
			fw.mu.Lock()
			got, started := fw.root, fw.started && fw.w != nil
			fw.mu.Unlock()
			if started && got == wantAbs {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("file watcher root did not become %q within 3s", wantAbs)
}

// VAL-CROSS-003: a file write performed via the CLI agent (an external process
// separate from the desktop) must be visible in the desktop explorer panel via a
// pushed FileChanged event, not polling-only, within ~2s.
//
// This test simulates the CLI as an external os.WriteFile to the workspace root
// (the same disk mutation the CLI agent's write_file tool performs via
// WriteGateway), and verifies:
//   - The fsnotify watcher fires a FileChangeEvent with Source="external".
//   - The event is streamed over GET /api/fs/events (SSE).
//   - The event arrives within 2s (debounce + watcher latency).
//   - The event path matches the written file (workspace-relative).
func TestCrossArea_CLIWriteVisibleInDesktopExplorer_FileChangedEvent(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	waitFileWatcherRoot(t, svc, ws)

	// Subscribe to file change events BEFORE the external write.
	ch := svc.FileWatcherSubscribe()
	defer svc.FileWatcherUnsubscribe(ch)

	// Simulate the CLI agent writing a file (external process — does not go
	// through the desktop WriteGateway / suppress path).
	const relPath = "docs/cli-written.txt"
	const content = "CLI-AGENT-WROTE-THIS"
	abs := filepath.Join(ws, filepath.FromSlash(relPath))
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// The FileChanged event must arrive within 2s (VAL-CROSS-003 <=2s bound).
	// Debounce adds ~120ms; fsnotify latency is typically <50ms.
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("file event channel closed before event arrived")
		}
		if ev.Source != "external" {
			t.Fatalf("event Source = %q, want %q (CLI writes are external)",
				ev.Source, "external")
		}
		// Path should be the workspace-relative slash path.
		if !strings.HasSuffix(filepath.ToSlash(ev.Path), relPath) {
			t.Fatalf("event Path = %q, want suffix %q", ev.Path, relPath)
		}
		if ev.Removed {
			t.Fatalf("event Removed = true for a write, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FileChanged event did not arrive within 2s (VAL-CROSS-003 bound)")
	}
}

// VAL-CROSS-003 (SSE variant): the GET /api/fs/events SSE endpoint must stream
// FileChangeEvent values to a desktop subscriber when an external CLI write
// happens. This exercises the full HTTP path the renderer uses.
func TestCrossArea_FileEventsSSE_CLIWriteStreamsToSubscriber(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	waitFileWatcherRoot(t, svc, ws)

	handler := svc.Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// SSE client: GET /api/fs/events with context cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/fs/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Simulate CLI agent writing a file (external process).
	const relPath = "from-cli.txt"
	const content = "written-by-cli-agent"
	abs := filepath.Join(ws, relPath)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Read one SSE frame within 2s.
	type sseEvent struct {
		Op      string `json:"op"`
		Path    string `json:"path"`
		Source  string `json:"source"`
		Removed bool   `json:"removed"`
	}
	frame, err := readSSEFrame(resp.Body, 2*time.Second)
	if err != nil {
		t.Fatalf("did not receive SSE frame within 2s: %v (VAL-CROSS-003 push bound)", err)
	}
	var ev sseEvent
	if err := json.Unmarshal([]byte(frame), &ev); err != nil {
		t.Fatalf("malformed SSE payload %q: %v", frame, err)
	}
	if ev.Source != "external" {
		t.Fatalf("SSE event Source = %q, want %q", ev.Source, "external")
	}
	if !strings.HasSuffix(ev.Path, relPath) {
		t.Fatalf("SSE event Path = %q, want suffix %q", ev.Path, relPath)
	}
	if ev.Removed {
		t.Fatalf("SSE event Removed = true for a write, want false")
	}
}

// VAL-CROSS-003 (suppress variant): desktop WriteGateway mutations must NOT
// double-fire a FileChanged event on the /api/fs/events stream (the chat SSE
// already emitted one). Only external writes (CLI/OS) should appear.
func TestCrossArea_DesktopWriteGatewayMutationSuppressedFromFileEvents(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	waitFileWatcherRoot(t, svc, ws)

	ch := svc.FileWatcherSubscribe()
	defer svc.FileWatcherUnsubscribe(ch)

	// Desktop WriteGateway mutation (SaveFile = user edit). The suppress window
	// should prevent the fsnotify echo from reaching subscribers.
	const relPath = "desktop-saved.txt"
	const content = "saved-from-desktop"
	if _, err := svc.SaveFile("", relPath, content); err != nil {
		t.Fatal(err)
	}

	// Wait beyond the debounce window (120ms) + suppress window (400ms).
	// No event should arrive for the suppressed desktop mutation.
	select {
	case ev, ok := <-ch:
		// Tolerate events for unrelated paths (e.g. config home writes), but
		// fail if the event is for our desktop-saved file.
		if ok && strings.HasSuffix(filepath.ToSlash(ev.Path), relPath) {
			t.Fatalf("desktop WriteGateway mutation should be suppressed, got event: %+v", ev)
		}
		// Unrelated event — tolerate (could be config home mtime changes).
	case <-time.After(600 * time.Millisecond):
		// Good: no event arrived within suppress+debounce window.
	}
}

// VAL-CROSS-003 (remove variant): a CLI delete must fire a FileChanged event
// with Removed=true so the explorer can drop the node from the tree.
func TestCrossArea_CLIRemoveFiresRemovedEvent(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()

	// Pre-create a file so we can delete it externally.
	const relPath = "to-be-deleted.txt"
	abs := filepath.Join(ws, relPath)
	if err := os.WriteFile(abs, []byte("gone-soon"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	waitFileWatcherRoot(t, svc, ws)

	ch := svc.FileWatcherSubscribe()
	defer svc.FileWatcherUnsubscribe(ch)

	// Simulate CLI deleting the file.
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before remove event arrived")
		}
		if !ev.Removed {
			t.Fatalf("remove event Removed = false, want true: %+v", ev)
		}
		if ev.Source != "external" {
			t.Fatalf("remove event Source = %q, want %q", ev.Source, "external")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("remove FileChanged event did not arrive within 2s")
	}
}

// VAL-CROSS-003 (workspace switch): switching the active workspace must
// re-attach the watcher to the new root so writes in the new workspace fire
// events (not the old one).
func TestCrossArea_WorkspaceSwitchReattachesWatcher(t *testing.T) {
	home := t.TempDir()
	wsA := t.TempDir()
	wsB := t.TempDir()

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(wsA); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(wsB); err != nil {
		t.Fatal(err)
	}
	waitFileWatcherRoot(t, svc, wsB)

	ch := svc.FileWatcherSubscribe()
	defer svc.FileWatcherUnsubscribe(ch)

	// Write in workspace B (now active) — should fire.
	absB := filepath.Join(wsB, "ws-b-file.txt")
	if err := os.WriteFile(absB, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		if !strings.HasSuffix(filepath.ToSlash(ev.Path), "ws-b-file.txt") {
			t.Fatalf("expected event for ws-b-file.txt, got path=%q", ev.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write in active workspace B did not fire event within 2s")
	}

	// Write in workspace A (no longer active) — should NOT fire.
	absA := filepath.Join(wsA, "ws-a-file.txt")
	if err := os.WriteFile(absA, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		// Tolerate stray events not for ws-a-file.txt (config writes), but
		// fail if the event is for the inactive workspace A file.
		if strings.HasSuffix(filepath.ToSlash(ev.Path), "ws-a-file.txt") {
			t.Fatalf("write in inactive workspace A should not fire event: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		// Good: no event for inactive workspace A.
	}
}

// readSSEFrame reads one "data: <payload>\n\n" frame from r within timeout.
// Returns the payload (without the "data: " prefix).
func readSSEFrame(r io.Reader, timeout time.Duration) (string, error) {
	type result struct {
		payload string
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 0, 1024)
		tmp := make([]byte, 512)
		for {
			n, err := r.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				// Look for a complete SSE frame: "data: ...\n\n"
				for {
					idx := strings.Index(string(buf), "\n\n")
					if idx < 0 {
						break
					}
					frame := string(buf[:idx])
					buf = buf[idx+2:]
					// Extract payload from "data: <payload>".
					for _, line := range strings.Split(frame, "\n") {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "data:") {
							payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
							if payload != "" {
								ch <- result{payload: payload, err: nil}
								return
							}
						}
					}
				}
			}
			if err != nil {
				ch <- result{payload: "", err: err}
				return
			}
		}
	}()
	select {
	case res := <-ch:
		return res.payload, res.err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout after %s", timeout)
	}
}
