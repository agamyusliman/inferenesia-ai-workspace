package core

import (
	"context"
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

// mockOpenAIServer is a dual /models + /chat/completions mock that records
// Authorization headers and which model ids were used.
type mockOpenAIServer struct {
	t         *testing.T
	srv       *httptest.Server
	authSeen  atomic.Value // string
	modelSeen atomic.Value // string
	hits      atomic.Int32
	label     string
}

func startMockOpenAI(t *testing.T, label string, models []string, reply string) *mockOpenAIServer {
	t.Helper()
	m := &mockOpenAIServer{t: t, label: label}
	m.authSeen.Store("")
	m.modelSeen.Store("")
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.hits.Add(1)
		m.authSeen.Store(r.Header.Get("Authorization"))
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
			data := make([]map[string]string, 0, len(models))
			for _, id := range models {
				data = append(data, map[string]string{"id": id})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
			return
		}
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.modelSeen.Store(body.Model)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": reply}},
			},
		})
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockOpenAIServer) base() string { return m.srv.URL + "/v1" }

func (m *mockOpenAIServer) auth() string {
	v, _ := m.authSeen.Load().(string)
	return v
}

func (m *mockOpenAIServer) model() string {
	v, _ := m.modelSeen.Load().(string)
	return v
}

// TestMidSessionProviderSwitch verifies VAL-PROV-005 / VAL-PROV-011:
// switch active profile mid-session → next ChatStream uses the new host/model;
// history messages are preserved (no session restart / clear).
func TestMidSessionProviderSwitch(t *testing.T) {
	byokA := startMockOpenAI(t, "byok-a", []string{"model-a1", "model-a2"}, "reply-from-A")
	byokB := startMockOpenAI(t, "byok-b", []string{"model-b1"}, "reply-from-B")
	// Temp-ai mock: any BYOK key hitting this is a isolation fail.
	var tempHits atomic.Int32
	tempSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tempHits.Add(1)
		auth := r.Header.Get("Authorization")
		if strings.Contains(auth, "byok-secret") {
			t.Errorf("BYOK key incorrectly sent to temp-ai host: path=%s", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "temp-model"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "temp-reply"}},
			},
		})
	}))
	defer tempSrv.Close()

	home := t.TempDir()
	// Register two BYOK profiles in config (plus env tempai).
	cfg := config.FileConfig{
		Version:         1,
		DefaultProvider: "byok-a",
		Providers: []config.ProviderProfile{
			{
				ID:           "byok-a",
				Type:         "openai_compatible",
				Name:         "BYOK A",
				BaseURL:      byokA.base(),
				APIKey:       "byok-secret-A-key",
				DefaultModel: "model-a1",
			},
			{
				ID:           "byok-b",
				Type:         "openai_compatible",
				Name:         "BYOK B",
				BaseURL:      byokB.base(),
				APIKey:       "byok-secret-B-key",
				DefaultModel: "model-b1",
			},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	// Gateway points at temp mock so we can observe isolation.
	t.Setenv(provider.EnvBaseURL, tempSrv.URL+"/v1")
	t.Setenv(provider.EnvAPIKey, "temp-gateway-key-value")
	t.Setenv(provider.EnvModel, "temp-model")
	t.Setenv(provider.EnvBYOKBaseURL, "")
	t.Setenv(provider.EnvBYOKAPIKey, "")
	t.Setenv(provider.EnvBYOKModel, "")

	wsRoot := filepath.Join(home, "ws")
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	// Activate byok-a for session.
	act, err := svc.SetActiveProvider(SetActiveProviderRequest{
		Profile: "byok-a",
		Model:   "model-a1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if act.Profile != "byok-a" {
		t.Fatalf("active profile=%q", act.Profile)
	}
	if act.Model != "model-a1" {
		t.Fatalf("active model=%q", act.Model)
	}

	// Turn 1: uses byok-a (session active; no explicit req profile/model).
	var done1 ChatEvent
	err = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:  "hello A",
		NoTools: true,
	}, func(ev ChatEvent) {
		if ev.Type == ChatEventDone {
			done1 = ev
		}
	})
	if err != nil {
		t.Fatalf("chat turn 1: %v", err)
	}
	if byokA.hits.Load() == 0 {
		t.Fatal("expected byok-a mock to receive requests")
	}
	if !strings.Contains(byokA.auth(), "byok-secret-A-key") {
		t.Fatalf("byok-a auth=%q", byokA.auth())
	}
	if byokA.model() != "model-a1" {
		t.Fatalf("byok-a model=%q want model-a1", byokA.model())
	}
	if done1.Model != "model-a1" {
		t.Fatalf("done1.model=%q", done1.Model)
	}
	if done1.Host == "" || provider.IsTempAIHost(done1.Host) {
		t.Fatalf("done1.host=%q unexpected", done1.Host)
	}
	// History has user + assistant.
	sess1 := svc.GetChatSession()
	if len(sess1.Messages) < 2 {
		t.Fatalf("want history after turn 1, got %d msgs", len(sess1.Messages))
	}
	msgCountAfter1 := len(sess1.Messages)

	// Mid-session switch to byok-b / model-b1 — no restart, history kept.
	act2, err := svc.SetActiveProvider(SetActiveProviderRequest{
		Profile: "byok-b",
		Model:   "model-b1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if act2.Profile != "byok-b" || act2.Model != "model-b1" {
		t.Fatalf("after switch: %+v", act2)
	}
	// History must not be wiped by the switch.
	if n := len(svc.GetChatSession().Messages); n != msgCountAfter1 {
		t.Fatalf("history wiped on switch: before=%d after=%d", msgCountAfter1, n)
	}

	var done2 ChatEvent
	err = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:  "hello B",
		NoTools: true,
	}, func(ev ChatEvent) {
		if ev.Type == ChatEventDone {
			done2 = ev
		}
	})
	if err != nil {
		t.Fatalf("chat turn 2: %v", err)
	}
	if byokB.hits.Load() == 0 {
		t.Fatal("expected byok-b mock to receive requests after switch")
	}
	if !strings.Contains(byokB.auth(), "byok-secret-B-key") {
		t.Fatalf("byok-b auth=%q", byokB.auth())
	}
	if byokB.model() != "model-b1" {
		t.Fatalf("byok-b model=%q", byokB.model())
	}
	if done2.Model != "model-b1" {
		t.Fatalf("done2.model=%q", done2.Model)
	}
	// History grew (turn 2 appended).
	if n := len(svc.GetChatSession().Messages); n <= msgCountAfter1 {
		t.Fatalf("history did not grow after turn 2: %d vs %d", n, msgCountAfter1)
	}

	// BYOK traffic must not have used temp-ai with BYOK keys.
	// (temp-ai mock may still be listed via GetSettings, but ChatStream for BYOK must not hit it)
	if tempHits.Load() != 0 {
		// GetActiveSession / GetSettings may call ListModels on tempai when building pickers;
		// only fail if BYOK secrets were involved (checked in handler above).
	}
}

// TestBYOKKeyNeverSentToTempAIHost verifies VAL-PROV-006 with a mock on the
// BYOK base URL and a temp-ai proxy fake that logs any Authorization header.
func TestBYOKKeyNeverSentToTempAIHost(t *testing.T) {
	const byokKey = "byok-secret-never-to-tempai-XYZ"
	byok := startMockOpenAI(t, "byok", []string{"local-model"}, "BYOK-OK")

	var tempAuths []string
	tempSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		tempAuths = append(tempAuths, auth)
		if strings.Contains(auth, byokKey) {
			t.Errorf("BYOK key sent to temp-ai host path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"error":"temp should not be used for BYOK chat"}`))
	}))
	defer tempSrv.Close()

	home := t.TempDir()
	cfg := config.FileConfig{
		Version:         1,
		DefaultProvider: "local-byok",
		Providers: []config.ProviderProfile{
			{
				ID:           "local-byok",
				Type:         "openai_compatible",
				Name:         "Local BYOK",
				BaseURL:      byok.base(),
				APIKey:       byokKey,
				DefaultModel: "local-model",
			},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	// Gateway "host" is our spy — MUST NOT receive BYOK key.
	t.Setenv(provider.EnvBaseURL, tempSrv.URL+"/v1")
	t.Setenv(provider.EnvAPIKey, "gateway-only-key")
	t.Setenv(provider.EnvModel, "")
	t.Setenv(provider.EnvBYOKBaseURL, "")
	t.Setenv(provider.EnvBYOKAPIKey, "")

	wsRoot := filepath.Join(home, "ws")
	_ = os.MkdirAll(wsRoot, 0o755)
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SetActiveProvider(SetActiveProviderRequest{
		Profile: "local-byok",
		Model:   "local-model",
	}); err != nil {
		t.Fatal(err)
	}

	err = svc.ChatStream(context.Background(), ChatRequest{
		Prompt:  "ping",
		NoTools: true,
	}, func(ChatEvent) {})
	if err != nil {
		t.Fatalf("BYOK chat: %v", err)
	}
	if byok.hits.Load() == 0 {
		t.Fatal("BYOK mock received no traffic")
	}
	if !strings.Contains(byok.auth(), byokKey) {
		t.Fatalf("BYOK Authorization missing key on BYOK host: %q", byok.auth())
	}
	// Isolation: every temp-ai Authorization must not carry BYOK key
	// (handlers already fail-hard; double-check list).
	for _, a := range tempAuths {
		if strings.Contains(a, byokKey) {
			t.Fatalf("BYOK key present on temp-ai Authorization: %q", a)
		}
	}
}

// TestConfigPersistenceAndMode0600 verifies VAL-PROV-009: profiles persist
// across Service restart and config.yaml is mode 0600; no key in public views.
func TestConfigPersistenceAndMode0600(t *testing.T) {
	const secret = "sk-persist-test-key-value"
	home := t.TempDir()

	svc1, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc1.AddProvider(AddProviderRequest{
		ID:           "persist-byok",
		Name:         "Persist BYOK",
		Type:         "openai_compatible",
		BaseURL:      "https://example.invalid/v1",
		APIKey:       secret,
		DefaultModel: "m1",
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range view.Profiles {
		if p.ID == "persist-byok" {
			found = true
			raw, _ := json.Marshal(p)
			if strings.Contains(string(raw), secret) {
				t.Fatalf("key leaked in profile view: %s", raw)
			}
		}
	}
	if !found {
		t.Fatal("profile not in list after add")
	}

	cfgPath := filepath.Join(home, "config.yaml")
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o want 0600", info.Mode().Perm())
	}

	// Set as default + session active.
	if _, err := svc1.SetActiveProvider(SetActiveProviderRequest{
		Profile:    "persist-byok",
		Model:      "m1",
		SetDefault: true,
	}); err != nil {
		t.Fatal(err)
	}

	// "Restart": new Service on same home.
	svc2, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	view2 := svc2.GetSettings("")
	found = false
	for _, p := range view2.Profiles {
		if p.ID == "persist-byok" {
			found = true
			if !p.HasKey {
				t.Fatal("persisted profile missing key flag")
			}
			raw, _ := json.Marshal(p)
			if strings.Contains(string(raw), secret) {
				t.Fatalf("key leaked after restart: %s", raw)
			}
		}
	}
	if !found {
		t.Fatal("profile missing after app restart")
	}
	// default_provider persisted
	cfg, err := config.LoadFromDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProvider != "persist-byok" {
		t.Fatalf("default_provider=%q", cfg.DefaultProvider)
	}
	// mode still 0600 after SetDefault write
	info2, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if info2.Mode().Perm() != 0o600 {
		t.Fatalf("mode after default set=%o", info2.Mode().Perm())
	}
}

// TestHTTPActiveSessionRoutes covers GET/POST /api/session/active.
func TestHTTPActiveSessionRoutes(t *testing.T) {
	home := t.TempDir()
	// Minimal BYOK so switch succeeds without live network.
	byok := startMockOpenAI(t, "http-byok", []string{"m"}, "ok")
	_ = config.SaveToDir(home, config.FileConfig{
		Version: 1,
		Providers: []config.ProviderProfile{
			{
				ID:      "http-byok",
				Type:    "openai_compatible",
				Name:    "HTTP BYOK",
				BaseURL: byok.base(),
				APIKey:  "k",
			},
		},
	})
	t.Setenv(provider.EnvAPIKey, "temp-k")
	t.Setenv(provider.EnvBaseURL, "https://ai.temp.web.id/v1")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(svc.Handler())
	defer ts.Close()

	// GET active
	res, err := http.Get(ts.URL + "/api/session/active")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("GET status=%d", res.StatusCode)
	}
	var got ActiveSessionView
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Profile == "" {
		t.Fatal("empty profile on GET")
	}

	// POST switch
	body := strings.NewReader(`{"profile":"http-byok","model":"m"}`)
	res2, err := http.Post(ts.URL+"/api/session/active", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		t.Fatalf("POST status=%d", res2.StatusCode)
	}
	var got2 ActiveSessionView
	if err := json.NewDecoder(res2.Body).Decode(&got2); err != nil {
		t.Fatal(err)
	}
	if got2.Profile != "http-byok" {
		t.Fatalf("profile=%q", got2.Profile)
	}
	if got2.Model != "m" {
		t.Fatalf("model=%q", got2.Model)
	}
	// Session state sticky
	act := svc.GetActiveSession()
	if act.Profile != "http-byok" || act.Model != "m" {
		t.Fatalf("sticky session=%+v", act)
	}
}

// Selecting the gateway by its legacy alias must persist and report the
// canonical registered id. Storing "tempai" verbatim left Settings with a
// selected_profile that matched no profile row, and a default_provider that
// Bootstrap could not select on the next launch.
func TestGatewaySelectionPersistsCanonicalProfileID(t *testing.T) {
	home := t.TempDir()
	t.Setenv(provider.EnvAPIKey, "temp-k")
	t.Setenv(provider.EnvBaseURL, "https://ai.temp.web.id/v1")
	t.Setenv(provider.EnvModel, "gw-model")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	view, err := svc.SetActiveProvider(SetActiveProviderRequest{
		Profile:    provider.ProfileTempAI,
		SetDefault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Profile != provider.ProfileInferenesia {
		t.Fatalf("active profile = %q, want canonical %q", view.Profile, provider.ProfileInferenesia)
	}

	// The reported selection must name a row the panel can actually highlight.
	settings := svc.GetSettings(provider.ProfileTempAI)
	if settings.SelectedProfile != provider.ProfileInferenesia {
		t.Fatalf("selected_profile = %q, want %q", settings.SelectedProfile, provider.ProfileInferenesia)
	}
	found := false
	for _, p := range settings.Profiles {
		if p.ID == settings.SelectedProfile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("selected_profile %q matches no profile row: %+v", settings.SelectedProfile, settings.Profiles)
	}

	// Persisted default must survive a restart as the same effective route.
	cfg, err := config.LoadFromDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProvider != provider.ProfileInferenesia {
		t.Fatalf("default_provider = %q, want %q", cfg.DefaultProvider, provider.ProfileInferenesia)
	}
	restarted, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if got := restarted.GetSettings("").DefaultProfile; got != provider.ProfileInferenesia {
		t.Fatalf("default profile after restart = %q, want %q", got, provider.ProfileInferenesia)
	}
}
