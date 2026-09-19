package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestForcePushRejectedWithoutApproval(t *testing.T) {
	root := initRepo(t)
	svc, err := New(Options{Root: root, AgentMode: true})
	if err != nil {
		t.Fatal(err)
	}
	// No remote needed — gate fires before network.
	res, err := svc.Push(PushOptions{Force: true, Approved: false})
	if err == nil || res.OK {
		t.Fatalf("expected force-push reject, got res=%+v err=%v", res, err)
	}
	if !errors.Is(err, ErrForcePushBlocked) && !strings.Contains(err.Error(), "approval") {
		t.Fatalf("want ErrForcePushBlocked, got %v", err)
	}
	if !strings.Contains(res.Message, "approval") {
		t.Fatalf("message: %s", res.Message)
	}

	res, err = svc.Push(PushOptions{ForceWithLease: true, Approved: false})
	if err == nil || res.OK {
		t.Fatalf("expected force-with-lease reject, got res=%+v err=%v", res, err)
	}
}

func TestPushRequireConfirmWithoutApproval(t *testing.T) {
	root := initRepo(t)
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Push(PushOptions{RequireConfirm: true, Approved: false})
	if err == nil || res.OK {
		t.Fatalf("expected confirm reject: %+v %v", res, err)
	}
	if !errors.Is(err, ErrApprovalRequired) && !strings.Contains(err.Error(), "approval") {
		t.Fatalf("want ErrApprovalRequired, got %v", err)
	}
}

func TestDestructiveResetHardRequiresApproval(t *testing.T) {
	root := initRepo(t)
	// Dirty file
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	res, err := svc.ResetHard(DestructiveOptions{Approved: false})
	if err == nil || res.OK {
		t.Fatalf("expected block: %+v %v", res, err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(before) != string(after) {
		t.Fatal("tree changed on rejected reset --hard")
	}

	res, err = svc.ResetHard(DestructiveOptions{Approved: true, Ref: "HEAD"})
	if err != nil || !res.OK {
		t.Fatalf("approved reset: %+v %v", res, err)
	}
	restored, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(restored) != "v1\n" {
		t.Fatalf("want restored v1, got %q", restored)
	}
}

func TestDeleteBranchRequiresApproval(t *testing.T) {
	root := initRepo(t)
	// Create side branch
	cmd := exec.Command("git", "branch", "feature-x")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=YuraTest",
		"GIT_AUTHOR_EMAIL=yura@test.local",
		"GIT_COMMITTER_NAME=YuraTest",
		"GIT_COMMITTER_EMAIL=yura@test.local",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("branch: %v %s", err, out)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.DeleteBranch(DestructiveOptions{Ref: "feature-x", Approved: false})
	if err == nil || res.OK {
		t.Fatalf("expected block: %+v %v", res, err)
	}
	// Branch still exists
	out, _ := exec.Command("git", "-C", root, "branch", "--list", "feature-x").CombinedOutput()
	if !strings.Contains(string(out), "feature-x") {
		t.Fatalf("branch deleted without approval: %s", out)
	}

	res, err = svc.DeleteBranch(DestructiveOptions{Ref: "feature-x", Approved: true})
	if err != nil || !res.OK {
		t.Fatalf("approved delete: %+v %v", res, err)
	}
	out, _ = exec.Command("git", "-C", root, "branch", "--list", "feature-x").CombinedOutput()
	if strings.Contains(string(out), "feature-x") {
		t.Fatalf("branch still exists: %s", out)
	}
}

func TestAgentCommitRequiresApproval(t *testing.T) {
	root := initRepo(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root, AgentMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Stage("a.txt"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.CommitAgent("agent commit", false)
	if err == nil || res.OK {
		t.Fatalf("expected reject: %+v %v", res, err)
	}
	// No new commit
	log, _ := svc.LogOneline(1)
	if strings.Contains(log, "agent commit") {
		t.Fatalf("commit landed without approval: %s", log)
	}

	res, err = svc.CommitAgent("agent commit", true)
	if err != nil || !res.OK {
		t.Fatalf("approved commit: %+v %v", res, err)
	}
	log, _ = svc.LogOneline(1)
	if !strings.Contains(log, "agent commit") {
		t.Fatalf("log: %s", log)
	}
}

func TestSandboxRejectsOutsideWorkspacePaths(t *testing.T) {
	root := t.TempDir()
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../etc/passwd", "../../secret", "/etc/passwd", `C:\Windows\System32`} {
		if err := svc.ValidateSandboxPath(p); err == nil {
			t.Fatalf("expected reject for %q", p)
		}
	}
	if err := svc.ValidateSandboxPath("ok/file.go"); err != nil {
		t.Fatalf("relative path should pass: %v", err)
	}
	// Stage must also reject .. via cleanPaths
	_, err = svc.Stage("../escape.txt")
	if err == nil {
		t.Fatal("Stage should reject path escape")
	}
}

func TestAgentHooksPathIsolation(t *testing.T) {
	root := t.TempDir()
	agent, err := New(Options{Root: root, AgentMode: true})
	if err != nil {
		t.Fatal(err)
	}
	args := agent.PrefixArgsForTest("commit", "-m", "x")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "core.hooksPath=/dev/null") {
		t.Fatalf("agent must set hooksPath=/dev/null: %v", args)
	}
	// Safe flags always present
	for _, want := range []string{"--no-optional-locks", "core.autocrlf=false", "core.fsmonitor=false"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
	// UI mode (non-agent) must NOT force hooksPath null (user hooks allowed for human UI).
	ui, err := New(Options{Root: root, AgentMode: false})
	if err != nil {
		t.Fatal(err)
	}
	uiArgs := strings.Join(ui.PrefixArgsForTest("status"), " ")
	if strings.Contains(uiArgs, "core.hooksPath=/dev/null") {
		t.Fatalf("UI mode should not null hooksPath: %s", uiArgs)
	}
	if !ui.IsAgentMode() == false {
		// just exercise API
	}
	if !agent.IsAgentMode() {
		t.Fatal("expected agent mode")
	}
}

func TestDefaultPushIsNonForce(t *testing.T) {
	// Unit: Push with Approved for confirm but Force=false never includes --force in args path.
	// We assert by checking that unapproved force fails and approved non-force tries `git push origin`
	// (will fail without remote, but Detail must not claim force and error path is non-force).
	root := initRepo(t)
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Push(PushOptions{Approved: true, RequireConfirm: true})
	// No remote → push fails, but must not be force-gated error
	if errors.Is(err, ErrForcePushBlocked) {
		t.Fatal("non-force push must not use force path")
	}
	if res.OK {
		t.Fatal("expected push failure without remote")
	}
	if strings.Contains(res.Message, "force-push") {
		t.Fatalf("default push message should not be force: %s", res.Message)
	}
}
