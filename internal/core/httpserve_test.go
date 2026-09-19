package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerOpenSwitchList(t *testing.T) {
	home := t.TempDir()
	a := filepath.Join(t.TempDir(), "proj-a")
	b := filepath.Join(t.TempDir(), "proj-b")
	mustMk(t, a)
	mustMk(t, b)
	mustW(t, filepath.Join(a, "AGENTS.md"), "a\n")
	mustMk(t, filepath.Join(a, "docs"))
	mustW(t, filepath.Join(b, "onlyb.txt"), "b\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	// health
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rr.Code != 200 {
		t.Fatalf("health %d", rr.Code)
	}
	var health map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &health)
	if health["product"] != "Inferenesia" {
		t.Fatalf("product = %v", health["product"])
	}

	// open A
	body, _ := json.Marshal(map[string]string{"path": a})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/workspaces/open", bytes.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("open A %d %s", rr.Code, rr.Body.String())
	}

	// list root of active
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/fs/list", nil))
	if rr.Code != 200 {
		t.Fatalf("list %d %s", rr.Code, rr.Body.String())
	}
	var entries []map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &entries)
	foundDocs := false
	foundAgents := false
	for _, e := range entries {
		if e["name"] == "docs" {
			foundDocs = true
		}
		if e["name"] == "AGENTS.md" {
			foundAgents = true
		}
	}
	if !foundDocs || !foundAgents {
		t.Fatalf("entries missing docs/AGENTS.md: %v", entries)
	}

	// open B and switch back to A via hub
	body, _ = json.Marshal(map[string]string{"path": b})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/workspaces/open", bytes.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("open B %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/hub", nil))
	var hub HubState
	_ = json.Unmarshal(rr.Body.Bytes(), &hub)
	if len(hub.Workspaces) != 2 {
		t.Fatalf("workspaces = %d", len(hub.Workspaces))
	}
	// find A id
	var aID string
	for _, w := range hub.Workspaces {
		if filepath.Clean(w.RootPath) == filepath.Clean(a) {
			aID = w.ID
		}
	}
	if aID == "" {
		t.Fatal("missing workspace A id")
	}
	body, _ = json.Marshal(map[string]string{"id": aID})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/workspaces/switch", bytes.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("switch %d %s", rr.Code, rr.Body.String())
	}
}

func TestValidateDevPort(t *testing.T) {
	if err := ValidateDevPort(4110); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDevPort(5000); err == nil {
		t.Fatal("expected 5000 denied")
	}
	if err := ValidateDevPort(7000); err == nil {
		t.Fatal("expected 7000 denied")
	}
}

func mustMk(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}
func mustW(t *testing.T, p, b string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
}
