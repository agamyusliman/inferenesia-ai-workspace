package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-CROSS-007: pre-commit staged-secret guard unit tests.
//
// IsSecretPath is the filename classifier the guard uses. It must catch the
// common secret filename patterns (.env, *.pem, id_rsa, credentials.json, …)
// and must NOT flag ordinary source files (main.go, README.md, config.yaml).

func TestIsSecretPath_EnvAndKeyFiles(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{".envrc", true},
		{"dev.env", true},
		{"prod.env.local", true},
		{"config/.env", true},
		{"sub/dir/.env", true},

		{"private.pem", true},
		{"ca.crt", true},
		{"id_rsa", true},
		{"id_ed25519", true},
		{"id_ecdsa", true},
		{"keys/id_rsa", true},

		{"auth.json", true},
		{"token.json", true},
		{"tokens.json", true},
		{"service-account-prod.json", true},
		{"my-service-account.json", true},
		{"google-credentials.json", true},
		{"firebase-adminsdk-abc.json", true},

		{"credentials.json", true},
		{"credentials.yaml", true},
		{"secrets.yml", true},
		{"secrets.toml", true},
		{"api-keys.yaml", true},
		{"apikeys.json", true},
		{"api_keys.txt", true},

		{".netrc", true},
		{".npmrc", true},

		// Negative cases — ordinary files.
		{"main.go", false},
		{"README.md", false},
		{"config.yaml", false},
		{"docs/architecture.md", false},
		{"internal/git/secrets.go", false},
		{"AGENTS.md", false},
		{"session.md", false},
		{"package.json", false},
		{"go.mod", false},
		{"env.example", false}, // examples are tracked on purpose
		{".env.example", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := IsSecretPath(tc.path)
			if got != tc.want {
				t.Fatalf("IsSecretPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// CheckStagedSecrets must flag a staged .env and refuse to commit it,
// while leaving a non-secret staged path alone.
func TestCheckStagedSecrets_BlocksEnvAndPem(t *testing.T) {
	root := initRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TEMP_AI_API_KEY=leak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server.key"), []byte("PRIVATE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "feature.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Stage(".env", "server.key", "feature.go"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	res, err := svc.CheckStagedSecrets()
	if err != nil {
		t.Fatalf("unexpected scan error: %v", err)
	}
	if res.OK {
		t.Fatalf("expected OK=false (staged secrets present), got %+v", res)
	}
	blocked := strings.Join(res.BlockedPaths, ",")
	for _, want := range []string{".env", "server.key"} {
		if !strings.Contains(blocked, want) {
			t.Fatalf("expected blocked path %q in %s", want, blocked)
		}
	}
	// feature.go must NOT be flagged.
	if strings.Contains(blocked, "feature.go") {
		t.Fatalf("feature.go should not be flagged as secret: %s", blocked)
	}
}

func TestCheckStagedSecrets_OKWhenNoSecrets(t *testing.T) {
	root := initRepo(t)
	if err := os.WriteFile(filepath.Join(root, "feature.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Stage("feature.go"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	res, err := svc.CheckStagedSecrets()
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
}

// Commit must be REFUSED when the index holds a staged secret (VAL-CROSS-007).
// The pre-commit guard leaves the tree unchanged (the secret stays staged, but
// no commit is created), and the error must wrap ErrStagedSecrets.
func TestCommit_RefusesStagedSecrets(t *testing.T) {
	root := initRepo(t)
	if err := os.WriteFile(filepath.Join(root, "feature.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TEMP_AI_API_KEY=leak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Stage("feature.go", ".env"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	before, err := svc.LogOneline(1)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Commit("leak the secret")
	if err == nil {
		t.Fatalf("expected commit to be refused, got OK=%v", res.OK)
	}
	if !errors.Is(err, ErrStagedSecrets) {
		t.Fatalf("expected ErrStagedSecrets, got %v", err)
	}
	if res.OK {
		t.Fatalf("result OK should be false")
	}
	if !strings.Contains(strings.ToLower(res.Message), "blocked") &&
		!strings.Contains(strings.ToLower(res.Message), "secret") {
		t.Fatalf("expected blocked/secret message, got %q", res.Message)
	}
	// Tree unchanged: no new commit landed.
	after, err := svc.LogOneline(1)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("log changed after refused commit:\nbefore=%safter=%s", before, after)
	}
	// The secret is still staged (not committed); unstage + commit feature.go
	// separately must now succeed.
	if _, err := svc.Unstage(".env"); err != nil {
		t.Fatalf("unstage .env: %v", err)
	}
	res, err = svc.Commit("feature only")
	if err != nil || !res.OK {
		t.Fatalf("commit after unstage .env: %+v %v", res, err)
	}
	log, _ := svc.LogOneline(1)
	if !strings.Contains(log, "feature only") {
		t.Fatalf("expected commit message in log: %s", log)
	}
	// `git show --stat` of the new commit must NOT mention .env.
	stat, err := svc.runCapture("show", "--stat", "HEAD")
	if err != nil {
		t.Fatalf("git show --stat: %v", err)
	}
	if strings.Contains(stat, ".env") {
		t.Fatalf("commit stat mentions .env (secret leaked):\n%s", stat)
	}
}

// Commit on the initial commit (no HEAD yet) must still refuse staged secrets.
// This exercises the ls-files fallback path in CheckStagedSecrets.
func TestCommit_RefusesStagedSecrets_InitialCommit(t *testing.T) {
	root := t.TempDir()
	initRepoNoInitialCommit(t, root)
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "id_rsa"), []byte("PRIVATE KEY DATA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Stage("main.go", "id_rsa"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	res, err := svc.Commit("initial")
	if err == nil {
		t.Fatalf("expected commit refused on initial, got OK=%v", res.OK)
	}
	if !errors.Is(err, ErrStagedSecrets) {
		t.Fatalf("expected ErrStagedSecrets, got %v", err)
	}
	// Still no HEAD commit exists.
	if _, err := svc.runCapture("rev-parse", "--short", "HEAD"); err == nil {
		t.Fatal("HEAD was created despite refused commit")
	}
	// Unstage id_rsa, then commit must succeed.
	if _, err := svc.Unstage("id_rsa"); err != nil {
		t.Fatalf("unstage id_rsa: %v", err)
	}
	res, err = svc.Commit("initial clean")
	if err != nil || !res.OK {
		t.Fatalf("commit after unstage id_rsa: %+v %v", res, err)
	}
}

// initRepoNoInitialCommit sets up a git repo without an initial commit so
// `git diff --cached` needs the ls-files fallback (no HEAD yet).
func initRepoNoInitialCommit(t *testing.T, root string) {
	t.Helper()
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
}
