package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/tools"
)

func TestTerminalStartWriteReadStopIndependent(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	sub := filepath.Join(root, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	// Open at workspace root.
	s1, err := svc.TerminalStart(TerminalStartRequest{WorkspaceID: ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if s1.ID == "" {
		t.Fatal("empty session id")
	}
	if filepath.Clean(s1.Cwd) != filepath.Clean(root) {
		t.Fatalf("cwd = %q want %q", s1.Cwd, root)
	}

	// Open in nested folder (Open in Integrated Terminal).
	s2, err := svc.TerminalStart(TerminalStartRequest{
		WorkspaceID: ws.ID,
		Cwd:         "nested",
		Title:       "nested",
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(s2.Cwd) != filepath.Clean(sub) {
		t.Fatalf("nested cwd = %q want %q", s2.Cwd, sub)
	}
	if s1.ID == s2.ID {
		t.Fatal("sessions must have distinct ids")
	}

	// Run echo in s1.
	if err := svc.TerminalWrite(TerminalWriteRequest{ID: s1.ID, Data: "echo hi-from-s1\n"}); err != nil {
		t.Fatal(err)
	}
	// Wait for output.
	var out1 string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, err := svc.TerminalRead(s1.ID)
		if err != nil {
			t.Fatal(err)
		}
		out1 = r.Output
		if strings.Contains(out1, "hi-from-s1") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(out1, "hi-from-s1") {
		t.Fatalf("s1 output missing hi-from-s1: %q", out1)
	}

	// List has both.
	list := svc.TerminalList()
	if len(list) < 2 {
		t.Fatalf("list len = %d want ≥2", len(list))
	}

	// Stop s1 only — s2 remains.
	stop, err := svc.TerminalStop(s1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stop.Stopped && !strings.Contains(stop.Message, "already") {
		// process may already have exited if shell dies; still OK if removed safely
		t.Logf("stop message: %s", stop.Message)
	}

	// s2 still writable.
	if err := svc.TerminalWrite(TerminalWriteRequest{ID: s2.ID, Data: "echo still-alive\n"}); err != nil {
		t.Fatalf("s2 write after s1 stop: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	var out2 string
	for time.Now().Before(deadline) {
		r, err := svc.TerminalRead(s2.ID)
		if err != nil {
			t.Fatal(err)
		}
		out2 = r.Output
		if strings.Contains(out2, "still-alive") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(out2, "still-alive") {
		t.Fatalf("s2 output missing still-alive: %q", out2)
	}

	_, _ = svc.TerminalStop(s2.ID)
}

func TestTerminalHTTPStartListWriteRead(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	// start
	body, _ := json.Marshal(map[string]string{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/terminal/start", bytes.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("start %d %s", rr.Code, rr.Body.String())
	}
	var sess TerminalSessionView
	if err := json.Unmarshal(rr.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Fatal("empty id")
	}
	if filepath.Clean(sess.Cwd) != filepath.Clean(root) {
		t.Fatalf("http cwd = %q want %q", sess.Cwd, root)
	}

	// write echo
	wb, _ := json.Marshal(map[string]string{"id": sess.ID, "data": "echo http-hi\n"})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/terminal/write", bytes.NewReader(wb)))
	if rr.Code != 200 {
		t.Fatalf("write %d %s", rr.Code, rr.Body.String())
	}

	// poll read
	var got string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/terminal/read?id="+sess.ID, nil))
		if rr.Code != 200 {
			t.Fatalf("read %d %s", rr.Code, rr.Body.String())
		}
		var res TerminalReadResult
		_ = json.Unmarshal(rr.Body.Bytes(), &res)
		got = res.Output
		if strings.Contains(got, "http-hi") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(got, "http-hi") {
		t.Fatalf("output missing http-hi: %q", got)
	}

	// list
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/terminal/list", nil))
	if rr.Code != 200 {
		t.Fatalf("list %d", rr.Code)
	}

	// stop
	sb, _ := json.Marshal(map[string]string{"id": sess.ID})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/terminal/stop", bytes.NewReader(sb)))
	if rr.Code != 200 {
		t.Fatalf("stop %d %s", rr.Code, rr.Body.String())
	}
}

// TestAgentToolsCanReadDesktopPanelSession proves VAL-DESK-011: a long-running
// process started from the desktop terminal panel is listable and tail-readable
// via the same in-process PTY manager that ChatStream wires into tools.
func TestAgentToolsCanReadDesktopPanelSession(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}

	// User opens integrated terminal panel (interactive shell).
	sess, err := svc.TerminalStart(TerminalStartRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = svc.TerminalStop(sess.ID) }()

	// Run a recipe/long-running-style command that emits a unique marker.
	marker := "DESK011-PANEL-LOG-MARKER"
	cmd := "echo " + marker + "; for i in 1 2 3; do echo LOG-$i; sleep 0.05; done\n"
	if err := svc.TerminalWrite(TerminalWriteRequest{ID: sess.ID, Data: cmd}); err != nil {
		t.Fatal(err)
	}

	// Wait until panel captures the marker (live logs).
	deadline := time.Now().Add(4 * time.Second)
	var panelOut string
	for time.Now().Before(deadline) {
		r, err := svc.TerminalRead(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		panelOut = r.Output
		if strings.Contains(panelOut, marker) {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !strings.Contains(panelOut, marker) {
		t.Fatalf("panel logs missing marker %q: %q", marker, panelOut)
	}

	// Agent tools share ensurePTY() — same path as ChatStream after VAL-DESK-011 fix.
	reg := tools.NewRegistry(nil)
	reg.SetTerminalBackend(tools.NewManagerTerminal(svc.ensurePTY(), ""))

	listRes := reg.Execute(context.Background(), tools.Call{
		ID:        "c1",
		Name:      tools.NameTerminalList,
		Arguments: `{}`,
	}, nil)
	if listRes.IsError {
		t.Fatalf("terminal_list error: %s", listRes.Content)
	}
	if !strings.Contains(listRes.Content, sess.ID) {
		t.Fatalf("terminal_list missing panel session %s: %s", sess.ID, listRes.Content)
	}

	readArgs, _ := json.Marshal(map[string]any{
		"session_id": sess.ID,
		"max_bytes":  65536,
	})
	readRes := reg.Execute(context.Background(), tools.Call{
		ID:        "c2",
		Name:      tools.NameTerminalRead,
		Arguments: string(readArgs),
	}, nil)
	if readRes.IsError {
		t.Fatalf("terminal_read error: %s", readRes.Content)
	}
	if !strings.Contains(readRes.Content, marker) {
		t.Fatalf("agent terminal_read missing panel marker %q: %s", marker, readRes.Content)
	}
	if !strings.Contains(readRes.Content, "LOG-") {
		t.Fatalf("agent terminal_read missing streamed log lines: %s", readRes.Content)
	}
}
