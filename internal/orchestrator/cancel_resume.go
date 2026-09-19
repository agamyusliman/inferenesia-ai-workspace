package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PersistFunc optionally persists TaskRun snapshots (store.UpsertTask).
// Wired by core; tests may omit (no-op).
type PersistFunc func(run TaskRun)

// SetPersist registers a durable snapshot callback (thread-safe).
func (m *Manager) SetPersist(fn PersistFunc) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persist = fn
}

func (m *Manager) persistRun(run *TaskRun) {
	if m == nil || run == nil {
		return
	}
	m.mu.Lock()
	fn := m.persist
	m.mu.Unlock()
	if fn != nil {
		fn(run.Snapshot())
	}
}

// Cancel cancels an in-flight background task by task_id (VAL-ORCH-006).
// Emits TaskDone with status=cancelled once the runner observes ctx cancel
// (target ≤2s for cooperative runners). Releases write locks held by the task.
func (m *Manager) Cancel(taskID string) error {
	if m == nil {
		return fmt.Errorf("orchestrator: manager is nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("orchestrator: task_id is required")
	}
	m.mu.Lock()
	tr, ok := m.tasks[taskID]
	if !ok || tr == nil {
		m.mu.Unlock()
		return fmt.Errorf("orchestrator: task %q not found", taskID)
	}
	if tr.Status != StatusRunning && tr.Status != StatusQueued {
		st := tr.Status
		m.mu.Unlock()
		return fmt.Errorf("orchestrator: task %q is not running (status=%s)", taskID, st)
	}
	cancel := tr.cancel
	// Mark cancel requested so finish path prefers cancelled over failed when
	// the runner returns ctx.Err() shortly after.
	tr.cancelRequested = true
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	} else {
		// Not yet entered goroutine — mark cancelled immediately.
		m.finishCancelled(tr, "cancelled before start")
	}
	// Always release locks so children cannot hold resources (VAL-ORCH-006).
	m.ReleaseAllLocks(taskID)
	return nil
}

// Resume restarts a cancelled or failed task by task_id, preserving prior
// context (prompt, partial summary, completed tool calls, results/logs).
// The resumed run does not re-execute already-completed tool calls when the
// runner uses SkipCompletedTools / CompletedToolCalls (VAL-ORCH-007).
func (m *Manager) Resume(ctx context.Context, taskID string, runner RunnerFunc) (TaskRun, error) {
	if m == nil {
		return TaskRun{}, fmt.Errorf("orchestrator: manager is nil")
	}
	if runner == nil {
		return TaskRun{}, fmt.Errorf("orchestrator: runner is required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TaskRun{}, fmt.Errorf("orchestrator: task_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.Lock()
	tr, ok := m.tasks[taskID]
	if !ok || tr == nil {
		m.mu.Unlock()
		return TaskRun{}, fmt.Errorf("orchestrator: task %q not found", taskID)
	}
	if tr.Status != StatusCancelled && tr.Status != StatusFailed {
		st := tr.Status
		m.mu.Unlock()
		return TaskRun{}, fmt.Errorf("orchestrator: task %q cannot resume (status=%s; need cancelled or failed)", taskID, st)
	}
	// Preserve prior context before resetting lifecycle fields.
	priorSummary := tr.ResultSummary
	priorResults := append([]string(nil), tr.Results...)
	priorLogs := append([]string(nil), tr.Logs...)
	priorTools := append([]ToolCallRecord(nil), tr.CompletedToolCalls...)
	priorPartial := tr.PartialOutput
	if priorPartial == "" {
		priorPartial = priorSummary
	}

	tr.Status = StatusQueued
	tr.Error = ""
	tr.EndedAt = time.Time{}
	tr.cancelRequested = false
	tr.cancel = nil
	// Keep CompletedToolCalls / Results / Logs / PartialOutput intact.
	tr.CompletedToolCalls = priorTools
	tr.Results = priorResults
	tr.Logs = priorLogs
	tr.PartialOutput = priorPartial
	if tr.ResultSummary == "" && priorSummary != "" {
		tr.ResultSummary = priorSummary
	}
	// Note resume in logs for evidence.
	tr.Logs = append(tr.Logs, fmt.Sprintf("resume: prior_status preserved tools=%d partial=%q",
		len(priorTools), truncate(priorPartial, 80)))
	m.mu.Unlock()

	// Run single-task body with preserved TaskRun pointer.
	taskCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	tr.cancel = cancel
	tr.Status = StatusRunning
	tr.StartedAt = time.Now().UTC()
	// Keep original CreatedAt; refresh StartedAt for this attempt.
	m.mu.Unlock()
	defer cancel()

	m.emitter()(Event{
		Type:      EventTaskSpawned,
		TaskID:    tr.TaskID,
		ParentID:  tr.ParentSessionID,
		Category:  tr.Category,
		Status:    StatusRunning,
		Detail:    "resume " + firstNonEmpty(tr.Description, truncate(tr.Prompt, 80)),
		Prompt:    tr.Prompt,
		Depth:     tr.Depth,
		StartedAt: tr.StartedAt,
	})
	m.persistRun(tr)

	// Provide prior completed tools to runner via run field; runner should skip them.
	summary, results, runErr := runner(taskCtx, tr)

	ended := time.Now().UTC()
	m.mu.Lock()
	tr.EndedAt = ended
	// Prefer cancelled if cancel was requested or ctx cancelled.
	if tr.cancelRequested || taskCtx.Err() != nil {
		tr.Status = StatusCancelled
		if runErr != nil {
			tr.Error = runErr.Error()
		} else {
			tr.Error = context.Canceled.Error()
		}
		// Final summary includes pre-cancel partial output (VAL-ORCH-007).
		tr.ResultSummary = mergePartial(priorPartial, summary)
	} else if runErr != nil {
		tr.Status = StatusFailed
		tr.Error = runErr.Error()
		tr.ResultSummary = mergePartial(priorPartial, summary)
		if strings.TrimSpace(tr.ResultSummary) == "" {
			tr.ResultSummary = "failed: " + runErr.Error()
		}
	} else {
		tr.Status = StatusCompleted
		tr.Error = ""
		// Final summary includes pre-cancel partial + new output.
		tr.ResultSummary = mergePartial(priorPartial, summary)
	}
	if len(results) > 0 {
		// Append new results; keep prior.
		tr.Results = append(tr.Results, results...)
	}
	if summary != "" {
		tr.Logs = append(tr.Logs, "resume_result="+summary)
	}
	// PartialOutput accumulates latest coherent summary.
	if tr.ResultSummary != "" {
		tr.PartialOutput = tr.ResultSummary
	}
	snap := tr.Snapshot()
	m.mu.Unlock()

	m.ReleaseAllLocks(tr.TaskID)
	m.persistRun(tr)

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
	return snap, nil
}

// LoadFromSnapshot installs a previously persisted TaskRun into the Manager
// (used when hydrating from store on process restart before resume).
func (m *Manager) LoadFromSnapshot(run TaskRun) {
	if m == nil || strings.TrimSpace(run.TaskID) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := run
	tr.cancel = nil
	// Copy slices.
	if run.Results != nil {
		tr.Results = append([]string(nil), run.Results...)
	}
	if run.Logs != nil {
		tr.Logs = append([]string(nil), run.Logs...)
	}
	if run.CompletedToolCalls != nil {
		tr.CompletedToolCalls = append([]ToolCallRecord(nil), run.CompletedToolCalls...)
	}
	m.tasks[tr.TaskID] = &tr
	found := false
	for _, id := range m.order {
		if id == tr.TaskID {
			found = true
			break
		}
	}
	if !found {
		m.order = append(m.order, tr.TaskID)
	}
}

// RecordToolCall appends a completed tool call to the live task (resume skip list).
func (m *Manager) RecordToolCall(taskID string, rec ToolCallRecord) {
	if m == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tr, ok := m.tasks[taskID]
	if !ok || tr == nil {
		return
	}
	tr.CompletedToolCalls = append(tr.CompletedToolCalls, rec)
	if rec.Content != "" {
		tr.PartialOutput = strings.TrimSpace(tr.PartialOutput + "\n" + rec.Name + ": " + truncate(rec.Content, 200))
	}
}

// HasCompletedTool reports whether tool call id (or name+args fingerprint) already ran.
func (run *TaskRun) HasCompletedTool(id, name, args string) bool {
	if run == nil {
		return false
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	args = strings.TrimSpace(args)
	for _, t := range run.CompletedToolCalls {
		if id != "" && t.ID == id {
			return true
		}
		if id == "" && t.Name == name && strings.TrimSpace(t.Args) == args {
			return true
		}
	}
	return false
}

// SkipCompletedTools is a helper for runners: if the tool already completed on
// this task, return the stored content without re-executing (VAL-ORCH-007).
func (run *TaskRun) SkipCompletedTools(id, name, args string) (content string, skipped bool) {
	if run == nil {
		return "", false
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	args = strings.TrimSpace(args)
	for _, t := range run.CompletedToolCalls {
		if id != "" && t.ID == id {
			return t.Content, true
		}
		if id == "" && t.Name == name && strings.TrimSpace(t.Args) == args {
			return t.Content, true
		}
	}
	return "", false
}

func (m *Manager) finishCancelled(tr *TaskRun, detail string) {
	if tr == nil {
		return
	}
	ended := time.Now().UTC()
	m.mu.Lock()
	tr.Status = StatusCancelled
	tr.EndedAt = ended
	if tr.Error == "" {
		tr.Error = context.Canceled.Error()
	}
	if tr.ResultSummary == "" && tr.PartialOutput != "" {
		tr.ResultSummary = tr.PartialOutput
	}
	if tr.ResultSummary == "" {
		tr.ResultSummary = "cancelled"
	}
	snap := tr.Snapshot()
	m.mu.Unlock()
	m.ReleaseAllLocks(tr.TaskID)
	m.persistRun(tr)
	m.emitter()(Event{
		Type:      EventTaskDone,
		TaskID:    snap.TaskID,
		ParentID:  snap.ParentSessionID,
		Category:  snap.Category,
		Status:    StatusCancelled,
		Detail:    detail,
		Summary:   snap.ResultSummary,
		Prompt:    snap.Prompt,
		Depth:     snap.Depth,
		StartedAt: snap.StartedAt,
		EndedAt:   snap.EndedAt,
		Error:     snap.Error,
	})
}

func mergePartial(prior, next string) string {
	prior = strings.TrimSpace(prior)
	next = strings.TrimSpace(next)
	switch {
	case prior == "" && next == "":
		return ""
	case prior == "":
		return next
	case next == "":
		return prior
	case strings.Contains(next, prior):
		return next
	default:
		return prior + "\n" + next
	}
}
