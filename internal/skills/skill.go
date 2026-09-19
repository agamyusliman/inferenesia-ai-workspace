// Package skills implements SKILL.md discovery with progressive load
// (metadata first; full body only on load) and session skill activation
// that observably changes the agent toolset and/or system prompt
// (VAL-ORCH-004). The multi-brain builtin skill implements Multi Brain
// read order + write rules (VAL-ORCH-005).
package skills

import (
	"fmt"
	"strings"
)

// Source identifies where a skill was discovered.
type Source string

const (
	SourceBuiltin  Source = "builtin"
	SourcePersonal Source = "personal" // ~/.inferenesia/skills
	SourceProject  Source = "project"  // <workspace>/.inferenesia/skills
	SourceClaude   Source = "claude"   // <workspace>/.claude/skills
	SourceAgents   Source = "agents"   // <workspace>/.agents/skills
	SourceExtra    Source = "extra"    // config.skills.paths
)

// Meta is discovery-time metadata only (~name + description). Bodies are
// not populated at list/discover time (progressive disclosure).
type Meta struct {
	// Name is the skill id (frontmatter name or directory name).
	Name string `json:"name"`
	// Description is the short frontmatter description.
	Description string `json:"description"`
	// Path is the absolute path to SKILL.md when on disk; empty for pure builtin.
	Path string `json:"path,omitempty"`
	// Source is personal/project/builtin/…
	Source Source `json:"source"`
	// DisableModelInvocation when true should not be auto-invoked by models.
	DisableModelInvocation bool `json:"disable_model_invocation,omitempty"`
	// UserInvocable when false hides from slash / skill tool (still listable for debug).
	UserInvocable bool `json:"user_invocable"`
}

// Skill is a fully loaded skill (body + tool policy).
type Skill struct {
	Meta
	// Body is the markdown procedure after the frontmatter.
	Body string `json:"body"`
	// AllowedTools, when non-empty, is the additional tool name set granted
	// while this skill is loaded (union with baseline registry tools).
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// MCPServers, when non-empty, is the set of MCP server specs that the
	// skill loader registers with the MCP host when this skill is loaded
	// (VAL-CROSS-012). Unloading the skill unregisters them so subsequent
	// task() calls cannot invoke the namespaced tools.
	//
	// Server names are rewritten to "<skill-name>:<server>" so two skills
	// cannot collide on the same MCP server name. Tools are then exposed
	// as mcp.<skill-name>:<server>.<tool> in the task tool namespace.
	MCPServers []MCPServerSpec `json:"mcp_servers,omitempty"`
	// RawFrontmatter is the unparsed YAML block for debugging.
	RawFrontmatter string `json:"-"`
	// FullText is the entire SKILL.md when loaded from disk (tests / preview).
	FullText string `json:"-"`
}

// MCPServerSpec is a minimal MCP server shape that a skill can declare in its
// frontmatter so loading the skill registers the namespaced tools and
// unloading unregisters them (VAL-CROSS-012).
//
// This mirrors the subset of internal/mcp.ServerConfig required by the skill
// loader. The skills package does NOT import internal/mcp (avoid cycles);
// internal/tools/skill.go translates MCPServerSpec into mcp.ServerConfig when
// registering on the MCP host.
type MCPServerSpec struct {
	// Name is the server key within the skill. The fully-qualified registry
	// name becomes "<skill-name>:<server>" to avoid collisions across skills.
	Name string `json:"name" yaml:"name"`
	// Type is "stdio" (alias "local") or "http" (alias "remote"). Required.
	Type string `json:"type" yaml:"type"`
	// Command is the stdio argv[0] (when Type=stdio).
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
	// Args are extra stdio argv (when Type=stdio).
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
	// CommandList is an alternate full-argv form for stdio.
	CommandList []string `json:"command_list,omitempty" yaml:"command_list,omitempty"`
	// BaseURL is the remote HTTP MCP endpoint (when Type=http). Accepts the
	// "url" alias via frontmatter.
	BaseURL string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	// Env is optional stdio environment (KEY=VALUE).
	Env []string `json:"env,omitempty" yaml:"env,omitempty"`
}

// PromptSection returns the system-prompt fragment injected when the skill is loaded.
func (s *Skill) PromptSection() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Skill: ")
	b.WriteString(s.Name)
	b.WriteString("\n\n")
	if d := strings.TrimSpace(s.Description); d != "" {
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	body := strings.TrimSpace(s.Body)
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	if len(s.AllowedTools) > 0 {
		b.WriteString("\nAllowed tools for this skill: ")
		b.WriteString(strings.Join(s.AllowedTools, ", "))
		b.WriteString("\n")
	}
	return b.String()
}

// String implements fmt.Stringer for CLI list lines.
func (m Meta) String() string {
	src := string(m.Source)
	if src == "" {
		src = "unknown"
	}
	desc := m.Description
	if desc == "" {
		desc = "(no description)"
	}
	return fmt.Sprintf("%s  [%s]  %s", m.Name, src, desc)
}

// QualifiedMCPServerName returns the registry-facing MCP server name a skill
// uses so two skills cannot collide on the same MCP host (VAL-CROSS-012).
// skillName is lower-cased and trimmed; serverName is preserved as-is after
// trimming. The host then namespaces tools as mcp.<qualified>.<tool>.
//
// The separator is "__" (double underscore) because the MCP host's
// validServerName allows [a-zA-Z0-9._-] but ParseNamespaced splits on the
// first '.', so a server name cannot contain '.'. Colon is also rejected by
// validServerName. Double underscore is safe and human-readable.
func QualifiedMCPServerName(skillName, serverName string) string {
	skillName = strings.ToLower(strings.TrimSpace(skillName))
	serverName = strings.TrimSpace(serverName)
	if skillName == "" {
		return serverName
	}
	if serverName == "" {
		return skillName
	}
	return skillName + "__" + serverName
}

// ValidateMCPServerSpec performs a minimal shape check on a skill-declared
// MCP server spec. It returns the normalized type ("stdio" or "http") or an
// error describing what is missing/unsupported. It does NOT attempt to start
// the server — that happens later on the MCP host.
func ValidateMCPServerSpec(spec MCPServerSpec) (normalizedType string, err error) {
	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name == "" {
		return "", fmt.Errorf("skills: mcp server name is required")
	}
	t := strings.ToLower(strings.TrimSpace(spec.Type))
	switch t {
	case "":
		return "", fmt.Errorf("skills: mcp server %q: type is required (stdio|http)", spec.Name)
	case "rag", "vector", "embedding", "embeddings":
		// Mirror internal/mcp hard reject (VAL-MCP-004) at the skill layer too.
		return "", fmt.Errorf("skills: mcp server %q: type %q is not supported (MCP host does not wire RAG/vector backends)", spec.Name, t)
	case "local", "stdio":
		t = "stdio"
	case "remote", "http":
		t = "http"
	default:
		return "", fmt.Errorf("skills: mcp server %q: unknown type %q (want stdio|http)", spec.Name, t)
	}
	switch t {
	case "stdio":
		argv := spec.commandArgv()
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			return "", fmt.Errorf("skills: mcp server %q: stdio requires command (or command_list)", spec.Name)
		}
	case "http":
		if strings.TrimSpace(spec.BaseURL) == "" {
			return "", fmt.Errorf("skills: mcp server %q: http requires base_url", spec.Name)
		}
	}
	return t, nil
}

// commandArgv returns the full argv for a stdio server spec.
func (spec MCPServerSpec) commandArgv() []string {
	if len(spec.CommandList) > 0 {
		var out []string
		for _, p := range spec.CommandList {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	cmd := strings.TrimSpace(spec.Command)
	if cmd == "" {
		return nil
	}
	out := []string{cmd}
	out = append(out, spec.Args...)
	return out
}
