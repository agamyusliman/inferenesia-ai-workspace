package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitWorkspace(t *testing.T) (svc *Service, root string) {
	t.Helper()
	home := t.TempDir()
	root = filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
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
	_ = exec.Command("git", "-C", root, "checkout", "-b", "main").Run()
	run("config", "user.email", "yura@test.local")
	run("config", "user.name", "YuraTest")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-m", "initial")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	return svc, root
}

func TestGitStatusDiffStageCommit(t *testing.T) {
	svc, root := setupGitWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := svc.GetGitStatus("")
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsRepo {
		t.Fatal("expected repo")
	}
	if st.UnstagedCount < 1 {
		t.Fatalf("expected unstaged: %+v", st)
	}
	// Diff
	diff, err := svc.GetGitDiff("", "f.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Empty || !strings.Contains(diff.Content, "changed") {
		t.Fatalf("bad diff: %+v", diff)
	}
	// Stage
	res, err := svc.GitStage(GitStageRequest{Path: "f.txt"})
	if err != nil || !res.OK {
		t.Fatalf("stage: %+v %v", res, err)
	}
	st, err = svc.GetGitStatus("")
	if err != nil {
		t.Fatal(err)
	}
	if st.StagedCount < 1 {
		t.Fatalf("expected staged: %+v", st)
	}
	// Unstage
	res, err = svc.GitUnstage(GitStageRequest{Path: "f.txt"})
	if err != nil || !res.OK {
		t.Fatalf("unstage: %+v %v", res, err)
	}
	// Stage + commit
	if _, err := svc.GitStage(GitStageRequest{Path: "f.txt"}); err != nil {
		t.Fatal(err)
	}
	cres, err := svc.GitCommit(GitCommitRequest{Message: "core commit"})
	if err != nil || !cres.OK {
		t.Fatalf("commit: %+v %v", cres, err)
	}
	logOut, err := exec.Command("git", "-C", root, "log", "--oneline", "-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logOut), "core commit") {
		t.Fatalf("log: %s", logOut)
	}
	st, err = svc.GetGitStatus("")
	if err != nil {
		t.Fatal(err)
	}
	if st.StagedCount != 0 {
		t.Fatalf("still staged: %+v", st)
	}
	// Graph has at least the commits we made
	g, err := svc.GetGitGraph("", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Commits) < 1 {
		t.Fatalf("graph empty: %+v", g)
	}
}

func TestGitFetchPullFailureClear(t *testing.T) {
	// Fresh local repo with no remote: fetch/pull should fail with clear message, not panic.
	svc, _ := setupGitWorkspace(t)
	fres, err := svc.GitFetch("")
	// err may be non-nil; result must mark not ok with message.
	if fres.OK {
		// Some git versions may still "succeed" with no remote configured differently;
		// only assert we got a coherent result struct.
		t.Logf("fetch ok unexpectedly: %+v", fres)
	} else if fres.Message == "" {
		t.Fatalf("fetch failure without message: %+v err=%v", fres, err)
	}
	pres, err := svc.GitPull("")
	if pres.OK {
		t.Logf("pull ok unexpectedly: %+v", pres)
	} else if pres.Message == "" {
		t.Fatalf("pull failure without message: %+v err=%v", pres, err)
	}
}

func TestGitMultiRepoDiscovery(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	// two nested projects
	for _, name := range []string{"cms-a", "cms-b"} {
		p := filepath.Join(ws, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "init")
		cmd.Dir = p
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		_ = os.WriteFile(filepath.Join(p, "x.txt"), []byte(name), 0o644)
	}
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	repos, err := svc.ListGitRepos()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %+v", repos)
	}
	stA, err := svc.GetGitStatus("cms-a")
	if err != nil {
		t.Fatal(err)
	}
	if stA.RepoID != "cms-a" || !stA.IsRepo {
		t.Fatalf("cms-a status: %+v", stA)
	}
	// Dirty only on cms-b
	if err := os.WriteFile(filepath.Join(ws, "cms-b", "x.txt"), []byte("dirty-b"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Need a first commit for meaningful status on cms-b — init only may show untracked
	stB, err := svc.GetGitStatus("cms-b")
	if err != nil {
		t.Fatal(err)
	}
	if stB.UnstagedCount+stB.UntrackedCount < 1 {
		t.Fatalf("expected dirty cms-b: %+v", stB)
	}
}

func TestGitPushRequiresConfirmation(t *testing.T) {
	svc, _ := setupGitWorkspace(t)
	// Without confirmed → blocked (VAL-GIT-011).
	res, err := svc.GitPush(GitPushRequest{Confirmed: false})
	if res.OK || err == nil {
		t.Fatalf("unconfirmed push should fail: %+v %v", res, err)
	}
	if !strings.Contains(res.Message, "confirm") && !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("message: %s err=%v", res.Message, err)
	}
	// Force without confirm blocked harder.
	res, _ = svc.GitPush(GitPushRequest{Confirmed: false, Force: true})
	if res.OK {
		t.Fatal("force push without confirm must not succeed")
	}
	if !strings.Contains(res.Message, "force") && !strings.Contains(res.Message, "approval") && !strings.Contains(res.Message, "confirm") {
		t.Fatalf("force message: %s", res.Message)
	}
}

func TestGitDestructiveRequiresConfirmation(t *testing.T) {
	svc, root := setupGitWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	res, err := svc.GitDestructive(GitDestructiveRequest{
		Kind:      "reset_hard",
		Confirmed: false,
	})
	if res.OK || err == nil {
		t.Fatalf("expected block: %+v %v", res, err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if string(before) != string(after) {
		t.Fatal("tree changed without confirm")
	}
	res, err = svc.GitDestructive(GitDestructiveRequest{
		Kind:      "reset_hard",
		Ref:       "HEAD",
		Confirmed: true,
	})
	if err != nil || !res.OK {
		t.Fatalf("approved: %+v %v", res, err)
	}
	restored, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if string(restored) != "base\n" {
		t.Fatalf("want base, got %q", restored)
	}
}

func TestResolveApprovalUnknownID(t *testing.T) {
	svc, _ := setupGitWorkspace(t)
	err := svc.ResolveApproval(ApprovalDecision{ID: "nope", Approved: true})
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
	if p := svc.GetPendingApproval(); p != nil {
		t.Fatalf("expected no pending: %+v", p)
	}
}
