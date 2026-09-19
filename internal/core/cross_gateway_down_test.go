package core_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// VAL-CROSS-009: gateway down → clear brand-correct error.
//
// Cross-area flow at the core.Service layer: when TEMP_AI_BASE_URL is
// unreachable or returns 401/5xx, ChatStream must:
//
//  1. fail fast (within timeout <= 10s) — no hang, no fake success;
//  2. emit an Error ChatEvent whose error string, after
//     provider.WrapGatewayErrorIfProfile, contains "gateway"/"unavailable"
//     and "Inferenesia" (brand-correct);
//  3. emit a Blocked ChatEvent so the agent loop / desktop UI can offer a
//     retry/blocked path (non-blocking error toast when the surface exists);
//  4. never print the API key value in any event payload;
//  5. leave non-tempai profiles (BYOK / Anthropic) with their existing
//     provider-specific errors (no spurious Blocked event).
//
// The test runs three scenarios against the tempai profile:
//   - dead localhost port (unreachable) → fail-fast + gateway wrap
//   - 401 unauthorized → gateway wrap with HTTP 401
//   - 5xx server error → gateway wrap with server-error reason
//
// A fourth scenario (BYOK profile at an unreachable host) verifies the
// non-tempai path keeps the original ConnectionError and emits no Blocked
// event (provider isolation — VAL-CLI-013).

func TestCrossArea_GatewayDownClearError_DeadLocalhostPort(t *testing.T) {
	// Point TEMP_AI_BASE_URL at a dead localhost port (port 1 is reserved).
	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	const apiKey = "dead-port-secret-key-XYZ"
	t.Setenv(provider.EnvBaseURL, "http://127.0.0.1:1/v1")
	t.Setenv(provider.EnvAPIKey, apiKey)
	t.Setenv(provider.EnvModel, "any-model")
	// Avoid dotenv re-injection from the repo .env (test isolation).
	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var (
		mu          sync.Mutex
		blockedEv   *core.ChatEvent
		errorEv     *core.ChatEvent
		started     = time.Now()
		doneCh      = make(chan error, 1)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		err := svc.ChatStream(ctx, core.ChatRequest{
			Prompt:  "ping",
			NoTools: true,
		}, func(ev core.ChatEvent) {
			mu.Lock()
			defer mu.Unlock()
			if ev.Type == core.ChatEventBlocked && blockedEv == nil {
				b := ev
				blockedEv = &b
			}
			if ev.Type == core.ChatEventError && errorEv == nil {
				e := ev
				errorEv = &e
			}
		})
		doneCh <- err
	}()

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected error for unreachable gateway, got nil (fake success)")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ChatStream hung > 10s on unreachable gateway (VAL-CROSS-009)")
	}
	elapsed := time.Since(started)
	if elapsed > 10*time.Second {
		t.Fatalf("took %v, want <= 10s", elapsed)
	}

	mu.Lock()
	defer mu.Unlock()

	// (1) Error event must be present.
	if errorEv == nil {
		t.Fatal("missing Error ChatEvent on gateway-down path")
	}
	// (2) Error string must contain "gateway"/"unavailable" after wrap and must
	// not contain the API key.
	// The ChatStream Error event carries the raw provider error string. The
	// brand-correct wrap happens at provider.WrapGatewayErrorIfProfile, which
	// the CLI layer applies. At the core layer we verify the Blocked event
	// (which IS the brand-correct wrapped form) below.
	if strings.Contains(errorEv.Error, apiKey) {
		t.Fatalf("Error event leaked API key: %q", errorEv.Error)
	}

	// (3) Blocked event must be present with brand-correct gateway wording.
	if blockedEv == nil {
		t.Fatal("missing Blocked ChatEvent on gateway-down path (VAL-CROSS-009)")
	}
	blockedText := blockedEv.Error
	if blockedText == "" {
		blockedText = blockedEv.Text
	}
	if !strings.Contains(blockedText, "gateway") {
		t.Errorf("Blocked event missing 'gateway': %q", blockedText)
	}
	if !strings.Contains(blockedText, "unavailable") {
		t.Errorf("Blocked event missing 'unavailable': %q", blockedText)
	}
	if !strings.Contains(blockedText, "Inferenesia") {
		t.Errorf("Blocked event missing brand 'Inferenesia': %q", blockedText)
	}
	if !strings.Contains(blockedText, "Inferenesia API") {
		t.Errorf("Blocked event missing provider label 'Inferenesia API': %q", blockedText)
	}
	// (4) API key must never appear in the Blocked event payload.
	if strings.Contains(blockedText, apiKey) {
		t.Fatalf("Blocked event leaked API key: %q", blockedText)
	}
	if blockedEv.Host != "" && strings.Contains(blockedEv.Host, apiKey) {
		t.Fatalf("Blocked event Host leaked API key: %q", blockedEv.Host)
	}
}

func TestCrossArea_GatewayDownClearError_401Unauthorized(t *testing.T) {
	// Mock gateway returns 401 for every endpoint.
	const apiKey = "tempai-401-secret-key-XYZ"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never echo the key back.
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer srv.Close()

	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(provider.EnvBaseURL, srv.URL+"/v1")
	t.Setenv(provider.EnvAPIKey, apiKey)
	t.Setenv(provider.EnvModel, "any-model")
	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var (
		mu        sync.Mutex
		blockedEv *core.ChatEvent
		errorEv   *core.ChatEvent
		doneCh    = make(chan error, 1)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		err := svc.ChatStream(ctx, core.ChatRequest{
			Prompt:  "ping",
			NoTools: true,
		}, func(ev core.ChatEvent) {
			mu.Lock()
			defer mu.Unlock()
			if ev.Type == core.ChatEventBlocked && blockedEv == nil {
				b := ev
				blockedEv = &b
			}
			if ev.Type == core.ChatEventError && errorEv == nil {
				e := ev
				errorEv = &e
			}
		})
		doneCh <- err
	}()
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		t.Fatal("ChatStream hung > 10s on 401 path")
	}
	mu.Lock()
	defer mu.Unlock()
	if errorEv == nil {
		t.Fatal("missing Error ChatEvent on 401 path")
	}
	if blockedEv == nil {
		t.Fatal("missing Blocked ChatEvent on 401 path (VAL-CROSS-009)")
	}
	blockedText := blockedEv.Error
	if blockedText == "" {
		blockedText = blockedEv.Text
	}
	if !strings.Contains(blockedText, "gateway") || !strings.Contains(blockedText, "unavailable") {
		t.Errorf("Blocked event missing gateway/unavailable: %q", blockedText)
	}
	if !strings.Contains(blockedText, "HTTP 401") {
		t.Errorf("Blocked event should mention HTTP 401: %q", blockedText)
	}
	if strings.Contains(blockedText, apiKey) {
		t.Fatalf("Blocked event leaked API key: %q", blockedText)
	}
}

func TestCrossArea_GatewayDownClearError_5xxServerError(t *testing.T) {
	// Mock gateway returns 503 for every endpoint.
	const apiKey = "tempai-5xx-secret-key-XYZ"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream gateway down"}}`)
	}))
	defer srv.Close()

	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(provider.EnvBaseURL, srv.URL+"/v1")
	t.Setenv(provider.EnvAPIKey, apiKey)
	t.Setenv(provider.EnvModel, "any-model")
	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var (
		mu        sync.Mutex
		blockedEv *core.ChatEvent
		errorEv   *core.ChatEvent
		doneCh    = make(chan error, 1)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		err := svc.ChatStream(ctx, core.ChatRequest{
			Prompt:  "ping",
			NoTools: true,
		}, func(ev core.ChatEvent) {
			mu.Lock()
			defer mu.Unlock()
			if ev.Type == core.ChatEventBlocked && blockedEv == nil {
				b := ev
				blockedEv = &b
			}
			if ev.Type == core.ChatEventError && errorEv == nil {
				e := ev
				errorEv = &e
			}
		})
		doneCh <- err
	}()
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		t.Fatal("ChatStream hung > 10s on 5xx path")
	}
	mu.Lock()
	defer mu.Unlock()
	if errorEv == nil {
		t.Fatal("missing Error ChatEvent on 5xx path")
	}
	if blockedEv == nil {
		t.Fatal("missing Blocked ChatEvent on 5xx path (VAL-CROSS-009)")
	}
	blockedText := blockedEv.Error
	if blockedText == "" {
		blockedText = blockedEv.Text
	}
	if !strings.Contains(blockedText, "gateway") || !strings.Contains(blockedText, "unavailable") {
		t.Errorf("Blocked event missing gateway/unavailable: %q", blockedText)
	}
	if strings.Contains(blockedText, apiKey) {
		t.Fatalf("Blocked event leaked API key: %q", blockedText)
	}
}

// TestCrossArea_GatewayDownClearError_BYOKNoBlockedEvent verifies the
// non-tempai path: a BYOK profile at an unreachable host keeps its original
// provider error and does NOT emit a Blocked event (only tempai is the
// mission-required gateway; BYOK stays isolated — VAL-CLI-013).
func TestCrossArea_GatewayDownClearError_BYOKNoBlockedEvent(t *testing.T) {
	const byokKey = "byok-secret-key-XYZ"
	home := t.TempDir()
	// Register a BYOK profile at a dead localhost port via YAML config.
	if err := config.SaveToDir(home, config.FileConfig{
		Version:         1,
		DefaultProvider: "byok-dead",
		Providers: []config.ProviderProfile{
			{
				ID:           "byok-dead",
				Type:         "openai_compatible",
				Name:         "BYOK Dead",
				BaseURL:      "http://127.0.0.1:1/v1",
				APIKey:       byokKey,
				DefaultModel: "byok-model",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Ensure tempai is NOT the default and has no key so it doesn't get picked.
	t.Setenv(provider.EnvAPIKey, "")
	t.Setenv(provider.EnvBaseURL, "")
	t.Setenv(provider.EnvModel, "")
	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var (
		mu        sync.Mutex
		blockedEv *core.ChatEvent
		errorEv   *core.ChatEvent
		doneCh    = make(chan error, 1)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		err := svc.ChatStream(ctx, core.ChatRequest{
			Prompt:  "ping",
			NoTools: true,
		}, func(ev core.ChatEvent) {
			mu.Lock()
			defer mu.Unlock()
			if ev.Type == core.ChatEventBlocked && blockedEv == nil {
				b := ev
				blockedEv = &b
			}
			if ev.Type == core.ChatEventError && errorEv == nil {
				e := ev
				errorEv = &e
			}
		})
		doneCh <- err
	}()
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		t.Fatal("ChatStream hung > 10s on BYOK unreachable path")
	}
	mu.Lock()
	defer mu.Unlock()
	// BYOK: an Error event is expected (the connection failed) but NO Blocked
	// event (BYOK is not the temp-ai gateway profile).
	if errorEv == nil {
		t.Fatal("missing Error ChatEvent on BYOK unreachable path")
	}
	if strings.Contains(errorEv.Error, byokKey) {
		t.Fatalf("BYOK Error event leaked key: %q", errorEv.Error)
	}
	if blockedEv != nil {
		t.Fatalf("BYOK profile should NOT emit a Blocked event (only tempai does); got: %+v", blockedEv)
	}
}

// TestCrossArea_GatewayDownClearError_MissingKeyNoBlockedEvent verifies that a
// missing API key on the tempai profile does NOT emit a Blocked event (the
// gateway is not "down", just unconfigured — VAL-CLI-008 / VAL-PROV-008).
// The Error event still fires with a clear "key required" message.
func TestCrossArea_GatewayDownClearError_MissingKeyNoBlockedEvent(t *testing.T) {
	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	// tempai profile with NO API key (missing-key path).
	t.Setenv(provider.EnvBaseURL, "https://ai.temp.web.id/v1")
	t.Setenv(provider.EnvAPIKey, "")
	t.Setenv(provider.EnvModel, "")
	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var (
		mu        sync.Mutex
		blockedEv *core.ChatEvent
		errorEv   *core.ChatEvent
		doneCh    = make(chan error, 1)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		err := svc.ChatStream(ctx, core.ChatRequest{
			Prompt:  "ping",
			NoTools: true,
		}, func(ev core.ChatEvent) {
			mu.Lock()
			defer mu.Unlock()
			if ev.Type == core.ChatEventBlocked && blockedEv == nil {
				b := ev
				blockedEv = &b
			}
			if ev.Type == core.ChatEventError && errorEv == nil {
				e := ev
				errorEv = &e
			}
		})
		doneCh <- err
	}()
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		t.Fatal("ChatStream hung > 10s on missing-key path")
	}
	mu.Lock()
	defer mu.Unlock()
	if errorEv == nil {
		t.Fatal("missing Error ChatEvent on missing-key path")
	}
	// Missing key must NOT emit a Blocked event (only transport/401/5xx do).
	if blockedEv != nil {
		t.Fatalf("missing-key path should NOT emit a Blocked event; got: %+v", blockedEv)
	}
	// Error message should mention missing key, NOT "gateway unavailable".
	lc := strings.ToLower(errorEv.Error)
	if !strings.Contains(lc, "missing") && !strings.Contains(lc, "key") {
		t.Fatalf("missing-key Error event should mention missing/key: %q", errorEv.Error)
	}
	if strings.Contains(lc, "gateway unavailable") {
		t.Fatalf("missing-key path should not say 'gateway unavailable': %q", errorEv.Error)
	}
}

// TestCrossArea_GatewayDownClearError_HTTPSSEStreamSurface verifies the SSE
// chat endpoint surfaces the Blocked event to the wire so the desktop UI can
// render the non-blocking error toast. The hub must not hang or fake success.
func TestCrossArea_GatewayDownClearError_HTTPSSEStreamSurface(t *testing.T) {
	const apiKey = "sse-gateway-secret-key-XYZ"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream gateway down"}}`)
	}))
	defer srv.Close()

	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(provider.EnvBaseURL, srv.URL+"/v1")
	t.Setenv(provider.EnvAPIKey, apiKey)
	t.Setenv(provider.EnvModel, "any-model")
	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	h := svc.Handler()
	body := strings.NewReader(`{"prompt":"ping","no_tools":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("SSE handler hung > 10s on 5xx gateway")
	}

	out := rr.Body.String()
	// SSE frames are "data: {json}\n\n". Look for a Blocked frame and an Error frame.
	if !strings.Contains(out, `"type":"Blocked"`) {
		t.Errorf("SSE response missing Blocked frame; body=%q", out)
	}
	if !strings.Contains(out, `"type":"Error"`) {
		t.Errorf("SSE response missing Error frame; body=%q", out)
	}
	// The Blocked frame payload must be brand-correct and never contain the key.
	if !strings.Contains(out, "gateway unavailable") {
		t.Errorf("SSE response missing 'gateway unavailable' text; body=%q", out)
	}
	if !strings.Contains(out, "Inferenesia") {
		t.Errorf("SSE response missing brand 'Inferenesia'; body=%q", out)
	}
	if strings.Contains(out, apiKey) {
		t.Fatalf("SSE response leaked API key; body=%q", out)
	}
	// Quick sanity: the body must be parseable as a sequence of SSE data frames.
	frameCount := strings.Count(out, "data: ")
	if frameCount == 0 {
		t.Fatalf("no SSE frames emitted; body=%q", out)
	}
	// Verify at least one frame decodes as a Blocked ChatEvent.
	lines := strings.Split(out, "\n")
	decBlocked := false
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if !strings.HasPrefix(ln, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(ln, "data: ")
		if payload == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		if ev["type"] == "Blocked" {
			decBlocked = true
			if msg, ok := ev["error"].(string); ok {
				if strings.Contains(msg, apiKey) {
					t.Fatalf("Blocked frame leaked API key: %q", msg)
				}
			}
		}
	}
	if !decBlocked {
		t.Errorf("no Blocked SSE frame decoded; body=%q", out)
	}
}
