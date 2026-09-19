package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// Soft cap compaction (VAL-MEM-009): when a bucket exceeds SoftCapEntries raw
// entry bullets, older entries are summarized into a context file and replaced
// by a single summary + pointer line so the bucket stays near the soft cap.

// CompactionResult describes what CompactBucket did.
type CompactionResult struct {
	// Compacted is true when older entries were summarized.
	Compacted bool
	// BeforeCount is the raw entry count before compaction.
	BeforeCount int
	// AfterCount is the raw entry count after (including the new summary line).
	AfterCount int
	// SummaryPath is the relative context path holding the compacted older entries.
	SummaryPath string
	// Content is the full bucket markdown after compaction (or original if skipped).
	Content string
}

// ListEntryLines returns entry bullet lines from a bucket (newest-first order as stored).
// Only lines that look like work-log entries under ## Entries (or whole-file bullets
// for fixtures without a header) are counted — not random list items in rules sections.
func ListEntryLines(bucketContent string) []string {
	content := strings.ReplaceAll(bucketContent, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	entriesIdx := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.EqualFold(trim, "## Entries") || strings.HasPrefix(strings.ToLower(trim), "## entries") {
			entriesIdx = i
			break
		}
	}

	var out []string
	if entriesIdx >= 0 {
		// Collect "- " bullets until next ## heading.
		for i := entriesIdx + 1; i < len(lines); i++ {
			trim := strings.TrimSpace(lines[i])
			if strings.HasPrefix(trim, "## ") {
				break
			}
			if strings.HasPrefix(trim, "- ") {
				out = append(out, trim)
			}
		}
		return out
	}

	// No ## Entries: treat consecutive "- " lines after the first as the log
	// (same as PrependEntry fixture style — first bullet and following).
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ") {
			out = append(out, trim)
		}
	}
	return out
}

// CountEntries returns the number of raw work-log entry bullets in a bucket.
func CountEntries(bucketContent string) int {
	return len(ListEntryLines(bucketContent))
}

// CompactBucketIfNeeded prepends nothing; it rewrites bucketContent so that if
// the entry count exceeds SoftCapEntries, the oldest overflow is moved into a
// context summary and replaced by one pointer line.
//
// keepNewest is SoftCapEntries-1 so that after adding a summary line the bucket
// holds SoftCapEntries raw lines (newest SoftCapEntries-1 + 1 summary pointer).
// If already within the soft cap, Content is returned unchanged and Compacted=false.
//
// The caller must write SummaryBody via WriteGateway when Compacted && SummaryPath set.
type CompactPlan struct {
	CompactionResult
	// SummaryBody is the markdown to write at SummaryPath (empty when not compacted).
	SummaryBody string
}

// PlanCompact computes the compacted bucket text and optional summary file.
// Does not touch disk.
func PlanCompact(bucketContent, bucketName string, when time.Time) CompactPlan {
	entries := ListEntryLines(bucketContent)
	n := len(entries)
	plan := CompactPlan{}
	plan.BeforeCount = n
	plan.Content = bucketContent
	plan.AfterCount = n
	if n <= SoftCapEntries {
		return plan
	}

	// Keep the newest (SoftCapEntries - 1) entries; compact the rest (oldest tail).
	// Entries list is newest-first.
	keepN := SoftCapEntries - 1
	if keepN < 1 {
		keepN = SoftCapEntries
	}
	keep := entries[:keepN]
	older := entries[keepN:] // older run, still newest-first among those

	when = when.In(wibLocation())
	summaryRel := ContextFileName(when, "compact", bucketName+"-older-entries")
	// Build summary context body.
	var sb strings.Builder
	sb.WriteString("# Compacted older entries: ")
	sb.WriteString(bucketName)
	sb.WriteString("\n\n")
	sb.WriteString("Soft-cap compaction (~")
	sb.WriteString(fmt.Sprintf("%d", SoftCapEntries))
	sb.WriteString(" entries/bucket). Older raw entries were summarized here.\n\n")
	sb.WriteString("## Entries (older, newest-first among compacted)\n\n")
	for _, e := range older {
		sb.WriteString(e)
		sb.WriteByte('\n')
	}
	summaryBody := secrethyg.Redact(sb.String())

	// Summary pointer line (counts as 1 raw entry).
	summaryLine := FormatEntryLine(when, "compact",
		fmt.Sprintf("summarized %d older entries (soft cap ~%d)", len(older), SoftCapEntries),
		summaryRel)
	summaryLine = secrethyg.Redact(summaryLine)

	// Rebuild bucket: header/preamble + keep (newest) + summary pointer at end of
	// the entry list (oldest position among remaining raw lines). Newest-first:
	// keep stays first, summary pointer is the oldest kept "entry".
	newContent := rewriteBucketEntries(bucketContent, append(keep, summaryLine))
	newContent = secrethyg.Redact(newContent)

	plan.Compacted = true
	plan.Content = newContent
	plan.SummaryPath = summaryRel
	plan.SummaryBody = summaryBody
	plan.AfterCount = CountEntries(newContent)
	return plan
}

// CompactBucket applies PlanCompact and writes the summary context via WriteGateway
// when compaction occurs. Returns CompactionResult; caller should then write
// result.Content as the bucket body (Append does this in one write).
func CompactBucket(gw *writegate.Gateway, bucketName, bucketContent string, when time.Time) (CompactionResult, error) {
	plan := PlanCompact(bucketContent, bucketName, when)
	if !plan.Compacted {
		return plan.CompactionResult, nil
	}
	if gw != nil && plan.SummaryPath != "" && plan.SummaryBody != "" {
		if _, err := gw.WriteMultibrain(plan.SummaryPath, []byte(plan.SummaryBody)); err != nil {
			return CompactionResult{}, fmt.Errorf("memory: write compaction summary: %w", err)
		}
	}
	return plan.CompactionResult, nil
}

// rewriteBucketEntries replaces work-log entry bullets with newEntries (newest-first),
// preserving the header / rules / ## Entries structure.
func rewriteBucketEntries(bucketContent string, newEntries []string) string {
	content := strings.ReplaceAll(bucketContent, "\r\n", "\n")
	if content == "" {
		content = defaultBucketTemplate("bucket")
	}
	lines := strings.Split(content, "\n")

	entriesIdx := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.EqualFold(trim, "## Entries") || strings.HasPrefix(strings.ToLower(trim), "## entries") {
			entriesIdx = i
			break
		}
	}

	// Normalize entry lines to start with "- ".
	norm := make([]string, 0, len(newEntries))
	for _, e := range newEntries {
		e = strings.TrimRight(e, "\n")
		if !strings.HasPrefix(strings.TrimSpace(e), "- ") {
			e = "- " + strings.TrimSpace(e)
		} else {
			e = strings.TrimSpace(e)
		}
		norm = append(norm, e)
	}

	if entriesIdx < 0 {
		// Fixture style without ## Entries: keep non-bullet prefix, replace all bullets.
		var prefix []string
		firstBullet := -1
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "- ") {
				if firstBullet < 0 {
					firstBullet = i
				}
			} else if firstBullet < 0 {
				prefix = append(prefix, line)
			}
		}
		// Drop trailing empty from prefix
		for len(prefix) > 0 && strings.TrimSpace(prefix[len(prefix)-1]) == "" {
			prefix = prefix[:len(prefix)-1]
		}
		out := append([]string{}, prefix...)
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, norm...)
		out = append(out, "")
		joined := strings.Join(out, "\n")
		if !strings.HasSuffix(joined, "\n") {
			joined += "\n"
		}
		return joined
	}

	// Keep everything up to and including ## Entries + following blank lines.
	// Drop old entry bullets and any trailing blanks in the entries section until next ##.
	endIdx := len(lines)
	for i := entriesIdx + 1; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "## ") {
			endIdx = i
			break
		}
	}
	// Preserve a single blank after ## Entries header if present.
	headEnd := entriesIdx + 1
	if headEnd < endIdx && strings.TrimSpace(lines[headEnd]) == "" {
		headEnd++
	}

	out := make([]string, 0, len(lines)+len(norm))
	out = append(out, lines[:headEnd]...)
	out = append(out, norm...)
	// blank before next section if any
	if endIdx < len(lines) {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, lines[endIdx:]...)
	} else {
		out = append(out, "")
	}
	joined := strings.Join(out, "\n")
	if !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	return joined
}

