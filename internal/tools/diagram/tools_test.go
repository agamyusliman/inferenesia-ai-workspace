package diagram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func newTools(t *testing.T) (*Tools, *writegate.Gateway, string) {
	t.Helper()
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return NewTools(gw), gw, root
}

// VAL-DIAG-002: create_diagram persists source to docs/diagrams/<slug>.mmd
// through WriteGateway; returned path is the saved file path; undo restores
// prior state.
func TestCreateDiagramSavesUnderDocsDiagrams(t *testing.T) {
	tools, gw, root := newTools(t)
	src := "flowchart TD\n  A[Start] --> B[End]"

	res := tools.Execute(testCtx(), Call{
		ID:        "c1",
		Name:      NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "auth-flow", "format": "mermaid", "source": src}),
	})
	if res.IsError {
		t.Fatalf("create_diagram error: %s", res.Content)
	}
	rel := "docs/diagrams/auth-flow.mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("file not saved: %v", err)
	}
	if string(data) != src {
		t.Fatalf("content mismatch: got %q", data)
	}
	if !strings.Contains(res.Content, rel) {
		t.Fatalf("result should include saved path %q: %q", rel, res.Content)
	}
	if !strings.Contains(res.Content, "WriteGateway") {
		t.Fatalf("result should mention WriteGateway: %q", res.Content)
	}
	// WriteGateway snapshot pushed.
	if gw.UndoDepth() != 1 {
		t.Fatalf("want undo depth 1, got %d", gw.UndoDepth())
	}
	// Undo restores prior state (file did not exist → removed).
	u := gw.Undo()
	if u.Noop {
		t.Fatalf("undo noop: %s", u.Message)
	}
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("undo should remove the created diagram")
	}
}

// VAL-DIAG-002: creating a diagram where one already existed restores prior
// content on undo (pre-agent dirty, not HEAD).
func TestCreateDiagramUndoRestoresPriorContent(t *testing.T) {
	tools, gw, root := newTools(t)
	rel := "docs/diagrams/flow.mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	prior := "flowchart TD\n  A --> B"
	if err := os.WriteFile(abs, []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}
	updated := "flowchart TD\n  A --> B --> C"
	res := tools.Execute(testCtx(), Call{
		ID:        "c1",
		Name:      NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "flow", "format": "mermaid", "source": updated}),
	})
	if res.IsError {
		t.Fatalf("create_diagram error: %s", res.Content)
	}
	if gw.UndoDepth() != 1 {
		t.Fatalf("undo depth=%d", gw.UndoDepth())
	}
	u := gw.Undo()
	if u.Noop {
		t.Fatal(u.Message)
	}
	got, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != prior {
		t.Fatalf("undo should restore prior content, got %q want %q", got, prior)
	}
}

// VAL-DIAG-003: update_diagram pushes a second snapshot; undo/redo traverse.
func TestUpdateDiagramUndoRedoTraversesEdits(t *testing.T) {
	tools, gw, root := newTools(t)
	v1 := "flowchart TD\n  A --> B"
	v2 := "flowchart TD\n  A --> B --> C"
	rel := "docs/diagrams/seq.mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))

	// Create v1.
	if res := tools.Execute(testCtx(), Call{
		ID: "c1", Name: NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "seq", "format": "mermaid", "source": v1}),
	}); res.IsError {
		t.Fatal(res.Content)
	}
	// Update to v2.
	if res := tools.Execute(testCtx(), Call{
		ID: "u1", Name: NameUpdateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "seq", "source": v2}),
	}); res.IsError {
		t.Fatal(res.Content)
	}
	if gw.UndoDepth() != 2 {
		t.Fatalf("want 2 undo units, got %d", gw.UndoDepth())
	}
	// Undo update → v1.
	gw.Undo()
	if got, _ := os.ReadFile(abs); string(got) != v1 {
		t.Fatalf("after undo want v1, got %q", got)
	}
	// Redo → v2.
	gw.Redo()
	if got, _ := os.ReadFile(abs); string(got) != v2 {
		t.Fatalf("after redo want v2, got %q", got)
	}
	// Undo again → v1, then undo create → absent.
	gw.Undo()
	if got, _ := os.ReadFile(abs); string(got) != v1 {
		t.Fatalf("second undo want v1, got %q", got)
	}
	gw.Undo()
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("final undo should remove the file (pre-create absent)")
	}
}

// VAL-DIAG-003: update_diagram via explicit path.
func TestUpdateDiagramByPath(t *testing.T) {
	tools, _, root := newTools(t)
	rel := "docs/diagrams/er.mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("erDiagram\n  USER ||--o{ POST : has"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := tools.Execute(testCtx(), Call{
		ID: "u1", Name: NameUpdateDiagram,
		Arguments: mustJSON(t, map[string]any{"path": rel, "source": "erDiagram\n  USER ||--o{ POST : has\n  POST ||--o{ COMMENT : has"}),
	})
	if res.IsError {
		t.Fatalf("update by path: %s", res.Content)
	}
	got, _ := os.ReadFile(abs)
	if !strings.Contains(string(got), "COMMENT") {
		t.Fatalf("update did not apply: %q", got)
	}
}

// VAL-DIAG-005: erDiagram accepted; unknown syntax returns parse error.
func TestParseErDiagramAccepted(t *testing.T) {
	src := "erDiagram\n  USER ||--o{ POST : has"
	res, err := Parse(FormatMermaid, src)
	if err != nil {
		t.Fatalf("erDiagram parse error: %v", err)
	}
	if res.DiagramType != "erDiagram" {
		t.Fatalf("diagram type=%s", res.DiagramType)
	}
}

func TestParseUnknownSyntaxReturnsError(t *testing.T) {
	_, err := Parse(FormatMermaid, "not-a-real-diagram-type TD\n  A --> B")
	if err == nil {
		t.Fatal("expected parse error for unknown diagram type")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("error should mention parse error: %v", err)
	}
}

func TestParseEmptySourceReturnsError(t *testing.T) {
	if _, err := Parse(FormatMermaid, ""); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestParseDBMLAccepted(t *testing.T) {
	src := "Table users {\n  id integer [primary key]\n}"
	res, err := Parse(FormatDBML, src)
	if err != nil {
		t.Fatalf("dbml parse error: %v", err)
	}
	if res.DiagramType != "dbml" {
		t.Fatalf("type=%s", res.DiagramType)
	}
}

func TestParsePlantUMLShape(t *testing.T) {
	good := "@startuml\nactor U\n@enduml"
	if _, err := Parse(FormatPlantUML, good); err != nil {
		t.Fatalf("plantuml parse: %v", err)
	}
	bad := "actor U\n@enduml"
	if _, err := Parse(FormatPlantUML, bad); err == nil {
		t.Fatal("expected parse error for plantuml without @startuml")
	}
}

// VAL-DIAG-005: create_diagram rejects unknown syntax before writing (no file
// created, no undo pushed).
func TestCreateDiagramRejectsUnknownSyntax(t *testing.T) {
	tools, gw, root := newTools(t)
	res := tools.Execute(testCtx(), Call{
		ID: "c1", Name: NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "bad", "format": "mermaid", "source": "nope TD"}),
	})
	if !res.IsError {
		t.Fatal("expected parse error")
	}
	if gw.UndoDepth() != 0 {
		t.Fatalf("no snapshot should be pushed, depth=%d", gw.UndoDepth())
	}
	abs := filepath.Join(root, "docs", "diagrams", "bad.mmd")
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("file must not be created on parse error")
	}
}

// VAL-DIAG-006: PlantUML unavailable returns clear error and does not block
// Mermaid/ER paths.
func TestPlantUMLUnavailableError(t *testing.T) {
	// Force unavailable by clearing env (resolver checks PATH/jar).
	t.Setenv("PLANTUML_JAR", "")
	r, err := ResolvePlantUML()
	if err == nil && r.Available() {
		t.Skip("plantuml installed on this machine; skip unavailable path")
	}
	if err == nil {
		t.Fatal("expected ErrPlantUMLUnavailable")
	}
	if !strings.Contains(err.Error(), "plantuml unavailable") {
		t.Fatalf("error should name plantuml: %v", err)
	}
}

func TestMermaidNotBlockedByPlantUML(t *testing.T) {
	tools, _, root := newTools(t)
	// Even if plantuml is unavailable, mermaid create must succeed.
	src := "erDiagram\n  USER ||--o{ POST : has"
	res := tools.Execute(testCtx(), Call{
		ID: "c1", Name: NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "er", "format": "mermaid", "source": src}),
	})
	if res.IsError {
		t.Fatalf("mermaid create should not be blocked by plantuml: %s", res.Content)
	}
	abs := filepath.Join(root, "docs", "diagrams", "er.mmd")
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("mermaid file not saved: %v", err)
	}
}

// VAL-DIAG-008: index file created and updated on create.
func TestCreateDiagramUpdatesIndex(t *testing.T) {
	tools, _, root := newTools(t)
	res := tools.Execute(testCtx(), Call{
		ID: "c1", Name: NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "auth-flow", "format": "mermaid", "source": "flowchart TD\n  A --> B", "title": "Auth Flow"}),
	})
	if res.IsError {
		t.Fatal(res.Content)
	}
	idx := filepath.Join(root, "docs", "diagrams", "README.md")
	data, err := os.ReadFile(idx)
	if err != nil {
		t.Fatalf("index not created: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "auth-flow") {
		t.Fatalf("index missing slug: %s", body)
	}
	if !strings.Contains(body, "Auth Flow") {
		t.Fatalf("index missing title: %s", body)
	}
	if !strings.Contains(body, "Inferenesia") {
		t.Fatalf("index should use Inferenesia brand: %s", body)
	}
}

func TestIndexUpdateIdempotent(t *testing.T) {
	tools, _, root := newTools(t)
	for i := 0; i < 2; i++ {
		if res := tools.Execute(testCtx(), Call{
			ID: "c", Name: NameCreateDiagram,
			Arguments: mustJSON(t, map[string]any{"slug": "x", "format": "mermaid", "source": "flowchart TD\n  A --> B"}),
		}); res.IsError {
			t.Fatal(res.Content)
		}
	}
	idx := filepath.Join(root, "docs", "diagrams", "README.md")
	data, _ := os.ReadFile(idx)
	if cnt := strings.Count(string(data), "docs/diagrams/x.mmd"); cnt != 1 {
		t.Fatalf("index should list x.mmd once, got %d", cnt)
	}
}

// VAL-DIAG-004: export writes to a workspace-sandboxed path under docs/diagrams.
func TestExportMermaidSVGPlaceholder(t *testing.T) {
	tools, _, root := newTools(t)
	// Create a diagram first.
	if res := tools.Execute(testCtx(), Call{
		ID: "c", Name: NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "flow", "format": "mermaid", "source": "flowchart TD\n  A --> B"}),
	}); res.IsError {
		t.Fatal(res.Content)
	}
	res := tools.Execute(testCtx(), Call{
		ID: "e", Name: NameExportDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "flow", "format": "svg"}),
	})
	if res.IsError {
		t.Fatalf("export svg: %s", res.Content)
	}
	out := filepath.Join(root, "docs", "diagrams", "flow.svg")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("export produced empty file")
	}
	if !strings.HasPrefix(string(data), "<?xml") && !strings.Contains(string(data), "<svg") {
		t.Fatalf("export not SVG: %s", data[:40])
	}
}

func TestExportRejectsOutsideWorkspace(t *testing.T) {
	tools, _, _ := newTools(t)
	// Create a diagram first.
	if res := tools.Execute(testCtx(), Call{
		ID: "c", Name: NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "flow", "format": "mermaid", "source": "flowchart TD\n  A --> B"}),
	}); res.IsError {
		t.Fatal(res.Content)
	}
	// Pass an explicit outside path via the path arg.
	res := tools.Execute(testCtx(), Call{
		ID: "e", Name: NameExportDiagram,
		Arguments: mustJSON(t, map[string]any{"path": "/tmp/outside-diagram.svg", "format": "svg"}),
	})
	if !res.IsError {
		t.Fatalf("export outside workspace should fail: %s", res.Content)
	}
}

func TestExportPlantUMLUnavailableClearError(t *testing.T) {
	t.Setenv("PLANTUML_JAR", "")
	r, _ := ResolvePlantUML()
	if r != nil && r.Available() {
		t.Skip("plantuml installed; skip")
	}
	tools, _, root := newTools(t)
	// Create a plantuml source directly (bypass parser by writing file).
	rel := "docs/diagrams/u.puml"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("@startuml\nactor U\n@enduml"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := tools.Execute(testCtx(), Call{
		ID: "e", Name: NameExportDiagram,
		Arguments: mustJSON(t, map[string]any{"path": rel, "format": "svg"}),
	})
	if !res.IsError {
		t.Fatal("expected plantuml unavailable error")
	}
	if !strings.Contains(res.Content, "plantuml unavailable") {
		t.Fatalf("error should name plantuml: %s", res.Content)
	}
}

// Slug validation.
func TestCreateDiagramRejectsBadSlug(t *testing.T) {
	tools, _, _ := newTools(t)
	for _, bad := range []string{"", "UPPER", "with space", "dot.dot", "-leading"} {
		res := tools.Execute(testCtx(), Call{
			ID: "c", Name: NameCreateDiagram,
			Arguments: mustJSON(t, map[string]any{"slug": bad, "format": "mermaid", "source": "flowchart TD\n  A --> B"}),
		})
		if !res.IsError {
			t.Fatalf("slug %q should be rejected", bad)
		}
	}
}

// Definitions include all three tools when wired.
func TestDefinitionsIncludeAllTools(t *testing.T) {
	tools, _, _ := newTools(t)
	defs := tools.Definitions()
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, n := range []string{NameCreateDiagram, NameUpdateDiagram, NameExportDiagram} {
		if !names[n] {
			t.Fatalf("missing tool def %q", n)
		}
	}
}

func TestDefinitionsNilWhenUnwired(t *testing.T) {
	tools := NewTools(nil)
	if defs := tools.Definitions(); len(defs) != 0 {
		t.Fatalf("nil gateway should yield no defs, got %d", len(defs))
	}
}

func TestUpdateDiagramNonexistentReturnsError(t *testing.T) {
	tools, _, _ := newTools(t)
	res := tools.Execute(testCtx(), Call{
		ID: "u", Name: NameUpdateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "missing", "source": "flowchart TD\n  A --> B"}),
	})
	if !res.IsError {
		t.Fatal("expected error for missing diagram")
	}
}
