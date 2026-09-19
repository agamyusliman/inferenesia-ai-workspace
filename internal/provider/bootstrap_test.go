package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// VAL-CLI-011: BYOK profile targets user base_url host, not ai.temp.web.id; key never in requests to temp-ai.
func TestBYOKChatTargetsUserBaseURLNotTempAI(t *testing.T) {
	var gotAuth string
	var gotPath string
	var hitTempAI atomic.Int32

	byokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "byok-model-1"}},
			})
			return
		}
		// chat/completions non-stream
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "BYOK-OK"}},
			},
		})
	}))
	defer byokSrv.Close()

	// Any hit to this server during BYOK run is a fail.
	tempFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitTempAI.Add(1)
		t.Errorf("BYOK request incorrectly hit temp-ai-like server path=%s auth_set=%v", r.URL.Path, r.Header.Get("Authorization") != "")
		w.WriteHeader(http.StatusTeapot)
	}))
	defer tempFake.Close()

	// Clear gateway env so dual mode can prefer BYOK.
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvBaseURL, tempFake.URL+"/v1")
	t.Setenv(EnvModel, "")
	t.Setenv(EnvBYOKBaseURL, byokSrv.URL+"/v1")
	t.Setenv(EnvBYOKAPIKey, "byok-test-key-value-never-print")
	t.Setenv(EnvBYOKModel, "byok-model-1")

	r := Bootstrap(BootstrapOptions{PreferredID: ProfileBYOK})
	if r.DefaultID() != ProfileBYOK {
		t.Fatalf("default=%q, want byok", r.DefaultID())
	}
	client, err := r.GetOpenAI(ProfileBYOK)
	if err != nil {
		t.Fatal(err)
	}

	host := client.RequestHost()
	if IsTempAIHost(host) {
		t.Fatalf("BYOK host should not be temp-ai, got %q", host)
	}
	if !strings.Contains(host, "127.0.0.1") && !strings.Contains(host, "localhost") {
		// httptest uses 127.0.0.1
		t.Fatalf("expected local mock host, got %q", host)
	}
	if strings.Contains(host, "byok-test-key") {
		t.Fatalf("RequestHost leaked key: %q", host)
	}

	text, err := client.ChatComplete(context.Background(), ChatRequest{
		Model:    "byok-model-1",
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("ChatComplete: %v", err)
	}
	if text != "BYOK-OK" {
		t.Fatalf("text=%q", text)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("missing Authorization on BYOK host, path=%s auth=%q", gotPath, gotAuth)
	}
	if !strings.Contains(gotAuth, "byok-test-key-value-never-print") {
		t.Fatalf("Authorization did not use BYOK key on user base_url")
	}
	if hitTempAI.Load() != 0 {
		t.Fatalf("temp-ai host was contacted %d times during BYOK chat", hitTempAI.Load())
	}
}

// VAL-CLI-011 with YAML profile (distinct base_url).
func TestBYOKYAMLProfileRoutesToUserBaseURL(t *testing.T) {
	var sawHostPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHostPath = r.Host + r.URL.Path
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "yaml-model"}},
			})
			return
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "yaml-byok-secret") {
			t.Errorf("request body must not contain API key")
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer yaml-byok-secret" {
			t.Errorf("Authorization = %q", auth)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "YAML-OK"}},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvBYOKBaseURL, "")
	t.Setenv(EnvBYOKAPIKey, "")
	t.Setenv("CUSTOM_BYOK_KEY", "yaml-byok-secret")

	file := config.FileConfig{
		DefaultProvider: "custom-byok",
		Providers: []config.ProviderProfile{
			{
				ID:        "custom-byok",
				Type:      "openai_compatible",
				Name:      "Custom",
				BaseURL:   srv.URL + "/v1",
				APIKeyEnv: "CUSTOM_BYOK_KEY",
			},
		},
	}
	r := Bootstrap(BootstrapOptions{File: file})
	if r.DefaultID() != "custom-byok" {
		t.Fatalf("default=%q", r.DefaultID())
	}
	c, err := r.GetOpenAI("custom-byok")
	if err != nil {
		t.Fatal(err)
	}
	if IsTempAIHost(c.RequestHost()) {
		t.Fatalf("host is temp-ai: %s", c.RequestHost())
	}
	out, err := c.ChatComplete(context.Background(), ChatRequest{
		Model:    "yaml-model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "YAML-OK" {
		t.Fatalf("out=%q", out)
	}
	if !strings.Contains(sawHostPath, "/v1/chat/completions") && !strings.Contains(sawHostPath, "/chat/completions") {
		t.Fatalf("unexpected path host+path=%q", sawHostPath)
	}
}

// VAL-CLI-013: BYOK-only bootstrap (TEMP_AI_API_KEY unset) routes default to byok.
func TestDualModeBYOKOnlyBootstrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "tool-model"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "BYOK-TOOLS-OK"}},
			},
		})
	}))
	defer srv.Close()

	// BYOK only — no TEMP_AI_API_KEY.
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvModel, "")
	t.Setenv(EnvBYOKBaseURL, srv.URL+"/v1")
	t.Setenv(EnvBYOKAPIKey, "only-byok-key")
	t.Setenv(EnvBYOKModel, "tool-model")

	r := Bootstrap(BootstrapOptions{})
	if r.DefaultID() != ProfileBYOK {
		t.Fatalf("default=%q want byok (temp-ai key unset)", r.DefaultID())
	}
	client, err := r.GetOpenAI("")
	if err != nil {
		t.Fatal(err)
	}
	if IsTempAIHost(client.RequestHost()) {
		t.Fatalf("should not target temp-ai host: %s", client.RequestHost())
	}
	if !client.APIKeyConfigured() {
		t.Fatal("BYOK key should be configured")
	}
	text, err := client.ChatComplete(context.Background(), ChatRequest{
		Model:    "tool-model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "BYOK-TOOLS-OK" {
		t.Fatalf("text=%q", text)
	}
}

// VAL-CLI-013: temp-ai-only still works (no BYOK env).
func TestDualModeTempAIOnlyBootstrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "temp-model"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "TEMP-OK"}},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(EnvBYOKBaseURL, "")
	t.Setenv(EnvBYOKAPIKey, "")
	t.Setenv(EnvAPIKey, "only-temp-ai-key")
	t.Setenv(EnvBaseURL, srv.URL+"/v1")
	t.Setenv(EnvModel, "temp-model")

	r := Bootstrap(BootstrapOptions{})
	if r.DefaultID() != ProfileInferenesia {
		t.Fatalf("default=%q want inferenesia", r.DefaultID())
	}
	// byok must not be registered without base url.
	if _, err := r.Get(ProfileBYOK); err == nil {
		t.Fatal("byok should not be registered when BYOK_BASE_URL unset")
	}
	c, err := r.GetOpenAI(ProfileInferenesia)
	if err != nil {
		t.Fatal(err)
	}
	// Legacy alias tempai should resolve to the same client.
	alias, err := r.GetOpenAI(ProfileTempAI)
	if err != nil {
		t.Fatalf("legacy alias tempai should resolve: %v", err)
	}
	if alias != c {
		t.Fatalf("tempai alias should resolve to same client as inferenesia")
	}
	text, err := c.ChatComplete(context.Background(), ChatRequest{
		Model:    "temp-model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "TEMP-OK" {
		t.Fatalf("text=%q", text)
	}
}

func TestRequestHostStripsPathAndNeverIncludesKey(t *testing.T) {
	h := RequestHost("https://api.openai.com/v1")
	if h != "https://api.openai.com" {
		t.Fatalf("got %q", h)
	}
	if IsTempAIHost("https://api.openai.com/v1") {
		t.Fatal("openai is not temp-ai")
	}
	if !IsTempAIHost(DefaultTempAIBaseURL) {
		t.Fatal("default gateway should match temp-ai host")
	}
	if !IsTempAIHost("https://ai.temp.web.id/v1/chat/completions") {
		t.Fatal("expected temp-ai match")
	}
}

func TestBootstrapPreferCLIProfile(t *testing.T) {
	t.Setenv(EnvAPIKey, "temp-key")
	t.Setenv(EnvBaseURL, DefaultTempAIBaseURL)
	t.Setenv(EnvBYOKBaseURL, "http://127.0.0.1:4112/v1")
	t.Setenv(EnvBYOKAPIKey, "byok-key")

	r := Bootstrap(BootstrapOptions{PreferredID: ProfileBYOK})
	if r.DefaultID() != ProfileBYOK {
		t.Fatalf("default=%q", r.DefaultID())
	}
	c, _ := r.GetOpenAI("")
	if IsTempAIHost(c.RequestHost()) {
		t.Fatalf("preferred byok should not target temp-ai: %s", c.RequestHost())
	}
}
