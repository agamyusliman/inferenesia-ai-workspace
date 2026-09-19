package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestWriteFileToolCreatesFile(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)

	var events []Event
	res := reg.Execute(context.Background(), Call{
		ID:   "call_1",
		Name: NameWriteFile,
		Arguments: `{"path":"hello.txt","content":"hi"}`,
	}, func(e Event) { events = append(events, e) })

	if res.IsError {
		t.Fatalf("result error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "hello.txt") {
		t.Fatalf("content=%q", res.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hi" {
		t.Fatalf("file=%q", data)
	}
	// Events: start + end
	if len(events) < 2 {
		t.Fatalf("events=%d", len(events))
	}
	if events[0].Type != EventStart {
		t.Fatalf("first event %v", events[0].Type)
	}
	if events[len(events)-1].Type != EventEnd {
		t.Fatalf("last event %v", events[len(events)-1].Type)
	}
}

func TestWriteFileToolSandboxDenial(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)

	outside := filepath.Join(os.TempDir(), "yura-tool-outside-"+filepath.Base(root), "x.txt")
	var events []Event
	res := reg.Execute(context.Background(), Call{
		ID:   "call_2",
		Name: NameWriteFile,
		Arguments: `{"path":"` + filepath.ToSlash(outside) + `","content":"nope"}`,
	}, func(e Event) { events = append(events, e) })

	if !res.IsError {
		t.Fatalf("expected error result, got %q", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "sandbox") {
		t.Fatalf("expected sandbox denial message, got %q", res.Content)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("outside file must not exist")
	}
	foundErrEv := false
	for _, e := range events {
		if e.Type == EventError {
			foundErrEv = true
		}
	}
	if !foundErrEv {
		t.Fatal("expected tool_error event")
	}
}

func TestShellToolEcho(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)

	res := reg.Execute(context.Background(), Call{
		ID:        "call_3",
		Name:      NameShell,
		Arguments: `{"command":"echo tool-ok"}`,
	}, nil)

	if res.IsError {
		t.Fatalf("shell error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "tool-ok") {
		t.Fatalf("content=%q", res.Content)
	}
	if !strings.Contains(res.Content, "exit_code: 0") {
		t.Fatalf("missing exit code: %q", res.Content)
	}
}

func TestDefinitionsIncludeWriteAndShell(t *testing.T) {
	reg := NewRegistry(nil)
	defs := reg.Definitions()
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Function.Name] = true
	}
	if !names[NameWriteFile] || !names[NameShell] {
		t.Fatalf("defs names missing: %+v", names)
	}
	if !names[NameWriteMultibrain] || !names[NameWriteDiagram] {
		t.Fatalf("mutation tools missing (VAL-UNDO-007): %+v", names)
	}
}

// VAL-UNDO-007: multibrain + diagram tools go through WriteGateway and are undoable.
func TestMultibrainAndDiagramToolsRouteThroughGateway(t *testing.T) {
	root := t.TempDir()
	mbBefore := filepath.Join(root, ".multibrain", "indexes", "agents.md")
	dgBefore := filepath.Join(root, "docs", "diagrams", "flow.mmd")
	if err := os.MkdirAll(filepath.Dir(mbBefore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dgBefore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mbBefore, []byte("MB-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dgBefore, []byte("DG-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)

	// Multibrain mutation.
	resMB := reg.Execute(context.Background(), Call{
		ID:        "mb1",
		Name:      NameWriteMultibrain,
		Arguments: `{"path":".multibrain/indexes/agents.md","content":"MB-AFTER"}`,
	}, nil)
	if resMB.IsError {
		t.Fatalf("multibrain write error: %s", resMB.Content)
	}
	if !strings.Contains(resMB.Content, "WriteGateway") || !strings.Contains(resMB.Content, "multibrain") {
		t.Fatalf("expected WriteGateway/multibrain ack: %q", resMB.Content)
	}

	// Diagram mutation.
	resDG := reg.Execute(context.Background(), Call{
		ID:        "dg1",
		Name:      NameWriteDiagram,
		Arguments: `{"path":"docs/diagrams/flow.mmd","content":"DG-AFTER"}`,
	}, nil)
	if resDG.IsError {
		t.Fatalf("diagram write error: %s", resDG.Content)
	}
	if !strings.Contains(resDG.Content, "WriteGateway") || !strings.Contains(resDG.Content, "diagram") {
		t.Fatalf("expected WriteGateway/diagram ack: %q", resDG.Content)
	}

	// FS mutation for three-source stack.
	resFS := reg.Execute(context.Background(), Call{
		ID:        "fs1",
		Name:      NameWriteFile,
		Arguments: `{"path":"app.txt","content":"FS-AFTER"}`,
	}, nil)
	if resFS.IsError {
		t.Fatalf("fs write error: %s", resFS.Content)
	}

	if gw.UndoDepth() != 3 {
		t.Fatalf("want 3 undo units for fs+multibrain+diagram, depth=%d", gw.UndoDepth())
	}

	// Undo stack listing: one entry per mutation.
	stack := gw.UndoStack()
	if len(stack) != 3 {
		t.Fatalf("stack units=%d", len(stack))
	}
	// Undo three times restores prior content.
	// Order of undos: last written first = fs (app.txt), then diagram, then multibrain.
	u1 := gw.Undo()
	if u1.Noop {
		t.Fatal(u1.Message)
	}
	// app.txt was new → removed or empty before; just ensure second undos restore MB/DG.
	u2 := gw.Undo()
	if u2.Noop || string(mustRead(t, dgBefore)) != "DG-BEFORE" {
		t.Fatalf("undo diagram: content=%q msg=%s", mustRead(t, dgBefore), u2.Message)
	}
	u3 := gw.Undo()
	if u3.Noop || string(mustRead(t, mbBefore)) != "MB-BEFORE" {
		t.Fatalf("undo multibrain: content=%q msg=%s", mustRead(t, mbBefore), u3.Message)
	}
}

func TestWriteMultibrainToolRejectsNonMultibrainPath(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	res := reg.Execute(context.Background(), Call{
		ID:        "bad",
		Name:      NameWriteMultibrain,
		Arguments: `{"path":"not-mb.txt","content":"x"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected error, got %q", res.Content)
	}
	if gw.UndoDepth() != 0 {
		t.Fatal("rejected write must not push undo")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
