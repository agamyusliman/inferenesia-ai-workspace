package mcp

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateServer_StdioAndHTTP(t *testing.T) {
	stdio, err := ValidateServer(ServerConfig{
		Name:    "echo",
		Type:    "local",
		Command: "/bin/true",
		Args:    []string{"--mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdio.Type != TypeStdio {
		t.Fatalf("type=%q", stdio.Type)
	}
	if stdio.Command != "/bin/true" || len(stdio.Args) != 1 {
		t.Fatalf("argv=%q %v", stdio.Command, stdio.Args)
	}

	http, err := ValidateServer(ServerConfig{
		Name:    "docs",
		Type:    "remote",
		BaseURL: "http://127.0.0.1:4115/mcp",
		Headers: map[string]string{"Authorization": "Bearer YOUR_TEST_TOKEN"},
		Auth:    &AuthConfig{Type: "bearer", TokenEnv: "MY_TOKEN"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if http.Type != TypeHTTP {
		t.Fatalf("type=%q", http.Type)
	}
	if http.BaseURL != "http://127.0.0.1:4115/mcp" {
		t.Fatalf("base=%q", http.BaseURL)
	}
	if http.Headers["Authorization"] != "Bearer YOUR_TEST_TOKEN" {
		t.Fatal("headers must round-trip (but never be logged)")
	}
}

func TestValidateServer_RejectsRAGType(t *testing.T) {
	// VAL-MCP-004: config schema rejects type: rag (and vector/embedding aliases).
	_, err := ValidateServer(ServerConfig{
		Name: "memory-backend",
		Type: "rag",
		URL:  "http://localhost:6333",
	})
	if err == nil {
		t.Fatal("expected error for type:rag")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "rag") || !strings.Contains(low, "not supported") {
		t.Fatalf("error should reject rag clearly: %v", err)
	}
	for _, bad := range []string{"vector", "embedding", "embeddings"} {
		_, err := ValidateServer(ServerConfig{Name: "x", Type: bad})
		if err == nil {
			t.Fatalf("type %q should be rejected", bad)
		}
	}
}

func TestValidateServer_YAMLRoundTrip(t *testing.T) {
	// VAL-MCP-002: config schema test (yaml round-trip)
	raw := `
mcp:
  echo:
    type: stdio
    command: /usr/bin/echo-mcp
    args: ["--stdio"]
  context7:
    type: http
    base_url: http://127.0.0.1:4118/mcp
    headers:
      X-Api-Key: YOUR_API_KEY_HERE
    auth:
      type: bearer
      token_env: YOUR_TOKEN_ENV_NAME
`
	var wrap struct {
		MCP map[string]ServerConfig `yaml:"mcp"`
	}
	if err := yaml.Unmarshal([]byte(raw), &wrap); err != nil {
		t.Fatal(err)
	}
	norm, err := ValidateServers(wrap.MCP)
	if err != nil {
		t.Fatal(err)
	}
	if len(norm) != 2 {
		t.Fatalf("len=%d", len(norm))
	}
	if norm["echo"].Type != TypeStdio || norm["echo"].Command != "/usr/bin/echo-mcp" {
		t.Fatalf("echo=%+v", norm["echo"])
	}
	if norm["context7"].Type != TypeHTTP || norm["context7"].BaseURL == "" {
		t.Fatalf("context7=%+v", norm["context7"])
	}
	// Marshal back without dropping auth shape.
	out, err := yaml.Marshal(map[string]any{"mcp": norm})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "base_url:") {
		t.Fatalf("round-trip missing base_url:\n%s", out)
	}
	if !strings.Contains(string(out), "token_env:") {
		t.Fatalf("round-trip missing auth.token_env:\n%s", out)
	}
}

func TestValidateServer_RequiresCommandOrBaseURL(t *testing.T) {
	_, err := ValidateServer(ServerConfig{Name: "a", Type: "stdio"})
	if err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("stdio without command: %v", err)
	}
	_, err = ValidateServer(ServerConfig{Name: "b", Type: "http"})
	if err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("http without base_url: %v", err)
	}
}

func TestNamespaceToolIsolation(t *testing.T) {
	// VAL-MCP-005
	ns := NamespaceTool("myserver", "fs.read")
	if ns != "mcp.myserver.fs.read" {
		t.Fatalf("ns=%q", ns)
	}
	if ns == "fs.read" || ns == "read_file" {
		t.Fatal("must not equal built-in names")
	}
	server, tool, ok := ParseNamespaced(ns)
	if !ok || server != "myserver" || tool != "fs.read" {
		t.Fatalf("parse %q → %q %q ok=%v", ns, server, tool, ok)
	}
	if IsNamespaced("read_file") {
		t.Fatal("built-in must not be namespaced")
	}
}

func TestFormatUnavailable(t *testing.T) {
	// VAL-MCP-003 message shape
	msg := FormatUnavailable("echo", "connection refused")
	if !strings.Contains(msg, `server "echo"`) || !strings.Contains(msg, "unavailable:") {
		t.Fatalf("msg=%q", msg)
	}
}
