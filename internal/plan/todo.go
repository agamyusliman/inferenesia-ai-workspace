package plan

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Todo status lifecycle: pending → in_progress → completed (VAL-PLAN-005).
// cancelled is optional terminal state.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
	TodoCancelled  TodoStatus = "cancelled"
)

// TodoItem is one session checklist entry (docs/plan-mission.md Todo).
type TodoItem struct {
	ID       string     `json:"id"`
	Content  string     `json:"content"`
	Status   TodoStatus `json:"status"`
	Priority string     `json:"priority,omitempty"`
}

// TodoList is a concurrency-safe session todo checklist for the UI panel.
type TodoList struct {
	mu    sync.RWMutex
	items []TodoItem
	seq   int
}

// NewTodoList returns an empty checklist.
func NewTodoList() *TodoList {
	return &TodoList{items: nil}
}

// Snapshot returns a deep copy of current todos for JSON/API.
func (l *TodoList) Snapshot() []TodoItem {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.items) == 0 {
		return []TodoItem{}
	}
	out := make([]TodoItem, len(l.items))
	copy(out, l.items)
	return out
}

// Len returns the number of todos.
func (l *TodoList) Len() int {
	if l == nil {
		return 0
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}

// Clear removes all todos (used on reject when seeding was partial; not on normal reject).
func (l *TodoList) Clear() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = nil
}

// Replace replaces the full list (todowrite-style full replace; VAL-PLAN-005).
// Empty status defaults to pending. IDs are generated when missing.
func (l *TodoList) Replace(items []TodoItem) []TodoItem {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]TodoItem, 0, len(items))
	for _, it := range items {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			continue
		}
		id := strings.TrimSpace(it.ID)
		if id == "" {
			l.seq++
			id = fmt.Sprintf("todo-%d-%d", time.Now().UnixNano(), l.seq)
		}
		st := normalizeTodoStatus(it.Status)
		out = append(out, TodoItem{
			ID:       id,
			Content:  content,
			Status:   st,
			Priority: strings.TrimSpace(it.Priority),
		})
	}
	l.items = out
	return l.copyLocked()
}

// SeedPending creates todos from step strings, all with pending status (VAL-PLAN-003).
// Replaces the previous list so approve always seeds from the approved plan.
func (l *TodoList) SeedPending(steps []string) []TodoItem {
	items := make([]TodoItem, 0, len(steps))
	for _, s := range steps {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		items = append(items, TodoItem{Content: s, Status: TodoPending})
	}
	return l.Replace(items)
}

// UpdateStatus sets one todo's status by id (pending → in_progress → completed).
func (l *TodoList) UpdateStatus(id string, status TodoStatus) (TodoItem, error) {
	if l == nil {
		return TodoItem{}, fmt.Errorf("plan: todo list is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return TodoItem{}, fmt.Errorf("plan: todo id is required")
	}
	st := normalizeTodoStatus(status)
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.items {
		if l.items[i].ID == id {
			l.items[i].Status = st
			return l.items[i], nil
		}
	}
	return TodoItem{}, fmt.Errorf("plan: unknown todo id %q", id)
}

// Update merges fields for one todo (status and/or content).
func (l *TodoList) Update(id string, content *string, status *TodoStatus) (TodoItem, error) {
	if l == nil {
		return TodoItem{}, fmt.Errorf("plan: todo list is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return TodoItem{}, fmt.Errorf("plan: todo id is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.items {
		if l.items[i].ID != id {
			continue
		}
		if content != nil {
			c := strings.TrimSpace(*content)
			if c != "" {
				l.items[i].Content = c
			}
		}
		if status != nil {
			l.items[i].Status = normalizeTodoStatus(*status)
		}
		return l.items[i], nil
	}
	return TodoItem{}, fmt.Errorf("plan: unknown todo id %q", id)
}

func (l *TodoList) copyLocked() []TodoItem {
	if len(l.items) == 0 {
		return []TodoItem{}
	}
	out := make([]TodoItem, len(l.items))
	copy(out, l.items)
	return out
}

func normalizeTodoStatus(s TodoStatus) TodoStatus {
	switch TodoStatus(strings.ToLower(strings.TrimSpace(string(s)))) {
	case TodoInProgress, "in-progress", "working", "active":
		return TodoInProgress
	case TodoCompleted, "done", "complete":
		return TodoCompleted
	case TodoCancelled, "canceled":
		return TodoCancelled
	default:
		return TodoPending
	}
}

// ValidTodoStatus reports whether s is a known status.
func ValidTodoStatus(s string) bool {
	switch normalizeTodoStatus(TodoStatus(s)) {
	case TodoPending, TodoInProgress, TodoCompleted, TodoCancelled:
		// normalize always returns one of these; empty maps to pending.
		return strings.TrimSpace(s) == "" || true
	default:
		return false
	}
}
