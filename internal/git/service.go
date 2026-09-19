// Package git implements GitService with safe flags for UI and agent helpers.
// Destructive ops and force-push are gated by callers (approval layer).
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FileStatus is one line from git status --porcelain.
type FileStatus struct {
	// Path is the workspace-relative path (forward slashes).
	Path string `json:"path"`
	// Index is the index/staged status letter (space = clean in index).
	Index string `json:"index"`
	// WorkTree is the worktree status letter (space = clean in worktree).
	WorkTree string `json:"worktree"`
	// Staged is true when the path has staged (index) changes.
	Staged bool `json:"staged"`
	// Unstaged is true when the path has unstaged worktree changes or is untracked.
	Unstaged bool `json:"unstaged"`
	// Untracked is true for "??" untracked paths.
	Untracked bool `json:"untracked"`
	// StatusLabel is a short display code: M, A, D, R, C, U, ?.
	StatusLabel string `json:"status_label"`
	// Original is set for renames (old path).
	Original string `json:"original,omitempty"`
}

// Status is a parsed git status --porcelain snapshot for the git panel.
type Status struct {
	// Branch is the current branch name (or HEAD/detached short sha).
	Branch string `json:"branch"`
	// IsRepo is false when the workspace is not a git repository.
	IsRepo bool `json:"is_repo"`
	// Root is the absolute path of the selected git work tree.
	Root string `json:"root"`
	// RepoID is workspace-relative repo id ("." or nested folder path).
	RepoID string `json:"repo_id,omitempty"`
	// RepoName is a short label for multi-repo source control.
	RepoName string `json:"repo_name,omitempty"`
	// Files lists every changed path (staged, unstaged, untracked).
	// Paths are relative to this repo's root (not the workspace root).
	Files []FileStatus `json:"files"`
	// StagedCount is the number of paths with staged changes.
	StagedCount int `json:"staged_count"`
	// UnstagedCount is the number of paths with unstaged changes (includes untracked).
	UnstagedCount int `json:"unstaged_count"`
	// UntrackedCount is the number of untracked paths.
	UntrackedCount int `json:"untracked_count"`
	// Porcelain is the raw `git status --porcelain` output (for tests / evidence).
	Porcelain string `json:"porcelain"`
	// Upstream is the @{upstream} name when set.
	Upstream string `json:"upstream,omitempty"`
	// Ahead / Behind relative to upstream when known.
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// Message is a human-readable status line for the panel.
	Message string `json:"message,omitempty"`
	// Repos lists all discovered git repos under the workspace (multi source-control).
	Repos []RepoRef `json:"repos,omitempty"`
}

// DiffResult is unified diff text for one path.
type DiffResult struct {
	Path    string `json:"path"`
	// Staged requests index vs HEAD when true; worktree vs index when false.
	Staged  bool   `json:"staged"`
	// Content is the raw unified diff (or full file for untracked).
	Content string `json:"content"`
	// Empty is true when there is no diff output.
	Empty   bool   `json:"empty"`
	Message string `json:"message,omitempty"`
}

// OpResult is a generic success/failure payload for stage/commit/fetch/pull.
type OpResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	// Detail is optional stderr/stdout (truncated) for the panel status strip.
	Detail string `json:"detail,omitempty"`
	// Hash is set for commit (new commit sha short).
	Hash string `json:"hash,omitempty"`
	// RepoID echoes the selected repo for multi source-control UIs.
	RepoID string `json:"repo_id,omitempty"`
	// Status is a refreshed status after successful mutating ops.
	Status *Status `json:"status,omitempty"`
}

// CommitRequest is a UI commit payload.
type CommitRequest struct {
	Message string `json:"message"`
	// Paths optionally limits the commit (unused when empty — commits the index).
	Paths []string `json:"paths,omitempty"`
}

// Service runs git against a fixed workspace root with safe global flags.
type Service struct {
	root string
	// agentMode, when true, forces hooksPath=/dev/null (agent helpers).
	agentMode bool
	// timeout bounds each git invocation.
	timeout time.Duration
}

// Options configures New.
type Options struct {
	// Root is the absolute workspace root (must be a directory).
	Root string
	// AgentMode forces core.hooksPath=/dev/null for agent tool isolation.
	AgentMode bool
	// Timeout per command (default 60s).
	Timeout time.Duration
}

// New returns a GitService bound to root.
func New(opts Options) (*Service, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return nil, fmt.Errorf("git: empty workspace root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("git: abs: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("git: root: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("git: root is not a directory: %s", abs)
	}
	to := opts.Timeout
	if to <= 0 {
		to = 60 * time.Second
	}
	return &Service{root: abs, agentMode: opts.AgentMode, timeout: to}, nil
}

// Root returns the absolute workspace root.
func (s *Service) Root() string { return s.root }

// Status runs `git status --porcelain` (+ branch) matching panel requirements.
func (s *Service) Status() (Status, error) {
	out := Status{Root: s.root, Files: []FileStatus{}}
	if !s.isRepo() {
		out.IsRepo = false
		out.Message = "not a git repository"
		return out, nil
	}
	out.IsRepo = true
	branch, _ := s.runCapture("rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "HEAD" {
		sha, _ := s.runCapture("rev-parse", "--short", "HEAD")
		branch = "HEAD (" + strings.TrimSpace(sha) + ")"
	}
	out.Branch = branch

	porc, err := s.runCapture("status", "--porcelain")
	if err != nil {
		out.Message = err.Error()
		return out, err
	}
	out.Porcelain = porc
	out.Files = parsePorcelain(porc)
	for _, f := range out.Files {
		if f.Staged {
			out.StagedCount++
		}
		if f.Unstaged {
			out.UnstagedCount++
		}
		if f.Untracked {
			out.UntrackedCount++
		}
	}

	// Ahead/behind (best-effort; silent if no upstream).
	up, err := s.runCapture("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err == nil && strings.TrimSpace(up) != "" {
		out.Upstream = strings.TrimSpace(up)
		ab, abErr := s.runCapture("rev-list", "--left-right", "--count", "HEAD...@{upstream}")
		if abErr == nil {
			fields := strings.Fields(strings.TrimSpace(ab))
			if len(fields) >= 2 {
				fmt.Sscanf(fields[0], "%d", &out.Ahead)
				fmt.Sscanf(fields[1], "%d", &out.Behind)
			}
		}
	}

	out.Message = fmt.Sprintf(
		"%s · %d staged · %d unstaged · %d untracked",
		out.Branch, out.StagedCount, out.UnstagedCount, out.UntrackedCount,
	)
	return out, nil
}

// Diff returns unified diff for path. staged=true → --cached; else worktree.
// For untracked files, returns a synthetic "new file" listing of contents.
func (s *Service) Diff(path string, staged bool) (DiffResult, error) {
	res := DiffResult{Path: path, Staged: staged}
	path = normalizeRel(path)
	if path == "" {
		res.Message = "empty path"
		return res, fmt.Errorf("git: empty path")
	}
	if !s.isRepo() {
		res.Message = "not a git repository"
		return res, fmt.Errorf("git: not a repository")
	}

	// Detect untracked / missing index for unstaged diff.
	if !staged {
		st, _ := s.Status()
		for _, f := range st.Files {
			if f.Path == path && f.Untracked {
				content, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(path)))
				if err != nil {
					res.Message = err.Error()
					return res, err
				}
				// Cap size for panel.
				if len(content) > 256*1024 {
					content = content[:256*1024]
				}
				var b strings.Builder
				b.WriteString("diff --git a/" + path + " b/" + path + "\n")
				b.WriteString("new file mode 100644\n")
				b.WriteString("--- /dev/null\n")
				b.WriteString("+++ b/" + path + "\n")
				lines := strings.Split(string(content), "\n")
				for i, line := range lines {
					// Preserve trailing line without extra blank when file has no final newline.
					if i == len(lines)-1 && line == "" && strings.HasSuffix(string(content), "\n") == false {
						break
					}
					if i == len(lines)-1 && line == "" {
						continue
					}
					b.WriteString("+" + line + "\n")
				}
				res.Content = b.String()
				res.Empty = res.Content == ""
				res.Message = "untracked file"
				return res, nil
			}
		}
	}

	args := []string{"diff", "--no-ext-diff", "--no-color"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	text, err := s.runCapture(args...)
	if err != nil {
		// Untracked or missing — try showing empty cleanly.
		res.Message = err.Error()
		res.Content = text
		return res, err
	}
	res.Content = text
	res.Empty = strings.TrimSpace(text) == ""
	if res.Empty {
		res.Message = "no differences"
	}
	return res, nil
}

// Stage runs `git add -- <paths>`.
func (s *Service) Stage(paths ...string) (OpResult, error) {
	return s.mutatePaths("stage", "add", "--", paths)
}

// Unstage runs `git restore --staged -- <paths>` (fallback: reset HEAD --).
func (s *Service) Unstage(paths ...string) (OpResult, error) {
	rel, err := cleanPaths(paths)
	if err != nil {
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if len(rel) == 0 {
		err := fmt.Errorf("git: no paths to unstage")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	args := append([]string{"restore", "--staged", "--"}, rel...)
	out, err := s.runCapture(args...)
	if err != nil {
		// Older git: git reset HEAD -- paths
		args2 := append([]string{"reset", "HEAD", "--"}, rel...)
		out2, err2 := s.runCapture(args2...)
		if err2 != nil {
			msg := firstNonEmpty(strings.TrimSpace(out+" "+out2), err2.Error())
			return OpResult{OK: false, Message: "unstage failed", Detail: msg}, err2
		}
		out = out2
	}
	st, _ := s.Status()
	return OpResult{
		OK:      true,
		Message: fmt.Sprintf("unstaged %d path(s)", len(rel)),
		Detail:  truncate(out, 4000),
		Status:  &st,
	}, nil
}

// Commit creates a commit from the index with the given message.
// Uses -m and no amend/force flags. Env locks author for determinism in tests when set.
//
// VAL-CROSS-007: a pre-commit staged-secret guard runs before commit. If the
// index holds any path that looks like a secret (`.env`, `*.pem`, `id_rsa`,
// `credentials.json`, …), the commit is refused with ErrStagedSecrets and the
// tree is left unchanged. The check only inspects staged paths, so a stray
// `.env` in the working tree that was never `git add`-ed does not block.
func (s *Service) Commit(message string) (OpResult, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		err := fmt.Errorf("git: empty commit message")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	// Refuse empty index: ensure something is staged.
	st, err := s.Status()
	if err != nil {
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if st.StagedCount == 0 {
		err := fmt.Errorf("git: nothing staged to commit")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	// VAL-CROSS-007: pre-commit staged-secret guard. Refuse to commit if the
	// index holds any secret-looking path so .env / keys never land in history.
	if secRes, secErr := s.ensureCommitSafe(); secErr != nil {
		return OpResult{
			OK:      false,
			Message: "commit blocked: " + secRes.Message,
			Detail:  secErr.Error(),
		}, secErr
	}
	out, err := s.runCapture("commit", "-m", message)
	if err != nil {
		return OpResult{OK: false, Message: "commit failed", Detail: firstNonEmpty(strings.TrimSpace(out), err.Error())}, err
	}
	hash, _ := s.runCapture("rev-parse", "--short", "HEAD")
	hash = strings.TrimSpace(hash)
	st2, _ := s.Status()
	return OpResult{
		OK:      true,
		Message: fmt.Sprintf("committed %s", hash),
		Detail:  truncate(out, 4000),
		Hash:    hash,
		Status:  &st2,
	}, nil
}

// Fetch runs `git fetch` (no force). Reports success/failure for the panel.
func (s *Service) Fetch() (OpResult, error) {
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	out, err := s.runCapture("fetch", "--prune")
	if err != nil {
		return OpResult{
			OK:      false,
			Message: "fetch failed",
			Detail:  firstNonEmpty(strings.TrimSpace(out), err.Error()),
		}, err
	}
	st, _ := s.Status()
	msg := "fetch succeeded"
	if strings.TrimSpace(out) == "" {
		msg = "fetch succeeded (already up to date)"
	}
	return OpResult{OK: true, Message: msg, Detail: truncate(out, 4000), Status: &st}, nil
}

// Pull runs `git pull` (no rebase/force by default).
func (s *Service) Pull() (OpResult, error) {
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	out, err := s.runCapture("pull", "--no-rebase", "--ff-only")
	if err != nil {
		// Retry without --ff-only so a simple merge pull can succeed when policy allows;
		// still no force. Surface failure clearly for the UI.
		out2, err2 := s.runCapture("pull", "--no-rebase")
		if err2 != nil {
			return OpResult{
				OK:      false,
				Message: "pull failed",
				Detail:  firstNonEmpty(strings.TrimSpace(out+"\n"+out2), err2.Error()),
			}, err2
		}
		out = out2
	}
	st, _ := s.Status()
	return OpResult{
		OK:      true,
		Message: "pull succeeded",
		Detail:  truncate(out, 4000),
		Status:  &st,
	}, nil
}

// LogOneline returns the first n commits oneline (for evidence / future panel).
func (s *Service) LogOneline(n int) (string, error) {
	if n <= 0 {
		n = 10
	}
	return s.runCapture("log", "--oneline", fmt.Sprintf("-%d", n))
}

func (s *Service) mutatePaths(action, sub string, flag string, paths []string) (OpResult, error) {
	rel, err := cleanPaths(paths)
	if err != nil {
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if len(rel) == 0 {
		err := fmt.Errorf("git: no paths to %s", action)
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	args := []string{sub}
	if flag != "" {
		args = append(args, flag)
	}
	args = append(args, rel...)
	out, err := s.runCapture(args...)
	if err != nil {
		return OpResult{
			OK:      false,
			Message: action + " failed",
			Detail:  firstNonEmpty(strings.TrimSpace(out), err.Error()),
		}, err
	}
	st, _ := s.Status()
	return OpResult{
		OK:      true,
		Message: fmt.Sprintf("%s %d path(s)", action+"d", len(rel)),
		Detail:  truncate(out, 4000),
		Status:  &st,
	}, nil
}

func (s *Service) isRepo() bool {
	// .git file (worktree) or directory
	out, err := s.runCapture("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// runCapture executes git with safe config flags and captures combined output.
func (s *Service) runCapture(args ...string) (string, error) {
	full := s.prefixArgs(args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = s.root
	// Isolate from user env that might inject weird config; keep PATH/HOME.
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// prefixArgs prepends safe -c flags and -C is not needed (cmd.Dir set).
func (s *Service) prefixArgs(args ...string) []string {
	// Safe flags on every call (docs/surfaces.md + VAL-GIT-008 prep).
	pre := []string{
		"--no-optional-locks",
		"-c", "core.autocrlf=false",
		"-c", "core.fsmonitor=false",
		"-c", "core.quotepath=false",
	}
	if s.agentMode {
		// Agent isolation: never run user hooks unexpectedly.
		pre = append(pre, "-c", "core.hooksPath=/dev/null")
	}
	return append(pre, args...)
}

// parsePorcelain parses `git status --porcelain` (v1) lines.
func parsePorcelain(porc string) []FileStatus {
	lines := strings.Split(porc, "\n")
	var out []FileStatus
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Porcelain: XY path  or  XY old -> new  (rename)
		// X/Y may be spaces; need at least 3 chars "XY ".
		if len(line) < 3 {
			continue
		}
		x := line[0]
		y := line[1]
		rest := strings.TrimSpace(line[2:])
		var original, path string
		if i := strings.Index(rest, " -> "); i >= 0 {
			original = rest[:i]
			path = rest[i+4:]
		} else {
			path = rest
		}
		path = filepath.ToSlash(path)
		original = filepath.ToSlash(original)
		f := FileStatus{
			Path:      path,
			Original:  original,
			Index:     string(x),
			WorkTree:  string(y),
			Untracked: x == '?' && y == '?',
		}
		// Staged if index has a non-space, non-? status.
		if x != ' ' && x != '?' {
			f.Staged = true
		}
		// Unstaged if worktree dirty or untracked.
		if y != ' ' || f.Untracked {
			f.Unstaged = true
		}
		f.StatusLabel = statusLabel(x, y, f.Untracked)
		out = append(out, f)
	}
	return out
}

func statusLabel(x, y byte, untracked bool) string {
	if untracked {
		return "?"
	}
	// Prefer index letter, then worktree.
	for _, c := range []byte{x, y} {
		switch c {
		case 'M', 'A', 'D', 'R', 'C', 'U':
			return string(c)
		}
	}
	if y == 'M' {
		return "M"
	}
	return "M"
}

func cleanPaths(paths []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, p := range paths {
		p = normalizeRel(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "..") || filepath.IsAbs(p) {
			return nil, fmt.Errorf("git: invalid path %q", p)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}

func normalizeRel(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	p = filepath.ToSlash(p)
	return p
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
