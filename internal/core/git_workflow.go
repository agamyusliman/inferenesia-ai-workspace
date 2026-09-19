package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/git"
	"gopkg.in/yaml.v3"
)

// GitWorkflow is the per-workspace git/deploy policy file payload.
type GitWorkflow = git.Workflow

// GitWorkflowRules is effective rules for one repo.
type GitWorkflowRules = git.WorkflowRules

// GetGitWorkflow loads .inferenesia/git-workflow.yaml (or defaults).
func (s *Service) GetGitWorkflow() (GitWorkflow, error) {
	root, err := s.activeRoot()
	if err != nil {
		return GitWorkflow{}, err
	}
	return git.LoadWorkflow(root)
}

// SaveGitWorkflow persists workflow YAML under the active workspace via
// WriteGateway so the file lands on the undo stack alongside agent writes
// (VAL-UNDO-007). The write is tagged with source "git-workflow".
func (s *Service) SaveGitWorkflow(w GitWorkflow) (GitWorkflow, error) {
	root, err := s.activeRoot()
	if err != nil {
		return GitWorkflow{}, err
	}
	if err := s.saveGitWorkflowViaGateway(root, w); err != nil {
		return GitWorkflow{}, err
	}
	out, err := git.LoadWorkflow(root)
	if err != nil {
		return GitWorkflow{}, err
	}
	s.setStatus("git workflow saved")
	return out, nil
}

// EnsureGitWorkflow creates a default workflow file if missing (Git panel "Init").
func (s *Service) EnsureGitWorkflow() (GitWorkflow, error) {
	root, err := s.activeRoot()
	if err != nil {
		return GitWorkflow{}, err
	}
	w, err := git.LoadWorkflow(root)
	if err != nil {
		return GitWorkflow{}, err
	}
	if w.Exists {
		return w, nil
	}
	if err := s.saveGitWorkflowViaGateway(root, w); err != nil {
		return GitWorkflow{}, err
	}
	return git.LoadWorkflow(root)
}


// saveGitWorkflowViaGateway serializes the workflow YAML the same way
// git.SaveWorkflow does, but routes the write through WriteGateway so the
// mutation lands on the undo stack (VAL-UNDO-007). The tiny YAML-marshal
// duplication is intentional to avoid importing writegate from internal/git.
func (s *Service) saveGitWorkflowViaGateway(root string, w git.Workflow) error {
	if w.Version == 0 {
		w.Version = 1
	}
	out := git.Workflow{Version: w.Version, Defaults: w.Defaults, Repos: w.Repos}
	b, err := yaml.Marshal(&out)
	if err != nil {
		return fmt.Errorf("git: marshal workflow: %w", err)
	}
	header := "# Inferenesia — per-workspace git / deploy workflow (no secrets).\n" +
		"# Agent reads this into chat context. Nested repos under repos:<id>.\n" +
		"# Edit from Git panel → Workflow tab, or by hand.\n\n"
	body := append([]byte(header), b...)

	// Prefer the gateway path; fall back to direct disk on gateway open error
	// so a broken workspace can still self-heal the file.
	gw, err := s.openGateway(root)
	if err != nil {
		return git.SaveWorkflow(root, w)
	}
	abs := git.WorkflowPath(root)
	rel, rerr := filepath.Rel(root, abs)
	if rerr != nil {
		return fmt.Errorf("git: workflow path rel: %w", rerr)
	}
	rel = filepath.ToSlash(rel)
	if gw.InTurn() {
		gw.EndTurn()
	}
	if _, err := gw.MakeDir(filepath.ToSlash(filepath.Dir(rel)), "git-workflow"); err != nil {
		return fmt.Errorf("git: mkdir workflow via gateway: %w", err)
	}
	if _, err := gw.WriteFileWithSource(rel, body, "git-workflow"); err != nil {
		return fmt.Errorf("git: write workflow via gateway: %w", err)
	}
	if err := s.saveGateway(root, gw); err != nil {
		return err
	}
	return nil
}

// gitWorkflowSystemSnippet builds agent prompt context for active (or first) repo.
func (s *Service) gitWorkflowSystemSnippet(repoID string) string {
	root, err := s.activeRoot()
	if err != nil {
		return ""
	}
	w, err := git.LoadWorkflow(root)
	if err != nil {
		return ""
	}
	// Prefer selected repo status for branch label
	st, err := s.GetGitStatus(repoID)
	branch := ""
	name := ""
	id := strings.TrimSpace(repoID)
	if err == nil {
		branch = st.Branch
		name = st.RepoName
		if id == "" {
			id = st.RepoID
		}
	}
	if id == "" {
		id = "."
	}
	md := w.AgentContextMarkdown(id, name, branch)
	// Cap size for system inject
	if len(md) > 6000 {
		md = md[:6000] + "\n…(truncated)"
	}
	return md
}

// GitWorkflowForAgent is exposed for CLI/tests: resolved markdown for a repo.
func (s *Service) GitWorkflowForAgent(repoID string) (string, error) {
	snip := s.gitWorkflowSystemSnippet(repoID)
	if snip == "" {
		return "", fmt.Errorf("core: no workspace or workflow")
	}
	return snip, nil
}
