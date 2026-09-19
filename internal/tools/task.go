package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
)

// NameTask is the model-facing orchestrator tool (VAL-ORCH-001+).
const NameTask = "task"

// TaskBackend is implemented by an orchestrator.Manager adapter in core.
// Callers at depth >= maxDepth get MaxDepthError (VAL-ORCH-002).
type TaskBackend interface {
	// CallerDepth is the nesting depth of the agent currently executing tools
	// (0 = primary). Sub-tasks run at depth 1 and cannot call task again.
	CallerDepth() int
	// SetCallerDepth is used by the runner when entering a sub-task (optional).
	SetCallerDepth(depth int)
	// SpawnParallel runs ≥1 explore/implement specs and returns a merged summary.
	SpawnParallel(ctx context.Context, specs []orchestrator.Spec) (merged string, runs []orchestrator.TaskRun, err error)
	// Synthesize consumes completed task results without re-explore (VAL-ORCH-011).
	Synthesize(note string) (summary string, consumed []string, err error)
	// SynthesisLog returns log lines proving synthesis used task results.
	SynthesisLog() []string
	// Cancel cancels an in-flight task by id (VAL-ORCH-006). Optional.
	Cancel(taskID string) error
	// Resume continues a cancelled/failed task (VAL-ORCH-007). Optional.
	Resume(ctx context.Context, taskID string) (orchestrator.TaskRun, error)
	// Categories lists enumerable task categories (VAL-ORCH-008).
	Categories() []orchestrator.CategoryInfo
}

// taskDepthContextKey carries caller depth into nested tool execution.
type taskDepthKey struct{}

// WithTaskDepth returns a context tagged with the current task nesting depth.
func WithTaskDepth(ctx context.Context, depth int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, taskDepthKey{}, depth)
}

// TaskDepthFromContext returns the depth stored by WithTaskDepth (0 if unset).
func TaskDepthFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if v, ok := ctx.Value(taskDepthKey{}).(int); ok {
		return v
	}
	return 0
}

// SetTaskBackend wires the task tool.
func (r *Registry) SetTaskBackend(tb TaskBackend) {
	if r == nil {
		return
	}
	r.task = tb
}

// TaskBackend returns the attached backend (may be nil).
func (r *Registry) TaskBackend() TaskBackend {
	if r == nil {
		return nil
	}
	return r.task
}

func taskDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name: NameTask,
				Description: "Spawn one or more background sub-tasks (explore/implement/verify) via the orchestrator. " +
					"Pass prompts[] (≥1) for parallel explore. Sub-tasks at max depth 1 cannot call task() again. " +
					"Results are merged for primary synthesis without re-running the same full search. " +
					"Use synthesize=true with an optional note to consume prior task results only.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "description": { "type": "string", "description": "Short UI label (3–5 words) for a single task" },
    "prompt": { "type": "string", "description": "Full instructions for a single sub-task" },
    "prompts": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Two or more explore prompts for parallel background tasks (preferred for parallel explore)"
    },
    "category": {
      "type": "string",
      "description": "explore | implement | verify | review | research (default explore = read-only)"
    },
    "run_in_background": { "type": "boolean", "description": "Prefer true for independent explore units" },
    "synthesize": {
      "type": "boolean",
      "description": "If true, synthesize from completed task results without re-running full explore (VAL-ORCH-011)"
    },
    "note": { "type": "string", "description": "Optional synthesis note when synthesize=true" },
    "task_id": { "type": "string", "description": "Resume or cancel an existing task by id (VAL-ORCH-006/007)" },
    "action": {
      "type": "string",
      "description": "Optional: cancel | resume | categories. Empty with prompts[] spawns; with task_id only resumes."
    }
  }
}`),
			},
		},
	}
}

type taskArgs struct {
	Description     string   `json:"description"`
	Prompt          string   `json:"prompt"`
	Prompts         []string `json:"prompts"`
	Category        string   `json:"category"`
	RunInBackground *bool    `json:"run_in_background"`
	Synthesize      bool     `json:"synthesize"`
	Note            string   `json:"note"`
	// TaskID resume / cancel target (VAL-ORCH-006/007).
	TaskID string `json:"task_id"`
	// Action optional: "cancel" | "resume" | "categories" | empty=spawn.
	Action string `json:"action"`
}

func (r *Registry) execTask(ctx context.Context, call Call) Result {
	if r == nil || r.task == nil {
		return Result{
			Name:    NameTask,
			Content: "task: orchestrator not configured",
			IsError: true,
		}
	}
	var args taskArgs
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return Result{
				Name:    NameTask,
				Content: fmt.Sprintf("task: invalid arguments JSON: %v", err),
				IsError: true,
			}
		}
	}

	action := strings.ToLower(strings.TrimSpace(args.Action))

	// Enumerate categories (VAL-ORCH-008).
	if action == "categories" {
		cats := r.task.Categories()
		var b strings.Builder
		b.WriteString("task categories:\n")
		for _, c := range cats {
			mode := "read-only"
			if c.AllowsWrite {
				mode = "writes via WriteGateway"
			}
			b.WriteString(fmt.Sprintf("- %s: %s (%s) tools=%s\n",
				c.Name, c.Description, mode, strings.Join(c.Tools, ",")))
		}
		return Result{Name: NameTask, Content: strings.TrimSpace(b.String())}
	}

	// Cancel by task_id (VAL-ORCH-006).
	if action == "cancel" {
		id := strings.TrimSpace(args.TaskID)
		if id == "" {
			return Result{Name: NameTask, Content: "task: task_id required for cancel", IsError: true}
		}
		if err := r.task.Cancel(id); err != nil {
			return Result{Name: NameTask, Content: err.Error(), IsError: true}
		}
		return Result{Name: NameTask, Content: fmt.Sprintf("cancelled task_id=%s", id)}
	}

	// Resume by task_id (VAL-ORCH-007): action=resume OR task_id without new prompts.
	noNewWork := strings.TrimSpace(args.Prompt) == "" && len(args.Prompts) == 0 && !args.Synthesize
	if action == "resume" || (action == "" && strings.TrimSpace(args.TaskID) != "" && noNewWork) {
		id := strings.TrimSpace(args.TaskID)
		if id == "" {
			return Result{Name: NameTask, Content: "task: task_id required for resume", IsError: true}
		}
		run, err := r.task.Resume(ctx, id)
		if err != nil {
			return Result{Name: NameTask, Content: err.Error(), IsError: true}
		}
		return Result{
			Name: NameTask,
			Content: fmt.Sprintf("resumed task_id=%s status=%s summary=%s",
				run.TaskID, run.Status, truncateTool(run.ResultSummary, 300)),
		}
	}

	// Synthesis path — consumes prior results, no new spawn (VAL-ORCH-011).
	if args.Synthesize {
		summary, consumed, err := r.task.Synthesize(args.Note)
		if err != nil {
			return Result{Name: NameTask, Content: err.Error(), IsError: true}
		}
		return Result{
			Name: NameTask,
			Content: fmt.Sprintf(
				"synthesis complete (consumed %d task result(s): %s)\n%s",
				len(consumed), strings.Join(consumed, ", "), summary,
			),
		}
	}

	// Depth gate at call site (VAL-ORCH-002). Prefer backend depth, fall back to ctx.
	depth := r.task.CallerDepth()
	if d := TaskDepthFromContext(ctx); d > depth {
		depth = d
	}

	// Build specs.
	var prompts []string
	for _, p := range args.Prompts {
		p = strings.TrimSpace(p)
		if p != "" {
			prompts = append(prompts, p)
		}
	}
	if len(prompts) == 0 {
		if p := strings.TrimSpace(args.Prompt); p != "" {
			prompts = append(prompts, p)
		}
	}
	if len(prompts) == 0 {
		return Result{
			Name:    NameTask,
			Content: "task: prompt or prompts[] is required (or set synthesize=true)",
			IsError: true,
		}
	}

	cat := strings.TrimSpace(args.Category)
	if cat == "" {
		cat = orchestrator.CategoryExplore
	}
	bg := true
	if args.RunInBackground != nil {
		bg = *args.RunInBackground
	}

	specs := make([]orchestrator.Spec, 0, len(prompts))
	for i, p := range prompts {
		desc := strings.TrimSpace(args.Description)
		if desc == "" {
			desc = fmt.Sprintf("%s %d", cat, i+1)
		} else if len(prompts) > 1 {
			desc = fmt.Sprintf("%s [%d]", desc, i+1)
		}
		specs = append(specs, orchestrator.Spec{
			Description:     desc,
			Prompt:          p,
			Category:        cat,
			Depth:           depth,
			RunInBackground: bg,
		})
	}

	merged, runs, err := r.task.SpawnParallel(ctx, specs)
	if err != nil {
		// Nested depth denial and other hard errors.
		return Result{Name: NameTask, Content: err.Error(), IsError: true}
	}

	// If every run failed with max-depth and none spawned, surface that as error.
	if len(runs) > 0 {
		allDepthDenied := true
		for _, run := range runs {
			if run.TaskID != "" || !strings.Contains(run.Error, "max depth") {
				allDepthDenied = false
				break
			}
		}
		if allDepthDenied {
			return Result{
				Name:    NameTask,
				Content: orchestrator.MaxDepthError,
				IsError: true,
			}
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("spawned %d sub-task(s)\n", countSpawned(runs)))
	for _, run := range runs {
		if run.TaskID == "" {
			b.WriteString(fmt.Sprintf("- denied: %s\n", run.Error))
			continue
		}
		line := fmt.Sprintf("- task_id=%s status=%s category=%s", run.TaskID, run.Status, run.Category)
		if run.Error != "" {
			line += " error=" + run.Error
		}
		if run.ResultSummary != "" {
			line += " summary=" + truncateTool(run.ResultSummary, 200)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(merged)
	b.WriteString("\n\n(Primary should synthesize from these results; call task with synthesize=true to merge without re-exploring.)")
	return Result{Name: NameTask, Content: strings.TrimSpace(b.String())}
}

func countSpawned(runs []orchestrator.TaskRun) int {
	n := 0
	for _, r := range runs {
		if r.TaskID != "" {
			n++
		}
	}
	return n
}

func truncateTool(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// ---- adapter helpers used by core ----

// TaskAdapter wraps orchestrator.Manager with caller depth and a default runner.
// Concurrent tool execution from multiple agent loops is safe via Manager mutex.
type TaskAdapter struct {
	Mgr    *orchestrator.Manager
	Runner orchestrator.RunnerFunc
	// WorkspaceID stamps every spawned task (hub active workspace / session).
	WorkspaceID string

	mu          sync.Mutex
	callerDepth int
}

// NewTaskAdapter builds a TaskBackend for tools.Registry.
func NewTaskAdapter(mgr *orchestrator.Manager, runner orchestrator.RunnerFunc) *TaskAdapter {
	return &TaskAdapter{Mgr: mgr, Runner: runner}
}

// SetWorkspaceID stamps spawn specs with the active workspace/session id.
func (a *TaskAdapter) SetWorkspaceID(id string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.WorkspaceID = strings.TrimSpace(id)
	a.mu.Unlock()
}

// CallerDepth implements TaskBackend.
func (a *TaskAdapter) CallerDepth() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.callerDepth
}

// SetCallerDepth implements TaskBackend.
func (a *TaskAdapter) SetCallerDepth(depth int) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.callerDepth = depth
	a.mu.Unlock()
}

// SpawnParallel implements TaskBackend.
func (a *TaskAdapter) SpawnParallel(ctx context.Context, specs []orchestrator.Spec) (string, []orchestrator.TaskRun, error) {
	if a == nil || a.Mgr == nil {
		return "", nil, fmt.Errorf("task: orchestrator manager not configured")
	}
	// Reject early when caller itself is already at max depth (nested task()).
	depth := a.CallerDepth()
	if len(specs) > 0 {
		// Specs already carry Depth from execTask; double-check.
		if err := a.Mgr.CanSpawn(depth); err != nil {
			return "", nil, err
		}
	}
	// Stamp workspace for every spawn so panel list is workspace-scoped.
	ws := ""
	a.mu.Lock()
	ws = a.WorkspaceID
	a.mu.Unlock()
	if ws != "" {
		for i := range specs {
			if strings.TrimSpace(specs[i].WorkspaceID) == "" {
				specs[i].WorkspaceID = ws
			}
		}
	}
	runner := a.Runner
	if runner == nil {
		runner = func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
			return fmt.Sprintf("sub-task %s completed (no runner): %s", run.TaskID, run.Prompt), []string{run.Prompt}, nil
		}
	}
	// Wrap runner so nested depth is visible if sub-task tools re-enter.
	wrapped := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		// Sub-task body runs at run.Depth (≥1). If the runner itself invokes
		// task tools, caller depth should be that depth.
		prev := a.CallerDepth()
		a.SetCallerDepth(run.Depth)
		defer a.SetCallerDepth(prev)
		ctx = WithTaskDepth(ctx, run.Depth)
		return runner(ctx, run)
	}
	return a.Mgr.SpawnParallel(ctx, specs, wrapped)
}

// Synthesize implements TaskBackend.
func (a *TaskAdapter) Synthesize(note string) (string, []string, error) {
	if a == nil || a.Mgr == nil {
		return "", nil, fmt.Errorf("task: orchestrator manager not configured")
	}
	return a.Mgr.SynthesizeFromResults(note)
}

// SynthesisLog implements TaskBackend.
func (a *TaskAdapter) SynthesisLog() []string {
	if a == nil || a.Mgr == nil {
		return nil
	}
	return a.Mgr.SynthesisLog()
}

// Cancel implements TaskBackend (VAL-ORCH-006).
func (a *TaskAdapter) Cancel(taskID string) error {
	if a == nil || a.Mgr == nil {
		return fmt.Errorf("task: orchestrator manager not configured")
	}
	return a.Mgr.Cancel(taskID)
}

// Resume implements TaskBackend (VAL-ORCH-007).
func (a *TaskAdapter) Resume(ctx context.Context, taskID string) (orchestrator.TaskRun, error) {
	if a == nil || a.Mgr == nil {
		return orchestrator.TaskRun{}, fmt.Errorf("task: orchestrator manager not configured")
	}
	runner := a.Runner
	if runner == nil {
		runner = func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
			// Default: preserve partial and mark resumed with prior context only.
			sum := run.PartialOutput
			if sum == "" {
				sum = run.ResultSummary
			}
			if sum == "" {
				sum = "resumed: " + run.Prompt
			}
			return sum + "\n(resume completed — no runner body)", run.Results, nil
		}
	}
	wrapped := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		prev := a.CallerDepth()
		a.SetCallerDepth(run.Depth)
		defer a.SetCallerDepth(prev)
		ctx = WithTaskDepth(ctx, run.Depth)
		return runner(ctx, run)
	}
	return a.Mgr.Resume(ctx, taskID, wrapped)
}

// Categories implements TaskBackend (VAL-ORCH-008).
func (a *TaskAdapter) Categories() []orchestrator.CategoryInfo {
	return orchestrator.KnownCategories()
}
