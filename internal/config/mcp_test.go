package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMCPSection_RejectsRAG(t *testing.T) {
	// VAL-MCP-004: config schema rejects type: rag
	_, err := ValidateMCPSection(map[string]MCPServerConfig{
		"vector": {Type: "rag", BaseURL: "http://127.0.0.1:6333"},
	})
	if err == nil {
		t.Fatal("expected rag rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rag") {
		t.Fatalf("err=%v", err)
	}
}

func TestMCPConfigYAMLRoundTrip(t *testing.T) {
	// VAL-MCP-002: yaml round-trip for HTTP MCP shape (base_url, headers, auth).
	home := t.TempDir()
	raw := `# Inferenesia configuration
version: 1
mcp:
  echo:
    type: stdio
    command: /usr/bin/echo-mcp
    args: ["--stdio"]
  docs:
    type: http
    base_url: http://127.0.0.1:4115/mcp
    headers:
      Authorization: Bearer YOUR_TEST_TOKEN
    auth:
      type: bearer
      token_env: DOCS_TOKEN
`
	path := filepath.Join(home, ConfigFileName)
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCP) != 2 {
		t.Fatalf("mcp len=%d", len(cfg.MCP))
	}
	if cfg.MCP["docs"].BaseURL != "http://127.0.0.1:4115/mcp" {
		t.Fatalf("docs base=%q", cfg.MCP["docs"].BaseURL)
	}
	if cfg.MCP["docs"].Headers["Authorization"] != "Bearer YOUR_TEST_TOKEN" {
		t.Fatalf("headers not loaded")
	}
	if cfg.MCP["docs"].Auth == nil || cfg.MCP["docs"].Auth.TokenEnv != "DOCS_TOKEN" {
		t.Fatalf("auth=%+v", cfg.MCP["docs"].Auth)
	}

	norm, err := ValidateMCPSection(cfg.MCP)
	if err != nil {
		t.Fatal(err)
	}
	if norm["echo"].Type != "stdio" || norm["docs"].Type != "http" {
		t.Fatalf("norm=%+v", norm)
	}

	// Save and reload preserves shape (mode 0600 when secrets present).
	if err := SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o want 0600", info.Mode().Perm())
	}
	// Confirm no accidental log of header in file path API — just that value is still only on disk.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "base_url:") {
		t.Fatalf("saved yaml missing base_url:\n%s", data)
	}
}
