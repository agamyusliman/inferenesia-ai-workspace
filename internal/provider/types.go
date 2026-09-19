// Package provider implements ModelRouter and OpenAI-compatible / Anthropic
// clients for Inferenesia. The default temp-ai gateway profile uses
// openai_compatible against TEMP_AI_* env vars.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Model is a discoverable model id from a provider.
type Model struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// Role identifies a chat message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is one function call requested by the model (OpenAI-compatible).
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ContentPart is one multimodal content unit for user messages (text and/or images).
// When Parts is non-empty on Message, providers that support vision serialize them as
// OpenAI-style content arrays; text-only clients fall back to Content.
type ContentPart struct {
	// Type is "text" or "image_url".
	Type string `json:"type"`
	// Text is set when Type == "text".
	Text string `json:"text,omitempty"`
	// ImageURL is a data: or https: URL when Type == "image_url".
	ImageURL string `json:"image_url,omitempty"`
	// MediaType is optional MIME for data URLs (image/png, …).
	MediaType string `json:"media_type,omitempty"`
}

// Message is one chat turn. For RoleTool, ToolCallID links the result.
// Assistant messages may include ToolCalls when the model requested tools.
// User messages may set Parts for multimodal vision (VAL-CHAT-022); Content remains
// the plain-text / durable fallback.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	// Parts is optional multimodal content (text + image_url data URLs). Empty for
	// text-only turns so history and stores stay simple.
	Parts      []ContentPart `json:"parts,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	// Name is optional tool name on tool-role messages (some providers expect it).
	Name string `json:"name,omitempty"`
}

// ToolDef is an OpenAI-compatible tool schema passed in ChatRequest.Tools.
type ToolDef struct {
	Type     string `json:"type"` // "function"
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// ChatRequest is a chat completion request (optionally with tools).
type ChatRequest struct {
	Model    string
	Messages []Message
	// Tools enables tool calling when non-empty (OpenAI tools array).
	Tools []ToolDef
	// Stream requests SSE token deltas when true (default for CLI chat).
	Stream bool
	// ToolChoice controls tool selection ("auto", "none", or omit for default auto when tools present).
	ToolChoice string
}

// EventType classifies StreamEvent values.
type EventType string

const (
	EventDelta     EventType = "delta"
	EventDone      EventType = "done"
	EventError     EventType = "error"
	EventToolCalls EventType = "tool_calls" // non-stream or stream finish with tool calls
)

// StreamEvent is one unit from ChatStream / Chat (with tools).
type StreamEvent struct {
	Type  EventType
	Delta string // partial assistant text when Type == EventDelta
	// Final is the fully assembled assistant message on EventDone (when available).
	Final string
	// ToolCalls is set on EventToolCalls (and may also be set on EventDone when finish_reason is tool_calls).
	ToolCalls []ToolCall
	// FinishReason is the OpenAI finish_reason when known ("stop", "tool_calls", …).
	FinishReason string
	Err          error
}

// ChatClient is the transport-agnostic model provider interface.
type ChatClient interface {
	// Name returns a stable profile id (e.g. "tempai").
	Name() string
	// ListModels returns model ids from the provider (e.g. GET /v1/models).
	ListModels(ctx context.Context) ([]Model, error)
	// ChatStream streams completion events. Callers must drain the channel.
	// Cancellation via ctx cancels the upstream HTTP request.
	ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
}

// Sentinel / classified errors for CLI messaging (VAL-CLI-008/009).
var (
	// ErrMissingAPIKey is returned when no API key is configured for the active profile.
	// Message is generic so BYOK and temp-ai modes share one sentinel (profile details in CLI).
	ErrMissingAPIKey = errors.New("missing API key")
	// ErrUnauthorized is returned for HTTP 401/403 from the provider.
	ErrUnauthorized = errors.New("unauthorized: invalid or expired API key")
	// ErrModelNotFound is returned when TEMP_AI_MODEL (or request model) is invalid.
	ErrModelNotFound = errors.New("model not found")
	// ErrEmptyModelList is returned when discovery succeeds but returns no models.
	ErrEmptyModelList = errors.New("no models available from provider")
	// ErrGatewayUnavailable is the underlying sentinel wrapped by GatewayError
	// when the temp-ai gateway is unreachable / returns 401/5xx (VAL-CROSS-009).
	// Never embeds the API key value; never triggers fake success.
	ErrGatewayUnavailable = errors.New("gateway unavailable")
)

// BlockedError is a clear "blocked / key required" state for a profile that
// cannot chat until credentials are configured (VAL-PROV-008).
// It is never a silent crash and never triggers a temp-ai proxy fallback.
type BlockedError struct {
	// Profile is the provider profile id (e.g. "anthropic").
	Profile string
	// Reason is a short operator-facing code ("key required").
	Reason string
	// Cause is the underlying sentinel (typically ErrMissingAPIKey).
	Cause error
}

func (e *BlockedError) Error() string {
	if e == nil {
		return "blocked: key required"
	}
	id := e.Profile
	if id == "" {
		id = "provider"
	}
	reason := e.Reason
	if reason == "" {
		reason = "key required"
	}
	// Explicit wording for UI/CLI: blocked + key required; notes isolation from temp-ai.
	return fmt.Sprintf("blocked: %s for profile %q — configure an API key (BYOK keys go direct to the provider; not proxied through temp-ai)", reason, id)
}

func (e *BlockedError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Cause != nil {
		return e.Cause
	}
	return ErrMissingAPIKey
}

// IsBlocked reports whether err is a BlockedError (key required / skip-blocked).
func IsBlocked(err error) bool {
	if err == nil {
		return false
	}
	var be *BlockedError
	return errors.As(err, &be)
}

func errorsIsBlocked(err error) bool {
	return IsBlocked(err)
}

// Config holds openai_compatible connection settings for one profile.
type Config struct {
	// ID is the profile id (e.g. "tempai").
	ID string
	// Name is a human label (e.g. "temp-ai").
	Name string
	// BaseURL is the OpenAI-compatible root including /v1 when used by the gateway.
	// Paths are joined as baseURL + "/models" and baseURL + "/chat/completions".
	BaseURL string
	// APIKey is the bearer token. Never log this value.
	APIKey string
	// DefaultModel is an optional preferred model id (from TEMP_AI_MODEL).
	// Empty means discover via ListModels and auto-select.
	DefaultModel string
	// HTTPTimeout is the dial/header timeout bound for fail-fast (default 15s).
	// Streaming responses themselves are not hard-capped by this value.
	HTTPTimeoutSeconds int
	// APIFormat records the effective wire format ("openai" | "anthropic")
	// carried from the profile so adapters know which protocol the host speaks.
	// Adapters may ignore this when they already encode a single protocol.
	APIFormat string
}
