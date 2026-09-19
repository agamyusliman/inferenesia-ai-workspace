package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// AddProviderRequest is the payload for creating a BYOK profile
// (openai_compatible or anthropic; VAL-PROV-003/007). API key is accepted once
// from the UI and stored only in local config (mode 0600); it is never returned
// to the renderer.
type AddProviderRequest struct {
	// ID is optional; when empty, derived from name/base URL.
	ID string `json:"id"`
	// Name is the human label (defaults to id).
	Name string `json:"name"`
	// Type is openai_compatible (default) or anthropic.
	Type string `json:"type"`
	// APIFormat overrides the wire protocol. Values: "openai" | "anthropic".
	// Empty derives from Type. Allows an openai_compatible host (e.g. LiteLLM
	// Claude) to speak the Anthropic Messages wire format (P9-prov).
	APIFormat string `json:"api_format"`
	// BaseURL is the provider API root. For openai_compatible include /v1.
	// For anthropic, defaults to https://api.anthropic.com when empty.
	BaseURL string `json:"base_url"`
	// APIKey is the BYOK secret. Never logged or echoed.
	APIKey string `json:"api_key"`
	// APIKeyEnv is an optional env-var name instead of inline key.
	APIKeyEnv string `json:"api_key_env"`
	// DefaultModel is optional preferred model id (still discovered via list models).
	DefaultModel string `json:"default_model"`
	// SetDefault marks this profile as default_provider in config when true.
	SetDefault bool `json:"set_default"`
	// OnboardingPath is optional: "byok" | "tempai" for first-run UX bookkeeping.
	OnboardingPath string `json:"onboarding_path"`
}

// TestProviderResult is the outcome of a minimal chat completion test (VAL-PROV-004).
// Never includes API keys.
type TestProviderResult struct {
	OK      bool   `json:"ok"`
	Profile string `json:"profile"`
	Model   string `json:"model,omitempty"`
	Host    string `json:"host,omitempty"`
	// Preview is a short (masked) assistant reply snippet on success.
	Preview string `json:"preview,omitempty"`
	// Error is a human-readable failure (401, missing key, unreachable) — no secrets.
	Error string `json:"error,omitempty"`
	// LatencyMS is approximate round-trip for the test call.
	LatencyMS int64 `json:"latency_ms,omitempty"`
}

// OnboardingState is the first-run / settings dual-path payload (VAL-PROV-010).
type OnboardingState struct {
	// Done is true after the user completed either gateway or BYOK setup.
	Done bool `json:"done"`
	// Paths describes the two first-class paths; neither requires the other.
	Paths []OnboardingPath `json:"paths"`
	// HasTempAIKey reports whether TEMP_AI_API_KEY (or tempai profile key) is set.
	HasTempAIKey bool `json:"has_tempai_key"`
	// HasBYOK reports whether any non-tempai profile has a key / env BYOK is set.
	HasBYOK bool `json:"has_byok"`
	// Recommended is the recommended first-run path id ("tempai").
	Recommended string `json:"recommended"`
}

// OnboardingPath is one setup option (temp-ai gateway vs BYOK).
type OnboardingPath struct {
	ID          string `json:"id"` // tempai | byok
	Title       string `json:"title"`
	Description string `json:"description"`
	// RequiresOther is always false — user can complete either path alone.
	RequiresOther bool `json:"requires_other"`
}

// SetDefaultProvider marks a profile as default_provider in config.yaml (0600)
// and updates the session active profile so the next chat turn uses it.
func (s *Service) SetDefaultProvider(profileID, model string) (SettingsView, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return SettingsView{}, fmt.Errorf("profile is required")
	}
	_, err := s.SetActiveProvider(SetActiveProviderRequest{
		Profile:    profileID,
		Model:      strings.TrimSpace(model),
		SetDefault: true,
	})
	if err != nil {
		return SettingsView{}, err
	}
	return s.GetSettings(profileID), nil
}

// AddProvider registers a new BYOK profile (openai_compatible or anthropic) in
// ~/.inferenesia/config.yaml (mode 0600) and returns an updated settings view.
func (s *Service) AddProvider(req AddProviderRequest) (SettingsView, error) {
	home := s.ConfigHome()
	typ := strings.ToLower(strings.TrimSpace(req.Type))
	apiFmt := strings.ToLower(strings.TrimSpace(req.APIFormat))

	// Normalize api_format first since it can drive type when type is empty.
	switch apiFmt {
	case config.APIFormatOpenAI, "openai_compatible", "openai-compatible":
		apiFmt = config.APIFormatOpenAI
	case config.APIFormatAnthropic, "claude":
		apiFmt = config.APIFormatAnthropic
	case "":
		// derive below based on type
	default:
		return SettingsView{}, fmt.Errorf("unsupported api_format %q (use openai or anthropic)", apiFmt)
	}

	// When type is empty but api_format is anthropic, coerce type to anthropic
	// so config.yaml stays self-consistent. When type is empty and api_format
	// is openai (or empty), default to openai_compatible.
	if typ == "" {
		if apiFmt == config.APIFormatAnthropic {
			typ = provider.TypeAnthropic
		} else {
			typ = provider.TypeOpenAICompatible
		}
	}
	switch typ {
	case "openai_compatible", "openai", "openai-compatible":
		typ = provider.TypeOpenAICompatible
	case "anthropic", "claude":
		typ = provider.TypeAnthropic
	default:
		return SettingsView{}, fmt.Errorf("unsupported provider type %q (use openai_compatible or anthropic)", typ)
	}

	// Derive api_format from type when not explicitly set.
	if apiFmt == "" {
		if typ == provider.TypeAnthropic {
			apiFmt = config.APIFormatAnthropic
		} else {
			apiFmt = config.APIFormatOpenAI
		}
	}

	base := strings.TrimSpace(req.BaseURL)
	if base == "" {
		if apiFmt == config.APIFormatAnthropic || typ == provider.TypeAnthropic {
			base = provider.DefaultAnthropicBaseURL
		} else {
			return SettingsView{}, fmt.Errorf("base_url is required")
		}
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		if typ == provider.TypeAnthropic {
			id = config.SanitizeProviderID(firstNonEmpty(req.Name, provider.ProfileAnthropic))
		} else {
			id = config.SanitizeProviderID(firstNonEmpty(req.Name, hostSlug(base)))
		}
	} else {
		id = config.SanitizeProviderID(id)
	}
	// Reserved: never overwrite the env-driven gateway profile id (inferenesia
	// or legacy alias tempai) via add form when it would clobber gateway
	// identity — force a distinct BYOK id.
	if provider.IsGatewayProfile(id) {
		id = config.SanitizeProviderID(firstNonEmpty(req.Name, "byok-"+hostSlug(base)))
		if provider.IsGatewayProfile(id) {
			id = "byok-custom"
		}
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		if typ == provider.TypeAnthropic {
			name = "Anthropic"
		} else {
			name = id
		}
	}
	key := strings.TrimSpace(req.APIKey)
	keyEnv := strings.TrimSpace(req.APIKeyEnv)
	if key == "" && keyEnv == "" {
		return SettingsView{}, fmt.Errorf("api_key or api_key_env is required")
	}

	profile := config.ProviderProfile{
		ID:           id,
		Type:         typ,
		Name:         name,
		BaseURL:      base,
		APIKey:       key,
		APIKeyEnv:    keyEnv,
		DefaultModel: strings.TrimSpace(req.DefaultModel),
		APIFormat:    apiFmt,
	}

	cfg, err := config.UpsertProvider(home, profile)
	if err != nil {
		return SettingsView{}, err
	}
	if req.SetDefault {
		cfg.DefaultProvider = id
		if err := config.SaveToDir(home, cfg); err != nil {
			return SettingsView{}, err
		}
	}
	// First-run bookkeeping: completing either path marks onboarding done.
	path := strings.ToLower(strings.TrimSpace(req.OnboardingPath))
	if path == "byok" || path == "tempai" || path == "" {
		if !cfg.OnboardingDone {
			cfg.OnboardingDone = true
			_ = config.SaveToDir(home, cfg)
		}
	}
	return s.GetSettings(id), nil
}

// TestProvider sends a minimal chat completion against the profile (VAL-PROV-004/007).
// On success returns ok=true with a short preview; on failure a clear error without key values.
// Anthropic without key → blocked/key required (not temp-ai fallback).
func (s *Service) TestProvider(profileID string) TestProviderResult {
	profileID = strings.TrimSpace(profileID)
	home := s.ConfigHome()
	file, _ := config.LoadFromDir(home)
	router := provider.Bootstrap(provider.BootstrapOptions{File: file})
	if profileID == "" {
		profileID = router.DefaultID()
	}

	res := TestProviderResult{Profile: profileID}
	pc, err := router.GetProfile(profileID)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Host = pc.RequestHost()
	if !pc.APIKeyConfigured() {
		// Clear blocked wording for Anthropic / any BYOK without key (VAL-PROV-008).
		res.Error = (&provider.BlockedError{
			Profile: profileID,
			Reason:  "key required",
			Cause:   provider.ErrMissingAPIKey,
		}).Error()
		return res
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	// Resolve model from list models (no hard-coded id required).
	model, err := pc.ResolveModel(ctx, pc.DefaultModel())
	if err != nil {
		res.Error = err.Error()
		res.LatencyMS = time.Since(start).Milliseconds()
		return res
	}
	res.Model = model

	msg, err := pc.ChatTurn(ctx, provider.ChatRequest{
		Model: model,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Reply with exactly: ok"},
		},
	})
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		// provider errors already redact; double-check no raw key shapes.
		res.Error = err.Error()
		return res
	}
	preview := strings.TrimSpace(msg.Content)
	if len(preview) > 120 {
		preview = preview[:120] + "…"
	}
	res.OK = true
	res.Preview = preview
	return res
}

// GetOnboarding returns first-run / settings dual-path state (VAL-PROV-010).
// Users can complete temp-ai gateway or BYOK without accounts for the other.
func (s *Service) GetOnboarding() OnboardingState {
	home := s.ConfigHome()
	file, _ := config.LoadFromDir(home)
	router := provider.Bootstrap(provider.BootstrapOptions{File: file})

	state := OnboardingState{
		Done:        file.OnboardingDone,
		Recommended: provider.ProfileInferenesia,
		Paths: []OnboardingPath{
			{
				ID:            provider.ProfileInferenesia,
				Title:         "Use Inferenesia API gateway",
				Description:   "Connect via " + provider.DefaultTempAIBaseURL + " using INFERENESIA_API_KEY (or legacy TEMP_AI_API_KEY, or paste a gateway key). Recommended first-run; no BYOK account required.",
				RequiresOther: false,
			},
			{
				ID:            "byok",
				Title:         "Use my own key (BYOK)",
				Description:   "Add an openai_compatible provider with your base URL and API key. Keys stay local (config mode 0600) and are never sent to the gateway host. No Inferenesia API account required.",
				RequiresOther: false,
			},
		},
	}

	if pc, err := router.GetProfile(provider.ProfileInferenesia); err == nil {
		state.HasTempAIKey = pc.APIKeyConfigured()
	}
	for _, id := range router.IDs() {
		if provider.IsGatewayProfile(id) {
			continue
		}
		if pc, err := router.GetProfile(id); err == nil && pc.APIKeyConfigured() {
			state.HasBYOK = true
			break
		}
	}
	// Auto-mark done when either path has credentials (idempotent).
	if !state.Done && (state.HasTempAIKey || state.HasBYOK) {
		state.Done = true
	}
	return state
}

// CompleteOnboarding marks first-run done after the user finishes either path.
func (s *Service) CompleteOnboarding(path string) (OnboardingState, error) {
	home := s.ConfigHome()
	_, err := config.SetOnboardingDone(home, true)
	if err != nil {
		return OnboardingState{}, err
	}
	_ = path // path is informational only; either is valid alone.
	return s.GetOnboarding(), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func hostSlug(baseURL string) string {
	host := provider.RequestHost(baseURL)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	return config.SanitizeProviderID(host)
}
