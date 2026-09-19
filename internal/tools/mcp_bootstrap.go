package tools

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/mcp"
)

// AttachMCPOptions controls MCP host bootstrap (VAL-MCP-001..007).
type AttachMCPOptions struct {
	// ConfigHome is ~/.yura-ai or $YURA_AI_HOME. Empty → config.Home().
	ConfigHome string
	// Workspace is the project root; when set, project .inferenesia/config.yaml overlays home.
	Workspace string
	// Log receives clear unavailable messages (never Authorization values).
	Log io.Writer
}

// AttachMCPFromConfig loads the mcp: section from config home, connects servers,
// and wires namespaced tools into reg (VAL-MCP-001/002/003/007).
//
// Hierarchy: env (YURA_AI_MCP) > project .inferenesia/ > home config.yaml > defaults.
// Failures for individual servers are isolated: clear unavailable messages go to
// logw (never including Authorization header values). Schema rejection
// (e.g. type:rag) is returned as an error so the caller can surface it, but the
// agent loop remains usable (no tools from that bad config).
//
// Returns the Host (caller should Close when the session ends) and a best-effort
// first connection error (still partially-registered hosts are returned).
func AttachMCPFromConfig(ctx context.Context, reg *Registry, configHome string, logw io.Writer) (*mcp.Host, error) {
	return AttachMCP(ctx, reg, AttachMCPOptions{ConfigHome: configHome, Log: logw})
}

// AttachMCPFromWorkspace is like AttachMCPFromConfig but also merges project
// <workspace>/.inferenesia/config.yaml under the env > project > home hierarchy (VAL-MCP-007).
func AttachMCPFromWorkspace(ctx context.Context, reg *Registry, configHome, workspace string, logw io.Writer) (*mcp.Host, error) {
	return AttachMCP(ctx, reg, AttachMCPOptions{
		ConfigHome: configHome,
		Workspace:  workspace,
		Log:        logw,
	})
}

// AttachMCP loads MCP servers via config.LoadMCPServers and registers them.
func AttachMCP(ctx context.Context, reg *Registry, opts AttachMCPOptions) (*mcp.Host, error) {
	if reg == nil {
		return nil, fmt.Errorf("tools: registry is nil")
	}
	home := opts.ConfigHome
	if home == "" {
		var err error
		home, err = config.Home()
		if err != nil {
			return nil, err
		}
	}
	servers, err := config.LoadMCPServers(home, opts.Workspace)
	if err != nil {
		// Schema reject (VAL-MCP-004) or load error — surface, do not crash agent.
		return nil, err
	}
	if len(servers) == 0 {
		return nil, nil
	}

	h := mcp.NewHost(mcp.DefaultClientInfo)
	if opts.Log != nil {
		logger := log.New(opts.Log, "", 0)
		h.Logf = func(format string, args ...any) {
			// Only format server status strings; never dump cfg.Headers.
			logger.Printf(format, args...)
		}
	}

	// Bound register phase; stdio process itself is NOT context-bound (lives until Close).
	regCtx := ctx
	if ctx == nil {
		regCtx = context.Background()
	}
	regCtx, cancel := context.WithTimeout(regCtx, 20*time.Second)
	defer cancel()

	first := h.RegisterAll(regCtx, servers)
	reg.SetMCPHost(h)
	return h, first
}

// AttachMCPHost wires an already-built host into the registry (tests / custom).
func AttachMCPHost(reg *Registry, h *mcp.Host) {
	if reg == nil {
		return
	}
	reg.SetMCPHost(h)
}
