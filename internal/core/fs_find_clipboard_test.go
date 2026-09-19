package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestFindInFolderScoped(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	mustMkdir(t, root)
	mustMkdir(t, filepath.Join(root, "docs"))
	mustMkdir(t, filepath.Join(root, "other"))
	mustWrite(t, filepath.Join(root, "docs", "readme.md"), "hello find-token in docs\n")
	mustWrite(t, filepath.Join(root, "other", "secret.md"), "hello find-token outside\n")
	mustWrite(t, filepath.Join(root, "docs", "note.txt"), "no match here\n")
	// Path match under docs
	mustWrite(t, filepath.Join(root, "docs", "find-token-name.go"), "package main\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	hits, err := svc.FindInFolder(ws.ID, "docs", "find-token", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit under docs")
	}
	for _, h := range hits {
		if !strings.HasPrefix(h.Path, "docs/") && h.Path != "docs" {
			t.Fatalf("result outside folder scope: %q", h.Path)
		}
		if strings.HasPrefix(h.Path, "other/") {
			t.Fatalf("negative: other/* must not appear, got %q", h.Path)
		}
	}
	// Content hit for readme
	foundContent := false
	foundPath := false
	for _, h := range hits {
		if h.Path == "docs/readme.md" && h.Kind == "content" {
			foundContent = true
		}
		if h.Path == "docs/find-token-name.go" && h.Kind == "path" {
			foundPath = true
		}
	}
	if !foundContent {
		t.Fatalf("expected content hit on docs/readme.md, hits=%+v", hits)
	}
	if !foundPath {
		t.Fatalf("expected path hit on docs/find-token-name.go, hits=%+v", hits)
	}

	// Empty query → empty list
	empty, err := svc.FindInFolder(ws.ID, "docs", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty query should return no hits, got %d", len(empty))
	}

	// Non-folder target rejected
	if _, err := svc.FindInFolder(ws.ID, "docs/readme.md", "x", 10); err == nil {
		t.Fatal("expected error for file target")
	}
}

func TestCopyPasteAutoRename(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	mustMkdir(t, root)
	mustMkdir(t, filepath.Join(root, "a"))
	mustMkdir(t, filepath.Join(root, "b"))
	mustWrite(t, filepath.Join(root, "a", "file.txt"), "BODY-A\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	// Copy into b → a/file.txt remains + b/file.txt
	copied, err := svc.CopyPath(ws.ID, "a/file.txt", "b")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Path != "b/file.txt" {
		t.Fatalf("copied path = %q", copied.Path)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b", "file.txt")); err != nil || string(data) != "BODY-A\n" {
		t.Fatalf("dest content = %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "a", "file.txt")); err != nil {
		t.Fatal("original missing after copy")
	}

	// Paste into same folder as original → auto-rename "file copy.txt"
	same, err := svc.CopyPath(ws.ID, "a/file.txt", "a")
	if err != nil {
		t.Fatal(err)
	}
	if same.Path != "a/file copy.txt" {
		t.Fatalf("same-folder paste path = %q want a/file copy.txt", same.Path)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a", "file copy.txt")); err != nil || string(data) != "BODY-A\n" {
		t.Fatalf("renamed copy content err=%v data=%q", err, data)
	}
	// Original still present
	if _, err := os.Stat(filepath.Join(root, "a", "file.txt")); err != nil {
		t.Fatal("original should remain after same-folder copy paste")
	}

	// SourceUser on stack (user edit)
	g, err := svc.openGateway(root)
	if err != nil {
		t.Fatal(err)
	}
	if g.UndoDepth() < 1 {
		t.Fatal("copy should push undo units")
	}
	stack := g.UndoStack()
	// Newest unit should carry user source
	newest := stack[len(stack)-1]
	for _, e := range newest.Entries {
		if e.Source != writegate.SourceUser {
			t.Fatalf("copy entry source = %q want %q", e.Source, writegate.SourceUser)
		}
	}
}

func TestCutPasteMoves(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	mustMkdir(t, root)
	mustMkdir(t, filepath.Join(root, "src"))
	mustMkdir(t, filepath.Join(root, "dst"))
	mustWrite(t, filepath.Join(root, "src", "move-me.txt"), "MOVE\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.MovePath(ws.ID, "src/move-me.txt", "dst")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "dst/move-me.txt" {
		t.Fatalf("moved path = %q", res.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "move-me.txt")); !os.IsNotExist(err) {
		t.Fatal("source should be gone after cut+paste")
	}
	if data, err := os.ReadFile(filepath.Join(root, "dst", "move-me.txt")); err != nil || string(data) != "MOVE\n" {
		t.Fatalf("dest after move = %q err=%v", data, err)
	}

	// Same-folder cut+paste is no-op
	noop, err := svc.MovePath(ws.ID, "dst/move-me.txt", "dst")
	if err != nil {
		t.Fatal(err)
	}
	if noop.Message != "unchanged" {
		t.Fatalf("same-folder move message = %q", noop.Message)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "move-me.txt")); err != nil {
		t.Fatal("file should still exist after no-op move")
	}
}

func TestUniqueDestNameHelper(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "1")
	n, err := uniqueDestName(dir, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if n != "a copy.txt" {
		t.Fatalf("got %q", n)
	}
	mustWrite(t, filepath.Join(dir, "a copy.txt"), "2")
	n2, err := uniqueDestName(dir, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if n2 != "a copy 2.txt" {
		t.Fatalf("got %q", n2)
	}
}

func TestSaveAndCreateAreUserSource(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	mustMkdir(t, root)
	mustWrite(t, filepath.Join(root, "ed.txt"), "old\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveFile(ws.ID, "ed.txt", "new-user\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateFile(ws.ID, "", "brand-new.txt"); err != nil {
		t.Fatal(err)
	}
	g, err := svc.openGateway(root)
	if err != nil {
		t.Fatal(err)
	}
	if g.UndoDepth() < 2 {
		t.Fatalf("expected >=2 undo units, got %d", g.UndoDepth())
	}
	for _, u := range g.UndoStack() {
		for _, e := range u.Entries {
			if e.Source != writegate.SourceUser {
				t.Fatalf("entry source = %q path=%s want user", e.Source, e.Snapshot.Rel)
			}
		}
	}
}
