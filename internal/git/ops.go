package git

import (
	"fmt"
	"strings"
)

// Push runs a non-force (or explicitly approved force) push.
// Default path never injects --force / --force-with-lease (VAL-GIT-007/008/011).
func (s *Service) Push(opts PushOptions) (OpResult, error) {
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}

	force := opts.Force || opts.ForceWithLease
	if force && !opts.Approved {
		err := fmt.Errorf("%w: force-push blocked without approval", ErrForcePushBlocked)
		return OpResult{
			OK:      false,
			Message: "force-push requires explicit approval",
			Detail:  err.Error(),
		}, err
	}
	if !force && opts.RequireConfirm && !opts.Approved {
		err := fmt.Errorf("%w: push requires confirmation", ErrApprovalRequired)
		return OpResult{
			OK:      false,
			Message: "push requires confirmation",
			Detail:  err.Error(),
		}, err
	}

	remote := strings.TrimSpace(opts.Remote)
	if remote == "" {
		remote = "origin"
	}
	// Reject remote names that look like option flags.
	if strings.HasPrefix(remote, "-") {
		err := fmt.Errorf("git: invalid remote %q", remote)
		return OpResult{OK: false, Message: err.Error()}, err
	}

	args := []string{"push"}
	// Force flags only when Approved (already checked).
	if opts.ForceWithLease {
		args = append(args, "--force-with-lease")
	} else if opts.Force {
		args = append(args, "--force")
	}
	args = append(args, remote)
	for _, r := range opts.Refspecs {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if strings.HasPrefix(r, "-") {
			err := fmt.Errorf("git: invalid refspec %q", r)
			return OpResult{OK: false, Message: err.Error()}, err
		}
		args = append(args, r)
	}

	out, err := s.runCapture(args...)
	if err != nil {
		return OpResult{
			OK:      false,
			Message: "push failed",
			Detail:  firstNonEmpty(strings.TrimSpace(out), err.Error()),
		}, err
	}
	st, _ := s.Status()
	msg := "push succeeded"
	if force {
		msg = "force-push succeeded"
	}
	return OpResult{
		OK:      true,
		Message: msg,
		Detail:  truncate(out, 4000),
		Status:  &st,
	}, nil
}

// ResetHard runs `git reset --hard [ref]` only with explicit approval (VAL-GIT-006/008).
func (s *Service) ResetHard(opts DestructiveOptions) (OpResult, error) {
	if !opts.Approved {
		err := fmt.Errorf("%w: reset --hard blocked without approval", ErrDestructiveBlocked)
		return OpResult{
			OK:      false,
			Message: "reset --hard requires explicit approval",
			Detail:  err.Error(),
		}, err
	}
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	ref := strings.TrimSpace(opts.Ref)
	if ref == "" {
		ref = "HEAD"
	}
	if strings.HasPrefix(ref, "-") || strings.Contains(ref, "..") {
		err := fmt.Errorf("git: invalid reset ref %q", ref)
		return OpResult{OK: false, Message: err.Error()}, err
	}
	out, err := s.runCapture("reset", "--hard", ref)
	if err != nil {
		return OpResult{
			OK:      false,
			Message: "reset --hard failed",
			Detail:  firstNonEmpty(strings.TrimSpace(out), err.Error()),
		}, err
	}
	st, _ := s.Status()
	return OpResult{
		OK:      true,
		Message: fmt.Sprintf("reset --hard %s", ref),
		Detail:  truncate(out, 4000),
		Status:  &st,
	}, nil
}

// DeleteBranch deletes a local branch; always requires approval (VAL-GIT-006).
func (s *Service) DeleteBranch(opts DestructiveOptions) (OpResult, error) {
	if !opts.Approved {
		err := fmt.Errorf("%w: branch delete blocked without approval", ErrDestructiveBlocked)
		return OpResult{
			OK:      false,
			Message: "branch delete requires explicit approval",
			Detail:  err.Error(),
		}, err
	}
	name := strings.TrimSpace(opts.Ref)
	if name == "" {
		err := fmt.Errorf("git: empty branch name")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if strings.HasPrefix(name, "-") || strings.Contains(name, "..") || strings.ContainsAny(name, " \t\n") {
		err := fmt.Errorf("git: invalid branch name %q", name)
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	flag := "-d"
	if opts.ForceDelete {
		flag = "-D"
	}
	out, err := s.runCapture("branch", flag, name)
	if err != nil {
		return OpResult{
			OK:      false,
			Message: "branch delete failed",
			Detail:  firstNonEmpty(strings.TrimSpace(out), err.Error()),
		}, err
	}
	st, _ := s.Status()
	return OpResult{
		OK:      true,
		Message: fmt.Sprintf("deleted branch %s", name),
		Detail:  truncate(out, 2000),
		Status:  &st,
	}, nil
}

// CommitAgent is agent-initiated commit; requires Approved=true (VAL-GIT-005).
// UI may call Commit() directly; agent tools must use this or pass approval first.
func (s *Service) CommitAgent(message string, approved bool) (OpResult, error) {
	if !approved {
		err := fmt.Errorf("%w: agent commit requires confirmation", ErrApprovalRequired)
		return OpResult{
			OK:      false,
			Message: "agent commit requires confirmation",
			Detail:  err.Error(),
		}, err
	}
	return s.Commit(message)
}

// IsAgentMode reports whether this service isolates hooksPath.
func (s *Service) IsAgentMode() bool { return s.agentMode }

// PrefixArgsForTest exposes prefixArgs for tests (hooksPath + safe flags).
func (s *Service) PrefixArgsForTest(args ...string) []string {
	return s.prefixArgs(args...)
}

// ValidateSandboxPath rejects path escape outside workspace (VAL-GIT-008).
// Paths must be relative and must not contain ".." segments.
func (s *Service) ValidateSandboxPath(path string) error {
	path = normalizeRel(path)
	if path == "" {
		return fmt.Errorf("git: empty path")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("git: path outside workspace root: %q", path)
	}
	// Absolute paths rejected even if under root (callers must pass rel).
	if strings.HasPrefix(path, "/") || (len(path) > 1 && path[1] == ':') {
		return fmt.Errorf("git: absolute path outside sandbox: %q", path)
	}
	return nil
}
