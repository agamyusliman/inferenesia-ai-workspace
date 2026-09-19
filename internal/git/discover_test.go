package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiscoverNestedRepos(t *testing.T) {
	ws := t.TempDir()
	// Nested project repos
	makeRepo := func(rel string) {
		t.Helper()
		root := filepath.Join(ws, rel)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "init")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v\n%s", rel, err, out)
		}
		_ = os.WriteFile(filepath.Join(root, "README.md"), []byte(rel+"\n"), 0o644)
	}
	makeRepo("cms-a")
	makeRepo("cms-b")
	// Non-repo folder should not appear alone
	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	// node_modules with fake .git should be skipped
	fake := filepath.Join(ws, "node_modules", "x")
	_ = os.MkdirAll(filepath.Join(fake, ".git"), 0o755)

	refs, err := DiscoverRepos(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("want 2 repos, got %d %+v", len(refs), refs)
	}
	ids := map[string]bool{}
	for _, r := range refs {
		ids[r.ID] = true
	}
	if !ids["cms-a"] || !ids["cms-b"] {
		t.Fatalf("ids: %+v", refs)
	}

	// Resolve
	root, err := ResolveRepoRoot(ws, "cms-a")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != "cms-a" {
		t.Fatalf("root=%s", root)
	}
}

func TestDiscoverWorkspaceRootRepo(t *testing.T) {
	ws := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = ws
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	refs, err := DiscoverRepos(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != "." {
		t.Fatalf("want single root repo, got %+v", refs)
	}
}
