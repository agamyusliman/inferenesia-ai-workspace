package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ActionsAvailability codes for graceful empty states (VAL-GHA-002/007).
const (
	ActionsOK          = "ok"
	ActionsNotRepo     = "not_repo"
	ActionsNoRemote    = "no_remote"
	ActionsNotGitHub   = "not_github"
	ActionsNoAuth      = "no_auth"
	ActionsAuthFailed  = "auth_failed"
	ActionsFetchError  = "fetch_error"
	ActionsEmpty       = "empty"
)

// WorkflowRun is one GitHub Actions run row for the Actions tab.
type WorkflowRun struct {
	// ID is the GitHub run database id.
	ID int64 `json:"id"`
	// Name is the workflow name (or display title).
	Name string `json:"name"`
	// DisplayTitle is the run title when distinct from Name.
	DisplayTitle string `json:"display_title,omitempty"`
	// Status is queued | in_progress | completed | waiting | requested | pending.
	Status string `json:"status"`
	// Conclusion is success | failure | cancelled | skipped | timed_out | action_required | neutral | "" when not finished.
	Conclusion string `json:"conclusion,omitempty"`
	// Branch is head_branch.
	Branch string `json:"branch,omitempty"`
	// Event is the trigger (push, pull_request, …).
	Event string `json:"event,omitempty"`
	// HTMLURL is the github.com run URL (open in browser).
	HTMLURL string `json:"html_url"`
	// CreatedAt is RFC3339 when available.
	CreatedAt string `json:"created_at,omitempty"`
	// UpdatedAt is RFC3339 when available.
	UpdatedAt string `json:"updated_at,omitempty"`
	// RelativeTime is a short human relative label (e.g. "2h ago").
	RelativeTime string `json:"relative_time,omitempty"`
	// Badge is a UI-friendly status key: queued|in_progress|success|failure|cancelled|skipped|unknown.
	Badge string `json:"badge"`
}

// ActionsResult is the Actions monitor payload (never includes tokens).
type ActionsResult struct {
	// OK is true when runs were fetched successfully (may still be empty list).
	OK bool `json:"ok"`
	// Availability is one of Actions* constants for UI empty/guidance states.
	Availability string `json:"availability"`
	// Message is user-facing guidance (no secrets).
	Message string `json:"message"`
	// Detail is optional non-secret diagnostic (redacted).
	Detail string `json:"detail,omitempty"`
	// Owner / Repo when remote is GitHub.
	Owner string `json:"owner,omitempty"`
	Repo  string `json:"repo,omitempty"`
	// RemoteURL is the origin URL with credentials stripped.
	RemoteURL string `json:"remote_url,omitempty"`
	// AuthMethod is "gh" | "token" | "" (never the token value).
	AuthMethod string `json:"auth_method,omitempty"`
	// Runs recent workflow runs (newest first).
	Runs []WorkflowRun `json:"runs"`
	// FetchedAt wall-clock of this response (RFC3339).
	FetchedAt string `json:"fetched_at,omitempty"`
	// RepoID echoes multi-repo selection.
	RepoID string `json:"repo_id,omitempty"`
	// Limit used for the list.
	Limit int `json:"limit,omitempty"`
}

// GitHubRemote is a parsed github.com host remote.
type GitHubRemote struct {
	Owner string
	Repo  string
	// Host is typically github.com (future GHES may differ).
	Host string
	// Raw is credential-stripped remote URL.
	Raw string
	// IsGitHub true when host is github.com (or github-like path).
	IsGitHub bool
}

var (
	// sshGitHub: git@github.com:owner/repo.git
	sshGitHub = regexp.MustCompile(`(?i)^(?:ssh://)?git@([^:/\s]+):(.+)$`)
	// scp-like without git@ already handled; also nina.v@example.com:owner/repo
)

// ParseGitHubRemote extracts owner/repo from a git remote URL.
// Supports https://github.com/o/r.git, git@github.com:o/r.git, ssh://git@github.com/o/r.git.
// Strips embedded userinfo (tokens never kept). Returns ok=false when not a GitHub remote.
func ParseGitHubRemote(remote string) (GitHubRemote, bool) {
	raw := strings.TrimSpace(remote)
	if raw == "" {
		return GitHubRemote{}, false
	}
	// Strip trailing .git later; remove whitespace.
	// SCP-style: git@host:path
	if m := sshGitHub.FindStringSubmatch(raw); m != nil {
		host := strings.ToLower(m[1])
		p := strings.TrimPrefix(m[2], "/")
		p = strings.TrimSuffix(p, ".git")
		owner, repo, ok := splitOwnerRepo(p)
		if !ok {
			return GitHubRemote{Host: host, Raw: stripUserinfo(raw)}, false
		}
		gr := GitHubRemote{
			Owner: owner,
			Repo:  repo,
			Host:  host,
			Raw:   fmt.Sprintf("git@%s:%s/%s.git", host, owner, repo),
		}
		gr.IsGitHub = isGitHubHost(host)
		return gr, gr.IsGitHub
	}

	// Normalize git:// and ensure parseable URL.
	uRaw := raw
	if !strings.Contains(uRaw, "://") && strings.Contains(uRaw, "@") && strings.Contains(uRaw, ":") {
		// Already handled above; fall through
	}
	// http(s) or ssh://
	if !strings.Contains(uRaw, "://") {
		// bare github.com/o/r
		if strings.HasPrefix(strings.ToLower(uRaw), "github.com/") {
			uRaw = "https://" + uRaw
		} else {
			return GitHubRemote{Raw: stripUserinfo(raw)}, false
		}
	}
	u, err := url.Parse(uRaw)
	if err != nil {
		return GitHubRemote{Raw: stripUserinfo(raw)}, false
	}
	host := strings.ToLower(u.Hostname())
	// Drop credentials from displayed raw
	u.User = nil
	p := strings.TrimPrefix(u.Path, "/")
	p = strings.TrimSuffix(p, ".git")
	// ssh://git@github.com/owner/repo
	owner, repo, ok := splitOwnerRepo(p)
	if !ok {
		return GitHubRemote{Host: host, Raw: u.String(), IsGitHub: isGitHubHost(host)}, false
	}
	gr := GitHubRemote{
		Owner:    owner,
		Repo:     repo,
		Host:     host,
		Raw:      fmt.Sprintf("https://%s/%s/%s", host, owner, repo),
		IsGitHub: isGitHubHost(host),
	}
	return gr, gr.IsGitHub
}

func isGitHubHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "github.com" || h == "www.github.com"
}

func splitOwnerRepo(p string) (owner, repo string, ok bool) {
	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	owner = parts[0]
	repo = parts[1]
	// ignore extra path (tree/main, etc.)
	if owner == "" || repo == "" || strings.Contains(owner, " ") {
		return "", "", false
	}
	return owner, repo, true
}

// stripUserinfo removes user:pass@ from URLs for safe logging/display.
func stripUserinfo(s string) string {
	// https://user:token@host/path
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			// ensure @ is before first /
			slash := strings.Index(rest, "/")
			if slash < 0 || at < slash {
				return s[:i+3] + rest[at+1:]
			}
		}
	}
	return s
}

// RedactSecrets strips common token patterns from diagnostic strings (VAL-GHA-006).
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	out := s
	// Bearer / token headers and env leakage
	out = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[^\s,;]+`).ReplaceAllString(out, "${1}[REDACTED]")
	out = regexp.MustCompile(`(?i)(token\s+)[a-z0-9_\-]{8,}`).ReplaceAllString(out, "${1}[REDACTED]")
	out = regexp.MustCompile(`(?i)gh[pousr]_[A-Za-z0-9_]{8,}`).ReplaceAllString(out, "[REDACTED]")
	out = regexp.MustCompile(`(?i)github_pat_[A-Za-z0-9_]{8,}`).ReplaceAllString(out, "[REDACTED]")
	out = regexp.MustCompile(`(?i)(GITHUB_TOKEN|GH_TOKEN|github_token)=[^\s&]+`).ReplaceAllString(out, "${1}=[REDACTED]")
	// https://x-access-token:TOKEN@…
	out = regexp.MustCompile(`(?i)(://)([^/@:\s]+):([^/@\s]+)@`).ReplaceAllString(out, "${1}[REDACTED]@")
	return out
}

// RemoteOriginURL returns `git remote get-url origin` (or empty).
func (s *Service) RemoteOriginURL() (string, error) {
	if !s.isRepo() {
		return "", fmt.Errorf("git: not a repository")
	}
	out, err := s.runCapture("remote", "get-url", "origin")
	if err != nil {
		// Try first remote
		list, lerr := s.runCapture("remote")
		if lerr != nil || strings.TrimSpace(list) == "" {
			return "", err
		}
		first := strings.Fields(list)[0]
		out2, err2 := s.runCapture("remote", "get-url", first)
		return strings.TrimSpace(out2), err2
	}
	return strings.TrimSpace(out), nil
}

// ListActionsOptions configures ListGitHubActions.
type ListActionsOptions struct {
	// Limit max runs (default 20, cap 50).
	Limit int
	// PreferGH when true tries `gh` first (default true).
	PreferGH bool
	// Token optional GITHUB_TOKEN override (tests); production reads env.
	Token string
	// HTTPClient optional for tests.
	HTTPClient *http.Client
	// Now optional clock for relative times.
	Now time.Time
	// SkipNetwork when true only resolves remote/availability (tests).
	SkipNetwork bool
}

// ListGitHubActions lists recent workflow runs for origin (VAL-GHA-002+).
// Auth: `gh` CLI (preferred) or GITHUB_TOKEN / GH_TOKEN. Tokens never appear in result fields.
func (s *Service) ListGitHubActions(opts ListActionsOptions) ActionsResult {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	res := ActionsResult{
		OK:           false,
		Runs:         []WorkflowRun{},
		FetchedAt:    now.UTC().Format(time.RFC3339),
		Limit:        limit,
		Availability: ActionsFetchError,
	}
	if !s.isRepo() {
		res.Availability = ActionsNotRepo
		res.Message = "Not a git repository — open a workspace with a git remote to monitor Actions."
		return res
	}
	remote, err := s.RemoteOriginURL()
	if err != nil || strings.TrimSpace(remote) == "" {
		res.Availability = ActionsNoRemote
		res.Message = "No git remote configured. Add a GitHub remote (origin) to monitor workflow runs."
		res.Detail = RedactSecrets(fmt.Sprint(err))
		return res
	}
	res.RemoteURL = stripUserinfo(remote)
	ghRemote, ok := ParseGitHubRemote(remote)
	if !ok {
		res.Availability = ActionsNotGitHub
		res.Message = "Remote is not GitHub. Actions monitoring needs a github.com remote."
		res.Detail = "remote=" + res.RemoteURL
		return res
	}
	res.Owner = ghRemote.Owner
	res.Repo = ghRemote.Repo
	res.RemoteURL = ghRemote.Raw

	if opts.SkipNetwork {
		res.Availability = ActionsNoAuth
		res.Message = "GitHub remote detected; network skipped."
		return res
	}

	preferGH := opts.PreferGH
	// PreferGH defaults true when field zero — check explicitly via omitzero? We use PreferGH || !opt forced false.
	// Callers set PreferGH true by default from core.
	_ = preferGH

	token := strings.TrimSpace(opts.Token)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GH_TOKEN"))
	}

	// 1) Try gh CLI
	if preferGH || token == "" {
		if runs, method, gerr := s.fetchRunsViaGH(ghRemote, limit, now); gerr == nil {
			res.OK = true
			res.AuthMethod = method
			res.Runs = runs
			if len(runs) == 0 {
				res.Availability = ActionsEmpty
				res.Message = fmt.Sprintf("No recent workflow runs for %s/%s.", ghRemote.Owner, ghRemote.Repo)
			} else {
				res.Availability = ActionsOK
				res.Message = fmt.Sprintf("%d recent run(s)", len(runs))
			}
			return res
		} else if token == "" {
			// no token fallback
			msg, avail := classifyActionsErr(gerr)
			res.Availability = avail
			res.Message = msg
			res.Detail = RedactSecrets(gerr.Error())
			return res
		}
		// else fall through to token API
	}

	// 2) REST API with token
	if token == "" {
		res.Availability = ActionsNoAuth
		res.Message = "GitHub auth required. Install and run `gh auth login`, or set GITHUB_TOKEN (never paste the token into chat)."
		return res
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	runs, err := fetchRunsViaAPI(client, ghRemote, token, limit, now)
	// Drop token ref
	token = ""
	if err != nil {
		msg, avail := classifyActionsErr(err)
		res.Availability = avail
		res.Message = msg
		res.Detail = RedactSecrets(err.Error())
		return res
	}
	res.OK = true
	res.AuthMethod = "token"
	res.Runs = runs
	if len(runs) == 0 {
		res.Availability = ActionsEmpty
		res.Message = fmt.Sprintf("No recent workflow runs for %s/%s.", ghRemote.Owner, ghRemote.Repo)
	} else {
		res.Availability = ActionsOK
		res.Message = fmt.Sprintf("%d recent run(s)", len(runs))
	}
	return res
}

func classifyActionsErr(err error) (message, availability string) {
	if err == nil {
		return "Unknown error", ActionsFetchError
	}
	e := strings.ToLower(RedactSecrets(err.Error()))
	switch {
	case strings.Contains(e, "401") || strings.Contains(e, "unauthorized") || strings.Contains(e, "bad credentials"):
		return "GitHub authentication failed. Run `gh auth login` or refresh GITHUB_TOKEN.", ActionsAuthFailed
	case strings.Contains(e, "403") || strings.Contains(e, "forbidden"):
		return "GitHub denied access (403). Check token scopes (repo/actions:read) or org SSO.", ActionsAuthFailed
	case strings.Contains(e, "404") || strings.Contains(e, "not found"):
		return "Repository or Actions API not found (private repo needs auth; or Actions disabled).", ActionsFetchError
	case strings.Contains(e, "gh not found") || strings.Contains(e, "executable file not found"):
		return "GitHub auth required. Install `gh` and run `gh auth login`, or set GITHUB_TOKEN.", ActionsNoAuth
	case strings.Contains(e, "not logged") || strings.Contains(e, "to re-authenticate") || strings.Contains(e, "auth"):
		return "GitHub auth required. Run `gh auth login` or set GITHUB_TOKEN.", ActionsNoAuth
	default:
		return "Failed to list workflow runs.", ActionsFetchError
	}
}

func (s *Service) fetchRunsViaGH(remote GitHubRemote, limit int, now time.Time) ([]WorkflowRun, string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, "", fmt.Errorf("gh not found: %w", err)
	}
	// Use gh api for stable JSON; avoids requiring workflow permission variants.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	apiPath := fmt.Sprintf("repos/%s/%s/actions/runs?per_page=%d", remote.Owner, remote.Repo, limit)
	cmd := exec.CommandContext(ctx, "gh", "api", apiPath)
	cmd.Dir = s.root
	// Do not pass GITHUB_TOKEN into cmdline; gh uses its own config. Keep env but never log.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("gh api: %w: %s", err, RedactSecrets(stderr.String()))
	}
	runs, err := parseActionsRunsJSON(stdout.Bytes(), now)
	if err != nil {
		return nil, "", err
	}
	return runs, "gh", nil
}

type ghActionsList struct {
	WorkflowRuns []ghRun `json:"workflow_runs"`
}

type ghRun struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	DisplayTitle string `json:"display_title"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HeadBranch   string `json:"head_branch"`
	Event        string `json:"event"`
	HTMLURL      string `json:"html_url"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func parseActionsRunsJSON(body []byte, now time.Time) ([]WorkflowRun, error) {
	var list ghActionsList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse actions json: %w", err)
	}
	out := make([]WorkflowRun, 0, len(list.WorkflowRuns))
	for _, r := range list.WorkflowRuns {
		out = append(out, mapGHRun(r, now))
	}
	return out, nil
}

func mapGHRun(r ghRun, now time.Time) WorkflowRun {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = strings.TrimSpace(r.DisplayTitle)
	}
	if name == "" {
		name = "Workflow"
	}
	created := r.CreatedAt
	if created == "" {
		created = r.UpdatedAt
	}
	w := WorkflowRun{
		ID:           r.ID,
		Name:         name,
		DisplayTitle: strings.TrimSpace(r.DisplayTitle),
		Status:       strings.ToLower(strings.TrimSpace(r.Status)),
		Conclusion:   strings.ToLower(strings.TrimSpace(r.Conclusion)),
		Branch:       r.HeadBranch,
		Event:        r.Event,
		HTMLURL:      r.HTMLURL,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		RelativeTime: relativeTime(created, now),
	}
	if w.HTMLURL == "" && r.ID > 0 {
		// best-effort; owner/repo not always known here
		w.HTMLURL = fmt.Sprintf("https://github.com/actions/runs/%d", r.ID)
	}
	w.Badge = badgeFor(w.Status, w.Conclusion)
	return w
}

func badgeFor(status, conclusion string) string {
	status = strings.ToLower(status)
	conclusion = strings.ToLower(conclusion)
	switch status {
	case "queued", "requested", "pending", "waiting":
		return "queued"
	case "in_progress":
		return "in_progress"
	case "completed":
		switch conclusion {
		case "success":
			return "success"
		case "failure", "timed_out", "action_required", "startup_failure":
			return "failure"
		case "cancelled":
			return "cancelled"
		case "skipped", "neutral", "stale":
			return "skipped"
		default:
			if conclusion != "" {
				return conclusion
			}
			return "unknown"
		}
	default:
		if status != "" {
			return status
		}
		return "unknown"
	}
}

func relativeTime(rfc3339 string, now time.Time) string {
	if rfc3339 == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		// try without Z variants
		t, err = time.Parse(time.RFC3339Nano, rfc3339)
		if err != nil {
			return ""
		}
	}
	d := now.Sub(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

func fetchRunsViaAPI(client *http.Client, remote GitHubRemote, token string, limit int, now time.Time) ([]WorkflowRun, error) {
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	u := "https://api.github.com/repos/" + url.PathEscape(remote.Owner) + "/" + url.PathEscape(remote.Repo) +
		"/actions/runs?per_page=" + strconv.Itoa(limit)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Inferenesia-ActionsMonitor")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never include Authorization or body token leakage.
		msg := RedactSecrets(string(body))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("github api HTTP %d: %s", resp.StatusCode, msg)
	}
	return parseActionsRunsJSON(body, now)
}

// OpenRunURL validates a github.com actions run URL for the open-in-browser action.
func OpenRunURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url")
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("only https urls allowed")
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return "", fmt.Errorf("only github.com urls allowed")
	}
	// Path should look like /owner/repo/actions/runs/123
	p := path.Clean(u.Path)
	if !strings.Contains(p, "/actions/runs/") {
		return "", fmt.Errorf("not a workflow run url")
	}
	return u.String(), nil
}
