package mcp

import (
	"fmt"
	"strings"
)

// NamespacePrefix is the fixed agent-facing prefix for all MCP tools (VAL-MCP-005).
const NamespacePrefix = "mcp."

// NamespaceTool builds the namespaced tool name: mcp.<server>.<tool>.
// Tool names may contain dots (e.g. fs.read → mcp.myserver.fs.read).
// The namespaced form never collides with built-in tools such as read_file / shell.
func NamespaceTool(serverName, toolName string) string {
	serverName = strings.TrimSpace(serverName)
	toolName = strings.TrimSpace(toolName)
	return NamespacePrefix + serverName + "." + toolName
}

// ParseNamespaced splits mcp.<server>.<tool> into server and raw tool name.
// Returns ok=false when the name is not an MCP namespaced tool.
func ParseNamespaced(full string) (server, tool string, ok bool) {
	full = strings.TrimSpace(full)
	if !strings.HasPrefix(full, NamespacePrefix) {
		return "", "", false
	}
	rest := full[len(NamespacePrefix):]
	// server is the first segment; tool is the remainder (may contain dots).
	i := strings.IndexByte(rest, '.')
	if i <= 0 || i >= len(rest)-1 {
		return "", "", false
	}
	server = rest[:i]
	tool = rest[i+1:]
	if server == "" || tool == "" {
		return "", "", false
	}
	return server, tool, true
}

// IsNamespaced reports whether name uses the MCP agent namespace.
func IsNamespaced(name string) bool {
	_, _, ok := ParseNamespaced(name)
	return ok
}

// FormatUnavailable is the canonical clear error for a down/unauth MCP server (VAL-MCP-003).
func FormatUnavailable(serverName, reason string) string {
	serverName = strings.TrimSpace(serverName)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unavailable"
	}
	return fmt.Sprintf(`mcp: server %q unavailable: %s`, serverName, reason)
}
