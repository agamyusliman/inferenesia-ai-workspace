package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-UNDO-001/002/003 via CLI surfaces: undo restores dirty, not HEAD; redo reapply.
func TestCLIUndoRedoPreAgentDirty(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	// Git repo: HEAD=HEAD-COMMITTED, dirty DIRTY-BEFORE.
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
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
	runGit("init")
	_ = exec.Command("git", "-C", ws, "checkout", "-b", "main").Run()
	fPath := filepath.Join(ws, "f.txt")
	if err := os.WriteFile(fPath, []byte("HEAD-COMMITTED"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "f.txt")
	runGit("commit", "-m", "initial")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate agent write via WriteGateway + persist (as chat would).
	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	stackPath := writegate.PersistPath(home, ws)
	if err := g.SaveStack(stackPath); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadFile(t, fPath)); got != "AFTER" {
		t.Fatalf("after agent write = %q", got)
	}

	// CLI undo
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("undo exit=%d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	msg := out.String() + errBuf.String()
	if string(mustReadFile(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("after undo = %q, want DIRTY-BEFORE", mustReadFile(t, fPath))
	}
	if strings.Contains(strings.ToLower(msg), "restore-to-head") {
		t.Fatalf("undo must not label restore-to-head: %q", msg)
	}
	if strings.Contains(msg, "HEAD-COMMITTED") && !strings.Contains(msg, "DIRTY") {
		// Message listing content is fine; must not be HEAD content on disk.
	}
	// Explicitly not HEAD on disk.
	if string(mustReadFile(t, fPath)) == "HEAD-COMMITTED" {
		t.Fatal("undo restored HEAD")
	}
	if !strings.Contains(strings.ToLower(msg), "pre-agent") && !strings.Contains(strings.ToLower(msg), "undid") {
		t.Fatalf("expected clear undo message, got %q", msg)
	}

	// CLI redo
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"redo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("redo exit=%d stderr=%q", code, errBuf.String())
	}
	if string(mustReadFile(t, fPath)) != "AFTER" {
		t.Fatalf("after redo = %q", mustReadFile(t, fPath))
	}

	// Second undo -> DIRTY-BEFORE
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("second undo exit=%d", code)
	}
	if string(mustReadFile(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("after second undo = %q", mustReadFile(t, fPath))
	}

	// Redo to AFTER then redo-on-empty.
	out.Reset()
	errBuf.Reset()
	_ = ExecuteWith([]string{"redo", "--workspace", ws}, &out, &errBuf)
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"redo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("empty redo should exit 0, got %d stderr=%q", code, errBuf.String())
	}
	combined := out.String() + errBuf.String()
	if !strings.Contains(strings.ToLower(combined), "nothing to redo") {
		t.Fatalf("expected nothing to redo message, got %q", combined)
	}
	if string(mustReadFile(t, fPath)) != "AFTER" {
		t.Fatalf("content changed on empty redo: %q", mustReadFile(t, fPath))
	}
}

// VAL-UNDO-006: fresh workspace CLI undo is safe no-op; no file/git mutation.
func TestCLIUndoEmptyStack(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	// Git workspace with a tracked file for git-state check.
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
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
	runGit("init")
	_ = exec.Command("git", "-C", ws, "checkout", "-b", "main").Run()
	fPath := filepath.Join(ws, "keep.txt")
	if err := os.WriteFile(fPath, []byte("UNTOUCHED"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "keep.txt")
	runGit("commit", "-m", "initial")
	// Soft dirty (uncommitted) that must survive empty undo.
	if err := os.WriteFile(fPath, []byte("USER-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	preContent := string(mustReadFile(t, fPath))
	preDiff, _ := exec.Command("git", "-C", ws, "diff").CombinedOutput()
	preStatus, _ := exec.Command("git", "-C", ws, "status", "--porcelain").CombinedOutput()

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("empty undo exit=%d stderr=%q", code, errBuf.String())
	}
	combined := out.String() + errBuf.String()
	if !strings.Contains(strings.ToLower(combined), "nothing to undo") {
		t.Fatalf("want nothing to undo, got %q", combined)
	}
	if string(mustReadFile(t, fPath)) != preContent {
		t.Fatalf("empty undo modified file: got %q want %q", mustReadFile(t, fPath), preContent)
	}
	postDiff, _ := exec.Command("git", "-C", ws, "diff").CombinedOutput()
	postStatus, _ := exec.Command("git", "-C", ws, "status", "--porcelain").CombinedOutput()
	if string(postDiff) != string(preDiff) {
		t.Fatalf("git diff changed on empty undo\npre:\n%s\npost:\n%s", preDiff, postDiff)
	}
	if string(postStatus) != string(preStatus) {
		t.Fatalf("git status changed on empty undo\npre:\n%s\npost:\n%s", preStatus, postStatus)
	}
}

// VAL-UNDO-004 via CLI: multi-file turn then one undo restores both.
func TestCLIMultiFileTurnUndo(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	aPath := filepath.Join(ws, "a.txt")
	bPath := filepath.Join(ws, "b.txt")
	if err := os.WriteFile(aPath, []byte("A-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("B-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("chat-1")
	if _, err := g.WriteFile("a.txt", []byte("A-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("b.txt", []byte("B-AFTER")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()
	if err := g.SaveStack(writegate.PersistPath(home, ws)); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("undo exit=%d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	if string(mustReadFile(t, aPath)) != "A-BEFORE" {
		t.Fatalf("a after undo = %q", mustReadFile(t, aPath))
	}
	if string(mustReadFile(t, bPath)) != "B-BEFORE" {
		t.Fatalf("b after undo = %q", mustReadFile(t, bPath))
	}

	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"redo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("redo exit=%d stderr=%q", code, errBuf.String())
	}
	if string(mustReadFile(t, aPath)) != "A-AFTER" || string(mustReadFile(t, bPath)) != "B-AFTER" {
		t.Fatalf("after redo a=%q b=%q", mustReadFile(t, aPath), mustReadFile(t, bPath))
	}
}

func TestHelpListsUndoRedo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
	combined := out.String()
	if !strings.Contains(combined, "undo") {
		t.Fatalf("help missing undo:\n%s", combined)
	}
	if !strings.Contains(combined, "redo") {
		t.Fatalf("help missing redo:\n%s", combined)
	}
}

// VAL-UNDO-009: CLI emits [file_changed] on write (via OnChange), undo, and redo with path names.
func TestCLIFileChangedEventsOnWriteUndoRedo(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	fPath := filepath.Join(ws, "named-file.txt")
	if err := os.WriteFile(fPath, []byte("BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate chat write path: gateway with event printer (stderr-style buffer).
	var eventBuf bytes.Buffer
	g, cleanup, err := gatewayForChat(ws, &eventBuf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("named-file.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	writeOut := eventBuf.String()
	if !strings.Contains(writeOut, "[file_changed]") || !strings.Contains(writeOut, "write") || !strings.Contains(writeOut, "named-file.txt") {
		t.Fatalf("write must emit [file_changed] with path, got %q", writeOut)
	}

	// CLI undo prints [file_changed] undo <path>
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("undo exit=%d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	undoMsg := out.String()
	if !strings.Contains(undoMsg, "[file_changed]") || !strings.Contains(undoMsg, "undo") || !strings.Contains(undoMsg, "named-file.txt") {
		t.Fatalf("undo CLI must show FileChanged with path, got %q", undoMsg)
	}
	if string(mustReadFile(t, fPath)) != "BEFORE" {
		t.Fatalf("after undo content=%q", mustReadFile(t, fPath))
	}

	// CLI redo
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"redo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("redo exit=%d stderr=%q", code, errBuf.String())
	}
	redoMsg := out.String()
	if !strings.Contains(redoMsg, "[file_changed]") || !strings.Contains(redoMsg, "redo") || !strings.Contains(redoMsg, "named-file.txt") {
		t.Fatalf("redo CLI must show FileChanged with path, got %q", redoMsg)
	}
	if string(mustReadFile(t, fPath)) != "AFTER" {
		t.Fatalf("after redo content=%q", mustReadFile(t, fPath))
	}
}

// VAL-UNDO-007 surface: multibrain + diagram writes through gateway leave one stack entry each; CLI undo restores.
func TestCLIUndoRoutesMultibrainAndDiagram(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(filepath.Join(ws, ".multibrain", "indexes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "docs", "diagrams"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	mb := filepath.Join(ws, ".multibrain", "indexes", "agents.md")
	dg := filepath.Join(ws, "docs", "diagrams", "flow.mmd")
	if err := os.WriteFile(mb, []byte("MB-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dg, []byte("DG-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteMultibrain(".multibrain/indexes/agents.md", []byte("MB-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteDiagram("docs/diagrams/flow.mmd", []byte("DG-AFTER")); err != nil {
		t.Fatal(err)
	}
	if err := g.SaveStack(writegate.PersistPath(home, ws)); err != nil {
		t.Fatal(err)
	}
	if g.UndoDepth() != 2 {
		t.Fatalf("depth=%d", g.UndoDepth())
	}

	// Undo diagram then multibrain via CLI (LIFO).
	var out, errBuf bytes.Buffer
	if code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf); code != 0 {
		t.Fatalf("undo1 exit=%d %s %s", code, out.String(), errBuf.String())
	}
	if string(mustReadFile(t, dg)) != "DG-BEFORE" {
		t.Fatalf("diagram after undo=%q", mustReadFile(t, dg))
	}
	out.Reset()
	errBuf.Reset()
	if code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf); code != 0 {
		t.Fatalf("undo2 exit=%d %s %s", code, out.String(), errBuf.String())
	}
	if string(mustReadFile(t, mb)) != "MB-BEFORE" {
		t.Fatalf("multibrain after undo=%q", mustReadFile(t, mb))
	}
}

// VAL-UNDO-010 via CLI: delete then recreate in one turn; undo restores DIRTY-BEFORE.
func TestCLIUndoDeleteThenCreate(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	fPath := filepath.Join(ws, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("del-create")
	if _, err := g.DeleteFile("f.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("RECREATED")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()
	if err := g.SaveStack(writegate.PersistPath(home, ws)); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("undo exit=%d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	if got := string(mustReadFile(t, fPath)); got != "DIRTY-BEFORE" {
		t.Fatalf("after CLI undo = %q, want DIRTY-BEFORE", got)
	}
}

// VAL-UNDO-011: write in session 1, undo in session 2 (fresh CLI process simulation).
func TestCLIUndoPersistsAcrossRestart(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	fPath := filepath.Join(ws, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Session 1: gateway write + save (as chat cleanup would).
	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("SESSION1-AFTER")); err != nil {
		t.Fatal(err)
	}
	stackPath := writegate.PersistPath(home, ws)
	if err := g.SaveStack(stackPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stackPath); err != nil {
		t.Fatalf("stack not persisted: %v", err)
	}

	// Session 2: only CLI undo (new process loads stack from YURA_AI_HOME).
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("session2 undo exit=%d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	if got := string(mustReadFile(t, fPath)); got != "DIRTY-BEFORE" {
		t.Fatalf("session2 undo content=%q want DIRTY-BEFORE", got)
	}
	if !strings.Contains(strings.ToLower(out.String()+errBuf.String()), "pre-agent") &&
		!strings.Contains(strings.ToLower(out.String()+errBuf.String()), "undid") {
		t.Logf("undo message: %q", out.String())
	}
}

// VAL-UNDO-012 via CLI: separate workspaces, undo A does not touch B.
func TestCLIUndoWorkspaceScoped(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	wsA := filepath.Join(tmp, "wsA")
	wsB := filepath.Join(tmp, "wsB")
	for _, ws := range []string{wsA, wsB} {
		if err := os.MkdirAll(ws, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("YURA_AI_HOME", home)

	if err := os.WriteFile(filepath.Join(wsA, "f.txt"), []byte("A-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsB, "f.txt"), []byte("B-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	gA, err := writegate.New(wsA)
	if err != nil {
		t.Fatal(err)
	}
	gB, err := writegate.New(wsB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gA.WriteFile("f.txt", []byte("A-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := gB.WriteFile("f.txt", []byte("B-AFTER")); err != nil {
		t.Fatal(err)
	}
	if err := gA.SaveStack(writegate.PersistPath(home, wsA)); err != nil {
		t.Fatal(err)
	}
	if err := gB.SaveStack(writegate.PersistPath(home, wsB)); err != nil {
		t.Fatal(err)
	}
	if writegate.PersistPath(home, wsA) == writegate.PersistPath(home, wsB) {
		t.Fatal("workspace stack paths must differ")
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"undo", "--workspace", wsA}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("undo A exit=%d %s %s", code, out.String(), errBuf.String())
	}
	if got := string(mustReadFile(t, filepath.Join(wsA, "f.txt"))); got != "A-DIRTY" {
		t.Fatalf("wsA=%q", got)
	}
	if got := string(mustReadFile(t, filepath.Join(wsB, "f.txt"))); got != "B-AFTER" {
		t.Fatalf("wsB must stay AFTER after undo A: %q", got)
	}
}

// VAL-UNDO-013 via CLI: external edit after undo → redo soft-fails, does not clobber.
func TestCLIRedoSoftFailsOnExternalEdit(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	fPath := filepath.Join(ws, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	if err := g.SaveStack(writegate.PersistPath(home, ws)); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := ExecuteWith([]string{"undo", "--workspace", ws}, &out, &errBuf); code != 0 {
		t.Fatalf("undo exit=%d", code)
	}
	if string(mustReadFile(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatal("undo setup failed")
	}
	// External edit.
	if err := os.WriteFile(fPath, []byte("EXTERNAL-EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errBuf.Reset()
	code := ExecuteWith([]string{"redo", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("redo exit=%d (soft-fail should exit 0): %s %s", code, out.String(), errBuf.String())
	}
	msg := strings.ToLower(out.String() + errBuf.String())
	if string(mustReadFile(t, fPath)) == "AFTER" {
		t.Fatal("CLI redo must not clobber external edit")
	}
	if got := string(mustReadFile(t, fPath)); got != "EXTERNAL-EDIT" {
		t.Fatalf("content=%q", got)
	}
	if !(strings.Contains(msg, "external") || strings.Contains(msg, "outside") ||
		strings.Contains(msg, "skip") || strings.Contains(msg, "mismatch") ||
		strings.Contains(msg, "changed")) {
		t.Fatalf("want clear soft-fail message, got %q", out.String()+errBuf.String())
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
