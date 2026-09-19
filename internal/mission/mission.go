// Package mission implements Mission/Goal runtime: long-running goals with
// optional budget, verification gates, and evidence-gated completion.
// Distinct from Plan Mode (see docs/plan-mission.md). VAL-MISSION-*.
package mission

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
)

// Status is the mission lifecycle state.
type Status string

const (
	// StatusPending is the initial state after create (list-visible, not yet active).
	StatusPending Status = "pending"
	// StatusActive means work is in progress.
	StatusActive Status = "active"
	// StatusVerifying means gates are running.
	StatusVerifying Status = "verifying"
	// StatusBlocked means a blocker was hit (gate failure, missing key, etc.).
	StatusBlocked Status = "blocked"
	// StatusCompleted is terminal success — only via Complete with evidence.
	StatusCompleted Status = "completed"
	// StatusCancelled is user-cancelled (terminal; not success).
	StatusCancelled Status = "cancelled"
)

// EvidenceRequiredMsg is the stable denial when complete is attempted without evidence.
// Validators look for "evidence required" (VAL-MISSION-002).
const EvidenceRequiredMsg = "mission: evidence required — attach verification evidence before complete"

// GateFailedMsg is the denial when a verification gate failed.
const GateFailedMsg = "mission: verification gate failed — fix gates before complete"

// Evidence is a single piece of verification proof (command result, log path, note).
// Content is always secret-redacted before storage (VAL-MISSION-008).
type Evidence struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // command | note | gate | screenshot | test
	Summary   string    `json:"summary"`
	Content   string    `json:"content,omitempty"`
	Command   string    `json:"command,omitempty"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	GateName  string    `json:"gate_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// GateResult is one verification gate outcome (go test, go vet, typecheck/build).
type GateResult struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Passed   bool   `json:"passed"`
	ExitCode int    `json:"exit_code"`
	// Output is redacted command output (no secrets).
	Output    string    `json:"output,omitempty"`
	RanAt     time.Time `json:"ran_at,omitempty"`
	DurationMs int64    `json:"duration_ms,omitempty"`
}

// Mission is one durable goal with status, budget, gates, and evidence.
type Mission struct {
	ID                 string       `json:"id"`
	WorkspaceID        string       `json:"workspace_id,omitempty"`
	Goal               string       `json:"goal"`
	BudgetTokens       int64        `json:"budget_tokens,omitempty"`
	TokensUsed         int64        `json:"tokens_used,omitempty"`
	Status             Status       `json:"status"`
	BlockerReason      string       `json:"blocker_reason,omitempty"`
	AcceptanceCriteria []string     `json:"acceptance_criteria,omitempty"`
	Evidence           []Evidence   `json:"evidence"`
	Gates              []GateResult `json:"gates"`
	// PlanID references the approved Plan Mode proposal this mission was
	// created from (empty for missions created directly). VAL-CROSS-004.
	PlanID string `json:"plan_id,omitempty"`
	// PlanSnapshot is the approved plan body/summary captured at approve time
	// so the mission always carries the plan that fed it (docs/plan-mission.md
	// "plan_snapshot"). VAL-CROSS-004.
	PlanSnapshot string `json:"plan_snapshot,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	CompletedAt  *time.Time   `json:"completed_at,omitempty"`
}

// Snapshot returns a deep copy safe for JSON/API.
func (m *Mission) Snapshot() Mission {
	if m == nil {
		return Mission{}
	}
	out := *m
	if m.AcceptanceCriteria != nil {
		out.AcceptanceCriteria = append([]string(nil), m.AcceptanceCriteria...)
	}
	if m.Evidence != nil {
		out.Evidence = append([]Evidence(nil), m.Evidence...)
	} else {
		out.Evidence = []Evidence{}
	}
	if m.Gates != nil {
		out.Gates = append([]GateResult(nil), m.Gates...)
	} else {
		out.Gates = []GateResult{}
	}
	if m.CompletedAt != nil {
		t := *m.CompletedAt
		out.CompletedAt = &t
	}
	return out
}

// HasEvidence reports whether at least one non-empty evidence item is attached.
func (m *Mission) HasEvidence() bool {
	if m == nil {
		return false
	}
	for _, e := range m.Evidence {
		if strings.TrimSpace(e.Summary) != "" || strings.TrimSpace(e.Content) != "" || strings.TrimSpace(e.Command) != "" {
			return true
		}
	}
	return false
}

// GatesPassed reports whether every recorded gate passed (or no gates run yet = false for completion).
// Completion requires evidence; when gates were run, all must have Passed=true.
func (m *Mission) GatesPassed() bool {
	if m == nil || len(m.Gates) == 0 {
		// No gates recorded yet — completion still allowed if evidence is attached
		// (UI/manual evidence). Failing gate path is enforced separately when gates exist.
		return true
	}
	for _, g := range m.Gates {
		if !g.Passed {
			return false
		}
	}
	return true
}

// Store is a concurrency-safe in-memory mission registry for core.Service.
type Store struct {
	mu   sync.RWMutex
	byID map[string]*Mission
	seq  int
}

// NewStore returns an empty mission store.
func NewStore() *Store {
	return &Store{byID: make(map[string]*Mission)}
}

// Create registers a new mission with initial status pending (VAL-MISSION-001).
// goal is required. budgetTokens is optional (0 = unlimited).
func (s *Store) Create(goal string, budgetTokens int64, workspaceID string, criteria []string) (Mission, error) {
	return s.CreateFromPlan(goal, budgetTokens, workspaceID, criteria, "", "")
}

// CreateFromPlan registers a mission optionally fed from an approved Plan Mode
// proposal (VAL-CROSS-004). planID and planSnapshot are stored verbatim
// (snapshot redacted) so the mission always carries the plan that produced it.
// When planID is empty, the mission is a direct-created mission (no plan link).
func (s *Store) CreateFromPlan(goal string, budgetTokens int64, workspaceID string, criteria []string, planID, planSnapshot string) (Mission, error) {
	if s == nil {
		return Mission{}, fmt.Errorf("mission: store is nil")
	}
	goal = strings.TrimSpace(secrethyg.Redact(goal))
	if goal == "" {
		return Mission{}, fmt.Errorf("mission: goal is required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := fmt.Sprintf("msn-%d-%d", now.UnixNano(), s.seq)
	cleanCrit := make([]string, 0, len(criteria))
	for _, c := range criteria {
		c = strings.TrimSpace(secrethyg.Redact(c))
		if c != "" {
			cleanCrit = append(cleanCrit, c)
		}
	}
	m := &Mission{
		ID:                 id,
		WorkspaceID:        strings.TrimSpace(workspaceID),
		Goal:               goal,
		BudgetTokens:       budgetTokens,
		Status:             StatusPending,
		AcceptanceCriteria: cleanCrit,
		Evidence:           []Evidence{},
		Gates:              []GateResult{},
		PlanID:             strings.TrimSpace(planID),
		PlanSnapshot:       strings.TrimSpace(secrethyg.Redact(planSnapshot)),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.byID[id] = m
	return m.Snapshot(), nil
}

// Get returns a mission snapshot by id, or error if missing.
func (s *Store) Get(id string) (Mission, error) {
	if s == nil {
		return Mission{}, fmt.Errorf("mission: store is nil")
	}
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.byID[id]
	if !ok {
		return Mission{}, fmt.Errorf("mission: unknown id %q", id)
	}
	return m.Snapshot(), nil
}

// List returns all missions newest-first.
func (s *Store) List() []Mission {
	return s.ListByWorkspace("")
}

// ListByWorkspace returns missions for workspaceID newest-first.
// Empty workspaceID returns all missions (legacy/global view).
func (s *Store) ListByWorkspace(workspaceID string) []Mission {
	if s == nil {
		return nil
	}
	ws := strings.TrimSpace(workspaceID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Mission, 0, len(s.byID))
	for _, m := range s.byID {
		if ws != "" && strings.TrimSpace(m.WorkspaceID) != ws {
			continue
		}
		out = append(out, m.Snapshot())
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// Cancel marks a mission cancelled (terminal). Completed missions cannot be cancelled.
func (s *Store) Cancel(id string) (Mission, error) {
	return s.setStatus(id, StatusCancelled, "cancelled by user")
}

// Delete removes a mission from the store (pending/cancelled/blocked/completed).
func (s *Store) Delete(id string) error {
	if s == nil {
		return fmt.Errorf("mission: store is nil")
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[id]; !ok {
		return fmt.Errorf("mission: unknown id %q", id)
	}
	delete(s.byID, id)
	return nil
}

// Activate moves pending → active (or keeps active).
func (s *Store) Activate(id string) (Mission, error) {
	return s.setStatus(id, StatusActive, "")
}

// SetBlocked sets status to blocked with a reason (VAL-MISSION-004 / VAL-MISSION-007).
func (s *Store) SetBlocked(id, reason string) (Mission, error) {
	reason = strings.TrimSpace(secrethyg.Redact(reason))
	if reason == "" {
		reason = "blocked"
	}
	return s.setStatus(id, StatusBlocked, reason)
}

// setStatus updates status and optional blocker reason.
func (s *Store) setStatus(id string, st Status, blocker string) (Mission, error) {
	if s == nil {
		return Mission{}, fmt.Errorf("mission: store is nil")
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[id]
	if !ok {
		return Mission{}, fmt.Errorf("mission: unknown id %q", id)
	}
	if m.Status == StatusCompleted {
		return Mission{}, fmt.Errorf("mission: already completed")
	}
	if m.Status == StatusCancelled && st != StatusCancelled {
		return Mission{}, fmt.Errorf("mission: already cancelled")
	}
	m.Status = st
	m.UpdatedAt = time.Now().UTC()
	if st == StatusBlocked || st == StatusCancelled {
		m.BlockerReason = blocker
	} else if st == StatusActive || st == StatusVerifying || st == StatusPending {
		m.BlockerReason = ""
	}
	return m.Snapshot(), nil
}

// AttachEvidence adds verification evidence after redaction (VAL-MISSION-003/008).
func (s *Store) AttachEvidence(id string, kind, summary, content, command string, exitCode *int, gateName string) (Mission, error) {
	if s == nil {
		return Mission{}, fmt.Errorf("mission: store is nil")
	}
	id = strings.TrimSpace(id)
	summary = strings.TrimSpace(secrethyg.Redact(summary))
	content = strings.TrimSpace(secrethyg.Redact(content))
	command = strings.TrimSpace(secrethyg.Redact(command))
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "note"
	}
	if summary == "" && content == "" && command == "" {
		return Mission{}, fmt.Errorf("mission: evidence summary or content is required")
	}
	if summary == "" {
		if command != "" {
			summary = "command: " + command
		} else {
			summary = "evidence"
		}
	}
	// Cap content size to avoid huge logs in memory.
	const maxContent = 32 << 10 // 32 KiB
	if len(content) > maxContent {
		content = content[:maxContent] + "\n…[truncated]"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[id]
	if !ok {
		return Mission{}, fmt.Errorf("mission: unknown id %q", id)
	}
	if m.Status == StatusCompleted {
		return Mission{}, fmt.Errorf("mission: already completed")
	}
	s.seq++
	now := time.Now().UTC()
	ev := Evidence{
		ID:        fmt.Sprintf("ev-%d-%d", now.UnixNano(), s.seq),
		Kind:      kind,
		Summary:   summary,
		Content:   content,
		Command:   command,
		ExitCode:  exitCode,
		GateName:  strings.TrimSpace(gateName),
		CreatedAt: now,
	}
	m.Evidence = append(m.Evidence, ev)
	m.UpdatedAt = now
	// Pending missions with evidence often become active.
	if m.Status == StatusPending {
		m.Status = StatusActive
	}
	return m.Snapshot(), nil
}

// Complete marks the mission completed only when verification evidence is present
// and no failing gates block completion (VAL-MISSION-002/003/005/007).
func (s *Store) Complete(id string) (Mission, error) {
	if s == nil {
		return Mission{}, fmt.Errorf("mission: store is nil")
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[id]
	if !ok {
		return Mission{}, fmt.Errorf("mission: unknown id %q", id)
	}
	if m.Status == StatusCompleted {
		return m.Snapshot(), nil
	}
	if !m.HasEvidence() {
		return Mission{}, fmt.Errorf("%s", EvidenceRequiredMsg)
	}
	if !m.GatesPassed() {
		return Mission{}, fmt.Errorf("%s", GateFailedMsg)
	}
	now := time.Now().UTC()
	m.Status = StatusCompleted
	m.BlockerReason = ""
	m.UpdatedAt = now
	m.CompletedAt = &now
	return m.Snapshot(), nil
}

// RecordGate stores a gate result and may set blocked on failure (VAL-MISSION-005).
func (s *Store) RecordGate(id string, gate GateResult) (Mission, error) {
	if s == nil {
		return Mission{}, fmt.Errorf("mission: store is nil")
	}
	id = strings.TrimSpace(id)
	gate.Name = strings.TrimSpace(gate.Name)
	if gate.Name == "" {
		return Mission{}, fmt.Errorf("mission: gate name is required")
	}
	gate.Command = secrethyg.Redact(gate.Command)
	gate.Output = secrethyg.Redact(gate.Output)
	// Cap output.
	const maxOut = 32 << 10
	if len(gate.Output) > maxOut {
		gate.Output = gate.Output[:maxOut] + "\n…[truncated]"
	}
	if gate.RanAt.IsZero() {
		gate.RanAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[id]
	if !ok {
		return Mission{}, fmt.Errorf("mission: unknown id %q", id)
	}
	if m.Status == StatusCompleted {
		return Mission{}, fmt.Errorf("mission: already completed")
	}
	// Replace existing gate with same name, else append.
	replaced := false
	for i := range m.Gates {
		if m.Gates[i].Name == gate.Name {
			m.Gates[i] = gate
			replaced = true
			break
		}
	}
	if !replaced {
		m.Gates = append(m.Gates, gate)
	}
	m.UpdatedAt = time.Now().UTC()
	m.Status = StatusVerifying
	if !gate.Passed {
		m.Status = StatusBlocked
		m.BlockerReason = fmt.Sprintf("gate %q failed (exit %d)", gate.Name, gate.ExitCode)
	} else if m.GatesPassed() && m.Status == StatusVerifying {
		// All gates green → active again until complete.
		m.Status = StatusActive
		m.BlockerReason = ""
	}
	return m.Snapshot(), nil
}

// SetVerifying marks status verifying (gates about to run).
func (s *Store) SetVerifying(id string) (Mission, error) {
	return s.setStatus(id, StatusVerifying, "")
}
