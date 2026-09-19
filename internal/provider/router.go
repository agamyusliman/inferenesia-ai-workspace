package provider

import (
	"fmt"
	"os"
	"strings"
)

// Env var names for the default Inferenesia API gateway profile.
//
// Dual-read: INFERENESIA_* preferred; INFERNESIA_* (typo brand) then TEMP_AI_*
// still work. The gateway host default URL stays https://ai.temp.web.id/v1
// until DNS cutover; the user-facing label is "Inferenesia API" (see
// internal/brand).
const (
	EnvBaseURL       = "INFERENESIA_BASE_URL"
	EnvBaseURLTypo   = "INFERNESIA_BASE_URL"
	EnvBaseURLLegacy = "TEMP_AI_BASE_URL"
	EnvAPIKey        = "INFERENESIA_API_KEY"
	EnvAPIKeyTypo    = "INFERNESIA_API_KEY"
	EnvAPIKeyLegacy  = "TEMP_AI_API_KEY"
	EnvModel         = "INFERENESIA_MODEL"
	EnvModelTypo     = "INFERNESIA_MODEL"
	EnvModelLegacy   = "TEMP_AI_MODEL"

	// DefaultTempAIBaseURL is the shipped gateway host when no base URL env is
	// set. Named after the legacy const for dual-read compatibility; the
	// current default is Inferenesia Cloud. The legacy host `ai.temp.web.id/v1`
	// remains functional via TEMP_AI_BASE_URL / config.yaml overlay.
	DefaultTempAIBaseURL = "https://inferenesia.cloud/v1"

	// ProfileInferenesia is the canonical default profile id for the gateway.
	ProfileInferenesia = "inferenesia"
	// ProfileInfernesia is the typo-brand profile id alias (still accepted).
	ProfileInfernesia = "infernesia"
	// ProfileTempAI is the legacy profile id alias (still accepted on lookup
	// and registered as an alias pointing at the same client).
	ProfileTempAI = "tempai"

	// DefaultProfileName is the user-facing display name for the gateway profile.
	DefaultProfileName = "Inferenesia API"
)

// firstEnv returns the first non-empty (trimmed) value from the given env var
// names, in order. Used for legacy dual-read across env renames.
func firstEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// Router resolves named provider profiles to ChatClient instances.
type Router struct {
	clients map[string]ChatClient
	// aliases maps alternate profile ids (e.g. legacy "tempai") to canonical ids.
	aliases map[string]string
	// defaultID is used when Get is called with empty id.
	defaultID string
}

// NewRouter returns an empty router.
func NewRouter() *Router {
	return &Router{
		clients: make(map[string]ChatClient),
		aliases: make(map[string]string),
	}
}

// Register adds or replaces a client under id.
func (r *Router) Register(id string, client ChatClient) {
	if r.clients == nil {
		r.clients = make(map[string]ChatClient)
	}
	if r.aliases == nil {
		r.aliases = make(map[string]string)
	}
	r.clients[id] = client
	if r.defaultID == "" {
		r.defaultID = id
	}
}

// RegisterAlias maps aliasID → canonicalID so legacy profile ids (e.g. "tempai")
// resolve to the same client as the canonical id (e.g. "inferenesia"). The
// alias must point at an already-registered canonical id; otherwise it is
// ignored. Lookup precedence: exact id → alias.
func (r *Router) RegisterAlias(aliasID, canonicalID string) {
	if aliasID == "" || canonicalID == "" || aliasID == canonicalID {
		return
	}
	if r.aliases == nil {
		r.aliases = make(map[string]string)
	}
	r.aliases[aliasID] = canonicalID
}

// SetDefault selects the default profile id.
func (r *Router) SetDefault(id string) {
	r.defaultID = id
}

// DefaultID returns the current default profile id.
func (r *Router) DefaultID() string {
	return r.defaultID
}

// IDs returns registered profile ids (unsorted).
func (r *Router) IDs() []string {
	if r == nil || r.clients == nil {
		return nil
	}
	out := make([]string, 0, len(r.clients))
	for id := range r.clients {
		out = append(out, id)
	}
	return out
}

// CanonicalID resolves a profile id to the id it is registered under, so legacy
// aliases ("tempai") never leak into persisted config or session state. Empty
// id resolves to the default. Unknown ids are returned unchanged so callers can
// still produce an "unknown provider profile" error with the user's spelling.
func (r *Router) CanonicalID(id string) string {
	if r == nil {
		return id
	}
	if id == "" {
		id = r.defaultID
	}
	if _, ok := r.clients[id]; ok {
		return id
	}
	if canonical, ok := r.aliases[id]; ok {
		if _, ok := r.clients[canonical]; ok {
			return canonical
		}
	}
	return id
}

// Get returns a registered client. Empty id uses the default. Legacy profile
// id aliases (e.g. "tempai" → "inferenesia") resolve to the canonical client.
func (r *Router) Get(id string) (ChatClient, error) {
	if id == "" {
		id = r.defaultID
	}
	if id == "" {
		return nil, fmt.Errorf("no default provider profile registered")
	}
	c, ok := r.clients[id]
	if !ok {
		if canonical, aok := r.aliases[id]; aok {
			c, ok = r.clients[canonical]
		}
	}
	if !ok {
		return nil, fmt.Errorf("unknown provider profile %q", id)
	}
	return c, nil
}

// GetOpenAI returns the OpenAI-compatible client for id (empty = default),
// or an error if the profile is not openai_compatible.
func (r *Router) GetOpenAI(id string) (*OpenAICompat, error) {
	c, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	oc, ok := c.(*OpenAICompat)
	if !ok {
		return nil, fmt.Errorf("provider profile %q is not openai_compatible", firstNonEmpty(id, r.defaultID))
	}
	return oc, nil
}

// GetAnthropic returns the Anthropic adapter for id (empty = default),
// or an error if the profile is not anthropic.
func (r *Router) GetAnthropic(id string) (*Anthropic, error) {
	c, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	ac, ok := c.(*Anthropic)
	if !ok {
		return nil, fmt.Errorf("provider profile %q is not anthropic", firstNonEmpty(id, r.defaultID))
	}
	return ac, nil
}

// GetProfile returns a Profile for any registered adapter (openai_compatible or anthropic).
// Prefer this over GetOpenAI for chat/settings so Anthropic BYOK routes correctly (VAL-PROV-007).
func (r *Router) GetProfile(id string) (Profile, error) {
	c, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if p, ok := c.(Profile); ok {
		return p, nil
	}
	return nil, fmt.Errorf("provider profile %q does not implement Profile", firstNonEmpty(id, r.defaultID))
}

// InferenesiaConfigFromEnv builds a Config for the Inferenesia API gateway from
// process env (INFERENESIA_* → INFERNESIA_* → TEMP_AI_*).
// Model is optional. API key may be empty (surfaced at use time). Base URL
// defaults to DefaultTempAIBaseURL when neither env is set.
func InferenesiaConfigFromEnv() Config {
	base := firstEnv(EnvBaseURL, EnvBaseURLTypo, EnvBaseURLLegacy)
	if base == "" {
		base = DefaultTempAIBaseURL
	}
	return Config{
		ID:           ProfileInferenesia,
		Name:         DefaultProfileName,
		BaseURL:      base,
		APIKey:       firstEnv(EnvAPIKey, EnvAPIKeyTypo, EnvAPIKeyLegacy),
		DefaultModel: firstEnv(EnvModel, EnvModelTypo, EnvModelLegacy),
	}
}

// TempAIConfigFromEnv is the legacy alias for InferenesiaConfigFromEnv.
// Kept for callers and tests that still reference the old name; returns the
// canonical Inferenesia config (ProfileInferenesia, label "Inferenesia API").
func TempAIConfigFromEnv() Config {
	return InferenesiaConfigFromEnv()
}

// NewTempAIClientFromEnv registers nothing; it returns a ready openai_compatible client
// for the gateway env only (does not load BYOK YAML profiles).
func NewTempAIClientFromEnv() *OpenAICompat {
	return NewOpenAICompat(InferenesiaConfigFromEnv())
}

// Note: BootstrapDefault and multi-profile Bootstrap live in bootstrap.go.
