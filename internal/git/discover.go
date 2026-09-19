package git

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepoRef describes one nested git repository under a workspace root.
// A single workspace may contain multiple independent repos (multi-root style).
type RepoRef struct {
	// ID is a stable relative path from workspace root ("." for the workspace itself).
	ID string `json:"id"`
	// Name is a short display label (folder name, or workspace base name).
	Name string `json:"name"`
	// Rel is the path relative to the workspace ("" or "." for workspace root).
	Rel string `json:"rel"`
	// Root is the absolute filesystem path of the git work tree.
	Root string `json:"root"`
	// Branch is current branch when available (best-effort).
	Branch string `json:"branch,omitempty"`
	// DirtyCount is staged+unstaged file count (best-effort for list badges).
	DirtyCount int `json:"dirty_count"`
	// IsRepo always true for discovered entries.
	IsRepo bool `json:"is_repo"`
}

// DiscoverRepos walks workspaceRoot for nested .git dirs/files.
// Depth is shallow: workspace root + immediate children that are repos,
// plus one more level of nesting under non-repo folders (capped).
// Skips node_modules, .git, dist, build, vendor, target, .cache.
func DiscoverRepos(workspaceRoot string) ([]RepoRef, error) {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("git: abs workspace: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("git: workspace: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("git: workspace is not a directory")
	}

	var found []string
	// 1) Workspace itself
	if isGitWorkTree(abs) {
		found = append(found, abs)
	}

	// 2) Walk limited depth for nested repos
	const maxDepth = 3 // workspace + 3 levels (e.g. apps/cms-a)
	_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path == abs {
			return nil
		}
		name := d.Name()
		if shouldSkipDir(name) {
			return filepath.SkipDir
		}
		// Depth relative to workspace
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/") + 1
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if isGitWorkTree(path) {
			found = append(found, path)
			// Do not walk inside a nested repo (treat as independent project root).
			return filepath.SkipDir
		}
		return nil
	})

	// Unique + stable order
	seen := map[string]bool{}
	var refs []RepoRef
	for _, root := range found {
		root = filepath.Clean(root)
		if seen[root] {
			continue
		}
		seen[root] = true
		rel, _ := filepath.Rel(abs, root)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		id := rel
		if id == "" {
			id = "."
		}
		name := filepath.Base(root)
		if rel == "" {
			name = filepath.Base(abs)
		}
		refs = append(refs, RepoRef{
			ID:     id,
			Name:   name,
			Rel:    rel,
			Root:   root,
			IsRepo: true,
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		// Workspace root (.) first, then by id
		if refs[i].ID == "." {
			return true
		}
		if refs[j].ID == "." {
			return false
		}
		return refs[i].ID < refs[j].ID
	})
	return refs, nil
}

// EnrichRepos fills branch + dirty counts for UI badges (best-effort).
func EnrichRepos(refs []RepoRef) []RepoRef {
	out := make([]RepoRef, len(refs))
	for i, r := range refs {
		out[i] = r
		svc, err := New(Options{Root: r.Root})
		if err != nil {
			continue
		}
		st, err := svc.Status()
		if err != nil || !st.IsRepo {
			continue
		}
		out[i].Branch = st.Branch
		out[i].DirtyCount = st.StagedCount + st.UnstagedCount
	}
	return out
}

// ResolveRepoRoot maps repo id (relative or ".") under workspaceRoot to an absolute path.
func ResolveRepoRoot(workspaceRoot, repoID string) (string, error) {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(repoID)
	if id == "" || id == "." {
		return abs, nil
	}
	id = strings.TrimPrefix(id, "./")
	if strings.Contains(id, "..") || filepath.IsAbs(id) {
		return "", fmt.Errorf("git: invalid repo id %q", repoID)
	}
	root := filepath.Clean(filepath.Join(abs, filepath.FromSlash(id)))
	if root != abs && !strings.HasPrefix(root, abs+string(os.PathSeparator)) {
		return "", fmt.Errorf("git: repo id escapes workspace")
	}
	if !isGitWorkTree(root) {
		return "", fmt.Errorf("git: not a repository: %s", id)
	}
	return root, nil
}

func isGitWorkTree(dir string) bool {
	// .git directory or gitfile
	p := filepath.Join(dir, ".git")
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	if st.IsDir() {
		return true
	}
	// git worktree: .git is a file
	if st.Mode().IsRegular() {
		return true
	}
	return false
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", "out", "vendor", "target",
		".cache", ".next", ".turbo", ".vercel", "coverage", "__pycache__",
		".venv", "venv", ".tox", "Pods", "Carthage":
		return true
	}
	if strings.HasPrefix(name, ".") && name != ".yura-ai" && name != ".multibrain" {
		// Skip most hidden dirs when scanning for nested repos
		return true
	}
	return false
}
