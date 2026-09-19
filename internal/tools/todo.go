package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
)

// Todo / plan proposal tools (VAL-PLAN-003/004/005).
const (
	NameProposePlan = "propose_plan"
	NameTodoWrite   = "todo_write"
	NameTodoUpdate  = "todo_update"
	NameTodoList    = "todo_list"
)

// TodoBackend is implemented by core.Service plan/todo store.
// Tools never own durable UI state; they call into the Service.
type TodoBackend interface {
	// ProposePlan records a pending plan and emits PlanProposed (VAL-PLAN-003).
	ProposePlan(title, summary, body string, steps []string) (plan.ProposedPlan, error)
	// ListTodos returns the current checklist.
	ListTodos() []plan.TodoItem
	// ReplaceTodos full-replaces the checklist (todowrite style).
	ReplaceTodos(items []plan.TodoItem) ([]plan.TodoItem, error)
	// UpdateTodo patches one item by id.
	UpdateTodo(id string, content *string, status *plan.TodoStatus) (plan.TodoItem, error)
}

// SetTodoBackend wires session todos + plan proposal tools.
func (r *Registry) SetTodoBackend(tb TodoBackend) {
	if r == nil {
		return
	}
	r.todo = tb
}

// TodoBackend returns the attached backend (may be nil).
func (r *Registry) TodoBackend() TodoBackend {
	if r == nil {
		return nil
	}
	return r.todo
}

func todoDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name: NameProposePlan,
				Description: "Propose an implementation plan for the user to Approve or Reject. " +
					"Use while planning (Plan Mode). On approve, steps become todos (pending). " +
					"On reject, no todos are created. Only one pending plan at a time.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title": { "type": "string", "description": "Short plan title" },
    "summary": { "type": "string", "description": "One-line summary of the approach" },
    "body": { "type": "string", "description": "Markdown plan body (intent, risks, test plan)" },
    "steps": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Ordered checklist steps that become todos when the user approves"
    }
  },
  "required": ["steps"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name: NameTodoWrite,
				Description: "Replace the full session todo checklist. Prefer after a plan is approved. " +
					"Statuses: pending | in_progress | completed | cancelled. " +
					"Typically only one item should be in_progress at a time.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "content": { "type": "string" },
          "status": { "type": "string", "enum": ["pending", "in_progress", "completed", "cancelled"] },
          "priority": { "type": "string" }
        },
        "required": ["content"]
      }
    }
  },
  "required": ["todos"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameTodoUpdate,
				Description: "Update one todo by id (status and/or content). Use to move pending → in_progress → completed as work progresses.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "Todo id from todo_list" },
    "status": { "type": "string", "enum": ["pending", "in_progress", "completed", "cancelled"] },
    "content": { "type": "string", "description": "Optional new content" }
  },
  "required": ["id"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameTodoList,
				Description: "List the current session todos (id, content, status). Read-only.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
	}
}

type proposePlanArgs struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Body    string   `json:"body"`
	Steps   []string `json:"steps"`
}

type todoWriteArgs struct {
	Todos []struct {
		ID       string `json:"id"`
		Content  string `json:"content"`
		Status   string `json:"status"`
		Priority string `json:"priority"`
	} `json:"todos"`
}

type todoUpdateArgs struct {
	ID      string  `json:"id"`
	Status  *string `json:"status"`
	Content *string `json:"content"`
}

func (r *Registry) execProposePlan(call Call) Result {
	if r.todo == nil {
		return Result{Name: NameProposePlan, Content: "propose_plan: not configured", IsError: true}
	}
	var args proposePlanArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameProposePlan, Content: fmt.Sprintf("propose_plan: invalid arguments: %v", err), IsError: true}
	}
	if len(args.Steps) == 0 {
		return Result{Name: NameProposePlan, Content: "propose_plan: steps array is required (non-empty)", IsError: true}
	}
	p, err := r.todo.ProposePlan(args.Title, args.Summary, args.Body, args.Steps)
	if err != nil {
		return Result{Name: NameProposePlan, Content: err.Error(), IsError: true}
	}
	return Result{
		Name: NameProposePlan,
		Content: fmt.Sprintf(
			"PlanProposed id=%s title=%q steps=%d — waiting for user Approve/Reject. Do not start mutating work until the user approves.",
			p.ID, p.Title, len(p.Steps),
		),
	}
}

func (r *Registry) execTodoWrite(call Call) Result {
	if r.todo == nil {
		return Result{Name: NameTodoWrite, Content: "todo_write: not configured", IsError: true}
	}
	var args todoWriteArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameTodoWrite, Content: fmt.Sprintf("todo_write: invalid arguments: %v", err), IsError: true}
	}
	items := make([]plan.TodoItem, 0, len(args.Todos))
	for _, t := range args.Todos {
		items = append(items, plan.TodoItem{
			ID:       t.ID,
			Content:  t.Content,
			Status:   plan.TodoStatus(t.Status),
			Priority: t.Priority,
		})
	}
	out, err := r.todo.ReplaceTodos(items)
	if err != nil {
		return Result{Name: NameTodoWrite, Content: err.Error(), IsError: true}
	}
	b, _ := json.Marshal(out)
	return Result{Name: NameTodoWrite, Content: string(b)}
}

func (r *Registry) execTodoUpdate(call Call) Result {
	if r.todo == nil {
		return Result{Name: NameTodoUpdate, Content: "todo_update: not configured", IsError: true}
	}
	var args todoUpdateArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameTodoUpdate, Content: fmt.Sprintf("todo_update: invalid arguments: %v", err), IsError: true}
	}
	if strings.TrimSpace(args.ID) == "" {
		return Result{Name: NameTodoUpdate, Content: "todo_update: id is required", IsError: true}
	}
	var status *plan.TodoStatus
	if args.Status != nil {
		st := plan.TodoStatus(strings.TrimSpace(*args.Status))
		status = &st
	}
	item, err := r.todo.UpdateTodo(args.ID, args.Content, status)
	if err != nil {
		return Result{Name: NameTodoUpdate, Content: err.Error(), IsError: true}
	}
	b, _ := json.Marshal(item)
	return Result{Name: NameTodoUpdate, Content: string(b)}
}

func (r *Registry) execTodoList(call Call) Result {
	if r.todo == nil {
		return Result{Name: NameTodoList, Content: "todo_list: not configured", IsError: true}
	}
	items := r.todo.ListTodos()
	if items == nil {
		items = []plan.TodoItem{}
	}
	b, _ := json.Marshal(items)
	return Result{Name: NameTodoList, Content: string(b)}
}
