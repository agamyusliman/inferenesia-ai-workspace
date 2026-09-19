package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Task is a durable orchestrator TaskRun row (VAL-ORCH-006/007).
// Resume loads prior context (prompt + completed tool outputs) from this table.
type Task struct {
	TaskID          string
	ParentSessionID string
	// WorkspaceID scopes the task to a hub workspace / session scratch id.
	WorkspaceID     string
	Category        string
	Description     string
	Prompt          string
	Status          string // queued|running|completed|failed|cancelled
	ResultSummary   string
	Error           string
	Depth           int
	Results         []string
	Logs            []string
	// CompletedTools holds already-finished tool call records so resume
	// does not re-execute them (VAL-ORCH-007).
	CompletedTools []ToolCallRecord
	CreatedAt      time.Time
	StartedAt      time.Time
	EndedAt        time.Time
}

// ToolCallRecord is one completed (or skipped-on-resume) tool invocation.
type ToolCallRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Args    string `json:"args,omitempty"`
	Content string `json:"content,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// ensureTasksTable is called from migrate().
func tasksSchema() string {
	// Do not create idx_tasks_workspace here: legacy DBs already have `tasks`
	// without workspace_id, so CREATE TABLE IF NOT EXISTS is a no-op and an
	// index on a missing column fails before ensureTasksWorkspaceColumn runs.
	return `
CREATE TABLE IF NOT EXISTS tasks (
  task_id TEXT PRIMARY KEY,
  parent_session_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT 'explore',
  description TEXT NOT NULL DEFAULT '',
  prompt TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  result_summary TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  depth INTEGER NOT NULL DEFAULT 0,
  results_json TEXT NOT NULL DEFAULT '[]',
  logs_json TEXT NOT NULL DEFAULT '[]',
  completed_tools_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  started_at TEXT NOT NULL DEFAULT '',
  ended_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_session_id);
CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at);
`
}

// ensureTasksWorkspaceColumn adds workspace_id to existing DBs (additive).
func (s *Store) ensureTasksWorkspaceColumn() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	rows, err := s.db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	has := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "workspace_id" {
			has = true
		}
	}
	if has {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE tasks ADD COLUMN workspace_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("store: alter tasks workspace_id: %w", err)
	}
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_workspace ON tasks(workspace_id)`)
	return nil
}

// UpsertTask inserts or replaces a task row (idempotent for status updates).
func (s *Store) UpsertTask(t Task) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	id := strings.TrimSpace(t.TaskID)
	if id == "" {
		return fmt.Errorf("store: task_id is required")
	}
	now := time.Now().UTC()
	created := t.CreatedAt
	if created.IsZero() {
		created = now
	}
	resultsJSON, err := json.Marshal(t.Results)
	if err != nil {
		return fmt.Errorf("store: marshal results: %w", err)
	}
	if t.Results == nil {
		resultsJSON = []byte("[]")
	}
	logsJSON, err := json.Marshal(t.Logs)
	if err != nil {
		return fmt.Errorf("store: marshal logs: %w", err)
	}
	if t.Logs == nil {
		logsJSON = []byte("[]")
	}
	toolsJSON, err := json.Marshal(t.CompletedTools)
	if err != nil {
		return fmt.Errorf("store: marshal completed_tools: %w", err)
	}
	if t.CompletedTools == nil {
		toolsJSON = []byte("[]")
	}
	status := strings.TrimSpace(t.Status)
	if status == "" {
		status = "queued"
	}
	cat := strings.TrimSpace(t.Category)
	if cat == "" {
		cat = "explore"
	}
	_, err = s.db.Exec(
		`INSERT INTO tasks (
		  task_id, parent_session_id, workspace_id, category, description, prompt, status,
		  result_summary, error, depth, results_json, logs_json, completed_tools_json,
		  created_at, started_at, ended_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
		  parent_session_id=excluded.parent_session_id,
		  workspace_id=excluded.workspace_id,
		  category=excluded.category,
		  description=excluded.description,
		  prompt=excluded.prompt,
		  status=excluded.status,
		  result_summary=excluded.result_summary,
		  error=excluded.error,
		  depth=excluded.depth,
		  results_json=excluded.results_json,
		  logs_json=excluded.logs_json,
		  completed_tools_json=excluded.completed_tools_json,
		  started_at=excluded.started_at,
		  ended_at=excluded.ended_at`,
		id,
		t.ParentSessionID,
		strings.TrimSpace(t.WorkspaceID),
		cat,
		t.Description,
		t.Prompt,
		status,
		t.ResultSummary,
		t.Error,
		t.Depth,
		string(resultsJSON),
		string(logsJSON),
		string(toolsJSON),
		created.UTC().Format(timestampFormat),
		formatOptionalTime(t.StartedAt),
		formatOptionalTime(t.EndedAt),
	)
	if err != nil {
		return fmt.Errorf("store: upsert task: %w", err)
	}
	return nil
}

// GetTask returns one task by id.
func (s *Store) GetTask(taskID string) (Task, error) {
	if s == nil || s.db == nil {
		return Task{}, fmt.Errorf("store: closed")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Task{}, fmt.Errorf("store: task_id is required")
	}
	row := s.db.QueryRow(
		`SELECT task_id, parent_session_id, workspace_id, category, description, prompt, status,
		        result_summary, error, depth, results_json, logs_json, completed_tools_json,
		        created_at, started_at, ended_at
		 FROM tasks WHERE task_id = ?`,
		taskID,
	)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return Task{}, fmt.Errorf("store: task %q not found", taskID)
	}
	if err != nil {
		return Task{}, fmt.Errorf("store: get task: %w", err)
	}
	return t, nil
}

// ListTasks returns tasks newest-first. limit ≤ 0 means no limit (cap 500).
func (s *Store) ListTasks(limit int) ([]Task, error) {
	return s.ListTasksByWorkspace("", limit)
}

// ListTasksByWorkspace returns tasks for workspaceID newest-first.
// Empty workspaceID returns all tasks.
func (s *Store) ListTasksByWorkspace(workspaceID string, limit int) ([]Task, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store: closed")
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	ws := strings.TrimSpace(workspaceID)
	var (
		rows *sql.Rows
		err  error
	)
	if ws == "" {
		rows, err = s.db.Query(
			`SELECT task_id, parent_session_id, workspace_id, category, description, prompt, status,
			        result_summary, error, depth, results_json, logs_json, completed_tools_json,
			        created_at, started_at, ended_at
			 FROM tasks ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT task_id, parent_session_id, workspace_id, category, description, prompt, status,
			        result_summary, error, depth, results_json, logs_json, completed_tools_json,
			        created_at, started_at, ended_at
			 FROM tasks WHERE workspace_id = ? ORDER BY created_at DESC LIMIT ?`,
			ws, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list tasks: %w", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Task{}
	}
	return out, nil
}

// DeleteTask removes a task row (best-effort cleanup).
func (s *Store) DeleteTask(taskID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("store: task_id is required")
	}
	_, err := s.db.Exec(`DELETE FROM tasks WHERE task_id = ?`, taskID)
	if err != nil {
		return fmt.Errorf("store: delete task: %w", err)
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanTask(row scannable) (Task, error) {
	var (
		t                                                             Task
		resultsJSON, logsJSON, toolsJSON                              string
		createdAt, startedAt, endedAt                                 string
		resultSummary, errText, parent, ws, cat, desc, prompt, st     string
		depth                                                         int
		id                                                            string
	)
	if err := row.Scan(
		&id, &parent, &ws, &cat, &desc, &prompt, &st,
		&resultSummary, &errText, &depth, &resultsJSON, &logsJSON, &toolsJSON,
		&createdAt, &startedAt, &endedAt,
	); err != nil {
		return Task{}, err
	}
	t.TaskID = id
	t.ParentSessionID = parent
	t.WorkspaceID = ws
	t.Category = cat
	t.Description = desc
	t.Prompt = prompt
	t.Status = st
	t.ResultSummary = resultSummary
	t.Error = errText
	t.Depth = depth
	t.CreatedAt = parseTime(createdAt)
	t.StartedAt = parseTime(startedAt)
	t.EndedAt = parseTime(endedAt)
	t.Results = decodeStringSlice(resultsJSON)
	t.Logs = decodeStringSlice(logsJSON)
	t.CompletedTools = decodeToolRecords(toolsJSON)
	return t, nil
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" || raw == "null" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func decodeToolRecords(raw string) []ToolCallRecord {
	if strings.TrimSpace(raw) == "" || raw == "null" {
		return []ToolCallRecord{}
	}
	var out []ToolCallRecord
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []ToolCallRecord{}
	}
	if out == nil {
		return []ToolCallRecord{}
	}
	return out
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timestampFormat)
}
