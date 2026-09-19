package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// P9-prov: a custom openai_compatible host with api_format=anthropic must
// register the Anthropic adapter (not OpenAICompat), so the Messages API wire
// format is used against the Claude-compatible gateway.
func TestBootstrapRoutesOpenAICompatibleWithAnthropicFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-mock"}]}`))
			return
		}
		if !strings.Contains(r.URL.Path, "/v1/messages") {
			t.Errorf("expected /v1/messages path for anthropic wire, got %s", r.URL.Path)
		}
		// Echo back an anthropic-shaped response so ChatTurn succeeds.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":      []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvBYOKBaseURL, "")
	t.Setenv(EnvAnthropicAPIKey, "")

	file := config.FileConfig{
		Providers: []config.ProviderProfile{
			{
				ID:        "litellm-claude",
				Type:      "openai_compatible",
				APIFormat: "anthropic",
				Name:      "LiteLLM Claude",
				BaseURL:   srv.URL,
				APIKey:    "litellm-test-key",
			},
		},
	}
	r := Bootstrap(BootstrapOptions{File: file})

	// Must be an *Anthropic adapter (the wire format), not *OpenAICompat.
	c, err := r.GetProfile("litellm-claude")
	if err != nil {
		t.Fatal(err)
	}
	if AdapterType(c) != TypeAnthropic {
		t.Fatalf("expected Anthropic adapter for openai_compatible+api_format=anthropic, got %s", AdapterType(c))
	}
	// Sanity: a non-stream turn routes to /v1/messages.
	msg, err := c.ChatTurn(context.Background(), ChatRequest{
		Model:    "claude-mock",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatTurn: %v", err)
	}
	if msg.Content != "ok" {
		t.Fatalf("content=%q", msg.Content)
	}
}

// P9-prov: openai_compatible with api_format=openai still routes to OpenAICompat.
func TestBootstrapRoutesOpenAICompatibleWithOpenAIFormat(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvBYOKBaseURL, "")
	file := config.FileConfig{
		Providers: []config.ProviderProfile{
			{
				ID:        "plain-openai",
				Type:      "openai_compatible",
				APIFormat: "openai",
				Name:      "Plain",
				BaseURL:   "http://127.0.0.1:4199/v1",
				APIKey:    "k",
			},
		},
	}
	r := Bootstrap(BootstrapOptions{File: file})
	c, err := r.GetProfile("plain-openai")
	if err != nil {
		t.Fatal(err)
	}
	if AdapterType(c) != TypeOpenAICompatible {
		t.Fatalf("expected OpenAICompat, got %s", AdapterType(c))
	}
}

// P9-prov: Anthropic prompt caching — system is emitted as an array of text
// blocks with cache_control: ephemeral, and the beta header is sent.
func TestAnthropicSystemBlocksHaveCacheControlEphemeral(t *testing.T) {
	var gotBeta string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBeta = r.Header.Get("anthropic-beta")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":      []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	c := NewAnthropic(Config{
		ID:      "anthropic",
		BaseURL: srv.URL,
		APIKey:  "k",
	})
	_, err := c.ChatTurn(context.Background(), ChatRequest{
		Model: "claude-test",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are a helpful coding agent."},
			{Role: RoleUser, Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("ChatTurn: %v", err)
	}

	if !strings.Contains(gotBeta, AnthropicPromptCachingBeta) {
		t.Fatalf("missing anthropic-beta prompt-caching header: %q", gotBeta)
	}

	var probe struct {
		System []struct {
			Type         string `json:"type"`
			Text         string `json:"text"`
			CacheControl *struct {
				Type string `json:"type"`
			} `json:"cache_control"`
		} `json:"system"`
	}
	if err := json.Unmarshal(gotBody, &probe); err != nil {
		t.Fatalf("unmarshal body: %v\nbody=%s", err, string(gotBody))
	}
	if len(probe.System) == 0 {
		t.Fatalf("expected system blocks, got none. body=%s", string(gotBody))
	}
	if probe.System[0].Type != "text" {
		t.Fatalf("system[0].type=%q want text", probe.System[0].Type)
	}
	if probe.System[0].Text == "" {
		t.Fatal("system[0].text empty")
	}
	if probe.System[0].CacheControl == nil {
		t.Fatal("expected cache_control on system block")
	}
	if probe.System[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("cache_control.type=%q want ephemeral", probe.System[0].CacheControl.Type)
	}
}

// P9-prov: when system is empty, the system field is omitted (no cache_control)
// and the prompt-caching beta header is NOT sent.
func TestAnthropicNoSystemOmitsCacheControlAndBetaHeader(t *testing.T) {
	var gotBeta string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBeta = r.Header.Get("anthropic-beta")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":      []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()

	c := NewAnthropic(Config{
		ID:      "anthropic",
		BaseURL: srv.URL,
		APIKey:  "k",
	})
	_, err := c.ChatTurn(context.Background(), ChatRequest{
		Model:    "claude-test",
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("ChatTurn: %v", err)
	}

	if gotBeta != "" {
		t.Fatalf("expected no anthropic-beta header when system empty, got %q", gotBeta)
	}
	if strings.Contains(string(gotBody), "cache_control") {
		t.Fatalf("body should not contain cache_control when system empty: %s", string(gotBody))
	}
	if strings.Contains(string(gotBody), `"system"`) {
		t.Fatalf("body should omit system field when empty: %s", string(gotBody))
	}
}
