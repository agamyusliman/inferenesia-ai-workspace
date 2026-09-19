package plan_test

import (
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
)

func TestSeedPendingCreatesPendingTodos(t *testing.T) {
	list := plan.NewTodoList()
	items := list.SeedPending([]string{"  explore codebase  ", "", "write tests", "ship"})
	if len(items) != 3 {
		t.Fatalf("got %d items: %+v", len(items), items)
	}
	for _, it := range items {
		if it.Status != plan.TodoPending {
			t.Fatalf("expected pending, got %s for %q", it.Status, it.Content)
		}
		if it.ID == "" {
			t.Fatal("expected generated id")
		}
	}
	if items[0].Content != "explore codebase" {
		t.Fatalf("content=%q", items[0].Content)
	}
}

func TestTodoStatusLifecycle(t *testing.T) {
	list := plan.NewTodoList()
	items := list.SeedPending([]string{"step one"})
	id := items[0].ID

	updated, err := list.UpdateStatus(id, plan.TodoInProgress)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != plan.TodoInProgress {
		t.Fatalf("status=%s", updated.Status)
	}

	updated, err = list.UpdateStatus(id, plan.TodoCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != plan.TodoCompleted {
		t.Fatalf("status=%s", updated.Status)
	}

	snap := list.Snapshot()
	if len(snap) != 1 || snap[0].Status != plan.TodoCompleted {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestRejectDoesNotSeedTodos(t *testing.T) {
	store := plan.NewProposalStore()
	list := plan.NewTodoList()

	p := store.Set("Title", "Summary", "Body", []string{"a", "b"})
	if p.Status != plan.ProposalPending {
		t.Fatalf("status=%s", p.Status)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("steps=%v", p.Steps)
	}

	rejected, err := store.Reject()
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != plan.ProposalRejected {
		t.Fatalf("rejected status=%s", rejected.Status)
	}
	if len(rejected.Steps) != 0 {
		t.Fatalf("reject must not expose steps for seeding: %v", rejected.Steps)
	}
	// Todo panel remains empty (VAL-PLAN-004).
	if list.Len() != 0 {
		t.Fatalf("todos should stay empty, got %d", list.Len())
	}
	if store.Get() != nil {
		t.Fatal("no pending after reject")
	}
}

func TestApproveSeedsTodos(t *testing.T) {
	store := plan.NewProposalStore()
	list := plan.NewTodoList()

	_ = store.Set("Impl", "do work", "## plan", []string{"one", "two"})
	approved, err := store.Approve()
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != plan.ProposalApproved {
		t.Fatalf("status=%s", approved.Status)
	}
	items := list.SeedPending(approved.Steps)
	if len(items) != 2 {
		t.Fatalf("seeded=%+v", items)
	}
	for _, it := range items {
		if it.Status != plan.TodoPending {
			t.Fatalf("expected pending: %+v", it)
		}
	}
	if store.Get() != nil {
		t.Fatal("no pending after approve")
	}
}

func TestReplaceFullList(t *testing.T) {
	list := plan.NewTodoList()
	list.SeedPending([]string{"old"})
	got := list.Replace([]plan.TodoItem{
		{Content: "new-a", Status: plan.TodoInProgress},
		{Content: "new-b", Status: plan.TodoCompleted},
	})
	if len(got) != 2 {
		t.Fatalf("got=%+v", got)
	}
	if got[0].Status != plan.TodoInProgress || got[1].Status != plan.TodoCompleted {
		t.Fatalf("statuses wrong: %+v", got)
	}
}
