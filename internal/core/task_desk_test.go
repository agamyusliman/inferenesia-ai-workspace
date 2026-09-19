package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
)

func TestListTasksAndCategoriesAPI(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	// Tasks are scoped to active workspace / session scratch.
	ws, err := svc.EnsureSessionWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	cats := svc.TaskCategories()
	if len(cats.Categories) < 3 {
		t.Fatalf("categories=%d", len(cats.Categories))
	}
	list := svc.ListTasks()
	if list.Tasks == nil {
		t.Fatal("tasks nil")
	}

	// Spawn + cancel via manager (panel list sees it when workspace_id matches).
	mgr := svc.OrchestratorManager()
	if mgr == nil {
		t.Fatal("mgr nil")
	}
	started := make(chan struct{})
	go func() {
		_, _, _ = mgr.SpawnParallel(context.Background(), []orchestrator.Spec{
			{
				Prompt:      "slow",
				Category:    orchestrator.CategoryExplore,
				Depth:       0,
				WorkspaceID: ws.ID,
			},
		}, func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
			close(started)
			select {
			case <-ctx.Done():
				return "partial", nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return "done", nil, nil
			}
		})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("no start")
	}
	tasks := svc.ListTasks()
	if len(tasks.Tasks) == 0 {
		t.Fatal("expected running task in list")
	}
	if tasks.Tasks[0].WorkspaceID != ws.ID {
		t.Fatalf("workspace_id=%q want %q", tasks.Tasks[0].WorkspaceID, ws.ID)
	}
	id := tasks.Tasks[0].TaskID
	if tasks.Tasks[0].Category == "" {
		t.Fatal("category empty")
	}
	view, err := svc.CancelTask(id)
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if view.Status != string(orchestrator.StatusCancelled) && view.Status != "cancelled" {
		// May still be settling; re-list.
		time.Sleep(50 * time.Millisecond)
		again := svc.ListTasks()
		found := false
		for _, trow := range again.Tasks {
			if trow.TaskID == id && trow.Status == "cancelled" {
				found = true
			}
		}
		if !found && view.Status != "cancelled" {
			t.Fatalf("cancel status=%s view=%+v list=%+v", view.Status, view, again.Tasks)
		}
	}

	// Resume
	resumed, err := svc.ResumeTask(context.Background(), id)
	if err != nil {
		t.Fatalf("ResumeTask: %v", err)
	}
	if resumed.TaskID != id {
		t.Fatalf("id mismatch %s", resumed.TaskID)
	}
	if !strings.Contains(resumed.Summary, "partial") && resumed.Status == "" {
		t.Logf("resume summary=%q status=%s", resumed.Summary, resumed.Status)
	}
	_ = filepath.Join(home, "sessions.db") // store path used by persist
}
