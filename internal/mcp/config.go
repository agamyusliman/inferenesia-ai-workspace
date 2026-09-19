// Package mcp implements the MCP client host for Inferenesia.
// It supports generic stdio and HTTP MCP servers only.
// Memory stays Multi Brain; this package does not wire external vector/RAG backends.
package mcp

import (
	"fmt"
	"strings"
)

// Transport type constants accepted in config.
const (
	TypeLocal  = "local"  // alias for stdio
	TypeStdio  = "stdio"  // spawn command + args
	TypeRemote = "remote" // alias for http
	TypeHTTP   = "http"   // remote base_url
)

// ServerConfig is the shaped config for one MCP server (VAL-MCP-001/002).
//
// YAML example:
//
//	mcp:
//	  echo:
//	    type: stdio
//	    command: ["path/to/server"]
//	  docs:
//	    type: http
//	    base_url: http://127.0.0.1:4115/mcp
//	    headers:
//	      Authorization: Bearer ...
//	    auth:
//	      type: bearer
//	      token_env: MY_TOKEN
//
// Secrets in headers/auth must never be logged (VAL-MCP-002).
type ServerConfig struct {
	// Name is the registry key (from map key or explicit field).
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Type is one of: local|stdio|remote|http. Required.
	// Rejected: rag and other non-MCP transport types (VAL-MCP-004).
	Type string `yaml:"type" json:"type"`

	// Command + Args for stdio/local (Command may be argv[0] only; Args the rest).
	// Config may also pass command as a string list in YAML via CommandList.
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	CommandList []string `yaml:"command_list,omitempty" json:"command_list,omitempty"` // alt: full argv

	// Env is optional extra environment for the stdio process (KEY=VALUE not logged if secret-like).
	Env []string `yaml:"env,omitempty" json:"env,omitempty"`

	// BaseURL is the remote HTTP MCP endpoint (also accepts url).
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	URL     string `yaml:"url,omitempty" json:"url,omitempty"` // alias for base_url

	// Headers are sent on every remote request. Values must never be logged.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// Auth is optional structured auth (bearer/token). Prefer token_env over inline token.
	Auth *AuthConfig `yaml:"auth,omitempty" json:"auth,omitempty"`

	// Enabled defaults true when nil/omitted.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// AuthConfig holds optional remote auth (never logged).
type AuthConfig struct {
	Type     string `yaml:"type,omitempty" json:"type,omitempty"` // bearer | header
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
	TokenEnv string `yaml:"token_env,omitempty" json:"token_env,omitempty"`
	Header   string `yaml:"header,omitempty" json:"header,omitempty"` // when type=header, default Authorization
}

// FileMCP is the mcp: section of config.yaml.
// Map keys become server names when ServerConfig.Name is empty.
type FileMCP struct {
	Servers map[string]ServerConfig `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}

// ValidateServer checks one server config and normalizes type aliases.
// Rejects type:rag and empty/invalid shapes (VAL-MCP-002/004).
func ValidateServer(cfg ServerConfig) (ServerConfig, error) {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return ServerConfig{}, fmt.Errorf("mcp: server name is required")
	}
	if !validServerName(name) {
		return ServerConfig{}, fmt.Errorf("mcp: invalid server name %q (use [a-zA-Z0-9._-]+)", name)
	}

	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	switch t {
	case "":
		return ServerConfig{}, fmt.Errorf("mcp: server %q: type is required (stdio|http|local|remote)", name)
	case "rag", "vector", "embedding", "embeddings":
		// Hard rejection — agent host is generic MCP only (VAL-MCP-004).
		return ServerConfig{}, fmt.Errorf("mcp: server %q: type %q is not supported (MCP host does not wire RAG/vector backends; use Multi Brain)", name, t)
	case TypeLocal, TypeStdio:
		t = TypeStdio
	case TypeRemote, TypeHTTP:
		t = TypeHTTP
	default:
		return ServerConfig{}, fmt.Errorf("mcp: server %q: unknown type %q (want stdio|http|local|remote)", name, cfg.Type)
	}

	out := cfg
	out.Name = name
	out.Type = t

	switch t {
	case TypeStdio:
		argv := commandArgv(cfg)
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			return ServerConfig{}, fmt.Errorf("mcp: server %q: stdio requires command (or command_list)", name)
		}
		out.Command = argv[0]
		if len(argv) > 1 {
			out.Args = append([]string(nil), argv[1:]...)
		} else {
			out.Args = nil
		}
		out.CommandList = nil
		out.BaseURL = ""
		out.URL = ""
	case TypeHTTP:
		base := strings.TrimSpace(cfg.BaseURL)
		if base == "" {
			base = strings.TrimSpace(cfg.URL)
		}
		if base == "" {
			return ServerConfig{}, fmt.Errorf("mcp: server %q: http requires base_url (or url)", name)
		}
		out.BaseURL = base
		out.URL = ""
		out.Command = ""
		out.Args = nil
		out.CommandList = nil
	}

	if out.Enabled == nil {
		en := true
		out.Enabled = &en
	}
	return out, nil
}

// ValidateServers validates a name→config map; keys supply Name when empty.
func ValidateServers(servers map[string]ServerConfig) (map[string]ServerConfig, error) {
	if len(servers) == 0 {
		return map[string]ServerConfig{}, nil
	}
	out := make(map[string]ServerConfig, len(servers))
	for key, cfg := range servers {
		if strings.TrimSpace(cfg.Name) == "" {
			cfg.Name = key
		}
		norm, err := ValidateServer(cfg)
		if err != nil {
			return nil, err
		}
		out[norm.Name] = norm
	}
	return out, nil
}

// IsEnabled reports whether the server should start (default true).
func (c ServerConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// CommandArgv returns the full argv for a stdio server.
func (c ServerConfig) CommandArgv() []string {
	return commandArgv(c)
}

func commandArgv(cfg ServerConfig) []string {
	if len(cfg.CommandList) > 0 {
		var out []string
		for _, p := range cfg.CommandList {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	cmd := strings.TrimSpace(cfg.Command)
	if cmd == "" {
		return nil
	}
	out := []string{cmd}
	out = append(out, cfg.Args...)
	return out
}

func validServerName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	// Disallow pure numeric or leading/trailing dots that break namespacing readability.
	if name == "." || name == ".." {
		return false
	}
	return true
}
