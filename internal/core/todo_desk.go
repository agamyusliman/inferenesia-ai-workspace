package core

import (
	"fmt"
	"strings"
	"sync"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
)

// ChatEventPlanProposed is emitted when the agent proposes a plan (VAL-PLAN-003).
const ChatEventPlanProposed ChatEventType = "PlanProposed"

// ChatEventTodoUpdated is emitted when the todo list changes (seed, write, status).
// The desktop todo panel re-fetches / applies the payload in real time (VAL-PLAN-005).
const ChatEventTodoUpdated ChatEventType = "TodoUpdated"

// PlanProposalView is the JSON shape for the pending plan dialog.
type PlanProposalView struct {
	ID        string   `json:"id"`
	Title     string   `json:"title,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Body      string   `json:"body,omitempty"`
	Steps     []string `json:"steps"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// TodoListView is the todo panel payload (VAL-PLAN-003/005).
type TodoListView struct {
	Todos   []plan.TodoItem `json:"todos"`
	Count   int             `json:"count"`
	Product string          `json:"product,omitempty"`
}

// PlanApproveResult is returned after Approve seeds todos and exits Plan Mode.
type PlanApproveResult struct {
	Plan   PlanProposalView `json:"plan"`
	Todos  TodoListView     `json:"todos"`
	Mode   PlanModeState    `json:"mode"`
	Hub    HubState         `json:"hub"`
	Seeded int              `json:"seeded"`
}

// PlanRejectResult is returned after Reject discards a plan without seeding (VAL-PLAN-004).
type PlanRejectResult struct {
	Plan  PlanProposalView `json:"plan"`
	Todos TodoListView     `json:"todos"`
	Mode  PlanModeState    `json:"mode"`
	Hub   HubState         `json:"hub"`
}

// planTodoState holds proposal + todos on Service (lazy-init).
type planTodoState struct {
	proposals *plan.ProposalStore
	todos     *plan.TodoList
	// perWorkspace caches todo snapshots when switching workspaces so lists
	// reappear when the user returns (UI clears while away).
	perWorkspace map[string][]plan.TodoItem
	activeWS     string
	// emit hooks the active chat stream for PlanProposed / TodoUpdated.
	emitMu sync.Mutex
	emit   func(ChatEvent)
}

func (s *Service) ensurePlanTodos() *planTodoState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.planTodos == nil {
		s.planTodos = &planTodoState{
			proposals:    plan.NewProposalStore(),
			todos:        plan.NewTodoList(),
			perWorkspace: make(map[string][]plan.TodoItem),
		}
	}
	return s.planTodos
}

// BindTodosToWorkspace saves the current todo list under the previous workspace
// id and loads the target workspace's cached list (or empty).
func (s *Service) BindTodosToWorkspace(newWorkspaceID string) {
	if s == nil {
		return
	}
	pt := s.ensurePlanTodos()
	newID := strings.TrimSpace(newWorkspaceID)
	if pt.perWorkspace == nil {
		pt.perWorkspace = make(map[string][]plan.TodoItem)
	}
	if pt.activeWS != "" && pt.activeWS != newID {
		pt.perWorkspace[pt.activeWS] = pt.todos.Snapshot()
	}
	pt.todos.Clear()
	if newID != "" {
		if snap, ok := pt.perWorkspace[newID]; ok && len(snap) > 0 {
			pt.todos.Replace(snap)
		}
	}
	pt.activeWS = newID
	pt.proposals.Clear()
}

func (pt *planTodoState) setEmit(emit func(ChatEvent)) {
	if pt == nil {
		return
	}
	pt.emitMu.Lock()
	defer pt.emitMu.Unlock()
	pt.emit = emit
}

func (pt *planTodoState) emitEvent(ev ChatEvent) {
	if pt == nil {
		return
	}
	pt.emitMu.Lock()
	emit := pt.emit
	pt.emitMu.Unlock()
	if emit == nil {
		return
	}
	defer func() { _ = recover() }()
	emit(ev)
}

// --- TodoBackend (tools.TodoBackend) ---

// ProposePlan records a pending plan and emits PlanProposed (VAL-PLAN-003).
func (s *Service) ProposePlan(title, summary, body string, steps []string) (plan.ProposedPlan, error) {
	if s == nil {
		return plan.ProposedPlan{}, fmt.Errorf("core: service is nil")
	}
	pt := s.ensurePlanTodos()
	p := pt.proposals.Set(title, summary, body, steps)
	// Surface to UI / SSE so Approve|Reject dialog can open.
	detail := strings.TrimSpace(p.Summary)
	if detail == "" {
		detail = strings.TrimSpace(p.Title)
	}
	if detail == "" {
		detail = "Plan proposed"
	}
	pt.emitEvent(ChatEvent{
		Type:   ChatEventPlanProposed,
		Name:   p.ID,
		Detail: detail,
		Text:   p.Body,
		// Path unused; put step count in Final-like fields via Detail.
		Final: fmt.Sprintf("%d steps", len(p.Steps)),
	})
	// Name holds plan id for clients that key on event.name.
	return p, nil
}

// ListTodos returns the current session checklist.
func (s *Service) ListTodos() []plan.TodoItem {
	if s == nil {
		return nil
	}
	return s.ensurePlanTodos().todos.Snapshot()
}

// ReplaceTodos full-replaces the checklist and emits TodoUpdated (VAL-PLAN-005).
func (s *Service) ReplaceTodos(items []plan.TodoItem) ([]plan.TodoItem, error) {
	if s == nil {
		return nil, fmt.Errorf("core: service is nil")
	}
	pt := s.ensurePlanTodos()
	out := pt.todos.Replace(items)
	s.emitTodoUpdated(pt, out)
	return out, nil
}

// UpdateTodo patches one item and emits TodoUpdated (pending→in_progress→completed).
func (s *Service) UpdateTodo(id string, content *string, status *plan.TodoStatus) (plan.TodoItem, error) {
	if s == nil {
		return plan.TodoItem{}, fmt.Errorf("core: service is nil")
	}
	pt := s.ensurePlanTodos()
	item, err := pt.todos.Update(id, content, status)
	if err != nil {
		return plan.TodoItem{}, err
	}
	s.emitTodoUpdated(pt, pt.todos.Snapshot())
	return item, nil
}

func (s *Service) emitTodoUpdated(pt *planTodoState, items []plan.TodoItem) {
	// Structured event: detail holds count; UI also polls GET /api/todos.
	detail := fmt.Sprintf("count=%d", len(items))
	// Encode statuses briefly for stream logs / tests without full JSON body.
	var parts []string
	for _, it := range items {
		parts = append(parts, fmt.Sprintf("%s:%s", it.ID, it.Status))
	}
	text := strings.Join(parts, ",")
	pt.emitEvent(ChatEvent{
		Type:   ChatEventTodoUpdated,
		Detail: detail,
		Text:   text,
		Final:  fmt.Sprintf("%d", len(items)),
	})
	s.setStatus(fmt.Sprintf("Todos updated (%d)", len(items)))
}

// GetTodos returns the todo panel snapshot.
func (s *Service) GetTodos() TodoListView {
	items := s.ListTodos()
	if items == nil {
		items = []plan.TodoItem{}
	}
	return TodoListView{
		Todos:   items,
		Count:   len(items),
		Product: s.BrandName(),
	}
}

// GetPendingPlan returns the pending PlanProposed proposal for the UI dialog.
func (s *Service) GetPendingPlan() *PlanProposalView {
	if s == nil {
		return nil
	}
	p := s.ensurePlanTodos().proposals.Get()
	if p == nil {
		return nil
	}
	return proposalToView(p)
}

// ApprovePlan seeds todos from the pending plan (pending status) and exits Plan Mode
// (VAL-PLAN-003, VAL-PLAN-007).
func (s *Service) ApprovePlan() (PlanApproveResult, error) {
	if s == nil {
		return PlanApproveResult{}, fmt.Errorf("core: service is nil")
	}
	pt := s.ensurePlanTodos()
	approved, err := pt.proposals.Approve()
	if err != nil {
		return PlanApproveResult{}, err
	}
	// Seed todos with initial pending status (VAL-PLAN-003).
	seeded := pt.todos.SeedPending(approved.Steps)
	s.emitTodoUpdated(pt, seeded)
	// Exit Plan Mode so write tools are re-enabled for execution (VAL-PLAN-007).
	mode, _ := s.ExitPlanMode("approve")
	s.setStatus(fmt.Sprintf("Plan approved — seeded %d todos", len(seeded)))
	return PlanApproveResult{
		Plan:   *proposalToView(&approved),
		Todos:  TodoListView{Todos: seeded, Count: len(seeded), Product: s.BrandName()},
		Mode:   mode,
		Hub:    s.GetHubState(),
		Seeded: len(seeded),
	}, nil
}

// RejectPlan discards the pending plan without creating todos (VAL-PLAN-004).
// Todo list is left unchanged (not cleared).
func (s *Service) RejectPlan() (PlanRejectResult, error) {
	if s == nil {
		return PlanRejectResult{}, fmt.Errorf("core: service is nil")
	}
	pt := s.ensurePlanTodos()
	// Snapshot todos before reject to prove unchanged.
	before := pt.todos.Snapshot()
	rejected, err := pt.proposals.Reject()
	if err != nil {
		return PlanRejectResult{}, err
	}
	// Do NOT seed; do NOT clear existing todos (VAL-PLAN-004).
	after := pt.todos.Snapshot()
	if len(after) != len(before) {
		// Should never happen; guard for safety.
		return PlanRejectResult{}, fmt.Errorf("core: reject changed todos unexpectedly")
	}
	// Exit Plan Mode on reject as well so user can resume normal work.
	mode, _ := s.ExitPlanMode("reject")
	s.setStatus("Plan rejected — no todos created")
	return PlanRejectResult{
		Plan:  *proposalToView(&rejected),
		Todos: TodoListView{Todos: after, Count: len(after), Product: s.BrandName()},
		Mode:  mode,
		Hub:   s.GetHubState(),
	}, nil
}

func proposalToView(p *plan.ProposedPlan) *PlanProposalView {
	if p == nil {
		return nil
	}
	steps := p.Steps
	if steps == nil {
		steps = []string{}
	}
	return &PlanProposalView{
		ID:        p.ID,
		Title:     p.Title,
		Summary:   p.Summary,
		Body:      p.Body,
		Steps:     steps,
		Status:    string(p.Status),
		CreatedAt: p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}


