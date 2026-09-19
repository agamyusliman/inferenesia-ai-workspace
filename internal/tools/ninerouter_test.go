package tools_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func setupNineRouterReg(t *testing.T, loadSkills ...string) (*tools.Registry, *skills.Loader) {
	t.Helper()
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))
	for _, name := range loadSkills {
		if _, err := loader.Load(name); err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
	}
	return reg, loader
}

// VAL-KIT-001/002: skill list shows 9router pack; tools only after load.
func TestNineRouterToolsetOptIn(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))

	// Discovery via skill list tool.
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: tools.NameSkill, Arguments: `{"action":"list"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "9router-chat") {
		t.Fatalf("list missing 9router-chat: %s", res.Content)
	}
	if !strings.Contains(res.Content, "9router-web-search") {
		t.Fatalf("list missing 9router-web-search: %s", res.Content)
	}
	if !strings.Contains(res.Content, "metadata only") {
		t.Fatalf("list should note progressive metadata: %s", res.Content)
	}

	before := toolNames(reg)
	if contains(before, skills.ToolNineRouterWebSearch) {
		t.Fatal("ninerouter_web_search must not appear before skill load")
	}
	if contains(before, skills.ToolNineRouterImage) {
		t.Fatal("ninerouter_image must not appear before skill load")
	}

	// Load web-search skill → tool appears.
	res = reg.Execute(context.Background(), tools.Call{
		ID: "2", Name: tools.NameSkill, Arguments: `{"action":"load","name":"9router-web-search"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	after := toolNames(reg)
	if !contains(after, skills.ToolNineRouterWebSearch) {
		t.Fatalf("after load missing ninerouter_web_search: %v", after)
	}
	// Unrelated kit tools still off.
	if contains(after, skills.ToolNineRouterSTT) {
		t.Fatal("stt tool should stay off until 9router-stt loaded")
	}

	// Prompt overlay includes skill body.
	res = reg.Execute(context.Background(), tools.Call{
		ID: "3", Name: tools.NameSkill, Arguments: `{"action":"prompt"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "NINEROUTER") && !strings.Contains(res.Content, "9Router") {
		t.Fatalf("prompt overlay missing 9Router guidance: %s", res.Content)
	}

	// Unload → tool gone.
	res = reg.Execute(context.Background(), tools.Call{
		ID: "4", Name: tools.NameSkill, Arguments: `{"action":"unload","name":"9router-web-search"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if contains(toolNames(reg), skills.ToolNineRouterWebSearch) {
		t.Fatal("tool should be gone after unload")
	}
}

// VAL-KIT-003: missing NINEROUTER_URL blocks cleanly after skill load.
func TestNineRouterMissingEnvBlocks(t *testing.T) {
	t.Setenv(tools.EnvNineRouterURL, "")
	t.Setenv(tools.EnvNineRouterKey, "")

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterImage)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterImage, Arguments: `{"prompt":"cat"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected blocked error, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "NINEROUTER_URL") {
		t.Fatalf("error should mention NINEROUTER_URL: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "blocked") &&
		!strings.Contains(strings.ToLower(res.Content), "not set") {
		t.Fatalf("should be clear blocked/config: %s", res.Content)
	}
	if strings.Contains(res.Content, "Bearer") {
		t.Fatalf("must not leak auth: %s", res.Content)
	}
}

// VAL-KIT-003: STT blocked without env.
func TestNineRouterSTTBlockedWithoutEnv(t *testing.T) {
	t.Setenv(tools.EnvNineRouterURL, "")
	t.Setenv(tools.EnvNineRouterKey, "")

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterSTT)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterSTT, Arguments: `{"path":"a.wav"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected blocked: %s", res.Content)
	}
	if !strings.Contains(res.Content, "NINEROUTER_URL") {
		t.Fatalf("must mention NINEROUTER_URL: %s", res.Content)
	}
}

// Tool without skill load is denied.
func TestNineRouterToolDeniedWithoutLoad(t *testing.T) {
	t.Setenv(tools.EnvNineRouterURL, "http://127.0.0.1:20128")
	reg, _ := setupNineRouterReg(t)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterEmbeddings, Arguments: `{"input":"hi"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected error without load: %s", res.Content)
	}
	if !strings.Contains(res.Content, "skill") && !strings.Contains(res.Content, "not available") {
		t.Fatalf("unexpected message: %s", res.Content)
	}
}

// VAL-KIT-004: image tool path works with mock server.
func TestNineRouterImageMock(t *testing.T) {
	const secret = "nr-image-secret-key-xyz"
	var sawAuth string
	var sawPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/images/generations" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if p, ok := payload["prompt"].(string); ok {
			sawPrompt = p
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created": 1,
			"data": []map[string]string{
				{"url": "https://cdn.example.com/img.png", "revised_prompt": "a cat"},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(tools.EnvNineRouterURL, srv.URL)
	t.Setenv(tools.EnvNineRouterKey, secret)

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterImage)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterImage,
		Arguments: `{"prompt":"watercolor cat","size":"1024x1024"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("image tool error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "https://cdn.example.com/img.png") {
		t.Fatalf("expected image url in result: %s", res.Content)
	}
	if sawPrompt != "watercolor cat" {
		t.Fatalf("prompt not sent: %q", sawPrompt)
	}
	if sawAuth != "Bearer "+secret {
		t.Fatalf("expected Authorization on request, got %q", sawAuth)
	}
	// VAL-KIT-010: secret never in tool result.
	if strings.Contains(res.Content, secret) {
		t.Fatalf("key leaked in result: %s", res.Content)
	}
}

// VAL-KIT-004: image blocked cleanly without env.
func TestNineRouterImageBlockedNoEnv(t *testing.T) {
	t.Setenv(tools.EnvNineRouterURL, "")
	reg, _ := setupNineRouterReg(t, skills.NameNineRouterImage)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterImage, Arguments: `{"prompt":"x"}`,
	}, nil)
	if !res.IsError || !strings.Contains(res.Content, "NINEROUTER_URL") {
		t.Fatalf("want blocked NINEROUTER_URL, got: %s", res.Content)
	}
}

// VAL-KIT-005: web_search returns structured title/url/snippet via mock HTTP.
func TestNineRouterWebSearchMock(t *testing.T) {
	const secret = "nr-search-secret-abc12345"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "tavily",
			"query":    "9Router open source",
			"results": []map[string]any{
				{
					"title":   "9Router GitHub",
					"url":     "https://github.com/decolua/9router",
					"snippet": "Open source AI gateway",
					"score":   0.95,
				},
				{
					"title":   "Docs",
					"url":     "https://example.com/docs",
					"snippet": "API reference",
				},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(tools.EnvNineRouterURL, srv.URL)
	t.Setenv(tools.EnvNineRouterKey, secret)

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterWebSearch)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterWebSearch,
		Arguments: `{"query":"9Router open source","max_results":5}`,
	}, nil)
	if res.IsError {
		t.Fatalf("search error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "9Router GitHub") {
		t.Fatalf("missing title: %s", res.Content)
	}
	if !strings.Contains(res.Content, "https://github.com/decolua/9router") {
		t.Fatalf("missing url: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Open source AI gateway") {
		t.Fatalf("missing snippet: %s", res.Content)
	}
	if strings.Contains(res.Content, secret) {
		t.Fatalf("key leaked: %s", res.Content)
	}
	// Structured JSON
	var parsed map[string]any
	if err := json.Unmarshal([]byte(res.Content), &parsed); err != nil {
		t.Fatalf("result should be JSON: %v\n%s", err, res.Content)
	}
	if parsed["count"] == nil {
		t.Fatalf("expected count field: %s", res.Content)
	}
}

// VAL-KIT-006: web_fetch returns content; format option honored.
func TestNineRouterWebFetchMock(t *testing.T) {
	var sawFormat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/web/fetch" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if f, ok := payload["format"].(string); ok {
			sawFormat = f
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "jina-reader",
			"url":      "https://example.com",
			"title":    "Example Domain",
			"content": map[string]any{
				"format": payload["format"],
				"text":   "# Example\n\nHello from mock fetch.",
				"length": 30,
			},
		})
	}))
	defer srv.Close()

	t.Setenv(tools.EnvNineRouterURL, srv.URL)
	t.Setenv(tools.EnvNineRouterKey, "")

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterWebFetch)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterWebFetch,
		Arguments: `{"url":"https://example.com","format":"markdown","model":"jina-reader"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("fetch error: %s", res.Content)
	}
	if sawFormat != "markdown" {
		t.Fatalf("format not sent: %q", sawFormat)
	}
	if !strings.Contains(res.Content, "Hello from mock fetch") {
		t.Fatalf("missing content: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Example Domain") {
		t.Fatalf("missing title: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"format"`) {
		t.Fatalf("format field missing in result: %s", res.Content)
	}
}

// VAL-KIT-007: embeddings optional without Qdrant; vectors from endpoint.
func TestNineRouterEmbeddingsMockNoQdrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		// No Qdrant involved — pure embed response.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"model":  "openai/text-embedding-3-small",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": []float64{0.1, -0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9},
				},
			},
			"usage": map[string]int{"prompt_tokens": 2, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	t.Setenv(tools.EnvNineRouterURL, srv.URL)
	t.Setenv(tools.EnvNineRouterKey, "embed-key-never-print")

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterEmbeddings)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterEmbeddings,
		Arguments: `{"input":"hello world","model":"openai/text-embedding-3-small"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("embeddings error: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"dimensions"`) {
		t.Fatalf("expected dimensions summary: %s", res.Content)
	}
	if !strings.Contains(res.Content, "no Qdrant") && !strings.Contains(res.Content, "enowx-rag") {
		t.Fatalf("should note no Qdrant/enowx-rag required: %s", res.Content)
	}
	// Must not require or mention wiring to vector DB install steps as dependency.
	if strings.Contains(res.Content, "Qdrant required") {
		t.Fatalf("must not require Qdrant: %s", res.Content)
	}
	if strings.Contains(res.Content, "embed-key-never-print") {
		t.Fatalf("key leaked: %s", res.Content)
	}
}

// VAL-KIT-008: STT with mock multipart success.
func TestNineRouterSTTMock(t *testing.T) {
	var gotMultipart bool
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			http.NotFound(w, r)
			return
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart/form-data") {
			http.Error(w, "want multipart", http.StatusBadRequest)
			return
		}
		gotMultipart = true
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotModel = r.FormValue("model")
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "hello from speech",
		})
	}))
	defer srv.Close()

	t.Setenv(tools.EnvNineRouterURL, srv.URL)
	t.Setenv(tools.EnvNineRouterKey, "stt-key-secret-value-zz")

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterSTT)
	// Place audio fixture in workspace.
	ws := reg.Gateway().RootPath()
	audioPath := filepath.Join(ws, "sample.wav")
	if err := os.WriteFile(audioPath, []byte("RIFF....WAVEfake"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterSTT,
		Arguments: `{"path":"sample.wav","model":"openai/whisper-1","language":"en"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("stt error: %s", res.Content)
	}
	if !gotMultipart {
		t.Fatal("expected multipart request")
	}
	if gotModel != "openai/whisper-1" {
		t.Fatalf("model=%q", gotModel)
	}
	if !strings.Contains(res.Content, "hello from speech") {
		t.Fatalf("missing transcript: %s", res.Content)
	}
	if strings.Contains(res.Content, "stt-key-secret-value-zz") {
		t.Fatalf("key leaked: %s", res.Content)
	}
}

// VAL-KIT-010: secrets never appear in tool results even if server echoes them.
func TestNineRouterSecretsNeverInResults(t *testing.T) {
	const secret = "super-secret-ninerouter-key-999"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Malicious server echoes Authorization — client must still redact tool content.
		auth := r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{"title": "x", "url": "https://x.test", "snippet": "auth was " + auth},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(tools.EnvNineRouterURL, srv.URL)
	t.Setenv(tools.EnvNineRouterKey, secret)

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterWebSearch)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterWebSearch,
		Arguments: `{"query":"test"}`,
	}, nil)
	// Result may succeed but must not contain the raw key.
	if strings.Contains(res.Content, secret) {
		t.Fatalf("secret leaked in tool result: %s", res.Content)
	}
	if strings.Contains(res.Content, "Bearer "+secret) {
		t.Fatalf("bearer secret leaked: %s", res.Content)
	}
}

// URL with credentials in userinfo is not echoed.
func TestNineRouterRedactBaseInErrors(t *testing.T) {
	// Unreachable host with userinfo to force connection error path if possible.
	// Use a closed port for fast fail.
	t.Setenv(tools.EnvNineRouterURL, "http://user:password-leak@127.0.0.1:1")
	t.Setenv(tools.EnvNineRouterKey, "key-should-not-appear-in-output")

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterWebSearch)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterWebSearch,
		Arguments: `{"query":"x"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected transport error, got success: %s", res.Content)
	}
	if strings.Contains(res.Content, "password-leak") {
		t.Fatalf("userinfo password leaked: %s", res.Content)
	}
	if strings.Contains(res.Content, "key-should-not-appear-in-output") {
		t.Fatalf("api key leaked: %s", res.Content)
	}
}

// Base URL without /v1 still hits /v1/search (normalization).
func TestNineRouterBaseURLNormalization(t *testing.T) {
	var sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{"title": "t", "url": "https://t.test", "snippet": "s"},
			},
		})
	}))
	defer srv.Close()

	// No trailing /v1
	t.Setenv(tools.EnvNineRouterURL, srv.URL)
	t.Setenv(tools.EnvNineRouterKey, "")

	reg, _ := setupNineRouterReg(t, skills.NameNineRouterWebSearch)
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: skills.ToolNineRouterWebSearch, Arguments: `{"query":"q"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if sawPath != "/v1/search" {
		t.Fatalf("path=%q want /v1/search", sawPath)
	}
}
