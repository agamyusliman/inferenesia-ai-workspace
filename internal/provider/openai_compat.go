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
	defaultHTTPTimeout = 15 * time.Second
	// Stream body can run longer than the dial timeout; use a long ResponseHeaderTimeout
	// but keep dial/TLS short so unreachable hosts fail under 30s (VAL-CLI-009).
	streamIdleTimeout = 5 * time.Minute
)

// OpenAICompat is an openai_compatible ChatClient (temp-ai, OpenAI, OpenRouter, Ollama, …).
type OpenAICompat struct {
	cfg    Config
	client *http.Client
}

// NewOpenAICompat builds a client from cfg. BaseURL must be non-empty for network ops.
func NewOpenAICompat(cfg Config) *OpenAICompat {
	if cfg.ID == "" {
		cfg.ID = "openai_compatible"
	}
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
	return &OpenAICompat{
		cfg: cfg,
		client: &http.Client{
			// Do not set Timeout on the client for streaming; cancel via context.
			Transport: transport,
		},
	}
}

// Name implements ChatClient.
func (c *OpenAICompat) Name() string {
	if c.cfg.ID != "" {
		return c.cfg.ID
	}
	return "openai_compatible"
}

// BaseURL returns the configured root (for tests/verbose).
func (c *OpenAICompat) BaseURL() string {
	return c.cfg.BaseURL
}

// RequestHost returns scheme://host for verbose/trace output (no path, no secrets).
func (c *OpenAICompat) RequestHost() string {
	return RequestHost(c.cfg.BaseURL)
}

// APIKeyConfigured reports whether a non-empty API key is set (never returns the key).
func (c *OpenAICompat) APIKeyConfigured() bool {
	return strings.TrimSpace(c.cfg.APIKey) != ""
}

// DefaultModel returns the configured optional model override.
func (c *OpenAICompat) DefaultModel() string {
	return strings.TrimSpace(c.cfg.DefaultModel)
}

// SetDefaultModel updates the preferred model (used after resolution).
func (c *OpenAICompat) SetDefaultModel(id string) {
	c.cfg.DefaultModel = strings.TrimSpace(id)
}

func (c *OpenAICompat) requireKey() error {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return ErrMissingAPIKey
	}
	return nil
}

func (c *OpenAICompat) requireBase() error {
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return &ConnectionError{BaseURL: "", Reason: "missing TEMP_AI_BASE_URL (or profile base_url)"}
	}
	return nil
}

func (c *OpenAICompat) endpoint(path string) string {
	base := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/")
	path = "/" + strings.TrimLeft(path, "/")
	return base + path
}

// ListModels implements ChatClient via GET {base}/models.
func (c *OpenAICompat) ListModels(ctx context.Context) ([]Model, error) {
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	if err := c.requireKey(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/models"), nil)
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

	var parsed modelsListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse models response: %w", err)
	}
	out := make([]Model, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		out = append(out, Model{ID: id, OwnedBy: m.OwnedBy})
	}
	if len(out) == 0 {
		return nil, ErrEmptyModelList
	}
	return out, nil
}

// ResolveModel picks the model id to use:
//  1. explicit non-empty preferred (req model / TEMP_AI_MODEL)
//  2. otherwise first model from ListModels
//
// When preferred is set, it is validated against the live list when possible;
// an unknown id yields ModelError (no silent fallback).
func (c *OpenAICompat) ResolveModel(ctx context.Context, preferred string) (string, error) {
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		preferred = strings.TrimSpace(c.cfg.DefaultModel)
	}

	models, err := c.ListModels(ctx)
	if err != nil {
		// If we already have a preferred and list fails for non-auth reasons,
		// still surface the list error so operators see discovery issues.
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
	return "", &ModelError{
		Model:  preferred,
		Reason: "not found in provider model list (set TEMP_AI_MODEL to a valid id from `inferenesia models`)",
		Cause:  ErrModelNotFound,
	}
}

// ChatStream implements ChatClient for /chat/completions with stream=true SSE.
// When req.Tools is non-empty, tool_call deltas are accumulated and emitted as EventToolCalls.
func (c *OpenAICompat) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	if err := c.requireKey(); err != nil {
		return nil, err
	}

	model, body, err := c.buildChatBody(ctx, req, true)
	if err != nil {
		return nil, err
	}
	_ = model

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, classifyTransport(c.cfg.BaseURL, err)
	}
	c.setAuth(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	// Use a transport copy with longer response-header timeout for streaming,
	// still dial-bounded for fail-fast.
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

// ChatComplete runs a non-streaming completion and returns the full assistant text.
// Tool calls in the response are not returned; use ChatTurn for tool loop support.
func (c *OpenAICompat) ChatComplete(ctx context.Context, req ChatRequest) (string, error) {
	msg, err := c.ChatTurn(ctx, req)
	if err != nil {
		return "", err
	}
	return msg.Content, nil
}

// ChatTurn runs a non-streaming completion and returns the assistant Message
// (including ToolCalls when finish_reason is tool_calls).
func (c *OpenAICompat) ChatTurn(ctx context.Context, req ChatRequest) (Message, error) {
	if err := c.requireBase(); err != nil {
		return Message{}, err
	}
	if err := c.requireKey(); err != nil {
		return Message{}, err
	}

	_, body, err := c.buildChatBody(ctx, req, false)
	if err != nil {
		return Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return Message{}, classifyTransport(c.cfg.BaseURL, err)
	}
	c.setAuth(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

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

	var parsed chatCompletionsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Message{}, fmt.Errorf("parse chat response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Message{}, fmt.Errorf("empty chat response: no choices")
	}
	choice := parsed.Choices[0]
	msg := Message{
		Role:    RoleAssistant,
		Content: choice.Message.Content,
	}
	if len(choice.Message.ToolCalls) > 0 {
		msg.ToolCalls = fromWireToolCalls(choice.Message.ToolCalls)
	}
	return msg, nil
}

func (c *OpenAICompat) buildChatBody(ctx context.Context, req ChatRequest, stream bool) (model string, body []byte, err error) {
	model = strings.TrimSpace(req.Model)
	if model == "" {
		model, err = c.ResolveModel(ctx, c.cfg.DefaultModel)
		if err != nil {
			return "", nil, err
		}
	}

	payload := chatCompletionsRequest{
		Model:    model,
		Messages: toWireMessages(req.Messages),
		Stream:   stream,
	}
	if len(req.Tools) > 0 {
		payload.Tools = toWireTools(req.Tools)
		// Prefer serial tool calls for simpler agent loop (architecture: parallel_tool_calls false ok).
		f := false
		payload.ParallelToolCalls = &f
		if strings.TrimSpace(req.ToolChoice) != "" {
			payload.ToolChoice = req.ToolChoice
		}
	}
	body, err = json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	return model, body, nil
}

func (c *OpenAICompat) streamHTTPClient() *http.Client {
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

func (c *OpenAICompat) setAuth(req *http.Request) {
	// Never log this header.
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
}

func (c *OpenAICompat) mapHTTPError(status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	// Never include raw body if it might echo the key; summarize only.
	snippet := sanitizeBodySnippet(body)
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		reason := "invalid or unauthorized API key"
		if snippet != "" {
			reason = snippet
		}
		return &AuthError{Reason: reason, Status: status, Cause: ErrUnauthorized}
	case http.StatusNotFound:
		// Could be bad path or bad model depending on endpoint.
		return fmt.Errorf("provider returned HTTP %d: %s", status, orDefault(snippet, "not found"))
	case http.StatusBadRequest:
		// Often invalid model — keep reason without secrets.
		return fmt.Errorf("provider returned HTTP %d: %s", status, orDefault(snippet, "bad request"))
	default:
		if status >= 500 {
			return fmt.Errorf("provider returned HTTP %d: %s", status, orDefault(snippet, "server error"))
		}
		return fmt.Errorf("provider returned HTTP %d: %s", status, orDefault(snippet, "request failed"))
	}
}

func (c *OpenAICompat) readSSE(ctx context.Context, resp *http.Response, ch chan<- StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()

	var assembled strings.Builder
	// Accumulate streamed tool_calls by index (OpenAI streams argument fragments).
	toolAcc := map[int]*ToolCall{}
	var lastFinish string

	scanner := bufio.NewScanner(resp.Body)
	// SSE chunks can be large for big models.
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
		if len(toolAcc) == 0 {
			return nil
		}
		// Stable order by index.
		maxIdx := -1
		for i := range toolAcc {
			if i > maxIdx {
				maxIdx = i
			}
		}
		out := make([]ToolCall, 0, len(toolAcc))
		for i := 0; i <= maxIdx; i++ {
			if tc, ok := toolAcc[i]; ok && tc != nil {
				if tc.Type == "" {
					tc.Type = "function"
				}
				out = append(out, *tc)
			}
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
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			final := assembled.String()
			calls := flushTools()
			if len(calls) > 0 {
				_ = send(StreamEvent{
					Type:         EventToolCalls,
					Final:        final,
					ToolCalls:    calls,
					FinishReason: orDefault(lastFinish, "tool_calls"),
				})
			}
			_ = send(StreamEvent{
				Type:         EventDone,
				Final:        final,
				ToolCalls:    calls,
				FinishReason: lastFinish,
			})
			return
		}
		var chunk sseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Skip malformed partial lines.
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			_ = send(StreamEvent{
				Type: EventError,
				Err:  fmt.Errorf("provider stream error: %s", chunk.Error.Message),
			})
			return
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				lastFinish = *choice.FinishReason
			}
			if choice.Delta.Content != "" {
				assembled.WriteString(choice.Delta.Content)
				if !send(StreamEvent{Type: EventDelta, Delta: choice.Delta.Content}) {
					return
				}
			}
			for _, td := range choice.Delta.ToolCalls {
				idx := td.Index
				acc, ok := toolAcc[idx]
				if !ok || acc == nil {
					acc = &ToolCall{Type: "function"}
					toolAcc[idx] = acc
				}
				if td.ID != "" {
					acc.ID = td.ID
				}
				if td.Type != "" {
					acc.Type = td.Type
				}
				if td.Function.Name != "" {
					acc.Function.Name += td.Function.Name
				}
				if td.Function.Arguments != "" {
					acc.Function.Arguments += td.Function.Arguments
				}
			}
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
	// Stream ended without [DONE] — still emit assembled final / tools.
	final := assembled.String()
	calls := flushTools()
	if len(calls) > 0 {
		_ = send(StreamEvent{
			Type:         EventToolCalls,
			Final:        final,
			ToolCalls:    calls,
			FinishReason: orDefault(lastFinish, "tool_calls"),
		})
	}
	_ = send(StreamEvent{
		Type:         EventDone,
		Final:        final,
		ToolCalls:    calls,
		FinishReason: lastFinish,
	})
}

// --- wire types ---

type modelsListResponse struct {
	Data []struct {
		ID      string `json:"id"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

type wireToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// wireContentPart is one OpenAI multimodal content array entry (text | image_url).
type wireContentPart struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *wireImageURLPart `json:"image_url,omitempty"`
}

type wireImageURLPart struct {
	URL string `json:"url"`
}

type wireMessage struct {
	Role string `json:"role"`
	// Content is either a string (text-only) or []wireContentPart for vision.
	// json.Marshal handles both via any.
	Content    any            `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type wireToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatCompletionsRequest struct {
	Model             string         `json:"model"`
	Messages          []wireMessage  `json:"messages"`
	Stream            bool           `json:"stream"`
	Tools             []wireToolDef  `json:"tools,omitempty"`
	ToolChoice        any            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role      string         `json:"role"`
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func toWireMessages(msgs []Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		// Multimodal user parts (text + image_url data URLs) for vision models (VAL-CHAT-022).
		// Providers without vision may skip image parts server-side; we still send the array.
		if len(m.Parts) > 0 {
			parts := make([]wireContentPart, 0, len(m.Parts))
			for _, p := range m.Parts {
				switch strings.ToLower(strings.TrimSpace(p.Type)) {
				case "image_url", "image":
					url := strings.TrimSpace(p.ImageURL)
					if url == "" {
						continue
					}
					parts = append(parts, wireContentPart{
						Type:     "image_url",
						ImageURL: &wireImageURLPart{URL: url},
					})
				default:
					// text (or empty type with Text set)
					txt := p.Text
					if txt == "" && p.Type == "" {
						continue
					}
					parts = append(parts, wireContentPart{Type: "text", Text: txt})
				}
			}
			if len(parts) > 0 {
				wm.Content = parts
			}
		}
		if len(m.ToolCalls) > 0 {
			wm.ToolCalls = toWireToolCalls(m.ToolCalls)
		}
		// Some gateways reject null content on assistant tool-call messages;
		// empty string is fine.
		out = append(out, wm)
	}
	return out
}

func toWireToolCalls(calls []ToolCall) []wireToolCall {
	out := make([]wireToolCall, 0, len(calls))
	for _, c := range calls {
		w := wireToolCall{ID: c.ID, Type: c.Type}
		if w.Type == "" {
			w.Type = "function"
		}
		w.Function.Name = c.Function.Name
		w.Function.Arguments = c.Function.Arguments
		out = append(out, w)
	}
	return out
}

func fromWireToolCalls(calls []wireToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(calls))
	for _, c := range calls {
		tc := ToolCall{ID: c.ID, Type: c.Type}
		if tc.Type == "" {
			tc.Type = "function"
		}
		tc.Function.Name = c.Function.Name
		tc.Function.Arguments = c.Function.Arguments
		out = append(out, tc)
	}
	return out
}

func toWireTools(defs []ToolDef) []wireToolDef {
	out := make([]wireToolDef, 0, len(defs))
	for _, d := range defs {
		w := wireToolDef{Type: d.Type}
		if w.Type == "" {
			w.Type = "function"
		}
		w.Function.Name = d.Function.Name
		w.Function.Description = d.Function.Description
		w.Function.Parameters = d.Function.Parameters
		out = append(out, w)
	}
	return out
}

func sanitizeBodySnippet(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	// Try OpenAI-style error envelope: {"error":{"message":"..."}} or {"error":"..."}.
	var env struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		if len(env.Error) > 0 {
			// Object form.
			var obj struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			}
			if json.Unmarshal(env.Error, &obj) == nil && obj.Message != "" {
				return truncate(stripSecrets(obj.Message), 200)
			}
			// String form: "error": "invalid key"
			var s string
			if json.Unmarshal(env.Error, &s) == nil && s != "" {
				return truncate(stripSecrets(s), 200)
			}
		}
		if env.Message != "" {
			return truncate(stripSecrets(env.Message), 200)
		}
	}
	s := strings.TrimSpace(string(body))
	s = stripSecrets(s)
	// Prefer human text without raw JSON braces when possible.
	if strings.HasPrefix(s, "{") {
		return "request rejected by provider"
	}
	return truncate(s, 200)
}

func stripSecrets(s string) string {
	// Defensive: drop bearer-looking tokens if error JSON echoed them.
	if i := strings.Index(strings.ToLower(s), "bearer "); i >= 0 {
		return s[:i] + "bearer [REDACTED]"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// AssembleFinal drains a stream channel and returns the concatenated final answer.
// On EventDone with Final set, that value wins; otherwise deltas are concatenated.
func AssembleFinal(ch <-chan StreamEvent) (string, error) {
	var b strings.Builder
	var final string
	for ev := range ch {
		switch ev.Type {
		case EventDelta:
			b.WriteString(ev.Delta)
		case EventDone:
			if ev.Final != "" {
				final = ev.Final
			} else if b.Len() > 0 {
				final = b.String()
			}
		case EventError:
			if ev.Err != nil {
				return b.String(), ev.Err
			}
			return b.String(), fmt.Errorf("stream error")
		}
	}
	if final != "" {
		return final, nil
	}
	return b.String(), nil
}
