package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/workspace"
)

func TestResolveTerminalCwdEmptyDoesNotUseSessionScratch(t *testing.T) {
	tmp := t.TempDir()
	regPath := filepath.Join(tmp, "workspaces.json")
	reg, err := workspace.Open(regPath)
	if err != nil {
		t.Fatalf("Open registry: %v", err)
	}
	w, err := reg.CreateSession("orphan")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if !workspace.IsSession(w) {
		t.Fatalf("expected session workspace, got %+v", w)
	}
	if !strings.Contains(filepath.ToSlash(w.RootPath), "/sessions/") {
		t.Fatalf("session root = %q, want under /sessions/", w.RootPath)
	}

	svc := &Service{registry: reg}
	got, err := svc.resolveTerminalCwd("", "")
	if err != nil {
		t.Fatalf("resolveTerminalCwd: %v", err)
	}
	if strings.Contains(filepath.ToSlash(got), "/sessions/") {
		t.Fatalf("terminal cwd = %q still under sessions scratch", got)
	}
}

func TestResolveTerminalCwdAbsoluteFolder(t *testing.T) {
	tmp := t.TempDir()
	folder := filepath.Join(tmp, "project")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	svc := &Service{}
	got, err := svc.resolveTerminalCwd("", folder)
	if err != nil {
		t.Fatalf("resolveTerminalCwd: %v", err)
	}
	abs, _ := filepath.Abs(folder)
	if filepath.Clean(got) != filepath.Clean(abs) {
		t.Fatalf("got %q want %q", got, abs)
	}
}

func TestResolveTerminalCwdPrefersFolderWhenActiveIsSession(t *testing.T) {
	tmp := t.TempDir()
	regPath := filepath.Join(tmp, "workspaces.json")
	reg, err := workspace.Open(regPath)
	if err != nil {
		t.Fatalf("Open registry: %v", err)
	}
	folder := filepath.Join(tmp, "project")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := reg.OpenPath(folder); err != nil {
		t.Fatalf("OpenPath folder: %v", err)
	}
	if _, err := reg.CreateSession("active-sess"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	active := reg.Active()
	if active == nil || !workspace.IsSession(*active) {
		t.Fatalf("expected active session, got %+v", active)
	}

	svc := &Service{registry: reg}
	got, err := svc.resolveTerminalCwd("", "")
	if err != nil {
		t.Fatalf("resolveTerminalCwd: %v", err)
	}
	abs, _ := filepath.Abs(folder)
	if filepath.Clean(got) != filepath.Clean(abs) {
		t.Fatalf("got %q want folder %q (not session scratch)", got, abs)
	}
}
