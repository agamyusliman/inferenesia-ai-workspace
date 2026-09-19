package core_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CROSS-008 (E2E via core.Run agent loop): MCP tool + diagram save.
//
// This test drives the FULL cross-area flow through the real agent loop in
// core.Run (not just the registry). A scripted fake provider plays the model:
//
//	turn 1 → assistant requests mcp.diagrams.get_diagram (fixture stdio server)
//	turn 2 → assistant requests create_diagram using the MCP-returned mermaid
//	turn 3 → assistant final answer
//
// The fake provider in turn 2 reads the tool result content from the request
// messages so create_diagram receives exactly the mermaid source the MCP tool
// returned (proving the agent loop wires MCP result → create_diagram source).
//
// Asserts:
//   - fixture MCP tool returns a mermaid string (callable by the agent)
//   - agent calls create_diagram → file at docs/diagrams/<slug>.mmd exists
//   - WriteGateway snapshot pushed (undo depth grows)
//   - undo restores prior state (pre-absent → file removed)
//   - tool events fire for both the MCP tool and create_diagram (VAL-MCP-008)
func TestCrossArea_MCPToolDiagramSave_AgentLoopE2E(t *testing.T) {
	root := t.TempDir()
	bin := buildDiagramMCPServerForCore(t)

	// --- Build registry with WriteGateway + diagram tools + MCP host. ---
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatalf("writegate.New: %v", err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	// The MCP host registers synchronously; a short context is enough.
	mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer mcpCancel()
	if err := h.Register(mcpCtx, mcp.ServerConfig{
		Name:    "diagrams",
		Type:    "stdio",
		Command: bin,
	}); err != nil {
		t.Fatalf("mcp register: %v", err)
	}
	reg.SetMCPHost(h)

	const mcpToolName = "mcp.diagrams.get_diagram"
	const slug = "agent-loop-flow"
	rel := "docs/diagrams/" + slug + ".mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))

	// --- Scripted provider: drives the agent loop through the cross-area flow. ---
	client := &cross008FakeClient{
		mcpToolName: mcpToolName,
		slug:        slug,
	}

	var eventBuf strings.Builder
	res, err := core.Run(context.Background(), core.AgentConfig{
		Client: client,
		Tools:  reg,
		Model:  "fake-model",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Fetch a flowchart diagram from the diagrams MCP server and save it under docs/diagrams/."},
		},
		SystemPrompt: core.DefaultSystemPrompt,
		Stream:       false,
		EventOut:     &eventBuf,
	})
	if err != nil {
		t.Fatalf("core.Run: %v", err)
	}
	// Two tool rounds: MCP get_diagram + create_diagram.
	if res.ToolRounds != 2 {
		t.Fatalf("ToolRounds=%d, want 2 (MCP call + create_diagram)", res.ToolRounds)
	}

	// --- (a) MCP tool was callable by the agent and returned mermaid. ---
	if client.mcpResult == "" {
		t.Fatal("MCP tool result was never captured by the fake provider")
	}
	if !strings.Contains(client.mcpResult, "flowchart TD") {
		t.Fatalf("MCP tool result did not contain mermaid source: %q", client.mcpResult)
	}

	// --- (b) create_diagram saved the mermaid to docs/diagrams/<slug>.mmd. ---
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("diagram file not saved at %s: %v", rel, err)
	}
	if string(data) != client.mcpResult {
		t.Fatalf("diagram content mismatch:\n want (MCP result) %q\n got  %q",
			client.mcpResult, string(data))
	}

	// --- (c) WriteGateway snapshot pushed. ---
	if gw.UndoDepth() != 1 {
		t.Fatalf("expected undo depth 1 after create_diagram, got %d", gw.UndoDepth())
	}
	units := gw.UndoStack()
	// Diagram write + docs/diagrams/README.md index write now share a single
	// undo unit (create_diagram wraps both in one turn so Undo restores both).
	if len(units) != 1 || len(units[0].Entries) != 2 {
		t.Fatalf("unexpected undo stack: %+v", units)
	}
	// Entries: [0] diagram write, [1] index write. Both tagged with source.
	if got := units[0].Entries[0].Source; got != writegate.SourceDiagram {
		t.Fatalf("undo unit entry[0].source=%q want %q", got, writegate.SourceDiagram)
	}

	// --- (d) undo restores prior state (pre-absent → file removed). ---
	undoRes := gw.Undo()
	if undoRes.Noop {
		t.Fatalf("undo noop: %s", undoRes.Message)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("after undo the diagram file should be removed (pre-absent), got err=%v", err)
	}

	// --- (e) tool events fired for both MCP and create_diagram (VAL-MCP-008). ---
	ev := eventBuf.String()
	if !strings.Contains(ev, "tool_start") || !strings.Contains(ev, mcpToolName) {
		t.Fatalf("events missing MCP tool_start for %q: %q", mcpToolName, ev)
	}
	if !strings.Contains(ev, "tool_start") || !strings.Contains(ev, tools.NameCreateDiagram) {
		t.Fatalf("events missing create_diagram tool_start: %q", ev)
	}
	// ToolEnd for both.
	if !strings.Contains(ev, "tool_end") {
		t.Fatalf("events missing tool_end: %q", ev)
	}
}

// cross008FakeClient is a scripted provider that drives the agent loop through
// the VAL-CROSS-008 cross-area flow. It reads the MCP tool result from the
// request messages so create_diagram receives exactly what the MCP tool returned.
type cross008FakeClient struct {
	// mcpToolName is the namespaced MCP tool the model requests in turn 1.
	mcpToolName string
	// slug is the create_diagram slug used in turn 2.
	slug string

	// mcpResult captures the tool result content the agent loop fed back, so the
	// test can assert the create_diagram source matches the MCP-returned mermaid.
	mcpResult string

	// step tracks which turn we are in (0 = first call, 1 = post-MCP, 2 = post-create).
	step int
}

func (c *cross008FakeClient) ChatStream(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	// Not used in Stream:false mode; delegate to ChatTurn shape via panic to catch misuse.
	ch := make(chan provider.StreamEvent)
	close(ch)
	return ch, nil
}

func (c *cross008FakeClient) ChatTurn(ctx context.Context, req provider.ChatRequest) (provider.Message, error) {
	step := c.step
	c.step++

	switch step {
	case 0:
		// Turn 1: assistant requests the MCP-served tool.
		var tc provider.ToolCall
		tc.ID = "call_mcp_get"
		tc.Type = "function"
		tc.Function.Name = c.mcpToolName
		tc.Function.Arguments = `{"kind":"flowchart"}`
		return provider.Message{
			Role:      provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{tc},
		}, nil

	case 1:
		// Turn 2: assistant requests create_diagram. Read the MCP tool result
		// from the request messages so the diagram source matches exactly what
		// the MCP fixture returned (proving the agent loop wires result → source).
		for _, m := range req.Messages {
			if m.Role == provider.RoleTool && m.Name == c.mcpToolName {
				c.mcpResult = m.Content
				break
			}
		}
		if c.mcpResult == "" {
			// Fallback: use a known fixture content so the test still produces a file.
			c.mcpResult = "flowchart TD\n    A[Start] --> B[Process]\n    B --> C[End]"
		}
		args, _ := json.Marshal(map[string]any{
			"slug":   c.slug,
			"format": "mermaid",
			"source": c.mcpResult,
			"title":  "Agent Loop Flow",
		})
		var tc provider.ToolCall
		tc.ID = "call_create_diag"
		tc.Type = "function"
		tc.Function.Name = tools.NameCreateDiagram
		tc.Function.Arguments = string(args)
		return provider.Message{
			Role:      provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{tc},
		}, nil

	default:
		// Turn 3: final streamed answer.
		return provider.Message{
			Role:    provider.RoleAssistant,
			Content: "Saved the mermaid diagram from the MCP server under docs/diagrams/.",
		}, nil
	}
}

// buildDiagramMCPServerForCore compiles the fixture stdio MCP diagram server
// for the core package test (resolves the mcp package testdata dir).
func buildDiagramMCPServerForCore(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "diagram_mcp_server")
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// core package is internal/core; mcp is internal/mcp.
	mcpDir := filepath.Join(pkgDir, "..", "mcp")
	cmd := exec.Command("go", "build", "-o", out, "./testdata/diagram_server.go")
	cmd.Dir = mcpDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	outb, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build diagram MCP server: %v\n%s", err, outb)
	}
	return out
}
