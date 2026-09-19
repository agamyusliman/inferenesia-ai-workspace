package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileConfig is the on-disk YAML shape under ~/.inferenesia/config.yaml.
// Secrets may be stored via api_key (prefer api_key_env). Mode must be 0600.
type FileConfig struct {
	Version         int               `yaml:"version"`
	DefaultProvider string            `yaml:"default_provider"`
	DefaultModel    string            `yaml:"default_model"`
	Providers       []ProviderProfile `yaml:"providers"`
	// OnboardingDone marks that the user completed first-run provider setup
	// (temp-ai gateway path and/or BYOK). Does not store secrets.
	OnboardingDone bool `yaml:"onboarding_done,omitempty"`
	// TokenSavers optional Settings toggles (RTK / Headroom / Caveman / Ponytail).
	// Defaults all off. See library/token-savers.md (VAL-SAVER).
	TokenSavers TokenSaversConfig `yaml:"token_savers,omitempty"`
	// LiveBlocks optional Settings toggle for generative UI previews (P9-live).
	// Default off. When on, ```live-block / ```yura-live / ```html:preview fences
	// render as sandboxed iframe previews in chat.
	LiveBlocks LiveBlocksConfig `yaml:"live_blocks,omitempty"`
	Autonomy string `yaml:"autonomy,omitempty"`
	// ModeModels maps chat modes and subagent roles to default profile/model.
	ModeModels ModeModelsConfig `yaml:"mode_models,omitempty"`
	// MCP is the optional MCP client host section (VAL-MCP-001/002).
	// Map keys are server names. Supports type stdio|local|http|remote only.
	// type:rag is rejected by ValidateMCPSection (VAL-MCP-004). Headers/auth
	// are stored locally; callers must never log header values.
	MCP map[string]MCPServerConfig `yaml:"mcp,omitempty"`
	// hasTokenSaversYAML is set during Load when the key was present (even if empty).
	// Unexported so it is not written back; used only to distinguish absent vs explicit.
	hasTokenSaversYAML bool `yaml:"-"`
}

// APIFormat wire-protocol constants for ProviderProfile.APIFormat.
// Empty means derive from Type at read time (NormalizedAPIFormat).
const (
	// APIFormatOpenAI is the OpenAI-compatible /v1/chat/completions wire format.
	APIFormatOpenAI = "openai"
	// APIFormatAnthropic is the Anthropic Messages API wire format.
	APIFormatAnthropic = "anthropic"
)

// ProviderProfile is one ModelRouter profile in config.yaml.
// type openai_compatible is the only type implemented at M1 (anthropic later).
type ProviderProfile struct {
	ID           string `yaml:"id"`
	Type         string `yaml:"type"` // openai_compatible | anthropic (later)
	Name         string `yaml:"name"`
	BaseURL      string `yaml:"base_url"`
	APIKey       string `yaml:"api_key"`     // optional inline key (prefer env)
	APIKeyEnv    string `yaml:"api_key_env"` // e.g. OPENAI_API_KEY
	DefaultModel string `yaml:"default_model"`
	// APIFormat overrides the wire format used by the adapter. Values:
	// "openai" | "anthropic". Empty derives from Type (P9-prov). Allows a
	// custom openai_compatible host (e.g. LiteLLM Claude) to speak the
	// Anthropic Messages wire format without changing type.
	APIFormat string `yaml:"api_format,omitempty"`
	// Models is an optional declared list (discovery still preferred at runtime).
	Models []struct {
		ID string `yaml:"id"`
	} `yaml:"models"`
}

// LoadFile reads and parses a config.yaml path. Missing file returns empty FileConfig, nil error.
func LoadFile(path string) (FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileConfig{}, nil
		}
		return FileConfig{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	// Detect presence of token_savers key so absent ≠ all-false explicit block.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["token_savers"]; ok {
			cfg.hasTokenSaversYAML = true
		}
	}
	cfg.TokenSavers.Normalize()
	cfg.ModeModels.Normalize()
	cfg.LiveBlocks.Normalize()
	return cfg, nil
}

// LoadFromHome loads config.yaml under the resolved config home (YURA_AI_HOME or ~/.yura-ai).
// Missing home/config returns empty FileConfig.
func LoadFromHome() (FileConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return FileConfig{}, err
	}
	return LoadFile(path)
}

// LoadFromDir loads config.yaml from an explicit home directory.
func LoadFromDir(home string) (FileConfig, error) {
	if strings.TrimSpace(home) == "" {
		return FileConfig{}, fmt.Errorf("empty config home")
	}
	return LoadFile(filepath.Join(home, ConfigFileName))
}

// SaveFile marshals cfg to path with mode 0600 (VAL-PROV-003 secret hygiene).
// Parent directories are created as needed (0700 for config home convention).
func SaveFile(path string, cfg FileConfig) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty config path")
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	// Header comment for humans; never embed secrets beyond what the YAML already holds.
	header := []byte("# Inferenesia configuration (mode 0600)\n# Hierarchy: env > this file > defaults\n# Never commit API keys. BYOK keys stay local and are not sent to the gateway host.\n\n")
	return WriteSecureFile(path, append(header, data...))
}

// SaveToDir writes config.yaml under home with mode 0600.
func SaveToDir(home string, cfg FileConfig) error {
	if strings.TrimSpace(home) == "" {
		return fmt.Errorf("empty config home")
	}
	if _, err := EnsureHomeAt(home); err != nil {
		return err
	}
	return SaveFile(filepath.Join(home, ConfigFileName), cfg)
}

// UpsertProvider adds or replaces a profile by id and persists the config.
// Keys are written only to the local config file (mode 0600); never logged.
func UpsertProvider(home string, profile ProviderProfile) (FileConfig, error) {
	id := strings.TrimSpace(profile.ID)
	if id == "" {
		return FileConfig{}, fmt.Errorf("provider id is required")
	}
	profile.ID = id
	if strings.TrimSpace(profile.Type) == "" {
		profile.Type = "openai_compatible"
	}
	if strings.TrimSpace(profile.Name) == "" {
		profile.Name = id
	}
	cfg, err := LoadFromDir(home)
	if err != nil {
		return FileConfig{}, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	found := false
	for i, p := range cfg.Providers {
		if p.ID == id {
			// Preserve previous key when the new payload omits it (edit form).
			if strings.TrimSpace(profile.APIKey) == "" && strings.TrimSpace(profile.APIKeyEnv) == "" {
				profile.APIKey = p.APIKey
				profile.APIKeyEnv = p.APIKeyEnv
			}
			cfg.Providers[i] = profile
			found = true
			break
		}
	}
	if !found {
		cfg.Providers = append(cfg.Providers, profile)
	}
	if err := SaveToDir(home, cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}

// SetOnboardingDone updates the first-run completion flag and persists.
func SetOnboardingDone(home string, done bool) (FileConfig, error) {
	cfg, err := LoadFromDir(home)
	if err != nil {
		return FileConfig{}, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	cfg.OnboardingDone = done
	if err := SaveToDir(home, cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}

// ResolvedAPIKey returns the effective API key for a profile:
//  1. non-empty inline api_key
//  2. else value of api_key_env from the process environment
// Never logs the key.
func (p ProviderProfile) ResolvedAPIKey() string {
	if k := strings.TrimSpace(p.APIKey); k != "" {
		return k
	}
	if env := strings.TrimSpace(p.APIKeyEnv); env != "" {
		return strings.TrimSpace(os.Getenv(env))
	}
	return ""
}

// ResolvedDefaultModel returns default_model, or the first models[].id when set.
func (p ProviderProfile) ResolvedDefaultModel() string {
	if m := strings.TrimSpace(p.DefaultModel); m != "" {
		return m
	}
	for _, m := range p.Models {
		if id := strings.TrimSpace(m.ID); id != "" {
			return id
		}
	}
	return ""
}

// NormalizedType returns the adapter type, defaulting to openai_compatible.
func (p ProviderProfile) NormalizedType() string {
	t := strings.ToLower(strings.TrimSpace(p.Type))
	if t == "" {
		return "openai_compatible"
	}
	return t
}

// NormalizedAPIFormat returns the effective wire format: APIFormatOpenAI or
// APIFormatAnthropic. Empty / unrecognized APIFormat derives from Type:
// anthropic/claude → anthropic; everything else → openai.
func (p ProviderProfile) NormalizedAPIFormat() string {
	f := strings.ToLower(strings.TrimSpace(p.APIFormat))
	switch f {
	case APIFormatOpenAI, "openai_compatible", "openai-compatible":
		return APIFormatOpenAI
	case APIFormatAnthropic, "claude":
		return APIFormatAnthropic
	}
	switch p.NormalizedType() {
	case "anthropic", "claude":
		return APIFormatAnthropic
	default:
		return APIFormatOpenAI
	}
}

// MaskBaseURL returns a display-safe base URL: strips userinfo and query/fragment,
// so credentials never appear in the UI (VAL-PROV-001).
func MaskBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Best-effort strip of userinfo@ and query.
		if i := strings.Index(raw, "://"); i >= 0 {
			rest := raw[i+3:]
			if at := strings.LastIndex(rest, "@"); at >= 0 {
				rest = rest[at+1:]
			}
			if q := strings.IndexAny(rest, "?#"); q >= 0 {
				rest = rest[:q]
			}
			return raw[:i+3] + rest
		}
		return raw
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	// Drop trailing slash noise for display.
	out := strings.TrimRight(u.String(), "/")
	return out
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// SanitizeProviderID turns a free-form name into a stable profile id.
func SanitizeProviderID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "byok"
	}
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}
