package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ExploreRunner is a built-in read-only sub-task runner for category=explore
// (VAL-ORCH-001, VAL-ORCH-011). It walks/search the workspace without writes
// so results can be synthesized by the primary without re-running full search.
type ExploreRunner struct {
	// Root is the workspace absolute path.
	Root string
	// MaxFiles caps how many files are listed/sampled (default 40).
	MaxFiles int
	// MaxBytes caps content snippet bytes per file (default 2KiB).
	MaxBytes int
}

// Run implements RunnerFunc-compatible explore body.
func (e ExploreRunner) Run(ctx context.Context, run *TaskRun) (summary string, results []string, err error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	root := strings.TrimSpace(e.Root)
	if root == "" {
		return "", nil, fmt.Errorf("explore: workspace root is required")
	}
	maxFiles := e.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 40
	}
	maxBytes := e.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 10
	}

	// Derive simple keywords from the prompt (tokens ≥3 chars).
	keywords := tokenize(run.Prompt)
	var hits []string
	var listed int

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil // skip unreadable
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name := d.Name()
		// Skip noisy / large trees.
		if d.IsDir() {
			switch name {
			case ".git", "node_modules", "vendor", "bin", "dist", ".wails", ".yura-ai":
				return filepath.SkipDir
			}
			return nil
		}
		// Skip obvious binaries / huge media.
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".woff", ".woff2",
			".exe", ".dll", ".so", ".dylib", ".bin", ".zip", ".tar", ".gz":
			return nil
		}
		listed++
		if listed > maxFiles*4 {
			// Soft bound on walk breadth.
			return filepath.SkipAll
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		// Match keywords against path or content.
		matched := len(keywords) == 0
		lowPath := strings.ToLower(rel)
		for _, kw := range keywords {
			if strings.Contains(lowPath, kw) {
				matched = true
				break
			}
		}
		if !matched {
			data, rerr := os.ReadFile(path)
			if rerr != nil || !utf8.Valid(data) {
				return nil
			}
			if len(data) > maxBytes {
				data = data[:maxBytes]
			}
			low := strings.ToLower(string(data))
			for _, kw := range keywords {
				if strings.Contains(low, kw) {
					matched = true
					// Keep a short snippet around first keyword.
					idx := strings.Index(low, kw)
					start := idx - 40
					if start < 0 {
						start = 0
					}
					end := idx + len(kw) + 40
					if end > len(data) {
						end = len(data)
					}
					snippet := strings.ReplaceAll(string(data[start:end]), "\n", " ")
					hits = append(hits, fmt.Sprintf("%s: …%s…", rel, strings.TrimSpace(snippet)))
					break
				}
			}
		} else {
			hits = append(hits, fmt.Sprintf("%s (path match)", rel))
		}
		if len(hits) >= maxFiles {
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil && walkErr != filepath.SkipAll {
		// context cancel
		if ctx.Err() != nil {
			return "", hits, ctx.Err()
		}
	}

	if len(hits) == 0 {
		summary = fmt.Sprintf("explore: no matches for prompt %q under %s", truncate(run.Prompt, 80), root)
		results = []string{summary}
		return summary, results, nil
	}
	summary = fmt.Sprintf("explore: %d hit(s) for %q", len(hits), truncate(run.Prompt, 60))
	results = hits
	return summary, results, nil
}

// NewExploreRunnerFunc returns a RunnerFunc bound to workspace root.
func NewExploreRunnerFunc(root string) RunnerFunc {
	er := ExploreRunner{Root: root}
	return er.Run
}

// WriteRunner is a test/implement helper that writes content via a callback
// after acquiring the manager write lock (VAL-ORCH-003).
type WriteFunc func(path string, content []byte) error

// LockedWriteRunner returns a RunnerFunc that:
//  1. acquires write lock on Spec-derived path from run.Prompt (format: "WRITE path=... content=...")
//  2. calls write
//  3. releases lock
//
// Used by tests for conflicting parallel writes. Production implement category
// will use tools through WriteGateway similarly.
func LockedWriteRunner(m *Manager, write WriteFunc) RunnerFunc {
	return func(ctx context.Context, run *TaskRun) (string, []string, error) {
		path, content, perr := parseWritePrompt(run.Prompt)
		if perr != nil {
			return "", nil, perr
		}
		if err := m.AcquireWriteLock(run.TaskID, path); err != nil {
			return "", nil, err
		}
		defer m.ReleaseWriteLock(run.TaskID, path)
		defer m.ReleaseAllLocks(run.TaskID)

		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		if write == nil {
			return "", nil, fmt.Errorf("write runner: write func is nil")
		}
		if err := write(path, []byte(content)); err != nil {
			return "", nil, err
		}
		sum := fmt.Sprintf("wrote %d bytes to %s", len(content), path)
		return sum, []string{sum}, nil
	}
}

// parseWritePrompt accepts:
//
//	WRITE path=rel/path content=bytes
//
// or free-form with path= and content= tokens.
func parseWritePrompt(prompt string) (path, content string, err error) {
	prompt = strings.TrimSpace(prompt)
	// Simple key extraction.
	path = extractKV(prompt, "path")
	content = extractKV(prompt, "content")
	if path == "" {
		return "", "", fmt.Errorf("write runner: path= required in prompt")
	}
	if content == "" {
		content = "parallel-write"
	}
	return path, content, nil
}

func extractKV(s, key string) string {
	// Find key= value until next known key or end.
	idx := strings.Index(s, key+"=")
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(key)+1:]
	// Stop at " path=" / " content=" / end (space-delimited keys only for first word of content is whole rest).
	if key == "content" {
		return strings.TrimSpace(rest)
	}
	// path ends at space + another key or end
	for _, stop := range []string{" content=", " path="} {
		if j := strings.Index(rest, stop); j >= 0 {
			rest = rest[:j]
			break
		}
	}
	return strings.TrimSpace(rest)
}

func tokenize(prompt string) []string {
	fields := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	})
	stop := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"find": true, "where": true, "what": true, "this": true, "that": true,
		"explore": true, "search": true, "look": true, "into": true, "under": true,
		"please": true, "show": true, "list": true, "all": true, "any": true,
	}
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		if len(f) < 3 || stop[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= 8 {
			break
		}
	}
	return out
}
