package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
)

// Env names for optional 9Router kit tools (never log values).
const (
	EnvNineRouterURL = "NINEROUTER_URL"
	EnvNineRouterKey = "NINEROUTER_KEY"

	// Default timeouts for kit HTTP calls.
	nineRouterHTTPTimeout = 60 * time.Second
	// Cap large JSON tool results returned to the model.
	nineRouterMaxResultBytes = 256 << 10 // 256 KiB
	// Cap how many embedding floats are echoed (full dim reported separately).
	nineRouterEmbedPreviewDim = 8
)

// nineRouterToolDefs returns tool schemas for skills that are currently loaded
// and grant the matching tool name (VAL-KIT-002: opt-in only).
func nineRouterToolDefs(r *Registry) []Definition {
	if r == nil || r.skill == nil {
		return nil
	}
	var defs []Definition
	if r.skill.HasExtraTool(skills.ToolNineRouterImage) {
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        skills.ToolNineRouterImage,
				Description: "Generate an image via 9Router POST /v1/images/generations (requires NINEROUTER_URL; skill 9router-image must be loaded).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "prompt": { "type": "string", "description": "Image description" },
    "model": { "type": "string", "description": "Image model id from /v1/models/image" },
    "size": { "type": "string", "description": "Optional size e.g. 1024x1024" },
    "n": { "type": "integer", "description": "Optional count" },
    "response_format": { "type": "string", "description": "url or b64_json" }
  },
  "required": ["prompt"]
}`),
			},
		})
	}
	if r.skill.HasExtraTool(skills.ToolNineRouterEmbeddings) {
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        skills.ToolNineRouterEmbeddings,
				Description: "Create embeddings via 9Router POST /v1/embeddings. Does not require Qdrant/enowx-rag. Skill 9router-embeddings must be loaded.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "input": { "type": "string", "description": "Text to embed (or use inputs for multiple)" },
    "inputs": { "type": "array", "items": { "type": "string" }, "description": "Multiple texts to embed" },
    "model": { "type": "string", "description": "Embedding model id" }
  }
}`),
			},
		})
	}
	if r.skill.HasExtraTool(skills.ToolNineRouterWebSearch) {
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        skills.ToolNineRouterWebSearch,
				Description: "Web search via 9Router POST /v1/search (requires NINEROUTER_URL; skill 9router-web-search must be loaded).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Search query" },
    "model": { "type": "string", "description": "Search provider/model e.g. tavily" },
    "max_results": { "type": "integer", "description": "Max results (default 5)" }
  },
  "required": ["query"]
}`),
			},
		})
	}
	if r.skill.HasExtraTool(skills.ToolNineRouterWebFetch) {
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        skills.ToolNineRouterWebFetch,
				Description: "Fetch URL content via 9Router POST /v1/web/fetch (markdown/text/html). Skill 9router-web-fetch must be loaded.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "url": { "type": "string", "description": "URL to fetch" },
    "model": { "type": "string", "description": "Fetch provider/model e.g. jina-reader" },
    "format": { "type": "string", "description": "markdown | text | html" }
  },
  "required": ["url"]
}`),
			},
		})
	}
	if r.skill.HasExtraTool(skills.ToolNineRouterSTT) {
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        skills.ToolNineRouterSTT,
				Description: "Speech-to-text via 9Router POST /v1/audio/transcriptions. Skill 9router-stt must be loaded.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Audio file path" },
    "model": { "type": "string", "description": "STT model id e.g. openai/whisper-1" },
    "language": { "type": "string", "description": "Optional ISO-639-1 language" }
  },
  "required": ["path"]
}`),
			},
		})
	}
	return defs
}

// nineRouterClient is a thin HTTP client for 9Router-style endpoints.
// Secrets (API key) never appear in tool results (VAL-KIT-010).
type nineRouterClient struct {
	baseURL string // normalized …/v1 root
	apiKey  string // never log or return
	client  *http.Client
}

// nineRouterFromEnv builds a client when NINEROUTER_URL is set.
// Missing URL returns a clear blocked error for VAL-KIT-003.
func nineRouterFromEnv() (*nineRouterClient, error) {
	raw := strings.TrimSpace(os.Getenv(EnvNineRouterURL))
	if raw == "" {
		return nil, fmt.Errorf(
			"blocked — NINEROUTER_URL is not set. Configure NINEROUTER_URL (and NINEROUTER_KEY if auth) for 9Router kit tools",
		)
	}
	base := normalizeNineRouterToolBase(raw)
	return &nineRouterClient{
		baseURL: base,
		apiKey:  strings.TrimSpace(os.Getenv(EnvNineRouterKey)),
		client:  &http.Client{Timeout: nineRouterHTTPTimeout},
	}, nil
}

// normalizeNineRouterToolBase ensures a /v1 root (same policy as provider Bootstrap).
func normalizeNineRouterToolBase(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return base
	}
	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, "/v1") {
		return base
	}
	return base + "/v1"
}

func (c *nineRouterClient) endpoint(path string) string {
	path = "/" + strings.TrimLeft(path, "/")
	// Avoid double /v1 when path already starts with /v1
	if strings.HasPrefix(path, "/v1/") || path == "/v1" {
		// strip /v1 from base if path includes it
		base := strings.TrimSuffix(c.baseURL, "/v1")
		return base + path
	}
	return c.baseURL + path
}

func (c *nineRouterClient) setAuth(req *http.Request) {
	if c == nil || c.apiKey == "" || req == nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// doJSON POSTs JSON and returns response body (redacted for secrets in errors).
func (c *nineRouterClient) doJSON(ctx context.Context, path string, payload any) (status int, body []byte, err error) {
	if c == nil {
		return 0, nil, fmt.Errorf("blocked — 9Router client not configured")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path), bytes.NewReader(raw))
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		// Never include Authorization / key in error text.
		return 0, nil, fmt.Errorf("9Router request failed for %s: %v", redactNineRouterBase(c.baseURL)+path, sanitizeHTTPErr(err))
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, nineRouterMaxResultBytes+1))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response: %v", sanitizeHTTPErr(err))
	}
	if len(body) > nineRouterMaxResultBytes {
		body = body[:nineRouterMaxResultBytes]
	}
	return resp.StatusCode, body, nil
}

func sanitizeHTTPErr(err error) string {
	if err == nil {
		return ""
	}
	return secrethyg.Redact(err.Error())
}

// execNineRouter handles optional 9Router kit tools after skill load.
// Full HTTP clients hit 9Router-style endpoints when NINEROUTER_URL is set;
// missing env → clear blocked (VAL-KIT-003..008). Keys never appear in results (VAL-KIT-010).
func (r *Registry) execNineRouter(call Call) Result {
	name := strings.TrimSpace(call.Name)
	if r == nil || r.skill == nil || !r.skill.HasExtraTool(name) {
		return nineRouterResult(name, call.ID,
			fmt.Sprintf("%s: not available (load the matching 9router-* skill first)", name), true)
	}

	client, err := nineRouterFromEnv()
	if err != nil {
		return nineRouterResult(name, call.ID, fmt.Sprintf("%s: %s", name, err.Error()), true)
	}

	ctx := context.Background()
	// Prefer request-scoped deadline if call was initiated under a parent cancel —
	// Registry.Execute already passed ctx for other tools; use Background with timeout here
	// since call signature is synchronous. Bound by client.Timeout.

	var content string
	var isErr bool
	switch name {
	case skills.ToolNineRouterImage:
		content, isErr = execNineRouterImage(ctx, client, call.Arguments)
	case skills.ToolNineRouterWebSearch:
		content, isErr = execNineRouterWebSearch(ctx, client, call.Arguments)
	case skills.ToolNineRouterWebFetch:
		content, isErr = execNineRouterWebFetch(ctx, client, call.Arguments)
	case skills.ToolNineRouterEmbeddings:
		content, isErr = execNineRouterEmbeddings(ctx, client, call.Arguments)
	case skills.ToolNineRouterSTT:
		workspace := ""
		if r.gw != nil {
			workspace = r.gw.RootPath()
		}
		content, isErr = execNineRouterSTT(ctx, client, call.Arguments, workspace)
	default:
		content = fmt.Sprintf("%s: unknown 9Router tool", name)
		isErr = true
	}
	return nineRouterResult(name, call.ID, content, isErr)
}

func nineRouterResult(name, id, content string, isErr bool) Result {
	// Always redact secrets from tool content (VAL-KIT-010).
	content = secrethyg.Redact(content)
	// Belt-and-suspenders: also strip any live NINEROUTER_KEY value.
	if k := strings.TrimSpace(os.Getenv(EnvNineRouterKey)); k != "" && len(k) >= 4 {
		content = strings.ReplaceAll(content, k, "[REDACTED]")
	}
	return Result{ID: id, Name: name, Content: content, IsError: isErr}
}

// --- Image (VAL-KIT-004) ----------------------------------------------------

func execNineRouterImage(ctx context.Context, client *nineRouterClient, argsJSON string) (string, bool) {
	var args struct {
		Prompt         string `json:"prompt"`
		Model          string `json:"model"`
		Size           string `json:"size"`
		N              int    `json:"n"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("ninerouter_image: invalid arguments: %v", err), true
	}
	prompt := strings.TrimSpace(args.Prompt)
	if prompt == "" {
		return "ninerouter_image: prompt is required", true
	}
	n := args.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	payload := map[string]any{
		"prompt": prompt,
		"n":      n,
	}
	if m := strings.TrimSpace(args.Model); m != "" {
		payload["model"] = m
	}
	if s := strings.TrimSpace(args.Size); s != "" {
		payload["size"] = s
	}
	if rf := strings.TrimSpace(args.ResponseFormat); rf != "" {
		payload["response_format"] = rf
	}

	status, body, err := client.doJSON(ctx, "/images/generations", payload)
	if err != nil {
		return fmt.Sprintf("ninerouter_image: %v", err), true
	}
	if status != http.StatusOK {
		return nineRouterHTTPError("ninerouter_image", status, body), true
	}

	// Parse OpenAI-compatible image response.
	var resp struct {
		Data []struct {
			URL           string `json:"url"`
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		// Soft degrade: return truncated raw JSON (already redacted on return path).
		return fmt.Sprintf("ninerouter_image: received HTTP 200 but could not parse images: %s", truncate(string(body), 400)), true
	}
	if len(resp.Data) == 0 {
		return "ninerouter_image: response contained no images (unsupported or empty)", true
	}

	type imgOut struct {
		URL           string `json:"url,omitempty"`
		B64JSON       string `json:"b64_json,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
		B64Len        int    `json:"b64_len,omitempty"`
	}
	out := make([]imgOut, 0, len(resp.Data))
	for _, d := range resp.Data {
		item := imgOut{RevisedPrompt: d.RevisedPrompt}
		if d.URL != "" {
			item.URL = d.URL
		}
		// Keep large b64 out of model context when possible; show length + prefix.
		if d.B64JSON != "" {
			item.B64Len = len(d.B64JSON)
			if len(d.B64JSON) <= 120 {
				item.B64JSON = d.B64JSON
			} else {
				item.B64JSON = d.B64JSON[:80] + "…"
			}
		}
		out = append(out, item)
	}
	raw, _ := json.MarshalIndent(map[string]any{
		"ok":     true,
		"images": out,
		"count":  len(out),
	}, "", "  ")
	return string(raw), false
}

// --- Web search (VAL-KIT-005) -----------------------------------------------

func execNineRouterWebSearch(ctx context.Context, client *nineRouterClient, argsJSON string) (string, bool) {
	var args struct {
		Query      string `json:"query"`
		Model      string `json:"model"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("ninerouter_web_search: invalid arguments: %v", err), true
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "ninerouter_web_search: query is required", true
	}
	max := args.MaxResults
	if max <= 0 {
		max = 5
	}
	if max > 20 {
		max = 20
	}
	payload := map[string]any{
		"query":       query,
		"max_results": max,
	}
	if m := strings.TrimSpace(args.Model); m != "" {
		payload["model"] = m
	}

	status, body, err := client.doJSON(ctx, "/search", payload)
	if err != nil {
		return fmt.Sprintf("ninerouter_web_search: %v", err), true
	}
	if status != http.StatusOK {
		return nineRouterHTTPError("ninerouter_web_search", status, body), true
	}

	// Accept both 9Router shape {results:[{title,url,snippet}]} and common variants.
	type hit struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Snippet string  `json:"snippet"`
		Score   float64 `json:"score,omitempty"`
	}
	var parsed struct {
		Results []hit   `json:"results"`
		Query   string  `json:"query"`
		Answer  *string `json:"answer"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Sprintf("ninerouter_web_search: parse error: %v; body=%s", err, truncate(string(body), 400)), true
	}
	// Alternate shapes: data.results or organic
	if len(parsed.Results) == 0 {
		var alt struct {
			Data struct {
				Results []hit `json:"results"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &alt) == nil && len(alt.Data.Results) > 0 {
			parsed.Results = alt.Data.Results
		}
	}

	// Normalize empty snippet fields from "content" or "description".
	var loose []map[string]any
	if len(parsed.Results) == 0 {
		_ = json.Unmarshal(body, &struct {
			Results *[]map[string]any `json:"results"`
		}{Results: &loose})
		for _, m := range loose {
			h := hit{
				Title:   strField(m, "title"),
				URL:     firstNonEmpty(strField(m, "url"), strField(m, "link")),
				Snippet: firstNonEmpty(strField(m, "snippet"), strField(m, "description"), strField(m, "content")),
			}
			if h.Title != "" || h.URL != "" {
				parsed.Results = append(parsed.Results, h)
			}
		}
	}

	structured := make([]map[string]any, 0, len(parsed.Results))
	for i, r := range parsed.Results {
		item := map[string]any{
			"title":   r.Title,
			"url":     r.URL,
			"snippet": r.Snippet,
			"rank":    i + 1,
		}
		if r.Score != 0 {
			item["score"] = r.Score
		}
		structured = append(structured, item)
	}
	out := map[string]any{
		"ok":      true,
		"query":   firstNonEmpty(parsed.Query, query),
		"count":   len(structured),
		"results": structured,
	}
	if parsed.Answer != nil && *parsed.Answer != "" {
		out["answer"] = *parsed.Answer
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	return string(raw), false
}

// --- Web fetch (VAL-KIT-006) ------------------------------------------------

func execNineRouterWebFetch(ctx context.Context, client *nineRouterClient, argsJSON string) (string, bool) {
	var args struct {
		URL    string `json:"url"`
		Model  string `json:"model"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("ninerouter_web_fetch: invalid arguments: %v", err), true
	}
	target := strings.TrimSpace(args.URL)
	if target == "" {
		return "ninerouter_web_fetch: url is required", true
	}
	format := strings.TrimSpace(args.Format)
	if format == "" {
		format = "markdown"
	}
	payload := map[string]any{
		"url":    target,
		"format": format,
	}
	if m := strings.TrimSpace(args.Model); m != "" {
		payload["model"] = m
	}

	status, body, err := client.doJSON(ctx, "/web/fetch", payload)
	if err != nil {
		return fmt.Sprintf("ninerouter_web_fetch: %v", err), true
	}
	if status != http.StatusOK {
		return nineRouterHTTPError("ninerouter_web_fetch", status, body), true
	}

	// Preferred 9Router shape: {title, content:{format,text}, url}
	var parsed struct {
		Provider string `json:"provider"`
		URL      string `json:"url"`
		Title    string `json:"title"`
		Content  any    `json:"content"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Sprintf("ninerouter_web_fetch: parse error: %v; body=%s", err, truncate(string(body), 400)), true
	}

	text, gotFormat := extractFetchContent(parsed.Content, format)
	// Alternate: data.content / markdown field
	if text == "" {
		var alt struct {
			Data struct {
				Title   string `json:"title"`
				Content any    `json:"content"`
				Text    string `json:"text"`
			} `json:"data"`
			Markdown string `json:"markdown"`
			Text     string `json:"text"`
		}
		if json.Unmarshal(body, &alt) == nil {
			if parsed.Title == "" {
				parsed.Title = alt.Data.Title
			}
			if t, f := extractFetchContent(alt.Data.Content, format); t != "" {
				text, gotFormat = t, f
			} else if alt.Data.Text != "" {
				text = alt.Data.Text
			} else if alt.Markdown != "" {
				text = alt.Markdown
				gotFormat = "markdown"
			} else if alt.Text != "" {
				text = alt.Text
			}
		}
	}
	if text == "" {
		// Last resort: return truncated raw body as text.
		text = truncate(string(body), 8000)
		gotFormat = "raw"
	}

	// Cap content returned to the model.
	const maxFetchChars = 12000
	truncatedFlag := false
	if len(text) > maxFetchChars {
		text = text[:maxFetchChars] + "\n…[truncated]"
		truncatedFlag = true
	}

	out := map[string]any{
		"ok":        true,
		"url":       firstNonEmpty(parsed.URL, target),
		"title":     parsed.Title,
		"format":    gotFormat,
		"content":   text,
		"length":    len(text),
		"truncated": truncatedFlag,
	}
	if parsed.Provider != "" {
		out["provider"] = parsed.Provider
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	return string(raw), false
}

func extractFetchContent(content any, wantFormat string) (text string, format string) {
	format = wantFormat
	switch v := content.(type) {
	case string:
		return v, format
	case map[string]any:
		if f, ok := v["format"].(string); ok && f != "" {
			format = f
		}
		if t, ok := v["text"].(string); ok {
			return t, format
		}
		if t, ok := v["markdown"].(string); ok {
			return t, "markdown"
		}
		if t, ok := v["html"].(string); ok {
			return t, "html"
		}
	}
	// JSON-unmarshaled content as typed struct is handled via map[string]any above;
	// also try re-marshal object with text field.
	if content != nil {
		b, err := json.Marshal(content)
		if err == nil {
			var obj struct {
				Format string `json:"format"`
				Text   string `json:"text"`
			}
			if json.Unmarshal(b, &obj) == nil && obj.Text != "" {
				if obj.Format != "" {
					format = obj.Format
				}
				return obj.Text, format
			}
		}
	}
	return "", format
}

// --- Embeddings (VAL-KIT-007) — no Qdrant/enowx-rag ---------------------------

func execNineRouterEmbeddings(ctx context.Context, client *nineRouterClient, argsJSON string) (string, bool) {
	var args struct {
		Input  string   `json:"input"`
		Inputs []string `json:"inputs"`
		Model  string   `json:"model"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("ninerouter_embeddings: invalid arguments: %v", err), true
	}
	var input any
	switch {
	case len(args.Inputs) > 0:
		input = args.Inputs
	case strings.TrimSpace(args.Input) != "":
		input = strings.TrimSpace(args.Input)
	default:
		return "ninerouter_embeddings: input or inputs is required", true
	}
	payload := map[string]any{
		"input": input,
	}
	if m := strings.TrimSpace(args.Model); m != "" {
		payload["model"] = m
	}

	status, body, err := client.doJSON(ctx, "/embeddings", payload)
	if err != nil {
		return fmt.Sprintf("ninerouter_embeddings: %v", err), true
	}
	if status != http.StatusOK {
		return nineRouterHTTPError("ninerouter_embeddings", status, body), true
	}

	var resp struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Data   []struct {
			Object    string    `json:"object"`
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Sprintf("ninerouter_embeddings: parse error: %v", err), true
	}
	if len(resp.Data) == 0 {
		return "ninerouter_embeddings: empty embedding data", true
	}

	// Summarize vectors (do not dump thousands of floats into the tool result).
	type embSummary struct {
		Index      int       `json:"index"`
		Dimensions int       `json:"dimensions"`
		Preview    []float64 `json:"preview"`
	}
	summaries := make([]embSummary, 0, len(resp.Data))
	for _, d := range resp.Data {
		prev := d.Embedding
		if len(prev) > nineRouterEmbedPreviewDim {
			prev = append([]float64(nil), d.Embedding[:nineRouterEmbedPreviewDim]...)
		}
		summaries = append(summaries, embSummary{
			Index:      d.Index,
			Dimensions: len(d.Embedding),
			Preview:    prev,
		})
	}
	out := map[string]any{
		"ok":     true,
		"model":  resp.Model,
		"count":  len(summaries),
		"data":   summaries,
		"note":   "Vectors returned from embeddings endpoint only — no Qdrant/enowx-rag storage required",
		"usage":  resp.Usage,
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	return string(raw), false
}

// --- STT (VAL-KIT-008) ------------------------------------------------------

func execNineRouterSTT(ctx context.Context, client *nineRouterClient, argsJSON, workspace string) (string, bool) {
	var args struct {
		Path     string `json:"path"`
		Model    string `json:"model"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("ninerouter_stt: invalid arguments: %v", err), true
	}
	p := strings.TrimSpace(args.Path)
	if p == "" {
		return "ninerouter_stt: path is required", true
	}
	abs, err := resolveAudioPath(p, workspace)
	if err != nil {
		return fmt.Sprintf("ninerouter_stt: %v", err), true
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Sprintf("ninerouter_stt: cannot read audio file: %v", sanitizeHTTPErr(err)), true
	}
	if len(data) == 0 {
		return "ninerouter_stt: audio file is empty", true
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if m := strings.TrimSpace(args.Model); m != "" {
		_ = w.WriteField("model", m)
	} else {
		_ = w.WriteField("model", "openai/whisper-1")
	}
	if lang := strings.TrimSpace(args.Language); lang != "" {
		_ = w.WriteField("language", lang)
	}
	part, err := w.CreateFormFile("file", filepath.Base(abs))
	if err != nil {
		return fmt.Sprintf("ninerouter_stt: form error: %v", err), true
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Sprintf("ninerouter_stt: write form: %v", err), true
	}
	if err := w.Close(); err != nil {
		return fmt.Sprintf("ninerouter_stt: close form: %v", err), true
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint("/audio/transcriptions"), &buf)
	if err != nil {
		return fmt.Sprintf("ninerouter_stt: build request: %v", err), true
	}
	client.setAuth(req)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := client.client.Do(req)
	if err != nil {
		return fmt.Sprintf("ninerouter_stt: request failed: %v", sanitizeHTTPErr(err)), true
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, nineRouterMaxResultBytes))
	if err != nil {
		return fmt.Sprintf("ninerouter_stt: read response: %v", sanitizeHTTPErr(err)), true
	}
	if resp.StatusCode != http.StatusOK {
		return nineRouterHTTPError("ninerouter_stt", resp.StatusCode, body), true
	}

	// JSON: {text: "..."} or plain text.
	var tr struct {
		Text     string `json:"text"`
		Language string `json:"language"`
		Duration float64 `json:"duration"`
	}
	text := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &tr) == nil && tr.Text != "" {
		out := map[string]any{
			"ok":   true,
			"text": tr.Text,
		}
		if tr.Language != "" {
			out["language"] = tr.Language
		}
		if tr.Duration != 0 {
			out["duration"] = tr.Duration
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		return string(raw), false
	}
	// Plain text response.
	out := map[string]any{"ok": true, "text": text}
	raw, _ := json.MarshalIndent(out, "", "  ")
	return string(raw), false
}

func resolveAudioPath(path, workspace string) (string, error) {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("audio path not found: %s", path)
		}
		return path, nil
	}
	if workspace == "" {
		return "", fmt.Errorf("relative path requires workspace; got %q", path)
	}
	abs := filepath.Join(workspace, path)
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("audio path not found under workspace: %s", path)
	}
	// Ensure still under workspace (no escape).
	rel, err := filepath.Rel(workspace, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("audio path escapes workspace")
	}
	return abs, nil
}

// --- helpers ----------------------------------------------------------------

func nineRouterHTTPError(tool string, status int, body []byte) string {
	msg := truncate(secrethyg.Redact(string(body)), 500)
	// Never leave Authorization fragments.
	msg = strings.ReplaceAll(msg, "Bearer ", "Bearer [REDACTED] ")
	return fmt.Sprintf("%s: 9Router HTTP %d: %s", tool, status, msg)
}

// redactNineRouterBase returns a display-safe base URL (no userinfo/query).
func redactNineRouterBase(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[at+1:]
			raw = raw[:i+3] + rest
		}
	}
	return strings.TrimRight(raw, "/")
}

// truncate is defined in tools.go (shared).

func strField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
