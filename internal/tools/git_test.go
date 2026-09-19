package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/git"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func initGitRepo(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "initial")
	return root
}

type scriptedApprover struct {
	mu      sync.Mutex
	next    []bool
	seen    []git.ApprovalRequest
	always  *bool
	err     error
}

func (a *scriptedApprover) RequestApproval(_ context.Context, req git.ApprovalRequest) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seen = append(a.seen, req)
	if a.err != nil {
		return false, a.err
	}
	if a.always != nil {
		return *a.always, nil
	}
	if len(a.next) == 0 {
		return false, nil
	}
	v := a.next[0]
	a.next = a.next[1:]
	return v, nil
}

func agentGit(t *testing.T, root string) *git.Service {
	t.Helper()
	svc, err := git.New(git.Options{Root: root, AgentMode: true})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestGitCommitNeedsApprovalRejectAndApprove(t *testing.T) {
	root := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gs := agentGit(t, root)
	if _, err := gs.Stage("a.txt"); err != nil {
		t.Fatal(err)
	}

	// Reject first
	rej := false
	ap := &scriptedApprover{always: &rej}
	reg := NewRegistry(nil)
	reg.SetGitBackend(gs, ap)
	res := reg.Execute(context.Background(), Call{
		ID:        "1",
		Name:      NameGitCommit,
		Arguments: `{"message":"agent commit reject"}`,
	}, nil)
	if !res.IsError || !strings.Contains(res.Content, "NeedsApproval") {
		t.Fatalf("reject: %+v", res)
	}
	log, _ := gs.LogOneline(1)
	if strings.Contains(log, "agent commit reject") {
		t.Fatal("commit should not exist after reject")
	}

	// Approve
	ok := true
	ap.always = &ok
	res = reg.Execute(context.Background(), Call{
		ID:        "2",
		Name:      NameGitCommit,
		Arguments: `{"message":"agent commit approve"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("approve: %+v", res)
	}
	log, _ = gs.LogOneline(1)
	if !strings.Contains(log, "agent commit approve") {
		t.Fatalf("log: %s", log)
	}
	if len(ap.seen) < 2 || ap.seen[0].Kind != git.OpCommit {
		t.Fatalf("approvals: %+v", ap.seen)
	}
	if !gs.IsAgentMode() {
		t.Fatal("agent backend must use agent mode")
	}
}

func TestGitForcePushNeedsApproval(t *testing.T) {
	root := initGitRepo(t)
	gs := agentGit(t, root)
	rej := false
	ap := &scriptedApprover{always: &rej}
	reg := NewRegistry(nil)
	reg.SetGitBackend(gs, ap)
	res := reg.Execute(context.Background(), Call{
		ID:        "1",
		Name:      NameGitPush,
		Arguments: `{"force":true}`,
	}, nil)
	if !res.IsError || !strings.Contains(res.Content, "NeedsApproval") {
		t.Fatalf("reject force: %+v", res)
	}
	if len(ap.seen) != 1 || ap.seen[0].Kind != git.OpForcePush {
		t.Fatalf("expected force_push approval kind: %+v", ap.seen)
	}
}

func TestGitResetHardRejectLeavesTree(t *testing.T) {
	root := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gs := agentGit(t, root)
	rej := false
	ap := &scriptedApprover{always: &rej}
	reg := NewRegistry(nil)
	reg.SetGitBackend(gs, ap)
	before, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	res := reg.Execute(context.Background(), Call{
		ID:        "1",
		Name:      NameGitResetHard,
		Arguments: `{}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected reject: %+v", res)
	}
	after, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(before) != string(after) {
		t.Fatal("tree changed on cancel")
	}
	// Approve and restore
	ok := true
	ap.always = &ok
	res = reg.Execute(context.Background(), Call{
		ID:        "2",
		Name:      NameGitResetHard,
		Arguments: `{"ref":"HEAD"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("approve reset: %+v", res)
	}
	restored, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(restored) != "v1\n" {
		t.Fatalf("got %q", restored)
	}
}

func TestGitToolsInDefinitions(t *testing.T) {
	reg := NewRegistry(nil)
	names := map[string]bool{}
	for _, d := range reg.Definitions() {
		names[d.Function.Name] = true
	}
	for _, want := range []string{NameGitCommit, NameGitPush, NameGitResetHard, NameGitBranchDelete} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}
}

func TestGitStatusWithBackend(t *testing.T) {
	root := initGitRepo(t)
	gs := agentGit(t, root)
	reg := NewRegistry(nil)
	reg.SetGitBackend(gs, nil)
	res := reg.Execute(context.Background(), Call{Name: NameGitStatus, Arguments: `{}`}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	var st git.Status
	if err := json.Unmarshal([]byte(res.Content), &st); err != nil {
		t.Fatal(err)
	}
	if !st.IsRepo {
		t.Fatal("expected repo")
	}
}

func TestNoApproverDeniesGatedCommit(t *testing.T) {
	root := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gs := agentGit(t, root)
	if _, err := gs.Stage("a.txt"); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(nil)
	reg.SetGitBackend(gs, nil) // no approver → deny
	res := reg.Execute(context.Background(), Call{
		Name:      NameGitCommit,
		Arguments: `{"message":"should fail"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected deny: %+v", res)
	}
}

// Ensure git defs appear even without writegate.
func TestRegistryGitDefsWithoutWritegate(t *testing.T) {
	gs := agentGit(t, initGitRepo(t))
	reg := NewRegistry(nil)
	reg.SetGitBackend(gs, nil)
	if len(reg.Definitions()) < 5 {
		t.Fatal("expected many tool defs")
	}
	_ = writegate.ErrSandboxDenied
	_ = errors.New("ok")
}
