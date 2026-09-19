package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveFileUserEditPersists(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	mustMkdir(t, root)
	mustWrite(t, filepath.Join(root, "note.txt"), "original\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.SaveFile(ws.ID, "note.txt", "edited by user\n")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "note.txt" {
		t.Fatalf("path = %q", res.Path)
	}
	if !strings.Contains(res.Message, "user") {
		t.Fatalf("message should mark user edit, got %q", res.Message)
	}
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "edited by user\n" {
		t.Fatalf("disk content = %q", string(data))
	}
	// Re-read via Service
	got, err := svc.ReadFile(ws.ID, "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "edited by user\n" {
		t.Fatalf("ReadFile = %q", got)
	}
	// Escapes / empty path rejected
	if _, err := svc.SaveFile(ws.ID, "", "x"); err == nil {
		t.Fatal("expected error for empty path")
	}
	if _, err := svc.SaveFile(ws.ID, "../outside.txt", "x"); err == nil {
		t.Fatal("expected sandbox error for escape")
	}
}

func TestCreateFileFolderRenameDelete(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	mustMkdir(t, root)
	mustWrite(t, filepath.Join(root, "keep.txt"), "keep\n")
	mustMkdir(t, filepath.Join(root, "sub"))

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	// New file in folder
	created, err := svc.CreateFile(ws.ID, "sub", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if created.Path != "sub/hello.txt" {
		t.Fatalf("path = %q", created.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "hello.txt")); err != nil {
		t.Fatalf("file missing on disk: %v", err)
	}
	// collision rejected
	if _, err := svc.CreateFile(ws.ID, "sub", "hello.txt"); err == nil {
		t.Fatal("expected collision error for create file")
	}

	// New folder
	folder, err := svc.CreateFolder(ws.ID, "sub", "nested")
	if err != nil {
		t.Fatal(err)
	}
	if folder.Path != "sub/nested" || !folder.IsDir {
		t.Fatalf("folder result = %+v", folder)
	}
	if st, err := os.Stat(filepath.Join(root, "sub", "nested")); err != nil || !st.IsDir() {
		t.Fatalf("folder missing: %v", err)
	}

	// Rename file
	renamed, err := svc.RenamePath(ws.ID, "sub/hello.txt", "world.txt")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Path != "sub/world.txt" {
		t.Fatalf("renamed path = %q", renamed.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "world.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "hello.txt")); !os.IsNotExist(err) {
		t.Fatal("old name still exists")
	}
	// collision rename rejected
	mustWrite(t, filepath.Join(root, "sub", "other.txt"), "x\n")
	_, renameErr := svc.RenamePath(ws.ID, "sub/world.txt", "other.txt")
	if renameErr == nil {
		t.Fatal("expected collision on rename")
	}
	if !strings.Contains(renameErr.Error(), "already exists") {
		t.Fatalf("collision msg = %v", renameErr)
	}

	// Delete file
	if _, err := svc.DeletePath(ws.ID, "sub/world.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "world.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted file still on disk")
	}

	// Nested file then recursive folder delete
	if _, err := svc.CreateFile(ws.ID, "sub/nested", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeletePath(ws.ID, "sub/nested"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "nested")); !os.IsNotExist(err) {
		t.Fatal("folder still exists after recursive delete")
	}

	// keep.txt still present
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestAddRemoveFolderWorkspace(t *testing.T) {
	home := t.TempDir()
	a := filepath.Join(t.TempDir(), "proj-a")
	b := filepath.Join(t.TempDir(), "proj-b")
	mustMkdir(t, a)
	mustMkdir(t, b)
	mustMkdir(t, filepath.Join(a, "inner"))

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	wa, err := svc.OpenWorkspace(a)
	if err != nil {
		t.Fatal(err)
	}
	// Add absolute folder
	wb, err := svc.AddFolderToWorkspace(wa.ID, b)
	if err != nil {
		t.Fatal(err)
	}
	list := svc.ListWorkspaces()
	if len(list) < 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(list))
	}
	// Add relative subfolder of A as multi-root entry
	inner, err := svc.AddFolderToWorkspace(wa.ID, "inner")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(inner.RootPath, "inner") {
		t.Fatalf("inner root = %q", inner.RootPath)
	}
	// Remove B — disk must remain
	if err := svc.RemoveWorkspace(wb.ID); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(b); err != nil || !st.IsDir() {
		t.Fatalf("removed workspace folder should remain on disk: %v", err)
	}
	// A still registered (plus inner)
	foundA := false
	for _, w := range svc.ListWorkspaces() {
		if w.ID == wa.ID {
			foundA = true
		}
		if w.ID == wb.ID {
			t.Fatal("B should be gone from registry")
		}
	}
	if !foundA {
		t.Fatal("A should remain")
	}
}

func TestRemoveSessionPurgesScratchAndChat(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.CreateSessionWorkspace("purge-me")
	if err != nil {
		t.Fatal(err)
	}
	root := ws.RootPath
	mustWrite(t, filepath.Join(root, "landing.html"), "<html>hi</html>")
	if _, err := svc.SetChatHistory(ws.ID, []DesktopChatMessage{
		{ID: "m-0", Role: "user", Content: "hello"},
		{ID: "m-1", Role: "assistant", Content: "hi"},
	}); err != nil {
		t.Fatal(err)
	}
	hist, err := svc.GetChatHistory(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Messages) < 1 {
		t.Fatalf("expected chat history before delete")
	}
	if err := svc.RemoveWorkspace(ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("session scratch should be removed, err=%v", err)
	}
	for _, w := range svc.ListWorkspaces() {
		if w.ID == ws.ID {
			t.Fatal("session still in registry")
		}
	}
}

func TestResolveAndSanitize(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "ws")
	mustMkdir(t, root)
	mustWrite(t, filepath.Join(root, "f.txt"), "x")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.ResolvePath(ws.ID, "f.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Relative != "f.txt" || !info.Exists || info.IsDir {
		t.Fatalf("info = %+v", info)
	}
	if info.Absolute != filepath.Join(root, "f.txt") {
		t.Fatalf("abs = %q", info.Absolute)
	}
	// bad names
	if _, err := svc.CreateFile(ws.ID, "", "../x"); err == nil {
		t.Fatal("expected reject traversal name")
	}
	if _, err := svc.CreateFile(ws.ID, "", "a/b"); err == nil {
		t.Fatal("expected reject slash in name")
	}
}
