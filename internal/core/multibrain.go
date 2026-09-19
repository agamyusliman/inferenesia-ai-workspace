package core

import (
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/memory"
)

// MultiBrainSessionMaxBytes caps the total Multi Brain context appended to the
// system prompt so the agent loop never dumps the entire .multibrain/ tree
// (VAL-MEM-002 / VAL-ORCH-005 bounded read invariant).
const (
	multiBrainSessionMax = 4000
	multiBrainBucketMax  = 3000
	multiBrainContextMax = 2000
)

// multiBrainSystemSnippet loads the workspace Multi Brain master index and a
// bounded set of bucket/context files, then returns a system-prompt section
// that makes the bucket context available to the agent (VAL-CROSS-002).
//
// Behavior:
//   - When .multibrain/session.md exists: returns a section containing the
//     read-order rules + session.md content + ≤2 selected buckets + pointed
//     context files only (never the whole tree).
//   - When .multibrain/ is absent or session.md missing: returns "" and the
//     caller proceeds without error. No RAG / Qdrant fallback is attempted
//     (memory package is file-only; no vector deps — VAL-MEM-010).
//
// query (usually the user prompt) ranks which buckets are selected so the
// agent sees context relevant to the current turn.
func (s *Service) multiBrainSystemSnippet(workspaceRoot, query string) string {
	if strings.TrimSpace(workspaceRoot) == "" {
		return ""
	}
	res, err := memory.Load(workspaceRoot, query)
	if err != nil || !res.Status.Present {
		// Detection failure is non-fatal for chat (rare: workspace stat issues).
		// Crucially, we never fall back to a vector store: memory.Load is file-only.
		return ""
	}
	return formatMultiBrainSnippet(res)
}

// formatMultiBrainSnippet builds the system-prompt addition from a bounded
// Multi Brain LoadResult. Pure function for testability (no fs access).
func formatMultiBrainSnippet(res memory.LoadResult) string {
	if !res.Status.Present {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Multi Brain memory (this workspace)\n")
	b.WriteString("Project memory lives under `.multibrain/`. Rules:\n")
	b.WriteString("1. Treat `session.md` as the master index (already loaded below).\n")
	b.WriteString("2. Only 1–2 bucket files are loaded; do not request the entire `.multibrain/` tree.\n")
	b.WriteString("3. Open additional context files only when a bucket entry points to them.\n")
	b.WriteString("4. Memory is file-based Multi Brain only — no RAG/vector store.\n\n")
	if res.Session != "" {
		b.WriteString("### .multibrain/session.md\n")
		b.WriteString(truncateMBRunes(res.Session, multiBrainSessionMax))
		b.WriteString("\n\n")
	}
	for name, content := range res.Buckets {
		b.WriteString("### .multibrain/indexes/")
		b.WriteString(name)
		b.WriteString(".md\n")
		b.WriteString(truncateMBRunes(content, multiBrainBucketMax))
		b.WriteString("\n\n")
	}
	for rel, content := range res.Contexts {
		b.WriteString("### ")
		b.WriteString(rel)
		b.WriteString("\n")
		b.WriteString(truncateMBRunes(content, multiBrainContextMax))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

// truncateMBRunes hard-caps a string for the system-prompt section.
func truncateMBRunes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max > 3 {
		return s[:max-3] + "..."
	}
	return s[:max]
}
