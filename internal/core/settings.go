package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/workspace"
)

// SettingsView is the settings panel payload (profiles, models, workspace prefs).
// No secret values are included (API keys never returned to the renderer).
type SettingsView struct {
	// Product is the brand string (Inferenesia).
	Product string `json:"product"`
	// ConfigHome is the resolved config home (no secrets).
	ConfigHome string `json:"config_home"`
	// Sections lists visible panel sections for the UI.
	Sections []SettingsSection `json:"sections"`
	// Profiles lists registered provider profiles without keys.
	Profiles []ProfileView `json:"profiles"`
	// DefaultProfile is the router default id.
	DefaultProfile string `json:"default_profile"`
	// SelectedProfile is the profile used for model discovery (query/selector).
	SelectedProfile string `json:"selected_profile,omitempty"`
	// Models is the live list from the selected profile GET /v1/models (not hard-coded).
	Models []ModelView `json:"models,omitempty"`
	// ModelsError is set when model discovery failed (clear error, no key leak).
	ModelsError string `json:"models_error,omitempty"`
	// Workspace holds workspace prefs for the active project.
	Workspace WorkspacePrefs `json:"workspace"`
	// Labels carries Revert agent vs Restore to HEAD for settings copy (VAL-DESK-010).
	Labels SettingsLabels `json:"labels"`
	// Onboarding is the dual-path first-run state (temp-ai vs BYOK; VAL-PROV-010).
	Onboarding OnboardingState `json:"onboarding"`
	// TokenSavers is the Settings → Token Saver section (VAL-SAVER-001/002/007).
	// Pointer so older clients ignoring the field stay fine; always set on GetSettings.
	TokenSavers *TokenSaversView `json:"token_savers,omitempty"`
	// LiveBlocks is the Settings → Live Blocks opt-in toggle (P9-live).
	// Pointer so older clients ignoring the field stay fine; always set on GetSettings.
	LiveBlocks *LiveBlocksView `json:"live_blocks,omitempty"`
	// MCP is the Settings → MCP servers section (status, toggle, tools).
	MCP      *MCPStatusView `json:"mcp,omitempty"`
	Autonomy *AutonomyView  `json:"autonomy,omitempty"`
}

// SettingsSection describes one config section in the panel.
type SettingsSection struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ProfileView is a public provider profile summary (no API key).
type ProfileView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// APIFormat is the effective wire format ("openai" | "anthropic"). Derived
	// from the profile's api_format or type (P9-prov).
	APIFormat string `json:"api_format,omitempty"`
	// BaseHost is scheme://host (no credentials) for compact display.
	BaseHost string `json:"base_host,omitempty"`
	// BaseURLMasked is the full base path with userinfo/query stripped (VAL-PROV-001).
	BaseURLMasked string `json:"base_url_masked,omitempty"`
	// CurrentModel is the preferred/default model for this profile (picker seed).
	CurrentModel string `json:"current_model,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	HasKey       bool   `json:"has_key"`
	IsDefault    bool   `json:"is_default"`
	// IsGateway marks the built-in temp-ai gateway profile.
	IsGateway bool `json:"is_gateway,omitempty"`
}

// ModelView is one discoverable model id.
type ModelView struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// WorkspacePrefs holds workspace-scoped preferences shown in settings.
type WorkspacePrefs struct {
	ActiveID   string `json:"active_id,omitempty"`
	ActiveName string `json:"active_name,omitempty"`
	RootPath   string `json:"root_path,omitempty"`
	// UndoIsPreAgentDirty documents the product rule for the prefs panel.
	UndoIsPreAgentDirty bool   `json:"undo_is_pre_agent_dirty"`
	UndoNote            string `json:"undo_note"`
}

// SettingsLabels distinguishes agent undo from git restore (VAL-DESK-010).
type SettingsLabels struct {
	RevertAgent   string `json:"revert_agent"`
	RestoreToHead string `json:"restore_to_head"`
	Undo          string `json:"undo"`
	Redo          string `json:"redo"`
}

// GetSettings returns the settings panel view. Models are discovered live when
// credentials are available; failures populate ModelsError without crashing.
func (s *Service) GetSettings(profile string) SettingsView {
	s.mu.Lock()
	home := s.configHome
	s.mu.Unlock()

	view := SettingsView{
		Product:    s.BrandName(),
		ConfigHome: home,
		Sections: []SettingsSection{
			{ID: "onboarding", Title: "Connect a model", Description: "temp-ai gateway or BYOK — complete either path without accounts for the other."},
			{ID: "profiles", Title: "Providers", Description: "Provider profiles (temp-ai gateway and BYOK). Keys stay in env/config, never in the UI."},
			{ID: "models", Title: "Models", Description: "Models discovered from the selected profile GET /v1/models (not hard-coded)."},
			{ID: "token-savers", Title: "Token Saver", Description: "Optional RTK / Headroom / Caveman / Ponytail toggles (defaults off; Not installed when missing)."},
			{ID: "live-blocks", Title: "Live Blocks", Description: "Opt-in generative UI previews in chat (```live-block / yura-live / html:preview → sandboxed iframe). Default off."},
			{ID: "mcp", Title: "MCP Servers", Description: "Model Context Protocol servers — tools available to the agent. Toggle, add, or remove servers. Built-in CocoIndex auto-detected when installed."},
			{ID: "autonomy", Title: "Autonomy Level", Description: "Control how much the agent can do without approval — off (all gated), low (edits only), medium (reversible ops), high (full autonomy)."},
			{ID: "workspace", Title: "Workspace prefs", Description: "Active workspace and undo policy (pre-agent dirty, not HEAD by default)."},
		},
		Labels: SettingsLabels{
			RevertAgent:   LabelRevertAgent,
			RestoreToHead: LabelRestoreToHead,
			Undo:          LabelUndo,
			Redo:          LabelRedo,
		},
		Workspace: WorkspacePrefs{
			UndoIsPreAgentDirty: true,
			UndoNote:            fmt.Sprintf("%s restores pre-agent dirty content. %s uses git and is a separate action.", LabelRevertAgent, LabelRestoreToHead),
		},
	}

	activeWorkspaceRoot := ""
	if active := s.registry.Active(); active != nil {
		view.Workspace.ActiveID = active.ID
		view.Workspace.ActiveName = active.Name
		view.Workspace.RootPath = active.RootPath
		if !workspace.IsSession(*active) {
			activeWorkspaceRoot = active.RootPath
		}
	}

	file, _ := config.LoadFromDir(home)
	// Do not pass PreferredID here: selecting a profile in the UI is only for
	// model discovery / test target, not for changing the default profile.
	router := provider.Bootstrap(provider.BootstrapOptions{
		File: file,
	})
	view.DefaultProfile = router.DefaultID()
	view.Onboarding = s.GetOnboarding()

	// Public profile list: env-backed + YAML. Never include api_key values.
	// Sort tempai first for stable UI.
	ids := router.IDs()
	sortProfileIDs(ids)
	for _, id := range ids {
		pv := ProfileView{
			ID:        id,
			Name:      id,
			Type:      "openai_compatible",
			IsDefault: id == view.DefaultProfile,
			IsGateway: provider.IsGatewayProfile(id),
		}
		var baseURL string
		if pc, err := router.GetProfile(id); err == nil {
			// Prefer human Name() when it is not just the id; else keep id label.
			if n := pc.Name(); n != "" && n != id {
				pv.Name = n
			}
			pv.BaseHost = pc.RequestHost()
			pv.DefaultModel = pc.DefaultModel()
			pv.CurrentModel = pc.DefaultModel()
			pv.HasKey = pc.APIKeyConfigured()
			baseURL = pc.BaseURL()
			pv.Type = provider.AdapterType(pc)
			// Seed api_format from the concrete adapter so non-YAML profiles
			// (tempai, byok env, ninerouter) still expose a wire format.
			if pv.APIFormat == "" {
				switch pv.Type {
				case provider.TypeAnthropic:
					pv.APIFormat = config.APIFormatAnthropic
				default:
					pv.APIFormat = config.APIFormatOpenAI
				}
			}
		}
		// Enrich from YAML for display name + masked URL.
		for _, p := range file.Providers {
			if p.ID == id {
				if strings.TrimSpace(p.Name) != "" {
					pv.Name = p.Name
				}
				if strings.TrimSpace(p.Type) != "" {
					pv.Type = p.NormalizedType()
				}
				pv.APIFormat = p.NormalizedAPIFormat()
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
		}
		if provider.IsGatewayProfile(id) {
			if pv.Name == id || pv.Name == "" || pv.Name == "temp-ai" {
				pv.Name = provider.DefaultProfileName
			}
			if baseURL == "" {
				baseURL = provider.DefaultTempAIBaseURL
			}
			if pv.APIFormat == "" {
				pv.APIFormat = config.APIFormatOpenAI
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
			if pv.APIFormat == "" {
				pv.APIFormat = config.APIFormatAnthropic
			}
		}
		if baseURL != "" {
			pv.BaseURLMasked = config.MaskBaseURL(baseURL)
			if pv.BaseHost == "" {
				pv.BaseHost = provider.RequestHost(baseURL)
			}
		}
		view.Profiles = append(view.Profiles, pv)
	}

	// Live model discovery via Go core (not renderer) for the selected profile.
	// Canonicalize so a legacy alias ("tempai") reports the registered id the
	// profile rows actually carry — otherwise the panel highlights nothing and
	// the model picker re-seeds from the wrong row on every reload.
	want := router.CanonicalID(strings.TrimSpace(profile))
	view.SelectedProfile = want
	if pc, err := router.GetProfile(want); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		models, lerr := pc.ListModels(ctx)
		if lerr != nil {
			// Include blocked/key-required wording when Anthropic has no key.
			view.ModelsError = lerr.Error()
		} else {
			// If the YAML profile declares an explicit models[] list, treat it as
			// an allowlist and filter discovery output down to those ids
			// (":own" variants are dropped — user only picks the canonical id).
			var allow map[string]struct{}
			for _, p := range file.Providers {
				if p.ID == want && len(p.Models) > 0 {
					allow = make(map[string]struct{}, len(p.Models))
					for _, dm := range p.Models {
						id := strings.TrimSpace(dm.ID)
						if id == "" {
							continue
						}
						allow[id] = struct{}{}
					}
					break
				}
			}
			for _, m := range models {
				if allow != nil {
					if _, ok := allow[m.ID]; !ok {
						continue
					}
				}
				view.Models = append(view.Models, ModelView{ID: m.ID, OwnedBy: m.OwnedBy})
			}
		}
		// Seed current model on the selected profile row when discovery works.
		if cur := pc.DefaultModel(); cur == "" && len(view.Models) > 0 {
			for i := range view.Profiles {
				if view.Profiles[i].ID == want && view.Profiles[i].CurrentModel == "" {
					view.Profiles[i].CurrentModel = view.Models[0].ID
				}
			}
		}
	} else if err != nil {
		view.ModelsError = err.Error()
	}

	// Embed Token Saver section (independent toggles; never fails GetSettings).
	ts := s.GetTokenSavers()
	view.TokenSavers = &ts

	// Embed Live Blocks section (opt-in toggle; never fails GetSettings).
	lb := s.GetLiveBlocks()
	view.LiveBlocks = &lb

	// Embed MCP servers section (status + toggle; never fails GetSettings).
	// Settings can be opened before chat initializes the MCP host; load it for
	// the active workspace so every effective project server is represented.
	st := s.ensureMCPState()
	st.mu.RLock()
	loadedWorkspace := st.workspaceRoot
	loadedServers := len(st.servers) > 0
	st.mu.RUnlock()
	if loadedWorkspace != activeWorkspaceRoot || !loadedServers {
		_ = s.ReloadMCP(activeWorkspaceRoot)
	}
	mcpView := s.GetMCPStatus()
	view.MCP = &mcpView

	autonomyView := s.GetAutonomy()
	view.Autonomy = &autonomyView

	return view
}

// sortProfileIDs puts tempai first, then lexicographic — stable UI list.
func sortProfileIDs(ids []string) {
	// simple insertion: move tempai to front, sort rest
	var temp []string
	var rest []string
	for _, id := range ids {
		if provider.IsGatewayProfile(id) {
			temp = append(temp, id)
		} else {
			rest = append(rest, id)
		}
	}
	// lexicographic rest
	for i := 0; i < len(rest); i++ {
		for j := i + 1; j < len(rest); j++ {
			if rest[j] < rest[i] {
				rest[i], rest[j] = rest[j], rest[i]
			}
		}
	}
	copy(ids, append(temp, rest...))
}
