package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// TestCanvasAIGenerateSuccess runs CanvasAI against a mock OpenAI-compatible
// chat server and asserts the returned elements array is parsed + id-minted.
func TestCanvasAIGenerateSuccess(t *testing.T) {
	home := t.TempDir()
	var chatHits atomic.Int32
	elementsContent := `[{"type":"rectangle","id":"n1","x":0,"y":0,"width":120,"height":60},{"type":"text","x":10,"y":20,"text":"Auth","fontSize":20}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"canvas-mock-1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			chatHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": elementsContent}},
				},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock", Type: "openai_compatible", Name: "Mock", BaseURL: srv.URL + "/v1", APIKey: "k"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	res := svc.CanvasAI(CanvasAIRequest{
		Prompt: "Draw auth flow",
		Mode:   CanvasAIGenerate,
	})
	if !res.OK {
		t.Fatalf("expected OK, got error=%q raw=%q", res.Error, res.RawText)
	}
	if chatHits.Load() < 1 {
		t.Fatal("expected chat completion call")
	}
	if res.Mode != CanvasAIGenerate {
		t.Fatalf("mode=%q", res.Mode)
	}
	if res.Model != "canvas-mock-1" {
		t.Fatalf("model=%q", res.Model)
	}
	var elements []map[string]any
	if err := json.Unmarshal([]byte(res.ElementsJSON), &elements); err != nil {
		t.Fatalf("unmarshal elements: %v\n%s", err, res.ElementsJSON)
	}
	if len(elements) != 2 {
		t.Fatalf("want 2 elements, got %d", len(elements))
	}
	if elements[0]["id"] != "n1" {
		t.Fatalf("el0 id=%v", elements[0]["id"])
	}
	if elements[0]["type"] != "rectangle" {
		t.Fatalf("el0 type=%v", elements[0]["type"])
	}
	if elements[1]["text"] != "Auth" {
		t.Fatalf("el1 text=%v", elements[1]["text"])
	}
	if res.LatencyMS < 0 {
		t.Fatal("expected non-negative latency")
	}
}

// TestCanvasAIEditModeIncludesExistingContext verifies edit mode sends the
// existing elements array as context in the system prompt.
func TestCanvasAIEditModeIncludesExistingContext(t *testing.T) {
	home := t.TempDir()
	var capturedBody string
	editedContent := `[{"type":"rectangle","id":"db","x":0,"y":0,"width":100,"height":50,"backgroundColor":"#3498db"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			buf, _ := io.ReadAll(r.Body)
			capturedBody = string(buf)
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": editedContent}},
				},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock", Type: "openai_compatible", Name: "Mock", BaseURL: srv.URL + "/v1", APIKey: "k"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	existing := `[{"type":"rectangle","id":"db","x":0,"y":0,"width":100,"height":50}]`
	res := svc.CanvasAI(CanvasAIRequest{
		Prompt:               "Change Database box to blue",
		Mode:                 CanvasAIEdit,
		ExistingElementsJSON: existing,
	})
	if !res.OK {
		t.Fatalf("expected OK, error=%q raw=%q", res.Error, res.RawText)
	}
	if res.Mode != CanvasAIEdit {
		t.Fatalf("mode=%q", res.Mode)
	}
	// The existing elements are JSON-escaped inside the system prompt content,
	// so check for the stable id rather than the raw string.
	if !strings.Contains(capturedBody, "db") {
		t.Fatalf("existing element id 'db' not in request body")
	}
	if !strings.Contains(capturedBody, "MODE: edit") {
		t.Fatalf("edit mode marker not in system prompt")
	}
	if !strings.Contains(capturedBody, "EXISTING ELEMENTS") {
		t.Fatalf("existing elements section not in system prompt")
	}
	var elements []map[string]any
	if err := json.Unmarshal([]byte(res.ElementsJSON), &elements); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, res.ElementsJSON)
	}
	if elements[0]["backgroundColor"] != "#3498db" {
		t.Fatalf("bg=%v", elements[0]["backgroundColor"])
	}
}

// TestCanvasAIStripsFences verifies markdown code fences are stripped before
// JSON parsing.
func TestCanvasAIStripsFences(t *testing.T) {
	home := t.TempDir()
	fencedContent := "```json\n[{\"type\":\"rectangle\",\"id\":\"r1\",\"x\":1,\"y\":2,\"width\":3,\"height\":4}]\n```"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": fencedContent}},
				},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock", Type: "openai_compatible", Name: "Mock", BaseURL: srv.URL + "/v1", APIKey: "k"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.CanvasAI(CanvasAIRequest{Prompt: "draw", Mode: CanvasAIGenerate})
	if !res.OK {
		t.Fatalf("expected OK, error=%q raw=%q", res.Error, res.RawText)
	}
	var elements []map[string]any
	if err := json.Unmarshal([]byte(res.ElementsJSON), &elements); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, res.ElementsJSON)
	}
	if len(elements) != 1 {
		t.Fatalf("want 1 element, got %d", len(elements))
	}
	if elements[0]["id"] != "r1" {
		t.Fatalf("id=%v", elements[0]["id"])
	}
}

// TestCanvasAIMintsMissingIDs verifies elements without an id get one minted.
func TestCanvasAIMintsMissingIDs(t *testing.T) {
	home := t.TempDir()
	content := `[{"type":"rectangle","x":0,"y":0},{"type":"text","id":"t1","text":"hi"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": content}},
				},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock", Type: "openai_compatible", Name: "Mock", BaseURL: srv.URL + "/v1", APIKey: "k"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.CanvasAI(CanvasAIRequest{Prompt: "draw", Mode: CanvasAIGenerate})
	if !res.OK {
		t.Fatalf("expected OK, error=%q", res.Error)
	}
	var elements []map[string]any
	if err := json.Unmarshal([]byte(res.ElementsJSON), &elements); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(elements) != 2 {
		t.Fatalf("want 2, got %d", len(elements))
	}
	id0, _ := elements[0]["id"].(string)
	if !strings.HasPrefix(id0, "el") || len(id0) < 4 {
		t.Fatalf("el0 id not minted: %q", id0)
	}
	if elements[1]["id"] != "t1" {
		t.Fatalf("el1 id=%v (should be preserved)", elements[1]["id"])
	}
}

// TestCanvasAIElementsObjectKey accepts {"elements":[...]} object shape.
func TestCanvasAIElementsObjectKey(t *testing.T) {
	home := t.TempDir()
	content := `{"elements":[{"type":"rectangle","id":"x1","x":0,"y":0,"width":10,"height":10}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": content}},
				},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock", Type: "openai_compatible", Name: "Mock", BaseURL: srv.URL + "/v1", APIKey: "k"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.CanvasAI(CanvasAIRequest{Prompt: "draw", Mode: CanvasAIGenerate})
	if !res.OK {
		t.Fatalf("expected OK, error=%q raw=%q", res.Error, res.RawText)
	}
	var elements []map[string]any
	if err := json.Unmarshal([]byte(res.ElementsJSON), &elements); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, res.ElementsJSON)
	}
	if len(elements) != 1 {
		t.Fatalf("want 1, got %d", len(elements))
	}
}

// TestCanvasAIEmptyPromptRejected verifies empty prompt returns a structured
// error without calling the provider.
func TestCanvasAIEmptyPromptRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.CanvasAI(CanvasAIRequest{Prompt: "   "})
	if res.OK {
		t.Fatal("expected failure for empty prompt")
	}
	if !strings.Contains(res.Error, "prompt is required") {
		t.Fatalf("error=%q", res.Error)
	}
}

// TestCanvasAIBlockedKey verifies missing API key surfaces as a blocked error.
func TestCanvasAIBlockedKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.CanvasAI(CanvasAIRequest{Prompt: "draw"})
	if res.OK {
		t.Fatal("expected failure with no key")
	}
	if !strings.Contains(res.Error, "blocked") && !strings.Contains(res.Error, "key required") {
		t.Fatalf("error=%q", res.Error)
	}
}

// TestCanvasAIParseFailureStructured verifies a non-JSON model output returns a
// structured error (never panics) with raw text preserved.
func TestCanvasAIParseFailureStructured(t *testing.T) {
	home := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": "Sorry, I cannot draw that."}},
				},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock", Type: "openai_compatible", Name: "Mock", BaseURL: srv.URL + "/v1", APIKey: "k"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.CanvasAI(CanvasAIRequest{Prompt: "draw", Mode: CanvasAIGenerate})
	if res.OK {
		t.Fatal("expected parse failure")
	}
	if res.RawText == "" {
		t.Fatal("expected raw text preserved")
	}
	if !strings.Contains(res.Error, "not valid JSON") {
		t.Fatalf("error=%q", res.Error)
	}
}

// TestCanvasAIHTTPRoute verifies POST /api/canvas/ai routes through the hub
// handler and returns the structured CanvasAIResult.
func TestCanvasAIHTTPRoute(t *testing.T) {
	home := t.TempDir()
	content := `[{"type":"rectangle","id":"r1","x":0,"y":0,"width":10,"height":10}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": content}},
				},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock", Type: "openai_compatible", Name: "Mock", BaseURL: srv.URL + "/v1", APIKey: "k"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	body := map[string]any{
		"prompt": "Draw a box",
		"mode":   "generate",
	}
	raw, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/canvas/ai", strings.NewReader(string(raw))))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var res CanvasAIResult
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v %s", err, rr.Body.String())
	}
	if !res.OK {
		t.Fatalf("expected OK, error=%q", res.Error)
	}
	var elements []map[string]any
	if err := json.Unmarshal([]byte(res.ElementsJSON), &elements); err != nil {
		t.Fatalf("unmarshal elements: %v", err)
	}
	if len(elements) != 1 {
		t.Fatalf("want 1 element, got %d", len(elements))
	}

	// GET should be rejected.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/canvas/ai", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET should be 405, got %d", rr.Code)
	}
}

// TestParseCanvasAIElements covers the pure parser directly: fences, object
// wrapper, empty array, missing type, id minting, prose-surrounded JSON.
func TestParseCanvasAIElements(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantErr   string
		wantCount int
		wantID0   string
	}{
		{
			name:      "bare array",
			input:     `[{"type":"rectangle","id":"a","x":0,"y":0}]`,
			wantCount: 1,
			wantID0:   "a",
		},
		{
			name:      "json fence",
			input:     "```json\n[{\"type\":\"ellipse\",\"id\":\"b\",\"x\":1}]\n```",
			wantCount: 1,
			wantID0:   "b",
		},
		{
			name:      "bare fence",
			input:     "```\n[{\"type\":\"text\",\"id\":\"c\",\"text\":\"hi\"}]\n```",
			wantCount: 1,
			wantID0:   "c",
		},
		{
			name:      "object with elements key",
			input:     `{"elements":[{"type":"line","id":"d","points":[[0,0]]}]}`,
			wantCount: 1,
			wantID0:   "d",
		},
		{
			name:      "missing id minted",
			input:     `[{"type":"rectangle","x":0}]`,
			wantCount: 1,
		},
		{
			name:    "empty array",
			input:   `[]`,
			wantErr: "empty",
		},
		{
			name:    "missing type",
			input:   `[{"id":"x"}]`,
			wantErr: "type",
		},
		{
			name:    "garbage",
			input:   `not json at all`,
			wantErr: "not valid JSON",
		},
		{
			name:      "prose-surrounded json",
			input:     "Here is your diagram:\n[{\"type\":\"rectangle\",\"id\":\"e\",\"x\":0,\"y\":0,\"width\":1,\"height\":1}]\nHope it helps!",
			wantCount: 1,
			wantID0:   "e",
		},
		{
			name:      "fence with leading whitespace",
			input:     "  ```\n  [{\"type\":\"rectangle\",\"id\":\"f\",\"x\":0}]\n  ```  ",
			wantCount: 1,
			wantID0:   "f",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			els, err := parseCanvasAIElements(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (elements=%v)", tc.wantErr, els)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error=%q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(els) != tc.wantCount {
				t.Fatalf("want %d elements, got %d", tc.wantCount, len(els))
			}
			if tc.wantID0 != "" {
				m, _ := els[0].(map[string]any)
				if id, _ := m["id"].(string); id != tc.wantID0 {
					t.Fatalf("id0=%q want %q", id, tc.wantID0)
				}
			}
		})
	}
}

// TestCanvasAISystemPromptGenerate verifies generate mode system prompt shape.
func TestCanvasAISystemPromptGenerate(t *testing.T) {
	s := canvasAISystemPrompt(CanvasAIGenerate, "")
	if !strings.Contains(s, "generate") {
		t.Fatal("missing generate mode marker")
	}
	if !strings.Contains(s, "Excalidraw") {
		t.Fatal("missing Excalidraw mention")
	}
	if strings.Contains(s, "EXISTING ELEMENTS") {
		t.Fatal("generate mode should not include existing elements section")
	}
}

// TestCanvasAISystemPromptEdit verifies edit mode includes existing elements.
func TestCanvasAISystemPromptEdit(t *testing.T) {
	existing := `[{"type":"rectangle","id":"x","x":0}]`
	s := canvasAISystemPrompt(CanvasAIEdit, existing)
	if !strings.Contains(s, "edit") {
		t.Fatal("missing edit mode marker")
	}
	if !strings.Contains(s, "EXISTING ELEMENTS") {
		t.Fatal("missing existing elements section")
	}
	if !strings.Contains(s, existing) {
		t.Fatal("existing elements not embedded")
	}
	if !strings.Contains(s, "COMPLETE updated") {
		t.Fatal("missing complete-not-patch instruction")
	}
}

// TestTruncateRunesMiddle verifies head+tail preservation with marker.
func TestTruncateRunesMiddle(t *testing.T) {
	s := strings.Repeat("x", 5000)
	out := truncateRunesMiddle(s, 100)
	if len([]rune(out)) > 200 {
		t.Fatalf("output too long: %d runes", len([]rune(out)))
	}
	if !strings.Contains(out, "[truncated]") {
		t.Fatal("missing truncation marker")
	}
	if !strings.HasPrefix(out, "xxx") {
		t.Fatal("missing head")
	}
	if !strings.HasSuffix(out, "xxx") {
		t.Fatal("missing tail")
	}
	// Short string unchanged
	short := "hello"
	if got := truncateRunesMiddle(short, 100); got != short {
		t.Fatalf("short string changed: %q", got)
	}
}
