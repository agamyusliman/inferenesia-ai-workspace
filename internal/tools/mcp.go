package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
)

// MCPHostBackend is the subset of *mcp.Host used by the tool registry
// so agents receive namespaced MCP tools (VAL-MCP-001/005/006).
type MCPHostBackend interface {
	// ListNamespacedTools returns agent-facing tool schemas.
	ListNamespacedTools() []MCPToolDef
	// CallNamespaced invokes mcp.<server>.<tool> with raw JSON arguments.
	CallNamespaced(ctx context.Context, namespacedName, argumentsJSON string) (content string, isError bool)
	// HasNamespacedTool reports whether the tool is currently available.
	HasNamespacedTool(namespacedName string) bool
}

// MCPStoppable is implemented by backends that support lifecycle stop (VAL-MCP-006).
type MCPStoppable interface {
	StopServer(name string) error
}

// MCPToolDef is one OpenAI-compatible MCP tool entry for Definitions().
type MCPToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// HostBackend adapts *mcp.Host to MCPHostBackend (avoids import cycles in tests that mock).
type HostBackend struct {
	Host *mcp.Host
}

// NewHostBackend wraps a host for tools.Registry.SetMCPHost.
func NewHostBackend(h *mcp.Host) *HostBackend {
	return &HostBackend{Host: h}
}

// ListNamespacedTools implements MCPHostBackend.
func (b *HostBackend) ListNamespacedTools() []MCPToolDef {
	if b == nil || b.Host == nil {
		return nil
	}
	// Use public Status + Call surface; rebuild from host NamespacedNames + List via Status.
	// Host exports ListToolDefs through a thin public method on HostBackend using status+schemas.
	return listFromHost(b.Host)
}

// CallNamespaced implements MCPHostBackend.
func (b *HostBackend) CallNamespaced(ctx context.Context, namespacedName, argumentsJSON string) (string, bool) {
	if b == nil || b.Host == nil {
		return "mcp: host not configured", true
	}
	return b.Host.Call(ctx, namespacedName, argumentsJSON)
}

// HasNamespacedTool implements MCPHostBackend.
func (b *HostBackend) HasNamespacedTool(namespacedName string) bool {
	if b == nil || b.Host == nil {
		return false
	}
	return b.Host.HasTool(namespacedName)
}

// StopServer implements MCPHostBackend (VAL-MCP-006).
func (b *HostBackend) StopServer(name string) error {
	if b == nil || b.Host == nil {
		return nil
	}
	return b.Host.Stop(name)
}

// StopMCPServer stops a named MCP server on the registry's host and drops its tools.
// Subsequent Execute calls for that server's namespaced tools return "unknown tool" (VAL-MCP-006).
func StopMCPServer(reg *Registry, name string) error {
	if reg == nil || reg.mcp == nil {
		return nil
	}
	if s, ok := reg.mcp.(MCPStoppable); ok {
		return s.StopServer(name)
	}
	return nil
}

// listFromHost / toolDefsFromHost pull definitions from the host public API.
func listFromHost(h *mcp.Host) []MCPToolDef {
	return toolDefsFromHost(h)
}

func toolDefsFromHost(h *mcp.Host) []MCPToolDef {
	if h == nil {
		return nil
	}
	raw := h.ToolDefinitions()
	out := make([]MCPToolDef, 0, len(raw))
	for _, d := range raw {
		out = append(out, MCPToolDef{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters,
		})
	}
	return out
}

// SetMCPHost wires MCP namespaced tools into the registry.
// Pass nil to clear. Accepts *mcp.Host or any MCPHostBackend.
func (r *Registry) SetMCPHost(h any) {
	if r == nil {
		return
	}
	switch v := h.(type) {
	case nil:
		r.mcp = nil
	case *mcp.Host:
		// Prefer concrete host before interface — *HostBackend also implements MCPHostBackend.
		r.mcp = NewHostBackend(v)
	case MCPHostBackend:
		r.mcp = v
	default:
		// ignore unknown types
	}
}

// MCPHost returns the attached MCP backend (may be nil).
func (r *Registry) MCPHost() MCPHostBackend {
	if r == nil {
		return nil
	}
	return r.mcp
}

func mcpDefs(r *Registry) []Definition {
	if r == nil || r.mcp == nil {
		return nil
	}
	var defs []Definition
	for _, t := range r.mcp.ListNamespacedTools() {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return defs
}

func (r *Registry) execMCP(ctx context.Context, call Call) Result {
	if r == nil || r.mcp == nil {
		return Result{
			Name:    call.Name,
			Content: "mcp: host not configured",
			IsError: true,
		}
	}
	content, isErr := r.mcp.CallNamespaced(ctx, call.Name, call.Arguments)
	return Result{
		Name:    call.Name,
		Content: content,
		IsError: isErr,
	}
}

// SkillMCPHostRegistrar adapts an *mcp.Host (wired into the registry via
// SetMCPHost) to the SkillMCPRegistrar interface so execSkill load/unload can
// register and unregister MCP servers that a skill declares in its frontmatter
// (VAL-CROSS-012).
//
// The adapter translates skills.MCPServerSpec → mcp.ServerConfig (the skills
// package cannot import internal/mcp without a cycle), qualifies server names
// as "<skill-name>:<server>" so two skills cannot collide, and tracks which
// qualified server names belong to which skill so unload removes only those.
type SkillMCPHostRegistrar struct {
	Host *mcp.Host
	// registerTimeout bounds the MCP initialize+tools/list phase per server.
	// Default 20s when zero (matches AttachMCP).
	RegisterTimeout time.Duration
	// Logf receives clear unavailable messages (never Authorization values).
	Logf func(format string, args ...any)

	mu      sync.Mutex
	bySkill map[string][]string // skillName (lower) → qualified server names
}

// NewSkillMCPHostRegistrar wraps an *mcp.Host for skill-declared MCP servers.
// Pass the same host that was wired via SetMCPHost so namespaced tools flow
// through the same backend the agent loop already uses.
func NewSkillMCPHostRegistrar(h *mcp.Host) *SkillMCPHostRegistrar {
	return &SkillMCPHostRegistrar{Host: h, RegisterTimeout: 20 * time.Second}
}

// RegisterSkillMCP registers every spec on the host with a qualified name
// (VAL-CROSS-012). Failures for individual servers are isolated: the host
// keeps already-registered servers and the agent loop continues. Returns the
// list of fully-qualified server names that were registered (for unload).
func (s *SkillMCPHostRegistrar) RegisterSkillMCP(ctx context.Context, skillName string, specs []skills.MCPServerSpec) ([]string, error) {
	if s == nil || s.Host == nil {
		return nil, fmt.Errorf("skill mcp: host not configured")
	}
	skillName = strings.ToLower(strings.TrimSpace(skillName))
	if skillName == "" {
		return nil, fmt.Errorf("skill mcp: skill name is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := s.RegisterTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	regCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var registered []string
	var firstErr error
	for _, spec := range specs {
		spec.Name = strings.TrimSpace(spec.Name)
		if spec.Name == "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("skill mcp: server name is required")
			}
			continue
		}
		normalizedType, verr := skills.ValidateMCPServerSpec(spec)
		if verr != nil {
			if firstErr == nil {
				firstErr = verr
			}
			if s.Logf != nil {
				s.Logf("skill mcp: %s", verr.Error())
			}
			continue
		}
		cfg := mcp.ServerConfig{
			Name:        skills.QualifiedMCPServerName(skillName, spec.Name),
			Type:        normalizedType,
			Command:     strings.TrimSpace(spec.Command),
			Args:        append([]string(nil), spec.Args...),
			CommandList: append([]string(nil), spec.CommandList...),
			BaseURL:     strings.TrimSpace(spec.BaseURL),
			Env:         append([]string(nil), spec.Env...),
		}
		if err := s.Host.Register(regCtx, cfg); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if s.Logf != nil {
				s.Logf("skill mcp: register %q: %v", cfg.Name, err)
			}
			// Continue — isolation: other servers in the same skill still register.
			continue
		}
		registered = append(registered, cfg.Name)
	}
	s.mu.Lock()
	if s.bySkill == nil {
		s.bySkill = map[string][]string{}
	}
	// Merge (idempotent if the same skill is loaded twice).
	existing := s.bySkill[skillName]
	seen := map[string]bool{}
	for _, n := range existing {
		seen[n] = true
	}
	for _, n := range registered {
		if !seen[n] {
			s.bySkill[skillName] = append(s.bySkill[skillName], n)
			seen[n] = true
		}
	}
	s.mu.Unlock()
	return registered, firstErr
}

// UnregisterSkillMCP stops and removes every MCP server previously registered
// for the named skill (VAL-CROSS-012). Subsequent task() calls for those
// namespaced tools return "unknown tool" from the MCP host.
func (s *SkillMCPHostRegistrar) UnregisterSkillMCP(skillName string) []string {
	if s == nil || s.Host == nil {
		return nil
	}
	skillName = strings.ToLower(strings.TrimSpace(skillName))
	s.mu.Lock()
	names := s.bySkill[skillName]
	delete(s.bySkill, skillName)
	s.mu.Unlock()
	var removed []string
	for _, name := range names {
		if err := s.Host.Unregister(name); err != nil {
			if s.Logf != nil {
				s.Logf("skill mcp: unregister %q: %v", name, err)
			}
			continue
		}
		removed = append(removed, name)
	}
	return removed
}

// RegisteredSkillServers returns the fully-qualified server names currently
// registered for the named skill (for diagnostics/tests).
func (s *SkillMCPHostRegistrar) RegisteredSkillServers(skillName string) []string {
	if s == nil {
		return nil
	}
	skillName = strings.ToLower(strings.TrimSpace(skillName))
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bySkill[skillName]...)
}
