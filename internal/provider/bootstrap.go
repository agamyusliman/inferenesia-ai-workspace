package provider

import (
	"net/url"
	"os"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// Env var names for optional env-only BYOK openai_compatible profile (no gateway required).
//
// Dual-read: INFERENESIA_BYOK_* preferred, then INFERNESIA_BYOK_* (typo), then YURA_AI_BYOK_*.
const (
	EnvBYOKBaseURL       = "INFERENESIA_BYOK_BASE_URL"
	EnvBYOKBaseURLTypo   = "INFERNESIA_BYOK_BASE_URL"
	EnvBYOKBaseURLLegacy = "YURA_AI_BYOK_BASE_URL"
	EnvBYOKAPIKey        = "INFERENESIA_BYOK_API_KEY"
	EnvBYOKAPIKeyTypo    = "INFERNESIA_BYOK_API_KEY"
	EnvBYOKAPIKeyLegacy  = "YURA_AI_BYOK_API_KEY"
	EnvBYOKModel         = "INFERENESIA_BYOK_MODEL"
	EnvBYOKModelTypo     = "INFERNESIA_BYOK_MODEL"
	EnvBYOKModelLegacy   = "YURA_AI_BYOK_MODEL"

	// ProfileBYOK is the default id for the env-driven BYOK profile.
	ProfileBYOK = "byok"

	// EnvAnthropicAPIKey is the standard env var for Anthropic BYOK.
	EnvAnthropicAPIKey = "ANTHROPIC_API_KEY"
	// EnvAnthropicBaseURL optionally overrides the Anthropic API root.
	EnvAnthropicBaseURL = "ANTHROPIC_BASE_URL"
	// EnvAnthropicModel is an optional preferred Anthropic model id.
	EnvAnthropicModel = "ANTHROPIC_MODEL"

	// ProfileAnthropic is the default id for the env/YAML Anthropic profile.
	ProfileAnthropic = "anthropic"

	// Optional 9Router openai_compatible profile (VAL-KIT-009). Chat uses the
	// existing ModelRouter path — no second LLM client.
	EnvNineRouterURL   = "NINEROUTER_URL"
	EnvNineRouterKey   = "NINEROUTER_KEY"
	EnvNineRouterModel = "NINEROUTER_MODEL"
	// ProfileNineRouter is the auto-registered profile id when NINEROUTER_URL is set.
	ProfileNineRouter = "ninerouter"
)

// BootstrapOptions controls multi-profile registration (temp-ai + BYOK).
type BootstrapOptions struct {
	// File is optional YAML config (providers + default_provider).
	File config.FileConfig
	// PreferredID, when set, becomes the router default if registered.
	PreferredID string
}

// Bootstrap builds a Router with:
//   - always: inferenesia from INFERENESIA_* / TEMP_AI_* (and YAML inferenesia/tempai overrides when present)
//   - optional: env BYOK via INFERENESIA_BYOK_BASE_URL + INFERENESIA_BYOK_API_KEY (profile "byok")
//   - optional: env 9Router via NINEROUTER_URL + NINEROUTER_KEY (profile "ninerouter")
//   - optional: each openai_compatible profile from config.FileConfig
//
// Default profile selection (first match wins):
//  1. PreferredID if registered
//  2. File.DefaultProvider if registered
//  3. inferenesia when INFERENESIA_API_KEY / TEMP_AI_API_KEY is set
//  4. byok when BYOK env is set (or first other profile that has a key)
//  5. inferenesia (may fail later with missing key)
//
// Dual credential modes (VAL-CLI-013): gateway-only and BYOK-only both work;
// neither mode requires the other credential set.
// 9Router chat reuses openai_compatible (VAL-KIT-009) — never a parallel agent loop.
//
// Legacy alias: profile id "tempai" is registered as an alias for "inferenesia"
// so existing config.yaml / CLI --profile tempai still resolve.
func Bootstrap(opts BootstrapOptions) *Router {
	r := NewRouter()

	// 1) Base inferenesia from env.
	tempCfg := InferenesiaConfigFromEnv()

	// 2) Env-driven BYOK profile stays opt-in via config.yaml only; the app no
	// longer auto-registers 9Router / Anthropic-from-env profiles.
	if byok := byokConfigFromEnv(); byok != nil {
		r.Register(byok.ID, NewOpenAICompat(*byok))
	}

	// 3) YAML profiles. The gateway id (inferenesia / typo / legacy tempai)
	// overlays the env gateway config; every other entry is a user-declared
	// BYOK profile that routes DIRECTLY to its own base_url — its key is never
	// sent to the gateway host (VAL-CLI-011 / VAL-PROV-009).
	for _, p := range opts.File.Providers {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		switch p.NormalizedType() {
		case "openai_compatible", "openai", "openai-compatible", "anthropic", "claude":
		default:
			continue
		}

		cfg := Config{
			ID:           id,
			Name:         firstNonEmpty(strings.TrimSpace(p.Name), id),
			BaseURL:      strings.TrimSpace(p.BaseURL),
			APIKey:       p.ResolvedAPIKey(),
			DefaultModel: firstNonEmpty(p.ResolvedDefaultModel(), strings.TrimSpace(opts.File.DefaultModel)),
			APIFormat:    p.NormalizedAPIFormat(),
		}

		if id == ProfileInferenesia || id == ProfileInfernesia || id == ProfileTempAI {
			cfg.APIFormat = config.APIFormatOpenAI
			if envBase := firstEnv(EnvBaseURL, EnvBaseURLTypo, EnvBaseURLLegacy); envBase != "" {
				cfg.BaseURL = envBase
			} else if cfg.BaseURL == "" {
				cfg.BaseURL = DefaultTempAIBaseURL
			}
			if k := firstEnv(EnvAPIKey, EnvAPIKeyTypo, EnvAPIKeyLegacy); k != "" {
				cfg.APIKey = k
			}
			if m := firstEnv(EnvModel, EnvModelTypo, EnvModelLegacy); m != "" {
				cfg.DefaultModel = m
			} else if cfg.DefaultModel == "" {
				cfg.DefaultModel = tempCfg.DefaultModel
			}
			if cfg.Name == "" || cfg.Name == id || cfg.Name == "temp-ai" || cfg.Name == "Infernesia API" {
				cfg.Name = DefaultProfileName
			}
			cfg.ID = ProfileInferenesia
			tempCfg = cfg
			continue
		}

		if cfg.BaseURL == "" && cfg.APIFormat != config.APIFormatAnthropic {
			continue
		}
		if cfg.APIFormat == config.APIFormatAnthropic {
			if cfg.BaseURL == "" {
				cfg.BaseURL = DefaultAnthropicBaseURL
			}
			r.Register(id, NewAnthropic(cfg))
			continue
		}
		r.Register(id, NewOpenAICompat(cfg))
	}

	// 3) Always register inferenesia (even with empty key — surface at use time).
	if tempCfg.BaseURL == "" {
		tempCfg.BaseURL = DefaultTempAIBaseURL
	}
	if tempCfg.ID == "" {
		tempCfg.ID = ProfileInferenesia
	}
	if tempCfg.Name == "" || tempCfg.Name == "temp-ai" {
		tempCfg.Name = DefaultProfileName
	}
	r.Register(ProfileInferenesia, NewOpenAICompat(tempCfg))
	// Legacy profile-id aliases still resolve to the gateway client.
	r.RegisterAlias(ProfileInfernesia, ProfileInferenesia)
	r.RegisterAlias(ProfileTempAI, ProfileInferenesia)

	defaultID := pickDefault(r, opts)
	r.SetDefault(defaultID)
	return r
}

// BootstrapDefault is the legacy helper: env inferenesia only (plus env BYOK if set).
func BootstrapDefault() *Router {
	return Bootstrap(BootstrapOptions{})
}

// BootstrapFromHome loads ~/.inferenesia/config.yaml (or INFERENESIA_HOME / legacy
// YURA_AI_HOME / ~/.yura-ai) and bootstraps.
func BootstrapFromHome() (*Router, error) {
	file, err := config.LoadFromHome()
	if err != nil {
		return nil, err
	}
	return Bootstrap(BootstrapOptions{File: file}), nil
}

func byokConfigFromEnv() *Config {
	base := firstEnv(EnvBYOKBaseURL, EnvBYOKBaseURLTypo, EnvBYOKBaseURLLegacy)
	if base == "" {
		return nil
	}
	return &Config{
		ID:           ProfileBYOK,
		Name:         "BYOK",
		BaseURL:      base,
		APIKey:       firstEnv(EnvBYOKAPIKey, EnvBYOKAPIKeyTypo, EnvBYOKAPIKeyLegacy),
		DefaultModel: firstEnv(EnvBYOKModel, EnvBYOKModelTypo, EnvBYOKModelLegacy),
	}
}

// nineRouterConfigFromEnv builds profile "ninerouter" when NINEROUTER_URL is set.
// Base URL is normalized to include /v1 so ChatStream hits /v1/chat/completions.
// Key is never logged (VAL-KIT-010).
func nineRouterConfigFromEnv() *Config {
	base := strings.TrimSpace(os.Getenv(EnvNineRouterURL))
	if base == "" {
		return nil
	}
	base = normalizeNineRouterBaseURL(base)
	return &Config{
		ID:           ProfileNineRouter,
		Name:         "9Router",
		BaseURL:      base,
		APIKey:       strings.TrimSpace(os.Getenv(EnvNineRouterKey)),
		DefaultModel: strings.TrimSpace(os.Getenv(EnvNineRouterModel)),
	}
}

// normalizeNineRouterBaseURL ensures an OpenAI-compatible /v1 root.
func normalizeNineRouterBaseURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return base
	}
	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, "/v1") {
		return base
	}
	// Already ends with /v1/something — leave as-is only if path is just /v1.
	return base + "/v1"
}

// anthropicConfigFromEnv builds the default Anthropic profile from env.
// Always returns a config (base defaults to api.anthropic.com); empty key
// surfaces as blocked/key-required at use time (VAL-PROV-008).
func anthropicConfigFromEnv() Config {
	base := strings.TrimSpace(os.Getenv(EnvAnthropicBaseURL))
	if base == "" {
		base = DefaultAnthropicBaseURL
	}
	return Config{
		ID:           ProfileAnthropic,
		Name:         "Anthropic",
		BaseURL:      base,
		APIKey:       strings.TrimSpace(os.Getenv(EnvAnthropicAPIKey)),
		DefaultModel: strings.TrimSpace(os.Getenv(EnvAnthropicModel)),
	}
}

func pickDefault(r *Router, opts BootstrapOptions) string {
	// 1) Explicit preferred (CLI --profile).
	// Both this and YAML default_provider may name a legacy alias ("tempai").
	// Return the canonical registered id so DefaultID() always matches an entry
	// in IDs() — settings/session views key profile rows off that equality.
	if id := strings.TrimSpace(opts.PreferredID); id != "" {
		if _, err := r.Get(id); err == nil {
			return r.CanonicalID(id)
		}
	}
	// 2) YAML default_provider.
	if id := strings.TrimSpace(opts.File.DefaultProvider); id != "" {
		if _, err := r.Get(id); err == nil {
			return r.CanonicalID(id)
		}
	}
	// 3) inferenesia when gateway key present (dual-read env).
	if firstEnv(EnvAPIKey, EnvAPIKeyTypo, EnvAPIKeyLegacy) != "" {
		return ProfileInferenesia
	}
	// 4) env BYOK when base (+ preferably key) is set.
	if firstEnv(EnvBYOKBaseURL, EnvBYOKBaseURLTypo, EnvBYOKBaseURLLegacy) != "" {
		if _, err := r.Get(ProfileBYOK); err == nil {
			return ProfileBYOK
		}
	}
	// 5) First non-gateway profile that has credentials (openai or anthropic).
	for _, id := range r.IDs() {
		if id == ProfileInferenesia {
			continue
		}
		c, err := r.Get(id)
		if err != nil {
			continue
		}
		if p, ok := c.(Profile); ok && p.APIKeyConfigured() {
			return id
		}
	}
	// 6) Fallback inferenesia (missing key surfaces on use).
	return ProfileInferenesia
}

// firstNonEmpty returns the first non-empty trimmed string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// RequestHost returns the host (scheme://host[:port]) for a base URL, suitable for
// verbose/trace output (VAL-CLI-011). Never includes API keys or query tokens.
func RequestHost(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		// Best-effort fallback without credentials.
		if i := strings.Index(baseURL, "://"); i >= 0 {
			rest := baseURL[i+3:]
			if j := strings.IndexAny(rest, "/?"); j >= 0 {
				rest = rest[:j]
			}
			return baseURL[:i+3] + rest
		}
		return baseURL
	}
	return u.Scheme + "://" + u.Host
}

// IsGatewayHost reports whether hostOrURL targets the Inferenesia Cloud
// gateway (default `inferenesia.cloud`) OR the legacy `ai.temp.web.id`
// host that is still readable via dual-read.
func IsGatewayHost(hostOrURL string) bool {
	h := strings.ToLower(RequestHost(hostOrURL))
	if h == "" {
		h = strings.ToLower(hostOrURL)
	}
	return strings.Contains(h, "inferenesia.cloud") || strings.Contains(h, "ai.temp.web.id")
}

// IsTempAIHost is retained as the legacy alias for IsGatewayHost so existing
// callers keep compiling.
func IsTempAIHost(hostOrURL string) bool {
	return IsGatewayHost(hostOrURL)
}
