package diagram

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Excalidraw JSON source format support (VAL-DIAG-007).
//
// The Excalidraw bridge persists `.excalidraw` source under docs/diagrams/
// through WriteGateway (undoable) and round-trips with the text editor (Monaco
// opens the JSON source; edits save back via WriteGateway). The Mermaid ->
// Excalidraw converter (see mermaid_to_excalidraw.go) preserves node/edge
// count on a basic fixture.
//
// The visual canvas (`@excalidraw/excalidraw` React component) remains a
// separate D5 surface; this package implements the format + bridge so the
// agent can author and convert Excalidraw sources today.

// ExcalidrawDocument is the top-level `.excalidraw` JSON shape. Only the
// fields needed for validation and round-trip are modeled; unknown fields are
// preserved via RawElements when re-serializing is not required.
type ExcalidrawDocument struct {
	Type     string             `json:"type,omitempty"`
	Version  int                `json:"version,omitempty"`
	Source   string             `json:"source,omitempty"`
	Elements []ExcalidrawElement `json:"elements"`
	AppState map[string]any     `json:"appState,omitempty"`
	Files    map[string]any     `json:"files,omitempty"`
}

// ExcalidrawElement is one element (rectangle / arrow / text / ellipse / …).
// Fields are a permissive subset; extra fields are tolerated.
type ExcalidrawElement struct {
	Type       string  `json:"type"`
	ID         string  `json:"id"`
	X          float64 `json:"x,omitempty"`
	Y          float64 `json:"y,omitempty"`
	Width      float64 `json:"width,omitempty"`
	Height     float64 `json:"height,omitempty"`
	Text       string  `json:"text,omitempty"`
	Points     [][]float64 `json:"points,omitempty"`
	StartBinding *ExcalidrawBinding `json:"startBinding,omitempty"`
	EndBinding   *ExcalidrawBinding `json:"endBinding,omitempty"`
	ContainerID string  `json:"containerId,omitempty"`
}

// ExcalidrawBinding links an arrow endpoint to a container element id.
type ExcalidrawBinding struct {
	ElementID string  `json:"elementId"`
	Focus     float64 `json:"focus"`
	Gap       float64 `json:"gap"`
}

// ExcalidrawSummary counts element kinds for round-trip / conversion checks.
type ExcalidrawSummary struct {
	Nodes     int // rectangle + ellipse + diamond shapes (Mermaid node analogs)
	Edges     int // arrow / line elements (Mermaid edge analogs)
	Texts     int // text elements
	Total     int // all elements
}

// ParseExcalidraw validates Excalidraw JSON source and returns the document.
// It requires valid JSON with an `elements` array; each element must have a
// `type` and `id`. Unknown shapes are tolerated (round-trip), but junk JSON is
// rejected with a *ParseError (VAL-DIAG-005).
func ParseExcalidraw(source string) (ExcalidrawDocument, error) {
	src := strings.TrimSpace(source)
	if src == "" {
		return ExcalidrawDocument{}, &ParseError{Format: FormatExcalidraw, Detail: "empty excalidraw source"}
	}
	var doc ExcalidrawDocument
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		return ExcalidrawDocument{}, &ParseError{
			Format: FormatExcalidraw,
			Detail: "invalid JSON: " + simplifyJSONErr(err),
		}
	}
	// Require an elements array. The top-level `type` is conventionally
	// "excalidraw" but some exports omit it; we accept either as long as
	// elements exist (round-trip friendly).
	if doc.Elements == nil {
		return ExcalidrawDocument{}, &ParseError{
			Format: FormatExcalidraw,
			Detail: "missing `elements` array (expected Excalidraw document with elements: [])",
		}
	}
	for i, el := range doc.Elements {
		if strings.TrimSpace(el.Type) == "" {
			return ExcalidrawDocument{}, &ParseError{
				Format: FormatExcalidraw,
				Detail: fmt.Sprintf("element at index %d missing `type`", i),
			}
		}
		if strings.TrimSpace(el.ID) == "" {
			return ExcalidrawDocument{}, &ParseError{
				Format: FormatExcalidraw,
				Detail: fmt.Sprintf("element at index %d (type %q) missing `id`", i, el.Type),
			}
		}
	}
	return doc, nil
}

// SummarizeExcalidraw counts element kinds in a parsed document.
// Nodes = rectangle/ellipse/diamond; Edges = arrow/line; Texts = text.
func SummarizeExcalidraw(doc ExcalidrawDocument) ExcalidrawSummary {
	var s ExcalidrawSummary
	s.Total = len(doc.Elements)
	for _, el := range doc.Elements {
		switch strings.ToLower(el.Type) {
		case "rectangle", "ellipse", "diamond":
			s.Nodes++
		case "arrow", "line":
			s.Edges++
		case "text":
			s.Texts++
		}
	}
	return s
}

// SerializeExcalidraw marshals a document to canonical JSON (2-space indent).
// Used by the converter to emit `.excalidraw` source.
func SerializeExcalidraw(doc ExcalidrawDocument) (string, error) {
	if doc.Version == 0 {
		doc.Version = 2
	}
	if doc.Type == "" {
		doc.Type = "excalidraw"
	}
	if doc.Source == "" {
		doc.Source = "inferenesia"
	}
	if doc.AppState == nil {
		doc.AppState = map[string]any{"viewBackgroundColor": "#ffffff"}
	}
	if doc.Files == nil {
		doc.Files = map[string]any{}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// simplifyJSONErr trims long offset hints from encoding/json errors so the
// parse error detail stays readable.
func simplifyJSONErr(err error) string {
	msg := err.Error()
	// Trim the trailing "offset N" suffix noise for display.
	if i := strings.LastIndex(msg, " offset "); i > 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}
