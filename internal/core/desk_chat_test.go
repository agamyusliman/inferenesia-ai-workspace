package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestUndoToolbarLabelsAndPreAgentDirty(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	// Dirty pre-agent content (not HEAD).
	path := filepath.Join(ws, "dirty.txt")
	if err := os.WriteFile(path, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	// Simulate agent write via WriteGateway + persist (as chat would).
	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("t1")
	if _, err := g.WriteFile("dirty.txt", []byte("AFTER-AGENT")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()
	stackPath := writegate.PersistPath(home, ws)
	if err := g.SaveStack(stackPath); err != nil {
		t.Fatal(err)
	}

	st, err := svc.GetUndoStack()
	if err != nil {
		t.Fatal(err)
	}
	if !st.CanUndo {
		t.Fatal("expected can_undo after agent write")
	}
	if st.RevertAgentLabel != LabelRevertAgent {
		t.Fatalf("Revert agent label = %q", st.RevertAgentLabel)
	}
	if st.RestoreToHeadLabel != LabelRestoreToHead {
		t.Fatalf("Restore to HEAD label = %q", st.RestoreToHeadLabel)
	}
	if st.RevertAgentLabel == st.RestoreToHeadLabel {
		t.Fatal("labels must differ")
	}

	// Undo via Service restores DIRTY-BEFORE (byte-identical).
	res, err := svc.UndoLast()
	if err != nil {
		t.Fatal(err)
	}
	if res.Noop {
		t.Fatalf("undo noop: %s", res.Message)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "DIRTY-BEFORE" {
		t.Fatalf("after undo got %q want DIRTY-BEFORE", got)
	}

	// Redo re-applies AFTER-AGENT.
	res, err = svc.RedoLast()
	if err != nil {
		t.Fatal(err)
	}
	if res.Noop {
		t.Fatalf("redo noop: %s", res.Message)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "AFTER-AGENT" {
		t.Fatalf("after redo got %q want AFTER-AGENT", got)
	}
}

func TestSettingsPanelSections(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	view := svc.GetSettings("")
	if view.Product != "Inferenesia" {
		t.Fatalf("product = %q", view.Product)
	}
	ids := map[string]bool{}
	for _, s := range view.Sections {
		ids[s.ID] = true
	}
	for _, want := range []string{"profiles", "models", "workspace"} {
		if !ids[want] {
			t.Fatalf("missing section %s in %#v", want, view.Sections)
		}
	}
	if view.Labels.RevertAgent != LabelRevertAgent {
		t.Fatalf("settings label revert = %q", view.Labels.RevertAgent)
	}
	if view.Labels.RestoreToHead != LabelRestoreToHead {
		t.Fatalf("settings label head = %q", view.Labels.RestoreToHead)
	}
}

func TestMentionsAndHTTPHandlers(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "hello.txt"), "hi\n")
	mustMkdir(t, filepath.Join(ws, "pkg"))
	mustWrite(t, filepath.Join(ws, "pkg", "a.go"), "package pkg\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	files, err := svc.ListMentionFiles("hello", 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if f.Path == "hello.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("mention list missing hello.txt: %#v", files)
	}

	h := svc.Handler()

	// undo stack labels via HTTP
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/undo/stack", nil))
	if rr.Code != 200 {
		t.Fatalf("undo stack %d %s", rr.Code, rr.Body.String())
	}
	var stack UndoStackState
	if err := json.Unmarshal(rr.Body.Bytes(), &stack); err != nil {
		t.Fatal(err)
	}
	if stack.RevertAgentLabel != "Revert agent" || stack.RestoreToHeadLabel != "Restore to HEAD" {
		t.Fatalf("labels = %#v", stack)
	}

	// settings
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rr.Code != 200 {
		t.Fatalf("settings %d %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"id":"profiles"`)) {
		t.Fatalf("settings missing profiles section: %s", rr.Body.String())
	}

	// mentions
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/mentions?q=a.go", nil))
	if rr.Code != 200 {
		t.Fatalf("mentions %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "pkg/a.go") && !strings.Contains(rr.Body.String(), "a.go") {
		t.Fatalf("mentions body: %s", rr.Body.String())
	}
}

func TestBuildUserMessageMentions(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "note.md"), "context-body\n")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	msg, err := svc.buildUserMessage(ws, "explain this", []string{"note.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "explain this") {
		t.Fatalf("missing prompt: %s", msg)
	}
	if !strings.Contains(msg, "@note.md") || !strings.Contains(msg, "context-body") {
		t.Fatalf("missing mention body: %s", msg)
	}
}
