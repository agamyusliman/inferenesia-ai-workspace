package core_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/plan"
)

// Integration-style HTTP checks for VAL-PLAN-003/004/005 against Service.Handler.
func TestPlanTodoHTTPLifecycle(t *testing.T) {
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	// Empty todos
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	if rr.Code != 200 {
		t.Fatalf("todos %d %s", rr.Code, rr.Body.String())
	}

	// VAL-PLAN-004: propose + reject leaves empty
	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}
	// Prefer hub POST /api/plan/propose (same Service path as propose_plan tool).
	proposeBody, _ := json.Marshal(map[string]any{
		"title":   "Reject me",
		"summary": "discard",
		"body":    "body",
		"steps":   []string{"step-a", "step-b"},
	})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/plan/propose", bytes.NewReader(proposeBody)))
	if rr.Code != 200 {
		t.Fatalf("propose: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/plan/pending", nil))
	if rr.Code != 200 || !bytes.Contains(rr.Body.Bytes(), []byte("Reject me")) {
		t.Fatalf("pending: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/plan/reject", bytes.NewReader([]byte("{}"))))
	if rr.Code != 200 {
		t.Fatalf("reject: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	var todos core.TodoListView
	if err := json.Unmarshal(rr.Body.Bytes(), &todos); err != nil {
		t.Fatal(err)
	}
	if todos.Count != 0 {
		t.Fatalf("reject seeded todos: %+v", todos)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/plan/pending", nil))
	if bytes.Contains(rr.Body.Bytes(), []byte("Reject me")) && !bytes.Contains(rr.Body.Bytes(), []byte(`"pending":null`)) {
		// pending should be null after reject
		var wrap map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &wrap)
		if wrap["pending"] != nil {
			t.Fatalf("pending still set: %s", rr.Body.String())
		}
	}

	// VAL-PLAN-003: approve seeds pending
	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProposePlan("Ship it", "do work", "## plan", []string{"explore", "implement", "verify"}); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/plan/approve", bytes.NewReader([]byte("{}"))))
	if rr.Code != 200 {
		t.Fatalf("approve: %s", rr.Body.String())
	}
	var ar core.PlanApproveResult
	if err := json.Unmarshal(rr.Body.Bytes(), &ar); err != nil {
		t.Fatal(err)
	}
	if ar.Seeded != 3 {
		t.Fatalf("seeded=%d", ar.Seeded)
	}
	for _, it := range ar.Todos.Todos {
		if it.Status != plan.TodoPending {
			t.Fatalf("status=%s content=%s", it.Status, it.Content)
		}
	}
	if ar.Mode.Active {
		t.Fatal("plan mode still active after approve")
	}

	// VAL-PLAN-005
	id := ar.Todos.Todos[0].ID
	for _, st := range []string{"in_progress", "completed"} {
		payload, _ := json.Marshal(map[string]any{"id": id, "status": st})
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/todos/update", bytes.NewReader(payload)))
		if rr.Code != 200 {
			t.Fatalf("update %s: %s", st, rr.Body.String())
		}
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	if err := json.Unmarshal(rr.Body.Bytes(), &todos); err != nil {
		t.Fatal(err)
	}
	if todos.Todos[0].Status != plan.TodoCompleted {
		t.Fatalf("want completed got %s", todos.Todos[0].Status)
	}
	if todos.Todos[1].Status != plan.TodoPending || todos.Todos[2].Status != plan.TodoPending {
		t.Fatalf("unexpected: %+v", todos.Todos)
	}

	// reject leaves existing todos unchanged
	if _, err := svc.ProposePlan("Another", "", "", []string{"nope"}); err != nil {
		t.Fatal(err)
	}
	before := todos.Count
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/plan/reject", bytes.NewReader([]byte("{}"))))
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	if err := json.Unmarshal(rr.Body.Bytes(), &todos); err != nil {
		t.Fatal(err)
	}
	if todos.Count != before {
		t.Fatalf("reject changed existing todos: before=%d after=%d", before, todos.Count)
	}
}
