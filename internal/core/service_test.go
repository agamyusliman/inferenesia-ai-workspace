package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceOpenSwitchListDir(t *testing.T) {
	home := t.TempDir()
	a := filepath.Join(t.TempDir(), "ws-a")
	b := filepath.Join(t.TempDir(), "ws-b")
	mustMkdir(t, a)
	mustMkdir(t, b)
	mustWrite(t, filepath.Join(a, "AGENTS.md"), "# A\n")
	mustMkdir(t, filepath.Join(a, "docs"))
	mustMkdir(t, filepath.Join(a, "cmd"))
	mustMkdir(t, filepath.Join(a, "internal"))
	mustWrite(t, filepath.Join(b, "README.md"), "B only\n")
	mustMkdir(t, filepath.Join(b, "only-b"))

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if got := svc.BrandName(); got != "Inferenesia" {
		t.Fatalf("brand = %q", got)
	}

	wa, err := svc.OpenWorkspace(a)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := svc.ListDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	for _, want := range []string{"docs", "AGENTS.md", "cmd", "internal"} {
		if !names[want] {
			t.Fatalf("workspace A missing %s: %#v", want, names)
		}
	}

	wb, err := svc.OpenWorkspace(b)
	if err != nil {
		t.Fatal(err)
	}
	// Switch back with one call (single click binding).
	if _, err := svc.SwitchWorkspace(wa.ID); err != nil {
		t.Fatal(err)
	}
	state := svc.GetHubState()
	if state.ActiveID != wa.ID {
		t.Fatalf("active after switch A = %q want %q", state.ActiveID, wa.ID)
	}
	if state.Product != "Inferenesia" {
		t.Fatalf("product brand = %q", state.Product)
	}
	if state.ChatContext == "" || state.ChatContext == "No workspace selected" {
		t.Fatalf("chat context not updated: %q", state.ChatContext)
	}

	if _, err := svc.SwitchWorkspace(wb.ID); err != nil {
		t.Fatal(err)
	}
	entriesB, err := svc.ListDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	namesB := map[string]bool{}
	for _, e := range entriesB {
		namesB[e.Name] = true
	}
	if !namesB["only-b"] || !namesB["README.md"] {
		t.Fatalf("workspace B tree wrong: %#v", namesB)
	}
	if namesB["docs"] {
		t.Fatalf("workspace B should not list A's docs: %#v", namesB)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
