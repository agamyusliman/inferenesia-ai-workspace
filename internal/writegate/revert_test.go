package writegate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-MEM-007 / VAL-CROSS-013: dirty user file → agent edit → revert last turn
// restores pre-agent dirty (not git HEAD). Also covers multibrain files in the
// same turn so memory roll-back is included when written via WriteGateway.
func TestRevertLastTurnRestoresDirtyAndMemory(t *testing.T) {
	root, fPath := initGitRepoWithDirty(t)

	// Pre-agent dirty (not HEAD).
	preHash := fileSHA256(t, fPath)
	if string(mustRead(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatal("fixture dirty missing")
	}

	// Seed multibrain bucket pre-state.
	mbDir := filepath.Join(root, ".multibrain", "indexes")
	if err := os.MkdirAll(mbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctxDir := filepath.Join(root, ".multibrain", "context")
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bucketRel := ".multibrain/indexes/architecture.md"
	bucketPre := "# architecture\n\n## Entries\n\n- old entry\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(bucketRel)), []byte(bucketPre), 0o644); err != nil {
		t.Fatal(err)
	}
	// Context file does not exist before the turn.
	ctxRel := ".multibrain/context/turn-ctx.md"

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("turn-mem-007")
	if _, err := g.WriteFile("f.txt", []byte("AGENT-AFTER")); err != nil {
		t.Fatal(err)
	}
	// Simulate memory append: bucket entry + new context file.
	bucketAfter := "# architecture\n\n## Entries\n\n- NEW turn entry -> .multibrain/context/turn-ctx.md\n- old entry\n"
	if _, err := g.WriteMultibrain(bucketRel, []byte(bucketAfter)); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteMultibrain(ctxRel, []byte("# context from turn\n")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()

	if string(mustRead(t, fPath)) != "AGENT-AFTER" {
		t.Fatal("agent write missing")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(ctxRel))); err != nil {
		t.Fatalf("context should exist: %v", err)
	}

	res := g.Revert()
	if res.Noop {
		t.Fatalf("revert noop: %s", res.Message)
	}
	if !strings.Contains(res.Message, "reverted last agent turn") {
		t.Fatalf("message = %q", res.Message)
	}

	// VAL-CROSS-013 / VAL-MEM-007: pre-agent dirty bytes, not HEAD.
	after := mustRead(t, fPath)
	if string(after) != "DIRTY-BEFORE" {
		t.Fatalf("after revert f.txt = %q want DIRTY-BEFORE (not HEAD)", after)
	}
	if fileSHA256(t, fPath) != preHash {
		t.Fatal("sha256 before-turn != after-revert")
	}
	// Confirm not HEAD.
	if string(after) == "HEAD-COMMITTED" {
		t.Fatal("revert restored HEAD — fail VAL-CROSS-013")
	}

	// Memory: bucket rolled back; context file absent.
	gotBucket := string(mustRead(t, filepath.Join(root, filepath.FromSlash(bucketRel))))
	if gotBucket != bucketPre {
		t.Fatalf("bucket after revert =\n%q\nwant\n%q", gotBucket, bucketPre)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(ctxRel))); !os.IsNotExist(err) {
		t.Fatalf("context file should be absent after revert, err=%v", err)
	}
}

// VAL-MEM-008: revert <file> restores only that file; other files/turn memory intact.
// Unchanged file → safe no-op with clear message.
func TestRevertFileSinglePath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("A-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("B-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Multibrain pre
	if err := os.MkdirAll(filepath.Join(root, ".multibrain", "indexes"), 0o755); err != nil {
		t.Fatal(err)
	}
	bucketRel := ".multibrain/indexes/agents.md"
	bucketPre := "# agents\n\n## Entries\n\n- keep\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(bucketRel)), []byte(bucketPre), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("t-file")
	if _, err := g.WriteFile("a.txt", []byte("A-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("b.txt", []byte("B-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteMultibrain(bucketRel, []byte("# agents\n\n## Entries\n\n- new\n- keep\n")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()

	res := g.RevertFile("a.txt")
	if res.Noop {
		t.Fatalf("revert a.txt noop: %s", res.Message)
	}
	if string(mustRead(t, filepath.Join(root, "a.txt"))) != "A-DIRTY" {
		t.Fatal("a.txt not restored")
	}
	// b.txt and multibrain must stay agent-written.
	if string(mustRead(t, filepath.Join(root, "b.txt"))) != "B-AFTER" {
		t.Fatal("b.txt should be untouched")
	}
	if !strings.Contains(string(mustRead(t, filepath.Join(root, filepath.FromSlash(bucketRel)))), "- new") {
		t.Fatal("memory should not be rolled back by single-file revert")
	}
	// Stack still has remaining entries (b + multibrain) for full undo later.
	if g.UndoDepth() != 1 {
		t.Fatalf("undo depth=%d want 1 (remaining turn files)", g.UndoDepth())
	}

	// Unchanged file / never touched.
	res2 := g.RevertFile("never.txt")
	if !res2.Noop {
		t.Fatalf("expected noop for unchanged file: %s", res2.Message)
	}
	if !strings.Contains(res2.Message, "not changed") {
		t.Fatalf("clear message missing: %q", res2.Message)
	}
}

// VAL-MEM-012: history lists agent turns/files; empty workspace → clear empty message.
func TestHistoryTimeline(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.History()) != 0 {
		t.Fatal("fresh gateway should have empty history")
	}
	empty := FormatHistory(nil)
	if !strings.Contains(empty, "no agent changes") && !strings.Contains(empty, "fresh workspace") {
		t.Fatalf("empty history message unclear: %q", empty)
	}

	if _, err := g.WriteFile("x.txt", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("turn-2")
	if _, err := g.WriteFile("y.txt", []byte("y")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("z.txt", []byte("z")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()

	h := g.History()
	if len(h) != 2 {
		t.Fatalf("history len=%d want 2", len(h))
	}
	// Newest first.
	if h[0].Index != 1 || h[0].FileCount != 2 {
		t.Fatalf("newest entry = %+v", h[0])
	}
	if h[0].TurnID != "turn-2" {
		t.Fatalf("turn id = %q", h[0].TurnID)
	}
	if h[1].FileCount != 1 {
		t.Fatalf("older entry = %+v", h[1])
	}
	text := FormatHistory(h)
	if !strings.Contains(text, "turn-2") || !strings.Contains(text, "y.txt") {
		t.Fatalf("format missing details:\n%s", text)
	}
}

func TestRevertEmptyStack(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	res := g.Revert()
	if !res.Noop || res.Message != MsgNothingToRevert {
		t.Fatalf("empty revert: %+v", res)
	}
}
