package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"gopkg.in/yaml.v3"
)

// MCPServerConfig is the on-disk YAML shape for one MCP server under mcp: in
// config.yaml (VAL-MCP-002). Transport types: stdio|local|http|remote only.
// type: rag / vector backends are rejected (VAL-MCP-004).
//
// Headers and auth tokens must never be logged by callers.
type MCPServerConfig = mcp.ServerConfig

// MCPAuthConfig is optional structured auth for remote HTTP MCP servers.
type MCPAuthConfig = mcp.AuthConfig

// EnvMCP is the preferred YAML map override for MCP servers (VAL-MCP-007).
// Hierarchy: env (INFERENESIA_MCP / INFERNESIA_MCP / YURA_AI_MCP) >
// project .inferenesia| .infernesia| .yura-ai /config.yaml > home config > defaults.
// Example:
//
//	INFERENESIA_MCP=$'echo:\n  type: stdio\n  command: /path/to/server'
const (
	EnvMCP       = "INFERENESIA_MCP"
	EnvMCPTypo   = "INFERNESIA_MCP"
	EnvMCPLegacy = "YURA_AI_MCP"
)

// ProjectConfigDirName is the preferred project-local config folder (under a workspace root).
const ProjectConfigDirName = ".inferenesia"

// TypoProjectConfigDirName is the misspelled project-local config folder.
const TypoProjectConfigDirName = ".infernesia"

// LegacyProjectConfigDirName is the pre-rebrand project-local config folder
// (still read when the preferred folder is absent).
const LegacyProjectConfigDirName = ".yura-ai"

// ValidateMCPSection validates and normalizes the mcp: map from config.yaml.
// Map keys become server names when Name is empty. Rejects type:rag (VAL-MCP-004).
func ValidateMCPSection(servers map[string]MCPServerConfig) (map[string]MCPServerConfig, error) {
	return mcp.ValidateServers(servers)
}

// ToMCPServers converts the FileConfig.MCP section into a normalized map ready
// for mcp.Host.RegisterAll. Empty/nil input returns an empty map (not an error).
func ToMCPServers(servers map[string]MCPServerConfig) (map[string]mcp.ServerConfig, error) {
	if len(servers) == 0 {
		return map[string]mcp.ServerConfig{}, nil
	}
	norm, err := ValidateMCPSection(servers)
	if err != nil {
		return nil, err
	}
	return norm, nil
}

// HasMCPAuth reports whether any MCP server entry may hold secret material
// (headers, inline token, or token_env). Used for 0600 mode when saving.
func HasMCPAuth(servers map[string]MCPServerConfig) bool {
	for _, s := range servers {
		if len(s.Headers) > 0 {
			return true
		}
		if s.Auth != nil {
			if strings.TrimSpace(s.Auth.Token) != "" || strings.TrimSpace(s.Auth.TokenEnv) != "" {
				return true
			}
		}
	}
	return false
}

// MCPSummary is a secret-safe summary for diagnostics (no header values).
func MCPSummary(servers map[string]MCPServerConfig) string {
	if len(servers) == 0 {
		return "mcp: (none)"
	}
	names := make([]string, 0, len(servers))
	for k, s := range servers {
		name := k
		if strings.TrimSpace(s.Name) != "" {
			name = s.Name
		}
		t := strings.TrimSpace(s.Type)
		if t == "" {
			t = "?"
		}
		names = append(names, fmt.Sprintf("%s(%s)", name, t))
	}
	return "mcp: " + strings.Join(names, ", ")
}

// LoadMCPServers resolves MCP configs with hierarchy (VAL-MCP-007):
//
//	env (YURA_AI_MCP) > project <workspace>/.inferenesia/config.yaml > home config.yaml > defaults (empty)
//
// home is ~/.yura-ai or $YURA_AI_HOME. workspace may be empty (skip project layer).
// Returned map is normalized (types aliased, argv filled). Schema errors bubble.
func LoadMCPServers(home, workspace string) (map[string]MCPServerConfig, error) {
	merged := map[string]MCPServerConfig{}

	// 1) Global home config.yaml
	if strings.TrimSpace(home) != "" {
		cfg, err := LoadFromDir(home)
		if err != nil {
			return nil, fmt.Errorf("mcp home config: %w", err)
		}
		for k, v := range cfg.MCP {
			merged[k] = v
		}
	}

	// 2) Project-local config overlays home by name.
	if ws := strings.TrimSpace(workspace); ws != "" {
		for _, dirName := range []string{ProjectConfigDirName, TypoProjectConfigDirName, LegacyProjectConfigDirName} {
			projPath := filepath.Join(ws, dirName, ConfigFileName)
			pcfg, err := LoadFile(projPath)
			if err != nil {
				return nil, fmt.Errorf("mcp project config: %w", err)
			}
			if len(pcfg.MCP) == 0 {
				continue
			}
			for k, v := range pcfg.MCP {
				merged[k] = v
			}
			break
		}
	}

	// 3) Env override (highest priority per name)
	if envMap, err := MCPServersFromEnv(); err != nil {
		return nil, err
	} else if len(envMap) > 0 {
		for k, v := range envMap {
			merged[k] = v
		}
	}

	if len(merged) == 0 {
		return map[string]MCPServerConfig{}, nil
	}
	return ValidateMCPSection(merged)
}

// MCPServerLayer identifies the highest-precedence configured layer for a server.
// Environment overrides are read-only; home and project entries can be updated in place.
type MCPServerLayer string

const (
	MCPServerLayerHome    MCPServerLayer = "home"
	MCPServerLayerProject MCPServerLayer = "project"
	MCPServerLayerEnv     MCPServerLayer = "env"
)

// MCPServerLayers reports the winning config layer for every server that a
// higher-precedence layer than home supplies. A name absent from the result is
// served by home config (or is a built-in with no config entry yet). Callers
// that need the layer for a whole status list must use this instead of calling
// EffectiveMCPServer per name, which re-reads and re-validates the entire
// hierarchy for each entry.
func MCPServerLayers(workspace string) (map[string]MCPServerLayer, error) {
	projectServers, _, err := loadProjectMCPServers(workspace)
	if err != nil {
		return nil, err
	}
	envServers, err := MCPServersFromEnv()
	if err != nil {
		return nil, err
	}

	out := make(map[string]MCPServerLayer, len(projectServers)+len(envServers))
	for name := range projectServers {
		out[name] = MCPServerLayerProject
	}
	for name := range envServers {
		out[name] = MCPServerLayerEnv
	}
	return out, nil
}

// EffectiveMCPServer returns the effective normalized server config and its
// highest-precedence source. The result follows LoadMCPServers precedence.
func EffectiveMCPServer(home, workspace, name string) (MCPServerConfig, MCPServerLayer, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MCPServerConfig{}, "", false, fmt.Errorf("mcp: server name is required")
	}

	servers, err := LoadMCPServers(home, workspace)
	if err != nil {
		return MCPServerConfig{}, "", false, err
	}
	cfg, ok := servers[name]
	if !ok {
		return MCPServerConfig{}, "", false, nil
	}

	layer := MCPServerLayerHome
	if projectServers, _, err := loadProjectMCPServers(workspace); err != nil {
		return MCPServerConfig{}, "", false, err
	} else if _, exists := projectServers[name]; exists {
		layer = MCPServerLayerProject
	}
	if envServers, err := MCPServersFromEnv(); err != nil {
		return MCPServerConfig{}, "", false, err
	} else if _, exists := envServers[name]; exists {
		layer = MCPServerLayerEnv
	}
	return cfg, layer, true, nil
}

// ErrMCPServerReadOnly reports a server whose effective config comes from a
// layer Settings cannot write. Callers use errors.Is to distinguish it from a
// genuine save failure.
var ErrMCPServerReadOnly = errors.New("mcp: server is not editable from this layer")

// SetMCPServerEnabled updates the layer that currently supplies the effective
// server config. Environment entries are deliberately read-only: persisting a
// lower-priority value would not change effective state and is therefore rejected.
func SetMCPServerEnabled(home, workspace, name string, enabled bool) error {
	cfg, layer, ok, err := EffectiveMCPServer(home, workspace, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("mcp: server %q not found in config", strings.TrimSpace(name))
	}
	if layer == MCPServerLayerEnv {
		return fmt.Errorf("%w: %q is controlled by %s", ErrMCPServerReadOnly, name, EnvMCP)
	}
	cfg.Enabled = &enabled

	if layer == MCPServerLayerProject {
		servers, path, err := loadProjectMCPServers(workspace)
		if err != nil {
			return err
		}
		servers[name] = cfg
		return saveMCPServersAtPath(path, servers)
	}

	fc, err := LoadFromDir(home)
	if err != nil {
		return err
	}
	if fc.MCP == nil {
		fc.MCP = map[string]MCPServerConfig{}
	}
	fc.MCP[name] = cfg
	return SaveToDir(home, fc)
}

func loadProjectMCPServers(workspace string) (map[string]MCPServerConfig, string, error) {
	ws := strings.TrimSpace(workspace)
	if ws == "" {
		return map[string]MCPServerConfig{}, "", nil
	}
	for _, dirName := range []string{ProjectConfigDirName, TypoProjectConfigDirName, LegacyProjectConfigDirName} {
		path := filepath.Join(ws, dirName, ConfigFileName)
		fc, err := LoadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("mcp project config: %w", err)
		}
		if len(fc.MCP) == 0 {
			continue
		}
		return fc.MCP, path, nil
	}
	return map[string]MCPServerConfig{}, filepath.Join(projectConfigDir(ws), ConfigFileName), nil
}

func saveMCPServersAtPath(path string, servers map[string]MCPServerConfig) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty project config path")
	}
	norm, err := ValidateMCPSection(servers)
	if err != nil {
		return err
	}
	fc, err := LoadFile(path)
	if err != nil {
		return err
	}
	fc.MCP = norm
	return SaveFile(path, fc)
}

// MCPServersFromEnv parses INFERENESIA_MCP / INFERNESIA_MCP / YURA_AI_MCP as a YAML map
// of server configs (VAL-MCP-007). Empty env returns nil, nil.
func MCPServersFromEnv() (map[string]MCPServerConfig, error) {
	raw := firstConfigEnv(EnvMCP, EnvMCPTypo, EnvMCPLegacy)
	if raw == "" {
		return nil, nil
	}
	// Accept either a bare map or a document with top-level "mcp:" key.
	var asMap map[string]MCPServerConfig
	if err := yaml.Unmarshal([]byte(raw), &asMap); err == nil && len(asMap) > 0 {
		// Heuristic: if the only key is "mcp" and nested would be wrong type, try wrapper.
		if _, onlyMCP := asMap["mcp"]; onlyMCP && len(asMap) == 1 {
			// Fall through to wrapper parse.
		} else {
			// Reject if a key looks like a file-level field rather than a server name
			// by requiring later Validate — return as-is first.
			// But if user put `mcp:\n  echo:...` the unmarshal into map[string]ServerConfig
			// puts type empty for "mcp" key. Detect and re-parse.
			if needWrapper(asMap) {
				return parseMCPEnvWrapper(raw)
			}
			return asMap, nil
		}
	}
	return parseMCPEnvWrapper(raw)
}

func needWrapper(m map[string]MCPServerConfig) bool {
	if len(m) != 1 {
		return false
	}
	v, ok := m["mcp"]
	if !ok {
		return false
	}
	// Wrapper only if "mcp" entry has no usable type (nested blob mis-parsed).
	return strings.TrimSpace(v.Type) == ""
}

func parseMCPEnvWrapper(raw string) (map[string]MCPServerConfig, error) {
	var wrap struct {
		MCP map[string]MCPServerConfig `yaml:"mcp"`
	}
	if err := yaml.Unmarshal([]byte(raw), &wrap); err != nil {
		return nil, fmt.Errorf("%s: parse: %w", EnvMCP, err)
	}
	if len(wrap.MCP) == 0 {
		// Try plain map again as final attempt.
		var asMap map[string]MCPServerConfig
		if err := yaml.Unmarshal([]byte(raw), &asMap); err != nil {
			return nil, fmt.Errorf("%s: parse: %w", EnvMCP, err)
		}
		return asMap, nil
	}
	return wrap.MCP, nil
}

// UpsertMCPServer adds or replaces one MCP server in home config.yaml and persists
// with mode 0600 when auth/headers present (always 0600 via SaveFile) (VAL-MCP-007).
func UpsertMCPServer(home, name string, cfg MCPServerConfig) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("mcp: server name is required")
	}
	if strings.TrimSpace(home) == "" {
		return fmt.Errorf("empty config home")
	}
	cfg.Name = name
	norm, err := ValidateServerOne(cfg)
	if err != nil {
		return err
	}
	fc, err := LoadFromDir(home)
	if err != nil {
		return err
	}
	if fc.Version == 0 {
		fc.Version = 1
	}
	if fc.MCP == nil {
		fc.MCP = map[string]MCPServerConfig{}
	}
	fc.MCP[name] = norm
	if err := SaveToDir(home, fc); err != nil {
		return err
	}
	// Reinforce 0600 whenever secrets may be present (VAL-MCP-007 / VAL-FOUND-011).
	if HasMCPAuth(fc.MCP) {
		path := filepath.Join(home, ConfigFileName)
		if err := os.Chmod(path, FilePerm); err != nil {
			return fmt.Errorf("chmod mcp config: %w", err)
		}
	}
	return nil
}

// ValidateServerOne validates a single server config (name required).
func ValidateServerOne(cfg MCPServerConfig) (MCPServerConfig, error) {
	return mcp.ValidateServer(cfg)
}

// RemoveMCPServer deletes one MCP server from home config.yaml (VAL-MCP-006/007).
// Missing name is a no-op.
func RemoveMCPServer(home, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("mcp: server name is required")
	}
	if strings.TrimSpace(home) == "" {
		return fmt.Errorf("empty config home")
	}
	fc, err := LoadFromDir(home)
	if err != nil {
		return err
	}
	if fc.MCP == nil {
		return fmt.Errorf("mcp: server %q not found in %s", name, home)
	}
	if _, ok := fc.MCP[name]; !ok {
		return fmt.Errorf("mcp: server %q not found in %s", name, home)
	}
	delete(fc.MCP, name)
	if len(fc.MCP) == 0 {
		fc.MCP = nil
	}
	return SaveToDir(home, fc)
}

// SaveMCPServers replaces the entire mcp: section in home config.yaml (VAL-MCP-007).
// Mode is always 0600; reinforced when HasMCPAuth.
func SaveMCPServers(home string, servers map[string]MCPServerConfig) error {
	if strings.TrimSpace(home) == "" {
		return fmt.Errorf("empty config home")
	}
	norm, err := ValidateMCPSection(servers)
	if err != nil {
		return err
	}
	fc, err := LoadFromDir(home)
	if err != nil {
		return err
	}
	if fc.Version == 0 {
		fc.Version = 1
	}
	if len(norm) == 0 {
		fc.MCP = nil
	} else {
		fc.MCP = norm
	}
	if err := SaveToDir(home, fc); err != nil {
		return err
	}
	if HasMCPAuth(norm) {
		path := filepath.Join(home, ConfigFileName)
		if err := os.Chmod(path, FilePerm); err != nil {
			return fmt.Errorf("chmod mcp config: %w", err)
		}
	}
	return nil
}

// SaveMCPServersToProject writes mcp: into <workspace>/.inferenesia/config.yaml
// (preferred) — or legacy <workspace>/.inferenesia/config.yaml when that already
// exists and the preferred dir does not (VAL-MCP-007, rebrand dual-write).
// Creates the project config dir (0700) when missing. Mode 0600 when auth present.
func SaveMCPServersToProject(workspace string, servers map[string]MCPServerConfig) error {
	ws := strings.TrimSpace(workspace)
	if ws == "" {
		return fmt.Errorf("empty workspace")
	}
	norm, err := ValidateMCPSection(servers)
	if err != nil {
		return err
	}
	dir := projectConfigDir(ws)
	if err := os.MkdirAll(dir, DirPerm); err != nil {
		return fmt.Errorf("mkdir project config: %w", err)
	}
	_ = os.Chmod(dir, DirPerm)
	path := filepath.Join(dir, ConfigFileName)
	fc, err := LoadFile(path)
	if err != nil {
		return err
	}
	if fc.Version == 0 {
		fc.Version = 1
	}
	if len(norm) == 0 {
		fc.MCP = nil
	} else {
		fc.MCP = norm
	}
	if err := SaveFile(path, fc); err != nil {
		return err
	}
	if HasMCPAuth(norm) {
		if err := os.Chmod(path, FilePerm); err != nil {
			return fmt.Errorf("chmod project mcp config: %w", err)
		}
	}
	return nil
}

// projectConfigDir returns an existing project config dir if present, else
// the preferred .inferenesia path for new writes.
func projectConfigDir(ws string) string {
	preferred := filepath.Join(ws, ProjectConfigDirName)
	if _, err := os.Stat(preferred); err == nil {
		return preferred
	}
	typo := filepath.Join(ws, TypoProjectConfigDirName)
	if _, err := os.Stat(typo); err == nil {
		return typo
	}
	legacy := filepath.Join(ws, LegacyProjectConfigDirName)
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return preferred
}

// firstConfigEnv returns the first non-empty (trimmed) value among the given
// env var names, in order. Used for legacy dual-read of MCP env.
func firstConfigEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}
