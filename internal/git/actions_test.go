package git

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseGitHubRemote(t *testing.T) {
	cases := []struct {
		in          string
		owner, repo string
		ok          bool
	}{
		{"https://github.com/acme/app.git", "acme", "app", true},
		{"https://github.com/acme/app", "acme", "app", true},
		{"https://x-access-token:ghs_secret@github.com/acme/app.git", "acme", "app", true},
		{"git@github.com:acme/app.git", "acme", "app", true},
		{"ssh://git@github.com/acme/app.git", "acme", "app", true},
		{"https://gitlab.com/acme/app.git", "", "", false},
		{"https://github.com/onlyone", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		gr, ok := ParseGitHubRemote(tc.in)
		if ok != tc.ok {
			t.Fatalf("%q: ok=%v want %v (%+v)", tc.in, ok, tc.ok, gr)
		}
		if ok {
			if gr.Owner != tc.owner || gr.Repo != tc.repo {
				t.Fatalf("%q: got %s/%s want %s/%s", tc.in, gr.Owner, gr.Repo, tc.owner, tc.repo)
			}
			if strings.Contains(gr.Raw, "ghs_secret") || strings.Contains(gr.Raw, "x-access-token") {
				t.Fatalf("credentials leaked in Raw: %s", gr.Raw)
			}
		}
	}
}

func TestRedactSecrets(t *testing.T) {
	in := "Authorization: Bearer gho_ABCDEFG1234567890 and GITHUB_TOKEN=ghs_secretvalue https://x-access-token:abc@github.com/a/b"
	out := RedactSecrets(in)
	if strings.Contains(out, "gho_ABCDEFG") || strings.Contains(out, "ghs_secret") || strings.Contains(out, "abc@") {
		t.Fatalf("not redacted: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected redaction markers: %s", out)
	}
}

func TestBadgeFor(t *testing.T) {
	if badgeFor("completed", "success") != "success" {
		t.Fatal("success")
	}
	if badgeFor("in_progress", "") != "in_progress" {
		t.Fatal("in_progress")
	}
	if badgeFor("queued", "") != "queued" {
		t.Fatal("queued")
	}
	if badgeFor("completed", "failure") != "failure" {
		t.Fatal("failure")
	}
	if badgeFor("completed", "cancelled") != "cancelled" {
		t.Fatal("cancelled")
	}
}

func TestOpenRunURL(t *testing.T) {
	u, err := OpenRunURL("https://github.com/acme/app/actions/runs/42")
	if err != nil || !strings.Contains(u, "/actions/runs/42") {
		t.Fatalf("%v %v", u, err)
	}
	if _, err := OpenRunURL("http://evil.com/x"); err == nil {
		t.Fatal("expected reject non-https")
	}
	if _, err := OpenRunURL("https://evil.com/actions/runs/1"); err == nil {
		t.Fatal("expected reject non-github")
	}
}

func TestListGitHubActions_NotGitHub(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "t@t.local")
	runGit(t, root, "config", "user.name", "T")
	// bare commit so remote works
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "a.txt")
	runGit(t, root, "commit", "-m", "i")
	runGit(t, root, "remote", "add", "origin", "https://gitlab.com/acme/app.git")

	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.ListGitHubActions(ListActionsOptions{PreferGH: false, SkipNetwork: false, Token: "unused"})
	if res.Availability != ActionsNotGitHub {
		t.Fatalf("want not_github got %+v", res)
	}
	if res.OK {
		t.Fatal("should not be ok")
	}
	// token never echoed
	blob, _ := json.Marshal(res)
	if strings.Contains(string(blob), "unused") {
		t.Fatalf("token leaked in JSON: %s", blob)
	}
}

func TestListGitHubActions_NoRemote(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.ListGitHubActions(ListActionsOptions{})
	if res.Availability != ActionsNoRemote && res.Availability != ActionsNotRepo {
		// empty repo may report no remote
		if res.Availability != ActionsNoRemote {
			// after init alone, is-inside-work-tree true but no remote
			t.Logf("availability=%s msg=%s", res.Availability, res.Message)
		}
	}
	if res.Availability != ActionsNoRemote {
		// create commit then still no remote
		runGit(t, root, "config", "user.email", "t@t.local")
		runGit(t, root, "config", "user.name", "T")
		if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", "a.txt")
		runGit(t, root, "commit", "-m", "i")
		res = svc.ListGitHubActions(ListActionsOptions{})
		if res.Availability != ActionsNoRemote {
			t.Fatalf("want no_remote got %s (%s)", res.Availability, res.Message)
		}
	}
}

func TestListGitHubActions_APIToken(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "t@t.local")
	runGit(t, root, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "a.txt")
	runGit(t, root, "commit", "-m", "i")
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/app.git")

	// mock API
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer test-secret-token" {
			sawAuth = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workflow_runs": []map[string]any{
				{
					"id":           99,
					"name":         "CI",
					"display_title": "CI #1",
					"status":       "completed",
					"conclusion":   "success",
					"head_branch":  "main",
					"event":        "push",
					"html_url":     "https://github.com/acme/app/actions/runs/99",
					"created_at":   time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
					"updated_at":   time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
				},
			},
		})
	}))
	defer srv.Close()

	// Monkeypatch is not available; call fetchRunsViaAPI directly + full path via custom client that rewrites URL.
	// Instead exercise parse + map through List with PreferGH false and intercept via HTTPClient that points…
	// fetchRunsViaAPI always hits api.github.com — unit-test the API helper with rewritten path via client Transport.
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// rewrite to mock
		nr, _ := http.NewRequest(req.Method, srv.URL, nil)
		nr.Header = req.Header
		return http.DefaultTransport.RoundTrip(nr)
	})}

	svc, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res := svc.ListGitHubActions(ListActionsOptions{
		PreferGH:   false,
		Token:      "test-secret-token",
		HTTPClient: client,
		Limit:      10,
		Now:        time.Now(),
	})
	if !res.OK || res.Availability != ActionsOK {
		t.Fatalf("want ok: %+v", res)
	}
	if !sawAuth {
		t.Fatal("API did not receive Authorization")
	}
	if len(res.Runs) != 1 || res.Runs[0].Name != "CI" || res.Runs[0].Badge != "success" {
		t.Fatalf("runs: %+v", res.Runs)
	}
	if res.AuthMethod != "token" {
		t.Fatalf("auth_method=%s", res.AuthMethod)
	}
	// Token never in payload
	blob, _ := json.Marshal(res)
	if strings.Contains(string(blob), "test-secret-token") {
		t.Fatalf("token leaked: %s", blob)
	}
	if res.Owner != "acme" || res.Repo != "app" {
		t.Fatalf("owner/repo %+v", res)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestParseActionsRunsJSON(t *testing.T) {
	raw := []byte(`{"workflow_runs":[{"id":1,"name":"Build","status":"in_progress","conclusion":null,"head_branch":"dev","html_url":"https://github.com/a/b/actions/runs/1","created_at":"2026-07-18T10:00:00Z"}]}`)
	runs, err := parseActionsRunsJSON(raw, time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC))
	if err != nil || len(runs) != 1 {
		t.Fatalf("%v %+v", err, runs)
	}
	if runs[0].Badge != "in_progress" || runs[0].RelativeTime != "2h ago" {
		t.Fatalf("%+v", runs[0])
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=T",
		"GIT_AUTHOR_EMAIL=t@t.local",
		"GIT_COMMITTER_NAME=T",
		"GIT_COMMITTER_EMAIL=t@t.local",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
