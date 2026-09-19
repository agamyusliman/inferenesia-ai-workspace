// Package diagram implements agent diagram tools (VAL-DIAG-001..008).
//
// create_diagram / update_diagram persist diagram source to
// docs/diagrams/<slug>.<ext> through WriteGateway (so undo applies, VAL-DIAG-002/003).
// export_diagram renders a saved diagram to PNG/SVG (optionally PDF) using the
// available renderer (mmdc / plantuml); export paths are workspace-sandboxed
// (VAL-DIAG-004). The parser accepts Mermaid (incl. erDiagram) and may accept
// DBML; unknown syntax returns a parse error (VAL-DIAG-005). PlantUML is
// optional: when the binary/jar is unavailable, requests return a clear
// "plantuml unavailable" error and never block Mermaid/ER paths (VAL-DIAG-006).
// Saved diagrams are linked from docs/diagrams/README.md in the same logical
// work unit as the create (VAL-DIAG-008).
package diagram

import (
	"strings"
)

// Format is the diagram source format.
type Format string

const (
	// FormatMermaid is Mermaid (.mmd). erDiagram is a Mermaid diagram type.
	FormatMermaid Format = "mermaid"
	// FormatDBML is DBML (.dbml). Optional; if unsupported, a clear error is returned.
	FormatDBML Format = "dbml"
	// FormatPlantUML is PlantUML (.puml). Optional; requires plantuml binary/jar.
	FormatPlantUML Format = "plantuml"
	// FormatExcalidraw is Excalidraw JSON (.excalidraw). Reserved for the bridge.
	FormatExcalidraw Format = "excalidraw"
)

// ParseError is returned for unknown/invalid diagram syntax (VAL-DIAG-005).
type ParseError struct {
	Format Format
	Detail string
}

func (e *ParseError) Error() string {
	if e == nil {
		return "diagram: parse error"
	}
	if e.Format != "" && e.Detail != "" {
		return "diagram: parse error (" + string(e.Format) + "): " + e.Detail
	}
	if e.Format != "" {
		return "diagram: parse error (" + string(e.Format) + ")"
	}
	if e.Detail != "" {
		return "diagram: parse error: " + e.Detail
	}
	return "diagram: parse error"
}

// Mermaid diagram types accepted by the parser (VAL-DIAG-005 erDiagram at minimum).
var mermaidDiagramTypes = map[string]bool{
	"flowchart":         true,
	"graph":             true,
	"sequenceDiagram":   true,
	"classDiagram":      true,
	"stateDiagram":      true,
	"stateDiagram-v2":   true,
	"erDiagram":         true,
	"journey":           true,
	"gantt":             true,
	"pie":               true,
	"mindmap":           true,
	"C4Context":         true,
	"gitGraph":          true,
	"requirementDiagram": true,
}

// Parse validates diagram source for the given format and returns a normalized
// ParseResult or a *ParseError for unknown/invalid syntax (VAL-DIAG-005).
//
// Mermaid (incl. erDiagram) is always supported. DBML is accepted (basic shape
// check) and may be supported by an external renderer. PlantUML source is
// accepted with a basic shape check; rendering requires the plantuml binary/jar
// (VAL-DIAG-006). Unknown formats return a parse error.
func Parse(format Format, source string) (ParseResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return ParseResult{}, &ParseError{Format: format, Detail: "empty diagram source"}
	}
	switch format {
	case FormatMermaid:
		return parseMermaid(source)
	case FormatDBML:
		return parseDBML(source)
	case FormatPlantUML:
		return parsePlantUML(source)
	case FormatExcalidraw:
		// Excalidraw bridge is deferred (VAL-DIAG-007). Accept raw JSON-ish source.
		return ParseResult{Format: format, DiagramType: "excalidraw"}, nil
	default:
		return ParseResult{}, &ParseError{Format: format, Detail: "unsupported format " + string(format)}
	}
}

// ParseResult is the outcome of a successful Parse.
type ParseResult struct {
	Format      Format
	DiagramType string // e.g. "erDiagram", "flowchart", "dbml", "plantuml"
	Source      string // trimmed source
}

func parseMermaid(source string) (ParseResult, error) {
	first := firstLine(source)
	// Mermaid diagrams start with a diagram type keyword on the first
	// non-comment line (e.g. "flowchart TD", "erDiagram", "sequenceDiagram").
	if first == "" {
		return ParseResult{}, &ParseError{Format: FormatMermaid, Detail: "missing diagram type (first line empty)"}
	}
	keyword := mermaidKeyword(first)
	if keyword == "" || !mermaidDiagramTypes[keyword] {
		return ParseResult{}, &ParseError{
			Format:  FormatMermaid,
			Detail:  "unknown or unsupported mermaid diagram type " + quote(keyword) + " (expected one of flowgraph, sequenceDiagram, erDiagram, classDiagram, stateDiagram-v2, etc.)",
		}
	}
	return ParseResult{Format: FormatMermaid, DiagramType: keyword, Source: source}, nil
}

// mermaidKeyword returns the leading mermaid diagram-type token from a first
// line, e.g. "flowchart" from "flowchart TD", "erDiagram" from "erDiagram".
func mermaidKeyword(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "---") // allow frontmatter-ish fences
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	// Some mermaid types have spaces? No: erDiagram, sequenceDiagram, etc. are single tokens.
	tok := line
	if i := strings.IndexAny(line, " \t"); i >= 0 {
		tok = line[:i]
	}
	return strings.TrimSpace(tok)
}

func parseDBML(source string) (ParseResult, error) {
	// Basic DBML shape: contains "Table" or "Ref" or "Enum" definitions.
	// We do not fully parse; we only confirm it looks like DBML so unknown
	// junk is rejected (VAL-DIAG-005).
	if !looksLikeDBML(source) {
		return ParseResult{}, &ParseError{Format: FormatDBML, Detail: "source does not look like DBML (expected 'Table', 'Ref', or 'Enum' definition)"}
	}
	return ParseResult{Format: FormatDBML, DiagramType: "dbml", Source: source}, nil
}

func looksLikeDBML(source string) bool {
	s := strings.ToLower(source)
	return strings.Contains(s, "table ") || strings.Contains(s, "table\t") ||
		strings.Contains(s, "ref ") || strings.Contains(s, "ref\t") ||
		strings.Contains(s, "enum ") || strings.Contains(s, "enum\t") ||
		strings.Contains(s, "project ") || strings.Contains(s, "tablegroup ")
}

func parsePlantUML(source string) (ParseResult, error) {
	// PlantUML source typically starts with @startuml and ends with @enduml.
	s := strings.TrimSpace(source)
	if !strings.HasPrefix(s, "@startuml") {
		return ParseResult{}, &ParseError{Format: FormatPlantUML, Detail: "plantuml source must start with @startuml"}
	}
	if !strings.Contains(s, "@enduml") {
		return ParseResult{}, &ParseError{Format: FormatPlantUML, Detail: "plantuml source must end with @enduml"}
	}
	return ParseResult{Format: FormatPlantUML, DiagramType: "plantuml", Source: source}, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func quote(s string) string {
	return "\"" + s + "\""
}

// ExtForFormat returns the file extension for a format (without leading dot).
func ExtForFormat(f Format) string {
	switch f {
	case FormatMermaid:
		return "mmd"
	case FormatDBML:
		return "dbml"
	case FormatPlantUML:
		return "puml"
	case FormatExcalidraw:
		return "excalidraw"
	default:
		return "mmd"
	}
}

// FormatFromExt infers a format from a file extension (lowercase, no dot).
// Empty/unknown returns FormatMermaid (the default diagram format).
func FormatFromExt(ext string) Format {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "mmd", "mermaid":
		return FormatMermaid
	case "dbml":
		return FormatDBML
	case "puml", "plantuml":
		return FormatPlantUML
	case "excalidraw":
		return FormatExcalidraw
	default:
		return FormatMermaid
	}
}
