package plan

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ProposalStatus is the lifecycle of a proposed plan awaiting user decision.
type ProposalStatus string

const (
	// ProposalNone means no pending proposal.
	ProposalNone ProposalStatus = "none"
	// ProposalPending waits for Approve/Reject (PlanProposed event already emitted).
	ProposalPending ProposalStatus = "pending"
	// ProposalApproved was approved (todos may have been seeded).
	ProposalApproved ProposalStatus = "approved"
	// ProposalRejected was discarded without seeding todos (VAL-PLAN-004).
	ProposalRejected ProposalStatus = "rejected"
)

// ProposedPlan is a plan draft waiting for user Approve/Reject (VAL-PLAN-003/004).
// Only one proposed plan is active at a time (docs/plan-mission.md).
type ProposedPlan struct {
	ID        string         `json:"id"`
	Title     string         `json:"title,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Body      string         `json:"body,omitempty"`
	Steps     []string       `json:"steps"`
	Status    ProposalStatus `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
}

// ProposalStore holds the single pending proposed plan (thread-safe).
type ProposalStore struct {
	mu   sync.RWMutex
	plan *ProposedPlan
	seq  int
}

// NewProposalStore returns an empty proposal store.
func NewProposalStore() *ProposalStore {
	return &ProposalStore{}
}

// Set stores a new pending proposal (replaces any previous pending one).
// Steps should be non-empty checklist items for todo seeding on approve.
func (s *ProposalStore) Set(title, summary, body string, steps []string) ProposedPlan {
	if s == nil {
		return ProposedPlan{Status: ProposalNone}
	}
	clean := make([]string, 0, len(steps))
	for _, step := range steps {
		step = strings.TrimSpace(step)
		if step != "" {
			clean = append(clean, step)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	p := &ProposedPlan{
		ID:        fmt.Sprintf("plan-%d-%d", time.Now().UnixNano(), s.seq),
		Title:     strings.TrimSpace(title),
		Summary:   strings.TrimSpace(summary),
		Body:      strings.TrimSpace(body),
		Steps:     clean,
		Status:    ProposalPending,
		CreatedAt: time.Now().UTC(),
	}
	s.plan = p
	return *p
}

// Get returns a copy of the current pending proposal, or nil if none/already decided.
func (s *ProposalStore) Get() *ProposedPlan {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.plan == nil || s.plan.Status != ProposalPending {
		return nil
	}
	cp := *s.plan
	if len(s.plan.Steps) > 0 {
		cp.Steps = append([]string(nil), s.plan.Steps...)
	}
	return &cp
}

// PeekAny returns the last proposal regardless of status (for diagnostics/UI history).
func (s *ProposalStore) PeekAny() *ProposedPlan {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.plan == nil {
		return nil
	}
	cp := *s.plan
	if len(s.plan.Steps) > 0 {
		cp.Steps = append([]string(nil), s.plan.Steps...)
	}
	return &cp
}

// Approve marks the pending proposal approved and returns a copy for todo seeding.
// Returns error if no pending proposal exists.
func (s *ProposalStore) Approve() (ProposedPlan, error) {
	if s == nil {
		return ProposedPlan{}, fmt.Errorf("plan: no proposal store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plan == nil || s.plan.Status != ProposalPending {
		return ProposedPlan{}, fmt.Errorf("plan: no pending plan to approve")
	}
	s.plan.Status = ProposalApproved
	cp := *s.plan
	if len(s.plan.Steps) > 0 {
		cp.Steps = append([]string(nil), s.plan.Steps...)
	}
	// Clear pending slot so Get() returns nil after approve.
	// Caller seeds todos from cp.Steps.
	return cp, nil
}

// Reject discards the pending proposal without returning steps for seeding (VAL-PLAN-004).
func (s *ProposalStore) Reject() (ProposedPlan, error) {
	if s == nil {
		return ProposedPlan{}, fmt.Errorf("plan: no proposal store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plan == nil || s.plan.Status != ProposalPending {
		return ProposedPlan{}, fmt.Errorf("plan: no pending plan to reject")
	}
	s.plan.Status = ProposalRejected
	cp := *s.plan
	// Explicitly empty steps on reject path so callers cannot accidentally seed.
	cp.Steps = nil
	return cp, nil
}

// Clear removes any stored proposal.
func (s *ProposalStore) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plan = nil
}
