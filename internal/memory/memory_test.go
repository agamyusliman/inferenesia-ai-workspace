package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectAbsentNoCrash(t *testing.T) {
	root := t.TempDir()
	st, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if st.Present {
		t.Fatal("expected Present=false")
	}
	if !strings.Contains(strings.ToLower(st.Message), "no multi-brain") {
		t.Fatalf("want 'no multi-brain' message, got %q", st.Message)
	}
	line := FormatStatusLine(st)
	if !strings.Contains(strings.ToLower(line), "no multi-brain") {
		t.Fatalf("status line: %q", line)
	}
}

func TestDetectPresent(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	st, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !st.Present {
		t.Fatal("expected Present=true")
	}
	if !st.SessionExists {
		t.Fatal("expected session.md")
	}
	if st.IndexCount < 2 {
		t.Fatalf("IndexCount=%d want >=2", st.IndexCount)
	}
	line := FormatStatusLine(st)
	if !strings.Contains(strings.ToLower(line), "multi-brain detected") {
		t.Fatalf("status line: %q", line)
	}
	if len(st.Buckets) < 2 {
		t.Fatalf("buckets parsed=%d", len(st.Buckets))
	}
}

// VAL-MEM-002: session.md is opened first; not all tree files opened.
func TestLoadSessionFirstBounded(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)

	// Poison tree: many index + context files that must NOT all open.
	for i := 0; i < 20; i++ {
		name := filepath.Join(root, DirName, IndexesDir, "extra-"+itoa(i)+".md")
		if err := os.WriteFile(name, []byte("- noise entry -> .multibrain/context/noise.md\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 30; i++ {
		p := filepath.Join(root, DirName, ContextDir, "noise-"+itoa(i)+".md")
		if err := os.WriteFile(p, []byte("should not all open"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Load(root, "architecture")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !res.Status.Present {
		t.Fatal("not present")
	}
	if res.Session == "" {
		t.Fatal("session content empty")
	}
	if len(res.Trace.Opened) == 0 {
		t.Fatal("trace empty")
	}
	first := res.Trace.Opened[0]
	if !strings.HasSuffix(filepath.ToSlash(first), "/.multibrain/session.md") &&
		filepath.Base(first) != SessionFile {
		t.Fatalf("first open must be session.md, got %q", first)
	}
	// Bounded: at most 1 session + MaxBuckets + MaxContexts
	if len(res.Trace.Opened) > 1+MaxBuckets+MaxContexts {
		t.Fatalf("opened too many files: %d (%v)", len(res.Trace.Opened), res.Trace.Opened)
	}
	// Must not open all 30 noise context files
	if len(res.Trace.Opened) >= 20 {
		t.Fatalf("opened almost entire tree: %d files", len(res.Trace.Opened))
	}
	// Selected bucket for architecture should be among opened paths
	archOpened := false
	for _, p := range res.Trace.Opened {
		if strings.Contains(p, "architecture.md") {
			archOpened = true
		}
	}
	if !archOpened {
		t.Fatalf("architecture bucket not opened; opened=%v", res.Trace.Opened)
	}
	if _, ok := res.Buckets["architecture"]; !ok {
		t.Fatalf("architecture content missing; keys=%v", keys(res.Buckets))
	}
	// Pointed context from architecture should open; noise-* should not.
	for _, p := range res.Trace.Opened {
		base := filepath.Base(p)
		if strings.HasPrefix(base, "noise-") {
			t.Fatalf("opened unpointed noise context: %s", p)
		}
	}
}

func TestLoadAbsentReturnsNoMultiBrain(t *testing.T) {
	root := t.TempDir()
	res, err := Load(root, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.Status.Present {
		t.Fatal("present")
	}
	if len(res.Trace.Opened) != 0 {
		t.Fatalf("opened files without multibrain: %v", res.Trace.Opened)
	}
}

// VAL-MEM-010: no RAG/vector wiring in this package (behavioral offline load).
func TestLoadWorksOfflineNoVectorDeps(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, true)
	// No env, no network — pure filesystem.
	res, err := Load(root, "agents")
	if err != nil {
		t.Fatalf("offline Load: %v", err)
	}
	if !res.Status.Present {
		t.Fatal("expected present")
	}
	if res.Session == "" {
		t.Fatal("session empty")
	}
}

func TestParseSessionBucketsAndPointers(t *testing.T) {
	session := `# Multi Brain Master Index

## Buckets

- ` + "`agents`" + ` — agent workflows. Last updated: 2026-07-17 16:05 WIB -> .multibrain/indexes/agents.md
- ` + "`architecture`" + ` — monorepo layout. Last updated: 2026-07-17 20:05 WIB -> .multibrain/indexes/architecture.md
`
	buckets := ParseSessionBuckets(session)
	if len(buckets) != 2 {
		t.Fatalf("got %d buckets: %+v", len(buckets), buckets)
	}
	if buckets[0].Name != "agents" || buckets[1].Name != "architecture" {
		t.Fatalf("names: %+v", buckets)
	}
	if !strings.Contains(buckets[0].Path, "agents.md") {
		t.Fatalf("path: %q", buckets[0].Path)
	}

	bucketBody := `- 2026-07-17 19:05 WIB — Droid: note -> .multibrain/context/2026-07-17-1905-droid-note.md
- older -> .multibrain/context/older.md
`
	ptrs := ContextPointers(bucketBody)
	if len(ptrs) != 2 {
		t.Fatalf("ptrs=%v", ptrs)
	}
	if ptrs[0] != ".multibrain/context/2026-07-17-1905-droid-note.md" {
		t.Fatalf("ptr0=%q", ptrs[0])
	}
}

func TestSelectBucketsLimitAndQuery(t *testing.T) {
	all := []BucketRef{
		{Name: "agents", Description: "workflows"},
		{Name: "architecture", Description: "monorepo core.Service"},
		{Name: "providers", Description: "temp-ai gateway"},
		{Name: "git-setup", Description: "repo bootstrap"},
	}
	sel := SelectBuckets(all, "", 2)
	if len(sel) != 2 || sel[0].Name != "agents" {
		t.Fatalf("default select: %+v", sel)
	}
	sel = SelectBuckets(all, "architecture", 2)
	if len(sel) == 0 || sel[0].Name != "architecture" {
		t.Fatalf("query select: %+v", sel)
	}
	sel = SelectBuckets(all, "gateway byok", MaxBuckets)
	if len(sel) == 0 || sel[0].Name != "providers" {
		t.Fatalf("token select: %+v", sel)
	}
}

func writeFixture(t *testing.T, root string, withCtx bool) {
	t.Helper()
	mb := filepath.Join(root, DirName)
	if err := os.MkdirAll(filepath.Join(mb, IndexesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mb, ContextDir), 0o755); err != nil {
		t.Fatal(err)
	}
	session := `# Multi Brain Master Index

## Buckets

- ` + "`agents`" + ` — shared notes for agent workflows. Last updated: 2026-07-17 16:05 WIB -> .multibrain/indexes/agents.md
- ` + "`architecture`" + ` — product architecture, monorepo layout. Last updated: 2026-07-17 20:05 WIB -> .multibrain/indexes/architecture.md
- ` + "`providers`" + ` — temp-ai gateway + BYOK. Last updated: 2026-07-17 18:45 WIB -> .multibrain/indexes/providers.md
`
	if err := os.WriteFile(filepath.Join(mb, SessionFile), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}
	agents := `# agents

- 2026-07-17 16:02 WIB — Droid: multi brain init -> .multibrain/context/agents-init.md
`
	arch := `# architecture

- 2026-07-17 19:05 WIB — Droid: WriteGateway -> .multibrain/context/arch-writegate.md
`
	prov := `# providers

- entry without context pointer
`
	if err := os.WriteFile(filepath.Join(mb, IndexesDir, "agents.md"), []byte(agents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mb, IndexesDir, "architecture.md"), []byte(arch), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mb, IndexesDir, "providers.md"), []byte(prov), 0o600); err != nil {
		t.Fatal(err)
	}
	if withCtx {
		if err := os.WriteFile(filepath.Join(mb, ContextDir, "agents-init.md"), []byte("init note"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mb, ContextDir, "arch-writegate.md"), []byte("writegate note"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
