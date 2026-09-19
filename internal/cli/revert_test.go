package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/memory"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-MEM-007 + VAL-CROSS-013 via CLI: dirty → agent (+ memory) → revert last turn.
func TestCLIRevertLastTurnMemoryAndFiles(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

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

	// Agent turn: edit f.txt + memory append (same turn via BeginTurn).
	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("cli-revert-turn")
	if _, err := g.WriteFile("f.txt", []byte("AGENT-AFTER")); err != nil {
		t.Fatal(err)
	}
	_, err = memory.Append(g, ws, memory.Record{
		Agent:   "Droid",
		Summary: "cli revert test entry",
		Bucket:  "agents",
		Decision: "test decision for context",
		Topic:   "revert-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	g.EndTurn()
	stackPath := writegate.PersistPath(home, ws)
	if err := g.SaveStack(stackPath); err != nil {
		t.Fatal(err)
	}

	// Snapshot memory after turn for comparison.
	bucketAfter := mustReadFile(t, filepath.Join(ws, ".multibrain", "indexes", "agents.md"))
	if !strings.Contains(string(bucketAfter), "cli revert test entry") {
		t.Fatal("memory entry missing after append")
	}
	// Find context file created.
	ctxDir := filepath.Join(ws, ".multibrain", "context")
	ctxEntries, _ := os.ReadDir(ctxDir)
	if len(ctxEntries) == 0 {
		t.Fatal("expected context file from decision")
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"revert", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("revert exit=%d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	msg := out.String()
	if !strings.Contains(msg, "reverted last agent turn") && !strings.Contains(msg, "pre-agent") {
		// Accept either revert or undo-style wording.
		if !strings.Contains(msg, "undid") {
			t.Fatalf("unexpected revert message: %q", msg)
		}
	}

	// File = dirty, not HEAD.
	if string(mustReadFile(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("after revert content = %q", mustReadFile(t, fPath))
	}
	// Memory roll-back: entry gone. Bucket/context may be restored to pre-turn
	// content or removed entirely if they were created in the turn (pre-absent).
	bucketPath := filepath.Join(ws, ".multibrain", "indexes", "agents.md")
	if data, err := os.ReadFile(bucketPath); err == nil {
		if strings.Contains(string(data), "cli revert test entry") {
			t.Fatalf("memory entry still present after revert:\n%s", data)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("read bucket: %v", err)
	}
	// Context from the turn should be gone (created in-turn → undo removes file).
	if entries, err := os.ReadDir(ctxDir); err == nil {
		for _, e := range entries {
			body, _ := os.ReadFile(filepath.Join(ctxDir, e.Name()))
			if strings.Contains(string(body), "test decision for context") {
				t.Fatalf("context from turn still present: %s", e.Name())
			}
		}
	}
}

// VAL-MEM-008 CLI: revert <file> only that file; unchanged → clear no-op.
func TestCLIRevertFile(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("A-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "b.txt"), []byte("B-DIRTY"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("t")
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
	code := ExecuteWith([]string{"revert", "a.txt", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit=%d %q %q", code, out.String(), errBuf.String())
	}
	if string(mustReadFile(t, filepath.Join(ws, "a.txt"))) != "A-DIRTY" {
		t.Fatal("a not restored")
	}
	if string(mustReadFile(t, filepath.Join(ws, "b.txt"))) != "B-AFTER" {
		t.Fatal("b should be unchanged")
	}

	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"revert", "never-touched.txt", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("noop revert exit=%d", code)
	}
	if !strings.Contains(out.String(), "not changed") {
		t.Fatalf("clear noop message missing: %q", out.String())
	}
}

// VAL-MEM-012 CLI: history + undo list empty and non-empty.
func TestCLIHistoryEmptyAndList(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YURA_AI_HOME", home)

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"history", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("history empty exit=%d %q", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "no agent changes") && !strings.Contains(out.String(), "fresh workspace") {
		t.Fatalf("empty history message: %q", out.String())
	}

	g, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("x.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := g.SaveStack(writegate.PersistPath(home, ws)); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"history", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("history exit=%d", code)
	}
	if !strings.Contains(out.String(), "x.txt") {
		t.Fatalf("history missing path: %q", out.String())
	}

	// undo list alias
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"undo", "list", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("undo list exit=%d %q", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "x.txt") {
		t.Fatalf("undo list missing path: %q", out.String())
	}
}
