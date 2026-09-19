package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Workflow is per-repo or workspace-default git / deploy policy for the agent.
// Stored under <workspace>/.inferenesia/git-workflow.yaml (preferred; legacy .yura-ai/ still read).
type Workflow struct {
	// Version schema (1).
	Version int `json:"version" yaml:"version"`
	// Defaults apply when a nested repo has no override.
	Defaults WorkflowRules `json:"defaults" yaml:"defaults"`
	// Repos keys are repo ids ("." or nested path like cms-a).
	Repos map[string]WorkflowRules `json:"repos,omitempty" yaml:"repos,omitempty"`
	// SourcePath absolute path of the file that was loaded (if any).
	SourcePath string `json:"source_path,omitempty" yaml:"-"`
	// Exists is true when a file was found on disk.
	Exists bool `json:"exists" yaml:"-"`
}

// WorkflowRules are agent-facing git / deploy rules for one repo.
type WorkflowRules struct {
	// Label optional human name.
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	// DefaultBranch preferred branch name (main/dev/release).
	DefaultBranch string `json:"default_branch,omitempty" yaml:"default_branch,omitempty"`
	// CommitStyle e.g. conventional, free.
	CommitStyle string `json:"commit_style,omitempty" yaml:"commit_style,omitempty"`
	// CommitTemplate optional message pattern for the agent.
	CommitTemplate string `json:"commit_template,omitempty" yaml:"commit_template,omitempty"`
	// BeforeCommit shell-ish checklist lines (agent must consider; not auto-run unless deploy tool).
	BeforeCommit []string `json:"before_commit,omitempty" yaml:"before_commit,omitempty"`
	// NeverCommit globs/paths the agent must not stage (.env, secrets).
	NeverCommit []string `json:"never_commit,omitempty" yaml:"never_commit,omitempty"`
	// Push policy hints.
	Push PushPolicy `json:"push,omitempty" yaml:"push,omitempty"`
	// Deploy optional post-merge / release workflow the agent may propose (not silent exec).
	Deploy DeployPolicy `json:"deploy,omitempty" yaml:"deploy,omitempty"`
	// Rules free-form bullet policy text for the agent system context.
	Rules string `json:"rules,omitempty" yaml:"rules,omitempty"`
	// AgentNotes extra Markdown for chat context.
	AgentNotes string `json:"agent_notes,omitempty" yaml:"agent_notes,omitempty"`
}

// PushPolicy describes remote write expectations.
type PushPolicy struct {
	// RequireConfirm always true for UI; agent must wait for user approval.
	RequireConfirm bool `json:"require_confirm" yaml:"require_confirm"`
	// AllowForce never default true.
	AllowForce bool `json:"allow_force" yaml:"allow_force"`
	// Remote name (origin).
	Remote string `json:"remote,omitempty" yaml:"remote,omitempty"`
	// Branch protected names the agent must not force-push.
	ProtectedBranches []string `json:"protected_branches,omitempty" yaml:"protected_branches,omitempty"`
}

// DeployPolicy is a suggested deploy path (agent proposes; user/CI executes).
type DeployPolicy struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	// When human-readable trigger e.g. "after merge to main".
	When string `json:"when,omitempty" yaml:"when,omitempty"`
	// Command is documentation / optional future runner (never silent force).
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
	// Steps ordered checklist for the agent.
	Steps []string `json:"steps,omitempty" yaml:"steps,omitempty"`
	// EnvHint non-secret deploy notes (e.g. "use staging profile").
	EnvHint string `json:"env_hint,omitempty" yaml:"env_hint,omitempty"`
}

// WorkflowPath returns <workspace>/.inferenesia/git-workflow.yaml (preferred).
// Legacy <workspace>/.yura-ai/git-workflow.yaml is still read by LoadWorkflow
// when the preferred path is absent (rebrand dual-read).
func WorkflowPath(workspaceRoot string) string {
	return filepath.Join(filepath.Clean(workspaceRoot), ".inferenesia", "git-workflow.yaml")
}

// legacyWorkflowPath returns the pre-rebrand workflow path (.yura-ai/).
func legacyWorkflowPath(workspaceRoot string) string {
	return filepath.Join(filepath.Clean(workspaceRoot), ".yura-ai", "git-workflow.yaml")
}

// DefaultWorkflow is a safe starter (never-commit secrets, no force-push).
func DefaultWorkflow() Workflow {
	return Workflow{
		Version: 1,
		Defaults: WorkflowRules{
			DefaultBranch:  "main",
			CommitStyle:    "conventional",
			CommitTemplate: "type(scope): summary",
			BeforeCommit: []string{
				"Review staged diff",
				"Do not stage secrets or .env files",
			},
			NeverCommit: []string{".env", ".env.*", "*.pem", "*credentials*", "*secret*"},
			Push: PushPolicy{
				RequireConfirm:    true,
				AllowForce:        false,
				Remote:            "origin",
				ProtectedBranches: []string{"main", "master", "release"},
			},
			Deploy: DeployPolicy{
				Enabled: false,
			},
			Rules: `Git safety (Inferenesia):
- Prefer small, focused commits with clear messages.
- Never commit credentials, API keys, tokens, or .env files.
- Never force-push main/release without explicit user confirmation.
- Destructive ops (reset --hard, branch -D) require user confirmation.
- Follow workspace Push Policy; local commit is fine; push only when policy allows.`,
		},
		Repos: map[string]WorkflowRules{},
	}
}

// LoadWorkflow reads .inferenesia/git-workflow.yaml (preferred) or legacy
// .yura-ai/git-workflow.yaml, or returns defaults (Exists=false).
func LoadWorkflow(workspaceRoot string) (Workflow, error) {
	path := WorkflowPath(workspaceRoot)
	b, err := os.ReadFile(path)
	if err != nil {
		// Legacy fallback: read .yura-ai/git-workflow.yaml when preferred absent.
		if os.IsNotExist(err) {
			legacyPath := legacyWorkflowPath(workspaceRoot)
			if lb, lerr := os.ReadFile(legacyPath); lerr == nil {
				b = lb
				path = legacyPath
				err = nil
			}
		}
	}
	if err != nil {
		if os.IsNotExist(err) {
			w := DefaultWorkflow()
			w.SourcePath = path
			w.Exists = false
			return w, nil
		}
		return Workflow{}, fmt.Errorf("git: read workflow: %w", err)
	}
	var w Workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		return Workflow{}, fmt.Errorf("git: parse workflow: %w", err)
	}
	if w.Version == 0 {
		w.Version = 1
	}
	if w.Repos == nil {
		w.Repos = map[string]WorkflowRules{}
	}
	// Merge empty defaults from DefaultWorkflow for missing NeverCommit etc.
	def := DefaultWorkflow().Defaults
	w.Defaults = mergeRules(def, w.Defaults)
	w.SourcePath = path
	w.Exists = true
	return w, nil
}

// SaveWorkflow writes workflow to .inferenesia/git-workflow.yaml (mode 0644; no secrets).
func SaveWorkflow(workspaceRoot string, w Workflow) error {
	path := WorkflowPath(workspaceRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("git: mkdir workflow: %w", err)
	}
	if w.Version == 0 {
		w.Version = 1
	}
	// Strip runtime fields
	out := Workflow{
		Version:  w.Version,
		Defaults: w.Defaults,
		Repos:    w.Repos,
	}
	b, err := yaml.Marshal(&out)
	if err != nil {
		return fmt.Errorf("git: marshal workflow: %w", err)
	}
	header := "# Inferenesia — per-workspace git / deploy workflow (no secrets).\n" +
		"# Agent reads this into chat context. Nested repos under repos:<id>.\n" +
		"# Edit from Git panel → Workflow tab, or by hand.\n\n"
	if err := os.WriteFile(path, append([]byte(header), b...), 0o644); err != nil {
		return fmt.Errorf("git: write workflow: %w", err)
	}
	return nil
}

// ResolveRules returns effective rules for repoID (repo override over defaults).
func (w Workflow) ResolveRules(repoID string) WorkflowRules {
	id := strings.TrimSpace(repoID)
	if id == "" {
		id = "."
	}
	base := w.Defaults
	if w.Repos != nil {
		if r, ok := w.Repos[id]; ok {
			return mergeRules(base, r)
		}
		// Also try without "./"
		if r, ok := w.Repos[strings.TrimPrefix(id, "./")]; ok {
			return mergeRules(base, r)
		}
	}
	return base
}

// AgentContextMarkdown builds a prompt fragment for the agent (no secrets).
func (w Workflow) AgentContextMarkdown(repoID, repoName, branch string) string {
	r := w.ResolveRules(repoID)
	var b strings.Builder
	b.WriteString("## Git workflow (workspace policy)\n")
	if w.Exists {
		b.WriteString(fmt.Sprintf("Source: `.inferenesia/git-workflow.yaml`\n"))
	} else {
		b.WriteString("Source: built-in defaults (no project file yet)\n")
	}
	if repoName != "" || repoID != "" {
		b.WriteString(fmt.Sprintf("Active repo: %s (id=%s)\n", firstNonEmpty(repoName, repoID), firstNonEmpty(repoID, ".")))
	}
	if branch != "" {
		b.WriteString(fmt.Sprintf("Branch: %s\n", branch))
	}
	if r.DefaultBranch != "" {
		b.WriteString(fmt.Sprintf("Default branch: %s\n", r.DefaultBranch))
	}
	if r.CommitStyle != "" {
		b.WriteString(fmt.Sprintf("Commit style: %s\n", r.CommitStyle))
	}
	if r.CommitTemplate != "" {
		b.WriteString(fmt.Sprintf("Commit template: %s\n", r.CommitTemplate))
	}
	if len(r.NeverCommit) > 0 {
		b.WriteString("Never commit: " + strings.Join(r.NeverCommit, ", ") + "\n")
	}
	if len(r.BeforeCommit) > 0 {
		b.WriteString("Before commit:\n")
		for _, step := range r.BeforeCommit {
			b.WriteString("- " + step + "\n")
		}
	}
	b.WriteString(fmt.Sprintf("Push: require_confirm=%v allow_force=%v remote=%s\n",
		r.Push.RequireConfirm || true, r.Push.AllowForce, firstNonEmpty(r.Push.Remote, "origin")))
	if len(r.Push.ProtectedBranches) > 0 {
		b.WriteString("Protected branches: " + strings.Join(r.Push.ProtectedBranches, ", ") + "\n")
	}
	if r.Deploy.Enabled {
		b.WriteString("Deploy enabled:\n")
		if r.Deploy.When != "" {
			b.WriteString("- when: " + r.Deploy.When + "\n")
		}
		if r.Deploy.Command != "" {
			b.WriteString("- command: " + r.Deploy.Command + "\n")
		}
		if r.Deploy.EnvHint != "" {
			b.WriteString("- env: " + r.Deploy.EnvHint + "\n")
		}
		for _, step := range r.Deploy.Steps {
			b.WriteString("- step: " + step + "\n")
		}
		b.WriteString("- Do not run production deploy without explicit user approval.\n")
	}
	if strings.TrimSpace(r.Rules) != "" {
		b.WriteString("\n### Rules\n")
		b.WriteString(strings.TrimSpace(r.Rules))
		b.WriteString("\n")
	}
	if strings.TrimSpace(r.AgentNotes) != "" {
		b.WriteString("\n### Notes\n")
		b.WriteString(strings.TrimSpace(r.AgentNotes))
		b.WriteString("\n")
	}
	return b.String()
}

func mergeRules(base, over WorkflowRules) WorkflowRules {
	out := base
	if over.Label != "" {
		out.Label = over.Label
	}
	if over.DefaultBranch != "" {
		out.DefaultBranch = over.DefaultBranch
	}
	if over.CommitStyle != "" {
		out.CommitStyle = over.CommitStyle
	}
	if over.CommitTemplate != "" {
		out.CommitTemplate = over.CommitTemplate
	}
	if len(over.BeforeCommit) > 0 {
		out.BeforeCommit = over.BeforeCommit
	}
	if len(over.NeverCommit) > 0 {
		out.NeverCommit = over.NeverCommit
	}
	// Push: merge fields
	if over.Push.Remote != "" {
		out.Push.Remote = over.Push.Remote
	}
	if over.Push.RequireConfirm {
		out.Push.RequireConfirm = true
	}
	// AllowForce only if explicitly set true in override (yaml zero is false — keep base unless override sets true)
	if over.Push.AllowForce {
		out.Push.AllowForce = true
	}
	// If override has any protected list, use it
	if len(over.Push.ProtectedBranches) > 0 {
		out.Push.ProtectedBranches = over.Push.ProtectedBranches
	}
	// Deploy: if override has Enabled true or any field
	if over.Deploy.Enabled || over.Deploy.Command != "" || over.Deploy.When != "" || len(over.Deploy.Steps) > 0 {
		out.Deploy = over.Deploy
	}
	if over.Rules != "" {
		out.Rules = over.Rules
	}
	if over.AgentNotes != "" {
		out.AgentNotes = over.AgentNotes
	}
	return out
}
