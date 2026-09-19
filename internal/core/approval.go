package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/git"
)

// ChatEventNeedsApproval is emitted when agent git ops need user confirmation (VAL-GIT-005+).
const ChatEventNeedsApproval ChatEventType = "NeedsApproval"

// ApprovalDecision is the user's response to a NeedsApproval dialog.
type ApprovalDecision struct {
	// ID must match the pending approval request.
	ID string `json:"id"`
	// Approved true = proceed; false = reject/cancel.
	Approved bool `json:"approved"`
}

// PendingApproval is one open NeedsApproval request for the UI.
type PendingApproval struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Detail    string    `json:"detail,omitempty"`
	RepoID    string    `json:"repo_id,omitempty"`
	Force     bool      `json:"force,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// gitApproverBridge implements tools.Approver using in-process wait + UI ResolveApproval.
type gitApproverBridge struct {
	mu      sync.Mutex
	pending map[string]chan bool
	// last holds the latest request for GetPendingApprovals (single-flight UI).
	last *PendingApproval
	// emit hooks into the active chat stream when set.
	emitMu sync.Mutex
	emit   func(ChatEvent)
	// timeout for waiting on user response (default 10m).
	timeout time.Duration
	// seq for ids
	seq int
}

func newGitApproverBridge() *gitApproverBridge {
	return &gitApproverBridge{
		pending: map[string]chan bool{},
		timeout: 10 * time.Minute,
	}
}

func (b *gitApproverBridge) setEmit(emit func(ChatEvent)) {
	b.emitMu.Lock()
	defer b.emitMu.Unlock()
	b.emit = emit
}

func (b *gitApproverBridge) RequestApproval(ctx context.Context, req git.ApprovalRequest) (bool, error) {
	if b == nil {
		return false, nil
	}
	b.mu.Lock()
	b.seq++
	id := req.ID
	if id == "" {
		id = fmt.Sprintf("appr-%d-%d", time.Now().UnixNano(), b.seq)
	}
	ch := make(chan bool, 1)
	b.pending[id] = ch
	pa := &PendingApproval{
		ID:        id,
		Kind:      req.Kind,
		Summary:   req.Summary,
		Detail:    req.Detail,
		RepoID:    req.RepoID,
		Force:     req.Force,
		CreatedAt: time.Now().UTC(),
	}
	b.last = pa
	timeout := b.timeout
	b.mu.Unlock()

	// Surface to UI / SSE
	b.emitMu.Lock()
	emit := b.emit
	b.emitMu.Unlock()
	if emit != nil {
		emit(ChatEvent{
			Type:          ChatEventNeedsApproval,
			Name:          req.Kind,
			Detail:        req.Summary,
			Text:          req.Detail,
			ApprovalID:    id,
			ApprovalKind:  req.Kind,
			ApprovalForce: req.Force,
		})
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		b.clear(id)
		return false, ctx.Err()
	case <-timer.C:
		b.clear(id)
		return false, fmt.Errorf("core: approval timed out for %s", id)
	case ok := <-ch:
		b.clear(id)
		return ok, nil
	}
}

func (b *gitApproverBridge) clear(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pending, id)
	if b.last != nil && b.last.ID == id {
		b.last = nil
	}
}

// Resolve resolves a pending approval (UI approve/reject).
func (b *gitApproverBridge) Resolve(id string, approved bool) error {
	if b == nil {
		return fmt.Errorf("core: no approval bridge")
	}
	b.mu.Lock()
	ch, ok := b.pending[id]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("core: unknown or expired approval id %q", id)
	}
	select {
	case ch <- approved:
		return nil
	default:
		// already resolved
		return nil
	}
}

// Pending returns the latest open approval request if any.
func (b *gitApproverBridge) Pending() *PendingApproval {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.last == nil {
		return nil
	}
	cp := *b.last
	return &cp
}

// --- Service methods ---

// ensureApprover returns the service-level git approval bridge (lazy).
func (s *Service) ensureApprover() *gitApproverBridge {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gitApprover == nil {
		s.gitApprover = newGitApproverBridge()
	}
	return s.gitApprover
}

// ResolveApproval records the user's decision for a NeedsApproval request (VAL-GIT-005+).
func (s *Service) ResolveApproval(dec ApprovalDecision) error {
	id := dec.ID
	if id == "" {
		return fmt.Errorf("core: empty approval id")
	}
	return s.ensureApprover().Resolve(id, dec.Approved)
}

// GetPendingApproval returns the latest open approval request (or nil).
func (s *Service) GetPendingApproval() *PendingApproval {
	return s.ensureApprover().Pending()
}
