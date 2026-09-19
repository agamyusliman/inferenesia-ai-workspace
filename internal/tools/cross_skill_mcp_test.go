package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CROSS-012: skill load grants MCP tools (orchestration + skills + MCP).
//
// Cross-area flow under test (internal/skills + internal/mcp + internal/tools
// + internal/orchestrator):
//
//	fixture stdio MCP server (echo) is declared in a skill's frontmatter
//	    `mcp-servers:` block
//	→ `skill load <x>` registers mcp.<skill>__<server>.<tool> on the MCP host
//	    (namespaced tools appear in the task tool namespace)
//	→ `task categories` / tool list includes the namespaced tool
//	→ a sub-task registry (explore category filter) advertises + can call it
//	→ `skill unload <x>` unregisters the server
//	→ subsequent task() sub-task call to the tool returns unknown-tool error
//
// This exercises ALL expected behaviors of the feature:
//   - skill load registers MCP-served tools scoped to that skill
//   - the namespaced tools appear in the task tool namespace for task() calls
//   - skill unload unregisters them
//   - subsequent task() cannot call them (unknown-tool error)
func TestCrossArea_SkillLoadGrantsMCPTools(t *testing.T) {
	root := t.TempDir()
	bin := buildCross012EchoServer(t)

	// --- Write a skill SKILL.md with an mcp-servers frontmatter block. ---
	// The skill declares a stdio MCP server pointing at the echo fixture.
	// Server name "echo" becomes qualified as "cross012-skill__echo" so two
	// skills cannot collide on the MCP host. The echo server exposes `echo`
	// and `fs.read`; the agent-facing namespaced tool is
	// mcp.cross012-skill__echo.echo.
	skillName := "cross012-skill"
	skillDir := filepath.Join(root, ".yura-ai", "skills", skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := fmt.Sprintf(`---
name: %s
description: VAL-CROSS-012 fixture skill that grants an MCP-served echo tool
allowed-tools: []
mcp-servers:
  - name: echo
    type: stdio
    command: %q
---

# %s

Loading this skill registers an MCP-served echo tool scoped to this skill.
The namespaced tool appears as mcp.%s__echo.echo in the task tool namespace.
`, skillName, bin, skillName, skillName)
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o600); err != nil {
		t.Fatal(err)
	}

	// --- Build the parent registry: WriteGateway + skill backend + MCP host
	// + skill MCP registrar + task backend. ---
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatalf("writegate.New: %v", err)
	}
	parentReg := tools.NewRegistry(gw)

	// Skill backend discovers the disk skill we just wrote.
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	parentReg.SetSkillBackend(tools.NewSkillBackend(loader))

	// MCP host + skill MCP registrar (the same host the agent loop uses).
	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	parentReg.SetMCPHost(h)
	parentReg.SetSkillMCPRegistrar(tools.NewSkillMCPHostRegistrar(h))

	// Task backend so we can drive task() and build sub-task registries.
	mgr := orchestrator.New(orchestrator.Options{})

	// subTaskToolCalls records every tool call the sub-task makes so we can
	// assert the MCP tool was callable after skill load and returned
	// unknown-tool after skill unload.
	var (
		logMu sync.Mutex
		logs  []cross012SubTaskToolCall
	)
	recordCall := func(name string, isError bool, content string) {
		logMu.Lock()
		defer logMu.Unlock()
		logs = append(logs, cross012SubTaskToolCall{Name: name, IsError: isError, Content: content})
	}

	// subRunner builds a CATEGORY-FILTERED (explore) sub-task registry that
	// shares the MCP host (so namespaced tools are visible) and attempts to
	// call the skill-granted MCP tool. The explore category filter must NOT
	// deny mcp.* namespaced tools (VAL-CROSS-012).
	//
	// The runner attempts the MCP call REGARDLESS of whether the tool is
	// currently advertised. After skill load it succeeds; after skill unload
	// the Execute path returns an unknown-tool error (the host no longer has
	// the server). The runner records every attempt so the test body can
	// assert on the captured log rather than failing the sub-task.
	subRunner := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		subReg := tools.NewRegistry(gw)
		subReg.SetMCPHost(h) // share the same host so the skill-granted tool is visible
		subReg.SetCategoryFilter(orchestrator.CategoryExplore)

		toolName := "mcp." + skillName + "__echo.echo"

		// Snapshot whether the tool is advertised (for the summary).
		advertised := false
		for _, d := range subReg.Definitions() {
			if d.Function.Name == toolName {
				advertised = true
				break
			}
		}

		// Attempt the MCP call. After skill load this succeeds; after skill
		// unload the host returns unknown-tool (the server was unregistered).
		res := subReg.Execute(ctx, tools.Call{
			ID:        "sub-mcp-call",
			Name:      toolName,
			Arguments: `{"message":"cross012-ping"}`,
		}, nil)
		recordCall(toolName, res.IsError, res.Content)

		status := "ok"
		if res.IsError {
			status = "error: " + res.Content
		}
		summary := fmt.Sprintf("mcp call advertised=%v status=%s", advertised, status)
		return summary, []string{summary}, nil
	}
	adapter := tools.NewTaskAdapter(mgr, subRunner)
	parentReg.SetTaskBackend(adapter)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Step 1: BEFORE skill load, the MCP tool is NOT registered. ---
	toolName := "mcp." + skillName + "__echo.echo"
	if parentReg.MCPHost().HasNamespacedTool(toolName) {
		t.Fatalf("MCP tool %q must NOT be registered before skill load", toolName)
	}
	for _, d := range parentReg.Definitions() {
		if d.Function.Name == toolName {
			t.Fatalf("MCP tool %q must NOT be advertised before skill load", toolName)
		}
	}

	// --- Step 2: `skill load <x>` registers the MCP-served tool. ---
	loadArgs, _ := json.Marshal(map[string]any{
		"action": "load",
		"name":   skillName,
	})
	loadRes := parentReg.Execute(ctx, tools.Call{
		ID:        "skill-load",
		Name:      tools.NameSkill,
		Arguments: string(loadArgs),
	}, nil)
	if loadRes.IsError {
		t.Fatalf("skill load error: %s", loadRes.Content)
	}
	// The load result must mention the registered MCP server.
	if !strings.Contains(loadRes.Content, "mcp_servers: registered") {
		t.Fatalf("skill load result should mention registered mcp servers: %s", loadRes.Content)
	}
	if !strings.Contains(loadRes.Content, skillName+"__echo") {
		t.Fatalf("skill load result should mention qualified server %s__echo: %s", skillName, loadRes.Content)
	}

	// The namespaced tool must now be registered on the host and advertised.
	if !parentReg.MCPHost().HasNamespacedTool(toolName) {
		t.Fatalf("MCP tool %q must be registered after skill load", toolName)
	}
	advertisedAfterLoad := false
	for _, d := range parentReg.Definitions() {
		if d.Function.Name == toolName {
			advertisedAfterLoad = true
			break
		}
	}
	if !advertisedAfterLoad {
		t.Fatalf("MCP tool %q must be advertised after skill load", toolName)
	}

	// --- Step 3: `task categories` / tool list includes the namespaced tool. ---
	// The task tool's `categories` action lists categories; the MCP tool is
	// not category-bound, but the parent registry's Definitions() (which the
	// agent sees as the tool list) must include it. We already asserted that
	// above; here we also verify the task tool itself is wired.
	catArgs, _ := json.Marshal(map[string]any{"action": "categories"})
	catRes := parentReg.Execute(ctx, tools.Call{
		ID:        "task-cats",
		Name:      tools.NameTask,
		Arguments: string(catArgs),
	}, nil)
	if catRes.IsError {
		t.Fatalf("task categories error: %s", catRes.Content)
	}
	if !strings.Contains(catRes.Content, "explore") {
		t.Fatalf("task categories should list explore: %s", catRes.Content)
	}

	// --- Step 4: a sub-task (task()) CAN call the MCP tool after skill load. ---
	taskArgs, _ := json.Marshal(map[string]any{
		"prompt":   "Call the skill-granted MCP echo tool",
		"category": orchestrator.CategoryExplore,
	})
	taskRes := parentReg.Execute(ctx, tools.Call{
		ID:        "parent-task-after-load",
		Name:      tools.NameTask,
		Arguments: string(taskArgs),
	}, nil)
	if taskRes.IsError {
		t.Fatalf("task() after skill load failed: %s", taskRes.Content)
	}
	// Verify the sub-task tool-call log recorded a successful MCP call.
	logMu.Lock()
	capturedAfterLoad := append([]cross012SubTaskToolCall(nil), logs...)
	logs = nil
	logMu.Unlock()
	foundMCPSuccess := false
	for _, c := range capturedAfterLoad {
		if c.Name == toolName && !c.IsError {
			foundMCPSuccess = true
			// The echo fixture returns "echo:<message>".
			if !strings.Contains(c.Content, "echo:cross012-ping") {
				t.Fatalf("sub-task MCP call returned %q (want echo:cross012-ping)", c.Content)
			}
		}
	}
	if !foundMCPSuccess {
		t.Fatalf("sub-task tool-call log must contain a successful %s call after skill load: %+v", toolName, capturedAfterLoad)
	}

	// --- Step 5: `skill unload <x>` unregisters the MCP server. ---
	unloadArgs, _ := json.Marshal(map[string]any{
		"action": "unload",
		"name":   skillName,
	})
	unloadRes := parentReg.Execute(ctx, tools.Call{
		ID:        "skill-unload",
		Name:      tools.NameSkill,
		Arguments: string(unloadArgs),
	}, nil)
	if unloadRes.IsError {
		t.Fatalf("skill unload error: %s", unloadRes.Content)
	}
	if !strings.Contains(unloadRes.Content, "unregistered mcp servers") {
		t.Fatalf("skill unload result should mention unregistered mcp servers: %s", unloadRes.Content)
	}
	// The namespaced tool must be gone from the host and from Definitions().
	if parentReg.MCPHost().HasNamespacedTool(toolName) {
		t.Fatalf("MCP tool %q must be unregistered after skill unload", toolName)
	}
	for _, d := range parentReg.Definitions() {
		if d.Function.Name == toolName {
			t.Fatalf("MCP tool %q must NOT be advertised after skill unload", toolName)
		}
	}

	// --- Step 6: subsequent task() cannot call it (unknown-tool error). ---
	// The sub-task runner now attempts the MCP call again; it MUST receive an
	// unknown-tool / unavailable error because the server was unregistered.
	taskRes2 := parentReg.Execute(ctx, tools.Call{
		ID:        "parent-task-after-unload",
		Name:      tools.NameTask,
		Arguments: string(taskArgs),
	}, nil)
	// task() itself does not fail (the sub-task body ran); the MCP CALL inside
	// the sub-task must have errored. Inspect the captured log.
	logMu.Lock()
	capturedAfterUnload := append([]cross012SubTaskToolCall(nil), logs...)
	logs = nil
	logMu.Unlock()
	_ = taskRes2 // task() result is non-fatal; the assertion is on the MCP call

	foundMCPError := false
	for _, c := range capturedAfterUnload {
		if c.Name == toolName && c.IsError {
			foundMCPError = true
			// The error content must indicate unknown-tool / unavailable.
			low := strings.ToLower(c.Content)
			if !strings.Contains(low, "unknown") && !strings.Contains(low, "unavailable") {
				t.Fatalf("post-unload MCP call error should mention unknown/unavailable: %s", c.Content)
			}
		}
	}
	if !foundMCPError {
		t.Fatalf("sub-task tool-call log must contain an errored %s call after skill unload: %+v", toolName, capturedAfterUnload)
	}
}

// cross012SubTaskToolCall is one recorded tool invocation by the sub-task.
type cross012SubTaskToolCall struct {
	Name    string
	IsError bool
	Content string
}

// buildCross012EchoServer compiles the fixture echo MCP stdio server into a
// temp binary for the VAL-CROSS-012 integration test. The fixture exposes
// `echo` and `fs.read` tools.
func buildCross012EchoServer(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "echo_mcp_server_cross012")
	// Resolve the mcp package testdata dir from the tools package dir.
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mcpDir := filepath.Join(pkgDir, "..", "mcp")
	cmd := exec.Command("go", "build", "-o", out, "./testdata/echo_server.go")
	cmd.Dir = mcpDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	outb, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build echo MCP server: %v\n%s", err, outb)
	}
	return out
}
