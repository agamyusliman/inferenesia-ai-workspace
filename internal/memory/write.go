package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// Default timezone label for Multi Brain timestamps (Asia/Jakarta / WIB).
const DefaultTZName = "WIB"

// SoftCapEntries is the soft max raw entries per bucket (VAL-MEM-009).
// When exceeded after prepend, older entries are compacted to a context pointer.
const SoftCapEntries = 25

// Record describes one Multi Brain write after meaningful work.
// All text fields are scrubbed of secrets before any disk write (VAL-MEM-006).
type Record struct {
	// Agent is the agent display name (e.g. "Droid").
	Agent string
	// Summary is a short one-line work summary for the bucket entry.
	Summary string
	// Topic is used in the optional context filename (slug).
	Topic string
	// Query ranks existing buckets (name/description) for best-match selection.
	Query string
	// Bucket, when set, forces that bucket name (creates file + session line if new).
	Bucket string
	// Description is the short scope text for a newly created bucket in session.md.
	Description string
	// Decision, when non-empty, triggers a context file (VAL-MEM-004).
	Decision string
	// Blocker, when non-empty, triggers a context file.
	Blocker string
	// Files is an important file list; non-empty triggers a context file.
	Files []string
	// Verification is evidence text; non-empty triggers a context file.
	Verification string
	// Body is optional extra free-form context body (also scrubbed).
	Body string
	// When is the entry timestamp; zero uses time.Now in DefaultTZName (+7).
	When time.Time
}

// AppendResult is the outcome of a Multi Brain append.
type AppendResult struct {
	// BucketName is the single bucket that was updated (VAL-MEM-011).
	BucketName string
	// BucketPath is the relative path of the bucket file from workspace root.
	BucketPath string
	// EntryLine is the one-line bucket entry that was prepended (newest-first).
	EntryLine string
	// ContextPath is the relative context file path when written; empty if skipped.
	ContextPath string
	// SessionUpdated is true when session.md was refreshed (or a bucket line added).
	SessionUpdated bool
	// CreatedBucket is true when a new indexes/<bucket>.md was created.
	CreatedBucket bool
	// Compacted is true when soft-cap compaction ran (VAL-MEM-009).
	Compacted bool
	// CompactSummaryPath is the context file holding older compacted entries.
	CompactSummaryPath string
	// EntryCount is the raw entry count in the bucket after append (+ compaction).
	EntryCount int
}

// Append writes one newest-first entry to the best-matching Multi Brain bucket,
// optionally writes a context file, and refreshes session.md Last updated.
//
// All file mutations go through WriteGateway WriteMultibrain (VAL-UNDO-007).
// Content is scrubbed of secrets before write (VAL-MEM-006).
// Only one bucket is touched (VAL-MEM-011); prior bucket entries stay intact (VAL-MEM-003).
func Append(gw *writegate.Gateway, workspaceRoot string, rec Record) (AppendResult, error) {
	if gw == nil {
		return AppendResult{}, fmt.Errorf("memory: write gateway is required")
	}
	root, err := absRoot(workspaceRoot)
	if err != nil {
		return AppendResult{}, err
	}
	if gw.RootPath() != "" && filepath.Clean(gw.RootPath()) != filepath.Clean(root) {
		// Gateway root must match workspace; still allow if caller resolved differently
		// as long as paths stay sandboxed via gateway Resolve.
		root = filepath.Clean(gw.RootPath())
	}

	rec = scrubRecord(rec)
	when := rec.When
	if when.IsZero() {
		when = time.Now().In(wibLocation())
	} else {
		when = when.In(wibLocation())
	}
	agent := strings.TrimSpace(rec.Agent)
	if agent == "" {
		agent = "Agent"
	}
	summary := strings.TrimSpace(rec.Summary)
	if summary == "" {
		summary = "meaningful work"
	}
	// Collapse entry summary to a single line (no newlines).
	summary = collapseOneLine(summary)

	// Ensure .multibrain layout exists (indexes + context dirs).
	if err := ensureLayout(gw, root); err != nil {
		return AppendResult{}, err
	}

	// Detect / parse session buckets.
	sessionRel := filepath.ToSlash(filepath.Join(DirName, SessionFile))
	sessionAbs := filepath.Join(root, DirName, SessionFile)
	sessionBody, err := os.ReadFile(sessionAbs)
	if err != nil && !os.IsNotExist(err) {
		return AppendResult{}, fmt.Errorf("memory: read session.md: %w", err)
	}
	sessionStr := string(sessionBody)
	buckets := ParseSessionBuckets(sessionStr)

	// Best single bucket (not dump-all).
	var chosen BucketRef
	var created bool
	if name := sanitizeBucketName(rec.Bucket); name != "" {
		if existing, ok := findBucket(buckets, name); ok {
			chosen = existing
		} else {
			desc := strings.TrimSpace(rec.Description)
			if desc == "" {
				desc = name
			}
			chosen = BucketRef{
				Name:        name,
				Path:        filepath.ToSlash(filepath.Join(DirName, IndexesDir, name+".md")),
				Description: desc,
			}
			created = true
		}
	} else {
		chosen = BestMatchBucket(buckets, rec.Query, "")
		if chosen.Name == "" {
			// No session buckets at all: create "agents" default area.
			chosen = BucketRef{
				Name:        "agents",
				Path:        filepath.ToSlash(filepath.Join(DirName, IndexesDir, "agents.md")),
				Description: "shared notes for agent workflows",
			}
			created = true
		}
	}

	// Context file when decision / blocker / files / verification present.
	var contextRel string
	if NeedsContext(rec) {
		topic := rec.Topic
		if topic == "" {
			topic = summary
		}
		contextRel = ContextFileName(when, agent, topic)
		ctxBody := FormatContext(rec, when, agent)
		ctxBody = secrethyg.Redact(ctxBody)
		if _, err := gw.WriteMultibrain(contextRel, []byte(ctxBody)); err != nil {
			return AppendResult{}, fmt.Errorf("memory: write context: %w", err)
		}
	}

	// Format entry line (newest-first).
	entryLine := FormatEntryLine(when, agent, summary, contextRel)
	entryLine = secrethyg.Redact(entryLine)

	// Load or seed bucket content, prepend entry under ## Entries.
	bucketRel := chosen.Path
	if bucketRel == "" {
		bucketRel = filepath.ToSlash(filepath.Join(DirName, IndexesDir, chosen.Name+".md"))
	}
	bucketAbs := filepath.Join(root, filepath.FromSlash(bucketRel))
	var bucketContent string
	if data, err := os.ReadFile(bucketAbs); err == nil {
		bucketContent = string(data)
	} else if os.IsNotExist(err) {
		bucketContent = defaultBucketTemplate(chosen.Name)
		created = true
	} else {
		return AppendResult{}, fmt.Errorf("memory: read bucket %s: %w", bucketRel, err)
	}
	newBucket := PrependEntry(bucketContent, entryLine)
	newBucket = secrethyg.Redact(newBucket)

	// Soft cap ~25 entries: compact older run to a context/memory pointer (VAL-MEM-009).
	compacted := false
	compactSummary := ""
	if CountEntries(newBucket) > SoftCapEntries {
		plan, err := CompactBucket(gw, chosen.Name, newBucket, when)
		if err != nil {
			return AppendResult{}, err
		}
		if plan.Compacted {
			newBucket = plan.Content
			compacted = true
			compactSummary = plan.SummaryPath
		}
	}

	if _, err := gw.WriteMultibrain(bucketRel, []byte(newBucket)); err != nil {
		return AppendResult{}, fmt.Errorf("memory: write bucket: %w", err)
	}

	// Refresh session.md Last updated (and add new bucket line if needed).
	newSession := sessionStr
	if newSession == "" {
		newSession = defaultSessionTemplate()
	}
	newSession, sessionChanged := RefreshSessionBucket(newSession, chosen, when)
	newSession = secrethyg.Redact(newSession)
	sessionUpdated := false
	if sessionChanged || newSession != sessionStr {
		if _, err := gw.WriteMultibrain(sessionRel, []byte(newSession)); err != nil {
			return AppendResult{}, fmt.Errorf("memory: write session.md: %w", err)
		}
		sessionUpdated = true
	}

	return AppendResult{
		BucketName:         chosen.Name,
		BucketPath:         bucketRel,
		EntryLine:          entryLine,
		ContextPath:        contextRel,
		SessionUpdated:     sessionUpdated,
		CreatedBucket:      created,
		Compacted:          compacted,
		CompactSummaryPath: compactSummary,
		EntryCount:         CountEntries(newBucket),
	}, nil
}

// BestMatchBucket selects a single best-matching bucket for a turn (VAL-MEM-011).
// forcedName, when non-empty, returns that bucket if present, else a ref with only Name set.
// Empty all + empty forcedName returns a zero BucketRef (caller may invent a default).
func BestMatchBucket(all []BucketRef, query, forcedName string) BucketRef {
	if name := sanitizeBucketName(forcedName); name != "" {
		if b, ok := findBucket(all, name); ok {
			return b
		}
		return BucketRef{
			Name: name,
			Path: filepath.ToSlash(filepath.Join(DirName, IndexesDir, name+".md")),
		}
	}
	if len(all) == 0 {
		return BucketRef{}
	}
	// Reuse ranking from SelectBuckets with limit 1.
	sel := SelectBuckets(all, query, 1)
	if len(sel) == 0 {
		return all[0]
	}
	return sel[0]
}

// NeedsContext reports whether a Record should produce a context file (VAL-MEM-004).
func NeedsContext(rec Record) bool {
	if strings.TrimSpace(rec.Decision) != "" {
		return true
	}
	if strings.TrimSpace(rec.Blocker) != "" {
		return true
	}
	if strings.TrimSpace(rec.Verification) != "" {
		return true
	}
	if strings.TrimSpace(rec.Body) != "" {
		return true
	}
	for _, f := range rec.Files {
		if strings.TrimSpace(f) != "" {
			return true
		}
	}
	return false
}

// FormatEntryLine builds one bucket entry:
//
//	timestamp — agent: summary[ -> .multibrain/context/...]
func FormatEntryLine(when time.Time, agent, summary, contextRel string) string {
	ts := FormatTimestamp(when)
	line := fmt.Sprintf("- %s — %s: %s", ts, strings.TrimSpace(agent), collapseOneLine(summary))
	contextRel = strings.TrimSpace(contextRel)
	if contextRel != "" {
		contextRel = filepath.ToSlash(contextRel)
		line += " -> " + contextRel
	}
	return line
}

// FormatTimestamp renders "2006-01-02 15:04 WIB" style stamps.
func FormatTimestamp(when time.Time) string {
	when = when.In(wibLocation())
	return when.Format("2006-01-02 15:04") + " " + DefaultTZName
}

// ContextFileName builds ".multibrain/context/YYYY-MM-DD-HHMM-<agent>-<topic>.md".
func ContextFileName(when time.Time, agent, topic string) string {
	when = when.In(wibLocation())
	stamp := when.Format("2006-01-02-1504")
	a := slugPart(agent)
	if a == "" {
		a = "agent"
	}
	t := slugPart(topic)
	if t == "" {
		t = "note"
	}
	// Cap topic slug length for portable filenames.
	if len(t) > 48 {
		t = t[:48]
		t = strings.Trim(t, "-")
	}
	return filepath.ToSlash(filepath.Join(DirName, ContextDir, stamp+"-"+a+"-"+t+".md"))
}

// FormatContext builds the markdown body for a context file (no secrets expected; still scrubbed by caller).
func FormatContext(rec Record, when time.Time, agent string) string {
	var b strings.Builder
	b.WriteString("# ")
	title := strings.TrimSpace(rec.Topic)
	if title == "" {
		title = collapseOneLine(rec.Summary)
	}
	if title == "" {
		title = "context"
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString("- **When:** ")
	b.WriteString(FormatTimestamp(when))
	b.WriteString("\n")
	b.WriteString("- **Agent:** ")
	b.WriteString(agent)
	b.WriteString("\n")
	if s := strings.TrimSpace(rec.Summary); s != "" {
		b.WriteString("- **Summary:** ")
		b.WriteString(collapseOneLine(s))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if d := strings.TrimSpace(rec.Decision); d != "" {
		b.WriteString("## Decision\n\n")
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	if bl := strings.TrimSpace(rec.Blocker); bl != "" {
		b.WriteString("## Blocker\n\n")
		b.WriteString(bl)
		b.WriteString("\n\n")
	}
	files := filterNonEmpty(rec.Files)
	if len(files) > 0 {
		b.WriteString("## Files\n\n")
		for _, f := range files {
			b.WriteString("- `")
			b.WriteString(f)
			b.WriteString("`\n")
		}
		b.WriteString("\n")
	}
	if v := strings.TrimSpace(rec.Verification); v != "" {
		b.WriteString("## Verification\n\n")
		b.WriteString(v)
		b.WriteString("\n\n")
	}
	if body := strings.TrimSpace(rec.Body); body != "" {
		b.WriteString("## Notes\n\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

// PrependEntry inserts entryLine as the newest entry under ## Entries (or creates that section).
// Prior entries remain intact (VAL-MEM-003). Newest-first: the new line becomes the first
// entry bullet after the Entries header / before any existing entry bullets.
func PrependEntry(bucketContent, entryLine string) string {
	entryLine = strings.TrimRight(entryLine, "\n")
	if !strings.HasPrefix(strings.TrimSpace(entryLine), "- ") {
		entryLine = "- " + strings.TrimSpace(entryLine)
	}
	content := bucketContent
	if content == "" {
		content = defaultBucketTemplate("bucket")
	}
	// Normalize line endings for processing.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	// Find ## Entries section header.
	entriesIdx := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.EqualFold(trim, "## Entries") || strings.HasPrefix(strings.ToLower(trim), "## entries") {
			entriesIdx = i
			break
		}
	}
	if entriesIdx < 0 {
		// No ## Entries header: treat first existing "- " entry bullet as the work log
		// (fixture style: title + blank + entries). Insert before that first bullet.
		firstEntry := -1
		for i, line := range lines {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "- ") {
				firstEntry = i
				break
			}
		}
		if firstEntry >= 0 {
			out := make([]string, 0, len(lines)+1)
			out = append(out, lines[:firstEntry]...)
			out = append(out, entryLine)
			out = append(out, lines[firstEntry:]...)
			joined := strings.Join(out, "\n")
			if !strings.HasSuffix(joined, "\n") {
				joined += "\n"
			}
			return joined
		}
		// No bullets either: append ## Entries section.
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, "", "## Entries", "", entryLine, "")
		return strings.Join(lines, "\n")
	}

	// Insert after ## Entries and any blank lines, before the first existing entry
	// (or before next ## heading).
	insertAt := entriesIdx + 1
	// Skip blank lines after the header (keep formatting neat).
	for insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) == "" {
		insertAt++
	}
	// Build new slice: before insert + entry + rest
	out := make([]string, 0, len(lines)+2)
	out = append(out, lines[:insertAt]...)
	out = append(out, entryLine)
	out = append(out, lines[insertAt:]...)
	// Ensure trailing newline when original had one / for POSIX friendliness.
	joined := strings.Join(out, "\n")
	if !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	return joined
}

// RefreshSessionBucket updates Last updated for the given bucket, or appends a new
// bucket line under ## Buckets when the name is new (VAL-MEM-005).
// Returns the new session content and whether it changed.
func RefreshSessionBucket(session string, bucket BucketRef, when time.Time) (string, bool) {
	if bucket.Name == "" {
		return session, false
	}
	when = when.In(wibLocation())
	ts := FormatTimestamp(when)
	path := bucket.Path
	if path == "" {
		path = filepath.ToSlash(filepath.Join(DirName, IndexesDir, bucket.Name+".md"))
	}
	path = filepath.ToSlash(path)
	desc := strings.TrimSpace(bucket.Description)
	if desc == "" {
		desc = bucket.Name
	}

	content := strings.ReplaceAll(session, "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		content = defaultSessionTemplate()
	}
	lines := strings.Split(content, "\n")
	// Match a session bucket line that already names this bucket: - `name` ...
	reName := regexp.MustCompile(`^(\s*-\s*)` + "`" + regexp.QuoteMeta(bucket.Name) + "`" + `(.*)$`)
	found := false
	for i, line := range lines {
		if m := reName.FindStringSubmatch(line); m != nil {
			// Rebuild the line preserving leading dash form; refresh Last updated + pointer.
			// Shape: - `name` — <desc>. Last updated: <ts> -> <path>
			// Try to keep existing description if present.
			rest := m[2]
			// Drop existing Last updated… and pointer for rebuild.
			descPart := rest
			if idx := strings.Index(strings.ToLower(descPart), "last updated"); idx >= 0 {
				descPart = descPart[:idx]
			}
			if idx := strings.Index(descPart, "->"); idx >= 0 {
				descPart = descPart[:idx]
			}
			descPart = strings.TrimSpace(descPart)
			descPart = strings.Trim(descPart, "—-–:.")
			descPart = strings.TrimSpace(descPart)
			// descPart may start with em-dash leftover from " — desc"
			for strings.HasPrefix(descPart, "—") || strings.HasPrefix(descPart, "-") || strings.HasPrefix(descPart, "–") {
				descPart = strings.TrimSpace(descPart[len([]byte(descPart[:1])):]) // not ideal for multi-byte
				// Safer: trim runes
				break
			}
			descPart = trimLeadingDashDesc(descPart)
			if descPart == "" {
				descPart = desc
			}
			newLine := fmt.Sprintf("- `%s` — %s. Last updated: %s -> %s",
				bucket.Name, ensureNoTrailingPeriod(descPart), ts, path)
			// Keep original indentation if any (m[1] already has "- ").
			// Prefer the rebuilt line with standard leading "- ".
			lines[i] = newLine
			found = true
			break
		}
	}
	if !found {
		// Insert under ## Buckets section (or append).
		newLine := fmt.Sprintf("- `%s` — %s. Last updated: %s -> %s",
			bucket.Name, ensureNoTrailingPeriod(desc), ts, path)
		lines = insertSessionBucketLine(lines, newLine)
	}
	joined := strings.Join(lines, "\n")
	if !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	return joined, joined != session
}

func insertSessionBucketLine(lines []string, newLine string) []string {
	bucketsIdx := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.EqualFold(trim, "## Buckets") || strings.HasPrefix(strings.ToLower(trim), "## buckets") {
			bucketsIdx = i
			break
		}
	}
	if bucketsIdx < 0 {
		// Append section.
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		return append(lines, "", "## Buckets", "", newLine, "")
	}
	// Find end of current bucket list (next ## or EOF).
	insertAt := len(lines)
	for i := bucketsIdx + 1; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "## ") {
			insertAt = i
			break
		}
	}
	// Prefer inserting after last "- `" line within the section.
	lastBullet := -1
	for i := bucketsIdx + 1; i < insertAt; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "- ") {
			lastBullet = i
		}
	}
	if lastBullet >= 0 {
		insertAt = lastBullet + 1
	} else {
		// after header + optional blank
		insertAt = bucketsIdx + 1
		if insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) == "" {
			insertAt++
		}
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:insertAt]...)
	out = append(out, newLine)
	out = append(out, lines[insertAt:]...)
	return out
}

// --- helpers ---

func scrubRecord(rec Record) Record {
	rec.Agent = secrethyg.Redact(rec.Agent)
	rec.Summary = secrethyg.Redact(rec.Summary)
	rec.Topic = secrethyg.Redact(rec.Topic)
	rec.Query = secrethyg.Redact(rec.Query)
	rec.Bucket = secrethyg.Redact(rec.Bucket)
	rec.Description = secrethyg.Redact(rec.Description)
	rec.Decision = secrethyg.Redact(rec.Decision)
	rec.Blocker = secrethyg.Redact(rec.Blocker)
	rec.Verification = secrethyg.Redact(rec.Verification)
	rec.Body = secrethyg.Redact(rec.Body)
	if len(rec.Files) > 0 {
		out := make([]string, len(rec.Files))
		for i, f := range rec.Files {
			// Do not put .env contents; path names are OK, but scrub key-like fragments.
			out[i] = secrethyg.Redact(f)
		}
		rec.Files = out
	}
	return rec
}

func ensureLayout(gw *writegate.Gateway, root string) error {
	// Create dirs with ordinary MkdirAll (directories are not content mutations).
	// Session/bucket files are created via WriteGateway when missing.
	for _, sub := range []string{
		filepath.Join(root, DirName),
		filepath.Join(root, DirName, IndexesDir),
		filepath.Join(root, DirName, ContextDir),
	} {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return fmt.Errorf("memory: mkdir %s: %w", sub, err)
		}
	}
	// If session.md missing, seed a skeleton via gateway.
	sessionAbs := filepath.Join(root, DirName, SessionFile)
	if _, err := os.Stat(sessionAbs); os.IsNotExist(err) {
		body := secrethyg.Redact(defaultSessionTemplate())
		if _, err := gw.WriteMultibrain(filepath.ToSlash(filepath.Join(DirName, SessionFile)), []byte(body)); err != nil {
			return err
		}
	}
	return nil
}

func defaultSessionTemplate() string {
	return `# Multi Brain Master Index

Use this file as the master index that points to sub-index files.

Rules:
- Keep this file short and easy to scan
- Use one line per named bucket
- Update ` + "`Last updated`" + ` when the related bucket receives a new entry
- Store the main work log in ` + "`.multibrain/indexes/*.md`" + `, not here

## Buckets

`
}

func defaultBucketTemplate(name string) string {
	name = sanitizeBucketName(name)
	if name == "" {
		name = "bucket"
	}
	return fmt.Sprintf(`# Named Sub-Index: `+"`%s`"+`

Use this file as a concise work log.

Rules:
- Put the newest entry at the top
- Keep each entry to one line
- Store longer detail in a pointed file when needed

## Entries

`, name)
}

func findBucket(all []BucketRef, name string) (BucketRef, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, b := range all {
		if strings.ToLower(b.Name) == name {
			return b, true
		}
	}
	return BucketRef{}, false
}

func sanitizeBucketName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	// Collapse repeated dashes.
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

func slugPart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func collapseOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func filterNonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func ensureNoTrailingPeriod(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimRight(s, ". ")
}

func trimLeadingDashDesc(s string) string {
	s = strings.TrimSpace(s)
	// Trim common " — desc" prefixes left after stripping Last updated.
	for {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "—") {
			s = strings.TrimSpace(strings.TrimPrefix(s, "—"))
			continue
		}
		if strings.HasPrefix(s, "–") {
			s = strings.TrimSpace(strings.TrimPrefix(s, "–"))
			continue
		}
		if strings.HasPrefix(s, "-") {
			s = strings.TrimSpace(strings.TrimPrefix(s, "-"))
			continue
		}
		if strings.HasPrefix(s, ":") {
			s = strings.TrimSpace(strings.TrimPrefix(s, ":"))
			continue
		}
		break
	}
	return s
}

func wibLocation() *time.Location {
	// Prefer named Asia/Jakarta when available; else fixed +7.
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone(DefaultTZName, 7*60*60)
}
