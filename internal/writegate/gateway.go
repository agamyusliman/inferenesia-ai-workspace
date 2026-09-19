// Package writegate is the single mutation path for agent file writes.
// All agent tools that change files must go through Gateway (VAL-UNDO-007, architecture).
//
// Snapshot-before-write + Undo/Redo restore pre-agent dirty content, never git HEAD
// by default (VAL-UNDO-001/002/003).
package writegate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrSandboxDenied is returned when a path resolves outside the workspace root.
var ErrSandboxDenied = fmt.Errorf("sandbox denied: path is outside the workspace root")

// Snapshot holds the pre-mutation state for a single path.
type Snapshot struct {
	// Path is the absolute path within the workspace.
	Path string `json:"path"`
	// Rel is the path relative to the workspace root (slash-normalized).
	Rel string `json:"rel"`
	// Before is the file bytes before the mutation (nil if the file did not exist).
	// JSON marshals as base64 for binary-safe persistence.
	Before []byte `json:"before"`
	// Existed is true when Path existed before the mutation.
	Existed bool `json:"existed"`
}

// Mutation source identifiers (VAL-UNDO-007 — every agent file path tags its origin).
// SourceUser tags manual desktop editor saves (user edit, not an agent turn).
// SourceResearch tags research artifacts written via the research skill
// (VAL-CROSS-005: docs/research/<slug>.md with screenshot referenced by path).
const (
	SourceFS         = "fs"
	SourceMultibrain = "multibrain"
	SourceDiagram    = "diagram"
	SourceUser       = "user"
	SourceResearch   = "research"
)

// Entry is one recorded mutation on the undo stack (write or delete).
type Entry struct {
	Snapshot Snapshot `json:"snapshot"`
	// After is the written content (intended content for failed mid-op recovery).
	// Nil when AfterAbsent is true (delete / remove).
	After []byte `json:"after"`
	// AfterAbsent is true when the agent mutation left the path deleted (VAL-UNDO-010).
	AfterAbsent bool `json:"after_absent,omitempty"`
	// Source tags which agent tool surface produced the mutation (fs, multibrain, diagram).
	Source string `json:"source,omitempty"`
}

// Gateway enforces sandbox and records snapshots before every write.
// Undo restores Snapshot.Before (pre-agent dirty), not git HEAD.
//
// Turn batching (VAL-UNDO-004/005): call BeginTurn before agent tool writes and
// EndTurn when the agent turn completes. All writes between those calls form
// one LIFO undo unit. Writes outside a turn remain single-entry units.
type Gateway struct {
	// Root is the absolute workspace root. Writes must stay under Root.
	Root string

	// Entries records successful writes in call order (flat history).
	Entries []Entry

	// undoStack / redoStack are unit stacks (LIFO). Unexported; use Undo/Redo/UndoStack.
	undoStack []StackUnit
	redoStack []StackUnit

	// openTurn, when non-nil, batches WriteFile calls into one StackUnit until EndTurn.
	openTurn *openTurn

	onChange OnChange
}

// openTurn accumulates entries for the active agent turn.
type openTurn struct {
	id      string
	entries []Entry
}

// New creates a Gateway for the given workspace root (must exist and be a directory).
func New(workspaceRoot string) (*Gateway, error) {
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("writegate: resolve workspace: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		// Fall back to Abs if EvalSymlinks fails (e.g. root does not exist yet).
		if !os.IsNotExist(err) {
			// Still require that Abs path is a directory when it exists.
			return nil, fmt.Errorf("writegate: eval workspace: %w", err)
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("writegate: workspace %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("writegate: workspace %q is not a directory", root)
	}
	// Prefer eval'd path when available.
	if eval, err := filepath.EvalSymlinks(root); err == nil {
		root = eval
	}
	return &Gateway{Root: root}, nil
}

// RootPath returns the absolute workspace root.
func (g *Gateway) RootPath() string {
	if g == nil {
		return ""
	}
	return g.Root
}

// ResolveInWorkspace cleans and resolves path under the workspace.
// path may be absolute (must still be under root) or relative to root.
// Returns absolute path and relative path from root.
func (g *Gateway) ResolveInWorkspace(path string) (abs, rel string, err error) {
	if g == nil || g.Root == "" {
		return "", "", fmt.Errorf("writegate: gateway not configured")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", fmt.Errorf("writegate: empty path")
	}
	// Null bytes and other control chars are always denied.
	if strings.ContainsRune(path, 0) {
		return "", "", fmt.Errorf("%w: invalid path", ErrSandboxDenied)
	}

	var candidate string
	if filepath.IsAbs(path) {
		candidate = filepath.Clean(path)
	} else {
		candidate = filepath.Clean(filepath.Join(g.Root, path))
	}

	// If the path exists, evaluate symlinks so a symlink escape cannot leave the root.
	// If it does not exist, evaluate parent directories that do exist.
	resolved, err := resolveExistingPrefix(candidate)
	if err != nil {
		return "", "", fmt.Errorf("writegate: resolve path: %w", err)
	}

	root := g.Root
	if evalRoot, err := filepath.EvalSymlinks(root); err == nil {
		root = evalRoot
	}
	root = filepath.Clean(root)

	if !isUnderRoot(resolved, root) {
		return "", "", fmt.Errorf("%w: %s", ErrSandboxDenied, path)
	}

	rel, err = filepath.Rel(root, resolved)
	if err != nil {
		return "", "", fmt.Errorf("%w: %s", ErrSandboxDenied, path)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("%w: %s", ErrSandboxDenied, path)
	}
	// Normalize rel to slash for stable reporting.
	rel = filepath.ToSlash(rel)
	if rel == "." {
		rel = ""
	}
	return resolved, rel, nil
}

// Prepare resolves path under the workspace and snapshots current content.
// It does not mutate the file.
func (g *Gateway) Prepare(path string) (Snapshot, error) {
	abs, rel, err := g.ResolveInWorkspace(path)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Path: abs, Rel: rel}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			snap.Existed = false
			snap.Before = nil
			return snap, nil
		}
		return Snapshot{}, fmt.Errorf("writegate: read before write %s: %w", rel, err)
	}
	snap.Existed = true
	snap.Before = data
	return snap, nil
}

// IsSandboxDenied reports whether err is (or wraps) ErrSandboxDenied.
func IsSandboxDenied(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "sandbox denied") || err == ErrSandboxDenied
}

func isUnderRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(path, root+sep)
}

// resolveExistingPrefix evaluates symlinks for the longest existing prefix of path.
func resolveExistingPrefix(path string) (string, error) {
	path = filepath.Clean(path)
	// Walk up until we find an existing component.
	cur := path
	var missing []string
	for {
		if _, err := os.Lstat(cur); err == nil {
			eval, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			// Re-join missing tail onto evaluated existing prefix.
			for i := len(missing) - 1; i >= 0; i-- {
				eval = filepath.Join(eval, missing[i])
			}
			return filepath.Clean(eval), nil
		}
		dir, base := filepath.Dir(cur), filepath.Base(cur)
		if dir == cur {
			// Reached filesystem root without existing path — treat as absolute clean path.
			return path, nil
		}
		missing = append(missing, base)
		cur = dir
	}
}
