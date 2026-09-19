// Package context7 is a thin, soft-fail HTTP client for the public Context7
// library-docs API. It is used by the builtin `context7_query` agent tool to
// fetch up-to-date, bounded documentation snippets for libraries/frameworks.
//
// Design rules (P9-ctx):
//   - No chat-blocking: every error path returns a clear error string to the
//     model; the agent loop never crashes because Context7 is offline, rate
//     limited, or missing a key.
//   - No new heavy deps: stdlib net/http only.
//   - Bounded: response bodies are capped (MaxResponseRunes) before they
//     enter the model context.
//   - Optional auth: `CONTEXT7_API_KEY` or `YURA_AI_CONTEXT7_API_KEY` is sent
//     as `Authorization: Bearer <key>` when present. Some endpoints work
//     anonymously with rate limits; 401/403 is handled gracefully.
//   - No secrets in logs: the API key is never logged.
package context7

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultBaseURL is the public Context7 API root.
const DefaultBaseURL = "https://context7.com/api/v2"

// DefaultTimeout bounds each Context7 HTTP call so a slow / dead upstream
// never blocks the agent loop.
const DefaultTimeout = 10 * time.Second

// MaxResponseRunes caps the doc snippet returned to the model. ~12k runes is
// enough for a focused API answer without bloating the context window.
const MaxResponseRunes = 12000

// Client is a Context7 HTTP client. The zero value is NOT usable; use New.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API root (tests / self-hosted).
func WithBaseURL(u string) Option {
	return func(c *Client) {
		if u = strings.TrimSpace(u); u != "" {
			c.baseURL = u
		}
	}
}

// WithAPIKey sets the Bearer token. Empty = anonymous (rate-limited).
func WithAPIKey(k string) Option {
	return func(c *Client) {
		c.apiKey = strings.TrimSpace(k)
	}
}

// WithHTTPClient injects an *http.Client (tests use httptest).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// New builds a Client. apiKey may be empty for anonymous use.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: DefaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Enabled reports whether Context7 is enabled. It is disabled when env
// YURA_AI_CONTEXT7=0 or CONTEXT7_DISABLED=1 (checked by the caller before
// constructing a Client, but also exposed here for the tool layer).
// This method is a no-op stub kept for API symmetry; the env gate is owned
// by the tool layer so the client stays free of os imports.
func (c *Client) Enabled() bool { return true }

// Library is one search hit from /libs/search.
type Library struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

// SearchResponse wraps the search endpoint envelope.
type SearchResponse struct {
	Libraries []Library `json:"libraries"`
}

// Search resolves a library name (e.g. "react", "next.js") to Context7
// library IDs. Returns at most a handful so the model can pick or re-call
// with a specific library_id. Soft-fail: returns an error string, never
// panics, never logs the key.
func (c *Client) Search(ctx context.Context, libraryName, query string) ([]Library, error) {
	libraryName = strings.TrimSpace(libraryName)
	if libraryName == "" {
		return nil, fmt.Errorf("context7: library_name is required")
	}
	q := url.Values{}
	q.Set("libraryName", libraryName)
	if qv := strings.TrimSpace(query); qv != "" {
		q.Set("query", qv)
	}
	body, err := c.do(ctx, "/libs/search", q)
	if err != nil {
		return nil, err
	}
	var resp SearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("context7: parse search response: %w", err)
	}
	return resp.Libraries, nil
}

// FetchContext returns bounded documentation snippets for a library + query.
// libraryID is a Context7 id like "/vercel/next.js" (from Search or known).
// The response is text the model can cite; capped to MaxResponseRunes.
func (c *Client) FetchContext(ctx context.Context, libraryID, query string) (string, error) {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return "", fmt.Errorf("context7: library_id is required")
	}
	q := url.Values{}
	q.Set("libraryId", libraryID)
	if qv := strings.TrimSpace(query); qv != "" {
		q.Set("query", qv)
	}
	body, err := c.do(ctx, "/context", q)
	if err != nil {
		return "", err
	}
	return capRunes(string(body), MaxResponseRunes), nil
}

// do performs a GET with optional Bearer auth and bounded body read.
// Status 401/403 → clear "unauthorized" error (anonymous rate-limited or
// bad key). Status 5xx / network → clear upstream error. Never logs key.
func (c *Client) do(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("context7: client not configured")
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + path
	if len(q) > 0 {
		endpoint = endpoint + "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("context7: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain;q=0.5")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("context7: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("context7: read response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("context7: unauthorized (status %d) — set CONTEXT7_API_KEY or YURA_AI_CONTEXT7_API_KEY, or continue without a key for rate-limited anonymous access", resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return nil, fmt.Errorf("context7: upstream status %d: %s", resp.StatusCode, snippet)
	}
	return raw, nil
}

func capRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n…[context7: truncated]"
}

// ResolveAPIKey returns the first non-empty Context7 key from the provided
// candidates (env values read by the caller). Empty result = anonymous.
func ResolveAPIKey(candidates ...string) string {
	for _, k := range candidates {
		if k = strings.TrimSpace(k); k != "" {
			return k
		}
	}
	return ""
}

// FetchDocs is the high-level convenience path used by the agent tool:
// given a library name (and optional explicit library_id), resolve and
// fetch bounded docs in one call. It returns a model-ready string and a
// clear error on any soft-fail. When libraryID is provided, Search is
// skipped. utf8.RuneCountInString is used to bound the final string.
func (c *Client) FetchDocs(ctx context.Context, libraryName, query, libraryID string) (string, error) {
	if id := strings.TrimSpace(libraryID); id != "" {
		return c.FetchContext(ctx, id, query)
	}
	libs, err := c.Search(ctx, libraryName, query)
	if err != nil {
		return "", err
	}
	if len(libs) == 0 {
		return "", fmt.Errorf("context7: no libraries matched %q", libraryName)
	}
	// Prefer an exact (case-insensitive) name match; else first hit.
	chosen := libs[0]
	needle := strings.ToLower(strings.TrimSpace(libraryName))
	for _, l := range libs {
		if strings.ToLower(strings.TrimSpace(l.Name)) == needle {
			chosen = l
			break
		}
	}
	body, err := c.FetchContext(ctx, chosen.ID, query)
	if err != nil {
		return "", err
	}
	header := fmt.Sprintf("Context7 docs for %s (%s):\n", chosen.Name, chosen.ID)
	out := header + body
	if utf8.RuneCountInString(out) > MaxResponseRunes {
		out = capRunes(out, MaxResponseRunes)
	}
	return out, nil
}
