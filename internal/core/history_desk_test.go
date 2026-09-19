package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestGetAgentChangeHistoryEmpty(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	h, err := svc.GetAgentChangeHistory()
	if err != nil {
		t.Fatal(err)
	}
	if h.Title != AgentChangesTitle {
		t.Fatalf("title=%q want %q", h.Title, AgentChangesTitle)
	}
	if h.Source != AgentChangesSource {
		t.Fatalf("source=%q want writegateway", h.Source)
	}
	if strings.Contains(strings.ToLower(h.Title), "git") {
		t.Fatalf("timeline title must not look like git history: %q", h.Title)
	}
	if h.Count != 0 || len(h.Entries) != 0 {
		t.Fatalf("expected empty timeline: %#v", h)
	}
	if !strings.Contains(h.EmptyMessage, "no agent changes") && !strings.Contains(h.EmptyMessage, "fresh") {
		t.Fatalf("empty message unclear: %q", h.EmptyMessage)
	}
	if !strings.Contains(strings.ToLower(h.Subtitle), "not git") {
		t.Fatalf("subtitle should distinguish from git: %q", h.Subtitle)
	}
}

func TestGetAgentChangeHistoryListsAgentAndUser(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	mustWriteFile(t, filepath.Join(ws, "agent.txt"), "DIRTY-BEFORE")
	mustWriteFile(t, filepath.Join(ws, "user.txt"), "USER-BEFORE")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	// Agent-style WriteGateway mutation (turn unit).
	g, err := svc.openGateway(ws)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("turn-history-1")
	if _, err := g.WriteFile("agent.txt", []byte("AGENT-AFTER")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()
	if err := svc.saveGateway(ws, g); err != nil {
		t.Fatal(err)
	}

	// User edit via core SaveFile (SourceUser).
	if _, err := svc.SaveFile("", "user.txt", "USER-AFTER"); err != nil {
		t.Fatal(err)
	}

	h, err := svc.GetAgentChangeHistory()
	if err != nil {
		t.Fatal(err)
	}
	if h.Count < 2 {
		t.Fatalf("want ≥2 entries, got %d: %#v", h.Count, h.Entries)
	}
	// Newest first.
	foundAgent, foundUser := false, false
	for _, e := range h.Entries {
		for _, p := range e.Paths {
			if p == "agent.txt" || strings.HasSuffix(p, "agent.txt") {
				foundAgent = true
				if e.Kind != "agent_turn" && e.Kind != "mixed" {
					// fs source → agent_turn
					t.Logf("agent entry kind=%s sources=%v", e.Kind, e.Sources)
				}
			}
			if p == "user.txt" || strings.HasSuffix(p, "user.txt") {
				foundUser = true
				if e.Kind != "user_edit" && e.Kind != "mixed" {
					t.Fatalf("user entry kind=%s sources=%v", e.Kind, e.Sources)
				}
			}
		}
	}
	if !foundAgent || !foundUser {
		t.Fatalf("timeline missing agent and/or user paths: %#v", h.Entries)
	}
	// Must not claim to be git.
	if h.Source == "git" || strings.Contains(strings.ToLower(h.Title), "git log") {
		t.Fatal("must not be git history")
	}
}

func TestAgentChangeHistoryHTTP(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	mustWriteFile(t, filepath.Join(ws, "f.txt"), "BEFORE")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	g, err := svc.openGateway(ws)
	if err != nil {
		t.Fatal(err)
	}
	g.BeginTurn("t-http")
	if _, err := g.WriteFile("f.txt", []byte("AFTER")); err != nil {
		t.Fatal(err)
	}
	g.EndTurn()
	if err := svc.saveGateway(ws, g); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/agent-changes", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
	var body AgentChangeHistory
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Title != AgentChangesTitle {
		t.Fatalf("title=%q", body.Title)
	}
	if body.Source != writegateSource {
		t.Fatalf("source=%q", body.Source)
	}
	if body.Count < 1 {
		t.Fatalf("empty body: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "f.txt") {
		t.Fatalf("body missing path: %s", rr.Body.String())
	}
	if strings.Contains(strings.ToLower(body.Title), "git history") {
		t.Fatalf("UI title must be distinct from git history: %q", body.Title)
	}
}

const writegateSource = AgentChangesSource

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Ensure writegate is imported for compile in this package even if only used via SaveFile.
var _ = writegate.SourceFS
