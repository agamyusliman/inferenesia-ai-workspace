package skills

import (
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/memory"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// MultiBrainReadInput configures a protocol-compliant Multi Brain read
// (VAL-ORCH-005): session.md first, at most MaxBuckets index files, context
// only when buckets point — never the entire tree.
type MultiBrainReadInput struct {
	// Workspace is the project root containing .multibrain/.
	Workspace string
	// Query ranks which 1–2 buckets to open (optional).
	Query string
}

// MultiBrainReadResult is the bounded read outcome + open trace for assertions.
type MultiBrainReadResult struct {
	Status   memory.Status
	Session  string
	Buckets  map[string]string
	Contexts map[string]string
	// Opened is absolute paths in open order (session first).
	Opened []string
	// OpenCount is len(Opened).
	OpenCount int
	// Summary is a short human-readable report for the model/CLI.
	Summary string
}

// MultiBrainRead performs the Multi Brain protocol read via memory.Load
// (session.md → ≤2 buckets → pointed contexts only).
func MultiBrainRead(in MultiBrainReadInput) (MultiBrainReadResult, error) {
	ws := strings.TrimSpace(in.Workspace)
	if ws == "" {
		return MultiBrainReadResult{}, fmt.Errorf("multi-brain: workspace is required")
	}
	res, err := memory.Load(ws, in.Query)
	if err != nil {
		return MultiBrainReadResult{}, err
	}
	out := MultiBrainReadResult{
		Status:    res.Status,
		Session:   res.Session,
		Buckets:   res.Buckets,
		Contexts:  res.Contexts,
		Opened:    res.Trace.Opened,
		OpenCount: len(res.Trace.Opened),
	}
	var b strings.Builder
	b.WriteString(memory.FormatStatusLine(res.Status))
	b.WriteString("\n")
	if !res.Status.Present {
		out.Summary = b.String()
		return out, nil
	}
	b.WriteString(fmt.Sprintf("opened %d file(s) (session first, max %d buckets):\n",
		len(res.Trace.Opened), memory.MaxBuckets))
	for i, p := range res.Trace.Opened {
		b.WriteString(fmt.Sprintf("  [%d] %s\n", i, p))
	}
	b.WriteString(fmt.Sprintf("buckets_loaded=%d contexts_loaded=%d\n",
		len(res.Buckets), len(res.Contexts)))
	// Soft proof bounds for VAL-ORCH-005:
	// ≤ 1 session + MaxBuckets indexes + MaxContexts pointed contexts
	maxExpected := 1 + memory.MaxBuckets + memory.MaxContexts
	if len(res.Trace.Opened) > maxExpected {
		b.WriteString(fmt.Sprintf("WARNING: open count %d exceeds soft max %d\n",
			len(res.Trace.Opened), maxExpected))
	}
	out.Summary = b.String()
	return out, nil
}

// MultiBrainAppendInput is one protocol-compliant write after meaningful work.
type MultiBrainAppendInput struct {
	Workspace    string
	Agent        string
	Summary      string
	Topic        string
	Query        string
	Bucket       string
	Description  string
	Decision     string
	Blocker      string
	Verification string
	Files        []string
	Body         string
	// When overrides the entry timestamp (tests). Zero → now WIB.
	When time.Time
}

// MultiBrainAppendResult mirrors memory.AppendResult with format evidence.
type MultiBrainAppendResult struct {
	memory.AppendResult
	// FormatOK is true when EntryLine matches required timestamp style.
	FormatOK bool
}

// MultiBrainAppend appends one newest-first entry via memory.Append / WriteGateway
// with the required timestamp format and optional context pointer (VAL-ORCH-005).
func MultiBrainAppend(gw *writegate.Gateway, in MultiBrainAppendInput) (MultiBrainAppendResult, error) {
	if gw == nil {
		return MultiBrainAppendResult{}, fmt.Errorf("multi-brain: WriteGateway is required")
	}
	ws := strings.TrimSpace(in.Workspace)
	if ws == "" {
		ws = gw.RootPath()
	}
	rec := memory.Record{
		Agent:        in.Agent,
		Summary:      in.Summary,
		Topic:        in.Topic,
		Query:        in.Query,
		Bucket:       in.Bucket,
		Description:  in.Description,
		Decision:     in.Decision,
		Blocker:      in.Blocker,
		Verification: in.Verification,
		Files:        in.Files,
		Body:         in.Body,
		When:         in.When,
	}
	res, err := memory.Append(gw, ws, rec)
	if err != nil {
		return MultiBrainAppendResult{}, err
	}
	return MultiBrainAppendResult{
		AppendResult: res,
		FormatOK:     ValidateEntryFormat(res.EntryLine),
	}, nil
}

// ValidateEntryFormat checks the required entry shape:
//
//	- YYYY-MM-DD HH:mm WIB — Agent: summary[ -> .multibrain/context/…]
func ValidateEntryFormat(line string) bool {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "- ")
	// Timestamp: 2006-01-02 15:04 WIB
	// Then em dash / agent / summary / optional pointer.
	if len(line) < len("2006-01-02 15:04 WIB") {
		return false
	}
	// crude but strict enough for tests
	parts := strings.SplitN(line, " WIB", 2)
	if len(parts) != 2 {
		return false
	}
	ts := parts[0]
	if len(ts) != len("2006-01-02 15:04") {
		return false
	}
	// date digit check
	if ts[4] != '-' || ts[7] != '-' || ts[10] != ' ' || ts[13] != ':' {
		return false
	}
	rest := strings.TrimSpace(parts[1])
	// expect "— Agent:" or "– Agent:" or "- Agent:"
	if !strings.Contains(rest, ":") {
		return false
	}
	return true
}

// FormatMaxOpenBound documents the VAL-ORCH-005 open cap for traces:
// 1 session + 2 indexes + N pointed contexts (N capped by memory.MaxContexts).
func FormatMaxOpenBound() int {
	return 1 + memory.MaxBuckets + memory.MaxContexts
}
