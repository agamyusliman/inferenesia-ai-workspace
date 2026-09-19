package provider

import "context"

// Profile is the shared ModelRouter client surface used by core.Service, CLI, and
// settings. Both openai_compatible and anthropic adapters implement this.
// Callers must not type-assert only to *OpenAICompat when routing mid-session
// chat (Anthropic BYOK is first-class; VAL-PROV-007/008).
type Profile interface {
	ChatClient
	// BaseURL returns the configured API root (no secrets).
	BaseURL() string
	// RequestHost returns scheme://host for verbose/trace (never keys).
	RequestHost() string
	// APIKeyConfigured reports whether a non-empty API key is set.
	APIKeyConfigured() bool
	// DefaultModel returns the preferred model override (may be empty).
	DefaultModel() string
	// SetDefaultModel updates the preferred model (session apply).
	SetDefaultModel(id string)
	// ResolveModel picks a model id (auto from list when preferred empty).
	ResolveModel(ctx context.Context, preferred string) (string, error)
	// ChatTurn runs a non-streaming completion (test call + tool loop fallback).
	ChatTurn(ctx context.Context, req ChatRequest) (Message, error)
}

// AdapterType returns a stable type label for a profile client.
func AdapterType(c ChatClient) string {
	switch c.(type) {
	case *Anthropic:
		return TypeAnthropic
	case *OpenAICompat:
		return TypeOpenAICompatible
	default:
		return "unknown"
	}
}

// Provider type constants (config.yaml type field).
const (
	TypeOpenAICompatible = "openai_compatible"
	TypeAnthropic        = "anthropic"
)
