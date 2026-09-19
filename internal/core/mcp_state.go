package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
)

// ErrMCPReadOnlyLayer marks a mutation refused because a higher-precedence
// config layer (environment override or project config) supplies the server.
// Callers map it to HTTP 409: the request is well-formed, the target is just
// not writable from Settings.
var ErrMCPReadOnlyLayer = errors.New("mcp: server is not editable from Settings")

// mcpState holds the persistent MCP host that survives across chat streams.
// The host is reloaded when the active workspace changes (project config overlay)
// or when the user toggles/adds/removes servers via the API.
type mcpState struct {
	mu      sync.RWMutex
	host    *mcp.Host
	servers map[string]config.MCPServerConfig
	// layers records the config layer that supplies each server's effective
	// config, so Settings can mark env overrides read-only without re-reading
	// the config hierarchy on every status poll.
	layers map[string]config.MCPServerLayer
	// workspaceRoot is the last workspace root used for config loading.
	workspaceRoot string
	// configHome is the resolved config home for MCP config loading.
	configHome string
}

// MCPServerView is the public API shape for one MCP server (no secrets).
type MCPServerView struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Enabled   bool     `json:"enabled"`
	OK        bool     `json:"ok"`
	Error     string   `json:"error,omitempty"`
	ToolNames []string `json:"tool_names,omitempty"`
	ToolCount int      `json:"tool_count"`
	// Builtin marks servers that Inferenesia auto-configures (e.g. cocoindex).
	Builtin bool `json:"builtin,omitempty"`
	// Command display (no env/secrets) for stdio servers.
	Command string `json:"command,omitempty"`
	// BaseURL display (no headers/secrets) for http servers.
	BaseURL string `json:"base_url,omitempty"`
	// Layer is the config source that wins for this server: home, project, env.
	Layer string `json:"layer,omitempty"`
	// Managed marks servers whose effective config comes from an environment
	// override. Settings must not offer a toggle for them: persisting a
	// lower-precedence value cannot change effective state.
	Managed bool `json:"managed,omitempty"`
}

// MCPStatusView is the /api/mcp/status payload.
type MCPStatusView struct {
	Servers      []MCPServerView `json:"servers"`
	TotalServers int             `json:"total_servers"`
	// EnabledCount is how many configured servers the user has switched on.
	// Health is only meaningful against this denominator — disabled servers are
	// intentionally not running and must not read as failures.
	EnabledCount int `json:"enabled_count"`
	HealthyCount int `json:"healthy_count"`
	TotalTools   int `json:"total_tools"`
}

// ensureMCPState lazily initializes the MCP state on Service.
func (s *Service) ensureMCPState() *mcpState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mcpState == nil {
		s.mcpState = &mcpState{
			configHome: s.configHome,
		}
	}
	return s.mcpState
}

// MCPHost returns the persistent MCP host, initializing it if needed.
// Returns nil when no MCP servers are configured.
func (s *Service) MCPHost() *mcp.Host {
	st := s.ensureMCPState()
	st.mu.RLock()
	h := st.host
	st.mu.RUnlock()
	return h
}

// ReloadMCP reloads the MCP host from config, merging home config + project overlay
// + built-in auto-detected servers (cocoindex). Safe to call on workspace switch
// or after config changes. The old host is closed before the new one replaces it.
func (s *Service) ReloadMCP(workspaceRoot string) error {
	st := s.ensureMCPState()
	home := s.ConfigHome()

	// Load user-configured servers from config.yaml hierarchy.
	servers, err := config.LoadMCPServers(home, workspaceRoot)
	if err != nil {
		// Schema reject — surface but don't crash.
		return fmt.Errorf("mcp reload: %w", err)
	}

	// Auto-detect and inject built-in servers. A persisted entry with the same
	// name (including enabled:false) always wins, making built-in toggles durable.
	for name, cfg := range builtinMCPServers() {
		if _, exists := servers[name]; !exists {
			servers[name] = cfg
		}
	}

	// Resolve which layer wins per server once, so status can mark env-managed
	// entries read-only without re-walking the hierarchy on every poll.
	layers, err := config.MCPServerLayers(workspaceRoot)
	if err != nil {
		return fmt.Errorf("mcp reload: %w", err)
	}

	var next *mcp.Host
	hasEnabled := false
	for _, cfg := range servers {
		if cfg.IsEnabled() {
			hasEnabled = true
			break
		}
	}
	if hasEnabled {
		next = mcp.NewHost(mcp.DefaultClientInfo)
		next.Logf = func(format string, args ...any) {
			log.Printf(format, args...)
		}

		regCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_ = next.RegisterAll(regCtx, servers)
		cancel()
	}

	st.mu.Lock()
	previous := st.host
	st.host = next
	st.servers = servers
	st.layers = layers
	st.workspaceRoot = workspaceRoot
	st.mu.Unlock()

	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

// GetMCPStatus returns the current MCP server status snapshot (no secrets).
func (s *Service) GetMCPStatus() MCPStatusView {
	st := s.ensureMCPState()
	st.mu.RLock()
	host := st.host
	servers := make(map[string]config.MCPServerConfig, len(st.servers))
	for name, cfg := range st.servers {
		servers[name] = cfg
	}
	layers := make(map[string]config.MCPServerLayer, len(st.layers))
	for name, layer := range st.layers {
		layers[name] = layer
	}
	st.mu.RUnlock()

	statusByName := map[string]mcp.ServerStatus{}
	if host != nil {
		for _, status := range host.Status() {
			statusByName[status.Name] = status
		}
	}

	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	view := MCPStatusView{Servers: make([]MCPServerView, 0, len(names))}
	for _, name := range names {
		serverCfg := servers[name]
		status, registered := statusByName[name]
		sv := MCPServerView{
			Name:    name,
			Type:    serverCfg.Type,
			Enabled: serverCfg.IsEnabled(),
			Builtin: isBuiltinMCPServer(name),
			Command: displayCommand(serverCfg),
			BaseURL: displayBaseURL(serverCfg),
		}
		layer := layers[name]
		if layer == "" {
			layer = config.MCPServerLayerHome
		}
		sv.Layer = string(layer)
		sv.Managed = layer == config.MCPServerLayerEnv
		if registered && sv.Enabled {
			sv.OK = status.OK
			sv.Error = status.Error
			sv.ToolNames = status.ToolNames
			sv.ToolCount = len(status.ToolNames)
		}
		view.Servers = append(view.Servers, sv)
		view.TotalServers++
		if sv.Enabled {
			view.EnabledCount++
		}
		if sv.OK {
			view.HealthyCount++
		}
		view.TotalTools += sv.ToolCount
	}
	return view
}

// ToggleMCPServer enables/disables an MCP server by name and persists to config.yaml.
// The host is reloaded after the config change.
func (s *Service) ToggleMCPServer(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("mcp: server name is required")
	}
	home := s.ConfigHome()
	st := s.ensureMCPState()
	st.mu.RLock()
	ws := st.workspaceRoot
	snapshot := st.servers != nil
	st.mu.RUnlock()
	// st.servers stays nil until the first reload, so a toggle issued before
	// any status read would report "not found" for a server that exists on disk.
	if !snapshot {
		if err := s.ReloadMCP(ws); err != nil {
			return err
		}
	}
	st.mu.RLock()
	serverCfg, displayed := st.servers[name]
	layer, layered := st.layers[name]
	st.mu.RUnlock()

	if !displayed {
		return fmt.Errorf("mcp: server %q not found in config", name)
	}
	// Environment overrides win over anything Settings can persist, so refuse
	// up front rather than writing a value the next reload would ignore.
	if layer == config.MCPServerLayerEnv {
		return fmt.Errorf("%w: %q is controlled by %s", ErrMCPReadOnlyLayer, name, config.EnvMCP)
	}

	if layered || !isBuiltinMCPServer(name) {
		if err := config.SetMCPServerEnabled(home, ws, name, enabled); err != nil {
			if errors.Is(err, config.ErrMCPServerReadOnly) {
				return fmt.Errorf("%w: %s", ErrMCPReadOnlyLayer, err)
			}
			return fmt.Errorf("mcp toggle: %w", err)
		}
	} else {
		// Auto-detected built-ins have no config entry until first toggle, and
		// SetMCPServerEnabled would reject them as "not found". Persist their
		// complete validated config in home so enabled:false stays visible and
		// a later toggle can reverse it.
		serverCfg.Enabled = &enabled
		if err := config.UpsertMCPServer(home, name, serverCfg); err != nil {
			return fmt.Errorf("mcp toggle: save built-in: %w", err)
		}
	}

	return s.ReloadMCP(ws)
}

// AddMCPServer adds a new MCP server to config.yaml and reloads the host.
func (s *Service) AddMCPServer(name string, cfg config.MCPServerConfig) error {
	home := s.ConfigHome()
	st := s.ensureMCPState()
	if err := config.UpsertMCPServer(home, name, cfg); err != nil {
		return err
	}
	st.mu.RLock()
	ws := st.workspaceRoot
	st.mu.RUnlock()
	return s.ReloadMCP(ws)
}

// RemoveMCPServer deletes an MCP server from home config.yaml and reloads the host.
// Only the home layer is writable here: a server supplied by a project overlay or
// an environment override would survive the delete, so removing it is rejected
// instead of reporting a success the user cannot observe.
func (s *Service) RemoveMCPServer(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("mcp: server name is required")
	}
	home := s.ConfigHome()
	st := s.ensureMCPState()
	st.mu.RLock()
	ws := st.workspaceRoot
	snapshot := st.servers != nil
	st.mu.RUnlock()
	// Without a loaded snapshot every layer reads as "home", so a project or
	// env server would pass the read-only guard and delete nothing silently.
	if !snapshot {
		if err := s.ReloadMCP(ws); err != nil {
			return err
		}
	}
	st.mu.RLock()
	layer := st.layers[name]
	st.mu.RUnlock()

	switch layer {
	case config.MCPServerLayerEnv:
		return fmt.Errorf("%w: %q is controlled by %s", ErrMCPReadOnlyLayer, name, config.EnvMCP)
	case config.MCPServerLayerProject:
		return fmt.Errorf("%w: %q comes from this project's config and must be removed there, not from %s",
			ErrMCPReadOnlyLayer, name, home)
	}

	if err := config.RemoveMCPServer(home, name); err != nil {
		return err
	}
	return s.ReloadMCP(ws)
}

// builtinMCPServers returns auto-detected built-in MCP server configs.
// These are injected on top of user config but never override user entries.
func builtinMCPServers() map[string]config.MCPServerConfig {
	out := map[string]config.MCPServerConfig{}

	// CocoIndex Code — semantic code search (AST-based, token-efficient).
	if path, err := exec.LookPath("ccc"); err == nil && path != "" {
		enabled := true
		out["cocoindex"] = config.MCPServerConfig{
			Name:    "cocoindex",
			Type:    mcp.TypeStdio,
			Command: path,
			Args:    []string{"mcp"},
			Enabled: &enabled,
		}
	}
	return out
}

// isBuiltinMCPServer reports whether name is a known built-in server.
func isBuiltinMCPServer(name string) bool {
	switch name {
	case "cocoindex":
		return true
	}
	return false
}

// displayCommand returns a safe display string for stdio server command (no env).
func displayCommand(cfg config.MCPServerConfig) string {
	if cfg.Type != mcp.TypeStdio {
		return ""
	}
	argv := cfg.CommandArgv()
	if len(argv) == 0 {
		return ""
	}
	return strings.Join(argv, " ")
}

// displayBaseURL returns a safe display string for http server base URL (no headers).
func displayBaseURL(cfg config.MCPServerConfig) string {
	if cfg.Type != mcp.TypeHTTP {
		return ""
	}
	return cfg.BaseURL
}

// WireMCPHostIntoRegistry wires the persistent MCP host into a tools.Registry.
// Used by chat.go instead of creating a per-stream host.
func (s *Service) WireMCPHostIntoRegistry(reg *tools.Registry, workspaceRoot string) (*mcp.Host, error) {
	st := s.ensureMCPState()

	// Reload if workspace changed or host not yet initialized.
	st.mu.RLock()
	currentWS := st.workspaceRoot
	host := st.host
	hasServers := len(st.servers) > 0
	st.mu.RUnlock()
	if (!hasServers && host == nil) || currentWS != workspaceRoot {
		if err := s.ReloadMCP(workspaceRoot); err != nil {
			// Non-fatal — emit error but continue without MCP.
			return nil, err
		}
		st.mu.RLock()
		host = st.host
		st.mu.RUnlock()
	}

	if host == nil {
		return nil, nil
	}

	reg.SetMCPHost(host)
	return host, nil
}
