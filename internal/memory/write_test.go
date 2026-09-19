package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-MEM-003: after meaningful work, append newest-first to best-matching bucket;
// prior entries remain intact; documented format.
func TestAppendEntryNewestFirstIntact(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	g := mustGateway(t, root)

	beforeArch := mustRead(t, filepath.Join(root, DirName, IndexesDir, "architecture.md"))
	beforeAgents := mustRead(t, filepath.Join(root, DirName, IndexesDir, "agents.md"))
	beforeProv := mustRead(t, filepath.Join(root, DirName, IndexesDir, "providers.md"))

	rec := Record{
		Agent:   "Droid",
		Summary: "WriteGateway multibrain append path",
		Topic:   "multibrain-write",
		Query:   "architecture monorepo WriteGateway",
		Files:   []string{"internal/memory/write.go"},
		When:    time.Date(2026, 7, 17, 21, 5, 0, 0, fixedWIB()),
	}
	res, err := Append(g, root, rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if res.BucketName != "architecture" {
		t.Fatalf("best bucket=%q want architecture", res.BucketName)
	}

	afterArch := mustRead(t, filepath.Join(root, DirName, IndexesDir, "architecture.md"))
	if afterArch == beforeArch {
		t.Fatal("architecture bucket unchanged")
	}
	// Newest entry first under ## Entries
	entryLine := findFirstEntryLine(afterArch)
	if entryLine == "" {
		t.Fatalf("no entry line in:\n%s", afterArch)
	}
	if !strings.Contains(entryLine, "2026-07-17 21:05 WIB") {
		t.Fatalf("timestamp format wrong: %q", entryLine)
	}
	if !strings.Contains(entryLine, "Droid:") {
		t.Fatalf("agent missing: %q", entryLine)
	}
	if !strings.Contains(entryLine, "WriteGateway multibrain append path") {
		t.Fatalf("summary missing: %q", entryLine)
	}
	// Prior content intact (old first entry still present).
	if !strings.Contains(afterArch, "WriteGateway -> .multibrain/context/arch-writegate.md") {
		t.Fatal("prior architecture entry missing")
	}
	// Prior body (header/rules) preserved.
	if !strings.Contains(afterArch, "# architecture") && !strings.Contains(afterArch, "Named Sub-Index") {
		// fixture uses "# architecture"
		if !strings.HasPrefix(strings.TrimSpace(afterArch), "# architecture") {
			// tolerate renamed header
			if !strings.Contains(afterArch, "## Entries") {
				t.Fatalf("bucket structure broken:\n%s", afterArch)
			}
		}
	}

	// VAL-MEM-011: other buckets unchanged.
	if mustRead(t, filepath.Join(root, DirName, IndexesDir, "agents.md")) != beforeAgents {
		t.Fatal("agents bucket changed on architecture-scoped turn")
	}
	if mustRead(t, filepath.Join(root, DirName, IndexesDir, "providers.md")) != beforeProv {
		t.Fatal("providers bucket changed on architecture-scoped turn")
	}
	// Hash check for other buckets
	if hash(beforeAgents) != hash(mustRead(t, filepath.Join(root, DirName, IndexesDir, "agents.md"))) {
		t.Fatal("agents sha changed")
	}
	if hash(beforeProv) != hash(mustRead(t, filepath.Join(root, DirName, IndexesDir, "providers.md"))) {
		t.Fatal("providers sha changed")
	}

	_ = res
}

// VAL-MEM-004: context file written for decision/blocker/files/verification;
// bucket entry points to it.
func TestContextFileWhenDecisionOrFiles(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	g := mustGateway(t, root)

	rec := Record{
		Agent:      "Droid",
		Summary:    "chose WriteGateway for multibrain writes",
		Topic:      "multibrain-context",
		Query:      "architecture",
		Decision:   "All Multi Brain writes go through WriteGateway WriteMultibrain.",
		Files:      []string{"internal/memory/write.go", "internal/writegate/undo.go"},
		Verification: "go test ./internal/memory/...",
		When:       time.Date(2026, 7, 17, 21, 10, 0, 0, fixedWIB()),
	}
	res, err := Append(g, root, rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if res.ContextPath == "" {
		t.Fatal("expected ContextPath set")
	}
	// Path shape: .multibrain/context/YYYY-MM-DD-HHMM-<agent>-<topic>.md
	base := filepath.Base(res.ContextPath)
	if !strings.HasPrefix(base, "2026-07-17-2110-droid-") {
		t.Fatalf("context filename shape: %q", base)
	}
	if !strings.HasSuffix(base, ".md") {
		t.Fatalf("context must be .md: %q", base)
	}
	abs := filepath.Join(root, filepath.FromSlash(res.ContextPath))
	body, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read context: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("context empty")
	}
	if !strings.Contains(string(body), "WriteGateway") {
		t.Fatalf("context missing decision: %s", body)
	}
	if !strings.Contains(string(body), "internal/memory/write.go") {
		t.Fatalf("context missing files: %s", body)
	}

	// Bucket entry pointer matches.
	arch := mustRead(t, filepath.Join(root, DirName, IndexesDir, "architecture.md"))
	entry := findFirstEntryLine(arch)
	if !strings.Contains(entry, "-> "+res.ContextPath) && !strings.Contains(entry, res.ContextPath) {
		t.Fatalf("bucket entry does not point to context:\nentry=%q\nctx=%q", entry, res.ContextPath)
	}
}

// Trivial turns (no decision/blocker/files/verification) may skip context.
func TestTrivialTurnSkipsContext(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	g := mustGateway(t, root)

	rec := Record{
		Agent:   "Droid",
		Summary: "noted a short observation",
		Topic:   "note",
		Query:   "agents",
		When:    time.Date(2026, 7, 17, 21, 12, 0, 0, fixedWIB()),
	}
	res, err := Append(g, root, rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if res.ContextPath != "" {
		t.Fatalf("trivial turn should skip context, got %q", res.ContextPath)
	}
	// Entry still appended without pointer.
	agents := mustRead(t, filepath.Join(root, DirName, IndexesDir, "agents.md"))
	entry := findFirstEntryLine(agents)
	if strings.Contains(entry, "-> .multibrain/context/") {
		t.Fatalf("trivial entry should not point to context: %q", entry)
	}
	if !strings.Contains(entry, "noted a short observation") {
		t.Fatalf("entry missing summary: %q", entry)
	}
}

// VAL-MEM-005: session.md Last updated refreshed for touched bucket; new bucket added if needed.
func TestSessionLastUpdatedRefreshed(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	g := mustGateway(t, root)

	sessionBefore := mustRead(t, filepath.Join(root, DirName, SessionFile))
	if !strings.Contains(sessionBefore, "Last updated: 2026-07-17 20:05 WIB") {
		// fixture architecture line uses 20:05
		if !strings.Contains(sessionBefore, "`architecture`") {
			t.Fatal("fixture missing architecture bucket line")
		}
	}

	rec := Record{
		Agent:   "Droid",
		Summary: "refresh session timestamps",
		Query:   "architecture",
		When:    time.Date(2026, 7, 17, 21, 20, 0, 0, fixedWIB()),
	}
	_, err := Append(g, root, rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	sessionAfter := mustRead(t, filepath.Join(root, DirName, SessionFile))
	if sessionAfter == sessionBefore {
		t.Fatal("session.md unchanged")
	}
	// architecture line should carry the new timestamp
	archLine := ""
	for _, line := range strings.Split(sessionAfter, "\n") {
		if strings.Contains(line, "`architecture`") {
			archLine = line
			break
		}
	}
	if archLine == "" {
		t.Fatal("architecture line gone from session.md")
	}
	if !strings.Contains(archLine, "Last updated: 2026-07-17 21:20 WIB") {
		t.Fatalf("Last updated not refreshed: %q", archLine)
	}
	// other bucket lines still present
	if !strings.Contains(sessionAfter, "`agents`") || !strings.Contains(sessionAfter, "`providers`") {
		t.Fatal("other session buckets lost")
	}
}

// New area introduces a new bucket + session line.
func TestNewBucketAddedToSession(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	g := mustGateway(t, root)

	rec := Record{
		Agent:       "Droid",
		Summary:     "opened browser adapter work",
		Topic:       "browser-engines",
		Query:       "browser",
		Bucket:      "browser", // force new area
		Description: "browser engines and CDP adapters",
		Files:       []string{"internal/browser/manager.go"},
		When:        time.Date(2026, 7, 17, 21, 30, 0, 0, fixedWIB()),
	}
	res, err := Append(g, root, rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if res.BucketName != "browser" {
		t.Fatalf("bucket=%q", res.BucketName)
	}
	bucketPath := filepath.Join(root, DirName, IndexesDir, "browser.md")
	if _, err := os.Stat(bucketPath); err != nil {
		t.Fatalf("new bucket file missing: %v", err)
	}
	session := mustRead(t, filepath.Join(root, DirName, SessionFile))
	if !strings.Contains(session, "`browser`") {
		t.Fatalf("session.md missing new bucket:\n%s", session)
	}
	if !strings.Contains(session, "Last updated: 2026-07-17 21:30 WIB") {
		t.Fatalf("new bucket Last updated missing:\n%s", session)
	}
}

// VAL-MEM-006: after a turn that "used" the gateway key, memory files have no secrets.
func TestNoSecretsInMultibrainWrites(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	g := mustGateway(t, root)

	const secret = "temp-abcdefghijklmnopqrstuvwxyz012345"
	t.Setenv("TEMP_AI_API_KEY", secret)

	rec := Record{
		Agent:    "Droid",
		Summary:  "used gateway with TEMP_AI_API_KEY=" + secret + " and Bearer " + secret,
		Topic:    "secret-hygiene",
		Query:    "providers",
		Decision: "never store TEMP_AI_API_KEY=" + secret + " in memory; sk-live-abcdefghijklmnop too",
		Blocker:  "got Authorization: Bearer " + secret + " from env",
		Files:    []string{".env"},
		When:     time.Date(2026, 7, 17, 21, 40, 0, 0, fixedWIB()),
	}
	res, err := Append(g, root, rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Scan all .multibrain/** content written/touched
	var leaked []string
	_ = filepath.Walk(filepath.Join(root, DirName), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		s := string(data)
		if strings.Contains(s, secret) {
			leaked = append(leaked, path+": secret value present")
		}
		if strings.Contains(s, "sk-live-abcdefghijklmnop") {
			leaked = append(leaked, path+": sk-live token present")
		}
		if strings.Contains(s, "TEMP_AI_API_KEY="+secret) {
			leaked = append(leaked, path+": TEMP_AI_API_KEY=value present")
		}
		return nil
	})
	if len(leaked) > 0 {
		t.Fatalf("secrets leaked into multibrain:\n%s", strings.Join(leaked, "\n"))
	}
	// Bucket/context still have redacted content (not empty summary).
	prov := mustRead(t, filepath.Join(root, DirName, IndexesDir, "providers.md"))
	if !strings.Contains(prov, "used gateway") && !strings.Contains(prov, "[REDACTED]") {
		t.Fatalf("providers entry missing after scrub:\n%s", prov)
	}
	if res.ContextPath == "" {
		t.Fatal("expected context for decision/blocker/files")
	}
	ctx := mustRead(t, filepath.Join(root, filepath.FromSlash(res.ContextPath)))
	if strings.Contains(ctx, secret) {
		t.Fatalf("context still has secret: %s", ctx)
	}
}

// Appends must go through WriteGateway (source=multibrain on stack).
func TestAppendRoutesThroughWriteGateway(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	g := mustGateway(t, root)

	rec := Record{
		Agent:   "Droid",
		Summary: "gateway routing check",
		Query:   "agents",
		Files:   []string{"internal/memory/write.go"},
		When:    time.Date(2026, 7, 17, 21, 50, 0, 0, fixedWIB()),
	}
	_, err := Append(g, root, rec)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if g.UndoDepth() == 0 {
		t.Fatal("expected WriteGateway undo units for multibrain writes")
	}
	units := g.UndoStack()
	foundMB := false
	for _, u := range units {
		for _, e := range u.Entries {
			if e.Source == writegate.SourceMultibrain {
				foundMB = true
			}
		}
	}
	if !foundMB {
		t.Fatalf("no multibrain-sourced entries on undo stack: %+v", units)
	}
}

// BestMatchBucket picks single best bucket (not dump-all).
func TestBestMatchBucketSingle(t *testing.T) {
	all := []BucketRef{
		{Name: "agents", Description: "workflows"},
		{Name: "architecture", Description: "monorepo core.Service layout"},
		{Name: "providers", Description: "temp-ai gateway"},
	}
	b := BestMatchBucket(all, "architecture WriteGateway monorepo", "")
	if b.Name != "architecture" {
		t.Fatalf("got %q", b.Name)
	}
	b = BestMatchBucket(all, "gateway models", "")
	if b.Name != "providers" {
		t.Fatalf("got %q", b.Name)
	}
	// Explicit override wins.
	b = BestMatchBucket(all, "architecture", "providers")
	if b.Name != "providers" {
		t.Fatalf("override want providers got %q", b.Name)
	}
}

func mustGateway(t *testing.T, root string) *writegate.Gateway {
	t.Helper()
	g, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func findFirstEntryLine(content string) string {
	lines := strings.Split(content, "\n")
	inEntries := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## Entries") {
			inEntries = true
			continue
		}
		if inEntries && strings.HasPrefix(trim, "- ") {
			return trim
		}
		// Also accept oldest fixture style with entry right under header
		if !inEntries && strings.HasPrefix(trim, "- ") && strings.Contains(trim, "WIB") {
			return trim
		}
	}
	// Fallback: first line that looks like an entry with WIB
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ") && strings.Contains(trim, "WIB") {
			return trim
		}
	}
	return ""
}

// fixedWIB returns a fixed timezone with offset +7 (WIB) for deterministic tests.
func fixedWIB() *time.Location {
	return time.FixedZone("WIB", 7*60*60)
}
