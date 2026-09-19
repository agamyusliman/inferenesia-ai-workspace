package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/memory"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
	"github.com/spf13/cobra"
)

func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Multi Brain memory status and write (no RAG)",
		Long: `Inspect and update project Multi Brain memory under .multibrain/.

Default memory for ` + brand.Name + ` is file-based Multi Brain only:
  session.md (master index) → 1–2 buckets under indexes/ → pointed context files.

Write path (after meaningful work):
  memory append --summary "…" [--query …] [--bucket …]
  → best-matching bucket (newest-first entry), optional context file,
    session.md Last updated refresh; secrets scrubbed; via WriteGateway.

There is no Qdrant / Voyage / enowx-rag / embedding path in this subsystem
(memory works fully offline).`,
		// bare `memory` → status (same as memory status)
		RunE: runMemoryStatusCmd,
	}
	addMemoryStatusFlags(cmd)
	status := &cobra.Command{
		Use:   "status",
		Short: "Detect Multi Brain and show bounded load status",
		Long: `Detect whether the workspace has .multibrain/.

When present, reads session.md first (master index), then at most 1–2 matching
bucket files (not the whole tree). With --verbose, prints the open order so
session.md is visibly first (VAL-MEM-001 / VAL-MEM-002).

When absent, prints "no multi-brain" and exits 0 (no crash).`,
		RunE: runMemoryStatusCmd,
	}
	addMemoryStatusFlags(status)
	cmd.AddCommand(status)

	appendCmd := &cobra.Command{
		Use:   "append",
		Short: "Append a Multi Brain entry (best-match bucket, optional context)",
		Long: `After meaningful work, append one newest-first entry to the best-matching
bucket under .multibrain/indexes/, optionally write a context file when
decision/blocker/files/verification is provided, and refresh session.md
Last updated for that bucket. Creates a new bucket when --bucket names a new area.

All writes go through WriteGateway (undoable, source=multibrain). Content is
scrubbed of API keys/tokens before write (VAL-MEM-003..006, VAL-MEM-011).`,
		RunE: runMemoryAppendCmd,
	}
	appendCmd.Flags().String("workspace", "", "Workspace root (default: current directory)")
	appendCmd.Flags().String("summary", "", "Short one-line summary (required)")
	appendCmd.Flags().String("agent", "Droid", "Agent display name")
	appendCmd.Flags().String("topic", "", "Topic slug for optional context filename")
	appendCmd.Flags().String("query", "", "Free-text to select the best-matching bucket")
	appendCmd.Flags().String("bucket", "", "Force bucket name (creates if missing)")
	appendCmd.Flags().String("description", "", "Scope text when creating a new bucket")
	appendCmd.Flags().String("decision", "", "Decision note (triggers context file)")
	appendCmd.Flags().String("blocker", "", "Blocker note (triggers context file)")
	appendCmd.Flags().String("verification", "", "Verification evidence (triggers context file)")
	appendCmd.Flags().StringSlice("file", nil, "Important file path (repeatable; triggers context)")
	appendCmd.Flags().String("body", "", "Extra free-form context body")
	_ = appendCmd.MarkFlagRequired("summary")
	cmd.AddCommand(appendCmd)

	return cmd
}

func addMemoryStatusFlags(cmd *cobra.Command) {
	cmd.Flags().String("workspace", "", "Workspace root (default: current directory)")
	cmd.Flags().BoolP("verbose", "v", false, "Trace which Multi Brain files were opened (session.md first)")
	cmd.Flags().String("query", "", "Optional free-text to rank which 1–2 buckets to open")
}

func runMemoryStatusCmd(cmd *cobra.Command, args []string) error {
	workspace, _ := cmd.Flags().GetString("workspace")
	verbose, _ := cmd.Flags().GetBool("verbose")
	query, _ := cmd.Flags().GetString("query")
	return runMemoryStatus(cmd, workspace, verbose, query)
}

func runMemoryStatus(cmd *cobra.Command, workspace string, verbose bool, query string) error {
	ws, err := resolveWorkspace(workspace)
	if err != nil {
		return err
	}

	// Always Detect first for the short status line (VAL-MEM-001).
	st, err := memory.Detect(ws)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), memory.FormatStatusLine(st))
	if !st.Present {
		if verbose {
			fmt.Fprintln(cmd.OutOrStdout(), "[memory] no files opened (multi-brain absent)")
		}
		return nil
	}

	// Bounded load: session first, then ≤2 buckets (+ pointed contexts).
	res, err := memory.Load(ws, query)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "workspace: %s\n", ws)
	fmt.Fprintf(cmd.OutOrStdout(), "session: %s", filepath.Join(ws, memory.DirName, memory.SessionFile))
	if res.Status.SessionExists {
		fmt.Fprintln(cmd.OutOrStdout(), " (ok)")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), " (missing)")
	}
	if len(res.Status.Buckets) > 0 {
		names := make([]string, 0, len(res.Status.Buckets))
		for _, b := range res.Status.Buckets {
			names = append(names, b.Name)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "buckets listed in session.md: %s\n", strings.Join(names, ", "))
	}
	if len(res.Buckets) > 0 {
		loaded := make([]string, 0, len(res.Buckets))
		for name := range res.Buckets {
			loaded = append(loaded, name)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "buckets opened (bounded ≤%d): %s\n", memory.MaxBuckets, strings.Join(loaded, ", "))
	}
	if len(res.Contexts) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "context files opened (pointed only): %d\n", len(res.Contexts))
	}
	fmt.Fprintln(cmd.OutOrStdout(), "path: file-based Multi Brain only (no RAG / vector store)")

	if verbose {
		fmt.Fprintln(cmd.OutOrStdout(), "[memory] open order (session.md must be first):")
		memory.WriteTrace(cmd.OutOrStdout(), res.Trace)
		if len(res.Trace.Opened) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "[memory] (no files opened)")
		} else {
			// Explicit callout for validators.
			base := filepath.Base(res.Trace.Opened[0])
			fmt.Fprintf(cmd.OutOrStdout(), "[memory] first read: %s\n", base)
		}
	}
	return nil
}

func runMemoryAppendCmd(cmd *cobra.Command, args []string) error {
	workspace, _ := cmd.Flags().GetString("workspace")
	summary, _ := cmd.Flags().GetString("summary")
	agent, _ := cmd.Flags().GetString("agent")
	topic, _ := cmd.Flags().GetString("topic")
	query, _ := cmd.Flags().GetString("query")
	bucket, _ := cmd.Flags().GetString("bucket")
	desc, _ := cmd.Flags().GetString("description")
	decision, _ := cmd.Flags().GetString("decision")
	blocker, _ := cmd.Flags().GetString("blocker")
	verification, _ := cmd.Flags().GetString("verification")
	files, _ := cmd.Flags().GetStringSlice("file")
	body, _ := cmd.Flags().GetString("body")

	ws, err := resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	gw, err := writegate.New(ws)
	if err != nil {
		return err
	}
	// Prefer local Asia/Jakarta-style timestamps for Multibrain consistency.
	when := time.Now()
	rec := memory.Record{
		Agent:        agent,
		Summary:      summary,
		Topic:        topic,
		Query:        query,
		Bucket:       bucket,
		Description:  desc,
		Decision:     decision,
		Blocker:      blocker,
		Verification: verification,
		Files:        files,
		Body:         body,
		When:         when,
	}
	res, err := memory.Append(gw, ws, rec)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "memory: appended to bucket %q (%s)\n", res.BucketName, res.BucketPath)
	fmt.Fprintf(cmd.OutOrStdout(), "entry: %s\n", res.EntryLine)
	if res.ContextPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "context: %s\n", res.ContextPath)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "context: (skipped — no decision/blocker/files/verification)")
	}
	if res.SessionUpdated {
		fmt.Fprintln(cmd.OutOrStdout(), "session.md: Last updated refreshed")
	}
	if res.CreatedBucket {
		fmt.Fprintf(cmd.OutOrStdout(), "bucket: created new area %q\n", res.BucketName)
	}
	if res.Compacted {
		fmt.Fprintf(cmd.OutOrStdout(), "compacted: soft cap ~%d; older entries -> %s\n",
			memory.SoftCapEntries, res.CompactSummaryPath)
	}
	if res.EntryCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "entries: %d\n", res.EntryCount)
	}
	return nil
}
