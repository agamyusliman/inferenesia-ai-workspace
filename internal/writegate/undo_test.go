package writegate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fileSHA256 returns hex sha256 of path contents (empty string if missing).
func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// initGitRepo creates a git repo with f.txt committed as HEAD-COMMITTED, then
// dirties f.txt to DIRTY-BEFORE (uncommitted). Returns workspace root.
func initGitRepoWithDirty(t *testing.T) (root, fPath string) {
	t.Helper()
	root = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	// Quiet "main" default when supported.
	_ = exec.Command("git", "-C", root, "checkout", "-b", "main").Run()
	fPath = filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("HEAD-COMMITTED"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-m", "initial")
	// Dirty working tree: differs from HEAD.
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, fPath
}

func gitDiff(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "diff")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		// diff exits 1 when there are differences — still return output.
		if len(out) == 0 {
			t.Fatalf("git diff: %v", err)
		}
	}
	return string(out)
}

// VAL-UNDO-001: write then undo restores byte-identical pre-agent dirty content;
// git diff after undo matches pre-agent dirty diff.
func TestUndoRestoresPreAgentDirtyNotHEAD(t *testing.T) {
	root, fPath := initGitRepoWithDirty(t)

	// Capture pre-agent state.
	preHash := fileSHA256(t, fPath)
	preDiff := gitDiff(t, root)
	if !strings.Contains(preDiff, "DIRTY-BEFORE") {
		t.Fatalf("pre-agent git diff should mention DIRTY-BEFORE:\n%s", preDiff)
	}
	// Confirm HEAD is still HEAD-COMMITTED.
	cmd := exec.Command("git", "show", "HEAD:f.txt")
	cmd.Dir = root
	headBytes, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(headBytes) != "HEAD-COMMITTED" {
		t.Fatalf("HEAD content = %q", headBytes)
	}

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, fPath)); got != "AFTER" {
		t.Fatalf("after write = %q", got)
	}

	res := g.Undo()
	if res.Noop {
		t.Fatalf("undo should not be noop: %s", res.Message)
	}
	// Byte-identical to pre-agent dirty.
	afterUndo := mustRead(t, fPath)
	if string(afterUndo) != "DIRTY-BEFORE" {
		t.Fatalf("after undo content = %q, want DIRTY-BEFORE", afterUndo)
	}
	if fileSHA256(t, fPath) != preHash {
		t.Fatalf("sha256 after undo != pre-agent")
	}
	// Explicitly not HEAD.
	if string(afterUndo) == "HEAD-COMMITTED" {
		t.Fatal("undo restored HEAD; must restore pre-agent dirty")
	}
	postDiff := gitDiff(t, root)
	if postDiff != preDiff {
		t.Fatalf("git diff after undo must match pre-agent dirty diff\npre:\n%s\npost:\n%s", preDiff, postDiff)
	}
	// Message must not claim restore-to-head.
	lower := strings.ToLower(res.Message)
	if strings.Contains(lower, "restore-to-head") || strings.Contains(lower, "restore to head") {
		t.Fatalf("undo message must not use restore-to-head label: %q", res.Message)
	}
	if !strings.Contains(lower, "pre-agent") {
		t.Logf("prefer pre-agent wording in message: %q", res.Message)
	}
}

// VAL-UNDO-002: post-undo is DIRTY-BEFORE not HEAD; no default restore-to-head.
func TestUndoNotDefaultRestoreToHead(t *testing.T) {
	root, fPath := initGitRepoWithDirty(t)
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	res := g.Undo()
	if res.Noop {
		t.Fatal("expected undo to apply")
	}
	got := string(mustRead(t, fPath))
	if got != "DIRTY-BEFORE" {
		t.Fatalf("content=%q want DIRTY-BEFORE", got)
	}
	if got == "HEAD-COMMITTED" {
		t.Fatal("got HEAD content")
	}
	lower := strings.ToLower(res.Message)
	for _, bad := range []string{"restore-to-head", "restored to head", "git head"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("default undo message must not be restore-to-head: %q", res.Message)
		}
	}
}

// VAL-UNDO-003: redo reapplies AFTER; second undo -> DIRTY-BEFORE; redo-on-empty no-op.
func TestRedoReappliesAndEmptyIsNoop(t *testing.T) {
	root, fPath := initGitRepoWithDirty(t)
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	u1 := g.Undo()
	if u1.Noop || string(mustRead(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("first undo failed: noop=%v content=%q msg=%s", u1.Noop, mustRead(t, fPath), u1.Message)
	}

	r1 := g.Redo()
	if r1.Noop {
		t.Fatalf("redo should apply: %s", r1.Message)
	}
	if string(mustRead(t, fPath)) != "AFTER" {
		t.Fatalf("after redo = %q, want AFTER", mustRead(t, fPath))
	}

	u2 := g.Undo()
	if u2.Noop || string(mustRead(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("second undo failed: content=%q msg=%s", mustRead(t, fPath), u2.Message)
	}

	// Clear redo by... actually we have redo after second undo.
	// Flush redo by undoing with empty? After second undo, redo has AFTER again.
	// redo-on-empty: redo until empty then one more.
	_ = g.Redo() // AFTER again
	// Now undo once more so redo is empty... write a new file to invalidate redo,
	// or redo when stack empty:
	// After redo we're at AFTER with redo empty.
	empty := g.Redo()
	if !empty.Noop {
		t.Fatalf("redo on empty must be noop, got: %s", empty.Message)
	}
	if !strings.Contains(strings.ToLower(empty.Message), "nothing to redo") {
		t.Fatalf("clear nothing-to-redo message required, got %q", empty.Message)
	}
	// Content unchanged.
	if string(mustRead(t, fPath)) != "AFTER" {
		t.Fatalf("content changed on empty redo: %q", mustRead(t, fPath))
	}
}

func TestWriteEmitsChangeEvent(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	var events []ChangeEvent
	g.SetOnChange(func(ev ChangeEvent) { events = append(events, ev) })
	if _, err := g.WriteFile("a.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Op != "write" || events[0].Rel != "a.txt" {
		t.Fatalf("events=%+v", events)
	}
	_ = g.Undo()
	if len(events) != 2 || events[1].Op != "undo" {
		t.Fatalf("events after undo=%+v", events)
	}
	_ = g.Redo()
	if len(events) != 3 || events[2].Op != "redo" {
		t.Fatalf("events after redo=%+v", events)
	}
}

func TestSnapshotCapturesBeforeWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := g.WriteFile("f.txt", []byte("AFTER"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(entry.Snapshot.Before, []byte("DIRTY-BEFORE")) {
		t.Fatalf("before=%q", entry.Snapshot.Before)
	}
	if !bytes.Equal(entry.After, []byte("AFTER")) {
		t.Fatalf("after=%q", entry.After)
	}
	if g.UndoDepth() != 1 {
		t.Fatalf("undo depth=%d", g.UndoDepth())
	}
}

func TestSaveLoadStackRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	stackPath := filepath.Join(t.TempDir(), "stack.json")
	if err := g.SaveStack(stackPath); err != nil {
		t.Fatal(err)
	}

	// Simulate new process: load into fresh gateway and undo.
	g2, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := g2.LoadStack(stackPath); err != nil {
		t.Fatal(err)
	}
	if g2.UndoDepth() != 1 {
		t.Fatalf("loaded undo depth=%d", g2.UndoDepth())
	}
	res := g2.Undo()
	if res.Noop {
		t.Fatal("loaded stack should allow undo")
	}
	if got := string(mustRead(t, filepath.Join(root, "f.txt"))); got != "DIRTY-BEFORE" {
		t.Fatalf("after load+undo = %q", got)
	}
}

func TestContentEqual(t *testing.T) {
	if !ContentEqual(nil, []byte{}) {
		t.Fatal("nil vs empty")
	}
	if ContentEqual([]byte("a"), []byte("b")) {
		t.Fatal("a vs b")
	}
}

// VAL-UNDO-004: multi-file turn undo restores BOTH files; redo reapplies both.
func TestMultiFileTurnUndoRedo(t *testing.T) {
	root := t.TempDir()
	aPath := filepath.Join(root, "a.txt")
	bPath := filepath.Join(root, "b.txt")
	if err := os.WriteFile(aPath, []byte("A-DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("B-DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	aHash := fileSHA256(t, aPath)
	bHash := fileSHA256(t, bPath)

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("turn-multi")
	if _, err := g.WriteFile("a.txt", []byte("A-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("b.txt", []byte("B-AFTER")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()

	if g.UndoDepth() != 1 {
		t.Fatalf("one turn must be one undo unit, depth=%d", g.UndoDepth())
	}
	if string(mustRead(t, aPath)) != "A-AFTER" || string(mustRead(t, bPath)) != "B-AFTER" {
		t.Fatalf("after write a=%q b=%q", mustRead(t, aPath), mustRead(t, bPath))
	}

	res := g.Undo()
	if res.Noop {
		t.Fatalf("undo noop: %s", res.Message)
	}
	if string(mustRead(t, aPath)) != "A-DIRTY-BEFORE" {
		t.Fatalf("a after undo = %q", mustRead(t, aPath))
	}
	if string(mustRead(t, bPath)) != "B-DIRTY-BEFORE" {
		t.Fatalf("b after undo = %q", mustRead(t, bPath))
	}
	if fileSHA256(t, aPath) != aHash || fileSHA256(t, bPath) != bHash {
		t.Fatal("sha256 after undo must match pre-agent for both files")
	}
	if len(res.Changes) != 2 {
		t.Fatalf("undo should change 2 files, got %d: %+v", len(res.Changes), res.Changes)
	}

	redo := g.Redo()
	if redo.Noop {
		t.Fatalf("redo noop: %s", redo.Message)
	}
	if string(mustRead(t, aPath)) != "A-AFTER" {
		t.Fatalf("a after redo = %q", mustRead(t, aPath))
	}
	if string(mustRead(t, bPath)) != "B-AFTER" {
		t.Fatalf("b after redo = %q", mustRead(t, bPath))
	}
	if len(redo.Changes) != 2 {
		t.Fatalf("redo should change 2 files, got %d", len(redo.Changes))
	}
}

// VAL-UNDO-005: three sequential turns; undos restore LIFO v2, v1, DIRTY-BEFORE.
func TestUndoStackTurnScopedLIFO(t *testing.T) {
	root := t.TempDir()
	fPath := filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	// T1 → v1, T2 → v2, T3 → v3 (each BeginTurn/EndTurn is one agent turn).
	for i, v := range []string{"v1", "v2", "v3"} {
		g.BeginTurn(fmt.Sprintf("T%d", i+1))
		if _, err := g.WriteFile("f.txt", []byte(v)); err != nil {
			t.Fatal(err)
		}
		g.EndTurn()
	}
	if g.UndoDepth() != 3 {
		t.Fatalf("want 3 turn units, depth=%d", g.UndoDepth())
	}
	if string(mustRead(t, fPath)) != "v3" {
		t.Fatalf("after T3 = %q", mustRead(t, fPath))
	}

	want := []string{"v2", "v1", "DIRTY-BEFORE"}
	for i, w := range want {
		res := g.Undo()
		if res.Noop {
			t.Fatalf("undo %d noop: %s", i+1, res.Message)
		}
		got := string(mustRead(t, fPath))
		if got != w {
			t.Fatalf("after undo %d content=%q want %q (LIFO by turn)", i+1, got, w)
		}
	}
	// Fourth undo is empty no-op.
	empty := g.Undo()
	if !empty.Noop || !strings.Contains(strings.ToLower(empty.Message), "nothing to undo") {
		t.Fatalf("fourth undo must be empty no-op, got noop=%v msg=%q", empty.Noop, empty.Message)
	}
	if string(mustRead(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("content changed on empty undo: %q", mustRead(t, fPath))
	}
}

// VAL-UNDO-006: empty undo is safe no-op (package-level; CLI covered separately).
func TestUndoEmptyStackNoopNoMutation(t *testing.T) {
	root := t.TempDir()
	fPath := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(fPath, []byte("UNTOUCHED"), 0o644); err != nil {
		t.Fatal(err)
	}
	preHash := fileSHA256(t, fPath)

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	res := g.Undo()
	if !res.Noop {
		t.Fatalf("empty undo must be noop, got %s", res.Message)
	}
	if !strings.Contains(strings.ToLower(res.Message), "nothing to undo") {
		t.Fatalf("want nothing to undo, got %q", res.Message)
	}
	if fileSHA256(t, fPath) != preHash {
		t.Fatal("empty undo must not modify any file")
	}
	if g.UndoDepth() != 0 || g.RedoDepth() != 0 {
		t.Fatalf("stacks must stay empty: undo=%d redo=%d", g.UndoDepth(), g.RedoDepth())
	}
}

// Writes without BeginTurn remain one unit each (backward compatible).
func TestWriteWithoutTurnIsSingleUnit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("A0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("B0"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("a.txt", []byte("A1")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("b.txt", []byte("B1")); err != nil {
		t.Fatal(err)
	}
	if g.UndoDepth() != 2 {
		t.Fatalf("without BeginTurn each write is its own unit, depth=%d", g.UndoDepth())
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
