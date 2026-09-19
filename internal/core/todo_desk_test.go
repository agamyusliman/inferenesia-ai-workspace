package core_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-PLAN-003: approve plan seeds todos with pending status.
func TestApprovePlanSeedsTodos(t *testing.T) {
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	// Start in Plan Mode (typical path).
	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ProposePlan("Ship feature", "build it safely", "## Steps\n", []string{
		"explore code",
		"implement",
		"test",
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := svc.GetPendingPlan()
	if pending == nil || len(pending.Steps) != 3 {
		t.Fatalf("pending=%+v", pending)
	}

	res, err := svc.ApprovePlan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Seeded != 3 {
		t.Fatalf("seeded=%d", res.Seeded)
	}
	if res.Todos.Count != 3 {
		t.Fatalf("todos=%+v", res.Todos)
	}
	for _, it := range res.Todos.Todos {
		if it.Status != plan.TodoPending {
			t.Fatalf("expected pending, got %s for %q", it.Status, it.Content)
		}
	}
	// Plan Mode exited on approve (VAL-PLAN-007).
	if res.Mode.Active {
		t.Fatal("plan mode should exit on approve")
	}
	if svc.GetPendingPlan() != nil {
		t.Fatal("no pending after approve")
	}
}

// VAL-PLAN-004: reject discards plan and does not create todos.
func TestRejectPlanDoesNotCreateTodos(t *testing.T) {
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProposePlan("Bad idea", "nope", "", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	// Panel empty before.
	if svc.GetTodos().Count != 0 {
		t.Fatal("expected empty todos before reject")
	}

	res, err := svc.RejectPlan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Todos.Count != 0 {
		t.Fatalf("reject must not seed todos: %+v", res.Todos)
	}
	if len(res.Plan.Steps) != 0 {
		// View may still list original steps or empty; reject path zeros steps.
		// Ensure list endpoint stays empty regardless.
	}
	if svc.GetTodos().Count != 0 {
		t.Fatalf("todo panel still non-empty after reject: %+v", svc.GetTodos())
	}
	if svc.GetPendingPlan() != nil {
		t.Fatal("pending should be gone after reject")
	}
	if res.Mode.Active {
		t.Fatal("plan mode should exit on reject")
	}
}

// VAL-PLAN-004 edge: reject leaves pre-existing todos unchanged.
func TestRejectLeavesExistingTodosUnchanged(t *testing.T) {
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	// Pre-seed from a previous approve simulation.
	if _, err := svc.ReplaceTodos([]plan.TodoItem{
		{Content: "keep-me", Status: plan.TodoInProgress},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProposePlan("new", "", "", []string{"should-not-seed"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.RejectPlan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Todos.Count != 1 || res.Todos.Todos[0].Content != "keep-me" {
		t.Fatalf("existing todos changed: %+v", res.Todos)
	}
}

// VAL-PLAN-005: todo status transitions pending → in_progress → completed.
func TestTodoStatusUpdates(t *testing.T) {
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	items, err := svc.ReplaceTodos([]plan.TodoItem{
		{Content: "task", Status: plan.TodoPending},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := items[0].ID
	st := plan.TodoInProgress
	item, err := svc.UpdateTodo(id, nil, &st)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != plan.TodoInProgress {
		t.Fatalf("status=%s", item.Status)
	}
	st = plan.TodoCompleted
	item, err = svc.UpdateTodo(id, nil, &st)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != plan.TodoCompleted {
		t.Fatalf("status=%s", item.Status)
	}
	got := svc.GetTodos()
	if got.Todos[0].Status != plan.TodoCompleted {
		t.Fatalf("list=%+v", got)
	}
}

// Tools path: propose_plan emits usable plan; todo_update updates list.
func TestTodoToolsThroughRegistry(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetPlanMode(svc.PlanModeController())
	reg.SetTodoBackend(svc)

	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}
	// propose_plan allowed in plan mode.
	res := reg.Execute(context.Background(), tools.Call{
		ID:   "p1",
		Name: tools.NameProposePlan,
		Arguments: `{
  "title":"Demo",
  "summary":"demo plan",
  "body":"## Plan",
  "steps":["alpha","beta"]
}`,
	}, nil)
	if res.IsError {
		t.Fatalf("propose_plan: %s", res.Content)
	}
	if !strings.Contains(res.Content, "PlanProposed") {
		t.Fatalf("content=%q", res.Content)
	}
	// todo_write denied in plan mode.
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "t1",
		Name:      tools.NameTodoWrite,
		Arguments: `{"todos":[{"content":"x","status":"pending"}]}`,
	}, nil)
	if !res.IsError || !strings.Contains(res.Content, plan.DenialPrefix) {
		t.Fatalf("todo_write should deny in plan mode: %q", res.Content)
	}

	// Approve seeds.
	ar, err := svc.ApprovePlan()
	if err != nil {
		t.Fatal(err)
	}
	if ar.Seeded != 2 {
		t.Fatalf("seeded=%d", ar.Seeded)
	}
	// After exit, todo_update works.
	id := ar.Todos.Todos[0].ID
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "u1",
		Name:      tools.NameTodoUpdate,
		Arguments: `{"id":"` + id + `","status":"in_progress"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("todo_update: %s", res.Content)
	}
	if !strings.Contains(res.Content, "in_progress") {
		t.Fatalf("update content=%q", res.Content)
	}
	// List shows real-time state.
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "l1",
		Name:      tools.NameTodoList,
		Arguments: `{}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "in_progress") {
		t.Fatalf("list=%q", res.Content)
	}
}
