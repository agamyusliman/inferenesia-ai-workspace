package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

func TestChatStream_ImageGen_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/responses") && !strings.Contains(r.URL.Path, "image_generation") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]any{
					{"type": "image_generation_call", "result": "aGVsbG8="},
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/images/generations") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"url": "https://cdn.example/out.png"}},
			})
			return
		}
		// models discovery for resolve
		if strings.Contains(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "img-model"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", "test-key-not-secret")
	t.Setenv("TEMP_AI_MODEL", "img-model")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}

	var events []ChatEvent
	err = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:        "draw a lighthouse",
		GenerateImage: true,
		NoTools:       true,
	}, func(ev ChatEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("ChatStream: %v events=%v", err, events)
	}
	var done *ChatEvent
	for i := range events {
		if events[i].Type == ChatEventDone {
			done = &events[i]
			break
		}
	}
	if done == nil {
		t.Fatalf("no Done event: %v", events)
	}
	if !strings.Contains(done.Final, "data:image/png;base64,") &&
		!strings.Contains(done.Final, "https://cdn.example/out.png") {
		t.Fatalf("final=%q", done.Final)
	}

	// Durable history should include user + assistant (not ephemeral).
	hist, herr := svc.GetChatHistory("")
	if herr != nil {
		t.Fatal(herr)
	}
	if len(hist.Messages) < 2 {
		t.Fatalf("history messages=%v", hist.Messages)
	}
}

func TestAppendImageGenTranscript_EmptyWorkspaceNoOp(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	before, herr := svc.GetChatHistory("")
	if herr != nil {
		t.Fatal(herr)
	}
	beforeN := len(before.Messages)

	// Empty workspaceID must never fall back to active session.
	svc.appendImageGenTranscript(ChatRequest{}, "", "user", nil, "assistant", "m", "p")
	svc.appendImageGenTranscript(ChatRequest{}, "   ", "user2", nil, "assistant2", "m", "p")

	after, aerr := svc.GetChatHistory("")
	if aerr != nil {
		t.Fatal(aerr)
	}
	if len(after.Messages) != beforeN {
		t.Fatalf("empty workspace append polluted history: before=%d after=%d", beforeN, len(after.Messages))
	}
}

// A playground turn (Image Studio / Library canvas) must persist to its own
// "pg:" thread only — never into the coding workspace transcript.
func TestAppendImageGenTranscript_PlaygroundDoesNotTouchWorkspace(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	before, herr := svc.GetChatHistory(ws.ID)
	if herr != nil {
		t.Fatal(herr)
	}
	beforeN := len(before.Messages)

	// Library-style playground turn: playground id set, workspace id present
	// only as a scope hint. The workspace thread must not grow.
	req := ChatRequest{PlaygroundID: "image::global", WorkspaceID: ws.ID}
	svc.appendImageGenTranscript(req, ws.ID, "draw an apple", nil, "![img](x)", "m", "p")

	after, aerr := svc.GetChatHistory(ws.ID)
	if aerr != nil {
		t.Fatal(aerr)
	}
	if len(after.Messages) != beforeN {
		t.Fatalf("playground image gen leaked into workspace chat: before=%d after=%d", beforeN, len(after.Messages))
	}

	// The playground thread itself must have received the turn.
	pgMsgs, _, _ := svc.loadPlaygroundHistory("image::global")
	if len(pgMsgs) < 2 {
		t.Fatalf("playground thread missing turn: %d messages", len(pgMsgs))
	}
}

func TestChatStream_ImageGen_UnsupportedDegrade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "chat-only"}},
			})
			return
		}
		// image endpoints missing
		http.NotFound(w, r)
	}))
	defer srv.Close()

	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", "test-key-not-secret")
	t.Setenv("TEMP_AI_MODEL", "chat-only")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}

	var events []ChatEvent
	err = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:        "cat portrait",
		GenerateImage: true,
	}, func(ev ChatEvent) {
		events = append(events, ev)
	})
	if err == nil {
		t.Fatal("expected error for unsupported image gen")
	}
	if !provider.IsImageGenUnsupported(err) && !strings.Contains(err.Error(), "Image generation is not supported") {
		// Error should still be clear for UI bubble.
		if !strings.Contains(err.Error(), "not supported") && !strings.Contains(err.Error(), "image") {
			t.Fatalf("unexpected err: %v", err)
		}
	}
	var sawError bool
	for _, ev := range events {
		if ev.Type == ChatEventError && strings.Contains(ev.Error, "Image generation is not supported") {
			sawError = true
		}
	}
	if !sawError {
		// Also accept any Error event with image-related message.
		for _, ev := range events {
			if ev.Type == ChatEventError && strings.Contains(strings.ToLower(ev.Error), "image") {
				sawError = true
			}
		}
	}
	if !sawError {
		t.Fatalf("expected Error event, got %v", events)
	}
}

func TestChatStream_ImageGen_MaxFourRefs(t *testing.T) {
	var sawN int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "m"}},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/images/") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if refs, ok := body["reference_images"].([]any); ok {
				sawN = len(refs)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"url": "https://cdn.example/r.png"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	_ = os.MkdirAll(root, 0o755)
	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", "test-key-not-secret")
	t.Setenv("TEMP_AI_MODEL", "m")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}

	refs := make([]ChatImagePart, 0, 6)
	for i := 0; i < 6; i++ {
		refs = append(refs, ChatImagePart{
			Name:    "r.png",
			DataURL: "data:image/png;base64,QUJD" + string(rune('A'+i)),
		})
	}
	_ = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:        "style transfer",
		GenerateImage: true,
		Images:        refs,
	}, func(ChatEvent) {})
	if sawN != MaxChatImages {
		t.Fatalf("reference_images count=%d want %d", sawN, MaxChatImages)
	}
}

func TestChatStream_EphemeralContinue_NotStoredAsUser(t *testing.T) {
	// Verify that Ephemeral continue prompts are not appended as user history rows.
	// Use a minimal mock chat completions that returns a short incomplete then complete answer.
	round := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "chat-model"}},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/chat/completions") {
			round++
			// Non-stream JSON for ChatTurn path; ChatStream with tools may use SSE —
			// agent loop uses stream; return SSE stop with short content.
			w.Header().Set("Content-Type", "text/event-stream")
			content := "Complete answer ends here."
			if round == 1 {
				// First real user prompt path is not this test; we seed history below.
			}
			// SSE delta then done
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"" + content + "\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	_ = os.MkdirAll(root, 0o755)
	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", "test-key-not-secret")
	t.Setenv("TEMP_AI_MODEL", "chat-model")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}

	// Seed durable history with a real user turn already present.
	svc.mu.Lock()
	svc.chat.Messages = []provider.Message{
		{Role: provider.RoleUser, Content: "Write a long essay"},
		{Role: provider.RoleAssistant, Content: "Part one is ready and"},
	}
	svc.chatWorkspaceID = svc.registry.ActiveID()
	snap := ChatSession{Messages: append([]provider.Message(nil), svc.chat.Messages...)}
	wid := svc.chatWorkspaceID
	svc.mu.Unlock()
	_ = svc.persistChat(wid, snap)

	before, _ := svc.GetChatHistory("")
	userBefore := 0
	for _, m := range before.Messages {
		if m.Role == "user" {
			userBefore++
		}
	}

	_ = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:    "Continue from where you left off. Complete the unfinished answer.",
		Ephemeral: true,
		Continue:  true,
		NoTools:   true,
	}, func(ChatEvent) {})

	after, _ := svc.GetChatHistory("")
	userAfter := 0
	var sawEphemeralPrompt bool
	for _, m := range after.Messages {
		if m.Role == "user" {
			userAfter++
			if strings.Contains(m.Content, "Continue from where you left off") {
				sawEphemeralPrompt = true
			}
		}
	}
	if userAfter != userBefore {
		t.Fatalf("user messages before=%d after=%d (ephemeral must not add user row)", userBefore, userAfter)
	}
	if sawEphemeralPrompt {
		t.Fatal("ephemeral continue prompt must not appear in durable user history")
	}
}
