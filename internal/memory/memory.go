// Package memory implements Multi Brain helpers: detect .multibrain/, read
// session.md as the master index first, then open only 1–2 matching buckets
// and pointed context files. No RAG / vector store wiring (VAL-MEM-010).
//
// Read order (AGENTS.md + docs/memory-rag.md):
//  1. .multibrain/session.md (master index)
//  2. 1–2 bucket files under .multibrain/indexes/*.md
//  3. .multibrain/context/* only when a bucket entry points to them
//  4. Never open/dump the entire .multibrain/ tree
package memory

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Directory names and files under the workspace Multi Brain tree.
const (
	DirName      = ".multibrain"
	SessionFile  = "session.md"
	IndexesDir   = "indexes"
	ContextDir   = "context"
	MaxBuckets   = 2 // hard bound: open at most 1–2 buckets per startup/load
	MaxContexts  = 8 // safety bound for pointed context files in one load
)

// Status describes Multi Brain presence and layout health for a workspace.
type Status struct {
	// Present is true when .multibrain/ exists as a directory.
	Present bool
	// Message is a short human-readable status (CLI / startup log).
	Message string
	// SessionPath is the absolute path to session.md when present.
	SessionPath string
	// SessionExists is true when session.md is readable.
	SessionExists bool
	// IndexCount is the number of *.md bucket files under indexes/.
	IndexCount int
	// ContextCount is the number of files under context/ (informational only; not all opened).
	ContextCount int
	// Buckets are names parsed from session.md (bucket id strings), when available.
	Buckets []BucketRef
}

// BucketRef is one named bucket line from session.md.
type BucketRef struct {
	// Name is the short bucket id (e.g. "architecture").
	Name string
	// Path is the relative path from workspace root (e.g. ".multibrain/indexes/architecture.md").
	Path string
	// Description is the short scope text from the session line (may be empty).
	Description string
}

// Trace records which Multi Brain files were opened and in what order.
// Used for verbose startup verification (VAL-MEM-002).
type Trace struct {
	// Opened is absolute paths in open order (session.md first when present).
	Opened []string
}

// LoadResult is the outcome of a bounded Multi Brain read.
type LoadResult struct {
	Status Status
	// Session is the raw content of session.md (empty if missing).
	Session string
	// Buckets maps bucket name → content for the selected (bounded) buckets only.
	Buckets map[string]string
	// Contexts maps relative context path → content for pointed files only.
	Contexts map[string]string
	// Trace records open order and paths.
	Trace Trace
}

// Detect reports whether the workspace has a Multi Brain tree without reading content.
// When .multibrain/ is absent: Present=false, Message mentions "no multi-brain" (VAL-MEM-001).
// When present: Present=true and Message indicates detection; session/indexes may still be incomplete.
func Detect(workspaceRoot string) (Status, error) {
	root, err := absRoot(workspaceRoot)
	if err != nil {
		return Status{}, err
	}
	mb := filepath.Join(root, DirName)
	info, err := os.Stat(mb)
	if err != nil {
		if os.IsNotExist(err) {
			return Status{
				Present: false,
				Message: "no multi-brain (.multibrain/ not found)",
			}, nil
		}
		return Status{}, fmt.Errorf("memory: stat %s: %w", mb, err)
	}
	if !info.IsDir() {
		return Status{
			Present: false,
			Message: "no multi-brain (.multibrain exists but is not a directory)",
		}, nil
	}

	st := Status{
		Present:     true,
		Message:     "multi-brain detected (.multibrain/)",
		SessionPath: filepath.Join(mb, SessionFile),
	}
	if si, err := os.Stat(st.SessionPath); err == nil && !si.IsDir() {
		st.SessionExists = true
	}
	idxDir := filepath.Join(mb, IndexesDir)
	if entries, err := os.ReadDir(idxDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
				st.IndexCount++
			}
		}
	}
	ctxDir := filepath.Join(mb, ContextDir)
	if entries, err := os.ReadDir(ctxDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				st.ContextCount++
			}
		}
	}
	// Best-effort parse of bucket names from session.md without full Load.
	if st.SessionExists {
		if data, err := os.ReadFile(st.SessionPath); err == nil {
			st.Buckets = ParseSessionBuckets(string(data))
		}
	}
	return st, nil
}

// Load performs a bounded Multi Brain read for the workspace:
//   - Always opens session.md first when present (VAL-MEM-002).
//   - Opens at most MaxBuckets (default 2) matching buckets for query/hints.
//   - Opens context files only when selected bucket entries point to them (capped).
//   - Never walks/opens the entire tree.
//
// query is optional free-text used to rank buckets (substring match on name/description).
// When empty, the first MaxBuckets listed in session.md are selected (stable order).
func Load(workspaceRoot string, query string) (LoadResult, error) {
	root, err := absRoot(workspaceRoot)
	if err != nil {
		return LoadResult{}, err
	}
	st, err := Detect(root)
	if err != nil {
		return LoadResult{}, err
	}
	res := LoadResult{
		Status:   st,
		Buckets:  make(map[string]string),
		Contexts: make(map[string]string),
	}
	if !st.Present {
		return res, nil
	}

	// 1) session.md first
	sessionPath := filepath.Join(root, DirName, SessionFile)
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			res.Status.Message = "multi-brain detected but session.md missing"
			res.Status.SessionExists = false
			return res, nil
		}
		return LoadResult{}, fmt.Errorf("memory: read session.md: %w", err)
	}
	res.Trace.Opened = append(res.Trace.Opened, sessionPath)
	res.Session = string(sessionData)
	res.Status.SessionExists = true
	res.Status.Buckets = ParseSessionBuckets(res.Session)

	// 2) select 1–2 buckets
	selected := SelectBuckets(res.Status.Buckets, query, MaxBuckets)
	for _, b := range selected {
		path := resolveBucketPath(root, b)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			// Missing bucket file: skip, do not fail the whole load.
			continue
		}
		res.Trace.Opened = append(res.Trace.Opened, path)
		res.Buckets[b.Name] = string(data)
	}

	// 3) pointed context files only (from selected bucket content).
	// Prefer newest pointers first (buckets are newest-first); cap to keep
	// startup reads bounded (not the whole context/ tree).
	// Per selected bucket: open at most 2 pointed contexts (MaxBuckets*2 default).
	const perBucketContexts = 2
	ctxOpened := 0
	for _, content := range res.Buckets {
		bucketCtx := 0
		for _, rel := range ContextPointers(content) {
			if ctxOpened >= MaxContexts || bucketCtx >= perBucketContexts {
				break
			}
			abs := filepath.Join(root, filepath.FromSlash(rel))
			// Ensure under .multibrain/context
			if !isUnder(abs, filepath.Join(root, DirName, ContextDir)) {
				continue
			}
			// Dedup if already opened.
			already := false
			for _, o := range res.Trace.Opened {
				if o == abs {
					already = true
					break
				}
			}
			if already {
				continue
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			res.Trace.Opened = append(res.Trace.Opened, abs)
			res.Contexts[rel] = string(data)
			ctxOpened++
			bucketCtx++
		}
	}

	res.Status.Message = fmt.Sprintf(
		"multi-brain loaded (session.md + %d bucket(s), %d context file(s))",
		len(res.Buckets), len(res.Contexts),
	)
	return res, nil
}

// WriteTrace writes a verbose human-readable open order for CLI --verbose.
func WriteTrace(w io.Writer, tr Trace) {
	if w == nil {
		return
	}
	for i, p := range tr.Opened {
		fmt.Fprintf(w, "[memory] opened[%d]: %s\n", i, p)
	}
}

// FormatStatusLine returns the short startup / memory status line (VAL-MEM-001).
func FormatStatusLine(st Status) string {
	if !st.Present {
		return "memory: no multi-brain"
	}
	if !st.SessionExists {
		return "memory: multi-brain detected (session.md missing)"
	}
	return fmt.Sprintf("memory: multi-brain detected (%d bucket(s) in session.md)", len(st.Buckets))
}

// bucketLine matches session.md bucket lines like:
//   - `architecture` — product architecture… Last updated: … -> .multibrain/indexes/architecture.md
var bucketLine = regexp.MustCompile(
	`(?m)^-\s*` + "`" + `([^` + "`" + `]+)` + "`" +
		`(?:\s*[—\-–:]\s*(.*?))?\s*(?:->\s*(\S+))?\s*$`,
)

// ParseSessionBuckets extracts bucket references from session.md content.
func ParseSessionBuckets(session string) []BucketRef {
	var out []BucketRef
	for _, m := range bucketLine.FindAllStringSubmatch(session, -1) {
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(m[2])
		// Drop trailing "Last updated: …" noise from description for ranking.
		if i := strings.Index(strings.ToLower(desc), "last updated"); i >= 0 {
			desc = strings.TrimSpace(desc[:i])
			desc = strings.TrimRight(desc, " .—-–")
		}
		path := strings.TrimSpace(m[3])
		if path == "" {
			path = filepath.ToSlash(filepath.Join(DirName, IndexesDir, name+".md"))
		}
		out = append(out, BucketRef{Name: name, Path: path, Description: desc})
	}
	return out
}

// SelectBuckets picks up to limit buckets matching query (name/description).
// Empty query selects the first limit buckets in session order.
func SelectBuckets(all []BucketRef, query string, limit int) []BucketRef {
	if limit <= 0 {
		limit = MaxBuckets
	}
	if len(all) == 0 {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		if len(all) > limit {
			return all[:limit]
		}
		return all
	}
	// Score: name hit > description hit; preserve session order among ties.
	type scored struct {
		b BucketRef
		s int
	}
	var hits []scored
	for _, b := range all {
		score := 0
		name := strings.ToLower(b.Name)
		desc := strings.ToLower(b.Description)
		if name == q {
			score = 100
		} else if strings.Contains(name, q) {
			score = 80
		} else if strings.Contains(desc, q) {
			score = 40
		} else {
			// token match
			for _, tok := range strings.Fields(q) {
				if tok == "" {
					continue
				}
				if strings.Contains(name, tok) {
					score += 30
				} else if strings.Contains(desc, tok) {
					score += 10
				}
			}
		}
		if score > 0 {
			hits = append(hits, scored{b: b, s: score})
		}
	}
	// Simple stable selection: higher score first, then original order.
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].s > hits[i].s {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	if len(hits) == 0 {
		// No match: fall back to first limit (still bounded).
		if len(all) > limit {
			return all[:limit]
		}
		return all
	}
	out := make([]BucketRef, 0, limit)
	for _, h := range hits {
		if len(out) >= limit {
			break
		}
		out = append(out, h.b)
	}
	return out
}

// contextPtr matches "-> .multibrain/context/<file>.md" style pointers in bucket entries.
var contextPtr = regexp.MustCompile(`->\s*(\.multibrain/context/[^\s)]+\.md)`)

// ContextPointers returns relative .multibrain/context/*.md paths referenced in bucket content.
func ContextPointers(bucketContent string) []string {
	ms := contextPtr.FindAllStringSubmatch(bucketContent, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, m := range ms {
		rel := strings.TrimSpace(m[1])
		rel = filepath.ToSlash(rel)
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	return out
}

func absRoot(workspaceRoot string) (string, error) {
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("memory: resolve workspace: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("memory: workspace %q: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("memory: workspace %q is not a directory", root)
	}
	return root, nil
}

func resolveBucketPath(root string, b BucketRef) string {
	rel := strings.TrimSpace(b.Path)
	if rel == "" {
		rel = filepath.ToSlash(filepath.Join(DirName, IndexesDir, b.Name+".md"))
	}
	rel = strings.TrimPrefix(rel, "./")
	abs := filepath.Join(root, filepath.FromSlash(rel))
	// Safety: must stay under .multibrain/indexes
	if !isUnder(abs, filepath.Join(root, DirName, IndexesDir)) &&
		!isUnder(abs, filepath.Join(root, DirName)) {
		return ""
	}
	return abs
}

func isUnder(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
