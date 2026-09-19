package diagram

import (
	"strings"
	"testing"
)

// VAL-DIAG-007: Mermaid -> Excalidraw conversion preserves node/edge count on
// a basic fixture. This is the core contract assertion.
func TestConvertMermaidToExcalidrawPreservesNodeEdgeCount(t *testing.T) {
	src := "flowchart TD\n  A[Start] --> B[Process]\n  B --> C[End]"
	g, err := ParseMermaidFlowchart(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("want 3 nodes, got %d (%+v)", len(g.Nodes), g.Nodes)
	}
	if len(g.Edges) != 2 {
		t.Fatalf("want 2 edges, got %d (%+v)", len(g.Edges), g.Edges)
	}
	doc, err := ConvertMermaidToExcalidraw(src)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	s := SummarizeExcalidraw(doc)
	if s.Nodes != len(g.Nodes) {
		t.Fatalf("node count not preserved: mermaid=%d excalidraw=%d", len(g.Nodes), s.Nodes)
	}
	if s.Edges != len(g.Edges) {
		t.Fatalf("edge count not preserved: mermaid=%d excalidraw=%d", len(g.Edges), s.Edges)
	}
	// Every node label round-trips as a text element bound to its shape.
	labels := map[string]bool{}
	for _, el := range doc.Elements {
		if el.Type == "text" && el.Text != "" && el.ContainerID != "" {
			labels[el.Text] = true
		}
	}
	for _, n := range g.Nodes {
		if !labels[n.Label] {
			t.Fatalf("node label %q missing from Excalidraw text elements", n.Label)
		}
	}
}

// VAL-DIAG-007: edge labels are preserved on the arrow.
func TestConvertMermaidToExcalidrawPreservesEdgeLabels(t *testing.T) {
	src := "flowchart LR\n  A -->|yes| B"
	doc, err := ConvertMermaidToExcalidraw(src)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	found := false
	for _, el := range doc.Elements {
		if el.Type == "text" && el.Text == "yes" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("edge label 'yes' not preserved")
	}
}

// VAL-DIAG-007: unsupported Mermaid types return a clear error (no corruption).
func TestConvertRejectsNonFlowchart(t *testing.T) {
	cases := []string{
		"sequenceDiagram\n  A->>B: hi",
		"erDiagram\n  USER ||--o{ POST : has",
		"stateDiagram-v2\n  [*] --> A",
	}
	for _, src := range cases {
		_, err := ConvertMermaidToExcalidraw(src)
		if err == nil {
			t.Fatalf("expected error for %q", src)
		}
		if !strings.Contains(err.Error(), "flowchart/graph only") {
			t.Fatalf("error should mention flowchart/graph: %v", err)
		}
	}
}

// VAL-DIAG-007: the produced Excalidraw document round-trips through
// ParseExcalidraw (so the editor can re-open the saved source).
func TestConvertProducesParseableExcalidraw(t *testing.T) {
	src := "flowchart LR\n  A[One] --> B[Two]\n  B --> C[Three]\n  A --> C"
	doc, err := ConvertMermaidToExcalidraw(src)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	out, err := SerializeExcalidraw(doc)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if _, err := ParseExcalidraw(out); err != nil {
		t.Fatalf("produced source is not a valid Excalidraw doc: %v", err)
	}
}

// VAL-DIAG-007: node shapes map to Excalidraw shape types.
func TestConvertNodeShapeMapping(t *testing.T) {
	src := "flowchart TD\n  A[Rect] --> B(Round)\n  B --> C{Decision}"
	doc, err := ConvertMermaidToExcalidraw(src)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	wantShapes := map[string]string{"A": "rectangle", "B": "ellipse", "C": "diamond"}
	gotShapes := map[string]string{}
	for _, el := range doc.Elements {
		// Shape ids look like node-<id>-<index>.
		if strings.HasPrefix(el.ID, "node-") {
			// Extract id between "node-" and "-<index>".
			rest := strings.TrimPrefix(el.ID, "node-")
			// rest is "<id>-<index>"; strip trailing index.
			if i := strings.LastIndex(rest, "-"); i > 0 {
				gotShapes[rest[:i]] = el.Type
			}
		}
	}
	for id, want := range wantShapes {
		if gotShapes[id] != want {
			t.Fatalf("node %q shape: want %q got %q", id, want, gotShapes[id])
		}
	}
}

// VAL-DIAG-007: chained edges (A --> B --> C) are preserved.
func TestConvertChainedEdges(t *testing.T) {
	src := "flowchart TD\n  A --> B --> C"
	doc, err := ConvertMermaidToExcalidraw(src)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	s := SummarizeExcalidraw(doc)
	if s.Nodes != 3 {
		t.Fatalf("want 3 nodes, got %d", s.Nodes)
	}
	if s.Edges != 2 {
		t.Fatalf("want 2 edges (chained), got %d", s.Edges)
	}
}

// VAL-DIAG-007: dotted and thick edges are tagged (style preserved as metadata).
func TestConvertEdgeStyles(t *testing.T) {
	src := "flowchart TD\n  A -.-> B\n  B ==> C"
	g, err := ParseMermaidFlowchart(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(g.Edges) != 2 {
		t.Fatalf("want 2 edges, got %d", len(g.Edges))
	}
	if g.Edges[0].Style != "dotted" {
		t.Fatalf("edge 0 style: want dotted, got %q", g.Edges[0].Style)
	}
	if g.Edges[1].Style != "thick" {
		t.Fatalf("edge 1 style: want thick, got %q", g.Edges[1].Style)
	}
}

// VAL-DIAG-007: direction is parsed (TD/LR/RL/BT).
func TestParseMermaidFlowchartDirection(t *testing.T) {
	cases := map[string]string{
		"flowchart TD\n  A --> B": "TD",
		"flowchart LR\n  A --> B": "LR",
		"flowchart RL\n  A --> B": "RL",
		"flowchart BT\n  A --> B": "BT",
		"graph TB\n  A --> B":    "TB",
	}
	for src, want := range cases {
		g, err := ParseMermaidFlowchart(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if g.Direction != want {
			t.Fatalf("direction for %q: want %q got %q", src, want, g.Direction)
		}
	}
}

// VAL-DIAG-007: comments, subgraph, class, style lines are skipped (no spurious nodes).
func TestParseMermaidFlowchartSkipsMetaLines(t *testing.T) {
	src := `flowchart TD
  %% a comment
  A --> B
  subgraph group
  end
  class A red
  style A fill:#f00
  B --> C`
	g, err := ParseMermaidFlowchart(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("want 3 real nodes (A,B,C), got %d", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Fatalf("want 2 edges, got %d", len(g.Edges))
	}
}
