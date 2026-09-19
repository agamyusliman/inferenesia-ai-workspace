package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListModelsParsesData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" && r.URL.Path != "/models" {
			// baseURL in tests may include /v1
			if !strings.HasSuffix(r.URL.Path, "/models") {
				t.Errorf("unexpected path %s", r.URL.Path)
			}
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("missing Authorization bearer")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "grok-4.5", "owned_by": "temp-ai"},
				{"id": "glm-5.2", "owned_by": "temp-ai"},
			},
		})
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      "tempai",
		BaseURL: srv.URL + "/v1",
		APIKey:  "secret-key-value",
	})
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2", len(models))
	}
	if models[0].ID != "grok-4.5" {
		t.Fatalf("first model = %q", models[0].ID)
	}
}

func TestListModelsMissingKey(t *testing.T) {
	c := NewOpenAICompat(Config{BaseURL: "https://example.invalid/v1", APIKey: ""})
	_, err := c.ListModels(context.Background())
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey", err)
	}
	if strings.Contains(err.Error(), "Bearer") {
		t.Fatalf("error leaked bearer: %v", err)
	}
}

func TestListModelsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{BaseURL: srv.URL, APIKey: "bad-key-not-real"})
	_, err := c.ListModels(context.Background())
	if !IsAuth(err) {
		t.Fatalf("want auth error, got %v", err)
	}
	if strings.Contains(err.Error(), "bad-key-not-real") {
		t.Fatalf("error leaked key: %v", err)
	}
}

func TestResolveModelAutoSelectsFirstWithoutOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "auto-first"}, {"id": "auto-second"}},
		})
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{BaseURL: srv.URL, APIKey: "k"})
	id, err := c.ResolveModel(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if id != "auto-first" {
		t.Fatalf("id=%q, want auto-first", id)
	}
}

func TestResolveModelRespectsValidOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "a"}, {"id": "b-chosen"}},
		})
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{BaseURL: srv.URL, APIKey: "k", DefaultModel: "b-chosen"})
	id, err := c.ResolveModel(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if id != "b-chosen" {
		t.Fatalf("id=%q, want b-chosen", id)
	}
}

func TestResolveModelInvalidOverrideErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "only-real"}},
		})
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{BaseURL: srv.URL, APIKey: "k"})
	_, err := c.ResolveModel(context.Background(), "not-a-real-model")
	if err == nil {
		t.Fatal("expected error for invalid model")
	}
	var me *ModelError
	if !errors.As(err, &me) {
		t.Fatalf("want ModelError, got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "not-a-real-model") {
		t.Fatalf("error should name invalid id: %v", err)
	}
}

func TestChatStreamDeltasAndFinal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		write := func(s string) {
			_, _ = io.WriteString(w, s)
			if flusher != nil {
				flusher.Flush()
			}
		}
		write("data: {\"choices\":[{\"delta\":{\"content\":\"PO\"}}]}\n\n")
		write("data: {\"choices\":[{\"delta\":{\"content\":\"NG\"}}]}\n\n")
		write("data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{BaseURL: srv.URL, APIKey: "k"})
	ch, err := c.ChatStream(context.Background(), ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
	var final string
	for ev := range ch {
		switch ev.Type {
		case EventDelta:
			deltas = append(deltas, ev.Delta)
		case EventDone:
			final = ev.Final
		case EventError:
			t.Fatalf("stream error: %v", ev.Err)
		}
	}
	if strings.Join(deltas, "") != "PONG" {
		t.Fatalf("deltas=%v", deltas)
	}
	if final != "PONG" {
		t.Fatalf("final=%q", final)
	}
	assembled, err := AssembleFinal(func() <-chan StreamEvent {
		// re-run for AssembleFinal helper coverage
		ch2, err := c.ChatStream(context.Background(), ChatRequest{
			Model:    "m",
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return ch2
	}())
	if err != nil || assembled != "PONG" {
		t.Fatalf("AssembleFinal = %q, %v", assembled, err)
	}
}

func TestChatStreamCancelViaContext(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		close(started)
		// Hold open until client disconnects.
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{BaseURL: srv.URL, APIKey: "k"})
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.ChatStream(ctx, ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	// Drain; should complete promptly.
	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream did not close after cancel")
		}
	}
}

func TestUnreachableBaseURLFailsFast(t *testing.T) {
	// Reserved TEST-NET address that should not route (or refuse quickly).
	// Combined with short timeout ensures <30s.
	c := NewOpenAICompat(Config{
		BaseURL:            "http://172.16.254.254:9/v1",
		APIKey:             "k",
		HTTPTimeoutSeconds: 2,
	})
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.ListModels(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if elapsed > 15*time.Second {
		t.Fatalf("took too long: %v", elapsed)
	}
	if !IsConnection(err) && !errors.Is(err, context.DeadlineExceeded) {
		// Accept raw timeout classified as connection.
		if !strings.Contains(strings.ToLower(err.Error()), "timeout") &&
			!strings.Contains(strings.ToLower(err.Error()), "refused") &&
			!strings.Contains(strings.ToLower(err.Error()), "unreachable") &&
			!strings.Contains(strings.ToLower(err.Error()), "connection") {
			t.Fatalf("unexpected err type: %v", err)
		}
	}
}

func TestChatCompleteNonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "hello final"}},
			},
		})
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{BaseURL: srv.URL, APIKey: "k"})
	text, err := c.ChatComplete(context.Background(), ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello final" {
		t.Fatalf("text=%q", text)
	}
}

func TestTempAIConfigFromEnv(t *testing.T) {
	t.Setenv(EnvBaseURL, "https://ai.temp.web.id/v1")
	t.Setenv(EnvAPIKey, "test-key-not-real-xxxxxxxx")
	t.Setenv(EnvModel, "grok-4.5")
	cfg := TempAIConfigFromEnv()
	if cfg.ID != ProfileInferenesia {
		t.Fatalf("id=%q, want inferenesia", cfg.ID)
	}
	if cfg.Name != DefaultProfileName {
		t.Fatalf("name=%q, want %q", cfg.Name, DefaultProfileName)
	}
	if cfg.BaseURL != "https://ai.temp.web.id/v1" {
		t.Fatalf("base=%q", cfg.BaseURL)
	}
	if cfg.DefaultModel != "grok-4.5" {
		t.Fatalf("model=%q", cfg.DefaultModel)
	}
	if cfg.APIKey == "" {
		t.Fatal("api key empty")
	}
}

func TestTempAIConfigDefaultBaseURL(t *testing.T) {
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvAPIKey, "k")
	t.Setenv(EnvModel, "")
	cfg := TempAIConfigFromEnv()
	if cfg.BaseURL != DefaultTempAIBaseURL {
		t.Fatalf("base=%q", cfg.BaseURL)
	}
	if cfg.DefaultModel != "" {
		t.Fatalf("model should be empty, got %q", cfg.DefaultModel)
	}
}

func TestRouterBootstrap(t *testing.T) {
	t.Setenv(EnvAPIKey, "k")
	t.Setenv(EnvBaseURL, "https://example.invalid/v1")
	r := BootstrapDefault()
	c, err := r.Get("")
	if err != nil {
		t.Fatal(err)
	}
	// Name() returns the profile id; the display name lives on Config.Name.
	if c.Name() != ProfileInferenesia {
		t.Fatalf("name=%q, want %q", c.Name(), ProfileInferenesia)
	}
	if oc, ok := c.(*OpenAICompat); ok {
		if oc.cfg.Name != DefaultProfileName {
			t.Fatalf("cfg.Name=%q, want %q", oc.cfg.Name, DefaultProfileName)
		}
	}
}

// Legacy env (TEMP_AI_*) still resolves to the Inferenesia profile (dual-read).
func TestTempAIConfigFromLegacyEnv(t *testing.T) {
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvModel, "")
	t.Setenv(EnvBaseURLLegacy, "https://ai.temp.web.id/v1")
	t.Setenv(EnvAPIKeyLegacy, "legacy-key-not-real")
	t.Setenv(EnvModelLegacy, "legacy-model")
	cfg := TempAIConfigFromEnv()
	if cfg.ID != ProfileInferenesia {
		t.Fatalf("id=%q, want inferenesia", cfg.ID)
	}
	if cfg.BaseURL != "https://ai.temp.web.id/v1" {
		t.Fatalf("base=%q (legacy env should be honored)", cfg.BaseURL)
	}
	if cfg.APIKey != "legacy-key-not-real" {
		t.Fatalf("api key=%q (legacy env should be honored)", cfg.APIKey)
	}
	if cfg.DefaultModel != "legacy-model" {
		t.Fatalf("model=%q (legacy env should be honored)", cfg.DefaultModel)
	}
}

// New env (INFERENESIA_*) wins over legacy (TEMP_AI_*).
func TestTempAIConfigNewEnvWinsOverLegacy(t *testing.T) {
	t.Setenv(EnvBaseURL, "https://new.inferenesia/v1")
	t.Setenv(EnvAPIKey, "new-key")
	t.Setenv(EnvModel, "new-model")
	t.Setenv(EnvBaseURLLegacy, "https://legacy/v1")
	t.Setenv(EnvAPIKeyLegacy, "legacy-key")
	t.Setenv(EnvModelLegacy, "legacy-model")
	cfg := TempAIConfigFromEnv()
	if cfg.BaseURL != "https://new.inferenesia/v1" {
		t.Fatalf("base=%q (new env should win)", cfg.BaseURL)
	}
	if cfg.APIKey != "new-key" {
		t.Fatalf("api key=%q (new env should win)", cfg.APIKey)
	}
	if cfg.DefaultModel != "new-model" {
		t.Fatalf("model=%q (new env should win)", cfg.DefaultModel)
	}
}

// Legacy profile id "tempai" resolves via alias to the inferenesia client.
func TestRouterGetLegacyAlias(t *testing.T) {
	t.Setenv(EnvAPIKey, "k")
	t.Setenv(EnvBaseURL, "https://example.invalid/v1")
	r := BootstrapDefault()
	canon, err := r.Get(ProfileInferenesia)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := r.Get(ProfileTempAI)
	if err != nil {
		t.Fatalf("legacy alias tempai should resolve: %v", err)
	}
	if alias != canon {
		t.Fatalf("alias tempai should resolve to same client as inferenesia")
	}
}
