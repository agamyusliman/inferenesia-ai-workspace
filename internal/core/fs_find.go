package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// FindHit is one match from Find in Folder (VAL-IDE-008).
type FindHit struct {
	// Path is workspace-relative slash path of the matching file.
	Path string `json:"path"`
	// Line is 1-based line number for content matches; 0 for path-only hits.
	Line int `json:"line,omitempty"`
	// Text is a short snippet of the matching line (truncated).
	Text string `json:"text,omitempty"`
	// Kind is "content" or "path".
	Kind string `json:"kind"`
}

// FindInFolder searches files under folderRel (workspace-relative) for query.
// Matches are scoped strictly under that folder; paths outside never appear.
// Both path basenames and file content lines are matched (case-insensitive).
// Empty query returns an empty list (not an error).
func (s *Service) FindInFolder(workspaceID, folderRel, query string, limit int) ([]FindHit, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return nil, err
	}
	folderRel = strings.TrimSpace(folderRel)
	folderRel = strings.TrimPrefix(folderRel, "./")
	folderRel = strings.ReplaceAll(folderRel, "\\", "/")
	if folderRel == "." {
		folderRel = ""
	}

	var absFolder string
	var cleanFolder string
	if folderRel == "" {
		absFolder = root
		cleanFolder = ""
	} else {
		absFolder, cleanFolder, err = resolveUnderRoot(root, folderRel)
		if err != nil {
			return nil, err
		}
	}
	st, err := os.Stat(absFolder)
	if err != nil {
		return nil, fmt.Errorf("core: find folder: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("core: find target is not a folder: %s", cleanFolder)
	}

	q := strings.TrimSpace(query)
	if q == "" {
		return []FindHit{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	qLower := strings.ToLower(q)
	out := make([]FindHit, 0)

	// Walk only the selected folder so sibling-tree hits never leak (VAL-IDE-008).
	err = filepath.WalkDir(absFolder, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == ".cache" ||
				name == ".next" || name == "target" || name == "__pycache__" ||
				name == "dist" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if name == ".env" || (strings.HasPrefix(name, ".env.") && name != ".env.example") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		// Enforce folder scope (VAL-IDE-008 negative: sibling paths must not appear).
		if !underFolder(rel, cleanFolder) {
			return nil
		}

		// Path/name match
		if strings.Contains(strings.ToLower(rel), qLower) ||
			strings.Contains(strings.ToLower(name), qLower) {
			out = append(out, FindHit{Path: rel, Kind: "path", Text: name})
			if len(out) >= limit {
				return filepath.SkipAll
			}
		}

		// Content match (text files only, skip binaries and large files)
		hits, done := contentHits(path, rel, qLower, limit-len(out))
		out = append(out, hits...)
		if done || len(out) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	return out, nil
}

// underFolder reports whether rel is cleanFolder itself or a descendant.
// cleanFolder empty means workspace root (everything under root is in scope).
func underFolder(rel, cleanFolder string) bool {
	rel = strings.TrimPrefix(rel, "./")
	if cleanFolder == "" {
		return rel != "" // exclude empty root pseudo-path
	}
	if rel == cleanFolder {
		return true
	}
	return strings.HasPrefix(rel, cleanFolder+"/")
}

func contentHits(abs, rel, qLower string, remaining int) (hits []FindHit, capped bool) {
	if remaining <= 0 {
		return nil, true
	}
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return nil, false
	}
	// Skip large files (>1 MiB) for content search.
	if st.Size() > 1<<20 {
		return nil, false
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	// Quick binary sniff: first 512 bytes with NUL → skip
	head := make([]byte, 512)
	n, _ := f.Read(head)
	if n > 0 {
		for i := 0; i < n; i++ {
			if head[i] == 0 {
				return nil, false
			}
		}
		// Rewind for full scan.
		if _, err := f.Seek(0, 0); err != nil {
			return nil, false
		}
	}

	sc := bufio.NewScanner(f)
	// Allow long lines (1 MiB buffer).
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if !utf8.ValidString(line) {
			continue
		}
		if !strings.Contains(strings.ToLower(line), qLower) {
			continue
		}
		snippet := strings.TrimSpace(line)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		hits = append(hits, FindHit{
			Path: rel,
			Line: lineNo,
			Text: snippet,
			Kind: "content",
		})
		if len(hits) >= remaining {
			return hits, true
		}
	}
	return hits, false
}
