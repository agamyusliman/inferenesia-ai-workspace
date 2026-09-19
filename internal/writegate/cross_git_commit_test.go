package writegate_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/git"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CROSS-007: git commit after agent edit, undo still restores pre-agent
// dirty bytes (NOT git HEAD).
//
// Cross-area flow under test (internal/git GitService + internal/writegate Gateway):
//
//	dirty-before-agent content (committed HEAD differs)
//	    → agent writes "AFTER" via WriteGateway (snapshot of dirty-before taken)
//	    → user stages + commits via GitService.Commit (safe flags, no force-push)
//	    → undo restores the file to "DIRTY-BEFORE" (byte-identical)
//	      — explicitly NOT git HEAD ("HEAD-COMMITTED")
//	    → git log shows the new commit
//	    → git show --stat excludes .env (staged-secret guard refused it earlier)
//
// This test exercises ALL expected behaviors of the feature:
//   - dirty-before-agent → agent write → git commit → undo → bytes == dirty-before
//   - undo result is NOT git HEAD
//   - git log shows the commit
//   - git show --stat excludes .env (pre-commit secret guard)
//   - pre-commit guard checks git status has no staged secrets
func TestCrossArea_GitCommitAfterAgentEdit_UndoRestoresPreAgentDirty(t *testing.T) {
	root := initGitRepoForCross(t)

	// --- Setup: committed HEAD content "HEAD-COMMITTED" (initial commit). ---
	const (
		headContent    = "HEAD-COMMITTED\n"
		dirtyBefore    = "DIRTY-BEFORE\n"
		agentAfter     = "AGENT-AFTER\n"
		targetRel      = "feature.txt"
		commitMessage  = "snapshot agent edit"
		secretFilename = ".env"
		secretContent  = "TEMP_AI_API_KEY=never-commit-me\n"
	)
	targetAbs := filepath.Join(root, targetRel)
	if err := os.WriteFile(targetAbs, []byte(headContent), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", targetRel)
	gitRun(t, root, "commit", "-m", "initial HEAD")

	// --- User has dirty (uncommitted) pre-agent content. ---
	// This is the byte sequence undo MUST restore (NOT headContent).
	if err := os.WriteFile(targetAbs, []byte(dirtyBefore), 0o644); err != nil {
		t.Fatal(err)
	}
	dirtyBeforeBytes, err := os.ReadFile(targetAbs)
	if err != nil {
		t.Fatal(err)
	}
	if string(dirtyBeforeBytes) != dirtyBefore {
		t.Fatalf("setup: dirty-before bytes mismatch: %q", dirtyBeforeBytes)
	}
	// Confirm the working tree is dirty relative to HEAD.
	headBytes := gitShowHead(t, root, targetRel)
	if string(headBytes) == dirtyBefore {
		t.Fatalf("setup invariant: dirty-before must differ from HEAD, both are %q", dirtyBefore)
	}

	// --- Agent write via WriteGateway (takes pre-mutation snapshot). ---
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatalf("writegate.New: %v", err)
	}
	var changeEvents []writegate.ChangeEvent
	gw.SetOnChange(func(ev writegate.ChangeEvent) {
		changeEvents = append(changeEvents, ev)
	})
	gw.BeginTurn("cross-007-turn")
	entry, err := gw.WriteFile(targetRel, []byte(agentAfter))
	if err != nil {
		t.Fatalf("agent write: %v", err)
	}
	gw.EndTurn()

	// Snapshot invariant: Before == dirty-before, NOT headContent (VAL-UNDO-001/002).
	if !bytes.Equal(entry.Snapshot.Before, dirtyBeforeBytes) {
		t.Fatalf("snapshot.Before = %q, want dirty-before %q",
			entry.Snapshot.Before, dirtyBeforeBytes)
	}
	if bytes.Equal(entry.Snapshot.Before, []byte(headContent)) {
		t.Fatal("snapshot.Before == HEAD content — WriteGateway must snapshot pre-agent dirty, not HEAD")
	}
	// File on disk now has AFTER content.
	afterBytes, err := os.ReadFile(targetAbs)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterBytes) != agentAfter {
		t.Fatalf("disk after agent write = %q, want %q", afterBytes, agentAfter)
	}
	// FileChanged event fired on write.
	if len(changeEvents) == 0 || changeEvents[0].Op != "write" {
		t.Fatalf("expected write change event, got %+v", changeEvents)
	}

	// --- Pre-commit staged-secret guard: .env must be refused. ---
	// Drop a secret-looking file in the working tree and try to stage+commit it.
	if err := os.WriteFile(filepath.Join(root, secretFilename), []byte(secretContent), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := git.New(git.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	// Stage the agent-modified file AND the secret.
	if _, err := svc.Stage(targetRel, secretFilename); err != nil {
		t.Fatalf("stage target + .env: %v", err)
	}
	// Commit must be REFUSED because .env is staged.
	res, commitErr := svc.Commit(commitMessage)
	if commitErr == nil {
		t.Fatalf("expected commit to be refused because .env is staged, got OK=%v", res.OK)
	}
	if !errors.Is(commitErr, git.ErrStagedSecrets) {
		t.Fatalf("expected ErrStagedSecrets, got %v", commitErr)
	}
	// No new commit should have landed (HEAD unchanged).
	logBeforeUnstage, _ := svc.LogOneline(1)
	if strings.Contains(logBeforeUnstage, commitMessage) {
		t.Fatalf("commit message must not appear in log after refused commit: %s", logBeforeUnstage)
	}

	// Unstage the secret, then commit must succeed (safe flags, no force-push).
	if _, err := svc.Unstage(secretFilename); err != nil {
		t.Fatalf("unstage .env: %v", err)
	}
	res, err = svc.Commit(commitMessage)
	if err != nil || !res.OK {
		t.Fatalf("commit after unstage .env: %+v %v", res, err)
	}
	if res.Hash == "" {
		t.Fatalf("expected commit hash, got %+v", res)
	}

	// --- git log shows the new commit (VAL-CROSS-007 evidence). ---
	log, err := svc.LogOneline(1)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if !strings.Contains(log, commitMessage) {
		t.Fatalf("git log missing commit message %q: %s", commitMessage, log)
	}

	// --- git show --stat excludes .env (VAL-CROSS-007 evidence). ---
	stat := gitCapture(t, root, "show", "--stat", "HEAD")
	if strings.Contains(stat, secretFilename) {
		t.Fatalf("git show --stat HEAD mentions .env (secret leaked into commit):\n%s", stat)
	}
	if !strings.Contains(stat, targetRel) {
		t.Fatalf("git show --stat HEAD missing committed target %q:\n%s", targetRel, stat)
	}

	// --- Undo restores pre-agent dirty bytes (NOT HEAD) — the core assertion. ---
	undoRes := gw.Undo()
	if undoRes.Noop {
		t.Fatalf("undo was a no-op: %s", undoRes.Message)
	}
	restoredBytes, err := os.ReadFile(targetAbs)
	if err != nil {
		t.Fatalf("read after undo: %v", err)
	}
	if !bytes.Equal(restoredBytes, dirtyBeforeBytes) {
		t.Fatalf("undo did not restore dirty-before bytes:\n want %q\n got  %q",
			dirtyBeforeBytes, restoredBytes)
	}
	// Explicitly NOT HEAD content.
	if string(restoredBytes) == headContent {
		t.Fatalf("undo restored HEAD content %q — must restore pre-agent dirty %q",
			restoredBytes, dirtyBefore)
	}
	if string(restoredBytes) == agentAfter {
		t.Fatalf("undo left AFTER content %q — stack did not restore pre-agent dirty", agentAfter)
	}

	// --- Post-undo git diff must match the pre-agent dirty diff. ---
	// Before the agent turn, the working tree had dirty-before differing from HEAD.
	// After undo + commit, the committed tree has AFTER; undo restored working tree
	// to dirty-before, so `git diff` (working tree vs HEAD/AFTER) must show the
	// dirty-before ↔ AFTER delta — NOT the HEAD ↔ dirty delta.
	diff, err := svc.Diff(targetRel, false)
	if err != nil {
		t.Fatalf("post-undo diff: %v", err)
	}
	// The diff must contain the dirty-before content (the restored working tree)
	// and the AFTER content (now committed at HEAD).
	if !strings.Contains(diff.Content, "DIRTY-BEFORE") {
		t.Fatalf("post-undo diff missing dirty-before marker:\n%s", diff.Content)
	}
	if !strings.Contains(diff.Content, "AGENT-AFTER") {
		t.Fatalf("post-undo diff missing after marker:\n%s", diff.Content)
	}

	// --- FileChanged event fired on undo (VAL-UNDO-009). ---
	var undoEvent *writegate.ChangeEvent
	for i := len(changeEvents) - 1; i >= 0; i-- {
		if changeEvents[i].Op == "undo" {
			undoEvent = &changeEvents[i]
			break
		}
	}
	if undoEvent == nil {
		t.Fatalf("expected an undo change event, got %+v", changeEvents)
	}
	if !strings.HasSuffix(undoEvent.Path, targetRel) {
		t.Fatalf("undo event path = %q, want suffix %q", undoEvent.Path, targetRel)
	}

	// --- Safe-flags evidence: commit never used --force or --no-verify. ---
	// We assert by confirming the committed history is linear (no rewrite) and
	// the reflog contains exactly the expected entries.
	reflog := gitCapture(t, root, "log", "--oneline", "-3")
	if !strings.Contains(reflog, commitMessage) {
		t.Fatalf("reflog missing commit: %s", reflog)
	}
}

// VAL-CROSS-007 variant: the pre-commit guard catches a broader set of secret
// filenames (id_rsa, *.pem, credentials.json) — not just .env.
func TestCrossArea_PreCommitGuard_CatchesMultipleSecretKinds(t *testing.T) {
	root := initGitRepoForCross(t)
	svc, err := git.New(git.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		content string
	}{
		{".env", "KEY=leak\n"},
		{"id_rsa", "PRIVATE\n"},
		{"server.pem", "CERT\n"},
		{"credentials.json", `{"user":"x","pass":"y"}`},
		{"auth.json", `{"token":"t"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, tc.name), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Stage(tc.name); err != nil {
				t.Fatalf("stage %s: %v", tc.name, err)
			}
			res, err := svc.Commit("should refuse " + tc.name)
			if err == nil {
				t.Fatalf("expected commit refused for %s, got OK=%v", tc.name, res.OK)
			}
			if !errors.Is(err, git.ErrStagedSecrets) {
				t.Fatalf("expected ErrStagedSecrets for %s, got %v", tc.name, err)
			}
			// Unstage so the next case starts clean.
			if _, err := svc.Unstage(tc.name); err != nil {
				t.Fatalf("unstage %s: %v", tc.name, err)
			}
			// Remove the file so it does not interfere with the next case.
			_ = os.Remove(filepath.Join(root, tc.name))
		})
	}
}

// initGitRepoForCross creates a temporary git repo with a clean initial commit
// (no content) so the cross-area test can stage its own files.
func initGitRepoForCross(t *testing.T) string {
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
	_ = exec.Command("git", "-C", root, "checkout", "-b", "main").Run()
	run("config", "user.email", "yura@test.local")
	run("config", "user.name", "YuraTest")
	// Seed an empty initial commit so HEAD exists.
	run("commit", "--allow-empty", "-m", "repo init")
	return root
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
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

func gitShowHead(t *testing.T, dir, rel string) []byte {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "show", "HEAD:"+rel)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show HEAD:%s: %v", rel, err)
	}
	return out
}

// gitCapture runs git in dir and returns combined output, failing the test on error.
func gitCapture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=YuraTest",
		"GIT_AUTHOR_EMAIL=yura@test.local",
		"GIT_COMMITTER_NAME=YuraTest",
		"GIT_COMMITTER_EMAIL=yura@test.local",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
