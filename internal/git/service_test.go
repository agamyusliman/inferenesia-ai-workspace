package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=YuraTest",
			"GIT_AUTHOR_EMAIL=yura@test.local",
			"GIT_COMMITTER_NAME=YuraTest",
			"GIT_COMMITTER_EMAIL=yura@test.local",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	// Ensure main branch for consistency.
	_ = exec.Command("git", "-C", root, "checkout", "-b", "main").Run()
	run("config", "user.email", "yura@test.local")
	run("config", "user.name", "YuraTest")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "initial")
	return root
}

func TestStatusMatchesPorcelain(t *testing.T) {
	root := initRepo(t)
	// Dirty modify + untracked.
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("v2-dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	st, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsRepo {
		t.Fatal("expected is_repo")
	}
	if st.Branch != "main" && !strings.Contains(st.Branch, "main") {
		// Some systems leave default master; either is fine if non-empty.
		if st.Branch == "" {
			t.Fatal("empty branch")
		}
	}
	raw, err := exec.Command("git", "-C", root, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(st.Porcelain) != strings.TrimSpace(string(raw)) {
		t.Fatalf("porcelain mismatch\nwant:\n%s\ngot:\n%s", raw, st.Porcelain)
	}
	if st.UnstagedCount < 1 {
		t.Fatalf("expected unstaged, got %+v", st)
	}
	if st.UntrackedCount < 1 {
		t.Fatalf("expected untracked, got %+v", st)
	}
	// Find a.txt modified.
	foundMod, foundNew := false, false
	for _, f := range st.Files {
		if f.Path == "a.txt" && f.Unstaged {
			foundMod = true
		}
		if f.Path == "new.txt" && f.Untracked {
			foundNew = true
		}
	}
	if !foundMod || !foundNew {
		t.Fatalf("files incomplete: %+v", st.Files)
	}
}

func TestStageUnstageCommit(t *testing.T) {
	root := initRepo(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	// Stage
	res, err := svc.Stage("a.txt")
	if err != nil || !res.OK {
		t.Fatalf("stage: %+v %v", res, err)
	}
	st, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.StagedCount < 1 {
		t.Fatalf("expected staged after stage: %+v", st)
	}
	// Unstage
	res, err = svc.Unstage("a.txt")
	if err != nil || !res.OK {
		t.Fatalf("unstage: %+v %v", res, err)
	}
	st, err = svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.StagedCount != 0 {
		t.Fatalf("expected no staged after unstage: %+v", st)
	}
	if st.UnstagedCount < 1 {
		t.Fatalf("expected unstaged after unstage: %+v", st)
	}
	// Stage + commit
	if _, err := svc.Stage("a.txt"); err != nil {
		t.Fatal(err)
	}
	cres, err := svc.Commit("panel commit")
	if err != nil || !cres.OK {
		t.Fatalf("commit: %+v %v", cres, err)
	}
	if cres.Hash == "" {
		t.Fatal("expected commit hash")
	}
	log, err := svc.LogOneline(1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "panel commit") {
		t.Fatalf("log missing message: %s", log)
	}
	st, err = svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	// Committed change should not remain staged.
	if st.StagedCount != 0 {
		t.Fatalf("staged after commit: %+v", st)
	}
}

func TestDiffMatchesGitDiff(t *testing.T) {
	root := initRepo(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("line-a\nline-b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	diff, err := svc.Diff("a.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Empty {
		t.Fatal("expected non-empty diff")
	}
	raw, err := exec.Command("git", "-C", root, "diff", "--", "a.txt").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	// Compare content ignoring optional prefix flags — core content must match.
	if strings.TrimSpace(diff.Content) != strings.TrimSpace(string(raw)) {
		// Our Diff also sets --no-ext-diff --no-color; match those.
		raw2, err := exec.Command("git", "-C", root, "diff", "--no-ext-diff", "--no-color", "--", "a.txt").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(diff.Content) != strings.TrimSpace(string(raw2)) {
			t.Fatalf("diff mismatch\nwant:\n%s\ngot:\n%s", raw2, diff.Content)
		}
	}
	if !strings.Contains(diff.Content, "+line-a") && !strings.Contains(diff.Content, "line-a") {
		t.Fatalf("diff missing addition: %s", diff.Content)
	}
}

func TestParsePorcelain(t *testing.T) {
	in := " M a.txt\nM  b.txt\n?? c.txt\nR  old.txt -> new.txt\n"
	files := parsePorcelain(in)
	if len(files) != 4 {
		t.Fatalf("len=%d %+v", len(files), files)
	}
	if !files[0].Unstaged || files[0].Staged {
		t.Fatalf("a.txt: %+v", files[0])
	}
	if !files[1].Staged || files[1].Unstaged {
		t.Fatalf("b.txt: %+v", files[1])
	}
	if !files[2].Untracked {
		t.Fatalf("c.txt: %+v", files[2])
	}
	if files[3].Path != "new.txt" || files[3].Original != "old.txt" {
		t.Fatalf("rename: %+v", files[3])
	}
}

func TestSafeFlagsPresent(t *testing.T) {
	svc, err := New(Options{Root: t.TempDir(), AgentMode: true})
	if err != nil {
		t.Fatal(err)
	}
	args := svc.prefixArgs("status", "--porcelain")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--no-optional-locks", "core.autocrlf=false", "core.fsmonitor=false", "core.hooksPath=/dev/null"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing safe flag %q in %v", want, args)
		}
	}
}
