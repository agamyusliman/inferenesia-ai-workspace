package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/store"
)

// TaskView is the desktop/CLI JSON shape for the task panel (VAL-ORCH-010).
type TaskView struct {
	TaskID      string `json:"task_id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Error       string `json:"error,omitempty"`
	// ElapsedMs is wall time since start (running) or start→end (done).
	ElapsedMs int64  `json:"elapsed_ms"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// TaskListView wraps tasks for the panel.
type TaskListView struct {
	Tasks   []TaskView `json:"tasks"`
	Product string     `json:"product"`
}

// TaskCategoriesView lists enumerable categories (VAL-ORCH-008).
type TaskCategoriesView struct {
	Categories []orchestrator.CategoryInfo `json:"categories"`
}

// ListTasks returns in-memory + store tasks for the active workspace only.
func (s *Service) ListTasks() TaskListView {
	out := TaskListView{Product: s.BrandName(), Tasks: []TaskView{}}
	if s == nil {
		return out
	}
	wsID := ""
	if active := s.GetActiveWorkspace(); active != nil {
		wsID = active.ID
	}
	if wsID == "" {
		return out
	}
	seen := map[string]bool{}
	mgr := s.OrchestratorManager()
	if mgr != nil {
		for _, tr := range mgr.ListByWorkspace(wsID) {
			out.Tasks = append(out.Tasks, taskRunToView(tr))
			seen[tr.TaskID] = true
		}
	}
	if st := s.sessionStore(); st != nil {
		rows, err := st.ListTasksByWorkspace(wsID, 100)
		if err == nil {
			for _, row := range rows {
				if seen[row.TaskID] {
					continue
				}
				out.Tasks = append(out.Tasks, storeTaskToView(row))
			}
		}
	}
	return out
}

// activeWorkspaceID returns the hub active workspace id (may be empty).
func (s *Service) activeWorkspaceID() string {
	if s == nil {
		return ""
	}
	if active := s.GetActiveWorkspace(); active != nil {
		return active.ID
	}
	return ""
}

// CancelTask cancels by task_id (VAL-ORCH-006).
func (s *Service) CancelTask(taskID string) (TaskView, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TaskView{}, fmt.Errorf("task_id is required")
	}
	mgr := s.OrchestratorManager()
	if mgr == nil {
		return TaskView{}, fmt.Errorf("orchestrator not available")
	}
	// Hydrate from store if missing in memory.
	if _, ok := mgr.Get(taskID); !ok {
		if st := s.sessionStore(); st != nil {
			if row, err := st.GetTask(taskID); err == nil {
				mgr.LoadFromSnapshot(storeToRun(row))
			}
		}
	}
	if err := mgr.Cancel(taskID); err != nil {
		// Durable-only force cancel when not live.
		if st := s.sessionStore(); st != nil {
			row, gerr := st.GetTask(taskID)
			if gerr != nil {
				return TaskView{}, err
			}
			if row.Status == string(orchestrator.StatusRunning) || row.Status == string(orchestrator.StatusQueued) {
				row.Status = string(orchestrator.StatusCancelled)
				row.Error = context.Canceled.Error()
				row.EndedAt = time.Now().UTC()
				if row.ResultSummary == "" {
					row.ResultSummary = "cancelled"
				}
				_ = st.UpsertTask(row)
				return storeTaskToView(row), nil
			}
		}
		return TaskView{}, err
	}
	// Wait briefly for cancel to settle (≤2s target).
	deadline := time.Now().Add(2 * time.Second)
	var snap orchestrator.TaskRun
	var ok bool
	for time.Now().Before(deadline) {
		snap, ok = mgr.Get(taskID)
		if ok && snap.Status == orchestrator.StatusCancelled {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !ok {
		return TaskView{}, fmt.Errorf("task %q not found after cancel", taskID)
	}
	return taskRunToView(snap), nil
}

// ResumeTask resumes a cancelled/failed task (VAL-ORCH-007).
func (s *Service) ResumeTask(ctx context.Context, taskID string) (TaskView, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TaskView{}, fmt.Errorf("task_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	mgr := s.OrchestratorManager()
	if mgr == nil {
		return TaskView{}, fmt.Errorf("orchestrator not available")
	}
	if _, ok := mgr.Get(taskID); !ok {
		if st := s.sessionStore(); st != nil {
			row, err := st.GetTask(taskID)
			if err != nil {
				return TaskView{}, err
			}
			mgr.LoadFromSnapshot(storeToRun(row))
		}
	}
	root := ""
	if s != nil {
		root = s.ActiveRoot()
	}
	runner := s.defaultTaskRunner(root)
	snap, err := mgr.Resume(ctx, taskID, runner)
	if err != nil {
		return TaskView{}, err
	}
	return taskRunToView(snap), nil
}

// TaskCategories returns enumerable categories (VAL-ORCH-008).
func (s *Service) TaskCategories() TaskCategoriesView {
	return TaskCategoriesView{Categories: orchestrator.KnownCategories()}
}

// wireTaskPersist attaches store.UpsertTask to Manager.persist.
func (s *Service) wireTaskPersist(mgr *orchestrator.Manager) {
	if s == nil || mgr == nil {
		return
	}
	mgr.SetPersist(func(run orchestrator.TaskRun) {
		st := s.sessionStore()
		if st == nil {
			return
		}
		_ = st.UpsertTask(runToStore(run))
	})
}

func (s *Service) defaultTaskRunner(root string) orchestrator.RunnerFunc {
	explore := orchestrator.NewExploreRunnerFunc(root)
	return func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		// Category filter does not change explore runner; implement/verify use same
		// lightweight body until full sub-agent LLM loop lands. Resume skips tools.
		if len(run.CompletedToolCalls) > 0 {
			var b strings.Builder
			if run.PartialOutput != "" {
				b.WriteString(run.PartialOutput)
			} else if run.ResultSummary != "" {
				b.WriteString(run.ResultSummary)
			}
			b.WriteString(fmt.Sprintf("\n(resume skipped %d completed tool call(s))", len(run.CompletedToolCalls)))
			if len(run.Results) > 0 {
				return strings.TrimSpace(b.String()), run.Results, nil
			}
			// Append light explore only when no prior results.
			if orchestrator.CategoryIsReadOnly(run.Category) {
				sum, results, err := explore(ctx, run)
				if err != nil {
					return strings.TrimSpace(b.String()), run.Results, err
				}
				return strings.TrimSpace(b.String() + "\n" + sum), append(run.Results, results...), nil
			}
			return strings.TrimSpace(b.String()), run.Results, nil
		}
		return explore(ctx, run)
	}
}

// sessionStore returns the durable store when available (may be nil).
func (s *Service) sessionStore() *store.Store {
	if s == nil {
		return nil
	}
	st, err := s.ensureStore()
	if err != nil {
		return nil
	}
	return st
}

// ActiveRoot returns the active workspace root path for runners.
func (s *Service) ActiveRoot() string {
	if s == nil {
		return ""
	}
	// Prefer hub active workspace.
	st := s.GetHubState()
	if st.Active != nil && st.Active.RootPath != "" {
		return st.Active.RootPath
	}
	return ""
}

func taskRunToView(tr orchestrator.TaskRun) TaskView {
	v := TaskView{
		TaskID:      tr.TaskID,
		WorkspaceID: tr.WorkspaceID,
		Category:    tr.Category,
		Status:      string(tr.Status),
		Description: tr.Description,
		Prompt:      tr.Prompt,
		Summary:     tr.ResultSummary,
		Error:       tr.Error,
	}
	if !tr.CreatedAt.IsZero() {
		v.CreatedAt = tr.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !tr.StartedAt.IsZero() {
		v.StartedAt = tr.StartedAt.UTC().Format(time.RFC3339Nano)
		end := tr.EndedAt
		if end.IsZero() {
			end = time.Now().UTC()
		}
		v.ElapsedMs = end.Sub(tr.StartedAt).Milliseconds()
		if v.ElapsedMs < 0 {
			v.ElapsedMs = 0
		}
	}
	if !tr.EndedAt.IsZero() {
		v.EndedAt = tr.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	return v
}

func storeTaskToView(t store.Task) TaskView {
	v := TaskView{
		TaskID:      t.TaskID,
		WorkspaceID: t.WorkspaceID,
		Category:    t.Category,
		Status:      t.Status,
		Description: t.Description,
		Prompt:      t.Prompt,
		Summary:     t.ResultSummary,
		Error:       t.Error,
	}
	if !t.CreatedAt.IsZero() {
		v.CreatedAt = t.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !t.StartedAt.IsZero() {
		v.StartedAt = t.StartedAt.UTC().Format(time.RFC3339Nano)
		end := t.EndedAt
		if end.IsZero() {
			end = time.Now().UTC()
		}
		v.ElapsedMs = end.Sub(t.StartedAt).Milliseconds()
		if v.ElapsedMs < 0 {
			v.ElapsedMs = 0
		}
	}
	if !t.EndedAt.IsZero() {
		v.EndedAt = t.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	return v
}

func runToStore(run orchestrator.TaskRun) store.Task {
	tools := make([]store.ToolCallRecord, 0, len(run.CompletedToolCalls))
	for _, t := range run.CompletedToolCalls {
		tools = append(tools, store.ToolCallRecord{
			ID: t.ID, Name: t.Name, Args: t.Args, Content: t.Content, IsError: t.IsError,
		})
	}
	return store.Task{
		TaskID:          run.TaskID,
		ParentSessionID: run.ParentSessionID,
		WorkspaceID:     run.WorkspaceID,
		Category:        run.Category,
		Description:     run.Description,
		Prompt:          run.Prompt,
		Status:          string(run.Status),
		ResultSummary:   firstNonEmptyStr(run.ResultSummary, run.PartialOutput),
		Error:           run.Error,
		Depth:           run.Depth,
		Results:         run.Results,
		Logs:            run.Logs,
		CompletedTools:  tools,
		CreatedAt:       run.CreatedAt,
		StartedAt:       run.StartedAt,
		EndedAt:         run.EndedAt,
	}
}

func storeToRun(t store.Task) orchestrator.TaskRun {
	tools := make([]orchestrator.ToolCallRecord, 0, len(t.CompletedTools))
	for _, c := range t.CompletedTools {
		tools = append(tools, orchestrator.ToolCallRecord{
			ID: c.ID, Name: c.Name, Args: c.Args, Content: c.Content, IsError: c.IsError,
		})
	}
	return orchestrator.TaskRun{
		TaskID:             t.TaskID,
		ParentSessionID:    t.ParentSessionID,
		WorkspaceID:        t.WorkspaceID,
		Category:           t.Category,
		Description:        t.Description,
		Prompt:             t.Prompt,
		Status:             orchestrator.TaskStatus(t.Status),
		ResultSummary:      t.ResultSummary,
		Error:              t.Error,
		Depth:              t.Depth,
		Results:            t.Results,
		Logs:               t.Logs,
		PartialOutput:      t.ResultSummary,
		CompletedToolCalls: tools,
		CreatedAt:          t.CreatedAt,
		StartedAt:          t.StartedAt,
		EndedAt:            t.EndedAt,
	}
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
