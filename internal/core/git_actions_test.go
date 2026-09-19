package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetGitActions_NonGitHubRemote(t *testing.T) {
	svc, root := setupGitWorkspace(t)
	// Replace origin with gitlab
	_ = exec.Command("git", "-C", root, "remote", "remove", "origin").Run()
	cmd := exec.Command("git", "-C", root, "remote", "add", "origin", "https://gitlab.com/acme/app.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		// fresh fixture may have no origin
		_ = out
		if err2 := exec.Command("git", "-C", root, "remote", "add", "origin", "https://gitlab.com/acme/app.git").Run(); err2 != nil {
			// try set-url if existed
			_ = exec.Command("git", "-C", root, "remote", "set-url", "origin", "https://gitlab.com/acme/app.git").Run()
		}
	}
	// ensure remote
	if out, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").CombinedOutput(); err != nil {
		t.Fatalf("remote: %v %s", err, out)
	}

	res, err := svc.GetGitActions("", 10)
	if err != nil {
		// may still return structured body
		t.Logf("err=%v", err)
	}
	if res.Availability != "not_github" {
		t.Fatalf("want not_github got %+v", res)
	}
	blob, _ := json.Marshal(res)
	if strings.Contains(strings.ToLower(string(blob)), "gho_") ||
		strings.Contains(string(blob), "GITHUB_TOKEN=") {
		t.Fatalf("token-like material: %s", blob)
	}
}

func TestGetGitActions_NoRemote(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@t.local",
			"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@t.local",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t.local")
	run("config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-m", "i")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(root); err != nil {
		t.Fatal(err)
	}
	res, _ := svc.GetGitActions("", 5)
	if res.Availability != "no_remote" {
		t.Fatalf("want no_remote got %s msg=%s", res.Availability, res.Message)
	}
	if res.OK {
		t.Fatal("should not be ok")
	}
}

func TestOpenGitActionsRun_RejectNonGitHub(t *testing.T) {
	svc, _ := setupGitWorkspace(t)
	_, err := svc.OpenGitActionsRun("https://evil.example/x")
	if err == nil {
		t.Fatal("expected reject")
	}
}
