package core

import (
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// ActiveSessionView is the mid-session provider/model state exposed to the desktop
// chat header picker (VAL-PROV-005, VAL-PROV-011). No secrets.
type ActiveSessionView struct {
	// Profile is the active provider profile id for the next chat turn.
	Profile string `json:"profile"`
	// Model is the preferred model id for the next turn (may be empty = auto).
	Model string `json:"model,omitempty"`
	// Host is the request host for the active profile (scheme://host). Never keys.
	Host string `json:"host,omitempty"`
	// ProfileName is a human label for the UI picker.
	ProfileName string `json:"profile_name,omitempty"`
	// IsGateway marks the temp-ai gateway profile.
	IsGateway bool `json:"is_gateway,omitempty"`
	// Profiles is a compact list for the mid-session picker.
	Profiles []ProfileView `json:"profiles,omitempty"`
	// Models is the live discovery list for the active profile (optional).
	Models []ModelView `json:"models,omitempty"`
	// ModelsError is set when discovery fails (no key leak).
	ModelsError string `json:"models_error,omitempty"`
}

// SetActiveProviderRequest switches the active profile/model mid-session
// without restarting the app or clearing chat history (VAL-PROV-005).
type SetActiveProviderRequest struct {
	// Profile is the provider profile id (tempai, byok, …). Required.
	Profile string `json:"profile"`
	// Model is optional preferred model id. Empty keeps current or profile default.
	Model string `json:"model,omitempty"`
	// SetDefault also writes default_provider (+ default_model when set) to config.
	SetDefault bool `json:"set_default,omitempty"`
}

// GetActiveSession returns the mid-session active provider/model (VAL-PROV-005).
// When nothing is selected yet, seeds from chat.Profile/chat.Model or config default.
func (s *Service) GetActiveSession() ActiveSessionView {
	s.mu.Lock()
	home := s.configHome
	profile := strings.TrimSpace(s.chat.Profile)
	model := strings.TrimSpace(s.chat.Model)
	s.mu.Unlock()

	file, _ := config.LoadFromDir(home)
	router := provider.Bootstrap(provider.BootstrapOptions{File: file})

	// Resolve legacy aliases to their registered id so the picker highlights a
	// real profile row instead of falling through to the "no match" branch.
	profile = router.CanonicalID(profile)
	// Prefer session model; fall back to profile default.
	view := ActiveSessionView{
		Profile:   profile,
		Model:     model,
		IsGateway: provider.IsGatewayProfile(profile),
	}

	// Compact public profile list (reuse settings sanitization logic).
	ids := router.IDs()
	sortProfileIDs(ids)
	for _, id := range ids {
		pv := ProfileView{
			ID:        id,
			Name:      id,
			Type:      "openai_compatible",
			IsDefault: id == router.DefaultID(),
			IsGateway: provider.IsGatewayProfile(id),
		}
		var baseURL string
		if pc, err := router.GetProfile(id); err == nil {
			if n := pc.Name(); n != "" && n != id {
				pv.Name = n
			}
			pv.BaseHost = pc.RequestHost()
			pv.DefaultModel = pc.DefaultModel()
			pv.CurrentModel = pc.DefaultModel()
			pv.HasKey = pc.APIKeyConfigured()
			baseURL = pc.BaseURL()
			pv.Type = provider.AdapterType(pc)
		}
		for _, p := range file.Providers {
			if p.ID != id {
				continue
			}
			if strings.TrimSpace(p.Name) != "" {
				pv.Name = p.Name
			}
			if strings.TrimSpace(p.Type) != "" {
				pv.Type = p.NormalizedType()
			}
			if strings.TrimSpace(p.DefaultModel) != "" {
				if pv.DefaultModel == "" {
					pv.DefaultModel = p.DefaultModel
				}
				if pv.CurrentModel == "" {
					pv.CurrentModel = p.DefaultModel
				}
			}
			if strings.TrimSpace(p.BaseURL) != "" {
				baseURL = p.BaseURL
			}
		}
		if provider.IsGatewayProfile(id) {
			if pv.Name == id || pv.Name == "" || pv.Name == "temp-ai" {
				pv.Name = provider.DefaultProfileName
			}
			if baseURL == "" {
				baseURL = provider.DefaultTempAIBaseURL
			}
		}
		if id == provider.ProfileAnthropic || pv.Type == provider.TypeAnthropic {
			if pv.Name == id || pv.Name == "" {
				pv.Name = "Anthropic"
			}
			if baseURL == "" {
				baseURL = provider.DefaultAnthropicBaseURL
			}
			pv.Type = provider.TypeAnthropic
		}
		if baseURL != "" {
			pv.BaseURLMasked = config.MaskBaseURL(baseURL)
			if pv.BaseHost == "" {
				pv.BaseHost = provider.RequestHost(baseURL)
			}
		}
		view.Profiles = append(view.Profiles, pv)
		if id == profile {
			view.ProfileName = pv.Name
			view.Host = pv.BaseHost
			if view.Model == "" {
				view.Model = firstNonEmpty(pv.CurrentModel, pv.DefaultModel)
			}
		}
	}

	if pc, err := router.GetProfile(profile); err == nil {
		if view.Host == "" {
			view.Host = pc.RequestHost()
		}
		if view.ProfileName == "" {
			view.ProfileName = pc.Name()
			if view.ProfileName == "" {
				view.ProfileName = profile
			}
		}
		if view.Model == "" {
			view.Model = pc.DefaultModel()
		}
	}

	return view
}

// SetActiveProvider switches the session's active profile/model mid-chat
// without restarting the app and without clearing history (VAL-PROV-005/011).
// Optional SetDefault persists default_provider (+ default_model) to config.yaml (0600).
func (s *Service) SetActiveProvider(req SetActiveProviderRequest) (ActiveSessionView, error) {
	profile := strings.TrimSpace(req.Profile)
	if profile == "" {
		return ActiveSessionView{}, fmt.Errorf("profile is required")
	}
	model := strings.TrimSpace(req.Model)

	home := s.ConfigHome()
	file, err := config.LoadFromDir(home)
	if err != nil {
		file = config.FileConfig{}
	}
	router := provider.Bootstrap(provider.BootstrapOptions{File: file})
	if _, err := router.Get(profile); err != nil {
		return ActiveSessionView{}, fmt.Errorf("unknown provider profile %q", profile)
	}
	// Persist and report the id the router actually registered. A legacy alias
	// ("tempai") resolves to the gateway client but is not a key in the
	// profile map, so storing it verbatim makes Settings re-open on a profile
	// row that never matches and default_provider point at an id Bootstrap
	// cannot select.
	profile = router.CanonicalID(profile)

	// Accept openai_compatible + anthropic (GetProfile). Missing key is not
	// a hard block here — it surfaces as blocked/key-required on chat send
	// (VAL-PROV-008) so the gateway remains independently usable.
	pc, err := router.GetProfile(profile)
	if err != nil {
		return ActiveSessionView{}, fmt.Errorf("profile %q is not available for chat yet: %w", profile, err)
	}
	if model == "" {
		model = pc.DefaultModel()
	}

	// Persist as default when requested (settings "use for all sessions").
	if req.SetDefault {
		cfg, loadErr := config.LoadFromDir(home)
		if loadErr != nil {
			cfg = config.FileConfig{Version: 1}
		}
		if cfg.Version == 0 {
			cfg.Version = 1
		}
		cfg.DefaultProvider = profile
		if model != "" {
			cfg.DefaultModel = model
		}
		// Also write default_model onto the YAML profile entry when present.
		for i, p := range cfg.Providers {
			if p.ID == profile && model != "" {
				cfg.Providers[i].DefaultModel = model
			}
		}
		if err := config.SaveToDir(home, cfg); err != nil {
			return ActiveSessionView{}, err
		}
	}

	// Update session-only active profile/model (does not wipe messages).
	s.mu.Lock()
	s.chat.Profile = profile
	if model != "" {
		s.chat.Model = model
	}
	// Persist model/profile on durable thread when workspace is bound.
	wid := s.chatWorkspaceID
	if wid == "" && s.registry != nil {
		wid = s.registry.ActiveID()
		s.chatWorkspaceID = wid
	}
	snap := ChatSession{
		Messages: append([]provider.Message(nil), s.chat.Messages...),
		Model:    s.chat.Model,
		Profile:  s.chat.Profile,
	}
	s.statusMsg = fmt.Sprintf("Active: %s / %s", profile, firstNonEmpty(model, "(auto)"))
	s.mu.Unlock()
	if wid != "" {
		_ = s.persistChat(wid, snap)
	}

	return s.GetActiveSession(), nil
}

// resolveChatProfileModel fills empty ChatRequest profile/model from the
// session active selection (mid-session switch without restart).
func (s *Service) resolveChatProfileModel(req *ChatRequest) {
	if req == nil {
		return
	}
	s.mu.Lock()
	sessProfile := strings.TrimSpace(s.chat.Profile)
	sessModel := strings.TrimSpace(s.chat.Model)
	s.mu.Unlock()
	if strings.TrimSpace(req.Profile) == "" && sessProfile != "" {
		req.Profile = sessProfile
	}
	if strings.TrimSpace(req.Model) == "" && sessModel != "" {
		req.Model = sessModel
	}
}
