package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// httpConn is a remote HTTP JSON-RPC MCP session (VAL-MCP-002).
// Headers and Authorization are never written to logs.
type httpConn struct {
	name    string
	cfg     ServerConfig
	client  *http.Client
	baseURL string
	headers map[string]string // resolved at connect; includes auth
	tools   []ToolSchema
	nextID  atomic.Int64
	mu      sync.Mutex
	closed  bool
}

func startHTTP(ctx context.Context, cfg ServerConfig, info ClientInfo) (*httpConn, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = strings.TrimSpace(cfg.URL)
	}
	if base == "" {
		return nil, fmt.Errorf("empty base_url")
	}

	headers := map[string]string{}
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		headers[k] = v
	}
	// Optional structured auth → Authorization (or custom header). Values not logged.
	if cfg.Auth != nil {
		token := strings.TrimSpace(cfg.Auth.Token)
		if token == "" && strings.TrimSpace(cfg.Auth.TokenEnv) != "" {
			token = strings.TrimSpace(os.Getenv(cfg.Auth.TokenEnv))
		}
		if token != "" {
			hdr := strings.TrimSpace(cfg.Auth.Header)
			if hdr == "" {
				hdr = "Authorization"
			}
			t := strings.ToLower(strings.TrimSpace(cfg.Auth.Type))
			if t == "" || t == "bearer" {
				if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
					token = "Bearer " + token
				}
			}
			headers[hdr] = token
		}
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}
	if _, ok := headers["Accept"]; !ok {
		headers["Accept"] = "application/json, text/event-stream"
	}

	c := &httpConn{
		name:    cfg.Name,
		cfg:     cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: base,
		headers: headers,
	}

	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tools, err := c.handshake(initCtx, info)
	if err != nil {
		return nil, err
	}
	c.tools = tools
	return c, nil
}

func (c *httpConn) Tools() []ToolSchema {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ToolSchema(nil), c.tools...)
}

func (c *httpConn) Call(ctx context.Context, tool string, args any) (string, bool, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return "", true, fmt.Errorf("%s", FormatUnavailable(c.name, "client closed"))
	}
	raw, err := c.rpc(ctx, "tools/call", callToolParams{Name: tool, Arguments: args})
	if err != nil {
		return "", true, err
	}
	var res callToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return string(raw), false, nil
	}
	var b strings.Builder
	for i, blk := range res.Content {
		if blk.Type == "" || blk.Type == "text" {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(blk.Text)
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		enc, _ := json.Marshal(res.StructuredContent)
		b.Write(enc)
	}
	return b.String(), res.IsError, nil
}

func (c *httpConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	// Drop header values from memory best-effort.
	for k := range c.headers {
		c.headers[k] = ""
	}
	return nil
}

func (c *httpConn) handshake(ctx context.Context, info ClientInfo) ([]ToolSchema, error) {
	if info.Name == "" {
		info = DefaultClientInfo
	}
	params := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    info.Name,
			"version": info.Version,
		},
	}
	if _, err := c.rpc(ctx, "initialize", params); err != nil {
		return nil, err
	}
	// notifications/initialized — ignore response errors for notify-only.
	_ = c.rpcNotify(ctx, "notifications/initialized", map[string]any{})
	raw, err := c.rpc(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var list listToolsResult
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, "tools/list decode: "+err.Error()))
	}
	return list.Tools, nil
}

func (c *httpConn) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	reqBody := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, "request build: "+err.Error()))
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		// Never attach headers to the error string.
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, "unreachable: "+sanitizeNetErr(err)))
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, "read body: "+err.Error()))
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, fmt.Sprintf("auth %d", resp.StatusCode)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not include response body if it might echo Authorization; keep short.
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, fmt.Sprintf("http %d", resp.StatusCode)))
	}

	// Accept either plain JSON-RPC object or SSE "data:" frames with a JSON body.
	raw := bytes.TrimSpace(respBody)
	if bytes.HasPrefix(raw, []byte("event:")) || bytes.Contains(raw, []byte("\ndata:")) {
		raw = extractSSEData(raw)
	}
	var rpcResp rpcResponse
	if err := json.Unmarshal(raw, &rpcResp); err != nil {
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, "bad json: "+err.Error()))
	}
	if rpcResp.Error != nil {
		msg := rpcResp.Error.Message
		// Strip any secret-looking substrings from remote error messages.
		msg = stripSecretFragments(msg)
		return nil, fmt.Errorf("%s", FormatUnavailable(c.name, msg))
	}
	return rpcResp.Result, nil
}

func (c *httpConn) rpcNotify(ctx context.Context, method string, params any) error {
	reqBody := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}

func extractSSEData(b []byte) []byte {
	// Concatenate data: lines (last event wins for simple fixtures).
	var last []byte
	for _, line := range bytes.Split(b, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data:")) {
			last = bytes.TrimSpace(line[len("data:"):])
		}
	}
	if len(last) > 0 {
		return last
	}
	return b
}

func sanitizeNetErr(err error) string {
	if err == nil {
		return "unknown"
	}
	s := err.Error()
	// Common net errors — keep host:port diagnostics, drop query strings that might hold tokens.
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[:i]
	}
	s = stripSecretFragments(s)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func stripSecretFragments(s string) string {
	// Defensive: if an Authorization value somehow leaked into an error, redact it.
	lower := strings.ToLower(s)
	if i := strings.Index(lower, "bearer "); i >= 0 {
		// redaction from bearer token onward to next whitespace/end
		end := i + len("bearer ")
		for end < len(s) && s[end] != ' ' && s[end] != '"' && s[end] != '\'' {
			end++
		}
		s = s[:i] + "bearer [REDACTED]" + s[end:]
	}
	if i := strings.Index(lower, "authorization:"); i >= 0 {
		// Replace the rest of the line-ish segment.
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != ',' && s[end] != ';' {
			end++
		}
		// Skip value after colon
		for end < len(s) && (s[end] == ' ' || s[end] == ':') {
			end++
		}
		valEnd := end
		for valEnd < len(s) && s[valEnd] != ' ' && s[valEnd] != '"' {
			valEnd++
		}
		if valEnd > end {
			s = s[:end] + "[REDACTED]" + s[valEnd:]
		}
	}
	return s
}

// SafeHeadersForLog returns header *names* only (never values) for diagnostics.
func SafeHeadersForLog(headers map[string]string) []string {
	if len(headers) == 0 {
		return nil
	}
	out := make([]string, 0, len(headers))
	for k := range headers {
		out = append(out, k)
	}
	return out
}
