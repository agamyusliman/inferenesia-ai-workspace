// Package plan implements Plan Mode: a read-only agent state that denies
// mutating tools (fs write, shell, git write) while allowing explore/read tools.
// Plan Mode is distinct from Mission/Goal (see docs/plan-mission.md).
package plan

import (
	"fmt"
	"strings"
	"sync"
)

// Mode is the Plan Mode state machine: inactive | active.
// Approve/reject of a proposed plan and cancel exit back to inactive
// (VAL-PLAN-001/006/007). Todo lifecycle is a separate concern.
type Mode string

const (
	// ModeInactive is the default: write tools allowed through normal gates.
	ModeInactive Mode = "inactive"
	// ModeActive is read-only Plan Mode: mutating tools are denied.
	ModeActive Mode = "active"
)

// DenialPrefix is the stable error prefix validators look for.
// Evidence strings should include this phrase (VAL-PLAN-001 / VAL-CORE-004).
const DenialPrefix = "plan mode: writes denied"

// DenialMessage returns the full tool error for a blocked mutating call.
func DenialMessage(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "tool"
	}
	return fmt.Sprintf("%s — tool %q is not allowed while Plan Mode is active (read-only)", DenialPrefix, name)
}

// ToolKind classifies agent tools for Plan Mode enforcement.
type ToolKind int

const (
	// KindRead is allowed in Plan Mode (explore, status, diff, list, …).
	KindRead ToolKind = iota
	// KindWrite is denied in Plan Mode (file mutate, commit, shell, …).
	KindWrite
	// KindUnknown is treated as write (fail-closed) when Plan Mode is active.
	KindUnknown
)

// State is a concurrency-safe Plan Mode controller.
// Wire one instance per core.Service (desktop + CLI share it).
type State struct {
	mu   sync.RWMutex
	mode Mode
	// reason is an optional human label (e.g. "user", "approve", "cancel").
	reason string
}

// New returns an inactive Plan Mode state.
func New() *State {
	return &State{mode: ModeInactive}
}

// Mode returns the current mode (inactive or active).
func (s *State) Mode() Mode {
	if s == nil {
		return ModeInactive
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// Active reports whether Plan Mode is currently active.
func (s *State) Active() bool {
	return s.Mode() == ModeActive
}

// Reason returns the last enter/exit reason string (may be empty).
func (s *State) Reason() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reason
}

// Enter activates Plan Mode (read-only). Idempotent when already active.
// reason is stored for UI/diagnostics (e.g. "user").
func (s *State) Enter(reason string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = ModeActive
	if strings.TrimSpace(reason) != "" {
		s.reason = strings.TrimSpace(reason)
	} else {
		s.reason = "enter"
	}
}

// Exit deactivates Plan Mode and re-enables write tools (VAL-PLAN-007).
// reason examples: "approve", "reject", "cancel", "user".
func (s *State) Exit(reason string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = ModeInactive
	if strings.TrimSpace(reason) != "" {
		s.reason = strings.TrimSpace(reason)
	} else {
		s.reason = "exit"
	}
}

// Snapshot is a JSON-friendly plan mode view for hub/UI (VAL-PLAN-001 indicator).
type Snapshot struct {
	Active bool   `json:"active"`
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
}

// Snapshot returns the current state for UI/API.
func (s *State) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{Active: false, Mode: string(ModeInactive)}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		Active: s.mode == ModeActive,
		Mode:   string(s.mode),
		Reason: s.reason,
	}
}

// ClassifyTool returns KindRead or KindWrite for a built-in tool name.
// Unknown names are KindUnknown (denied in Plan Mode).
func ClassifyTool(name string) ToolKind {
	n := strings.TrimSpace(name)
	if n == "" {
		return KindUnknown
	}
	switch n {
	// --- Explicit read / explore tools ---
	case "read_file", "grep", "list_dir",
		"git_status", "git_diff", "git_log", "git_branch_list",
		"terminal_read", "terminal_list",
		// propose_plan + todo_list are planning/read surfaces (allowed in Plan Mode).
		"propose_plan", "todo_list",
		// Mission read surface.
		"get_goal",
		// Skill discovery / status + multi-brain read (VAL-ORCH-004/005).
		"skill", "skill_status", "multibrain_read",
		// Browser read-ish surfaces (status/snapshot/wait) — no host shell (VAL-BRW-009).
		"browser_status", "browser_snapshot", "browser_wait_human":
		return KindRead
	// --- Explicit mutate tools ---
	case "write_file", "write_multibrain", "write_diagram",
		"create_diagram", "update_diagram", "export_diagram", "convert_diagram",
		"shell",
		"git_stage", "git_unstage", "git_commit",
		"git_fetch", "git_pull", "git_push",
		"git_checkout", "git_reset_hard", "git_branch_delete",
		"terminal_write", "terminal_start", "terminal_stop",
		// Todo mutations are write-like; after Approve, Plan Mode exits first.
		"todo_write", "todo_update",
		// Mission mutations (complete/block/create/evidence).
		"create_goal", "update_goal", "attach_mission_evidence",
		// task() dispatches sub-agents; denied in Plan Mode (fail-closed).
		"task",
		// Multi Brain write via skill protocol (WriteGateway).
		"multibrain_append",
		// Browser session mutations (launch/navigate/eval/close).
		"browser_set_engine", "browser_navigate", "browser_screenshot",
		"browser_evaluate", "browser_close":
		return KindWrite
	default:
		// Fail-closed for new tools: plan mode must not silently allow writes.
		// Convention: names containing write/commit/push/delete/shell are write-like;
		// names containing read/list/status/diff/log/grep are read-like.
		lower := strings.ToLower(n)
		if strings.Contains(lower, "read") ||
			strings.Contains(lower, "list") ||
			strings.Contains(lower, "status") ||
			strings.Contains(lower, "diff") ||
			strings.Contains(lower, "log") ||
			strings.Contains(lower, "grep") ||
			strings.Contains(lower, "search") ||
			strings.HasPrefix(lower, "get_") {
			return KindRead
		}
		if strings.Contains(lower, "write") ||
			strings.Contains(lower, "commit") ||
			strings.Contains(lower, "push") ||
			strings.Contains(lower, "delete") ||
			strings.Contains(lower, "shell") ||
			strings.Contains(lower, "create") ||
			strings.Contains(lower, "stage") ||
			strings.Contains(lower, "mutate") {
			return KindWrite
		}
		return KindUnknown
	}
}

// IsWriteTool reports whether the tool mutates state (denied in Plan Mode).
func IsWriteTool(name string) bool {
	k := ClassifyTool(name)
	return k == KindWrite || k == KindUnknown
}

// IsReadTool reports whether the tool is allowed while Plan Mode is active.
func IsReadTool(name string) bool {
	return ClassifyTool(name) == KindRead
}

// AllowTool returns nil if the tool may run under the current mode.
// When Plan Mode is active and the tool is mutating/unknown, returns an error
// whose text includes DenialPrefix (VAL-PLAN-001/006).
func (s *State) AllowTool(name string) error {
	if s == nil || !s.Active() {
		return nil
	}
	if IsWriteTool(name) {
		return fmt.Errorf("%s", DenialMessage(name))
	}
	return nil
}

// FilterDefinitions keeps only read tools when active; otherwise returns all.
// Used to present a read-only tool set to the model in Plan Mode (VAL-PLAN-006).
func FilterDefinitions[T any](active bool, defs []T, nameOf func(T) string) []T {
	if !active {
		return defs
	}
	out := make([]T, 0, len(defs))
	for _, d := range defs {
		if IsReadTool(nameOf(d)) {
			out = append(out, d)
		}
	}
	return out
}
