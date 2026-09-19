package core

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

	"github.com/agamyusliman/inferenesia-app/internal/memory"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// VAL-CROSS-002: when .multibrain/session.md exists, the session loads it as the
// master index and makes bucket context available to the agent (observable via
// the system-prompt section sent to the provider). This test exercises the
// core.Service bootstrap path (desktop/same-as-CLI Service.ChatStream) end-to-end
// against a mock gateway that records the system message of the first request.
func TestCrossArea_MultiBrainSession_PresentMakesBucketContextAvailable(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()

	// Build a small .multibrain/ tree with session.md + one matching bucket.
	mb := filepath.Join(ws, ".multibrain")
	if err := os.MkdirAll(filepath.Join(mb, "indexes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mb, "context"), 0o755); err != nil {
		t.Fatal(err)
	}
	const sessionMarker = "MULTIBRAIN-MASTER-INDEX-MARKER"
	const bucketMarker = "MULTIBRAIN-BUCKET-ARCH-MARKER"
	session := `# Multi Brain Master Index

## Buckets

- ` + "`architecture`" + ` — monorepo layout. Last updated: 2026-07-18 09:00 WIB -> .multibrain/indexes/architecture.md
`
	if err := os.WriteFile(filepath.Join(mb, "session.md"), []byte(sessionMarker+"\n"+session), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mb, "indexes", "architecture.md"), []byte(bucketMarker+"\n# architecture\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Mock temp-ai gateway: capture the system message of the chat request.
	var capturedSystem string
	var sawRequest bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "mock-model"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		for _, m := range req.Messages {
			if m.Role == "system" {
				capturedSystem += m.Content + "\n"
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
		fl.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", "mock-key-not-secret")
	t.Setenv("TEMP_AI_MODEL", "")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var events []ChatEvent
	err = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:  "query architecture",
		NoTools: true,
	}, func(ev ChatEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("ChatStream: %v events=%v", err, events)
	}
	if !sawRequest {
		t.Fatalf("gateway was never contacted by ChatStream")
	}

	// (a) The system prompt must include a Multi Brain section (VAL-CROSS-002).
	if !strings.Contains(capturedSystem, "## Multi Brain memory (this workspace)") {
		t.Fatalf("system prompt missing Multi Brain section:\n%s", capturedSystem)
	}
	// (b) session.md content (master index) must be available to the agent.
	if !strings.Contains(capturedSystem, sessionMarker) {
		t.Fatalf("system prompt missing session.md master index content:\n%s", capturedSystem)
	}
	// (c) At least one bucket's context must be available to the agent.
	if !strings.Contains(capturedSystem, bucketMarker) {
		t.Fatalf("system prompt missing bucket context:\n%s", capturedSystem)
	}
	// (d) Read-order rules included so the agent treats session.md as master index.
	if !strings.Contains(capturedSystem, "master index") {
		t.Fatalf("system prompt missing master-index rule:\n%s", capturedSystem)
	}
	// (e) No RAG fallback advertised: the section explicitly says no RAG/vector store.
	if !strings.Contains(capturedSystem, "no RAG/vector store") {
		t.Fatalf("system prompt missing no-RAG note:\n%s", capturedSystem)
	}
}

// VAL-CROSS-002 (absent path): when .multibrain/ is absent, the session proceeds
// without error and does not attempt RAG/Qdrant fallback. The system prompt must
// NOT mention Multi Brain memory, and the trace must be free of qdrant/voyage.
func TestCrossArea_MultiBrainSession_AbsentNoErrorNoRAGFallback(t *testing.T) {
	home := t.TempDir()
	// Workspace with NO .multibrain/ directory.
	ws := t.TempDir()
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	var capturedSystem string
	var sawRequest bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "mock-model"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		for _, m := range req.Messages {
			if m.Role == "system" {
				capturedSystem += m.Content + "\n"
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n")
		fl.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", "mock-key-not-secret")
	t.Setenv("TEMP_AI_MODEL", "")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var events []ChatEvent
	err = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:  "hello",
		NoTools: true,
	}, func(ev ChatEvent) {
		events = append(events, ev)
	})
	// (a) No error despite absent multi-brain.
	if err != nil {
		t.Fatalf("ChatStream should proceed without error when multibrain absent, got: %v", err)
	}
	if !sawRequest {
		t.Fatalf("gateway was never contacted — chat did not proceed")
	}
	// (b) System prompt must NOT contain a Multi Brain session section (no multibrain present).
	//     Use the unique section header so the default tool description ("write Multi Brain memory files")
	//     is not mistaken for the loaded context section.
	if strings.Contains(capturedSystem, "## Multi Brain memory (this workspace)") {
		t.Fatalf("system prompt should not include Multi Brain section when absent:\n%s", capturedSystem)
	}
	// (c) No RAG / Qdrant fallback wiring in the prompt.
	lower := strings.ToLower(capturedSystem)
	for _, bad := range []string{"qdrant", "voyage", "enowx", "rag retrieve", "vector store"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("system prompt mentions %q (RAG fallback must not be attempted):\n%s", bad, capturedSystem)
		}
	}
}

// TestFormatMultiBrainSnippet_Present verifies the pure formatter includes the
// master index and bounded bucket context with the read-order rules.
func TestFormatMultiBrainSnippet_Present(t *testing.T) {
	res := memory.LoadResult{
		Status: memory.Status{
			Present:       true,
			SessionExists: true,
		},
		Session: "SESSION-MASTER-CONTENT",
		Buckets: map[string]string{
			"architecture": "ARCH-BUCKET-CONTENT",
		},
		Contexts: map[string]string{
			".multibrain/context/arch-note.md": "ARCH-CONTEXT-CONTENT",
		},
	}
	out := formatMultiBrainSnippet(res)
	if !strings.Contains(out, "## Multi Brain memory (this workspace)") {
		t.Fatalf("missing section header:\n%s", out)
	}
	if !strings.Contains(out, "master index") {
		t.Fatalf("missing master-index rule:\n%s", out)
	}
	if !strings.Contains(out, "SESSION-MASTER-CONTENT") {
		t.Fatalf("missing session content:\n%s", out)
	}
	if !strings.Contains(out, "ARCH-BUCKET-CONTENT") {
		t.Fatalf("missing bucket content:\n%s", out)
	}
	if !strings.Contains(out, "ARCH-CONTEXT-CONTENT") {
		t.Fatalf("missing context content:\n%s", out)
	}
	if !strings.Contains(out, "no RAG/vector store") {
		t.Fatalf("missing no-RAG note:\n%s", out)
	}
}

// TestFormatMultiBrainSnippet_AbsentReturnsEmpty verifies the absent path
// yields no snippet (no RAG fallback).
func TestFormatMultiBrainSnippet_AbsentReturnsEmpty(t *testing.T) {
	res := memory.LoadResult{
		Status: memory.Status{Present: false},
	}
	if out := formatMultiBrainSnippet(res); out != "" {
		t.Fatalf("expected empty snippet when absent, got:\n%s", out)
	}
}

// TestMultiBrainSystemSnippet_AbsentWorkspace verifies the service helper
// returns "" (and does not error) for a workspace with no .multibrain/.
func TestMultiBrainSystemSnippet_AbsentWorkspace(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	if out := svc.multiBrainSystemSnippet(ws, "anything"); out != "" {
		t.Fatalf("expected empty snippet for absent multibrain, got:\n%s", out)
	}
}

// TestMultiBrainSystemSnippet_PresentWorkspace verifies the service helper
// loads session.md as master index + bucket context.
func TestMultiBrainSystemSnippet_PresentWorkspace(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	mb := filepath.Join(ws, ".multibrain", "indexes")
	if err := os.MkdirAll(mb, 0o755); err != nil {
		t.Fatal(err)
	}
	session := `## Buckets

- ` + "`agents`" + ` — notes -> .multibrain/indexes/agents.md
`
	if err := os.WriteFile(filepath.Join(ws, ".multibrain", "session.md"), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mb, "agents.md"), []byte("AGENTS-BODY"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := svc.multiBrainSystemSnippet(ws, "agents")
	if !strings.Contains(out, "## Multi Brain memory (this workspace)") {
		t.Fatalf("missing section:\n%s", out)
	}
	if !strings.Contains(out, "AGENTS-BODY") {
		t.Fatalf("missing agents bucket body:\n%s", out)
	}
}

// Compile-time guard: ensure the provider message type is wired as expected
// so the test captures remain valid if the provider package evolves.
var _ = provider.RoleSystem
