package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-DIAG-002: create_diagram via Registry routes through WriteGateway and
// saves under docs/diagrams/; undo restores prior state.
func TestRegistryCreateDiagramSavesViaGateway(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	src := "flowchart TD\n  A[Start] --> B[End]"
	res := reg.Execute(context.Background(), tools.Call{
		ID:        "c1",
		Name:      tools.NameCreateDiagram,
		Arguments: `{"slug":"auth-flow","format":"mermaid","source":"` + escapeNewlines(src) + `","title":"Auth Flow"}`,
	}, nil)
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
		t.Fatalf("result should mention saved path: %q", res.Content)
	}
	if gw.UndoDepth() != 1 {
		t.Fatalf("undo depth=%d", gw.UndoDepth())
	}
	// Undo restores prior state (file absent → removed).
	u := gw.Undo()
	if u.Noop {
		t.Fatal(u.Message)
	}
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("undo should remove created diagram")
	}
}

// VAL-DIAG-008: index updated in same work unit as create.
func TestRegistryCreateDiagramUpdatesIndex(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	res := reg.Execute(context.Background(), tools.Call{
		ID:        "c1",
		Name:      tools.NameCreateDiagram,
		Arguments: `{"slug":"er","format":"mermaid","source":"erDiagram\n  USER ||--o{ POST : has","title":"ER"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("create_diagram: %s", res.Content)
	}
	idx := filepath.Join(root, "docs", "diagrams", "README.md")
	data, err := os.ReadFile(idx)
	if err != nil {
		t.Fatalf("index not created: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "er") || !strings.Contains(body, "Inferenesia") {
		t.Fatalf("index missing entry/brand: %s", body)
	}
}

// VAL-DIAG-003: update_diagram via Registry pushes second snapshot; undo/redo.
func TestRegistryUpdateDiagramUndoRedo(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	v1 := "flowchart TD\n  A --> B"
	v2 := "flowchart TD\n  A --> B --> C"
	if res := reg.Execute(context.Background(), tools.Call{
		ID: "c", Name: tools.NameCreateDiagram,
		Arguments: `{"slug":"seq","format":"mermaid","source":"` + escapeNewlines(v1) + `"}`,
	}, nil); res.IsError {
		t.Fatal(res.Content)
	}
	if res := reg.Execute(context.Background(), tools.Call{
		ID: "u", Name: tools.NameUpdateDiagram,
		Arguments: `{"slug":"seq","source":"` + escapeNewlines(v2) + `"}`,
	}, nil); res.IsError {
		t.Fatal(res.Content)
	}
	if gw.UndoDepth() != 2 {
		t.Fatalf("want 2 undo units, got %d", gw.UndoDepth())
	}
	abs := filepath.Join(root, "docs", "diagrams", "seq.mmd")
	gw.Undo()
	if got, _ := os.ReadFile(abs); string(got) != v1 {
		t.Fatalf("after undo want v1, got %q", got)
	}
	gw.Redo()
	if got, _ := os.ReadFile(abs); string(got) != v2 {
		t.Fatalf("after redo want v2, got %q", got)
	}
}

// VAL-DIAG-004: export_diagram via Registry writes a sandboxed file.
func TestRegistryExportDiagramSVG(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	if res := reg.Execute(context.Background(), tools.Call{
		ID: "c", Name: tools.NameCreateDiagram,
		Arguments: `{"slug":"flow","format":"mermaid","source":"flowchart TD\n  A --> B"}`,
	}, nil); res.IsError {
		t.Fatal(res.Content)
	}
	res := reg.Execute(context.Background(), tools.Call{
		ID: "e", Name: tools.NameExportDiagram,
		Arguments: `{"slug":"flow","format":"svg"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("export: %s", res.Content)
	}
	out := filepath.Join(root, "docs", "diagrams", "flow.svg")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("export not written: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty export")
	}
}

// VAL-DIAG-005: unknown syntax returns a parse error via Registry.
func TestRegistryCreateDiagramParseError(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	res := reg.Execute(context.Background(), tools.Call{
		ID: "c", Name: tools.NameCreateDiagram,
		Arguments: `{"slug":"bad","format":"mermaid","source":"not-a-diagram-type TD"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(res.Content, "parse error") {
		t.Fatalf("error should mention parse error: %s", res.Content)
	}
	if gw.UndoDepth() != 0 {
		t.Fatalf("no snapshot on parse error, depth=%d", gw.UndoDepth())
	}
}

// Definitions include diagram tools when wired.
func TestRegistryDefinitionsIncludeDiagramTools(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	names := map[string]bool{}
	for _, d := range reg.Definitions() {
		names[d.Function.Name] = true
	}
	for _, n := range []string{tools.NameCreateDiagram, tools.NameUpdateDiagram, tools.NameExportDiagram, tools.NameConvertDiagram} {
		if !names[n] {
			t.Fatalf("missing diagram tool def %q", n)
		}
	}
}

// Definitions exclude diagram tools when not wired.
func TestRegistryDefinitionsExcludeDiagramToolsWhenUnwired(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	// No EnsureDiagramFromGateway call.
	for _, d := range reg.Definitions() {
		switch d.Function.Name {
		case tools.NameCreateDiagram, tools.NameUpdateDiagram, tools.NameExportDiagram, tools.NameConvertDiagram:
			t.Fatalf("diagram tool should not be advertised when unwired: %q", d.Function.Name)
		}
	}
}

// Plan Mode denies diagram tools (create/update/export are mutations).
func TestPlanModeDeniesDiagramTools(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	// Wire a plan mode controller and activate it.
	pm := plan.New()
	reg.SetPlanMode(pm)
	pm.Enter("test")
	defer pm.Exit("test")

	for _, name := range []string{tools.NameCreateDiagram, tools.NameUpdateDiagram, tools.NameExportDiagram, tools.NameConvertDiagram} {
		res := reg.Execute(context.Background(), tools.Call{
			ID: "p", Name: name,
			Arguments: `{"slug":"x","source":"flowchart TD\n  A --> B"}`,
		}, nil)
		if !res.IsError {
			t.Fatalf("plan mode should deny %s", name)
		}
		if !strings.Contains(res.Content, "plan mode") {
			t.Fatalf("denial should mention plan mode: %s", res.Content)
		}
	}
}

// Explore category denies diagram write tools (VAL-ORCH-008).
func TestExploreCategoryDeniesDiagramTools(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)
	reg.SetCategoryFilter("explore")

	for _, name := range []string{tools.NameCreateDiagram, tools.NameUpdateDiagram, tools.NameExportDiagram, tools.NameConvertDiagram} {
		res := reg.Execute(context.Background(), tools.Call{
			ID: "e", Name: name,
			Arguments: `{"slug":"x","source":"flowchart TD\n  A --> B"}`,
		}, nil)
		if !res.IsError {
			t.Fatalf("explore should deny %s", name)
		}
	}
	// And not advertised.
	for _, d := range reg.Definitions() {
		switch d.Function.Name {
		case tools.NameCreateDiagram, tools.NameUpdateDiagram, tools.NameExportDiagram, tools.NameConvertDiagram:
			t.Fatalf("explore defs should not include %q", d.Function.Name)
		}
	}
}

// Implement category allows diagram tools.
func TestImplementCategoryAllowsDiagramTools(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)
	reg.SetCategoryFilter("implement")

	res := reg.Execute(context.Background(), tools.Call{
		ID: "c", Name: tools.NameCreateDiagram,
		Arguments: `{"slug":"impl","format":"mermaid","source":"flowchart TD\n  A --> B"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("implement should allow create_diagram: %s", res.Content)
	}
}

// VAL-DIAG-007: convert_diagram via Registry saves .excalidraw under
// docs/diagrams/ through WriteGateway (undoable); node/edge counts preserved.
func TestRegistryConvertDiagramMermaidToExcalidraw(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	src := "flowchart TD\n  A[Start] --> B[End]"
	res := reg.Execute(context.Background(), tools.Call{
		ID:        "conv",
		Name:      tools.NameConvertDiagram,
		Arguments: `{"slug":"bridge","source":"` + escapeNewlines(src) + `","title":"Bridge"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("convert_diagram error: %s", res.Content)
	}
	rel := "docs/diagrams/bridge.excalidraw"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("file not saved: %v", err)
	}
	// Saved source must be valid JSON with type=excalidraw.
	body := string(data)
	if !strings.Contains(body, "excalidraw") {
		t.Fatalf("saved file does not look like Excalidraw JSON: %s", body[:80])
	}
	if gw.UndoDepth() != 1 {
		t.Fatalf("undo depth=%d", gw.UndoDepth())
	}
	if !strings.Contains(res.Content, "nodes=2") || !strings.Contains(res.Content, "edges=1") {
		t.Fatalf("result should report preserved counts: %s", res.Content)
	}
	// Undo restores prior state (file removed).
	u := gw.Undo()
	if u.Noop {
		t.Fatal(u.Message)
	}
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("undo should remove the created .excalidraw")
	}
}

func escapeNewlines(s string) string {
	return strings.ReplaceAll(s, "\n", `\n`)
}
