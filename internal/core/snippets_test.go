package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalSnippetsDefaultsNoSecretsAndPersist(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListTerminalSnippets()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(list.Path, home) {
		t.Fatalf("snippet path %q not under config home %q", list.Path, home)
	}
	if !strings.Contains(list.Path, "snippets") {
		t.Fatalf("expected snippets/ under home, got %q", list.Path)
	}
	if len(list.Snippets) == 0 {
		t.Fatal("expected default snippets")
	}
	for _, sn := range list.Snippets {
		if sn.Name == "" || sn.Body == "" {
			t.Fatalf("empty default snippet: %+v", sn)
		}
		// Defaults must never look like credentials.
		lower := strings.ToLower(sn.Name + " " + sn.Body)
		for _, bad := range []string{"api_key", "apikey", "password", "secret", "token", "bearer", "sk-"} {
			if strings.Contains(lower, bad) {
				t.Fatalf("default snippet looks secret-like (%q): %+v", bad, sn)
			}
		}
	}

	// File written under config home.
	if _, err := os.Stat(list.Path); err != nil {
		t.Fatalf("snippet file missing: %v", err)
	}

	// Save a new user snippet.
	saved, err := svc.SaveTerminalSnippet(TerminalSnippetSaveRequest{
		Name: "Echo hello",
		Body: "echo hello-snippet\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.Name != "Echo hello" {
		t.Fatalf("saved = %+v", saved)
	}
	list2, err := svc.ListTerminalSnippets()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sn := range list2.Snippets {
		if sn.ID == saved.ID {
			found = true
			if sn.Body != "echo hello-snippet\n" {
				t.Fatalf("body = %q", sn.Body)
			}
		}
	}
	if !found {
		t.Fatal("saved snippet missing from list")
	}

	// Update existing.
	updated, err := svc.SaveTerminalSnippet(TerminalSnippetSaveRequest{
		ID:   saved.ID,
		Name: "Echo hello v2",
		Body: "echo hello-v2\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Echo hello v2" || updated.Body != "echo hello-v2\n" {
		t.Fatalf("updated = %+v", updated)
	}

	// Delete.
	afterDel, err := svc.DeleteTerminalSnippet(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sn := range afterDel.Snippets {
		if sn.ID == saved.ID {
			t.Fatal("deleted snippet still present")
		}
	}

	// On-disk path stays under home only.
	wantRel := filepath.Join("snippets", "terminal.json")
	if !strings.HasSuffix(list.Path, wantRel) {
		t.Fatalf("path suffix want %q got %q", wantRel, list.Path)
	}
}

func TestTerminalSnippetsRejectSecretLike(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SaveTerminalSnippet(TerminalSnippetSaveRequest{
		Name: "My key",
		Body: "export TEMP_AI_API_KEY=sk-super-secret-value-not-allowed\n",
	})
	if err == nil {
		t.Fatal("expected reject of secret-like body")
	}
	if !strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
}

func TestTerminalSnippetsHTTPHandlers(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	// LIST
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/snippets/terminal", nil))
	if rr.Code != 200 {
		t.Fatalf("list %d %s", rr.Code, rr.Body.String())
	}
	var listed TerminalSnippetList
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Snippets) == 0 {
		t.Fatal("empty defaults over HTTP")
	}
	if !strings.Contains(listed.Path, "terminal.json") {
		t.Fatalf("path = %q", listed.Path)
	}

	// SAVE
	body, _ := json.Marshal(map[string]string{
		"name": "Pwd once",
		"body": "pwd\n",
	})
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/snippets/terminal", bytes.NewReader(body))
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("save %d %s", rr.Code, rr.Body.String())
	}
	var saved TerminalSnippet
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" {
		t.Fatal("empty id from save")
	}

	// DELETE
	delBody, _ := json.Marshal(map[string]string{"id": saved.ID})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/snippets/terminal/delete", bytes.NewReader(delBody))
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("delete %d %s", rr.Code, rr.Body.String())
	}
}
