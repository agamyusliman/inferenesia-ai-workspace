package diagram

import (
	"fmt"
	"strings"
)

// Mermaid -> Excalidraw converter (VAL-DIAG-007 bridge).
//
// The converter produces a minimal but valid Excalidraw document whose element
// list mirrors the Mermaid flowchart's nodes and edges: one shape per node,
// one arrow per edge. It is deliberately simple (not a pixel-faithful port)
// because the contract only requires preserving node/edge semantics on a
// basic fixture. A future D5 canvas surface can open this source in the
// Excalidraw editor for further editing.
//
// Supported Mermaid input:
//   - flowchart / graph with TD/LR/RL/BT direction
//   - nodes: `id`, `id[Label]`, `id(Label)`, `id{Label}`, `id([Label])`
//   - edges: `A --> B`, `A -->|text| B`, `A --- B`, `A -.-> B`, `A ==> B`
//
// Unsupported diagram types (sequenceDiagram, erDiagram, etc.) return a clear
// error rather than a corrupt conversion. The round-trip property tested is:
// Parse(FormatMermaid, src) and SummarizeExcalidraw(ConvertMermaidToExcalidraw(src))
// agree on node/edge counts.

// MermaidGraph holds the parsed node/edge model of a Mermaid flowchart.
type MermaidGraph struct {
	Direction string
	Nodes     []MermaidNode
	Edges     []MermaidEdge
}

// MermaidNode is one flowchart node.
type MermaidNode struct {
	ID    string
	Label string
	Shape string // rectangle | ellipse | diamond | stadium
}

// MermaidEdge is one flowchart edge.
type MermaidEdge struct {
	From  string
	To    string
	Label string
	Style string // solid | dotted | thick
}

// ParseMermaidFlowchart extracts nodes + edges from a Mermaid flowchart/graph
// source. It is a tolerant parser: it recognizes the common subset and skips
// lines it cannot parse (subgraph/styling/class) without failing. Unknown
// diagram types return an error (the converter refuses to corrupt them).
func ParseMermaidFlowchart(source string) (MermaidGraph, error) {
	src := strings.TrimSpace(source)
	if src == "" {
		return MermaidGraph{}, &ParseError{Format: FormatMermaid, Detail: "empty mermaid source"}
	}
	lines := strings.Split(src, "\n")
	if len(lines) == 0 {
		return MermaidGraph{}, &ParseError{Format: FormatMermaid, Detail: "empty mermaid source"}
	}
	first := strings.TrimSpace(lines[0])
	keyword := mermaidKeyword(first)
	if keyword != "flowchart" && keyword != "graph" {
		return MermaidGraph{}, &ParseError{
			Format: FormatMermaid,
			Detail: "converter supports flowchart/graph only (got " + quote(keyword) + "); sequenceDiagram/erDiagram/etc. are not converted to Excalidraw",
		}
	}
	// Direction: second token of first line (TD/LR/RL/BT), default TD.
	direction := "TD"
	if parts := strings.Fields(first); len(parts) >= 2 {
		d := strings.ToUpper(parts[1])
		switch d {
		case "TD", "TB", "LR", "RL", "BT":
			direction = d
		}
	}

	g := MermaidGraph{Direction: direction}
	nodeByID := map[string]*MermaidNode{}
	addNode := func(id, label, shape string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := nodeByID[id]; ok {
			return
		}
		if label == "" {
			label = id
		}
		n := MermaidNode{ID: id, Label: label, Shape: shape}
		g.Nodes = append(g.Nodes, n)
		nodeByID[id] = &g.Nodes[len(g.Nodes)-1]
	}

	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "%%") || strings.HasPrefix(line, "---") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "subgraph") || strings.HasPrefix(lower, "end") ||
			strings.HasPrefix(lower, "class") || strings.HasPrefix(lower, "style") ||
			strings.HasPrefix(lower, "linkstyle") || strings.HasPrefix(lower, "click") {
			continue
		}
		// Split on edge connectors. We support a few shapes.
		parsed, ok := parseFlowchartLine(line)
		if !ok {
			continue
		}
		// Ensure endpoints exist.
		for _, n := range parsed.nodes {
			addNode(n.ID, n.Label, n.Shape)
		}
		g.Edges = append(g.Edges, parsed.edges...)
	}
	return g, nil
}

type flowchartLine struct {
	nodes []MermaidNode
	edges []MermaidEdge
}

// parseFlowchartLine parses one flowchart body line into nodes + edges.
// It supports the common `A --> B` family and `A[Label]` / `A(Label)` /
// `A{Label}` / `A([Label])` node shapes. Chained edges (A --> B --> C) are
// handled by scanning left-to-right: parse a node decl, then if a connector
// follows, record an edge whose `To` is the next node decl, and repeat.
// Returns ok=false when the line is neither an edge nor a node declaration.
func parseFlowchartLine(line string) (flowchartLine, bool) {
	var out flowchartLine
	s := strings.TrimSpace(line)
	if s == "" {
		return out, false
	}
	// Parse the leading node decl.
	node, rest, ok := parseLeadingNodeDecl(s)
	if !ok {
		return out, false
	}
	out.nodes = append(out.nodes, node)
	lastID := node.ID
	for {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			// Standalone node decl; ok if we parsed at least one node.
			return out, len(out.nodes) > 0
		}
		// Expect a connector next.
		conn, label, cstart, cend, found := nextEdgeConnector(rest)
		if !found {
			// Leftover text that is not a connector: tolerate by stopping here
			// (we already have at least one node, so this line is a node decl).
			return out, len(out.nodes) > 0
		}
		_ = cstart
		// Record an edge whose `To` is resolved by the next node decl.
		edge := MermaidEdge{
			From:  lastID,
			To:    "",
			Label: label,
			Style: edgeStyle(conn),
		}
		// Trim the connector off the front of `rest`.
		rest = strings.TrimSpace(rest[cend:])
		// Parse the next leading node decl.
		n2, rest2, ok2 := parseLeadingNodeDecl(rest)
		if !ok2 {
			// Connector with no right-hand node: treat the edge as dangling
			// (skip it) to avoid corrupting counts.
			return out, len(out.edges) > 0 || len(out.nodes) > 0
		}
		edge.To = n2.ID
		out.edges = append(out.edges, edge)
		out.nodes = append(out.nodes, n2)
		lastID = n2.ID
		rest = rest2
	}
}

// parseLeadingNodeDecl parses a node declaration from the start of s.
// Returns the node, the remainder of s after the node decl (including any
// trailing edge connector), and ok=false if no leading id is found.
// Node shapes: A, A[Label], A[[Label]], A(Label), A((Label)), A([Label]),
// A{Label}. The remainder starts where the shape ends.
func parseLeadingNodeDecl(s string) (MermaidNode, string, bool) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return MermaidNode{}, "", false
	}
	// Leading id: alphanumeric/underscore/dash.
	i := 0
	for i < len(s) && isIDByte(s[i]) {
		i++
	}
	if i == 0 {
		return MermaidNode{}, s, false
	}
	id := s[:i]
	rest := s[i:]
	// No shape bracket: bare node id.
	if rest == "" || rest[0] == ' ' || rest[0] == '\t' {
		return MermaidNode{ID: id, Label: id, Shape: "rectangle"}, rest, true
	}
	// Try bracketed shapes. We match the longest opener first.
	type opener struct {
		open, close string
		shape       string
	}
	openers := []opener{
		{"[[", "]]", "rectangle"},
		{"[(", ")]", "ellipse"},
		{"([", "])", "stadium"},
		{"((", "))", "ellipse"},
		{"[", "]", "rectangle"},
		{"(", ")", "ellipse"},
		{"{", "}", "diamond"},
	}
	for _, op := range openers {
		if strings.HasPrefix(rest, op.open) {
			inner := rest[len(op.open):]
			closeIdx := strings.Index(inner, op.close)
			if closeIdx < 0 {
				// No closer: treat as bare id (tolerate).
				return MermaidNode{ID: id, Label: id, Shape: "rectangle"}, rest, true
			}
			label := strings.TrimSpace(inner[:closeIdx])
			remainder := inner[closeIdx+len(op.close):]
			return MermaidNode{ID: id, Label: label, Shape: op.shape}, remainder, true
		}
	}
	// No recognized opener: bare node id.
	return MermaidNode{ID: id, Label: id, Shape: "rectangle"}, rest, true
}

// nextEdgeConnector finds the first edge connector in s.
// Returns connector, label, start index, end index (exclusive), ok.
// Start/end indices cover the whole connector + optional label so the caller
// can split left/right cleanly.
func nextEdgeConnector(s string) (conn, label string, start, end int, ok bool) {
	// Connectors (longest first to avoid prefix clashes):
	//   -->|text|  -->  ===>  ==>  -.->  ---  -.-  --
	// We scan for each, respecting that `-->` must not match inside `--->`.
	type cand struct {
		conn  string
		label bool
	}
	cands := []cand{
		{"-->", false},
		{"-.->", false},
		{"==>", false},
		{"===>", false},
		{"---", false},
		{"-.-", false},
	}
	// Prefer longer connectors when indices tie.
	bestStart := len(s)
	bestIdx := -1
	for i, c := range cands {
		idx := strings.Index(s, c.conn)
		if idx < 0 {
			continue
		}
		if idx < bestStart || (idx == bestStart && len(c.conn) > len(cands[bestIdx].conn)) {
			bestStart = idx
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return "", "", 0, 0, false
	}
	chosen := cands[bestIdx]
	start = bestStart
	end = bestStart + len(chosen.conn)
	conn = chosen.conn
	// Edge label forms: -->|text|  or  -.->|text|
	if end < len(s) && s[end] == '|' {
		closeIdx := strings.IndexByte(s[end+1:], '|')
		if closeIdx >= 0 {
			label = strings.TrimSpace(s[end+1 : end+1+closeIdx])
			end = end + 1 + closeIdx + 1
		}
	}
	return conn, label, start, end, true
}

func isIDByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func edgeStyle(conn string) string {
	switch conn {
	case "==>", "===>":
		return "thick"
	case "-.->", "-.-":
		return "dotted"
	default:
		return "solid"
	}
}

// ConvertMermaidToExcalidraw produces a valid Excalidraw document from a
// Mermaid flowchart source. It preserves node count (one shape per node) and
// edge count (one arrow per edge). Layout is a simple grid (TD) or row (LR).
func ConvertMermaidToExcalidraw(source string) (ExcalidrawDocument, error) {
	g, err := ParseMermaidFlowchart(source)
	if err != nil {
		return ExcalidrawDocument{}, err
	}
	return buildExcalidrawFromGraph(g), nil
}

// buildExcalidrawFromGraph lays out nodes in a grid and adds arrows for edges.
// Layout constants are conservative so shapes do not overlap.
func buildExcalidrawFromGraph(g MermaidGraph) ExcalidrawDocument {
	const (
		nodeW   = 160.0
		nodeH   = 60.0
		gapX    = 80.0
		gapY    = 80.0
		marginX = 80.0
		marginY = 80.0
		cols    = 3
	)
	doc := ExcalidrawDocument{
		Type:     "excalidraw",
		Version:  2,
		Source:   "inferenesia",
		AppState: map[string]any{"viewBackgroundColor": "#ffffff"},
		Files:    map[string]any{},
		Elements: nil,
	}
	// Place nodes.
	pos := map[string][2]float64{} // id -> (x, y)
	for i, n := range g.Nodes {
		col := i % cols
		row := i / cols
		x := marginX + float64(col)*(nodeW+gapX)
		y := marginY + float64(row)*(nodeH+gapY)
		pos[n.ID] = [2]float64{x, y}
		id := fmt.Sprintf("node-%s-%d", n.ID, i)
		shapeType := "rectangle"
		switch n.Shape {
		case "ellipse", "stadium":
			shapeType = "ellipse"
		case "diamond":
			shapeType = "diamond"
		}
		el := ExcalidrawElement{
			Type:   shapeType,
			ID:     id,
			X:      x,
			Y:      y,
			Width:  nodeW,
			Height: nodeH,
		}
		doc.Elements = append(doc.Elements, el)
		// Attach a text element inside the shape so the label round-trips.
		textID := "text-" + id
		doc.Elements = append(doc.Elements, ExcalidrawElement{
			Type:       "text",
			ID:         textID,
			X:          x + 8,
			Y:          y + nodeH/2 - 12,
			Width:      nodeW - 16,
			Height:     24,
			Text:       n.Label,
			ContainerID: id,
		})
	}
	// Place arrows for edges.
	for i, e := range g.Edges {
		from, ok1 := pos[e.From]
		to, ok2 := pos[e.To]
		if !ok1 || !ok2 {
			continue
		}
		x1 := from[0] + nodeW/2
		y1 := from[1] + nodeH/2
		x2 := to[0] + nodeW/2
		y2 := to[1] + nodeH/2
		id := fmt.Sprintf("edge-%d-%s-%s", i, e.From, e.To)
		arrow := ExcalidrawElement{
			Type:   "arrow",
			ID:     id,
			X:      x1,
			Y:      y1,
			Width:  x2 - x1,
			Height: y2 - y1,
			Points: [][]float64{{0, 0}, {x2 - x1, y2 - y1}},
			StartBinding: &ExcalidrawBinding{ElementID: "node-" + e.From + "-" + nodeIndex(g, e.From)},
			EndBinding:   &ExcalidrawBinding{ElementID: "node-" + e.To + "-" + nodeIndex(g, e.To)},
		}
		doc.Elements = append(doc.Elements, arrow)
		if e.Label != "" {
			mx := (x1 + x2) / 2
			my := (y1 + y2) / 2
			doc.Elements = append(doc.Elements, ExcalidrawElement{
				Type: "text",
				ID:   "label-" + id,
				X:    mx,
				Y:    my - 12,
				Text: e.Label,
			})
		}
	}
	return doc
}

// nodeIndex returns the index of node id in g.Nodes (for stable binding ids).
func nodeIndex(g MermaidGraph, id string) string {
	for i, n := range g.Nodes {
		if n.ID == id {
			return fmt.Sprintf("%d", i)
		}
	}
	return "0"
}
