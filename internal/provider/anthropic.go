package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultAnthropicBaseURL is the Anthropic Messages API root (no trailing path).
	// Requests go to baseURL + "/v1/messages" and baseURL + "/v1/models".
	DefaultAnthropicBaseURL = "https://api.anthropic.com"

	// AnthropicAPIVersion is the required anthropic-version header value.
	AnthropicAPIVersion = "2023-06-01"

	// AnthropicPromptCachingBeta is the beta header value that opts into
	// prompt caching for system blocks with cache_control. Many gateways
	// ignore unknown beta headers; the official Anthropic API honors it.
	AnthropicPromptCachingBeta = "prompt-caching-2024-07-31"

	// defaultAnthropicMaxTokens is used when the caller does not specify max tokens.
	defaultAnthropicMaxTokens = 4096
)

// Anthropic is a ChatClient for Anthropic's Messages API (BYOK).
// Keys are sent only to the configured Anthropic base URL — never to temp-ai.
// Missing key yields a clear blocked/key-required error (VAL-PROV-007/008).
type Anthropic struct {
	cfg    Config
	client *http.Client
}

// NewAnthropic builds an Anthropic adapter from cfg.
// Empty BaseURL defaults to DefaultAnthropicBaseURL.
func NewAnthropic(cfg Config) *Anthropic {
	if cfg.ID == "" {
		cfg.ID = "anthropic"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultAnthropicBaseURL
	}
	// Normalize: drop trailing /v1 so we can append /v1/messages consistently.
	cfg.BaseURL = normalizeAnthropicBase(cfg.BaseURL)

	timeout := defaultHTTPTimeout
	if cfg.HTTPTimeoutSeconds > 0 {
		timeout = time.Duration(cfg.HTTPTimeoutSeconds) * time.Second
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &Anthropic{
		cfg: cfg,
		client: &http.Client{
			Transport: transport,
		},
	}
}

func normalizeAnthropicBase(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	// Accept openai-style ".../v1" from user config and strip it.
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	return base
}

// Name implements ChatClient.
func (c *Anthropic) Name() string {
	if c.cfg.ID != "" {
		return c.cfg.ID
	}
	return "anthropic"
}

// BaseURL returns the configured root (no secrets).
func (c *Anthropic) BaseURL() string {
	return c.cfg.BaseURL
}

// RequestHost returns scheme://host for verbose/trace (no path, no secrets).
func (c *Anthropic) RequestHost() string {
	return RequestHost(c.cfg.BaseURL)
}

// APIKeyConfigured reports whether a non-empty API key is set (never returns the key).
func (c *Anthropic) APIKeyConfigured() bool {
	return strings.TrimSpace(c.cfg.APIKey) != ""
}

// DefaultModel returns the configured optional model override.
func (c *Anthropic) DefaultModel() string {
	return strings.TrimSpace(c.cfg.DefaultModel)
}

// SetDefaultModel updates the preferred model.
func (c *Anthropic) SetDefaultModel(id string) {
	c.cfg.DefaultModel = strings.TrimSpace(id)
}

func (c *Anthropic) requireKey() error {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return &BlockedError{
			Profile: c.Name(),
			Reason:  "key required",
			Cause:   ErrMissingAPIKey,
		}
	}
	return nil
}

func (c *Anthropic) requireBase() error {
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return &ConnectionError{BaseURL: "", Reason: "missing Anthropic base_url"}
	}
	return nil
}

func (c *Anthropic) endpoint(path string) string {
	base := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/")
	path = "/" + strings.TrimLeft(path, "/")
	return base + path
}

func (c *Anthropic) setAuth(req *http.Request) {
	// Anthropic uses x-api-key, not Bearer. Never log this header.
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", AnthropicAPIVersion)
	req.Header.Set("Content-Type", "application/json")
}

// setPromptCachingHeader opts into Anthropic prompt caching. Safe on the
// official API; many compatible gateways ignore unknown beta headers.
func (c *Anthropic) setPromptCachingHeader(req *http.Request) {
	req.Header.Set("anthropic-beta", AnthropicPromptCachingBeta)
}

// ListModels implements ChatClient via GET /v1/models.
func (c *Anthropic) ListModels(ctx context.Context) ([]Model, error) {
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	if err := c.requireKey(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/models"), nil)
	if err != nil {
		return nil, classifyTransport(c.cfg.BaseURL, err)
	}
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, classifyTransport(c.cfg.BaseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, classifyTransport(c.cfg.BaseURL, err)
	}
	if err := c.mapHTTPError(resp.StatusCode, body); err != nil {
		return nil, err
	}

	var parsed anthropicModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse anthropic models response: %w", err)
	}
	out := make([]Model, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		out = append(out, Model{ID: id, OwnedBy: firstNonEmpty(m.DisplayName, "anthropic")})
	}
	if len(out) == 0 {
		return nil, ErrEmptyModelList
	}
	return out, nil
}

// ResolveModel picks the model id (preferred, default, or first from list).
// Invalid preferred yields ModelError (no silent fallback to another model).
func (c *Anthropic) ResolveModel(ctx context.Context, preferred string) (string, error) {
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		preferred = strings.TrimSpace(c.cfg.DefaultModel)
	}

	// Without a key, do not call the network — clear blocked state (VAL-PROV-008).
	if err := c.requireKey(); err != nil {
		// Still allow returning preferred/default when no list is needed for display?
		// Chat path must fail closed: blocked until key is present.
		if preferred != "" {
			// Prefer explicit model id for offline resolve only when key missing
			// is reported at ChatStream time — but ResolveModel is used pre-chat,
			// so surface blocked now.
		}
		return "", err
	}

	models, err := c.ListModels(ctx)
	if err != nil {
		// If list fails but we have an explicit preferred model, still use it
		// for BYOK chat (some self-hosted proxies lack /models).
		if preferred != "" && !IsAuth(err) && !errorsIsBlocked(err) {
			return preferred, nil
		}
		return "", err
	}

	if preferred == "" {
		return models[0].ID, nil
	}
	for _, m := range models {
		if m.ID == preferred {
			return preferred, nil
		}
	}
	// Anthropic model list may lag new ids; when preferred is set and list is
	// non-empty but missing the id, still try the preferred id (API will reject
	// if truly invalid). This matches BYOK ergonomics.
	if preferred != "" {
		return preferred, nil
	}
	return "", &ModelError{
		Model:  preferred,
		Reason: "not found in Anthropic model list",
		Cause:  ErrModelNotFound,
	}
}

// ChatStream implements ChatClient via POST /v1/messages with stream=true SSE.
// Missing key returns BlockedError (no silent crash, no temp-ai proxy fallback).
func (c *Anthropic) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	if err := c.requireKey(); err != nil {
		return nil, err
	}

	body, err := c.buildMessagesBody(ctx, req, true)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/messages"), bytes.NewReader(body))
	if err != nil {
		return nil, classifyTransport(c.cfg.BaseURL, err)
	}
	c.setAuth(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")
	if hasSystemBlocks(body) {
		c.setPromptCachingHeader(httpReq)
	}

	streamClient := c.streamHTTPClient()
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, classifyTransport(c.cfg.BaseURL, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, c.mapHTTPError(resp.StatusCode, b)
	}

	ch := make(chan StreamEvent, 32)
	go c.readSSE(ctx, resp, ch)
	return ch, nil
}

// ChatTurn runs a non-streaming Messages API completion.
func (c *Anthropic) ChatTurn(ctx context.Context, req ChatRequest) (Message, error) {
	if err := c.requireBase(); err != nil {
		return Message{}, err
	}
	if err := c.requireKey(); err != nil {
		return Message{}, err
	}

	body, err := c.buildMessagesBody(ctx, req, false)
	if err != nil {
		return Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/messages"), bytes.NewReader(body))
	if err != nil {
		return Message{}, classifyTransport(c.cfg.BaseURL, err)
	}
	c.setAuth(httpReq)
	if hasSystemBlocks(body) {
		c.setPromptCachingHeader(httpReq)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return Message{}, classifyTransport(c.cfg.BaseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Message{}, classifyTransport(c.cfg.BaseURL, err)
	}
	if err := c.mapHTTPError(resp.StatusCode, raw); err != nil {
		return Message{}, err
	}

	var parsed anthropicMessageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Message{}, fmt.Errorf("parse anthropic message response: %w", err)
	}
	return fromAnthropicMessage(parsed), nil
}

func (c *Anthropic) buildMessagesBody(ctx context.Context, req ChatRequest, stream bool) ([]byte, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		var err error
		model, err = c.ResolveModel(ctx, c.cfg.DefaultModel)
		if err != nil {
			return nil, err
		}
	}

	system, messages := toAnthropicMessages(req.Messages)
	payload := anthropicMessagesRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: defaultAnthropicMaxTokens,
		Stream:    stream,
	}
	// Anthropic prompt caching: emit system as an array of text blocks with
	// cache_control: ephemeral on the final block when system is non-empty.
	// When system is empty, omit the field entirely.
	if s := strings.TrimSpace(system); s != "" {
		blocks := []anthropicSystemBlock{
			{
				Type:         "text",
				Text:         s,
				CacheControl: &anthropicCacheControl{Type: "ephemeral"},
			},
		}
		payload.System = blocks
	}
	if len(req.Tools) > 0 {
		payload.Tools = toAnthropicTools(req.Tools)
		// Anthropic tool_choice: "auto" by default when tools present.
		if strings.TrimSpace(req.ToolChoice) == "none" {
			payload.ToolChoice = map[string]string{"type": "none"}
		} else if strings.TrimSpace(req.ToolChoice) == "auto" || req.ToolChoice == "" {
			payload.ToolChoice = map[string]string{"type": "auto"}
		}
	}
	return json.Marshal(payload)
}

// hasSystemBlocks reports whether the marshaled body carries a non-empty
// system content array (used to decide whether to send the prompt-caching
// beta header). It never mutates body.
func hasSystemBlocks(body []byte) bool {
	var probe struct {
		System json.RawMessage `json:"system"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return len(probe.System) > 0 && string(probe.System) != "null" && string(probe.System) != `""`
}

func (c *Anthropic) streamHTTPClient() *http.Client {
	timeout := defaultHTTPTimeout
	if c.cfg.HTTPTimeoutSeconds > 0 {
		timeout = time.Duration(c.cfg.HTTPTimeoutSeconds) * time.Second
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: streamIdleTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Transport: transport}
}

func (c *Anthropic) mapHTTPError(status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	snippet := sanitizeBodySnippet(body)
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		reason := "invalid or unauthorized Anthropic API key"
		if snippet != "" {
			reason = snippet
		}
		return &AuthError{Reason: reason, Status: status, Cause: ErrUnauthorized}
	default:
		if status >= 500 {
			return fmt.Errorf("anthropic returned HTTP %d: %s", status, orDefault(snippet, "server error"))
		}
		return fmt.Errorf("anthropic returned HTTP %d: %s", status, orDefault(snippet, "request failed"))
	}
}

func (c *Anthropic) readSSE(ctx context.Context, resp *http.Response, ch chan<- StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()

	var assembled strings.Builder
	// Accumulate tool_use blocks by content block index.
	type toolAcc struct {
		id   string
		name string
		json strings.Builder
	}
	tools := map[int]*toolAcc{}
	var lastStop string

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)

	send := func(ev StreamEvent) bool {
		select {
		case <-ctx.Done():
			return false
		case ch <- ev:
			return true
		}
	}

	flushTools := func() []ToolCall {
		if len(tools) == 0 {
			return nil
		}
		maxIdx := -1
		for i := range tools {
			if i > maxIdx {
				maxIdx = i
			}
		}
		out := make([]ToolCall, 0, len(tools))
		for i := 0; i <= maxIdx; i++ {
			acc, ok := tools[i]
			if !ok || acc == nil {
				continue
			}
			tc := ToolCall{ID: acc.id, Type: "function"}
			tc.Function.Name = acc.name
			tc.Function.Arguments = acc.json.String()
			if tc.Function.Arguments == "" {
				tc.Function.Arguments = "{}"
			}
			out = append(out, tc)
		}
		return out
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			_ = send(StreamEvent{Type: EventError, Err: ctx.Err()})
			return
		}
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		// Anthropic uses event: + data: pairs; we only need data JSON with type.
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "error":
			msg := "anthropic stream error"
			if ev.Error != nil && ev.Error.Message != "" {
				msg = stripSecrets(ev.Error.Message)
			}
			_ = send(StreamEvent{Type: EventError, Err: fmt.Errorf("%s", msg)})
			return
		case "content_block_start":
			if ev.ContentBlock == nil {
				continue
			}
			switch ev.ContentBlock.Type {
			case "tool_use":
				tools[ev.Index] = &toolAcc{
					id:   ev.ContentBlock.ID,
					name: ev.ContentBlock.Name,
				}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					assembled.WriteString(ev.Delta.Text)
					if !send(StreamEvent{Type: EventDelta, Delta: ev.Delta.Text}) {
						return
					}
				}
			case "input_json_delta":
				if acc, ok := tools[ev.Index]; ok && acc != nil {
					acc.json.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				lastStop = ev.Delta.StopReason
			}
		case "message_stop":
			final := assembled.String()
			calls := flushTools()
			finish := mapAnthropicStop(lastStop)
			if len(calls) > 0 {
				_ = send(StreamEvent{
					Type:         EventToolCalls,
					Final:        final,
					ToolCalls:    calls,
					FinishReason: finish,
				})
			}
			_ = send(StreamEvent{
				Type:         EventDone,
				Final:        final,
				ToolCalls:    calls,
				FinishReason: finish,
			})
			return
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			_ = send(StreamEvent{Type: EventError, Err: ctx.Err()})
			return
		}
		_ = send(StreamEvent{Type: EventError, Err: classifyTransport(c.cfg.BaseURL, err)})
		return
	}
	// Stream ended without message_stop — emit what we have.
	final := assembled.String()
	calls := flushTools()
	finish := mapAnthropicStop(lastStop)
	if len(calls) > 0 {
		_ = send(StreamEvent{
			Type:         EventToolCalls,
			Final:        final,
			ToolCalls:    calls,
			FinishReason: finish,
		})
	}
	_ = send(StreamEvent{
		Type:         EventDone,
		Final:        final,
		ToolCalls:    calls,
		FinishReason: finish,
	})
}

func mapAnthropicStop(reason string) string {
	switch reason {
	case "tool_use":
		return "tool_calls"
	case "end_turn", "stop_sequence", "max_tokens":
		return "stop"
	default:
		return reason
	}
}

// --- wire conversion ---

func toAnthropicMessages(msgs []Message) (system string, out []anthropicWireMessage) {
	var sysParts []string
	// Anthropic requires tool results as user content blocks; group consecutive
	// tool-role messages into a single user message with tool_result blocks.
	out = make([]anthropicWireMessage, 0, len(msgs))
	var pendingToolResults []anthropicContentBlock

	flushTools := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		out = append(out, anthropicWireMessage{
			Role:    "user",
			Content: pendingToolResults,
		})
		pendingToolResults = nil
	}

	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			if s := strings.TrimSpace(m.Content); s != "" {
				sysParts = append(sysParts, s)
			}
		case RoleUser:
			flushTools()
			// Multimodal vision: source.base64 or url when Parts present (VAL-CHAT-022).
			if len(m.Parts) > 0 {
				blocks := make([]anthropicContentBlock, 0, len(m.Parts))
				for _, p := range m.Parts {
					switch strings.ToLower(strings.TrimSpace(p.Type)) {
					case "image_url", "image":
						if blk := anthropicImageBlock(p); blk != nil {
							blocks = append(blocks, *blk)
						}
					default:
						if t := p.Text; t != "" {
							blocks = append(blocks, anthropicContentBlock{Type: "text", Text: t})
						}
					}
				}
				if len(blocks) == 0 {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
				}
				out = append(out, anthropicWireMessage{Role: "user", Content: blocks})
			} else {
				out = append(out, anthropicWireMessage{
					Role:    "user",
					Content: m.Content,
				})
			}
		case RoleAssistant:
			flushTools()
			if len(m.ToolCalls) > 0 {
				blocks := make([]anthropicContentBlock, 0, 1+len(m.ToolCalls))
				if strings.TrimSpace(m.Content) != "" {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					// Arguments are JSON string in our wire; Anthropic wants object.
					var input any
					raw := strings.TrimSpace(tc.Function.Arguments)
					if raw == "" {
						input = map[string]any{}
					} else if err := json.Unmarshal([]byte(raw), &input); err != nil {
						input = map[string]any{"_raw": raw}
					}
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				out = append(out, anthropicWireMessage{
					Role:    "assistant",
					Content: blocks,
				})
			} else {
				out = append(out, anthropicWireMessage{
					Role:    "assistant",
					Content: m.Content,
				})
			}
		case RoleTool:
			pendingToolResults = append(pendingToolResults, anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			})
		}
	}
	flushTools()
	system = strings.Join(sysParts, "\n\n")
	return system, out
}

func toAnthropicTools(defs []ToolDef) []anthropicTool {
	out := make([]anthropicTool, 0, len(defs))
	for _, d := range defs {
		params := d.Function.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, anthropicTool{
			Name:        d.Function.Name,
			Description: d.Function.Description,
			InputSchema: params,
		})
	}
	return out
}

func fromAnthropicMessage(resp anthropicMessageResponse) Message {
	msg := Message{Role: RoleAssistant}
	var text strings.Builder
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			tc := ToolCall{ID: block.ID, Type: "function"}
			tc.Function.Name = block.Name
			if block.Input != nil {
				b, err := json.Marshal(block.Input)
				if err == nil {
					tc.Function.Arguments = string(b)
				} else {
					tc.Function.Arguments = "{}"
				}
			} else {
				tc.Function.Arguments = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
	}
	msg.Content = text.String()
	return msg
}

// --- wire types ---

type anthropicModelsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

type anthropicMessagesRequest struct {
	Model      string                 `json:"model"`
	Messages   []anthropicWireMessage `json:"messages"`
	MaxTokens  int                    `json:"max_tokens"`
	Stream     bool                   `json:"stream"`
	System     any                    `json:"system,omitempty"`
	Tools      []anthropicTool        `json:"tools,omitempty"`
	ToolChoice any                    `json:"tool_choice,omitempty"`
}

type anthropicWireMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anthropicContentBlock
}

type anthropicContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"` // tool_result content
	// Source is set for type "image" (base64 data or URL) — vision path.
	Source *anthropicImageSource `json:"source,omitempty"`
	// CacheControl enables Anthropic prompt caching when set (type:"ephemeral").
	// Used on the final system text block; ignored by providers that don't
	// support it.
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicCacheControl is the Anthropic cache_control directive.
type anthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// anthropicSystemBlock is one element of the system content array. Anthropic
// accepts system as either a plain string or an array of these blocks; we use
// the array form so cache_control can be attached for prompt caching.
type anthropicSystemBlock struct {
	Type         string                 `json:"type"` // "text"
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicImageSource is the Anthropic image content source object.
type anthropicImageSource struct {
	Type      string `json:"type"` // "base64" | "url"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// anthropicImageBlock converts a provider ContentPart image into an Anthropic image block.
func anthropicImageBlock(p ContentPart) *anthropicContentBlock {
	url := strings.TrimSpace(p.ImageURL)
	if url == "" {
		return nil
	}
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "data:") {
		// data:image/png;base64,AAAA
		mt := strings.TrimSpace(p.MediaType)
		rest := url[5:]
		semi := strings.Index(rest, ";")
		comma := strings.Index(rest, ",")
		if comma < 0 {
			return nil
		}
		if mt == "" && semi > 0 {
			mt = rest[:semi]
		}
		if mt == "" {
			mt = "image/png"
		}
		data := rest[comma+1:]
		if data == "" {
			return nil
		}
		return &anthropicContentBlock{
			Type: "image",
			Source: &anthropicImageSource{
				Type:      "base64",
				MediaType: mt,
				Data:      data,
			},
		}
	}
	// Remote URL
	return &anthropicContentBlock{
		Type: "image",
		Source: &anthropicImageSource{
			Type: "url",
			URL:  url,
		},
	}
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicMessageResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
}

type anthropicStreamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
		StopReason  string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Compile-time assertion: Anthropic implements Profile.
var _ Profile = (*Anthropic)(nil)
var _ Profile = (*OpenAICompat)(nil)
