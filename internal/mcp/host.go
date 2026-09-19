package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
)

// conn is the internal server connection interface.
type conn interface {
	Tools() []ToolSchema
	Call(ctx context.Context, tool string, args any) (string, bool, error)
	Close() error
}

// ServerStatus describes one registered server (for CLI/UI diagnostics).
type ServerStatus struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	OK        bool     `json:"ok"`
	Error     string   `json:"error,omitempty"`
	ToolNames []string `json:"tool_names,omitempty"` // raw remote names (not namespaced)
}

// Host manages multiple MCP servers and exposes namespaced tools to the agent (VAL-MCP-001..005).
// Failures on one server never crash the host or the agent loop (VAL-MCP-003).
type Host struct {
	mu      sync.RWMutex
	servers map[string]*serverEntry
	info    ClientInfo
	// Logf receives clear unavailable messages. Defaults to log.Printf.
	// Implementations MUST NOT log Authorization/header values.
	Logf func(format string, args ...any)
}

type serverEntry struct {
	cfg   ServerConfig
	conn  conn
	err   string // last failure reason (formatted with FormatUnavailable when set)
	tools []ToolSchema
}

// NewHost builds an empty MCP host.
func NewHost(info ClientInfo) *Host {
	if info.Name == "" {
		info = DefaultClientInfo
	}
	return &Host{
		servers: make(map[string]*serverEntry),
		info:    info,
		Logf:    log.Printf,
	}
}

func (h *Host) logf(format string, args ...any) {
	if h != nil && h.Logf != nil {
		h.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// Register validates cfg, connects (stdio or http), lists tools, and stores them.
// On connection failure it records a clear unavailable error and returns that error
// without panicking — callers can continue registering other servers (VAL-MCP-003).
func (h *Host) Register(ctx context.Context, cfg ServerConfig) error {
	if h == nil {
		return fmt.Errorf("mcp: host is nil")
	}
	norm, err := ValidateServer(cfg)
	if err != nil {
		return err
	}
	if !norm.IsEnabled() {
		return nil
	}

	var c conn
	var tools []ToolSchema
	var connectErr error

	switch norm.Type {
	case TypeStdio:
		sc, err := startStdio(ctx, norm, h.info)
		if err != nil {
			connectErr = err
		} else {
			c = sc
			tools = sc.Tools()
		}
	case TypeHTTP:
		hc, err := startHTTP(ctx, norm, h.info)
		if err != nil {
			connectErr = err
		} else {
			c = hc
			tools = hc.Tools()
		}
	default:
		return fmt.Errorf("mcp: server %q: unsupported type %q", norm.Name, norm.Type)
	}

	h.mu.Lock()
	// Replace any previous registration with the same name.
	if prev, ok := h.servers[norm.Name]; ok && prev.conn != nil {
		_ = prev.conn.Close()
	}
	entry := &serverEntry{cfg: norm}
	if connectErr != nil {
		msg := connectErr.Error()
		// Ensure canonical format when missing.
		if !strings.Contains(msg, "unavailable:") {
			msg = FormatUnavailable(norm.Name, msg)
		}
		entry.err = msg
		h.servers[norm.Name] = entry
		h.mu.Unlock()
		h.logf("%s", msg)
		return connectErr
	}
	entry.conn = c
	entry.tools = tools
	h.servers[norm.Name] = entry
	h.mu.Unlock()
	return nil
}

// RegisterAll registers many servers. Failures are isolated; returns the first
// error while still attempting the rest. Agent loop continues either way.
func (h *Host) RegisterAll(ctx context.Context, servers map[string]ServerConfig) error {
	norm, err := ValidateServers(servers)
	if err != nil {
		// Schema rejection (e.g. type:rag) fails hard before any connect.
		return err
	}
	var first error
	// Stable-ish: name order does not matter; isolation is the invariant.
	for name, cfg := range norm {
		cfg.Name = name
		if err := h.Register(ctx, cfg); err != nil {
			if first == nil {
				first = err
			}
			// continue — isolation VAL-MCP-003
		}
	}
	return first
}

// Stop is an alias for Unregister: terminates the stdio process / closes the
// HTTP client and drops the server's tools (VAL-MCP-006).
func (h *Host) Stop(name string) error {
	return h.Unregister(name)
}

// Unregister stops a server and removes its tools (VAL-MCP-006).
// For stdio, the child process is signaled and killed; for HTTP the client is closed.
// Subsequent Host.Call for those tools returns a clear "unknown tool" (does not hang).
func (h *Host) Unregister(name string) error {
	if h == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	h.mu.Lock()
	entry, ok := h.servers[name]
	if ok {
		delete(h.servers, name)
	}
	h.mu.Unlock()
	if !ok {
		return nil
	}
	if entry.conn != nil {
		return entry.conn.Close()
	}
	return nil
}

// ServerPID returns the OS process id of a stdio MCP server (0 if not running /
// not stdio / unregistered). Used by lifecycle tests and diagnostics (VAL-MCP-006).
func (h *Host) ServerPID(name string) int {
	if h == nil {
		return 0
	}
	name = strings.TrimSpace(name)
	h.mu.RLock()
	defer h.mu.RUnlock()
	e, ok := h.servers[name]
	if !ok || e.conn == nil {
		return 0
	}
	type pidder interface{ Pid() int }
	if p, ok := e.conn.(pidder); ok {
		return p.Pid()
	}
	return 0
}

// Close shuts down all servers.
func (h *Host) Close() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	entries := make([]*serverEntry, 0, len(h.servers))
	for _, e := range h.servers {
		entries = append(entries, e)
	}
	h.servers = make(map[string]*serverEntry)
	h.mu.Unlock()
	var first error
	for _, e := range entries {
		if e.conn != nil {
			if err := e.conn.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// Status returns per-server status (no secrets).
func (h *Host) Status() []ServerStatus {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]ServerStatus, 0, len(h.servers))
	for name, e := range h.servers {
		st := ServerStatus{
			Name:  name,
			Type:  e.cfg.Type,
			OK:    e.err == "" && e.conn != nil,
			Error: e.err,
		}
		for _, t := range e.tools {
			st.ToolNames = append(st.ToolNames, t.Name)
		}
		out = append(out, st)
	}
	return out
}

// toolDef is a simplified view for tools.Registry wiring (avoids import cycle).
type toolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ListToolDefs returns agent-facing namespaced tool definitions (VAL-MCP-005).
// Only tools from healthy servers are included; failed servers are omitted
// (errors already emitted at Register time).
func (h *Host) ListToolDefs() []toolDef {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []toolDef
	for name, e := range h.servers {
		if e.err != "" || e.conn == nil {
			continue
		}
		for _, t := range e.tools {
			params := t.InputSchema
			if len(params) == 0 {
				params = DefaultInputSchema
			}
			ns := NamespaceTool(name, t.Name)
			desc := t.Description
			if desc == "" {
				desc = fmt.Sprintf("MCP tool %s from server %s", t.Name, name)
			} else {
				desc = fmt.Sprintf("[MCP:%s] %s", name, desc)
			}
			out = append(out, toolDef{
				Name:        ns,
				Description: desc,
				Parameters:  params,
			})
		}
	}
	return out
}

// HasTool reports whether a namespaced tool is currently registered and healthy.
func (h *Host) HasTool(namespaced string) bool {
	server, tool, ok := ParseNamespaced(namespaced)
	if !ok {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	e, ok := h.servers[server]
	if !ok || e.err != "" || e.conn == nil {
		return false
	}
	for _, t := range e.tools {
		if t.Name == tool {
			return true
		}
	}
	return false
}

// Call invokes a namespaced MCP tool. Failures return clear errors; never panics.
// After Stop/Unregister, calls return "unknown tool …" promptly (VAL-MCP-006).
// Soft-failed but still-listed servers keep the clear "unavailable" shape (VAL-MCP-003).
func (h *Host) Call(ctx context.Context, namespaced string, argumentsJSON string) (content string, isError bool) {
	if h == nil {
		return "mcp: host not configured", true
	}
	server, tool, ok := ParseNamespaced(namespaced)
	if !ok {
		return fmt.Sprintf("unknown tool %q", namespaced), true
	}

	h.mu.RLock()
	e, exists := h.servers[server]
	h.mu.RUnlock()
	if !exists {
		// Lifecycle: removed server → clear unknown tool (not hang) (VAL-MCP-006).
		return fmt.Sprintf("unknown tool %q", namespaced), true
	}
	if e.err != "" || e.conn == nil {
		// Soft-failed server still listed: clear unavailable (VAL-MCP-003).
		msg := e.err
		if msg == "" {
			msg = FormatUnavailable(server, "unavailable")
		}
		return msg, true
	}

	// Tool not advertised on this healthy server → unknown tool.
	known := false
	for _, t := range e.tools {
		if t.Name == tool {
			known = true
			break
		}
	}
	if !known {
		return fmt.Sprintf("unknown tool %q", namespaced), true
	}

	args, err := ParseToolArguments(argumentsJSON)
	if err != nil {
		return err.Error(), true
	}

	text, toolErr, callErr := e.conn.Call(ctx, tool, args)
	if callErr != nil {
		// Mark server soft-failed if the connection died, but keep entry for diagnostics.
		msg := callErr.Error()
		if !strings.Contains(msg, "unavailable:") {
			msg = FormatUnavailable(server, msg)
		}
		h.logf("%s", msg)
		return msg, true
	}
	if text == "" && !toolErr {
		text = "(empty MCP tool result)"
	}
	return text, toolErr
}

// NamespacedNames returns all healthy namespaced tool names.
func (h *Host) NamespacedNames() []string {
	defs := h.ListToolDefs()
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}
