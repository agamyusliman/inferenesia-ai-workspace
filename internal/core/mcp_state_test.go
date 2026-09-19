package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

func TestMCPDisabledServerRemainsVisibleAndReenables(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	server := buildEchoMCPServer(t)
	t.Setenv("PATH", t.TempDir())
	if err := config.UpsertMCPServer(home, "echo", config.MCPServerConfig{
		Name: "echo", Type: "stdio", Command: server,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReloadMCP(""); err != nil {
		t.Fatal(err)
	}

	before := requireMCPServerView(t, svc.GetMCPStatus(), "echo")
	if !before.Enabled || !before.OK || before.ToolCount != 2 {
		t.Fatalf("enabled status = %+v", before)
	}
	if err := svc.ToggleMCPServer("echo", false); err != nil {
		t.Fatal(err)
	}
	disabled := requireMCPServerView(t, svc.GetMCPStatus(), "echo")
	if disabled.Enabled || disabled.OK || disabled.ToolCount != 0 || len(disabled.ToolNames) != 0 {
		t.Fatalf("disabled status = %+v", disabled)
	}
	if got := svc.MCPHost(); got != nil {
		t.Fatal("disabled-only config must not create or start a host")
	}
	if err := svc.ToggleMCPServer("echo", true); err != nil {
		t.Fatal(err)
	}
	after := requireMCPServerView(t, svc.GetMCPStatus(), "echo")
	if !after.Enabled || !after.OK || after.ToolCount != 2 {
		t.Fatalf("re-enabled status = %+v", after)
	}
}

func TestMCPBuiltinTogglePersistsAndRemainsReversible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH fixture uses an executable shell script")
	}
	home := t.TempDir()
	bin := t.TempDir()
	ccc := filepath.Join(bin, "ccc")
	if err := os.WriteFile(ccc, []byte("#!/bin/sh\nexec \"$MCP_ECHO_SERVER\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MCP_ECHO_SERVER", buildEchoMCPServer(t))

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReloadMCP(""); err != nil {
		t.Fatal(err)
	}
	before := requireMCPServerView(t, svc.GetMCPStatus(), "cocoindex")
	if !before.Builtin || !before.Enabled || !before.OK {
		t.Fatalf("auto-detected builtin = %+v", before)
	}

	if err := svc.ToggleMCPServer("cocoindex", false); err != nil {
		t.Fatal(err)
	}
	disabled := requireMCPServerView(t, svc.GetMCPStatus(), "cocoindex")
	if disabled.Enabled || disabled.OK || disabled.ToolCount != 0 {
		t.Fatalf("disabled builtin = %+v", disabled)
	}
	persisted, err := config.LoadFromDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.MCP["cocoindex"].IsEnabled() {
		t.Fatal("builtin disabled state was not persisted")
	}

	if err := svc.ReloadMCP(""); err != nil {
		t.Fatal(err)
	}
	if requireMCPServerView(t, svc.GetMCPStatus(), "cocoindex").Enabled {
		t.Fatal("builtin became enabled after reload")
	}
	if err := svc.ToggleMCPServer("cocoindex", true); err != nil {
		t.Fatal(err)
	}
	after := requireMCPServerView(t, svc.GetMCPStatus(), "cocoindex")
	if !after.Enabled || !after.OK || after.ToolCount != 2 {
		t.Fatalf("re-enabled builtin = %+v", after)
	}
}

// An env-supplied server overrides every persistable layer, so Settings must
// mark it read-only and refuse the mutation rather than writing home config
// that the next reload silently discards.
func TestMCPEnvOverrideIsReadOnlyAndReportsLayer(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	server := buildEchoMCPServer(t)
	t.Setenv("PATH", t.TempDir())
	if err := config.UpsertMCPServer(home, "echo", config.MCPServerConfig{
		Name: "echo", Type: "stdio", Command: server,
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvMCP, "echo:\n  type: stdio\n  command: "+server+"\n")
	if err := svc.ReloadMCP(""); err != nil {
		t.Fatal(err)
	}

	view := requireMCPServerView(t, svc.GetMCPStatus(), "echo")
	if view.Layer != string(config.MCPServerLayerEnv) || !view.Managed {
		t.Fatalf("env server not reported as managed: %+v", view)
	}

	for _, err := range []error{
		svc.ToggleMCPServer("echo", false),
		svc.RemoveMCPServer("echo"),
	} {
		if err == nil || !strings.Contains(err.Error(), config.EnvMCP) {
			t.Fatalf("expected env override refusal naming %s, got %v", config.EnvMCP, err)
		}
	}
	if !requireMCPServerView(t, svc.GetMCPStatus(), "echo").Enabled {
		t.Fatal("refused mutation still changed reported state")
	}
	homeConfig, err := config.LoadFromDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if !homeConfig.MCP["echo"].IsEnabled() {
		t.Fatal("refused mutation wrote home config anyway")
	}
}

// Disabled servers are intentionally not running; counting them against
// healthy_count made a deliberate opt-out look like a broken server.
func TestMCPStatusCountsEnabledSeparatelyFromHealthy(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	server := buildEchoMCPServer(t)
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"echo", "echo-off"} {
		if err := config.UpsertMCPServer(home, name, config.MCPServerConfig{
			Name: name, Type: "stdio", Command: server,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.ReloadMCP(""); err != nil {
		t.Fatal(err)
	}
	if err := svc.ToggleMCPServer("echo-off", false); err != nil {
		t.Fatal(err)
	}

	status := svc.GetMCPStatus()
	if status.TotalServers != 2 || status.EnabledCount != 1 || status.HealthyCount != 1 {
		t.Fatalf("counts = total %d, enabled %d, healthy %d; want 2/1/1",
			status.TotalServers, status.EnabledCount, status.HealthyCount)
	}
}

func requireMCPServerView(t *testing.T, status MCPStatusView, name string) MCPServerView {
	t.Helper()
	for _, server := range status.Servers {
		if server.Name == name {
			return server
		}
	}
	t.Fatalf("server %q missing from status: %+v", name, status)
	return MCPServerView{}
}

func buildEchoMCPServer(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "echo_mcp_server")
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", out, "./testdata/echo_server.go")
	cmd.Dir = filepath.Join(pkgDir, "..", "mcp")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build echo MCP server: %v\n%s", err, output)
	}
	return out
}
