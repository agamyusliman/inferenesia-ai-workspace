package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CROSS-011: orchestrator sub-task uses browser + parent writes diagram
// (multi-area).
//
// Cross-area flow under test (internal/orchestrator + internal/browser +
// internal/tools/diagram + internal/writegate):
//
//	parallel task(explore) sub-task calls browser_navigate (read-only, allowed
//	    for explore category) to gather page info → returns sanitized page text
//	    as its merged result
//	parent agent calls create_diagram with the merged browser-gathered result
//	    → file saved at docs/diagrams/<slug>.mmd through WriteGateway
//	sub-task must be read-only: it did NOT call any write tool (write_file,
//	    create_diagram, etc.) — the category filter denied every write attempt
//	only the parent writes (create_diagram routes through WriteGateway; undo
//	    restores prior state)
//
// This exercises ALL expected behaviors of the feature:
//   - explore category grants read-only browser tools (navigate/snapshot) (VAL-CROSS-011)
//   - explore category denies every write tool (write_file, create_diagram, …)
//   - sub-task tool-call log filtered to contain zero write-tool invocations
//   - parent create_diagram saves to docs/diagrams/<slug>.mmd via WriteGateway
//   - WriteGateway snapshot pushed (source=diagram); undo restores prior state
func TestCrossArea_OrchestratorSubtaskBrowser_ParentWritesDiagram(t *testing.T) {
	root := t.TempDir()

	// --- Parent registry: WriteGateway + diagram tools + browser backend + task. ---
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatalf("writegate.New: %v", err)
	}
	parentReg := tools.NewRegistry(gw)
	tools.EnsureDiagramFromGateway(parentReg)

	// Shared stub browser backend used by BOTH the parent and the sub-task
	// registry. The sub-task uses it read-only (navigate + snapshot); the
	// parent never touches the browser — it only writes the diagram.
	bb := &crossOrchStubBrowser{
		workspaceRoot: root,
		pageText:      "Auth Service Architecture: Login → Token → Session → Refresh",
	}
	parentReg.SetBrowserBackend(bb)

	// --- Orchestrator + task backend wired into the parent registry. ---
	mgr := orchestrator.New(orchestrator.Options{})

	// toolCallLog records every tool call the sub-task makes so we can assert
	// no write tool was invoked (VAL-CROSS-011: "tool-call log filtered").
	var (
		logMu sync.Mutex
		logs  []subTaskToolCall
	)
	recordCall := func(name string, isError bool, content string) {
		logMu.Lock()
		defer logMu.Unlock()
		logs = append(logs, subTaskToolCall{Name: name, IsError: isError, Content: content})
	}

	// The sub-task runner builds a CATEGORY-FILTERED (explore) registry sharing
	// the same WriteGateway + browser backend, then drives the browser
	// read-only and attempts a write (which must be denied).
	subRunner := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		// Build the sub-task's own registry scoped to explore (read-only).
		// It shares the WriteGateway (so a write attempt would route through
		// it) and the browser backend (so navigate/snapshot work), but the
		// category filter denies every write tool.
		subReg := tools.NewRegistry(gw)
		subReg.SetBrowserBackend(bb)
		subReg.SetCategoryFilter(orchestrator.CategoryExplore)

		// (1) Verify the explore category advertises browser_navigate but NOT
		// any write tool (VAL-CROSS-011 + VAL-ORCH-008).
		advertised := map[string]bool{}
		for _, d := range subReg.Definitions() {
			advertised[d.Function.Name] = true
		}
		if !advertised[tools.NameBrowserNavigate] {
			return "", nil, fmt.Errorf("explore category must advertise browser_navigate (read-only browser gather), got: %v", advertised)
		}
		for _, writeTool := range []string{
			tools.NameWriteFile, tools.NameWriteMultibrain, tools.NameWriteDiagram,
			tools.NameCreateDiagram, tools.NameUpdateDiagram, tools.NameExportDiagram,
			tools.NameConvertDiagram, tools.NameShell,
		} {
			if advertised[writeTool] {
				return "", nil, fmt.Errorf("explore category must NOT advertise write tool %q", writeTool)
			}
		}

		// (2) Sub-task calls browser_navigate (read-only, allowed for explore).
		navURL := "data:text/html,<html><head><title>Auth Service</title></head><body>Auth Service Architecture: Login → Token → Session → Refresh</body></html>"
		navRes := subReg.Execute(ctx, tools.Call{
			ID:        "sub-nav",
			Name:      tools.NameBrowserNavigate,
			Arguments: `{"url":"` + navURL + `"}`,
		}, nil)
		recordCall(tools.NameBrowserNavigate, navRes.IsError, navRes.Content)
		if navRes.IsError {
			return "", nil, fmt.Errorf("sub-task browser_navigate failed: %s", navRes.Content)
		}

		// (3) Sub-task calls browser_snapshot to gather page text (read-only).
		snapRes := subReg.Execute(ctx, tools.Call{
			ID:        "sub-snap",
			Name:      tools.NameBrowserSnapshot,
			Arguments: `{}`,
		}, nil)
		recordCall(tools.NameBrowserSnapshot, snapRes.IsError, snapRes.Content)
		if snapRes.IsError {
			return "", nil, fmt.Errorf("sub-task browser_snapshot failed: %s", snapRes.Content)
		}
		gathered := snapRes.Content

		// (4) Sub-task attempts a WRITE tool (create_diagram). This MUST be
		// denied by the explore category filter (read-only). The denial is
		// recorded so the assertion can prove no write succeeded.
		writeRes := subReg.Execute(ctx, tools.Call{
			ID:        "sub-write-attempt",
			Name:      tools.NameCreateDiagram,
			Arguments: `{"slug":"should-not-exist","source":"flowchart TD\n    A --> B"}`,
		}, nil)
		recordCall(tools.NameCreateDiagram, writeRes.IsError, writeRes.Content)
		if !writeRes.IsError {
			return "", nil, fmt.Errorf("explore sub-task must NOT be able to call create_diagram (read-only); got success: %s", writeRes.Content)
		}
		if !strings.Contains(writeRes.Content, "denied") && !strings.Contains(writeRes.Content, "read-only") {
			return "", nil, fmt.Errorf("denial message must mention denied/read-only: %s", writeRes.Content)
		}

		// (5) Sub-task attempts write_file too — also must be denied.
		wfRes := subReg.Execute(ctx, tools.Call{
			ID:        "sub-wf-attempt",
			Name:      tools.NameWriteFile,
			Arguments: `{"path":"should-not-exist.txt","content":"nope"}`,
		}, nil)
		recordCall(tools.NameWriteFile, wfRes.IsError, wfRes.Content)
		if !wfRes.IsError {
			return "", nil, fmt.Errorf("explore sub-task must NOT be able to call write_file (read-only)")
		}

		// Return the gathered browser text as the merged result for the parent.
		summary := "browser gather ok: " + truncateStr(gathered, 120)
		return summary, []string{gathered}, nil
	}

	adapter := tools.NewTaskAdapter(mgr, subRunner)
	parentReg.SetTaskBackend(adapter)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- Spawn the parallel task(explore) sub-task via the parent's task tool. ---
	taskArgs, _ := json.Marshal(map[string]any{
		"prompt":   "Use the browser to gather the auth service architecture from the page",
		"category": orchestrator.CategoryExplore,
	})
	taskRes := parentReg.Execute(ctx, tools.Call{
		ID:        "parent-task",
		Name:      tools.NameTask,
		Arguments: string(taskArgs),
	}, nil)
	if taskRes.IsError {
		t.Fatalf("parent task() failed: %s", taskRes.Content)
	}
	// The merged result must contain the browser-gathered text.
	if !strings.Contains(taskRes.Content, "browser gather ok") {
		t.Fatalf("task result should contain browser gather summary: %s", taskRes.Content)
	}

	// --- Assert: sub-task did NOT call any write tool successfully (tool-call log filtered). ---
	logMu.Lock()
	captured := append([]subTaskToolCall(nil), logs...)
	logMu.Unlock()
	writeTools := map[string]bool{
		tools.NameWriteFile: true, tools.NameWriteMultibrain: true, tools.NameWriteDiagram: true,
		tools.NameCreateDiagram: true, tools.NameUpdateDiagram: true, tools.NameExportDiagram: true,
		tools.NameConvertDiagram: true, tools.NameShell: true,
	}
	for _, c := range captured {
		if writeTools[c.Name] && !c.IsError {
			t.Fatalf("VAL-CROSS-011 violation: sub-task successfully called write tool %q (content: %s) — sub-task must be read-only",
				c.Name, c.Content)
		}
	}
	// The sub-task MUST have called browser_navigate (read-only browser gather).
	foundNavigate := false
	for _, c := range captured {
		if c.Name == tools.NameBrowserNavigate {
			foundNavigate = true
		}
	}
	if !foundNavigate {
		t.Fatal("sub-task tool-call log must contain browser_navigate (read-only browser gather)")
	}

	// --- Parent calls create_diagram with the merged browser-gathered result. ---
	// The parent turns the gathered auth architecture text into a mermaid
	// diagram and saves it via WriteGateway (only the parent writes).
	const slug = "auth-service-arch"
	rel := "docs/diagrams/" + slug + ".mmd"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	mermaidSrc := "flowchart TD\n    Login --> Token\n    Token --> Session\n    Session --> Refresh"
	createArgs, _ := json.Marshal(map[string]any{
		"slug":   slug,
		"format": "mermaid",
		"source": mermaidSrc,
		"title":  "Auth Service Architecture (browser-gathered)",
	})
	// Batch the parent's create_diagram as one undo unit.
	parentReg.BeginTurn("cross-011-parent-turn")
	diagRes := parentReg.Execute(ctx, tools.Call{
		ID:        "parent-diag",
		Name:      tools.NameCreateDiagram,
		Arguments: string(createArgs),
	}, nil)
	parentReg.EndTurn()
	if diagRes.IsError {
		t.Fatalf("parent create_diagram error: %s", diagRes.Content)
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
	// (c) WriteGateway snapshot pushed (one undo unit for the parent turn).
	if gw.UndoDepth() != 1 {
		t.Fatalf("expected undo depth 1 after parent create_diagram, got %d", gw.UndoDepth())
	}
	units := gw.UndoStack()
	if len(units) != 1 || len(units[0].Entries) != 2 {
		t.Fatalf("expected 1 unit with 2 entries (diagram + index), got %+v", units)
	}
	if got := units[0].Entries[0].Source; got != writegate.SourceDiagram {
		t.Fatalf("undo unit source=%q want %q", got, writegate.SourceDiagram)
	}

	// --- Undo restores prior state (only the parent's write is on the stack). ---
	// The sub-task made NO WriteGateway mutation (read-only), so the only undo
	// unit is the parent's create_diagram. Undo removes the diagram file
	// (pre-absent → file removed).
	undoRes := gw.Undo()
	if undoRes.Noop {
		t.Fatalf("undo was a no-op: %s", undoRes.Message)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("after undo the diagram file should be removed (pre-absent), got err=%v", err)
	}

	// --- No stray write files from the sub-task's denied write attempts. ---
	for _, stray := range []string{
		filepath.Join(root, "should-not-exist.txt"),
		filepath.Join(root, "docs", "diagrams", "should-not-exist.mmd"),
	} {
		if _, err := os.Stat(stray); err == nil {
			t.Fatalf("sub-task write attempt must NOT have created a file: %s", stray)
		}
	}
}

// subTaskToolCall is one recorded tool invocation by the sub-task (for the
// filtered tool-call log assertion in VAL-CROSS-011).
type subTaskToolCall struct {
	Name    string
	IsError bool
	Content string
}

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// crossOrchStubBrowser is a stub BrowserBackend for the VAL-CROSS-011
// integration test. It records navigate calls and returns deterministic
// sanitized page text from Snapshot (no cookies/auth ever leaked — VAL-BRW-006).
type crossOrchStubBrowser struct {
	workspaceRoot string
	pageText      string
	mu            sync.Mutex
	navigated     string
}

func (s *crossOrchStubBrowser) SetEngine(ctx context.Context, id string, headless *bool) error {
	return nil
}
func (s *crossOrchStubBrowser) Status() (string, string, string, int, bool, string) {
	return "e2e", "ready", "", 0, false, ""
}
func (s *crossOrchStubBrowser) Navigate(ctx context.Context, url string) error {
	if err := browser.ValidateNavigateURL(url); err != nil {
		return err
	}
	s.mu.Lock()
	s.navigated = url
	s.mu.Unlock()
	return nil
}
func (s *crossOrchStubBrowser) Screenshot(ctx context.Context, outPath string) (string, int, error) {
	return "", 0, nil
}
func (s *crossOrchStubBrowser) Evaluate(ctx context.Context, expression string) (string, error) {
	return browser.SanitizeForLLM(s.pageText), nil
}
func (s *crossOrchStubBrowser) Snapshot(ctx context.Context) (string, error) {
	return browser.PageTextResultForLLM(s.pageText), nil
}
func (s *crossOrchStubBrowser) WaitHuman(ctx context.Context, reason string, minMs, maxMs int) (int, error) {
	return 1, nil
}
func (s *crossOrchStubBrowser) Close(ctx context.Context) error {
	return nil
}
func (s *crossOrchStubBrowser) ArtifactDir() string { return "" }
