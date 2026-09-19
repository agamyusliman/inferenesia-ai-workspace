package git

import "errors"

// Gated operation kinds for agent tools and approval UI (VAL-GIT-005/006/007).
const (
	// OpCommit is an agent-initiated commit (UI commit may skip agent gate).
	OpCommit = "commit"
	// OpPush is a non-force push (may still require confirm by policy).
	OpPush = "push"
	// OpForcePush is git push --force or --force-with-lease.
	OpForcePush = "force_push"
	// OpResetHard is git reset --hard.
	OpResetHard = "reset_hard"
	// OpBranchDelete is branch delete (git branch -d/-D).
	OpBranchDelete = "branch_delete"
)

// ErrApprovalRequired is returned when a gated git op runs without explicit approval.
var ErrApprovalRequired = errors.New("git: approval required")

// ErrForcePushBlocked is returned when force-push is attempted without approval.
var ErrForcePushBlocked = errors.New("git: force-push requires explicit approval")

// ErrDestructiveBlocked is returned when reset --hard / branch -D run without approval.
var ErrDestructiveBlocked = errors.New("git: destructive operation requires explicit approval")

// ApprovalRequest describes a NeedsApproval surface payload.
type ApprovalRequest struct {
	// ID is set by the core approver bridge (optional at GitService layer).
	ID string `json:"id,omitempty"`
	// Kind is one of Op* constants.
	Kind string `json:"kind"`
	// Summary is a short human-readable line for the dialog title/body.
	Summary string `json:"summary"`
	// Detail is optional expanded text (branch, message, remote).
	Detail string `json:"detail,omitempty"`
	// RepoID multi source-control id ("." or nested).
	RepoID string `json:"repo_id,omitempty"`
	// Force is true when this is a force or force-with-lease push.
	Force bool `json:"force,omitempty"`
}

// PushOptions configures Push with safe defaults (non-force).
type PushOptions struct {
	// Remote defaults to "origin".
	Remote string
	// Refspecs optional (e.g. HEAD:main); empty = default upstream push.
	Refspecs []string
	// Force maps to --force (requires Approved=true).
	Force bool
	// ForceWithLease maps to --force-with-lease (requires Approved=true).
	ForceWithLease bool
	// Approved must be true for force / force-with-lease. For normal push,
	// callers may still set ConfirmRequired and Approved for policy gates.
	Approved bool
	// RequireConfirm when true, normal (non-force) push also needs Approved=true.
	// Default UI path sets this true (VAL-GIT-011).
	RequireConfirm bool
}

// DestructiveOptions gates reset --hard and branch delete.
type DestructiveOptions struct {
	// Approved must be true or the op returns ErrDestructiveBlocked.
	Approved bool
	// ForceDelete maps branch -D vs -d.
	ForceDelete bool
	// Ref is the reset target (default HEAD) or branch name to delete.
	Ref string
}
