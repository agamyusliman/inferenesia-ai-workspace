package core

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// TokenSaverStatus is one entry in Settings → Token Saver (VAL-SAVER-001/002/007).
// Status is "ready" or "not_installed" (display: "Not installed"). Never crashes when missing.
type TokenSaverStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	// Installed is true when the binary/endpoint is present and usable for probing.
	Installed bool `json:"installed"`
	// Status is "ready" or "not_installed" for UI badges.
	Status string `json:"status"`
	// Detail is a short install hint (no secrets).
	Detail string `json:"detail,omitempty"`
	// Command and BaseURL are safe editable settings for RTK/Headroom.
	Command string `json:"command,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// TokenSaversView is the Settings Token Saver section payload.
type TokenSaversView struct {
	// Product brand for display.
	Product string `json:"product"`
	// ConfigHome is the resolved config dir (path only).
	ConfigHome string `json:"config_home"`
	// Savers lists RTK, Headroom, Caveman, Ponytail (independent toggles).
	Savers []TokenSaverStatus `json:"savers"`
	// Note documents default-off + optional nature.
	Note string `json:"note"`
}

// SetTokenSaverRequest toggles one saver on/off and/or updates its setup.
// Every mutable field is a pointer so an omitted field means "leave as is":
// saving an RTK command must not silently flip the toggle off just because the
// caller did not restate it.
type SetTokenSaverRequest struct {
	// ID is rtk | headroom | caveman | ponytail.
	ID string `json:"id"`
	// Enabled is the desired toggle state. Omitted keeps the persisted state.
	Enabled *bool `json:"enabled,omitempty"`
	// Command is the RTK binary override; empty string clears back to default.
	Command *string `json:"command,omitempty"`
	// BaseURL is the Headroom compress endpoint; empty string clears it.
	BaseURL *string `json:"base_url,omitempty"`
}

// tokenSaverMeta is static UI copy for the four optional savers.
var tokenSaverMeta = []struct {
	ID          string
	Name        string
	Description string
}{
	{
		ID:          config.TokenSaverRTK,
		Name:        "RTK",
		Description: "Compress tool outputs (git, grep, ls, tree, logs) before they enter model context (~60–90% fewer tokens when available).",
	},
	{
		ID:          config.TokenSaverHeadroom,
		Name:        "Headroom",
		Description: "Compress prompts via a configured /v1/compress-style endpoint before routing to the model.",
	},
	{
		ID:          config.TokenSaverCaveman,
		Name:        "Caveman",
		Description: "Terse system-prompt bias aimed at shorter model completions (fewer output tokens).",
	},
	{
		ID:          config.TokenSaverPonytail,
		Name:        "Ponytail",
		Description: "Minimal-code bias: YAGNI, reuse stdlib, prefer deletion over addition when editing.",
	},
}

// GetTokenSavers returns the Settings Token Saver section with live install status.
// Missing binaries/endpoints show status "not_installed" (UI: Not installed). Defaults off.
func (s *Service) GetTokenSavers() TokenSaversView {
	home := s.ConfigHome()
	savers, err := config.LoadTokenSavers(home)
	if err != nil {
		// Never crash UI: surface empty defaults.
		savers = config.DefaultTokenSavers()
	}
	return buildTokenSaversView(s.BrandName(), home, savers)
}

// SetTokenSaver updates one saver's toggle and/or setup and persists under
// config home config.yaml (mode 0600). Enabling a missing saver is allowed and
// does not error; status remains not_installed. Chat remains safe when the
// saver is unavailable (pipeline pass-through in m5c-token-saver-pipeline).
//
// Nothing is written unless the whole request validates, so a rejected setup
// value cannot leave the saver half-updated.
func (s *Service) SetTokenSaver(req SetTokenSaverRequest) (TokenSaversView, error) {
	id := strings.ToLower(strings.TrimSpace(req.ID))
	if id == "" {
		return TokenSaversView{}, fmt.Errorf("token saver id is required (rtk, headroom, caveman, ponytail)")
	}
	home := s.ConfigHome()
	current, err := config.LoadTokenSavers(home)
	if err != nil {
		current = config.DefaultTokenSavers()
	}

	// Reject an unknown id before touching any field so a typo cannot rewrite
	// another saver's setup. SetEnabled is the single source of valid ids.
	updated, err := current.SetEnabled(id, current.IsEnabled(id))
	if err != nil {
		return TokenSaversView{}, err
	}

	if req.Command != nil {
		if id != config.TokenSaverRTK {
			return TokenSaversView{}, fmt.Errorf("token saver %q has no command setting", id)
		}
		updated.RTK.Command = strings.TrimSpace(*req.Command)
	}
	if req.BaseURL != nil {
		if id != config.TokenSaverHeadroom {
			return TokenSaversView{}, fmt.Errorf("token saver %q has no base_url setting", id)
		}
		value := strings.TrimSpace(*req.BaseURL)
		if value != "" {
			u, perr := url.Parse(value)
			if perr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return TokenSaversView{}, fmt.Errorf("headroom base_url must be an http or https URL")
			}
		}
		updated.Headroom.BaseURL = value
	}
	// An omitted enabled field keeps the persisted toggle: saving a setup value
	// must never flip the saver off as a side effect.
	if req.Enabled != nil {
		updated, err = updated.SetEnabled(id, *req.Enabled)
		if err != nil {
			return TokenSaversView{}, err
		}
	}

	saved, err := config.SaveTokenSavers(home, updated)
	if err != nil {
		return TokenSaversView{}, fmt.Errorf("save token savers: %w", err)
	}
	// Report what landed on disk, not what was requested.
	return buildTokenSaversView(s.BrandName(), home, saved.TokenSavers), nil
}

func buildTokenSaversView(product, home string, savers config.TokenSaversConfig) TokenSaversView {
	savers.Normalize()
	view := TokenSaversView{
		Product:    product,
		ConfigHome: home,
		Note:       "Optional token savers. Defaults off. Enabling a missing tool shows Not installed and never crashes chat.",
	}
	for _, m := range tokenSaverMeta {
		st := TokenSaverStatus{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			Enabled:     savers.IsEnabled(m.ID),
		}
		switch m.ID {
		case config.TokenSaverRTK:
			st.Command = savers.RTKCommand()
			ok, detail := config.ProbeRTKAvailable(savers.RTKCommand())
			st.Installed = ok
			st.Detail = detail
		case config.TokenSaverHeadroom:
			// The user typed this URL in Settings; masking it here would make
			// the field round-trip lossy (query/userinfo silently dropped on
			// the next save). Provider base URLs are masked, saver setup is not.
			st.BaseURL = savers.Headroom.BaseURL
			ok, detail := config.ProbeHeadroomAvailable(savers.Headroom.BaseURL)
			st.Installed = ok
			st.Detail = detail
		case config.TokenSaverCaveman, config.TokenSaverPonytail:
			// Prompt-bias savers need no external binary — always "ready".
			st.Installed = true
			st.Detail = "system prompt fragment"
		}
		if st.Installed {
			st.Status = "ready"
		} else {
			st.Status = "not_installed"
			if st.Detail == "" {
				st.Detail = "Not installed"
			}
		}
		view.Savers = append(view.Savers, st)
	}
	return view
}

// TokenSaversConfig returns the raw toggles for pipeline hooks (m5c-token-saver-pipeline).
// Failures return defaults (all off) so chat never panics.
func (s *Service) TokenSaversConfig() config.TokenSaversConfig {
	home := s.ConfigHome()
	c, err := config.LoadTokenSavers(home)
	if err != nil {
		return config.DefaultTokenSavers()
	}
	return c
}
