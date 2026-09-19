package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CROSS-008: MCP tool + diagram save.
//
// Cross-area flow under test (internal/mcp Host + internal/tools Registry +
// internal/tools/diagram + internal/writegate Gateway):
//
//	fixture stdio MCP server exposes `get_diagram` → returns a mermaid string
//	    → agent calls mcp.<server>.get_diagram via the registry
//	    → agent calls create_diagram with that mermaid string
//	    → file saved at docs/diagrams/<slug>.mmd through WriteGateway
//	    → WriteGateway snapshot pushed (source=diagram)
//	    → undo restores prior state (pre-agent dirty, not HEAD)
//
// This exercises ALL expected behaviors of the feature:
//   - MCP-served tool is callable by the agent (registry.Execute dispatches mcp.* names)
//   - create_diagram saves to docs/diagrams/<slug>.mmd via WriteGateway
//   - WriteGateway snapshot pushed (UndoDepth grows)
//   - undo restores prior content (pre-absent → file removed; pre-dirty → bytes restored)
func TestCrossArea_MCPToolDiagramSave_UndoRestores(t *testing.T) {
	root := t.TempDir()
	bin := buildDiagramMCPServer(t)

	// --- Build registry with WriteGateway + diagram tools + MCP host. ---
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatalf("writegate.New: %v", err)
	}
	reg := tools.NewRegistry(gw)
	// Diagram tools route every create/update through WriteGateway.
	tools.EnsureDiagramFromGateway(reg)

	// Wire the fixture MCP server (stdio). Namespaced tools become
	// mcp.<server>.<tool> so they cannot collide with built-ins.
	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := h.Register(ctx, mcp.ServerConfig{
		Name:    "diagrams",
		Type:    "stdio",
		Command: bin,
	}); err != nil {
		t.Fatalf("mcp register: %v", err)
	}
	reg.SetMCPHost(h)

	// The namespaced tool must be advertised to the model.
	toolName := "mcp.diagrams.get_diagram"
	if !reg.MCPHost().HasNamespacedTool(toolName) {
		t.Fatalf("MCP tool %q not registered", toolName)
	}
	advertised := false
	for _, d := range reg.Definitions() {
		if d.Function.Name == toolName {
			advertised = true
			break
		}
	}
	if !advertised {
		t.Fatalf("MCP tool %q missing from registry Definitions()", toolName)
	}

	// --- Step 1: agent calls the MCP-served tool to get the diagram source. ---
	// (This mirrors how the model would invoke mcp.diagrams.get_diagram and then
	// feed the returned text into create_diagram.)
	mcpRes := reg.Execute(ctx, tools.Call{
		ID:        "mcp-1",
		Name:      toolName,
		Arguments: `{"kind":"flowchart"}`,
	}, nil)
	if mcpRes.IsError {
		t.Fatalf("MCP tool call error: %s", mcpRes.Content)
	}
	// The MCP fixture returns the mermaid source as the tool result content.
	mermaidSrc := mcpRes.Content
	wantFragment := "flowchart TD"
	if !strings.Contains(mermaidSrc, wantFragment) {
		t.Fatalf("MCP tool did not return mermaid source: got %q (want fragment %q)",
			mermaidSrc, wantFragment)
	}

	// --- Step 2: agent calls create_diagram to save the result via WriteGateway. ---
	const slug = "mcp-auth-flow"
	rel := "docs/diagrams/" + slug + ".mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	createArgs, _ := json.Marshal(map[string]any{
		"slug":   slug,
		"format": "mermaid",
		"source": mermaidSrc,
		"title":  "MCP Auth Flow",
	})
	// Batch the agent turn so the single create_diagram write is one undo unit.
	reg.BeginTurn("cross-008-turn")
	diagRes := reg.Execute(ctx, tools.Call{
		ID:        "diag-1",
		Name:      tools.NameCreateDiagram,
		Arguments: string(createArgs),
	}, nil)
	reg.EndTurn()
	if diagRes.IsError {
		t.Fatalf("create_diagram error: %s", diagRes.Content)
	}
	// (a) File exists at docs/diagrams/<slug>.mmd.
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("diagram file not saved at %s: %v", rel, err)
	}
	if string(data) != mermaidSrc {
		t.Fatalf("diagram content mismatch:\n want %q\n got  %q", mermaidSrc, string(data))
	}
	// (b) create_diagram result mentions the saved path + WriteGateway.
	if !strings.Contains(diagRes.Content, rel) {
		t.Fatalf("create_diagram result should mention saved path %q: %q", rel, diagRes.Content)
	}
	if !strings.Contains(diagRes.Content, "WriteGateway") {
		t.Fatalf("create_diagram result should mention WriteGateway: %q", diagRes.Content)
	}
	// (c) WriteGateway snapshot pushed (one undo unit for the turn).
	if gw.UndoDepth() != 1 {
		t.Fatalf("expected undo depth 1 after create_diagram, got %d", gw.UndoDepth())
	}
	// The unit must tag its source as diagram (VAL-UNDO-007).
	units := gw.UndoStack()
	if len(units) != 1 || len(units[0].Entries) != 2 {
		t.Fatalf("expected 1 unit with 2 entries (diagram + index), got %+v", units)
	}
	if got := units[0].Entries[0].Source; got != writegate.SourceDiagram {
		t.Fatalf("undo unit source=%q want %q", got, writegate.SourceDiagram)
	}

	// --- Step 3: undo restores prior state (pre-agent dirty, not HEAD). ---
	// The diagram file did not exist before the agent turn, so undo must remove it.
	undoRes := gw.Undo()
	if undoRes.Noop {
		t.Fatalf("undo was a no-op: %s", undoRes.Message)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("after undo the diagram file should be removed (pre-absent), got err=%v", err)
	}
	// docs/diagrams dir may remain; that's fine — only the diagram file is restored.
	// Redo reapplies the agent change (round-trip check).
	redoRes := gw.Redo()
	if redoRes.Noop {
		t.Fatalf("redo was a no-op: %s", redoRes.Message)
	}
	data2, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("after redo diagram file should exist again: %v", err)
	}
	if string(data2) != mermaidSrc {
		t.Fatalf("after redo content mismatch:\n want %q\n got  %q", mermaidSrc, string(data2))
	}
}

// VAL-CROSS-008 variant: the diagram file existed with pre-agent dirty content
// before the agent turn. Undo must restore those pre-agent dirty bytes (NOT
// remove the file, NOT restore to git HEAD).
func TestCrossArea_MCPToolDiagramSave_UndoRestoresPreAgentDirty(t *testing.T) {
	root := t.TempDir()
	bin := buildDiagramMCPServer(t)

	gw, err := writegate.New(root)
	if err != nil {
		t.Fatalf("writegate.New: %v", err)
	}
	reg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(reg)

	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := h.Register(ctx, mcp.ServerConfig{
		Name:    "diagrams",
		Type:    "stdio",
		Command: bin,
	}); err != nil {
		t.Fatalf("mcp register: %v", err)
	}
	reg.SetMCPHost(h)

	// Pre-agent dirty content: a real diagram file already on disk.
	const slug = "existing-flow"
	rel := "docs/diagrams/" + slug + ".mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	const dirtyBefore = "flowchart TD\n    A --> B"
	if err := os.WriteFile(abs, []byte(dirtyBefore), 0o644); err != nil {
		t.Fatal(err)
	}

	// Agent fetches a new diagram source from the MCP tool and updates the file.
	mcpRes := reg.Execute(ctx, tools.Call{
		ID:        "mcp-1",
		Name:      "mcp.diagrams.get_diagram",
		Arguments: `{"kind":"er"}`,
	}, nil)
	if mcpRes.IsError {
		t.Fatalf("MCP tool call error: %s", mcpRes.Content)
	}
	mermaidSrc := mcpRes.Content
	if !strings.Contains(mermaidSrc, "erDiagram") {
		t.Fatalf("expected erDiagram source from MCP, got %q", mermaidSrc)
	}

	// create_diagram with the same slug overwrites the existing file via WriteGateway.
	createArgs, _ := json.Marshal(map[string]any{
		"slug":   slug,
		"format": "mermaid",
		"source": mermaidSrc,
	})
	reg.BeginTurn("cross-008-dirty-turn")
	diagRes := reg.Execute(ctx, tools.Call{
		ID:        "diag-1",
		Name:      tools.NameCreateDiagram,
		Arguments: string(createArgs),
	}, nil)
	reg.EndTurn()
	if diagRes.IsError {
		t.Fatalf("create_diagram error: %s", diagRes.Content)
	}
	// File now holds the MCP-returned source.
	got, _ := os.ReadFile(abs)
	if string(got) != mermaidSrc {
		t.Fatalf("file content after agent write = %q, want %q", got, mermaidSrc)
	}
	// Snapshot captured the pre-agent dirty content.
	if gw.UndoDepth() != 1 {
		t.Fatalf("undo depth=%d want 1", gw.UndoDepth())
	}
	units := gw.UndoStack()
	if len(units) != 1 || len(units[0].Entries) != 2 {
		t.Fatalf("unexpected stack: %+v", units)
	}
	if string(units[0].Entries[0].Snapshot.Before) != dirtyBefore {
		t.Fatalf("snapshot.Before = %q, want dirty-before %q",
			units[0].Entries[0].Snapshot.Before, dirtyBefore)
	}
	if !units[0].Entries[0].Snapshot.Existed {
		t.Fatal("snapshot.Existed must be true (file existed pre-agent)")
	}

	// Undo restores the pre-agent dirty content (NOT removed, NOT HEAD).
	undoRes := gw.Undo()
	if undoRes.Noop {
		t.Fatalf("undo noop: %s", undoRes.Message)
	}
	restored, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("after undo file should still exist with pre-agent content: %v", err)
	}
	if string(restored) != dirtyBefore {
		t.Fatalf("after undo content = %q, want pre-agent dirty %q", restored, dirtyBefore)
	}
}

// buildDiagramMCPServer compiles the fixture stdio MCP diagram server into a
// temp binary for the cross-area integration test. The fixture exposes a
// get_diagram tool returning deterministic mermaid sources.
func buildDiagramMCPServer(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "diagram_mcp_server")
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Resolve the mcp package dir from the tools package dir.
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
