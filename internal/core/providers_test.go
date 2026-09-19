package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// TestAddProviderPersistsBYOKYAML verifies VAL-PROV-003: add openai_compatible
// BYOK profile → config.yaml mode 0600, appears in profiles list (no key in API).
func TestAddProviderPersistsBYOKYAML(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.AddProvider(AddProviderRequest{
		ID:             "my-openai",
		Name:           "My OpenAI",
		Type:           "openai_compatible",
		BaseURL:        "https://api.openai.com/v1",
		APIKey:         "sk-test-not-real-secret",
		DefaultModel:   "",
		OnboardingPath: "byok",
	})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, p := range view.Profiles {
		if p.ID == "my-openai" {
			found = true
			if p.Name != "My OpenAI" {
				t.Fatalf("name=%q", p.Name)
			}
			if p.Type != "openai_compatible" {
				t.Fatalf("type=%q", p.Type)
			}
			if !p.HasKey {
				t.Fatal("expected HasKey")
			}
			if p.BaseURLMasked == "" || strings.Contains(p.BaseURLMasked, "sk-") {
				t.Fatalf("base_url_masked bad: %q", p.BaseURLMasked)
			}
			// Key must never appear in JSON-ish view.
			raw, _ := json.Marshal(p)
			if strings.Contains(string(raw), "sk-test") {
				t.Fatalf("API key leaked in profile view: %s", raw)
			}
		}
		if p.ID == provider.ProfileInferenesia {
			// Default inferenesia (gateway) still listed (VAL-PROV-001).
			if p.Name == "" {
				t.Fatal("inferenesia missing name")
			}
		}
	}
	if !found {
		t.Fatalf("BYOK profile missing from list: %+v", view.Profiles)
	}
	// inferenesia (gateway) always present.
	hasTemp := false
	for _, p := range view.Profiles {
		if p.ID == provider.ProfileInferenesia {
			hasTemp = true
		}
	}
	if !hasTemp {
		t.Fatal("expected default inferenesia profile in list")
	}

	cfgPath := filepath.Join(home, "config.yaml")
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode=%o want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "my-openai") {
		t.Fatalf("config missing profile id:\n%s", data)
	}
	if !strings.Contains(string(data), "api.openai.com") {
		t.Fatalf("config missing base_url:\n%s", data)
	}
	// Key stored locally (required by VAL-PROV-003) — not printed by handoff tooling.
	if !strings.Contains(string(data), "sk-test-not-real-secret") {
		t.Fatal("expected inline api_key persisted (local only)")
	}
	// Onboarding marked done after BYOK path.
	if !view.Onboarding.Done {
		t.Fatal("expected onboarding done after BYOK add")
	}
}

// TestGetSettingsListsTempAI verifies VAL-PROV-001 profile fields.
func TestGetSettingsListsTempAI(t *testing.T) {
	home := t.TempDir()
	// No TEMP_AI key required for list (discovery may error).
	t.Setenv("TEMP_AI_API_KEY", "")
	t.Setenv("TEMP_AI_BASE_URL", "https://ai.temp.web.id/v1")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	view := svc.GetSettings("")
	var temp *ProfileView
	for i := range view.Profiles {
		if view.Profiles[i].ID == provider.ProfileInferenesia {
			temp = &view.Profiles[i]
			break
		}
	}
	if temp == nil {
		t.Fatalf("inferenesia not listed: %+v", view.Profiles)
	}
	if temp.Name == "" {
		t.Fatal("inferenesia name empty")
	}
	if temp.BaseURLMasked == "" && temp.BaseHost == "" {
		t.Fatal("expected masked base url or host")
	}
	if strings.Contains(temp.BaseURLMasked, "sk-") {
		t.Fatalf("key in base: %q", temp.BaseURLMasked)
	}
}

// TestModelPickerRefreshesPerProfile verifies VAL-PROV-002: ListModels is called
// against the selected profile base URL (mock servers, not hard-coded ids).
func TestModelPickerRefreshesPerProfile(t *testing.T) {
	home := t.TempDir()

	var hitsA, hitsB atomic.Int32
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			hitsA.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"model-a1"},{"id":"model-a2"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			hitsB.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"model-b-only"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srvB.Close()

	// Write two BYOK profiles pointing at the mocks.
	cfg := config.FileConfig{
		Version:         1,
		DefaultProvider: "mock-a",
		Providers: []config.ProviderProfile{
			{ID: "mock-a", Type: "openai_compatible", Name: "Mock A", BaseURL: srvA.URL + "/v1", APIKey: "key-a"},
			{ID: "mock-b", Type: "openai_compatible", Name: "Mock B", BaseURL: srvB.URL + "/v1", APIKey: "key-b"},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	// Clear gateway key so default doesn't try live network for models list.
	t.Setenv("TEMP_AI_API_KEY", "")
	t.Setenv("TEMP_AI_BASE_URL", "https://ai.temp.web.id/v1")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	viewA := svc.GetSettings("mock-a")
	if viewA.SelectedProfile != "mock-a" {
		t.Fatalf("selected=%q", viewA.SelectedProfile)
	}
	if len(viewA.Models) < 1 {
		t.Fatalf("models empty a: err=%q", viewA.ModelsError)
	}
	if viewA.Models[0].ID != "model-a1" {
		t.Fatalf("want model-a1 got %q", viewA.Models[0].ID)
	}
	if hitsA.Load() < 1 {
		t.Fatal("expected /models hit on mock A")
	}

	viewB := svc.GetSettings("mock-b")
	if viewB.SelectedProfile != "mock-b" {
		t.Fatalf("selected b=%q", viewB.SelectedProfile)
	}
	if len(viewB.Models) != 1 || viewB.Models[0].ID != "model-b-only" {
		t.Fatalf("models b=%+v err=%q", viewB.Models, viewB.ModelsError)
	}
	if hitsB.Load() < 1 {
		t.Fatal("expected /models hit on mock B after switch")
	}
}

// TestProviderTestCallSuccess verifies VAL-PROV-004 against a mock chat server.
func TestProviderTestCallSuccess(t *testing.T) {
	home := t.TempDir()
	var chatHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"mock-chat-1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			chatHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "mock-test", Type: "openai_compatible", Name: "Mock Test", BaseURL: srv.URL + "/v1", APIKey: "k"},
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
	res := svc.TestProvider("mock-test")
	if !res.OK {
		t.Fatalf("test failed: %+v", res)
	}
	if res.Model != "mock-chat-1" {
		t.Fatalf("model=%q", res.Model)
	}
	if chatHits.Load() < 1 {
		t.Fatal("expected chat completion call")
	}
	if strings.Contains(res.Preview, "k") && res.Preview != "ok" {
		// preview is assistant text only
	}
	if res.Error != "" {
		t.Fatalf("error=%q", res.Error)
	}
}

// TestProviderTestCallAuthError verifies clear error without key leak (VAL-PROV-004).
func TestProviderTestCallAuthError(t *testing.T) {
	home := t.TempDir()
	badKey := "sk-bad-key-value-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()
	cfg := config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{ID: "bad", Type: "openai_compatible", Name: "Bad", BaseURL: srv.URL + "/v1", APIKey: badKey},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.TestProvider("bad")
	if res.OK {
		t.Fatal("expected failure")
	}
	if res.Error == "" {
		t.Fatal("expected error message")
	}
	if strings.Contains(res.Error, badKey) {
		t.Fatalf("key leaked in error: %q", res.Error)
	}
}

// TestOnboardingDualPaths verifies VAL-PROV-010: both paths listed; neither requires the other.
func TestOnboardingDualPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TEMP_AI_API_KEY", "")
	t.Setenv("YURA_AI_BYOK_BASE_URL", "")
	t.Setenv("YURA_AI_BYOK_API_KEY", "")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	state := svc.GetOnboarding()
	if len(state.Paths) < 2 {
		t.Fatalf("want 2 paths, got %+v", state.Paths)
	}
	ids := map[string]bool{}
	for _, p := range state.Paths {
		ids[p.ID] = true
		if p.RequiresOther {
			t.Fatalf("path %s should not require the other", p.ID)
		}
	}
	if !ids[provider.ProfileInferenesia] || !ids["byok"] {
		t.Fatalf("missing paths: %+v", state.Paths)
	}
	// Completing BYOK alone marks done.
	_, err = svc.AddProvider(AddProviderRequest{
		ID:             "solo-byok",
		Name:           "Solo BYOK",
		BaseURL:        "http://127.0.0.1:4111/v1",
		APIKey:         "local-only-key",
		OnboardingPath: "byok",
	})
	if err != nil {
		t.Fatal(err)
	}
	state = svc.GetOnboarding()
	if !state.Done {
		t.Fatal("onboarding should be done after BYOK without temp-ai key")
	}
	if state.HasTempAIKey {
		// TEMP_AI may still be empty
	}
	if !state.HasBYOK {
		t.Fatal("expected HasBYOK")
	}
}

// TestSettingsHTTPAddAndTest routes VAL-PROV via desktop hub.
func TestSettingsHTTPAddAndTest(t *testing.T) {
	home := t.TempDir()
	var chatHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			chatHits.Add(1)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	body := map[string]any{
		"id":       "http-byok",
		"name":     "HTTP BYOK",
		"type":     "openai_compatible",
		"base_url": srv.URL + "/v1",
		"api_key":  "k",
	}
	raw, _ := json.Marshal(body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/providers", strings.NewReader(string(raw))))
	if rr.Code != 200 {
		t.Fatalf("add %d %s", rr.Code, rr.Body.String())
	}
	var view SettingsView
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range view.Profiles {
		if p.ID == "http-byok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("profile not in response: %+v", view.Profiles)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/providers/test", strings.NewReader(`{"profile":"http-byok"}`)))
	if rr.Code != 200 {
		t.Fatalf("test %d %s", rr.Code, rr.Body.String())
	}
	var res TestProviderResult
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("test result: %+v", res)
	}
	if chatHits.Load() < 1 {
		t.Fatal("chat not called")
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/onboarding", nil))
	if rr.Code != 200 {
		t.Fatalf("onboarding %d", rr.Code)
	}
}
