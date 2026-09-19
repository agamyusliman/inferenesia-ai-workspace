package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenPathAndSwitch(t *testing.T) {
	home := t.TempDir()
	a := filepath.Join(t.TempDir(), "proj-a")
	b := filepath.Join(t.TempDir(), "proj-b")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}

	reg, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	wa, err := reg.OpenPath(a)
	if err != nil {
		t.Fatal(err)
	}
	if wa.RootPath != a && filepath.Clean(wa.RootPath) != filepath.Clean(a) {
		t.Fatalf("root path = %q want %q", wa.RootPath, a)
	}
	if reg.ActiveID() != wa.ID {
		t.Fatalf("active = %q want %q", reg.ActiveID(), wa.ID)
	}

	wb, err := reg.OpenPath(b)
	if err != nil {
		t.Fatal(err)
	}
	if reg.ActiveID() != wb.ID {
		t.Fatalf("active after open B = %q want %q", reg.ActiveID(), wb.ID)
	}

	// One-click switch back to A
	got, err := reg.Switch(wa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != wa.ID {
		t.Fatalf("switch returned %q want %q", got.ID, wa.ID)
	}
	if reg.ActiveID() != wa.ID {
		t.Fatalf("active after switch = %q want %q", reg.ActiveID(), wa.ID)
	}

	// Dedupe same path
	again, err := reg.OpenPath(a)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != wa.ID {
		t.Fatalf("dedupe failed: %q vs %q", again.ID, wa.ID)
	}
	if len(reg.List()) != 2 {
		t.Fatalf("list len = %d want 2", len(reg.List()))
	}

	// Persist across reopen
	reg2, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if reg2.ActiveID() != wa.ID {
		t.Fatalf("persisted active = %q want %q", reg2.ActiveID(), wa.ID)
	}
	if len(reg2.List()) != 2 {
		t.Fatalf("persisted list len = %d want 2", len(reg2.List()))
	}
}

func TestCreateSessionRenameRemove(t *testing.T) {
	home := t.TempDir()
	reg, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	s1, err := reg.CreateSession("Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if s1.Kind != KindSession {
		t.Fatalf("kind=%q", s1.Kind)
	}
	if !IsSession(s1) {
		t.Fatal("IsSession false")
	}
	if reg.ActiveID() != s1.ID {
		t.Fatalf("active=%q", reg.ActiveID())
	}
	renamed, err := reg.Rename(s1.ID, "Beta")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Beta" {
		t.Fatalf("name=%q", renamed.Name)
	}
	s2, err := reg.CreateSession("Gamma")
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.List()) < 2 {
		t.Fatalf("list=%d", len(reg.List()))
	}
	if err := reg.Remove(s1.ID); err != nil {
		t.Fatal(err)
	}
	if reg.ActiveID() != s2.ID && reg.ActiveID() == s1.ID {
		t.Fatal("removed session still active")
	}
	empty, err := ListDir(s2.RootPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil {
		t.Fatal("ListDir empty returned nil")
	}
}

func TestListDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# agents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, "node_modules", "x"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".git"), 0o755)

	entries, err := ListDir(root, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = e.IsDir
	}
	for _, want := range []string{"docs", "AGENTS.md", "cmd", "internal"} {
		if _, ok := names[want]; !ok {
			t.Fatalf("missing %s in %#v", want, names)
		}
	}
	if names["node_modules"] || names[".git"] {
		t.Fatalf("should skip node_modules/.git: %#v", names)
	}

	// Escape denied
	if _, err := ListDir(root, "../"); err == nil {
		t.Fatal("expected escape error")
	}
}
