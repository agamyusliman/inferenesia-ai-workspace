package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/pty"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-TERM-007: agent terminal_read tool returns PTY output markers.
func TestTerminalReadToolReturnsOutput(t *testing.T) {
	root := t.TempDir()
	t.Setenv(pty.PersistRootEnv, filepath.Join(root, "pty"))

	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	mgr := pty.NewManager(root)
	reg.SetTerminalBackend(NewManagerTerminal(mgr, filepath.Join(root, "pty")))

	marker := "DEV-LOG-MARKER-42"
	startRes := reg.Execute(context.Background(), Call{
		ID:   "c1",
		Name: NameTerminalStart,
		Arguments: `{"command":"while true; do echo ` + marker + `; sleep 0.1; done","id":"dev1"}`,
	}, nil)
	if startRes.IsError {
		t.Fatalf("start: %s", startRes.Content)
	}
	if !strings.Contains(startRes.Content, "session=dev1") {
		t.Fatalf("start content: %s", startRes.Content)
	}
	defer func() {
		_ = reg.Execute(context.Background(), Call{
			ID: "cstop", Name: NameTerminalStop,
			Arguments: `{"session_id":"dev1"}`,
		}, nil)
	}()

	var readRes Result
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		readRes = reg.Execute(context.Background(), Call{
			ID:   "c2",
			Name: NameTerminalRead,
			Arguments: `{"session_id":"dev1"}`,
		}, nil)
		if !readRes.IsError && strings.Contains(readRes.Content, marker) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if readRes.IsError {
		t.Fatalf("read error: %s", readRes.Content)
	}
	if !strings.Contains(readRes.Content, marker) {
		t.Fatalf("read missing marker: %s", readRes.Content)
	}
	if !strings.Contains(readRes.Content, "status=") || !strings.Contains(readRes.Content, "pid=") {
		t.Fatalf("read missing status/pid: %s", readRes.Content)
	}
}

// VAL-TERM-008 via tool: write input, read echo.
func TestTerminalWriteToolEchoes(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	mgr := pty.NewManager(root)
	reg.SetTerminalBackend(NewManagerTerminal(mgr, ""))

	startRes := reg.Execute(context.Background(), Call{
		ID: "c1", Name: NameTerminalStart,
		Arguments: `{"command":"cat","id":"cat1"}`,
	}, nil)
	// ManagerTerminal Start uses Command via Shell — cat via shell string "cat" works.
	if startRes.IsError {
		// Start uses Shell: opts.Shell = cmd. "cat" is fine.
		t.Fatalf("start: %s", startRes.Content)
	}
	defer func() {
		_ = reg.Execute(context.Background(), Call{
			ID: "cx", Name: NameTerminalStop, Arguments: `{"session_id":"cat1"}`,
		}, nil)
	}()

	// Wait until running.
	time.Sleep(50 * time.Millisecond)
	w := reg.Execute(context.Background(), Call{
		ID: "c2", Name: NameTerminalWrite,
		Arguments: `{"session_id":"cat1","data":"hello-pty\n"}`,
	}, nil)
	if w.IsError {
		t.Fatalf("write: %s", w.Content)
	}

	deadline := time.Now().Add(3 * time.Second)
	var readRes Result
	for time.Now().Before(deadline) {
		readRes = reg.Execute(context.Background(), Call{
			ID: "c3", Name: NameTerminalRead,
			Arguments: `{"session_id":"cat1"}`,
		}, nil)
		if strings.Contains(readRes.Content, "hello-pty") {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("no echo: %s", readRes.Content)
}

func TestTerminalListEmptyAndStopNoop(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	mgr := pty.NewManager(root)
	reg.SetTerminalBackend(NewManagerTerminal(mgr, ""))

	list := reg.Execute(context.Background(), Call{
		ID: "l1", Name: NameTerminalList, Arguments: `{}`,
	}, nil)
	if list.IsError {
		t.Fatal(list.Content)
	}
	if !strings.Contains(list.Content, "no sessions") {
		t.Fatalf("want empty: %s", list.Content)
	}

	// Unknown stop is safe no-op.
	stop := reg.Execute(context.Background(), Call{
		ID: "s1", Name: NameTerminalStop,
		Arguments: `{"session_id":"ghost"}`,
	}, nil)
	if stop.IsError {
		t.Fatalf("stop ghost should not error: %s", stop.Content)
	}
	if !strings.Contains(stop.Content, "not found") {
		t.Fatalf("want not found message: %s", stop.Content)
	}
}

func TestTerminalDefsIncluded(t *testing.T) {
	reg := NewRegistry(nil)
	names := map[string]bool{}
	for _, d := range reg.Definitions() {
		names[d.Function.Name] = true
	}
	for _, want := range []string{
		NameTerminalRead, NameTerminalWrite, NameTerminalList,
		NameTerminalStart, NameTerminalStop,
	} {
		if !names[want] {
			t.Fatalf("missing tool definition %s", want)
		}
	}
}
