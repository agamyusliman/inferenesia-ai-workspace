package writegate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := g.WriteFile("hello.txt", []byte("hi"))
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if entry.Snapshot.Rel != "hello.txt" {
		t.Fatalf("rel = %q", entry.Snapshot.Rel)
	}
	if entry.Snapshot.Existed {
		t.Fatal("new file should not have Existed")
	}
	data, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hi" {
		t.Fatalf("content = %q", data)
	}
	if len(g.Entries) != 1 {
		t.Fatalf("entries = %d", len(g.Entries))
	}
}

func TestWriteFileSnapshotsDirtyBefore(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	if err := os.WriteFile(path, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := g.WriteFile("f.txt", []byte("AFTER"))
	if err != nil {
		t.Fatal(err)
	}
	if string(entry.Snapshot.Before) != "DIRTY-BEFORE" {
		t.Fatalf("before = %q", entry.Snapshot.Before)
	}
	if !entry.Snapshot.Existed {
		t.Fatal("expected existed")
	}
	if string(entry.After) != "AFTER" {
		t.Fatalf("after = %q", entry.After)
	}
}

func TestSandboxDeniesAbsoluteOutside(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-x.txt")
	_, err = g.WriteFile(outside, []byte("nope"))
	if !IsSandboxDenied(err) {
		t.Fatalf("want sandbox denied, got %v", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside file must not exist, err=%v", err)
	}
}

func TestSandboxDeniesParentTraversal(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	// Create a marker outside sibling of root to detect escape.
	parent := filepath.Dir(root)
	marker := filepath.Join(parent, "escape-marker-should-not-exist-"+filepath.Base(root)+".txt")
	defer os.Remove(marker) // safety cleanup if written

	rel := ".." + string(os.PathSeparator) + filepath.Base(marker)
	// Use a path that walks out via ..
	escapePath := filepath.Join("subdir", "..", "..", filepath.Base(marker))
	_, err = g.WriteFile(escapePath, []byte("escaped"))
	if !IsSandboxDenied(err) {
		// On some roots TempDir is deep enough that .. stays under a shared tmp —
		// still must not write outside *workspace* root.
		if err == nil {
			t.Fatal("escape path must be denied")
		}
		// If not sandboxed but error for other reason, still check marker not under parent escape intent.
		t.Fatalf("want sandbox denied, got %v", err)
	}
	// The explicit outside absolute is the stronger check; also try double-dot relative.
	_, err = g.WriteFile("../outside-relative.txt", []byte("nope"))
	if !IsSandboxDenied(err) {
		t.Fatalf("want sandbox denied for ../, got %v", err)
	}
	_ = rel
}

func TestSandboxDeniesTmpOutside(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(os.TempDir(), "yura-ai-sandbox-test-outside-"+filepath.Base(root), "x.txt")
	_, err = g.WriteFile(target, []byte("nope"))
	if !IsSandboxDenied(err) {
		t.Fatalf("want sandbox denied, got %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("file must not be created outside workspace")
	}
}

func TestResolveNestedRelative(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	abs, rel, err := g.ResolveInWorkspace("a/b/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "a/b/c.txt" {
		t.Fatalf("rel=%q", rel)
	}
	// On macOS, TempDir may be /var/... while EvalSymlinks yields /private/var/....
	// Compare against the gateway's evaluated Root.
	if !strings.HasPrefix(abs, g.Root) {
		t.Fatalf("abs=%q not under gateway root %q", abs, g.Root)
	}
	_, err = g.WriteFile("a/b/c.txt", []byte("ok"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteNestedCreatesDirs(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteFile("pkg/sub/f.go", []byte("package sub\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "pkg", "sub", "f.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "package sub") {
		t.Fatalf("content=%q", data)
	}
}
