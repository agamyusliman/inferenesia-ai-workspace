package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ProtocolVersion is the MCP initialize protocolVersion we advertise.
const ProtocolVersion = "2024-11-05"

// jsonRPC wire types (subset for tools/list + tools/call).

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return "rpc error"
	}
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// ToolSchema is one MCP tool from tools/list.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type listToolsResult struct {
	Tools []ToolSchema `json:"tools"`
}

type callToolParams struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments,omitempty"`
}

type callToolResult struct {
	Content           []contentBlock `json:"content"`
	IsError           bool           `json:"isError,omitempty"`
	StructuredContent any            `json:"structuredContent,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ClientInfo identifies this host to MCP servers.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DefaultClientInfo is used when Host does not override it.
var DefaultClientInfo = ClientInfo{Name: "inferenesia", Version: "0.1.0"}

// session is a bidirectional JSON-RPC session over a framed reader/writer.
type session struct {
	w      io.Writer
	r      *bufio.Reader
	mu     sync.Mutex // write lock
	nextID atomic.Int64
	// pending maps stringified id → response channel
	pendingMu sync.Mutex
	pending   map[string]chan rpcResponse
	// closed when read loop ends
	done   chan struct{}
	errMu  sync.Mutex
	closed error
}

func newSession(r io.Reader, w io.Writer) *session {
	s := &session{
		w:       w,
		r:       bufio.NewReader(r),
		pending: make(map[string]chan rpcResponse),
		done:    make(chan struct{}),
	}
	go s.readLoop()
	return s
}

func (s *session) Close() {
	s.errMu.Lock()
	if s.closed == nil {
		s.closed = fmt.Errorf("session closed")
	}
	s.errMu.Unlock()
	s.pendingMu.Lock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.pendingMu.Unlock()
}

func (s *session) setClosed(err error) {
	s.errMu.Lock()
	if s.closed == nil {
		s.closed = err
	}
	s.errMu.Unlock()
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	s.pendingMu.Lock()
	for id, ch := range s.pending {
		// non-blocking best effort
		select {
		case ch <- rpcResponse{Error: &rpcError{Code: -32000, Message: err.Error()}}:
		default:
		}
		close(ch)
		delete(s.pending, id)
	}
	s.pendingMu.Unlock()
}

func (s *session) readLoop() {
	for {
		msg, err := readFramed(s.r)
		if err != nil {
			s.setClosed(err)
			return
		}
		var resp rpcResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			// ignore non-response traffic (notifications from server)
			continue
		}
		if resp.ID == nil && resp.Error == nil && len(resp.Result) == 0 {
			continue // notification
		}
		// Prefer responses with id.
		if resp.ID == nil {
			continue
		}
		key := idKey(resp.ID)
		s.pendingMu.Lock()
		ch, ok := s.pending[key]
		if ok {
			delete(s.pending, key)
		}
		s.pendingMu.Unlock()
		if ok {
			ch <- resp
			close(ch)
		}
	}
}

func (s *session) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	key := idKey(id)
	ch := make(chan rpcResponse, 1)
	s.pendingMu.Lock()
	s.pending[key] = ch
	s.pendingMu.Unlock()

	if err := s.writeMsg(req); err != nil {
		s.pendingMu.Lock()
		delete(s.pending, key)
		s.pendingMu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		s.pendingMu.Lock()
		delete(s.pending, key)
		s.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-s.done:
		return nil, fmt.Errorf("mcp session closed")
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("mcp session closed")
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (s *session) notify(method string, params any) error {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return s.writeMsg(req)
}

func (s *session) writeMsg(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeFramed(s.w, body)
}

// Initialize performs MCP handshake + tools/list.
func (s *session) Initialize(ctx context.Context, info ClientInfo) ([]ToolSchema, error) {
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
	if _, err := s.call(ctx, "initialize", params); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	// notifications/initialized has no id
	if err := s.notify("notifications/initialized", map[string]any{}); err != nil {
		return nil, fmt.Errorf("initialized notify: %w", err)
	}
	raw, err := s.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	var list listToolsResult
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("tools/list decode: %w", err)
	}
	return list.Tools, nil
}

// CallTool invokes tools/call and returns text content.
func (s *session) CallTool(ctx context.Context, name string, arguments any) (string, bool, error) {
	raw, err := s.call(ctx, "tools/call", callToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return "", true, err
	}
	var res callToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		// Some servers return plain text result
		return string(raw), false, nil
	}
	var b strings.Builder
	for i, c := range res.Content {
		if c.Type == "" || c.Type == "text" {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(c.Text)
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		enc, _ := json.Marshal(res.StructuredContent)
		b.Write(enc)
	}
	return b.String(), res.IsError, nil
}

func idKey(id any) string {
	switch v := id.(type) {
	case string:
		return "s:" + v
	case float64:
		return "n:" + strconv.FormatInt(int64(v), 10)
	case int64:
		return "n:" + strconv.FormatInt(v, 10)
	case int:
		return "n:" + strconv.Itoa(v)
	case json.Number:
		return "n:" + v.String()
	default:
		return fmt.Sprintf("x:%v", id)
	}
}

// writeFramed writes a Content-Length framed message (LSP/MCP stdio style).
func writeFramed(w io.Writer, body []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// readFramed reads one Content-Length framed message, or a single NDJSON line as fallback.
func readFramed(r *bufio.Reader) ([]byte, error) {
	// Peek: either starts with Content-Length or with '{' (NDJSON).
	b, err := r.Peek(1)
	if err != nil {
		return nil, err
	}
	if b[0] == '{' {
		line, err := r.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			return nil, err
		}
		return bytes.TrimSpace(line), nil
	}

	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "content-length:") {
			n, err := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			if err != nil {
				// try case-insensitive split
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					return nil, fmt.Errorf("bad Content-Length: %q", line)
				}
				n, err = strconv.Atoi(strings.TrimSpace(parts[1]))
				if err != nil {
					return nil, fmt.Errorf("bad Content-Length: %q", line)
				}
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	if contentLength > 16<<20 {
		return nil, fmt.Errorf("message too large: %d", contentLength)
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// ParseToolArguments turns a model JSON arguments string into a generic value for MCP.
func ParseToolArguments(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("invalid tool arguments JSON: %w", err)
	}
	return v, nil
}

// DefaultInputSchema is used when a server omits inputSchema.
var DefaultInputSchema = json.RawMessage(`{"type":"object","properties":{}}`)
