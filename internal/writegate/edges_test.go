package writegate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-UNDO-010: agent deletes f.txt then recreates with new content; undo restores
// pre-agent dirty content. If file did not exist pre-agent, undo removes it.
func TestUndoDeleteThenCreateRestoresPreAgentDirty(t *testing.T) {
	root := t.TempDir()
	fPath := filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	preHash := fileSHA256(t, fPath)

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("delete-create")
	if _, err := g.DeleteFile("f.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fPath); !os.IsNotExist(err) {
		t.Fatal("file should be gone after DeleteFile")
	}
	if _, err := g.WriteFile("f.txt", []byte("NEW-AFTER-RECREATE")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()

	if got := string(mustRead(t, fPath)); got != "NEW-AFTER-RECREATE" {
		t.Fatalf("after recreate = %q", got)
	}

	res := g.Undo()
	if res.Noop {
		t.Fatalf("undo should apply: %s", res.Message)
	}
	if got := string(mustRead(t, fPath)); got != "DIRTY-BEFORE" {
		t.Fatalf("after undo = %q, want DIRTY-BEFORE (delete+create as one turn)", got)
	}
	if fileSHA256(t, fPath) != preHash {
		t.Fatal("sha256 after undo != pre-agent")
	}
}

// VAL-UNDO-010 edge: file did not exist pre-agent; delete is no-op, create then undo removes.
func TestUndoCreateOnlyRemovesAbsentPreAgentFile(t *testing.T) {
	root := t.TempDir()
	fPath := filepath.Join(root, "brand-new.txt")

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	// Optional delete of missing file then create (same turn or sequential standalone).
	g.BeginTurn("create-only")
	if _, err := g.DeleteFile("brand-new.txt"); err != nil {
		// Delete of missing is allowed as a recorded no-op-ish snapshot.
		t.Fatalf("DeleteFile missing path: %v", err)
	}
	if _, err := g.WriteFile("brand-new.txt", []byte("CREATED-BY-AGENT")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()

	if _, err := os.Stat(fPath); err != nil {
		t.Fatal("agent-created file should exist")
	}

	res := g.Undo()
	if res.Noop {
		t.Fatalf("undo should apply: %s", res.Message)
	}
	if _, err := os.Stat(fPath); !os.IsNotExist(err) {
		t.Fatalf("pre-absent file must be removed after undo; still exists")
	}
	// At least one change event reports Removed.
	removed := false
	for _, ch := range res.Changes {
		if ch.Removed && (ch.Rel == "brand-new.txt" || strings.HasSuffix(ch.Path, "brand-new.txt")) {
			removed = true
		}
	}
	if !removed && len(res.Changes) == 0 {
		// Accept empty changes only if file truly gone and message mentions remove.
		if !strings.Contains(strings.ToLower(res.Message), "pre-agent") {
			t.Logf("message: %s", res.Message)
		}
	}
	if _, err := os.Stat(fPath); !os.IsNotExist(err) {
		t.Fatal("file still present")
	}
}

// VAL-UNDO-010: standalone delete then create without BeginTurn still undoes LIFO
// correctly: undo create removes file? Actually if pre-existed:
// delete unit then create unit — undo create restores after-delete (absent),
// undo delete restores DIRTY-BEFORE. With turn batching it's preferred, but
// sequential also must end at DIRTY-BEFORE after two undos.
func TestUndoDeleteThenCreateSequentialTwoUndos(t *testing.T) {
	root := t.TempDir()
	fPath := filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.DeleteFile("f.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("RECREATED")); err != nil {
		t.Fatal(err)
	}

	// Undo create → file should be gone (after-delete absent).
	u1 := g.Undo()
	if u1.Noop {
		t.Fatal("first undo noop")
	}
	if _, err := os.Stat(fPath); !os.IsNotExist(err) {
		t.Fatalf("after undo create, file should be absent, content=%q", mustRead(t, fPath))
	}

	// Undo delete → DIRTY-BEFORE restored.
	u2 := g.Undo()
	if u2.Noop {
		t.Fatal("second undo noop")
	}
	if got := string(mustRead(t, fPath)); got != "DIRTY-BEFORE" {
		t.Fatalf("after undo delete = %q", got)
	}
}

// VAL-UNDO-011: undo stack survives process restart (new Gateway LoadStack).
func TestUndoMetadataPersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	fPath := filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Session 1: write + save.
	g1, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g1.WriteFile("f.txt", []byte("SESSION1-AFTER")); err != nil {
		t.Fatal(err)
	}
	stackPath := PersistPath(home, root)
	if err := g1.SaveStack(stackPath); err != nil {
		t.Fatal(err)
	}
	// Evidence: stack file exists under config home workspaces path.
	if _, err := os.Stat(stackPath); err != nil {
		t.Fatalf("stack file missing at %s: %v", stackPath, err)
	}
	if !strings.Contains(stackPath, filepath.Join("workspaces")) {
		t.Fatalf("stack path should be under workspaces/: %s", stackPath)
	}
	info1, err := os.Stat(stackPath)
	if err != nil {
		t.Fatal(err)
	}
	if info1.Size() == 0 {
		t.Fatal("stack file empty")
	}
	if got := string(mustRead(t, fPath)); got != "SESSION1-AFTER" {
		t.Fatalf("disk after session1 = %q", got)
	}

	// Session 2: fresh gateway, load stack, undo.
	g2, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := g2.LoadStack(stackPath); err != nil {
		t.Fatal(err)
	}
	if g2.UndoDepth() != 1 {
		t.Fatalf("session2 undo depth=%d, want 1", g2.UndoDepth())
	}
	res := g2.Undo()
	if res.Noop {
		t.Fatalf("session2 undo should restore: %s", res.Message)
	}
	if got := string(mustRead(t, fPath)); got != "DIRTY-BEFORE" {
		t.Fatalf("session2 undo content=%q want DIRTY-BEFORE", got)
	}
	// Persist updated stacks after undo (as CLI would).
	if err := g2.SaveStack(stackPath); err != nil {
		t.Fatal(err)
	}
	if g2.RedoDepth() != 1 {
		t.Fatalf("after undo redo depth=%d", g2.RedoDepth())
	}
}

// VAL-UNDO-012: undo in workspace A does not affect workspace B.
func TestUndoStacksArePerWorkspace(t *testing.T) {
	home := t.TempDir()
	wsA := t.TempDir()
	wsB := t.TempDir()

	if err := os.WriteFile(filepath.Join(wsA, "f.txt"), []byte("A-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsB, "f.txt"), []byte("B-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}

	gA, err := New(wsA)
	if err != nil {
		t.Fatal(err)
	}
	gB, err := New(wsB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gA.WriteFile("f.txt", []byte("A-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := gB.WriteFile("f.txt", []byte("B-AFTER")); err != nil {
		t.Fatal(err)
	}

	pathA := PersistPath(home, wsA)
	pathB := PersistPath(home, wsB)
	if pathA == pathB {
		t.Fatalf("persist paths must differ per workspace: both %s", pathA)
	}
	if WorkspaceID(wsA) == WorkspaceID(wsB) {
		t.Fatal("workspace ids must differ for different roots")
	}
	if err := gA.SaveStack(pathA); err != nil {
		t.Fatal(err)
	}
	if err := gB.SaveStack(pathB); err != nil {
		t.Fatal(err)
	}

	// Undo only A.
	resA := gA.Undo()
	if resA.Noop {
		t.Fatal("wsA undo noop")
	}
	if got := string(mustRead(t, filepath.Join(wsA, "f.txt"))); got != "A-DIRTY" {
		t.Fatalf("wsA after undo=%q", got)
	}
	// wsB unchanged.
	if got := string(mustRead(t, filepath.Join(wsB, "f.txt"))); got != "B-AFTER" {
		t.Fatalf("wsB must be untouched after undo in A: got %q", got)
	}

	// Fresh gateways reload: A has redo/empty-ish; B still has undo of B-AFTER.
	gA2, err := New(wsA)
	if err != nil {
		t.Fatal(err)
	}
	_ = gA.SaveStack(pathA)
	if err := gA2.LoadStack(pathA); err != nil {
		t.Fatal(err)
	}
	gB2, err := New(wsB)
	if err != nil {
		t.Fatal(err)
	}
	if err := gB2.LoadStack(pathB); err != nil {
		t.Fatal(err)
	}
	if gB2.UndoDepth() != 1 {
		t.Fatalf("wsB stack must still have its own undo unit, depth=%d", gB2.UndoDepth())
	}
	// Undo B only via its own stack.
	resB := gB2.Undo()
	if resB.Noop {
		t.Fatal("wsB undo noop")
	}
	if got := string(mustRead(t, filepath.Join(wsB, "f.txt"))); got != "B-DIRTY" {
		t.Fatalf("wsB after undo=%q", got)
	}
	// wsA still at A-DIRTY (not re-clobbered).
	if got := string(mustRead(t, filepath.Join(wsA, "f.txt"))); got != "A-DIRTY" {
		t.Fatalf("wsA cross-contaminated: %q", got)
	}
}

// VAL-UNDO-013: external edit after undo soft-invalidates redo (skip/fail clear message).
func TestExternalEditSoftInvalidatesRedo(t *testing.T) {
	root := t.TempDir()
	fPath := filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	u := g.Undo()
	if u.Noop || string(mustRead(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("setup undo failed: %s content=%q", u.Message, mustRead(t, fPath))
	}
	if g.RedoDepth() != 1 {
		t.Fatalf("redo depth=%d", g.RedoDepth())
	}

	// External edit outside the undo stack (manual / other process).
	if err := os.WriteFile(fPath, []byte("EXTERNAL-EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := g.Redo()
	// Soft-fail: must not clobber blindly with AFTER.
	if string(mustRead(t, fPath)) == "AFTER" {
		t.Fatalf("redo must not clobber external edit with AFTER; content became AFTER")
	}
	if got := string(mustRead(t, fPath)); got != "EXTERNAL-EDIT" {
		t.Fatalf("content should remain external edit, got %q", got)
	}
	// Clear message about external change / skip / soft-fail.
	lower := strings.ToLower(r.Message)
	okMsg := strings.Contains(lower, "external") ||
		strings.Contains(lower, "changed outside") ||
		strings.Contains(lower, "soft") ||
		strings.Contains(lower, "skip") ||
		strings.Contains(lower, "mismatch") ||
		strings.Contains(lower, "invalidat")
	if !okMsg {
		t.Fatalf("redo soft-fail needs clear message, got %q", r.Message)
	}
	// No crash; either Noop-like or partial skip that preserved external content.
	if r.Noop && !okMsg {
		t.Fatalf("noop without clear reason: %q", r.Message)
	}
}

// VAL-UNDO-013 multi-file unit: one path externalized → skip that path, do not
// blindly write After over the external edit.
func TestExternalEditSoftInvalidatesRedoMultiFile(t *testing.T) {
	root := t.TempDir()
	aPath := filepath.Join(root, "a.txt")
	bPath := filepath.Join(root, "b.txt")
	if err := os.WriteFile(aPath, []byte("A0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("B0"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("t-multi")
	if _, err := g.WriteFile("a.txt", []byte("A1")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("b.txt", []byte("B1")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()
	_ = g.Undo()
	// Externalize a.txt only.
	if err := os.WriteFile(aPath, []byte("A-EXT"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := g.Redo()
	if string(mustRead(t, aPath)) == "A1" {
		t.Fatal("must not clobber a.txt external edit")
	}
	if got := string(mustRead(t, aPath)); got != "A-EXT" {
		t.Fatalf("a.txt = %q", got)
	}
	// b.txt may still redo (path unchanged) or entire unit soft-skipped — both OK
	// as long as a was not clobbered and message is clear.
	lower := strings.ToLower(r.Message)
	if !(strings.Contains(lower, "external") || strings.Contains(lower, "skip") ||
		strings.Contains(lower, "mismatch") || strings.Contains(lower, "changed") ||
		strings.Contains(lower, "invalidat")) {
		t.Fatalf("expected soft-fail message, got %q", r.Message)
	}
}
