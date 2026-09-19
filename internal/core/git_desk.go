package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/git"
)

// GitStatus is the desktop git panel snapshot (VAL-GIT-001).
type GitStatus = git.Status

// GitFileStatus is one changed path in the panel.
type GitFileStatus = git.FileStatus

// GitDiffResult is the panel diff view payload (VAL-GIT-002).
type GitDiffResult = git.DiffResult

// GitOpResult is stage/unstage/commit/fetch/pull outcome (VAL-GIT-003/004/010).
type GitOpResult = git.OpResult

// GitRepoRef is one discovered repo under the workspace.
type GitRepoRef = git.RepoRef

// GitGraphResult is the Graph tab payload.
type GitGraphResult = git.GraphResult

// GitStageRequest stages one or more paths in a selected repo.
type GitStageRequest struct {
	RepoID string   `json:"repo_id,omitempty"`
	Paths  []string `json:"paths"`
	Path   string   `json:"path,omitempty"` // convenience single path
}

// GitCommitRequest is a UI commit (VAL-GIT-004).
type GitCommitRequest struct {
	RepoID  string `json:"repo_id,omitempty"`
	Message string `json:"message"`
}

// GitDiffRequest selects a file for the diff view.
type GitDiffRequest struct {
	RepoID string `json:"repo_id,omitempty"`
	Path   string `json:"path"`
	Staged bool   `json:"staged"`
}

// GitGraphRequest is optional limit for graph log.
type GitGraphRequest struct {
	RepoID string `json:"repo_id,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// GitBranchList is the checkout picker payload.
type GitBranchList = git.BranchList

// GitBranchInfo is one branch row.
type GitBranchInfo = git.BranchInfo

// GitCheckoutRequest switches branch (or creates with create=true).
type GitCheckoutRequest struct {
	RepoID string `json:"repo_id,omitempty"`
	// Name is branch or remote branch (origin/dev).
	Name string `json:"name"`
	// Create runs checkout -b name.
	Create bool `json:"create,omitempty"`
}

// gitServiceFor builds a GitService for workspace + optional nested repo id.
func (s *Service) gitServiceFor(repoID string) (*git.Service, git.RepoRef, []git.RepoRef, error) {
	wsRoot, err := s.activeRoot()
	if err != nil {
		return nil, git.RepoRef{}, nil, err
	}
	repos, err := git.DiscoverRepos(wsRoot)
	if err != nil {
		return nil, git.RepoRef{}, nil, err
	}
	if len(repos) == 0 {
		// No git repo found — still return a service on workspace so Status reports not-a-repo.
		svc, err := git.New(git.Options{Root: wsRoot, AgentMode: false})
		if err != nil {
			return nil, git.RepoRef{}, nil, err
		}
		return svc, git.RepoRef{ID: ".", Name: filepath.Base(wsRoot), Root: wsRoot}, nil, nil
	}
	repos = git.EnrichRepos(repos)
	id := strings.TrimSpace(repoID)
	if id == "" {
		id = repos[0].ID
	}
	var selected git.RepoRef
	found := false
	for _, r := range repos {
		if r.ID == id {
			selected = r
			found = true
			break
		}
	}
	if !found {
		// Fallback resolve
		root, err := git.ResolveRepoRoot(wsRoot, id)
		if err != nil {
			// Default to first discovered
			selected = repos[0]
		} else {
			selected = git.RepoRef{
				ID:     id,
				Name:   filepath.Base(root),
				Root:   root,
				IsRepo: true,
			}
			if id == "." {
				selected.Name = filepath.Base(wsRoot)
			}
		}
	}
	svc, err := git.New(git.Options{Root: selected.Root, AgentMode: false})
	if err != nil {
		return nil, selected, repos, err
	}
	return svc, selected, repos, nil
}

// ListGitRepos discovers nested git repositories under the active workspace.
func (s *Service) ListGitRepos() ([]GitRepoRef, error) {
	wsRoot, err := s.activeRoot()
	if err != nil {
		return nil, err
	}
	refs, err := git.DiscoverRepos(wsRoot)
	if err != nil {
		return nil, err
	}
	return git.EnrichRepos(refs), nil
}

// GetGitStatus returns branch + porcelain-derived file list for repoID (VAL-GIT-001).
// Empty repoID selects the first discovered repo (or workspace root).
func (s *Service) GetGitStatus(repoID string) (GitStatus, error) {
	gs, selected, repos, err := s.gitServiceFor(repoID)
	if err != nil {
		return GitStatus{IsRepo: false, Message: err.Error(), Files: []GitFileStatus{}, Repos: repos}, err
	}
	st, err := gs.Status()
	if err != nil {
		st.Repos = repos
		st.RepoID = selected.ID
		st.RepoName = selected.Name
		return st, err
	}
	st.Repos = repos
	st.RepoID = selected.ID
	st.RepoName = selected.Name
	s.setStatus(st.Message)
	return st, nil
}

// GetGitDiff returns unified diff for a path in the selected repo (VAL-GIT-002).
func (s *Service) GetGitDiff(repoID, path string, staged bool) (GitDiffResult, error) {
	gs, _, _, err := s.gitServiceFor(repoID)
	if err != nil {
		return GitDiffResult{Path: path, Message: err.Error()}, err
	}
	return gs.Diff(path, staged)
}

// GetGitGraph returns commit graph for the Graph tab.
func (s *Service) GetGitGraph(repoID string, limit int) (GitGraphResult, error) {
	gs, selected, _, err := s.gitServiceFor(repoID)
	if err != nil {
		return GitGraphResult{RepoID: repoID, Message: err.Error()}, err
	}
	res, err := gs.Graph(limit)
	res.RepoID = selected.ID
	return res, err
}

// GitStage stages paths from the UI (VAL-GIT-003).
func (s *Service) GitStage(req GitStageRequest) (GitOpResult, error) {
	paths := collectPaths(req.Paths, req.Path)
	gs, selected, _, err := s.gitServiceFor(req.RepoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: req.RepoID}, err
	}
	res, err := gs.Stage(paths...)
	res.RepoID = selected.ID
	if res.OK {
		// re-attach multi-repo metadata
		if st, e := s.GetGitStatus(selected.ID); e == nil {
			res.Status = &st
		}
		s.setStatus(res.Message)
	}
	return res, err
}

// GitUnstage unstages paths from the UI (VAL-GIT-003).
func (s *Service) GitUnstage(req GitStageRequest) (GitOpResult, error) {
	paths := collectPaths(req.Paths, req.Path)
	gs, selected, _, err := s.gitServiceFor(req.RepoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: req.RepoID}, err
	}
	res, err := gs.Unstage(paths...)
	res.RepoID = selected.ID
	if res.OK {
		if st, e := s.GetGitStatus(selected.ID); e == nil {
			res.Status = &st
		}
		s.setStatus(res.Message)
	}
	return res, err
}

// GitCommit commits the index with the given message (VAL-GIT-004).
func (s *Service) GitCommit(req GitCommitRequest) (GitOpResult, error) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		err := fmt.Errorf("core: empty commit message")
		return GitOpResult{OK: false, Message: err.Error(), RepoID: req.RepoID}, err
	}
	gs, selected, _, err := s.gitServiceFor(req.RepoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: req.RepoID}, err
	}
	res, err := gs.Commit(msg)
	res.RepoID = selected.ID
	if res.OK {
		if st, e := s.GetGitStatus(selected.ID); e == nil {
			res.Status = &st
		}
		s.setStatus(res.Message)
	}
	return res, err
}

// GitFetch runs git fetch and returns clear success/failure (VAL-GIT-010).
func (s *Service) GitFetch(repoID string) (GitOpResult, error) {
	gs, selected, _, err := s.gitServiceFor(repoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: repoID}, err
	}
	res, err := gs.Fetch()
	res.RepoID = selected.ID
	if st, e := s.GetGitStatus(selected.ID); e == nil {
		res.Status = &st
	}
	s.setStatus(res.Message)
	return res, err
}

// GitPull runs git pull and returns clear success/failure (VAL-GIT-010).
func (s *Service) GitPull(repoID string) (GitOpResult, error) {
	gs, selected, _, err := s.gitServiceFor(repoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: repoID}, err
	}
	res, err := gs.Pull()
	res.RepoID = selected.ID
	if st, e := s.GetGitStatus(selected.ID); e == nil {
		res.Status = &st
	}
	s.setStatus(res.Message)
	return res, err
}

// GitPushRequest is a UI push (VAL-GIT-011). Force is gated separately (VAL-GIT-007).
type GitPushRequest struct {
	RepoID         string `json:"repo_id,omitempty"`
	Remote         string `json:"remote,omitempty"`
	Force          bool   `json:"force,omitempty"`
	ForceWithLease bool   `json:"force_with_lease,omitempty"`
	// Confirmed must be true when the user accepted the UI confirmation dialog.
	// Force paths still require Confirmed + force flag (never silent force).
	Confirmed bool `json:"confirmed"`
}

// GitDestructiveRequest gates reset --hard / branch delete from UI.
type GitDestructiveRequest struct {
	RepoID      string `json:"repo_id,omitempty"`
	// Kind is "reset_hard" or "branch_delete".
	Kind string `json:"kind"`
	// Ref is reset target or branch name.
	Ref string `json:"ref,omitempty"`
	// ForceDelete maps to branch -D.
	ForceDelete bool `json:"force_delete,omitempty"`
	// Confirmed must be true (UI confirmation dialog).
	Confirmed bool `json:"confirmed"`
}

// GitPush runs a non-force push by default; force requires Confirmed (VAL-GIT-007/011).
// UI must show confirmation before calling with Confirmed=true.
func (s *Service) GitPush(req GitPushRequest) (GitOpResult, error) {
	gs, selected, _, err := s.gitServiceFor(req.RepoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: req.RepoID}, err
	}
	force := req.Force || req.ForceWithLease
	// Always require explicit confirmation for UI push (policy default).
	if !req.Confirmed {
		msg := "push requires confirmation"
		if force {
			msg = "force-push requires explicit approval"
		}
		return GitOpResult{OK: false, Message: msg, RepoID: selected.ID}, fmt.Errorf("core: %s", msg)
	}
	opts := git.PushOptions{
		Remote:         req.Remote,
		Force:          req.Force,
		ForceWithLease: req.ForceWithLease,
		Approved:       req.Confirmed,
		RequireConfirm: true,
	}
	res, err := gs.Push(opts)
	res.RepoID = selected.ID
	if st, e := s.GetGitStatus(selected.ID); e == nil {
		res.Status = &st
	}
	s.setStatus(res.Message)
	return res, err
}

// GitDestructive runs reset --hard or branch delete only with Confirmed (VAL-GIT-006).
func (s *Service) GitDestructive(req GitDestructiveRequest) (GitOpResult, error) {
	gs, selected, _, err := s.gitServiceFor(req.RepoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: req.RepoID}, err
	}
	if !req.Confirmed {
		return GitOpResult{
			OK:      false,
			Message: "destructive git operation requires confirmation",
			RepoID:  selected.ID,
		}, fmt.Errorf("core: destructive confirmation required")
	}
	var res git.OpResult
	switch strings.TrimSpace(req.Kind) {
	case git.OpResetHard, "reset-hard":
		res, err = gs.ResetHard(git.DestructiveOptions{Approved: true, Ref: req.Ref})
	case git.OpBranchDelete, "branch-delete":
		res, err = gs.DeleteBranch(git.DestructiveOptions{
			Approved:    true,
			Ref:         req.Ref,
			ForceDelete: req.ForceDelete,
		})
	default:
		err = fmt.Errorf("core: unknown destructive kind %q", req.Kind)
		return GitOpResult{OK: false, Message: err.Error(), RepoID: selected.ID}, err
	}
	res.RepoID = selected.ID
	if st, e := s.GetGitStatus(selected.ID); e == nil {
		res.Status = &st
	}
	s.setStatus(res.Message)
	return res, err
}

// ListGitBranches returns local + remote branches for the checkout picker.
func (s *Service) ListGitBranches(repoID string) (GitBranchList, error) {
	gs, selected, _, err := s.gitServiceFor(repoID)
	if err != nil {
		return GitBranchList{Message: err.Error(), RepoID: repoID}, err
	}
	list, err := gs.Branches()
	list.RepoID = selected.ID
	return list, err
}

// GitCheckout switches or creates a branch (important SCM action).
func (s *Service) GitCheckout(req GitCheckoutRequest) (GitOpResult, error) {
	gs, selected, _, err := s.gitServiceFor(req.RepoID)
	if err != nil {
		return GitOpResult{OK: false, Message: err.Error(), RepoID: req.RepoID}, err
	}
	res, err := gs.Checkout(req.Name, req.Create)
	res.RepoID = selected.ID
	if res.OK {
		if st, e := s.GetGitStatus(selected.ID); e == nil {
			res.Status = &st
		}
		s.setStatus(res.Message)
	}
	return res, err
}

func collectPaths(paths []string, single string) []string {
	out := make([]string, 0, len(paths)+1)
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if single = strings.TrimSpace(single); single != "" {
		out = append(out, single)
	}
	return out
}
