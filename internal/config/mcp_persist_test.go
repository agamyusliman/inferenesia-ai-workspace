package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-MCP-007: MCP server configs persist across sessions; mode 0600 when auth present.
func TestMCP_PersistenceRoundTripAndMode0600(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}

	cfg := MCPServerConfig{
		Name:    "docs",
		Type:    "http",
		BaseURL: "http://127.0.0.1:4115/mcp",
		Headers: map[string]string{"Authorization": "Bearer persist-secret-token"},
		Auth:    &MCPAuthConfig{Type: "bearer", TokenEnv: "DOCS_TOKEN"},
	}
	if err := UpsertMCPServer(home, "docs", cfg); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	path := filepath.Join(home, ConfigFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o want 0600 when auth present", info.Mode().Perm())
	}
	if !HasMCPAuth(map[string]MCPServerConfig{"docs": cfg}) {
		t.Fatal("HasMCPAuth should be true")
	}

	// Simulate new session: fresh load from disk.
	loaded, err := LoadMCPServers(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded=%d", len(loaded))
	}
	got := loaded["docs"]
	if got.Type != "http" || got.BaseURL != "http://127.0.0.1:4115/mcp" {
		t.Fatalf("loaded docs=%+v", got)
	}
	if got.Headers["Authorization"] != "Bearer persist-secret-token" {
		t.Fatalf("headers not persisted: %+v", got.Headers)
	}
	if got.Auth == nil || got.Auth.TokenEnv != "DOCS_TOKEN" {
		t.Fatalf("auth=%+v", got.Auth)
	}

	// Upsert second server + remove first — persistence across ops.
	if err := UpsertMCPServer(home, "echo", MCPServerConfig{
		Name: "echo", Type: "stdio", Command: "/usr/bin/true",
	}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveMCPServer(home, "docs"); err != nil {
		t.Fatal(err)
	}
	after, err := LoadMCPServers(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after["docs"]; ok {
		t.Fatal("docs should be removed")
	}
	if after["echo"].Type != "stdio" {
		t.Fatalf("echo=%+v", after["echo"])
	}
}

// VAL-MCP-007: env override > project config > home config > defaults.
func TestMCP_HierarchyEnvProjectHome(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}

	// Home layer: server "a" and "b".
	if err := SaveMCPServers(home, map[string]MCPServerConfig{
		"a": {Name: "a", Type: "stdio", Command: "/home/a"},
		"b": {Name: "b", Type: "stdio", Command: "/home/b"},
	}); err != nil {
		t.Fatal(err)
	}

	// Project overlays "b", adds "c".
	if err := SaveMCPServersToProject(ws, map[string]MCPServerConfig{
		"b": {Name: "b", Type: "stdio", Command: "/proj/b"},
		"c": {Name: "c", Type: "http", BaseURL: "http://127.0.0.1:4116/mcp"},
	}); err != nil {
		t.Fatal(err)
	}
	projPath := filepath.Join(ws, ProjectConfigDirName, ConfigFileName)
	info, err := os.Stat(projPath)
	if err != nil {
		t.Fatal(err)
	}
	// Project without auth may still be 0600 via SaveFile; ensure writable only by owner if secrets.
	_ = info

	// Env overlays "a" and adds "d".
	envYAML := `
a:
  type: stdio
  command: /env/a
d:
  type: http
  base_url: http://127.0.0.1:4118/mcp
  headers:
    Authorization: Bearer env-token
`
	t.Setenv(EnvMCP, envYAML)

	merged, err := LoadMCPServers(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	// a from env
	if merged["a"].Command != "/env/a" {
		t.Fatalf("a command=%q want /env/a (env wins)", merged["a"].Command)
	}
	// b from project (over home)
	if merged["b"].Command != "/proj/b" {
		t.Fatalf("b command=%q want /proj/b", merged["b"].Command)
	}
	// c from project
	if merged["c"].Type != "http" {
		t.Fatalf("c=%+v", merged["c"])
	}
	// d from env
	if merged["d"].BaseURL != "http://127.0.0.1:4118/mcp" {
		t.Fatalf("d=%+v", merged["d"])
	}
	if !HasMCPAuth(merged) {
		t.Fatal("env headers should mark HasMCPAuth")
	}

	// Without env, project still overlays home.
	t.Setenv(EnvMCP, "")
	noEnv, err := LoadMCPServers(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if noEnv["a"].Command != "/home/a" {
		t.Fatalf("a without env=%q", noEnv["a"].Command)
	}
	if noEnv["b"].Command != "/proj/b" {
		t.Fatalf("b without env=%q", noEnv["b"].Command)
	}
}

func TestMCP_EnvWrapperForm(t *testing.T) {
	t.Setenv(EnvMCP, `
mcp:
  wrap:
    type: stdio
    command: /bin/wrap
`)
	m, err := MCPServersFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if m["wrap"].Command != "/bin/wrap" {
		t.Fatalf("got=%+v", m)
	}
}

func TestMCP_SaveMCPServersRejectsRAG(t *testing.T) {
	home := t.TempDir()
	err := SaveMCPServers(home, map[string]MCPServerConfig{
		"bad": {Type: "rag", BaseURL: "http://127.0.0.1:6333"},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "rag") {
		t.Fatalf("err=%v", err)
	}
}

func TestSetMCPServerEnabledUpdatesEffectiveProjectLayerOnly(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}
	if err := SaveMCPServers(home, map[string]MCPServerConfig{
		"shared": {Name: "shared", Type: "stdio", Command: "/home/shared"},
		"other":  {Name: "other", Type: "stdio", Command: "/home/other"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveMCPServersToProject(ws, map[string]MCPServerConfig{
		"shared": {Name: "shared", Type: "stdio", Command: "/project/shared"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := SetMCPServerEnabled(home, ws, "shared", false); err != nil {
		t.Fatal(err)
	}
	effective, err := LoadMCPServers(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if effective["shared"].IsEnabled() {
		t.Fatal("effective project server remained enabled")
	}
	if effective["shared"].Command != "/project/shared" {
		t.Fatalf("effective command = %q, want project value", effective["shared"].Command)
	}
	homeConfig, err := LoadFromDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if !homeConfig.MCP["shared"].IsEnabled() {
		t.Fatal("lower-precedence home server was mutated")
	}
	if homeConfig.MCP["other"].Command != "/home/other" {
		t.Fatal("unrelated home MCP config was mutated")
	}
}

func TestSetMCPServerEnabledRejectsEnvironmentOverride(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}
	if err := SaveMCPServers(home, map[string]MCPServerConfig{
		"shared": {Name: "shared", Type: "stdio", Command: "/home/shared"},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvMCP, "shared:\n  type: stdio\n  command: /env/shared\n")

	if err := SetMCPServerEnabled(home, "", "shared", false); err == nil || !strings.Contains(err.Error(), EnvMCP) {
		t.Fatalf("expected read-only env override error, got %v", err)
	}
	homeConfig, err := LoadFromDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if !homeConfig.MCP["shared"].IsEnabled() {
		t.Fatal("home config changed even though env still controls effective state")
	}
}

// A delete that matches nothing must report it: Settings otherwise renders a
// success the user cannot observe and the server stays in the list.
func TestRemoveMCPServerReportsMissingInsteadOfSilentSuccess(t *testing.T) {
	home := t.TempDir()

	if err := RemoveMCPServer(home, "ghost"); err == nil {
		t.Fatal("removing from an empty config reported success")
	}

	if err := UpsertMCPServer(home, "real", MCPServerConfig{
		Type:    "stdio",
		Command: "echo",
	}); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if err := RemoveMCPServer(home, "ghost"); err == nil {
		t.Fatal("removing an unknown name reported success")
	}
	if err := RemoveMCPServer(home, "real"); err != nil {
		t.Fatalf("removing an existing server failed: %v", err)
	}
}
