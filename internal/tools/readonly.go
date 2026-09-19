package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Read-only tool names for Plan Mode / explore (VAL-PLAN-002).
const (
	NameReadFile = "read_file"
	NameGrep     = "grep"
	NameListDir  = "list_dir"
)

func readOnlyDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameReadFile,
				Description: "Read a UTF-8 text file inside the workspace (path relative to root). Large files are truncated into chunks — use offset to continue. Binary files return a short notice, not an error. Does not modify files.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "File path relative to workspace root" },
    "offset": { "type": "integer", "description": "Byte offset to start reading (default 0; use next_offset from a truncated read)" },
    "max_bytes": { "type": "integer", "description": "Max bytes for this chunk (default 64KiB, max 512KiB)" }
  },
  "required": ["path"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGrep,
				Description: "Search file contents under the workspace (or a subfolder) for a substring or regex. Read-only; does not modify files.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": { "type": "string", "description": "Substring or Go-style regex to search for" },
    "path": { "type": "string", "description": "Optional folder/file relative to workspace (default: workspace root)" },
    "regex": { "type": "boolean", "description": "If true, treat pattern as regex (default false = literal substring)" },
    "case_insensitive": { "type": "boolean", "description": "Case-insensitive match" },
    "max_matches": { "type": "integer", "description": "Max matching lines to return (default 50)" }
  },
  "required": ["pattern"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameListDir,
				Description: "List files and directories under a workspace-relative path. Read-only.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Directory relative to workspace root (default \".\")" }
  }
}`),
			},
		},
	}
}

type readFileArgs struct {
	Path     string `json:"path"`
	Offset   int    `json:"offset"`
	MaxBytes int    `json:"max_bytes"`
}

type grepArgs struct {
	Pattern         string `json:"pattern"`
	Path            string `json:"path"`
	Regex           bool   `json:"regex"`
	CaseInsensitive bool   `json:"case_insensitive"`
	MaxMatches      int    `json:"max_matches"`
}

type listDirArgs struct {
	Path string `json:"path"`
}

func (r *Registry) execReadFile(call Call) Result {
	if r.gw == nil {
		return Result{Name: NameReadFile, Content: "read_file: WriteGateway/workspace not configured", IsError: true}
	}
	var args readFileArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameReadFile, Content: fmt.Sprintf("read_file: invalid arguments: %v", err), IsError: true}
	}
	path := strings.TrimSpace(args.Path)
	if path == "" {
		return Result{Name: NameReadFile, Content: "read_file: path is required", IsError: true}
	}
	abs, err := r.resolveWorkspacePath(path)
	if err != nil {
		return Result{Name: NameReadFile, Content: err.Error(), IsError: true}
	}
	max := args.MaxBytes
	if max <= 0 {
		max = 64 << 10
	}
	if max > 512<<10 {
		max = 512 << 10
	}
	offset := args.Offset
	if offset < 0 {
		offset = 0
	}

	// Use windowed read so large files are chunked, not rejected.
	// Map abs back to workspace-relative for workspace.ReadFileWindow via open+seek.
	st, err := os.Stat(abs)
	if err != nil {
		return Result{Name: NameReadFile, Content: fmt.Sprintf("read_file: %v", err), IsError: true}
	}
	if st.IsDir() {
		// Model often passes a folder to read_file — auto list instead of hard error.
		return r.listDirAt(path, abs)
	}
	total := st.Size()
	if int64(offset) >= total {
		return Result{Name: NameReadFile, Content: fmt.Sprintf("read_file: offset %d past end (size %d)", offset, total)}
	}

	f, err := os.Open(abs)
	if err != nil {
		return Result{Name: NameReadFile, Content: fmt.Sprintf("read_file: %v", err), IsError: true}
	}
	defer f.Close()
	if _, err := f.Seek(int64(offset), 0); err != nil {
		return Result{Name: NameReadFile, Content: fmt.Sprintf("read_file: seek: %v", err), IsError: true}
	}

	toRead := max
	remain := int(total - int64(offset))
	truncated := remain > max
	if !truncated {
		toRead = remain
	}
	buf := make([]byte, toRead)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return Result{Name: NameReadFile, Content: fmt.Sprintf("read_file: %v", err), IsError: true}
	}
	data := buf[:n]

	// Binary / non-text: do not dump into model context.
	if bytesContainNUL(data) || !utf8.Valid(data) {
		return Result{
			Name: NameReadFile,
			Content: fmt.Sprintf(
				"read_file: binary or non-UTF-8 file %q (size %d bytes). Not loaded into chat. Prefer text sources, or use shell for binary inspection.",
				path, total,
			),
		}
	}

	body := string(data)
	if truncated {
		next := offset + n
		body += fmt.Sprintf(
			"\n…[truncated: bytes %d–%d of %d; re-call read_file with offset=%d max_bytes=%d for next chunk]",
			offset, offset+n, total, next, max,
		)
	}
	return Result{Name: NameReadFile, Content: body}
}

func bytesContainNUL(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func (r *Registry) execListDir(call Call) Result {
	if r.gw == nil {
		return Result{Name: NameListDir, Content: "list_dir: workspace not configured", IsError: true}
	}
	var args listDirArgs
	if len(strings.TrimSpace(call.Arguments)) > 0 && call.Arguments != "{}" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return Result{Name: NameListDir, Content: fmt.Sprintf("list_dir: invalid arguments: %v", err), IsError: true}
		}
	}
	rel := strings.TrimSpace(args.Path)
	if rel == "" {
		rel = "."
	}
	abs, err := r.resolveWorkspacePath(rel)
	if err != nil {
		return Result{Name: NameListDir, Content: err.Error(), IsError: true}
	}
	return r.listDirAt(rel, abs)
}

// listDirAt formats directory entries. Used by list_dir and by read_file when path is a dir.
func (r *Registry) listDirAt(rel, abs string) Result {
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Result{Name: NameListDir, Content: fmt.Sprintf("list_dir: %v", err), IsError: true}
	}
	var b strings.Builder
	// Clarify auto-redirect when the model called read_file on a folder.
	fmt.Fprintf(&b, "directory %q — listing entries (read_file auto-used list_dir):\n", rel)
	for _, e := range entries {
		name := e.Name()
		if name == ".git" || name == "node_modules" {
			continue
		}
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		fmt.Fprintf(&b, "%s%s\n", name, suffix)
	}
	out := strings.TrimSpace(b.String())
	if out == "" || strings.HasSuffix(out, ":") {
		out = fmt.Sprintf("directory %q is empty", rel)
	}
	return Result{Name: NameListDir, Content: out}
}

func (r *Registry) execGrep(call Call) Result {
	if r.gw == nil {
		return Result{Name: NameGrep, Content: "grep: workspace not configured", IsError: true}
	}
	var args grepArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameGrep, Content: fmt.Sprintf("grep: invalid arguments: %v", err), IsError: true}
	}
	pattern := args.Pattern
	if pattern == "" {
		return Result{Name: NameGrep, Content: "grep: pattern is required", IsError: true}
	}
	max := args.MaxMatches
	if max <= 0 {
		max = 50
	}
	if max > 200 {
		max = 200
	}

	startRel := strings.TrimSpace(args.Path)
	if startRel == "" {
		startRel = "."
	}
	startAbs, err := r.resolveWorkspacePath(startRel)
	if err != nil {
		return Result{Name: NameGrep, Content: err.Error(), IsError: true}
	}

	var matcher func(line string) bool
	if args.Regex {
		flags := ""
		if args.CaseInsensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + pattern)
		if err != nil {
			return Result{Name: NameGrep, Content: fmt.Sprintf("grep: bad regex: %v", err), IsError: true}
		}
		matcher = re.MatchString
	} else {
		needle := pattern
		if args.CaseInsensitive {
			needle = strings.ToLower(pattern)
		}
		matcher = func(line string) bool {
			hay := line
			if args.CaseInsensitive {
				hay = strings.ToLower(line)
			}
			return strings.Contains(hay, needle)
		}
	}

	root := r.gw.RootPath()
	var hits []string
	walkErr := filepath.WalkDir(startAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == "dist" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip likely binary/noise by extension.
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf", ".zip",
			".exe", ".dll", ".so", ".dylib", ".wasm", ".bin", ".o", ".a":
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1<<20 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !utf8.Valid(data) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if matcher(line) {
				snip := strings.TrimSpace(line)
				if len(snip) > 200 {
					snip = snip[:200] + "…"
				}
				hits = append(hits, fmt.Sprintf("%s:%d: %s", rel, i+1, snip))
				if len(hits) >= max {
					return errGrepDone
				}
			}
		}
		return nil
	})
	if walkErr != nil && walkErr != errGrepDone {
		return Result{Name: NameGrep, Content: fmt.Sprintf("grep: %v", walkErr), IsError: true}
	}
	if len(hits) == 0 {
		return Result{Name: NameGrep, Content: "no matches"}
	}
	return Result{Name: NameGrep, Content: strings.Join(hits, "\n")}
}

var errGrepDone = fmt.Errorf("grep done")

// resolveWorkspacePath maps a relative (or absolute-under-root) path into the
// sandbox and rejects escapes.
func (r *Registry) resolveWorkspacePath(relOrAbs string) (string, error) {
	if r == nil || r.gw == nil {
		return "", fmt.Errorf("workspace not configured")
	}
	root := r.gw.RootPath()
	p := strings.TrimSpace(relOrAbs)
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	var abs string
	if filepath.IsAbs(p) {
		abs = filepath.Clean(p)
	} else {
		abs = filepath.Clean(filepath.Join(root, p))
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("workspace root: %w", err)
	}
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	// Ensure abs is under root (with separator edge).
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("sandbox denied: path %q is outside the workspace", p)
	}
	return abs, nil
}
