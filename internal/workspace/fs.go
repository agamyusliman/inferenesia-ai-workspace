package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Default hidden/skip names for the explorer tree (not exhaustive gitignore).
var defaultSkipNames = map[string]bool{
	".git":         true,
	"node_modules": true,
	".DS_Store":    true,
	"dist":         true,
	"build":        true,
	".vite":        true,
	".turbo":       true,
	"coverage":     true,
	"vendor":       true,
	// Never surface live env/secrets in the explorer tree.
	".env": true,
}

// DirEntry is one node in the explorer tree.
type DirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"` // relative to workspace root, forward-slash style
	IsDir bool   `json:"is_dir"`
}

// DefaultReadMaxBytes is the default window for ReadFileText when maxBytes <= 0.
// Large files are truncated/chunked — never hard-rejected solely for size.
const DefaultReadMaxBytes int64 = 256 * 1024

// ListDir lists immediate children under rel (relative to root).
// rel="" or "." means the workspace root. Paths outside root are denied.
func ListDir(root, rel string) ([]DirEntry, error) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return nil, fmt.Errorf("workspace: empty root")
	}
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.ReplaceAll(rel, "\\", "/")
	if rel == "." {
		rel = ""
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return nil, fmt.Errorf("workspace: absolute path denied")
	}
	if strings.Contains(rel, "..") {
		return nil, fmt.Errorf("workspace: path escapes root")
	}
	abs := root
	if rel != "" {
		abs = filepath.Join(root, filepath.FromSlash(rel))
	}
	abs = filepath.Clean(abs)
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return nil, fmt.Errorf("workspace: path escapes root")
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace: readdir: %w", err)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if defaultSkipNames[name] {
			continue
		}
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		out = append(out, DirEntry{
			Name:  name,
			Path:  childRel,
			IsDir: e.IsDir(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ReadFileResult is a bounded window of a workspace file (chunk-friendly).
type ReadFileResult struct {
	Content   string
	TotalSize int64
	Offset    int64
	Truncated bool
	// NextOffset is set when Truncated and more bytes remain (for follow-up reads).
	NextOffset int64
	Binary     bool
}

// ReadFileText reads a workspace-relative text file.
// Large files are NOT rejected: a window of at most maxBytes is returned
// starting at offset (default 0). Binary content is detected and reported.
func ReadFileText(root, rel string, maxBytes int64) (string, error) {
	res, err := ReadFileWindow(root, rel, 0, maxBytes)
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// ReadFileWindow returns a byte window of a file for editor / agent / @mention.
// offset is the start byte; maxBytes is the max window size (default 256KiB).
// Never fails solely because the full file is larger than maxBytes.
func ReadFileWindow(root, rel string, offset, maxBytes int64) (ReadFileResult, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultReadMaxBytes
	}
	if offset < 0 {
		offset = 0
	}
	root = filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" {
		return ReadFileResult{}, fmt.Errorf("workspace: empty file path")
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	absClean := filepath.Clean(abs)
	if absClean != root && !strings.HasPrefix(absClean, root+string(os.PathSeparator)) {
		return ReadFileResult{}, fmt.Errorf("workspace: path escapes root")
	}
	st, err := os.Stat(absClean)
	if err != nil {
		return ReadFileResult{}, fmt.Errorf("workspace: stat: %w", err)
	}
	if st.IsDir() {
		return ReadFileResult{}, fmt.Errorf("workspace: is a directory")
	}
	total := st.Size()
	if offset >= total {
		return ReadFileResult{
			Content:   "",
			TotalSize: total,
			Offset:    offset,
			Truncated: false,
		}, nil
	}

	f, err := os.Open(absClean)
	if err != nil {
		return ReadFileResult{}, fmt.Errorf("workspace: open: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ReadFileResult{}, fmt.Errorf("workspace: seek: %w", err)
	}

	// Read window + a small probe for binary / incomplete UTF-8 at boundary.
	toRead := maxBytes
	remain := total - offset
	truncated := remain > maxBytes
	if !truncated {
		toRead = remain
	}
	buf := make([]byte, toRead)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return ReadFileResult{}, fmt.Errorf("workspace: read: %w", err)
	}
	data := buf[:n]

	// Binary probe: NUL in window or invalid UTF-8 (common for bins/images).
	if isLikelyBinary(data) {
		return ReadFileResult{
			Content: fmt.Sprintf(
				"[binary file: %s — size %d bytes; not loaded into chat. Use shell/file tools for binaries, or read a text path.]",
				rel, total,
			),
			TotalSize: total,
			Offset:    offset,
			Truncated: false,
			Binary:    true,
		}, nil
	}

	// Fix incomplete trailing UTF-8 rune if we truncated mid-sequence.
	if truncated {
		data = trimIncompleteUTF8(data)
	}
	if !utf8.Valid(data) {
		// Still not valid text after trim — treat as binary-ish.
		return ReadFileResult{
			Content: fmt.Sprintf(
				"[non-UTF-8 file: %s — size %d bytes; showing first %d bytes as escaped preview is disabled. Open a text file instead.]",
				rel, total, len(data),
			),
			TotalSize: total,
			Offset:    offset,
			Truncated: truncated,
			Binary:    true,
		}, nil
	}

	body := string(data)
	next := int64(0)
	if truncated {
		next = offset + int64(len(data))
		body += fmt.Sprintf(
			"\n…[truncated: showing bytes %d–%d of %d; call again with offset=%d max_bytes=%d for next chunk]",
			offset, offset+int64(len(data)), total, next, maxBytes,
		)
	}

	return ReadFileResult{
		Content:    body,
		TotalSize:  total,
		Offset:     offset,
		Truncated:  truncated,
		NextOffset: next,
		Binary:     false,
	}, nil
}

func isLikelyBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// NUL byte is a strong binary signal.
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	// High ratio of non-text control bytes (excluding tab/lf/cr).
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	ctrl := 0
	for _, b := range sample {
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			ctrl++
		}
	}
	return ctrl*100/len(sample) > 5
}

func trimIncompleteUTF8(data []byte) []byte {
	if len(data) == 0 || utf8.Valid(data) {
		return data
	}
	// Drop up to 3 trailing bytes that may be an incomplete rune.
	for i := 0; i < 4 && len(data) > 0; i++ {
		data = data[:len(data)-1]
		if utf8.Valid(data) {
			return data
		}
	}
	return data
}
