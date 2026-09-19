package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// VAL-MEM-009: after forcing >25 appends, bucket does not exceed ~25 raw entries;
// a summary + pointer replaces the older run.
func TestSoftCapCompactionOnAppend(t *testing.T) {
	root := t.TempDir()
	// Minimal multibrain layout with one empty bucket.
	if err := os.MkdirAll(filepath.Join(root, DirName, IndexesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, DirName, ContextDir), 0o755); err != nil {
		t.Fatal(err)
	}
	session := `# Multi Brain Master Index

## Buckets

- ` + "`agents`" + ` — test. Last updated: 2026-07-17 10:00 WIB -> .multibrain/indexes/agents.md
`
	if err := os.WriteFile(filepath.Join(root, DirName, SessionFile), []byte(session), 0o644); err != nil {
		t.Fatal(err)
	}
	bucketBody := defaultBucketTemplate("agents")
	if err := os.WriteFile(filepath.Join(root, DirName, IndexesDir, "agents.md"), []byte(bucketBody), 0o644); err != nil {
		t.Fatal(err)
	}

	g := mustGateway(t, root)
	base := time.Date(2026, 7, 17, 12, 0, 0, 0, fixedWIB())

	// Force SoftCapEntries+5 appends so we exceed the soft cap.
	n := SoftCapEntries + 5
	for i := 0; i < n; i++ {
		rec := Record{
			Agent:   "Droid",
			Summary: fmt.Sprintf("work item %03d", i),
			Bucket:  "agents",
			When:    base.Add(time.Duration(i) * time.Minute),
		}
		// No context trigger for trivial turns (faster, pure entry cap).
		res, err := Append(g, root, rec)
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if res.BucketName != "agents" {
			t.Fatalf("bucket=%q", res.BucketName)
		}
	}

	final := mustRead(t, filepath.Join(root, DirName, IndexesDir, "agents.md"))
	count := CountEntries(final)
	if count > SoftCapEntries {
		t.Fatalf("entry count after over-cap = %d (soft cap %d)\n%s", count, SoftCapEntries, final)
	}
	if count < SoftCapEntries-1 {
		// After compaction we expect near soft cap (keep SoftCapEntries-1 + summary).
		t.Fatalf("entry count too low: %d", count)
	}
	// Summary + pointer present.
	if !strings.Contains(final, "summarized") && !strings.Contains(final, "soft cap") {
		t.Fatalf("expected compact summary pointer in bucket:\n%s", final)
	}
	if !strings.Contains(final, "-> .multibrain/context/") {
		t.Fatalf("expected pointer to compacted context:\n%s", final)
	}
	// Compacted context file exists with older entries.
	entries := ListEntryLines(final)
	var summaryPtr string
	for _, e := range entries {
		if strings.Contains(e, "summarized") {
			summaryPtr = e
			break
		}
	}
	if summaryPtr == "" {
		t.Fatal("no summary entry line")
	}
	// Extract path after "-> "
	idx := strings.Index(summaryPtr, "-> ")
	if idx < 0 {
		t.Fatalf("no pointer in %q", summaryPtr)
	}
	rel := strings.TrimSpace(summaryPtr[idx+3:])
	sumPath := filepath.Join(root, filepath.FromSlash(rel))
	body, err := os.ReadFile(sumPath)
	if err != nil {
		t.Fatalf("compact summary missing %s: %v", rel, err)
	}
	if !strings.Contains(string(body), "work item") {
		t.Fatalf("summary should contain older entries:\n%s", body)
	}
	// Newest work still at top of bucket.
	first := findFirstEntryLine(final)
	if !strings.Contains(first, fmt.Sprintf("work item %03d", n-1)) {
		t.Fatalf("newest entry missing at top: %q", first)
	}
}

func TestPlanCompactNoopUnderCap(t *testing.T) {
	body := defaultBucketTemplate("x")
	for i := 0; i < 3; i++ {
		body = PrependEntry(body, FormatEntryLine(time.Now(), "A", fmt.Sprintf("e%d", i), ""))
	}
	plan := PlanCompact(body, "x", time.Now())
	if plan.Compacted {
		t.Fatal("should not compact under soft cap")
	}
	if plan.BeforeCount != 3 {
		t.Fatalf("before=%d", plan.BeforeCount)
	}
}

func TestCountEntriesIgnoresRulesBullets(t *testing.T) {
	body := `# Named Sub-Index: agents

Rules:
- Put the newest entry at the top
- Keep each entry to one line

## Entries

- 2026-07-17 12:00 WIB — Droid: real entry
- 2026-07-17 11:00 WIB — Droid: older
`
	if got := CountEntries(body); got != 2 {
		t.Fatalf("CountEntries=%d want 2 (rules bullets ignored)", got)
	}
}
