// Package orchestrator implements task() dispatch: parallel background sub-tasks,
// depth limits, write-conflict isolation, and failure isolation (VAL-ORCH-*).
//
// Architecture invariants (architecture.md §Core invariants #7):
//   - depth default 1 — nested task() denied
//   - parallel background primarily for read-heavy explore
//   - no conflicting writes from parallel sub-tasks
package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EventType classifies orchestrator lifecycle events for CLI/desktop subscribers.
type EventType string

const (
	// EventTaskSpawned is emitted when a sub-task starts (distinct task_id).
	EventTaskSpawned EventType = "TaskSpawned"
	// EventTaskDone is emitted when a sub-task finishes (ok, err, or cancelled).
	EventTaskDone EventType = "TaskDone"
	// EventBlocked is emitted when a conflicting write is rejected (VAL-ORCH-003).
	EventBlocked EventType = "Blocked"
	// EventNeedsApproval is emitted when a write needs serialization/approval.
	EventNeedsApproval EventType = "NeedsApproval"
)

// TaskStatus is the lifecycle of one TaskRun.
type TaskStatus string

const (
	StatusQueued    TaskStatus = "queued"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusCancelled TaskStatus = "cancelled"
)

// DefaultMaxDepth is the default nesting depth for task() (VAL-ORCH-002).
// Sub-tasks at depth >= MaxDepth cannot call task() again.
const DefaultMaxDepth = 1

// DefaultMaxConcurrent is the default max parallel background tasks.
const DefaultMaxConcurrent = 6

// MaxDepthError is the stable denial for nested task() (VAL-ORCH-002).
// Validators look for "max depth" / "orchestrator: max depth 1 reached".
const MaxDepthError = "orchestrator: max depth 1 reached"

// WriteConflictError is returned when a parallel sub-task loses a write lock.
const WriteConflictError = "orchestrator: write blocked — overlapping path held by another sub-task"

// Category names for task() (explore/implement/verify) — used by this feature
// and expanded by m10-orchestrator-cancel-resume-categories-panel.
const (
	CategoryExplore   = "explore"
	CategoryImplement = "implement"
	CategoryVerify    = "verify"
	CategoryReview    = "review"
	CategoryResearch  = "research"
)

// Event is one TaskSpawned / TaskDone / Blocked emission.
type Event struct {
	Type      EventType  `json:"type"`
	TaskID    string     `json:"task_id,omitempty"`
	ParentID  string     `json:"parent_id,omitempty"`
	Category  string     `json:"category,omitempty"`
	Status    TaskStatus `json:"status,omitempty"`
	Detail    string     `json:"detail,omitempty"`
	Error     string     `json:"error,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	Prompt    string     `json:"prompt,omitempty"`
	Depth     int        `json:"depth,omitempty"`
	StartedAt time.Time  `json:"started_at,omitempty"`
	EndedAt   time.Time  `json:"ended_at,omitempty"`
	// SynthesisConsumed is set when primary synthesis reuses task results
	// without re-running the full explore search (VAL-ORCH-011).
	SynthesisConsumed bool `json:"synthesis_consumed,omitempty"`
}

// Emitter receives orchestrator events (CLI printer, desktop SSE, tests).
type Emitter func(Event)

// ToolCallRecord is one completed tool invocation stored for resume skip (VAL-ORCH-007).
// Mirrors store.ToolCallRecord shape without importing store (avoid cycles).
type ToolCallRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Args    string `json:"args,omitempty"`
	Content string `json:"content,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// TaskRun is one sub-task record (session store shape from docs/orchestration.md).
type TaskRun struct {
	TaskID          string     `json:"task_id"`
	ParentSessionID string     `json:"parent_session_id,omitempty"`
	// WorkspaceID scopes the task to a hub workspace / session id.
	WorkspaceID     string     `json:"workspace_id,omitempty"`
	Category        string     `json:"category"`
	Description     string     `json:"description,omitempty"`
	Prompt          string     `json:"prompt"`
	Status          TaskStatus `json:"status"`
	ResultSummary   string     `json:"result_summary,omitempty"`
	Error           string     `json:"error,omitempty"`
	Depth           int        `json:"depth"`
	// Results are structured tool outputs / explore findings for primary synthesis
	// without re-running the same search (VAL-ORCH-011).
	Results   []string  `json:"results,omitempty"`
	Logs      []string  `json:"logs,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`

	// PartialOutput is accumulated text before cancel (included in final summary on resume).
	PartialOutput string `json:"partial_output,omitempty"`
	// CompletedToolCalls lists tool invocations already finished so Resume skips them.
	CompletedToolCalls []ToolCallRecord `json:"completed_tool_calls,omitempty"`

	// cancel cancels an in-flight run (VAL-ORCH-006).
	cancel context.CancelFunc `json:"-"`
	// cancelRequested is set by Cancel so finish prefers StatusCancelled.
	cancelRequested bool `json:"-"`
}

// Snapshot returns a deep-enough copy safe for JSON/API.
func (t *TaskRun) Snapshot() TaskRun {
	if t == nil {
		return TaskRun{}
	}
	out := *t
	out.cancel = nil
	out.cancelRequested = false
	if t.Results != nil {
		out.Results = append([]string(nil), t.Results...)
	}
	if t.Logs != nil {
		out.Logs = append([]string(nil), t.Logs...)
	}
	if t.CompletedToolCalls != nil {
		out.CompletedToolCalls = append([]ToolCallRecord(nil), t.CompletedToolCalls...)
	}
	return out
}

// Spec describes one unit to spawn via task() / SpawnParallel.
type Spec struct {
	Description string
	Prompt      string
	// Category defaults to explore for read-only background work.
	Category string
	// Depth is the nesting level of the *caller* (0 = primary).
	// When Depth >= MaxDepth, spawn is rejected (VAL-ORCH-002).
	Depth int
	// ParentSessionID optionally groups tasks under a chat session.
	ParentSessionID string
	// WorkspaceID scopes the task to a hub workspace / session scratch id.
	WorkspaceID string
	// TaskID when set resumes/reuses an id (reserved for resume feature).
	TaskID string
	// RunInBackground is informational for this package (SpawnParallel always
	// runs sub-tasks concurrently).
	RunInBackground bool
}

// RunnerFunc executes one sub-task body. Implementations may call tools (read or
// write). Return a non-empty summary and optional structured results for synthesis.
// Errors are isolated: other parallel tasks still complete (VAL-ORCH-009).
type RunnerFunc func(ctx context.Context, run *TaskRun) (summary string, results []string, err error)

// Options configures a Manager.
type Options struct {
	// MaxDepth defaults to DefaultMaxDepth (1).
	MaxDepth int
	// MaxConcurrent defaults to DefaultMaxConcurrent.
	MaxConcurrent int
	// Emitter receives TaskSpawned/TaskDone/Blocked events.
	Emitter Emitter
}

// Manager owns task() dispatch, write locks for parallel sub-tasks, and results
// available for primary synthesis (VAL-ORCH-001/002/003/009/011).
// Cancel/Resume/categories: VAL-ORCH-006/007/008.
type Manager struct {
	mu            sync.Mutex
	maxDepth      int
	maxConcurrent int
	emit          Emitter
	// persist optionally writes TaskRun to store (VAL-ORCH-006/007).
	persist PersistFunc

	// tasks by id.
	tasks map[string]*TaskRun
	// order of task ids (newest last).
	order []string

	// pathLocks tracks relative paths currently held for write by a task_id
	// so overlapping parallel writes block/reject (VAL-ORCH-003).
	pathLocks map[string]string // rel path → task_id

	// synthLog records synthesis consuming prior task results (VAL-ORCH-011).
	synthLog []string
}

// New returns a Manager with defaults.
func New(opts Options) *Manager {
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	maxConc := opts.MaxConcurrent
	if maxConc <= 0 {
		maxConc = DefaultMaxConcurrent
	}
	emit := opts.Emitter
	if emit == nil {
		emit = func(Event) {}
	}
	return &Manager{
		maxDepth:      maxDepth,
		maxConcurrent: maxConc,
		emit:          emit,
		tasks:         make(map[string]*TaskRun),
		pathLocks:     make(map[string]string),
	}
}

// MaxDepth returns the configured depth limit.
func (m *Manager) MaxDepth() int {
	if m == nil {
		return DefaultMaxDepth
	}
	return m.maxDepth
}

// SetEmitter replaces the event callback (thread-safe).
func (m *Manager) SetEmitter(fn Emitter) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn == nil {
		fn = func(Event) {}
	}
	m.emit = fn
}

func (m *Manager) emitter() Emitter {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.emit
}

// SpawnParallel runs ≥1 specs concurrently (VAL-ORCH-001).
// Each successful start emits TaskSpawned with a distinct task_id; each finish
// emits TaskDone. One failure does not abort siblings (VAL-ORCH-009).
//
// depthCheck: if any Spec.Depth >= MaxDepth, that spawn is rejected with
// MaxDepthError and does not emit TaskSpawned (VAL-ORCH-002).
//
// The returned merged summary includes every input prompt/result so primary
// synthesis can consume them without re-running full explore (VAL-ORCH-011).
func (m *Manager) SpawnParallel(ctx context.Context, specs []Spec, runner RunnerFunc) (merged string, runs []TaskRun, err error) {
	if m == nil {
		return "", nil, fmt.Errorf("orchestrator: manager is nil")
	}
	if runner == nil {
		return "", nil, fmt.Errorf("orchestrator: runner is required")
	}
	if len(specs) == 0 {
		return "", nil, fmt.Errorf("orchestrator: at least one task spec is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	type slot struct {
		spec Spec
		run  *TaskRun
		ok   bool // false when depth denied before spawn
		deny string
	}

	slots := make([]slot, len(specs))
	for i, sp := range specs {
		sp = normalizeSpec(sp)
		slots[i].spec = sp
		if sp.Depth >= m.maxDepth {
			slots[i].ok = false
			slots[i].deny = MaxDepthError
			continue
		}
		id := strings.TrimSpace(sp.TaskID)
		if id == "" {
			id = newTaskID()
		}
		tr := &TaskRun{
			TaskID:          id,
			ParentSessionID: sp.ParentSessionID,
			WorkspaceID:     sp.WorkspaceID,
			Category:        sp.Category,
			Description:     sp.Description,
			Prompt:          sp.Prompt,
			Status:          StatusQueued,
			Depth:           sp.Depth + 1, // the sub-task itself lives at parent+1
			CreatedAt:       time.Now().UTC(),
			Logs:            []string{},
			Results:         []string{},
		}
		m.mu.Lock()
		m.tasks[id] = tr
		m.order = append(m.order, id)
		m.mu.Unlock()
		m.persistRun(tr)
		slots[i].run = tr
		slots[i].ok = true
	}

	var wg sync.WaitGroup
	var muOut sync.Mutex
	outRuns := make([]TaskRun, 0, len(slots))

	// Cap concurrent goroutines (still start as soon as a slot frees).
	sem := make(chan struct{}, m.maxConcurrent)

	for i := range slots {
		sl := &slots[i]
		if !sl.ok {
			// Nested task() denied — no TaskSpawned (VAL-ORCH-002).
			denied := TaskRun{
				TaskID:   "",
				Category: sl.spec.Category,
				Prompt:   sl.spec.Prompt,
				Status:   StatusFailed,
				Error:    sl.deny,
				Depth:    sl.spec.Depth,
			}
			muOut.Lock()
			outRuns = append(outRuns, denied)
			muOut.Unlock()
			continue
		}

		wg.Add(1)
		go func(sl *slot) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			run := sl.run
			taskCtx, cancel := context.WithCancel(ctx)
			run.cancel = cancel
			defer cancel()

			now := time.Now().UTC()
			m.mu.Lock()
			run.Status = StatusRunning
			run.StartedAt = now
			m.mu.Unlock()

			m.emitter()(Event{
				Type:      EventTaskSpawned,
				TaskID:    run.TaskID,
				ParentID:  run.ParentSessionID,
				Category:  run.Category,
				Status:    StatusRunning,
				Detail:    firstNonEmpty(run.Description, truncate(run.Prompt, 80)),
				Prompt:    run.Prompt,
				Depth:     run.Depth,
				StartedAt: now,
			})
			m.persistRun(run)

			summary, results, runErr := runner(taskCtx, run)

			ended := time.Now().UTC()
			m.mu.Lock()
			run.EndedAt = ended
			// Cancel path (VAL-ORCH-006): prefer cancelled when request or ctx cancelled.
			cancelled := run.cancelRequested || taskCtx.Err() != nil ||
				(runErr != nil && (runErr == context.Canceled || strings.Contains(runErr.Error(), "context canceled")))
			if cancelled {
				run.Status = StatusCancelled
				if runErr != nil {
					run.Error = runErr.Error()
				} else {
					run.Error = context.Canceled.Error()
				}
				// Preserve partial output for resume (VAL-ORCH-007).
				if strings.TrimSpace(summary) != "" {
					run.ResultSummary = summary
					run.PartialOutput = summary
				} else if run.PartialOutput != "" {
					run.ResultSummary = run.PartialOutput
				} else {
					run.ResultSummary = "cancelled"
				}
			} else if runErr != nil {
				run.Status = StatusFailed
				run.Error = runErr.Error()
				// Keep partial summary/results if any (VAL-ORCH-009).
				if strings.TrimSpace(summary) != "" {
					run.ResultSummary = summary
					run.PartialOutput = summary
				} else {
					run.ResultSummary = "failed: " + runErr.Error()
				}
			} else {
				run.Status = StatusCompleted
				run.Error = ""
				run.ResultSummary = summary
				if summary != "" {
					run.PartialOutput = summary
				}
			}
			if len(results) > 0 {
				// Append when reusing an id; replace when first full set.
				if len(run.Results) == 0 {
					run.Results = append([]string(nil), results...)
				} else {
					run.Results = append(run.Results, results...)
				}
			}
			// Always attach the input prompt as a result line for synthesis merge.
			run.Logs = append(run.Logs, fmt.Sprintf("prompt=%s", run.Prompt))
			if summary != "" {
				run.Logs = append(run.Logs, "result="+summary)
			}
			snap := run.Snapshot()
			m.mu.Unlock()

			// Release write locks so cancelled tasks free child resources (VAL-ORCH-006).
			m.ReleaseAllLocks(run.TaskID)
			m.persistRun(run)

			ev := Event{
				Type:      EventTaskDone,
				TaskID:    snap.TaskID,
				ParentID:  snap.ParentSessionID,
				Category:  snap.Category,
				Status:    snap.Status,
				Detail:    firstNonEmpty(snap.Description, truncate(snap.Prompt, 80)),
				Summary:   snap.ResultSummary,
				Prompt:    snap.Prompt,
				Depth:     snap.Depth,
				StartedAt: snap.StartedAt,
				EndedAt:   snap.EndedAt,
			}
			if snap.Error != "" {
				ev.Error = snap.Error
			}
			m.emitter()(ev)

			muOut.Lock()
			outRuns = append(outRuns, snap)
			muOut.Unlock()
		}(sl)
	}

	wg.Wait()

	// Stable-ish order: match original specs where possible.
	merged = mergeSummaries(outRuns)
	return merged, outRuns, nil
}

// SpawnOne is a convenience for a single Spec.
func (m *Manager) SpawnOne(ctx context.Context, spec Spec, runner RunnerFunc) (TaskRun, error) {
	merged, runs, err := m.SpawnParallel(ctx, []Spec{spec}, runner)
	_ = merged
	if err != nil {
		return TaskRun{}, err
	}
	if len(runs) == 0 {
		return TaskRun{}, fmt.Errorf("orchestrator: no task produced")
	}
	r := runs[0]
	if r.Status == StatusFailed && r.Error != "" && r.TaskID == "" {
		// depth deny
		return r, fmt.Errorf("%s", r.Error)
	}
	return r, nil
}

// CanSpawn reports whether a caller at the given depth may invoke task().
// Primary depth is 0; after one spawn the sub-task depth is 1 and cannot nest.
func (m *Manager) CanSpawn(callerDepth int) error {
	if m == nil {
		return fmt.Errorf("orchestrator: manager is nil")
	}
	if callerDepth >= m.maxDepth {
		return fmt.Errorf("%s", MaxDepthError)
	}
	return nil
}

// AcquireWriteLock tries to lock a workspace-relative path for exclusive write
// by taskID. On conflict returns WriteConflictError and emits Blocked
// (VAL-ORCH-003). path is normalized to slash-relative form.
func (m *Manager) AcquireWriteLock(taskID, path string) error {
	if m == nil {
		return fmt.Errorf("orchestrator: manager is nil")
	}
	taskID = strings.TrimSpace(taskID)
	rel := normalizeRel(path)
	if rel == "" {
		return fmt.Errorf("orchestrator: empty write path")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if holder, ok := m.pathLocks[rel]; ok && holder != taskID {
		m.emit(Event{
			Type:   EventBlocked,
			TaskID: taskID,
			Detail: rel,
			Error:  WriteConflictError,
			Status: StatusFailed,
		})
		return fmt.Errorf("%s (path %q held by %s)", WriteConflictError, rel, holder)
	}
	// Also block if any held path is a parent/child of rel (overlapping).
	for held, holder := range m.pathLocks {
		if holder == taskID {
			continue
		}
		if pathsOverlap(held, rel) {
			m.emit(Event{
				Type:   EventBlocked,
				TaskID: taskID,
				Detail: rel,
				Error:  WriteConflictError,
				Status: StatusFailed,
			})
			return fmt.Errorf("%s (path %q overlaps %q held by %s)", WriteConflictError, rel, held, holder)
		}
	}
	m.pathLocks[rel] = taskID
	return nil
}

// ReleaseWriteLock releases a path held by taskID (no-op if not held).
func (m *Manager) ReleaseWriteLock(taskID, path string) {
	if m == nil {
		return
	}
	rel := normalizeRel(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	if holder, ok := m.pathLocks[rel]; ok && holder == taskID {
		delete(m.pathLocks, rel)
	}
}

// ReleaseAllLocks drops every lock held by taskID (call on TaskDone).
func (m *Manager) ReleaseAllLocks(taskID string) {
	if m == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	m.mu.Lock()
	defer m.mu.Unlock()
	for p, holder := range m.pathLocks {
		if holder == taskID {
			delete(m.pathLocks, p)
		}
	}
}

// Get returns a task snapshot by id (zero value + false if missing).
func (m *Manager) Get(taskID string) (TaskRun, bool) {
	if m == nil {
		return TaskRun{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tr, ok := m.tasks[taskID]
	if !ok || tr == nil {
		return TaskRun{}, false
	}
	return tr.Snapshot(), true
}

// List returns all known task snapshots (oldest first).
func (m *Manager) List() []TaskRun {
	return m.ListByWorkspace("")
}

// ListByWorkspace returns tasks for workspaceID (oldest first).
// Empty workspaceID returns all tasks.
func (m *Manager) ListByWorkspace(workspaceID string) []TaskRun {
	if m == nil {
		return nil
	}
	ws := strings.TrimSpace(workspaceID)
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TaskRun, 0, len(m.order))
	for _, id := range m.order {
		tr, ok := m.tasks[id]
		if !ok || tr == nil {
			continue
		}
		if ws != "" && strings.TrimSpace(tr.WorkspaceID) != ws {
			continue
		}
		out = append(out, tr.Snapshot())
	}
	return out
}

// CompletedResults returns result payloads from completed tasks for primary
// synthesis (VAL-ORCH-011). Does not re-run explore searches.
func (m *Manager) CompletedResults() []TaskRun {
	if m == nil {
		return nil
	}
	var out []TaskRun
	for _, tr := range m.List() {
		if tr.Status == StatusCompleted || (tr.Status == StatusFailed && tr.ResultSummary != "") {
			out = append(out, tr)
		}
	}
	return out
}

// SynthesizeFromResults builds a synthesis summary from completed task results
// without re-executing explore searches. Logs that synthesis consumed results
// (VAL-ORCH-011). Returns empty if no prior results.
func (m *Manager) SynthesizeFromResults(note string) (summary string, consumed []string, err error) {
	if m == nil {
		return "", nil, fmt.Errorf("orchestrator: manager is nil")
	}
	results := m.CompletedResults()
	if len(results) == 0 {
		return "", nil, fmt.Errorf("orchestrator: no completed task results to synthesize")
	}
	var b strings.Builder
	b.WriteString("synthesis from task results (no re-explore):\n")
	for _, tr := range results {
		line := fmt.Sprintf("- [%s] %s: %s", tr.TaskID, tr.Category, firstNonEmpty(tr.ResultSummary, "(empty)"))
		b.WriteString(line)
		b.WriteByte('\n')
		consumed = append(consumed, tr.TaskID)
		for _, r := range tr.Results {
			b.WriteString("    · ")
			b.WriteString(r)
			b.WriteByte('\n')
		}
	}
	if strings.TrimSpace(note) != "" {
		b.WriteString("note: ")
		b.WriteString(strings.TrimSpace(note))
		b.WriteByte('\n')
	}
	summary = strings.TrimSpace(b.String())
	logLine := fmt.Sprintf("synthesis consumed task_ids=%s", strings.Join(consumed, ","))
	m.mu.Lock()
	m.synthLog = append(m.synthLog, logLine)
	m.mu.Unlock()
	m.emitter()(Event{
		Type:              EventTaskDone,
		Detail:            "synthesis",
		Summary:           summary,
		SynthesisConsumed: true,
		Status:            StatusCompleted,
	})
	return summary, consumed, nil
}

// SynthesisLog returns log lines proving synthesis used prior task results.
func (m *Manager) SynthesisLog() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.synthLog...)
}

// ---- helpers ----

func normalizeSpec(sp Spec) Spec {
	sp.Description = strings.TrimSpace(sp.Description)
	sp.Prompt = strings.TrimSpace(sp.Prompt)
	sp.Category = strings.TrimSpace(sp.Category)
	if sp.Category == "" {
		sp.Category = CategoryExplore
	}
	if sp.Depth < 0 {
		sp.Depth = 0
	}
	return sp
}

func mergeSummaries(runs []TaskRun) string {
	var b strings.Builder
	b.WriteString("merged task results:\n")
	for _, r := range runs {
		id := r.TaskID
		if id == "" {
			id = "(denied)"
		}
		status := string(r.Status)
		if r.Error != "" {
			status = status + " err=" + r.Error
		}
		b.WriteString(fmt.Sprintf("- [%s] cat=%s status=%s prompt=%q → %s\n",
			id, r.Category, status, truncate(r.Prompt, 60), firstNonEmpty(r.ResultSummary, "(no summary)")))
	}
	return strings.TrimSpace(b.String())
}

func newTaskID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	return "task-" + hex.EncodeToString(b[:])
}

func normalizeRel(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	return path
}

func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	// parent/child: "foo" vs "foo/bar"
	if strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
		return true
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
