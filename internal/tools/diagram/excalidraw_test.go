package diagram

import (
	"encoding/json"
	"strings"
	"testing"
)

// VAL-DIAG-007: Excalidraw source parses (JSON validation) and round-trips.
func TestParseExcalidrawValidDocument(t *testing.T) {
	src := `{
  "type": "excalidraw",
  "version": 2,
  "source": "inferenesia",
  "elements": [
    {"type":"rectangle","id":"a","x":0,"y":0,"width":100,"height":60},
    {"type":"ellipse","id":"b","x":200,"y":0,"width":100,"height":60},
    {"type":"arrow","id":"e1","x":100,"y":30,"width":100,"height":0,"points":[[0,0],[100,0]]}
  ],
  "appState": {"viewBackgroundColor":"#ffffff"},
  "files": {}
}`
	doc, err := ParseExcalidraw(src)
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if len(doc.Elements) != 3 {
		t.Fatalf("want 3 elements, got %d", len(doc.Elements))
	}
	s := SummarizeExcalidraw(doc)
	if s.Nodes != 2 {
		t.Fatalf("want 2 nodes (rect+ellipse), got %d", s.Nodes)
	}
	if s.Edges != 1 {
		t.Fatalf("want 1 edge (arrow), got %d", s.Edges)
	}
	if s.Total != 3 {
		t.Fatalf("want total=3, got %d", s.Total)
	}
}

func TestParseExcalidrawRejectsJunk(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"empty", "", "empty"},
		{"not json", "hello world", "invalid JSON"},
		{"missing elements", `{"type":"excalidraw"}`, "missing `elements`"},
		{"element no type", `{"elements":[{"id":"x"}]}`, "missing `type`"},
		{"element no id", `{"elements":[{"type":"rectangle"}]}`, "missing `id`"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseExcalidraw(c.src)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q should contain %q", err.Error(), c.want)
			}
			// Must be a *ParseError for VAL-DIAG-005 consistency.
			if _, ok := err.(*ParseError); !ok {
				t.Fatalf("expected *ParseError, got %T", err)
			}
		})
	}
}

// VAL-DIAG-007: serialize -> parse round-trips (editor round-trip guard).
func TestSerializeParseRoundTrip(t *testing.T) {
	doc := ExcalidrawDocument{
		Elements: []ExcalidrawElement{
			{Type: "rectangle", ID: "n1", X: 10, Y: 10, Width: 100, Height: 60, Text: ""},
			{Type: "arrow", ID: "e1", X: 110, Y: 40, Width: 90, Height: 0,
				Points: [][]float64{{0, 0}, {90, 0}}},
		},
	}
	out, err := SerializeExcalidraw(doc)
	if err != nil {
		t.Fatal(err)
	}
	// The serialized output must be valid JSON.
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("serialized output is not JSON: %v", err)
	}
	// And must round-trip through ParseExcalidraw.
	doc2, err := ParseExcalidraw(out)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	s1 := SummarizeExcalidraw(doc)
	s2 := SummarizeExcalidraw(doc2)
	if s1 != s2 {
		t.Fatalf("summary changed across round-trip: %+v vs %+v", s1, s2)
	}
	if doc2.Type != "excalidraw" {
		t.Fatalf("type not set: %q", doc2.Type)
	}
	if doc2.Version != 2 {
		t.Fatalf("version not set: %d", doc2.Version)
	}
}
