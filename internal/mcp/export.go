package mcp

import "encoding/json"

// ToolDefinition is the public tool schema exported to tools.Registry (VAL-MCP-001/005).
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolDefinitions returns namespaced MCP tools for healthy servers only.
func (h *Host) ToolDefinitions() []ToolDefinition {
	raw := h.ListToolDefs()
	out := make([]ToolDefinition, 0, len(raw))
	for _, d := range raw {
		out = append(out, ToolDefinition(d))
	}
	return out
}
