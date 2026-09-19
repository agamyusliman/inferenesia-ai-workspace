// Package tools registers built-in agent tools (fs, shell, …).
// File mutations always go through writegate.Gateway.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// Tool names exposed to the model (OpenAI-compatible function names).
const (
	NameWriteFile      = "write_file"
	NameWriteMultibrain = "write_multibrain"
	NameWriteDiagram   = "write_diagram"
	NameShell          = "shell"
	// Diagram tools (VAL-DIAG-002/003/004). create/update route through
	// WriteGateway; export renders to PNG/SVG under the workspace sandbox.
	NameCreateDiagram = "create_diagram"
	NameUpdateDiagram = "update_diagram"
	NameExportDiagram = "export_diagram"
	// NameConvertDiagram is the Mermaid -> Excalidraw bridge (VAL-DIAG-007).
	NameConvertDiagram = "convert_diagram"
)

// mcpNamespacePrefix is the agent-facing prefix for every MCP-served tool
// (mirrors internal/mcp.NamespacePrefix). Declared locally so tools.go can
// reference it without importing internal/mcp (which would create a cycle
// for the category-filter path). MCP namespaced tools pass through the
// task category filter so a sub-task can invoke skill-granted MCP tools
// after the parent loaded the skill (VAL-CROSS-012).
const mcpNamespacePrefix = "mcp."

// Definition is an OpenAI-style tool schema entry.
type Definition struct {
	Type     string       `json:"type"` // always "function"
	Function FunctionSpec `json:"function"`
}

// FunctionSpec is the function object inside a tool definition.
type FunctionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Call is one model-requested tool invocation.
type Call struct {
	ID        string // tool_call id from the provider
	Name      string
	Arguments string // raw JSON arguments
}

// Result is returned to the model as a tool role message.
type Result struct {
	ID      string
	Name    string
	Content string
	IsError bool
}

// EventType classifies tool events for CLI/desktop subscribers.
type EventType string

const (
	EventStart EventType = "tool_start"
	EventEnd   EventType = "tool_end"
	EventError EventType = "tool_error"
)

// Event is emitted around tool execution (VAL-CLI-005/006/007 evidence).
type Event struct {
	Type    EventType
	Name    string
	ID      string
	Detail  string // human-readable: path, command, denial, etc.
	Content string // tool result body (may be truncated for display)
	IsError bool
}

// Registry maps tool names to handlers and publishes Definition list to the model.
type Registry struct {
	gw      *writegate.Gateway
	shellWD string
	// ShellTimeout bounds short shell exec (default 30s).
	ShellTimeout time.Duration
	// MaxShellOutput caps captured stdout+stderr (bytes).
	MaxShellOutput int
	// terminal is optional PTY backend for terminal_* tools (VAL-TERM-007+).
	terminal TerminalBackend
	// git is optional agent GitService (AgentMode) for git_* tools (VAL-GIT-005+).
	git GitBackend
	// gitApprover surfaces NeedsApproval for commit/push/destructive ops.
	gitApprover Approver
	// planMode, when set and active, denies mutating tools (VAL-PLAN-001/006).
	planMode *plan.State
	// todo is optional session todo + propose_plan backend (VAL-PLAN-003/005).
	todo TodoBackend
	// mission is optional Mission/Goal backend (VAL-MISSION-*).
	mission MissionBackend
	// task is optional orchestrator task() backend (VAL-ORCH-001+).
	task TaskBackend
	// skill is optional progressive skill loader backend (VAL-ORCH-004/005).
	skill SkillBackend
	// skillMCP registers/unregisters MCP servers that a loaded skill declares
	// in its frontmatter (VAL-CROSS-012). May be nil — skills with mcp-servers
	// then load their body/prompt but do not register MCP tools.
	skillMCP SkillMCPRegistrar
	// browser is optional BrowserManager backend (VAL-BRW-006..010).
	browser BrowserBackend
	// mcp is optional MCP host backend (VAL-MCP-001/005). Namespaced tools only.
	mcp MCPHostBackend
	// diagram is optional diagram tools backend (VAL-DIAG-002/003/004).
	diagram DiagramBackend
	// context7 is optional builtin library-docs backend (P9-ctx). When nil
	// or env-disabled, the tool is not advertised (or returns a clear
	// disabled message if explicitly registered).
	context7 Context7Backend
	// categoryFilter, when non-empty, restricts Definitions/Execute for
	// sub-task tool sets (explore/implement/verify) (VAL-ORCH-008).
	categoryFilter string
	// autonomyLevel gates tool execution by risk level (off/low/medium/high).
	autonomyLevel string
}

// NewRegistry builds a registry with write_file and shell tools.
// workspaceRoot is the sandbox root for writes and the default shell cwd.
func NewRegistry(gw *writegate.Gateway) *Registry {
	wd := ""
	if gw != nil {
		wd = gw.RootPath()
	}
	return &Registry{
		gw:             gw,
		shellWD:        wd,
		ShellTimeout:   30 * time.Second,
		MaxShellOutput: 64 << 10, // 64 KiB
	}
}

// Gateway returns the WriteGateway used by this registry (may be nil).
func (r *Registry) Gateway() *writegate.Gateway {
	if r == nil {
		return nil
	}
	return r.gw
}

// SetPlanMode wires Plan Mode enforcement into this registry (shared with core.Service).
// When mode is active, Execute denies write tools and Definitions filters to read-only.
func (r *Registry) SetPlanMode(st *plan.State) {
	if r == nil {
		return
	}
	r.planMode = st
}

// SetCategoryFilter limits advertised/executable tools to a task category tool set
// (explore/implement/verify) (VAL-ORCH-008). Empty string clears the filter.
func (r *Registry) SetCategoryFilter(category string) {
	if r == nil {
		return
	}
	r.categoryFilter = strings.TrimSpace(category)
}

func (r *Registry) SetAutonomyLevel(level string) {
	if r == nil {
		return
	}
	r.autonomyLevel = strings.TrimSpace(level)
}

func (r *Registry) AutonomyLevel() string {
	if r == nil {
		return ""
	}
	return r.autonomyLevel
}

// CategoryFilter returns the active category tool filter (empty when unrestricted).
func (r *Registry) CategoryFilter() string {
	if r == nil {
		return ""
	}
	return r.categoryFilter
}

// PlanMode returns the attached Plan Mode state (may be nil).
func (r *Registry) PlanMode() *plan.State {
	if r == nil {
		return nil
	}
	return r.planMode
}

// BeginTurn opens a WriteGateway turn so multi-file writes batch into one undo unit.
func (r *Registry) BeginTurn(turnID string) {
	if r == nil || r.gw == nil {
		return
	}
	r.gw.BeginTurn(turnID)
}

// EndTurn commits the open WriteGateway turn (if any).
func (r *Registry) EndTurn() {
	if r == nil || r.gw == nil {
		return
	}
	r.gw.EndTurn()
}

// Definitions returns OpenAI-compatible tool schemas for the agent loop.
// All file mutation tools (write_file, write_multibrain, write_diagram) route
// exclusively through WriteGateway (VAL-UNDO-007). Terminal tools use PTY
// (VAL-TERM-007/008/012) and do not bypass WriteGateway for files.
// When Plan Mode is active, only the read-only tool set is advertised (VAL-PLAN-006).
func (r *Registry) Definitions() []Definition {
	defs := []Definition{}
	defs = append(defs, readOnlyDefs()...)
	defs = append(defs,
		Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameWriteFile,
				Description: "Create or overwrite a text file inside the workspace via WriteGateway (undoable). Path must be relative to the workspace root (or absolute but still under the workspace). Writes outside the workspace are denied.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "File path relative to workspace root (e.g. hello.txt)" },
    "content": { "type": "string", "description": "Full file contents to write" }
  },
  "required": ["path", "content"]
}`),
			},
		},
		Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameWriteMultibrain,
				Description: "Write or append Multi Brain memory under .multibrain/ via WriteGateway (undoable snapshot). Path must be under .multibrain/ (e.g. .multibrain/indexes/agents.md or .multibrain/context/note.md).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Path under .multibrain/ (relative to workspace)" },
    "content": { "type": "string", "description": "Full file contents to write" }
  },
  "required": ["path", "content"]
}`),
			},
		},
		Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameWriteDiagram,
				Description: "Save a diagram source file under docs/diagrams/ (or docs/*.mmd) via WriteGateway (undoable). Prefer docs/diagrams/<name>.mmd for Mermaid.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Path under docs/diagrams/ (e.g. docs/diagrams/auth-flow.mmd)" },
    "content": { "type": "string", "description": "Full diagram source (mermaid/dbml/etc.)" }
  },
  "required": ["path", "content"]
}`),
			},
		},
		Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameShell,
				Description: "Run a short shell command in the workspace directory and return stdout, stderr, and exit code. Prefer simple read-only or diagnostic commands. Do not use for writing files outside the workspace.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": { "type": "string", "description": "Shell command to execute (e.g. echo tool-ok)" }
  },
  "required": ["command"]
}`),
			},
		},
	)
	defs = append(defs, terminalDefs()...)
	defs = append(defs, gitDefs()...)
	// Plan propose + session todos (VAL-PLAN-003/005). Available when backend wired.
	if r != nil && r.todo != nil {
		defs = append(defs, todoDefs()...)
	}
	// Mission/Goal tools (VAL-MISSION-001+). Available when backend wired.
	if r != nil && r.mission != nil {
		defs = append(defs, missionDefs()...)
	}
	// Orchestrator task() tool (VAL-ORCH-001+). Available when backend wired.
	if r != nil && r.task != nil {
		defs = append(defs, taskDefs()...)
	}
	// Skill list/load + multi-brain tools when skill backend is wired (VAL-ORCH-004/005).
	// Skill-granted tools (multibrain_*, skill_status) appear only after skill load.
	if r != nil && r.skill != nil {
		defs = append(defs, skillDefs(r)...)
		// Research tool (research_save) only after research skill load (VAL-CROSS-005).
		defs = append(defs, researchDefs(r)...)
		// Optional 9Router kit tools only after matching skill load (VAL-KIT-002).
		defs = append(defs, nineRouterToolDefs(r)...)
	}
	// Browser multi-engine tools when backend wired (VAL-BRW-006..010).
	// No shell/exec under browser_* namespace (VAL-BRW-009).
	if r != nil && r.browser != nil {
		defs = append(defs, browserDefs()...)
	}
	// MCP namespaced tools when host is wired (VAL-MCP-001/005): mcp.<server>.<tool>.
	// Names never collide with built-ins (e.g. mcp.srv.fs.read vs read_file).
	if r != nil && r.mcp != nil {
		defs = append(defs, mcpDefs(r)...)
	}
	// Diagram tools (create/update/export) when backend wired (VAL-DIAG-002/003/004).
	if r != nil && r.diagram != nil {
		defs = append(defs, diagramDefs(r)...)
	}
	// Builtin Context7 library-docs tool (P9-ctx). Available when a backend
	// is wired and not env-disabled. Read-only; safe in every category.
	if r != nil && r.context7 != nil {
		defs = append(defs, context7Defs(r)...)
	}
	// Assertion tools for verify category (always registered handlers; filter may hide).
	defs = append(defs, assertDefs()...)
	if r != nil && r.planMode != nil && r.planMode.Active() {
		defs = plan.FilterDefinitions(true, defs, func(d Definition) string {
			return d.Function.Name
		})
	}
	// Category filter for sub-task registries (VAL-ORCH-008).
	// MCP namespaced tools (mcp.<server>.<tool>) granted by a loaded skill
	// are allowed in every category: the skill scopes what the MCP server
	// exposes, and a sub-task must be able to call them after the parent
	// loaded the skill (VAL-CROSS-012). The category filter still denies
	// built-in write tools (write_file, create_diagram, …) for read-only
	// categories.
	if r != nil && r.categoryFilter != "" {
		allowed := map[string]bool{}
		for _, n := range orchestrator.ToolsForCategory(r.categoryFilter) {
			allowed[n] = true
		}
		var filtered []Definition
		for _, d := range defs {
			name := d.Function.Name
			if allowed[name] {
				filtered = append(filtered, d)
				continue
			}
			// MCP namespaced tools pass through the category filter so a
			// sub-task can invoke skill-granted MCP tools (VAL-CROSS-012).
			if strings.HasPrefix(name, mcpNamespacePrefix) {
				filtered = append(filtered, d)
			}
		}
		return filtered
	}
	return defs
}

// Execute runs one tool call and returns a Result for the model.
// emit is optional; when non-nil it receives start/end/error events.
// Plan Mode denials (VAL-PLAN-001/006) happen before any side effects.
func (r *Registry) Execute(ctx context.Context, call Call, emit func(Event)) Result {
	if emit == nil {
		emit = func(Event) {}
	}
	name := strings.TrimSpace(call.Name)
	emit(Event{Type: EventStart, Name: name, ID: call.ID, Detail: summarizeArgs(name, call.Arguments)})

	// Core-layer Plan Mode gate: reject mutating tools before handlers run.
	if r != nil && r.planMode != nil {
		if err := r.planMode.AllowTool(name); err != nil {
			res := Result{
				ID:      call.ID,
				Name:    name,
				Content: err.Error(),
				IsError: true,
			}
			emit(Event{
				Type:    EventError,
				Name:    name,
				ID:      call.ID,
				Detail:  summarizeArgs(name, call.Arguments),
				Content: res.Content,
				IsError: true,
			})
			return res
		}
	}

	// Category tool set (VAL-ORCH-008): explore/verify cannot call write_file, etc.
	// MCP namespaced tools (mcp.<server>.<tool>) are exempt: a sub-task must be
	// able to call skill-granted MCP tools after the parent loaded the skill
	// (VAL-CROSS-012). The skill scopes what the MCP server exposes.
	if r != nil && r.categoryFilter != "" && !strings.HasPrefix(name, mcpNamespacePrefix) {
		if !orchestrator.ToolAllowedInCategory(r.categoryFilter, name) {
			msg := orchestrator.DenyToolError(r.categoryFilter, name).Error()
			res := Result{ID: call.ID, Name: name, Content: msg, IsError: true}
			emit(Event{
				Type:    EventError,
				Name:    name,
				ID:      call.ID,
				Detail:  summarizeArgs(name, call.Arguments),
				Content: msg,
				IsError: true,
			})
			return res
		}
	}

	if r != nil && r.autonomyLevel != "" {
		if denied, reason := autonomyDenies(r.autonomyLevel, name); denied {
			res := Result{ID: call.ID, Name: name, Content: reason, IsError: true}
			emit(Event{
				Type:    EventError,
				Name:    name,
				ID:      call.ID,
				Detail:  summarizeArgs(name, call.Arguments),
				Content: reason,
				IsError: true,
			})
			return res
		}
	}

	var res Result
	switch name {
	case NameReadFile:
		res = r.execReadFile(call)
	case NameGrep:
		res = r.execGrep(call)
	case NameListDir:
		res = r.execListDir(call)
	case NameWriteFile:
		res = r.execWriteFile(call)
	case NameWriteMultibrain:
		res = r.execWriteMultibrain(call)
	case NameWriteDiagram:
		res = r.execWriteDiagram(call)
	case NameCreateDiagram, NameUpdateDiagram, NameExportDiagram, NameConvertDiagram:
		res = r.execDiagram(ctx, call)
	case NameContext7Query:
		res = r.execContext7Query(ctx, call)
	case NameShell:
		res = r.execShell(ctx, call)
	case NameAssertContains:
		res = r.execAssertContains(call)
	case NameAssertExitCode:
		res = r.execAssertExitCode(call)
	case NameAssertFileExists:
		res = r.execAssertFileExists(call)
	case NameTerminalRead:
		res = r.execTerminalRead(call)
	case NameTerminalWrite:
		res = r.execTerminalWrite(call)
	case NameTerminalList:
		res = r.execTerminalList(call)
	case NameTerminalStart:
		res = r.execTerminalStart(call)
	case NameTerminalStop:
		res = r.execTerminalStop(call)
	case NameGitStatus, NameGitDiff, NameGitLog, NameGitStage, NameGitUnstage,
		NameGitCommit, NameGitFetch, NameGitPull, NameGitPush, NameGitBranchList,
		NameGitCheckout, NameGitResetHard, NameGitBranchDelete:
		res = r.execGit(ctx, call)
	case NameProposePlan:
		res = r.execProposePlan(call)
	case NameTodoWrite:
		res = r.execTodoWrite(call)
	case NameTodoUpdate:
		res = r.execTodoUpdate(call)
	case NameTodoList:
		res = r.execTodoList(call)
	case NameGetGoal:
		res = r.execGetGoal(call)
	case NameCreateGoal:
		res = r.execCreateGoal(call)
	case NameUpdateGoal:
		res = r.execUpdateGoal(call)
	case NameAttachMissionEvidence:
		res = r.execAttachMissionEvidence(call)
	case NameTask:
		res = r.execTask(ctx, call)
	case NameSkill:
		res = r.execSkill(call)
	case NameSkillStatus:
		res = r.execSkillStatus(call)
	case NameMultibrainRead:
		res = r.execMultibrainRead(call)
	case NameMultibrainAppend:
		res = r.execMultibrainAppend(call)
	case NameResearchSave:
		res = r.execResearchSave(call)
	case skills.ToolNineRouterImage, skills.ToolNineRouterEmbeddings,
		skills.ToolNineRouterWebSearch, skills.ToolNineRouterWebFetch,
		skills.ToolNineRouterSTT:
		res = r.execNineRouter(call)
	case NameBrowserSetEngine, NameBrowserStatus, NameBrowserNavigate,
		NameBrowserScreenshot, NameBrowserEvaluate, NameBrowserSnapshot,
		NameBrowserWaitHuman, NameBrowserClose:
		res = r.execBrowser(ctx, call)
	default:
		// MCP namespaced tools: mcp.<server>.<tool> (VAL-MCP-001/005).
		if r != nil && r.mcp != nil && r.mcp.HasNamespacedTool(name) {
			res = r.execMCP(ctx, call)
			break
		}
		// Explicit denial for forbidden browser shell bridges (VAL-BRW-009).
		if browser.HasShellBridgeTool(name) {
			res = Result{
				ID:      call.ID,
				Name:    name,
				Content: "browser: shell/exec tools are not exposed under the browser namespace (sandbox)",
				IsError: true,
			}
			break
		}
		// Clear unknown-tool for unregistered MCP names (and any other miss).
		if strings.HasPrefix(name, "mcp.") {
			res = Result{
				ID:      call.ID,
				Name:    name,
				Content: fmt.Sprintf("unknown tool %q (MCP tool not registered or server unavailable)", name),
				IsError: true,
			}
			break
		}
		res = Result{
			ID:      call.ID,
			Name:    name,
			Content: fmt.Sprintf("unknown tool %q", name),
			IsError: true,
		}
	}
	res.ID = call.ID
	if res.Name == "" {
		res.Name = name
	}

	evType := EventEnd
	if res.IsError {
		evType = EventError
	}
	emit(Event{
		Type:    evType,
		Name:    res.Name,
		ID:      res.ID,
		Detail:  summarizeArgs(name, call.Arguments),
		Content: res.Content,
		IsError: res.IsError,
	})
	return res
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (r *Registry) execWriteFile(call Call) Result {
	return r.execGatewayWrite(call, NameWriteFile, writegate.SourceFS, func(path string, content []byte) (writegate.Entry, error) {
		return r.gw.WriteFileWithSource(path, content, writegate.SourceFS)
	})
}

func (r *Registry) execWriteMultibrain(call Call) Result {
	return r.execGatewayWrite(call, NameWriteMultibrain, writegate.SourceMultibrain, func(path string, content []byte) (writegate.Entry, error) {
		return r.gw.WriteMultibrain(path, content)
	})
}

func (r *Registry) execWriteDiagram(call Call) Result {
	return r.execGatewayWrite(call, NameWriteDiagram, writegate.SourceDiagram, func(path string, content []byte) (writegate.Entry, error) {
		return r.gw.WriteDiagram(path, content)
	})
}

// execGatewayWrite is the shared path for all agent file mutations (VAL-UNDO-007).
func (r *Registry) execGatewayWrite(
	call Call,
	toolName string,
	source string,
	write func(path string, content []byte) (writegate.Entry, error),
) Result {
	if r.gw == nil {
		return Result{
			Name:    toolName,
			Content: toolName + ": WriteGateway not configured",
			IsError: true,
		}
	}
	var args writeFileArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{
			Name:    toolName,
			Content: fmt.Sprintf("%s: invalid arguments JSON: %v", toolName, err),
			IsError: true,
		}
	}
	path := strings.TrimSpace(args.Path)
	if path == "" {
		return Result{
			Name:    toolName,
			Content: toolName + ": path is required",
			IsError: true,
		}
	}

	entry, err := write(path, []byte(args.Content))
	if err != nil {
		// Clear denial message for the model (VAL-CLI-007).
		if writegate.IsSandboxDenied(err) {
			return Result{
				Name: toolName,
				Content: fmt.Sprintf(
					"sandbox denied: cannot write outside the workspace root (%s). Use a path relative to the workspace. Error: %v",
					r.gw.RootPath(), err,
				),
				IsError: true,
			}
		}
		return Result{
			Name:    toolName,
			Content: fmt.Sprintf("%s failed: %v", toolName, err),
			IsError: true,
		}
	}
	rel := entry.Snapshot.Rel
	if rel == "" {
		rel = filepath.Base(entry.Snapshot.Path)
	}
	src := entry.Source
	if src == "" {
		src = source
	}
	return Result{
		Name: toolName,
		Content: fmt.Sprintf(
			"wrote %d bytes to %s via WriteGateway (source=%s)",
			len(entry.After), rel, src,
		),
	}
}

type shellArgs struct {
	Command string `json:"command"`
}

func (r *Registry) execShell(ctx context.Context, call Call) Result {
	var args shellArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{
			Name:    NameShell,
			Content: fmt.Sprintf("shell: invalid arguments JSON: %v", err),
			IsError: true,
		}
	}
	cmdStr := strings.TrimSpace(args.Command)
	if cmdStr == "" {
		return Result{
			Name:    NameShell,
			Content: "shell: command is required",
			IsError: true,
		}
	}

	timeout := r.ShellTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use login-less sh -c for short commands.
	cmd := exec.CommandContext(cctx, "sh", "-c", cmdStr)
	if r.shellWD != "" {
		cmd.Dir = r.shellWD
	}
	// Minimal env — inherit process env so tools like echo work; no secret logging here.
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else if cctx.Err() != nil {
			return Result{
				Name:    NameShell,
				Content: fmt.Sprintf("shell: command timed out or canceled: %v", cctx.Err()),
				IsError: true,
			}
		} else {
			return Result{
				Name:    NameShell,
				Content: fmt.Sprintf("shell: failed to start: %v", runErr),
				IsError: true,
			}
		}
	}

	out := stdout.String()
	errOut := stderr.String()
	max := r.MaxShellOutput
	if max <= 0 {
		max = 64 << 10
	}
	out = truncate(out, max)
	errOut = truncate(errOut, max)

	var b strings.Builder
	fmt.Fprintf(&b, "exit_code: %d\n", exitCode)
	fmt.Fprintf(&b, "stdout:\n%s", out)
	if errOut != "" {
		fmt.Fprintf(&b, "\nstderr:\n%s", errOut)
	}
	return Result{
		Name:    NameShell,
		Content: b.String(),
		IsError: exitCode != 0,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…[truncated]"
}

func summarizeArgs(name, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return name
	}
	switch name {
	case NameWriteFile, NameWriteMultibrain, NameWriteDiagram, NameReadFile:
		var a writeFileArgs
		if json.Unmarshal([]byte(raw), &a) == nil && a.Path != "" {
			return fmt.Sprintf("path=%s", a.Path)
		}
	case NameGrep:
		var a grepArgs
		if json.Unmarshal([]byte(raw), &a) == nil && a.Pattern != "" {
			p := a.Pattern
			if len(p) > 80 {
				p = p[:80] + "…"
			}
			return fmt.Sprintf("pattern=%q", p)
		}
	case NameListDir:
		var a listDirArgs
		if json.Unmarshal([]byte(raw), &a) == nil {
			if a.Path == "" {
				return "path=."
			}
			return fmt.Sprintf("path=%s", a.Path)
		}
	case NameShell:
		var a shellArgs
		if json.Unmarshal([]byte(raw), &a) == nil && a.Command != "" {
			c := a.Command
			if len(c) > 120 {
				c = c[:120] + "…"
			}
			return fmt.Sprintf("command=%q", c)
		}
	case NameTerminalRead, NameTerminalStop:
		var a terminalStopArgs
		if json.Unmarshal([]byte(raw), &a) == nil && a.SessionID != "" {
			return fmt.Sprintf("session_id=%s", a.SessionID)
		}
	case NameTerminalWrite:
		var a terminalWriteArgs
		if json.Unmarshal([]byte(raw), &a) == nil && a.SessionID != "" {
			return fmt.Sprintf("session_id=%s bytes=%d", a.SessionID, len(a.Data))
		}
	case NameTerminalStart:
		var a terminalStartArgs
		if json.Unmarshal([]byte(raw), &a) == nil && a.Command != "" {
			c := a.Command
			if len(c) > 80 {
				c = c[:80] + "…"
			}
			return fmt.Sprintf("command=%q", c)
		}
	}
	if len(raw) > 120 {
		return raw[:120] + "…"
	}
	return raw
}
