package context7

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/libs/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("libraryName") != "react" {
			t.Errorf("unexpected libraryName: %s", r.URL.Query().Get("libraryName"))
		}
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("expected Bearer auth, got: %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(SearchResponse{Libraries: []Library{
			{ID: "/facebook/react", Name: "React", Description: "A JS library"},
		}})
	}))
	defer srv.Close()

	c := New("testkey", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	libs, err := c.Search(context.Background(), "react", "hooks")
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || libs[0].ID != "/facebook/react" {
		t.Fatalf("unexpected libs: %+v", libs)
	}
}

func TestClient_FetchContext_Bounded(t *testing.T) {
	bigBody := strings.Repeat("docline\n", 5000) // ~40k chars
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/context" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("libraryId") != "/vercel/next.js" {
			t.Errorf("unexpected libraryId: %s", r.URL.Query().Get("libraryId"))
		}
		fmt.Fprint(w, bigBody)
	}))
	defer srv.Close()

	c := New("", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	out, err := c.FetchContext(context.Background(), "/vercel/next.js", "routing")
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(out)) > MaxResponseRunes+100 {
		t.Errorf("response not bounded: %d runes", len([]rune(out)))
	}
	if !strings.Contains(out, "[context7: truncated]") {
		t.Errorf("expected truncated marker, got tail: %q", out[len(out)-50:])
	}
}

func TestClient_FetchDocs_PrefersExactNameMatch(t *testing.T) {
	searchHit := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/libs/search" {
			_ = json.NewEncoder(w).Encode(SearchResponse{Libraries: []Library{
				{ID: "/other/react-like", Name: "React-Like"},
				{ID: "/facebook/react", Name: "React"},
			}})
			return
		}
		if r.URL.Path == "/context" {
			id := r.URL.Query().Get("libraryId")
			if id != "/facebook/react" {
				t.Errorf("expected exact-match id /facebook/react, got %s", id)
			}
			fmt.Fprintf(w, "docs for %s", id)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	_ = searchHit

	c := New("", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	out, err := c.FetchDocs(context.Background(), "react", "hooks", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/facebook/react") {
		t.Errorf("expected exact-match docs, got: %q", out)
	}
}

func TestClient_FetchDocs_WithExplicitID(t *testing.T) {
	searchCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/libs/search" {
			searchCalled = true
			return
		}
		if r.URL.Path == "/context" {
			fmt.Fprint(w, "explicit docs")
			return
		}
	}))
	defer srv.Close()

	c := New("", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	out, err := c.FetchDocs(context.Background(), "react", "hooks", "/facebook/react")
	if err != nil {
		t.Fatal(err)
	}
	if searchCalled {
		t.Error("search should be skipped when library_id is provided")
	}
	if !strings.Contains(out, "explicit docs") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestClient_Unauthorized_SoftFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer srv.Close()

	c := New("badkey", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), "react", "")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("expected unauthorized message, got: %v", err)
	}
}

func TestClient_5xx_SoftFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer srv.Close()

	c := New("", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), "react", "")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "upstream status 500") {
		t.Errorf("expected upstream status message, got: %v", err)
	}
}

func TestClient_NetworkError_SoftFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately to force network error

	c := New("", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), "react", "")
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "context7:") {
		t.Errorf("expected context7-prefixed error, got: %v", err)
	}
}

func TestClient_NoMatch_SoftFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(SearchResponse{Libraries: nil})
	}))
	defer srv.Close()

	c := New("", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.FetchDocs(context.Background(), "nonexistentlib", "x", "")
	if err == nil {
		t.Fatal("expected no-match error")
	}
	if !strings.Contains(err.Error(), "no libraries matched") {
		t.Errorf("expected no-match message, got: %v", err)
	}
}

func TestClient_RequiresLibraryName(t *testing.T) {
	c := New("")
	_, err := c.Search(context.Background(), "  ", "")
	if err == nil || !strings.Contains(err.Error(), "library_name is required") {
		t.Errorf("expected required error, got: %v", err)
	}
}

func TestResolveAPIKey(t *testing.T) {
	if got := ResolveAPIKey("", "  ", "key2"); got != "key2" {
		t.Errorf("expected key2, got %q", got)
	}
	if got := ResolveAPIKey("first", "second"); got != "first" {
		t.Errorf("expected first, got %q", got)
	}
	if got := ResolveAPIKey("", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestClient_KeyNeverInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	secret := "super-secret-key-do-not-leak"
	c := New(secret, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), "react", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("API key leaked in error message: %v", err)
	}
}
