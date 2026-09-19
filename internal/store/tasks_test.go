package store

import (
	"path/filepath"
	"testing"
	"time"
)

// VAL-ORCH-006/007 store foundation: tasks table persists and reloads.
func TestTaskUpsertGetListAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sessions.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Now().UTC()
	task := Task{
		TaskID:          "task-abc",
		ParentSessionID: "sess-1",
		Category:        "explore",
		Description:     "find auth",
		Prompt:          "Find auth middleware",
		Status:          "cancelled",
		ResultSummary:   "partial-before-cancel",
		Error:           "context canceled",
		Depth:           1,
		Results:         []string{"a.go: hit"},
		Logs:            []string{"prompt=Find auth"},
		CompletedTools: []ToolCallRecord{
			{ID: "tc-1", Name: "read_file", Args: `{"path":"a.go"}`, Content: "body"},
		},
		CreatedAt: now,
		StartedAt: now,
		EndedAt:   now.Add(time.Second),
	}
	if err := s1.UpsertTask(task); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetTask("task-abc")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("status=%s", got.Status)
	}
	if got.ResultSummary != "partial-before-cancel" {
		t.Fatalf("summary=%s", got.ResultSummary)
	}
	if len(got.CompletedTools) != 1 || got.CompletedTools[0].Name != "read_file" {
		t.Fatalf("completed tools=%+v", got.CompletedTools)
	}
	if len(got.Results) != 1 {
		t.Fatalf("results=%v", got.Results)
	}

	list, err := s2.ListTasks(10)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(list) != 1 || list[0].TaskID != "task-abc" {
		t.Fatalf("list=%+v", list)
	}

	// Update status to completed on resume.
	got.Status = "completed"
	got.ResultSummary = "partial-before-cancel\nfinal"
	got.Error = ""
	if err := s2.UpsertTask(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	again, err := s2.GetTask("task-abc")
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "completed" {
		t.Fatalf("status after update=%s", again.Status)
	}
}
