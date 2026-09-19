package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileMention is a workspace-relative file path for chat @mention pickers.
type FileMention struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// ListFiles walks the workspace tree and returns file paths for mention pickers.
// Skips heavy/hidden dirs (node_modules, .git, …). Cap limits results (default 400).
func ListFiles(root string, query string, cap int) ([]FileMention, error) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return nil, fmt.Errorf("workspace: empty root")
	}
	if cap <= 0 {
		cap = 400
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace: root is not a directory")
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var out []FileMention

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable branches.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if defaultSkipNames[name] {
				return filepath.SkipDir
			}
			// Skip other large cache-style dirs.
			if name == ".cache" || name == ".next" || name == "target" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		// Hide env/secret file names from mention list.
		if name == ".env" || (strings.HasPrefix(name, ".env.") && name != ".env.example") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if q != "" {
			lower := strings.ToLower(rel)
			if !strings.Contains(lower, q) && !strings.Contains(strings.ToLower(name), q) {
				return nil
			}
		}
		out = append(out, FileMention{Path: rel, Name: name})
		if len(out) >= cap {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	return out, nil
}
