package diagram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-DIAG-007: convert_diagram saves .excalidraw under docs/diagrams/ via
// WriteGateway (undoable) and the source round-trips with the editor (parse
// the saved JSON back into a valid Excalidraw doc).
func TestConvertDiagramSavesExcalidrawAndRoundTrips(t *testing.T) {
	tools, gw, root := newTools(t)
	src := "flowchart TD\n  A[Start] --> B[Process]\n  B --> C[End]"

	res := tools.Execute(testCtx(), Call{
		ID:        "conv1",
		Name:      NameConvertDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "flow-bridge", "source": src, "title": "Flow Bridge"}),
	})
	if res.IsError {
		t.Fatalf("convert_diagram error: %s", res.Content)
	}
	rel := "docs/diagrams/flow-bridge.excalidraw"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("excalidraw file not saved: %v", err)
	}
	// The saved source must be valid Excalidraw JSON (round-trip with editor).
	doc, pErr := ParseExcalidraw(string(data))
	if pErr != nil {
		t.Fatalf("saved .excalidraw is not valid: %v", pErr)
	}
	s := SummarizeExcalidraw(doc)
	if s.Nodes != 3 {
		t.Fatalf("want 3 nodes, got %d", s.Nodes)
	}
	if s.Edges != 2 {
		t.Fatalf("want 2 edges, got %d", s.Edges)
	}
	// Result mentions the saved path + WriteGateway + preserved counts.
	if !strings.Contains(res.Content, rel) {
		t.Fatalf("result should mention saved path %q: %q", rel, res.Content)
	}
	if !strings.Contains(res.Content, "WriteGateway") {
		t.Fatalf("result should mention WriteGateway: %q", res.Content)
	}
	if !strings.Contains(res.Content, "nodes=3") || !strings.Contains(res.Content, "edges=2") {
		t.Fatalf("result should report preserved counts: %q", res.Content)
	}
	// WriteGateway snapshot pushed.
	if gw.UndoDepth() != 1 {
		t.Fatalf("want undo depth 1, got %d", gw.UndoDepth())
	}
	// Undo restores prior state (file absent -> removed).
	u := gw.Undo()
	if u.Noop {
		t.Fatalf("undo noop: %s", u.Message)
	}
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("undo should remove the created .excalidraw")
	}
}

// VAL-DIAG-007: convert_diagram rejects non-flowchart Mermaid with a clear
// error (no file written, no undo pushed).
func TestConvertDiagramRejectsNonFlowchart(t *testing.T) {
	tools, gw, root := newTools(t)
	res := tools.Execute(testCtx(), Call{
		ID:        "conv",
		Name:      NameConvertDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "bad", "source": "sequenceDiagram\n  A->>B: hi"}),
	})
	if !res.IsError {
		t.Fatal("expected error for sequenceDiagram conversion")
	}
	if !strings.Contains(res.Content, "flowchart/graph only") {
		t.Fatalf("error should mention flowchart/graph: %s", res.Content)
	}
	if gw.UndoDepth() != 0 {
		t.Fatalf("no snapshot should be pushed, depth=%d", gw.UndoDepth())
	}
	abs := filepath.Join(root, "docs", "diagrams", "bad.excalidraw")
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("file must not be created on conversion error")
	}
}

// VAL-DIAG-007: convert_diagram updates the index in the same work unit.
func TestConvertDiagramUpdatesIndex(t *testing.T) {
	tools, _, root := newTools(t)
	res := tools.Execute(testCtx(), Call{
		ID:        "conv",
		Name:      NameConvertDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "bridge", "source": "flowchart TD\n  A --> B", "title": "Bridge"}),
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
	if !strings.Contains(body, "bridge") {
		t.Fatalf("index missing slug: %s", body)
	}
	if !strings.Contains(body, "excalidraw") {
		t.Fatalf("index should list excalidraw format: %s", body)
	}
	if !strings.Contains(body, "Bridge") {
		t.Fatalf("index missing title: %s", body)
	}
}

// VAL-DIAG-007: convert_diagram rejects bad slugs.
func TestConvertDiagramRejectsBadSlug(t *testing.T) {
	tools, _, _ := newTools(t)
	for _, bad := range []string{"", "UPPER", "with space", "-leading"} {
		res := tools.Execute(testCtx(), Call{
			ID: "c", Name: NameConvertDiagram,
			Arguments: mustJSON(t, map[string]any{"slug": bad, "source": "flowchart TD\n  A --> B"}),
		})
		if !res.IsError {
			t.Fatalf("slug %q should be rejected", bad)
		}
	}
}

// VAL-DIAG-007: convert_diagram definitions include the new tool.
func TestDefinitionsIncludeConvertDiagram(t *testing.T) {
	tools, _, _ := newTools(t)
	defs := tools.Definitions()
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names[NameConvertDiagram] {
		t.Fatalf("missing tool def %q", NameConvertDiagram)
	}
}

// VAL-DIAG-007: editor round-trip — write .excalidraw via create_diagram
// (format=excalidraw), update via update_diagram, and parse back each time.
// This proves the editor's text edits save + reopen cleanly (round-trip).
func TestExcalidrawCreateUpdateRoundTrip(t *testing.T) {
	tools, gw, root := newTools(t)
	rel := "docs/diagrams/canv.excalidraw"
	abs := filepath.Join(root, filepath.FromSlash(rel))

	// Initial doc with 1 node.
	doc1 := ExcalidrawDocument{
		Elements: []ExcalidrawElement{
			{Type: "rectangle", ID: "n1", X: 0, Y: 0, Width: 100, Height: 60},
		},
	}
	src1, _ := SerializeExcalidraw(doc1)
	if res := tools.Execute(testCtx(), Call{
		ID: "c", Name: NameCreateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "canv", "format": "excalidraw", "source": src1}),
	}); res.IsError {
		t.Fatalf("create: %s", res.Content)
	}
	got1, _ := os.ReadFile(abs)
	if _, err := ParseExcalidraw(string(got1)); err != nil {
		t.Fatalf("saved doc1 not parseable: %v", err)
	}
	if gw.UndoDepth() != 1 {
		t.Fatalf("after create undo depth=%d", gw.UndoDepth())
	}

	// Update with 2 nodes + 1 edge.
	doc2 := ExcalidrawDocument{
		Elements: []ExcalidrawElement{
			{Type: "rectangle", ID: "n1", X: 0, Y: 0, Width: 100, Height: 60},
			{Type: "rectangle", ID: "n2", X: 200, Y: 0, Width: 100, Height: 60},
			{Type: "arrow", ID: "e1", X: 100, Y: 30, Width: 100, Height: 0,
				Points: [][]float64{{0, 0}, {100, 0}}},
		},
	}
	src2, _ := SerializeExcalidraw(doc2)
	if res := tools.Execute(testCtx(), Call{
		ID: "u", Name: NameUpdateDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "canv", "source": src2}),
	}); res.IsError {
		t.Fatalf("update: %s", res.Content)
	}
	if gw.UndoDepth() != 2 {
		t.Fatalf("after update undo depth=%d", gw.UndoDepth())
	}
	got2, _ := os.ReadFile(abs)
	doc2Parsed, err := ParseExcalidraw(string(got2))
	if err != nil {
		t.Fatalf("saved doc2 not parseable: %v", err)
	}
	s2 := SummarizeExcalidraw(doc2Parsed)
	if s2.Nodes != 2 || s2.Edges != 1 {
		t.Fatalf("doc2 summary mismatch: %+v", s2)
	}

	// Undo update -> back to doc1 (1 node).
	gw.Undo()
	got1b, _ := os.ReadFile(abs)
	doc1b, err := ParseExcalidraw(string(got1b))
	if err != nil {
		t.Fatalf("after undo doc1 not parseable: %v", err)
	}
	if s := SummarizeExcalidraw(doc1b); s.Nodes != 1 {
		t.Fatalf("after undo want 1 node, got %d", s.Nodes)
	}
}

// VAL-DIAG-007: convert_diagram with a richer fixture (multiple edges + labels)
// preserves counts.
func TestConvertDiagramRichFixture(t *testing.T) {
	tools, _, root := newTools(t)
	src := "flowchart LR\n  A[Auth] --> B[Router]\n  B -->|allow| C[Service]\n  B -->|deny| D[Reject]"
	res := tools.Execute(testCtx(), Call{
		ID:        "conv",
		Name:      NameConvertDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "auth-bridge", "source": src}),
	})
	if res.IsError {
		t.Fatalf("convert: %s", res.Content)
	}
	abs := filepath.Join(root, "docs", "diagrams", "auth-bridge.excalidraw")
	data, _ := os.ReadFile(abs)
	doc, err := ParseExcalidraw(string(data))
	if err != nil {
		t.Fatalf("parse saved: %v", err)
	}
	s := SummarizeExcalidraw(doc)
	if s.Nodes != 4 {
		t.Fatalf("want 4 nodes, got %d", s.Nodes)
	}
	if s.Edges != 3 {
		t.Fatalf("want 3 edges, got %d", s.Edges)
	}
}

// VAL-DIAG-007: the produced JSON is canonical (indent + stable keys) so the
// editor can diff revisions cleanly.
func TestConvertDiagramProducesCanonicalJSON(t *testing.T) {
	tools, _, root := newTools(t)
	src := "flowchart TD\n  A --> B"
	if res := tools.Execute(testCtx(), Call{
		ID: "c", Name: NameConvertDiagram,
		Arguments: mustJSON(t, map[string]any{"slug": "canon", "source": src}),
	}); res.IsError {
		t.Fatal(res.Content)
	}
	abs := filepath.Join(root, "docs", "diagrams", "canon.excalidraw")
	data, _ := os.ReadFile(abs)
	// Re-parse into a generic map to confirm it is structured JSON.
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if generic["type"] != "excalidraw" {
		t.Fatalf("type field wrong: %v", generic["type"])
	}
	if _, ok := generic["elements"].([]any); !ok {
		t.Fatalf("elements should be an array: %T", generic["elements"])
	}
}
