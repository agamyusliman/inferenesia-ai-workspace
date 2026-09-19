package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// VAL-PROV-007: with Anthropic key present, chat streams via anthropic adapter (mock).
func TestAnthropicChatStreamWithKey(t *testing.T) {
	var gotAPIKey string
	var gotVersion string
	var gotPath string
	var hitTempAI atomic.Int32

	anthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotPath = r.URL.Path
		// Never use Authorization Bearer for Anthropic.
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Anthropic must not send Authorization header: %q", auth)
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "claude-test-1"}},
			})
			return
		}
		// Stream SSE like Anthropic Messages API.
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		write := func(s string) {
			_, _ = io.WriteString(w, s)
			if flusher != nil {
				flusher.Flush()
			}
		}
		write("event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		write("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		write("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		write("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" Claude\"}}\n\n")
		write("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		write("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
		write("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer anthSrv.Close()

	tempFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitTempAI.Add(1)
		t.Errorf("Anthropic chat must not hit temp-ai: %s", r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer tempFake.Close()

	t.Setenv(EnvAPIKey, "temp-key")
	t.Setenv(EnvBaseURL, tempFake.URL+"/v1")
	t.Setenv(EnvAnthropicAPIKey, "")
	t.Setenv(EnvAnthropicBaseURL, "")

	file := config.FileConfig{
		Providers: []config.ProviderProfile{
			{
				ID:           ProfileAnthropic,
				Type:         "anthropic",
				Name:         "Anthropic",
				BaseURL:      anthSrv.URL,
				APIKey:       "anth-test-key-never-print",
				DefaultModel: "claude-test-1",
			},
		},
	}
	r := Bootstrap(BootstrapOptions{File: file, PreferredID: ProfileAnthropic})
	client, err := r.GetProfile(ProfileAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	if AdapterType(client) != TypeAnthropic {
		t.Fatalf("adapter type=%s", AdapterType(client))
	}
	if IsTempAIHost(client.RequestHost()) {
		t.Fatalf("host is temp-ai: %s", client.RequestHost())
	}

	ch, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "claude-test-1",
		Messages: []Message{{Role: RoleUser, Content: "Say hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	final, err := AssembleFinal(ch)
	if err != nil {
		t.Fatalf("AssembleFinal: %v", err)
	}
	if final != "Hello Claude" {
		t.Fatalf("final=%q want Hello Claude", final)
	}
	if gotAPIKey != "anth-test-key-never-print" {
		t.Fatalf("x-api-key not sent to Anthropic host (got %q)", gotAPIKey)
	}
	if gotVersion != AnthropicAPIVersion {
		t.Fatalf("anthropic-version=%q", gotVersion)
	}
	if !strings.Contains(gotPath, "/v1/messages") {
		t.Fatalf("path=%q want /v1/messages", gotPath)
	}
	if hitTempAI.Load() != 0 {
		t.Fatalf("temp-ai contacted %d times", hitTempAI.Load())
	}
}

// YAML anthropic with empty key still registers; chat is blocked (VAL-PROV-008).
func TestAnthropicYAMLProfileNoKeyIsBlocked(t *testing.T) {
	t.Setenv(EnvAnthropicAPIKey, "")
	t.Setenv(EnvAPIKey, "temp-ok")
	t.Setenv(EnvBaseURL, DefaultTempAIBaseURL)

	file := config.FileConfig{
		Providers: []config.ProviderProfile{
			{
				ID:   "my-claude",
				Type: "anthropic",
				Name: "My Claude",
				// no key
			},
		},
	}
	r := Bootstrap(BootstrapOptions{File: file})
	c, err := r.GetProfile("my-claude")
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKeyConfigured() {
		t.Fatal("expected no key")
	}
	_, err = c.ChatStream(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: RoleUser, Content: "x"}},
	})
	if !IsBlocked(err) {
		t.Fatalf("want blocked, got %v", err)
	}
	// Default tempai still registered and independent.
	if _, err := r.Get(ProfileTempAI); err != nil {
		t.Fatal(err)
	}
}

func TestAnthropicNonStreamChatTurn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Error("missing x-api-key")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": "pong"},
			},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	c := NewAnthropic(Config{
		ID:      "anthropic",
		BaseURL: srv.URL,
		APIKey:  "k",
	})
	msg, err := c.ChatTurn(context.Background(), ChatRequest{
		Model:    "claude-test",
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "pong" {
		t.Fatalf("content=%q", msg.Content)
	}
}

func TestAnthropicToolRoundTripWire(t *testing.T) {
	// Ensure tool_result + tool_use convert without panic.
	sys, msgs := toAnthropicMessages([]Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "use tool"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{{
				ID:   "toolu_1",
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "write_file", Arguments: `{"path":"a.txt"}`},
			}},
		},
		{Role: RoleTool, ToolCallID: "toolu_1", Content: "ok", Name: "write_file"},
	})
	if sys != "sys" {
		t.Fatalf("system=%q", sys)
	}
	if len(msgs) < 3 {
		t.Fatalf("msgs len=%d", len(msgs))
	}
	// Last should be user with tool_result content blocks.
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Fatalf("last role=%s", last.Role)
	}
}

func TestBlockedErrorMessageClear(t *testing.T) {
	err := &BlockedError{Profile: "anthropic", Reason: "key required", Cause: ErrMissingAPIKey}
	s := err.Error()
	if !strings.Contains(s, "blocked") {
		t.Fatalf("missing blocked: %s", s)
	}
	if !strings.Contains(s, "key required") {
		t.Fatalf("missing key required: %s", s)
	}
	if !strings.Contains(s, "not proxied through temp-ai") {
		t.Fatalf("should clarify no temp-ai proxy: %s", s)
	}
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatal("should unwrap to ErrMissingAPIKey")
	}
}
