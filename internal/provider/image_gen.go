package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// MaxImageGenRefs is the max reference images accepted with image generation (VAL-CHAT-025).
const MaxImageGenRefs = 4

// imageGenHTTPTimeout matches working gateway clients (kekasir-blog: 180s).
const imageGenHTTPTimeout = 180 * time.Second

// ImageGenRequest is a gateway image generation call.
// Core uses this when the desktop toggle sets generate_image true.
type ImageGenRequest struct {
	// Prompt is the text prompt (required).
	Prompt string
	// Model is optional; empty uses the profile default / discovery.
	Model string
	// N is how many images to request (default 1, max 4).
	N int
	// Size is optional size hint ("1024x1024", …).
	Size string
	// Temperature is optional (0–2). Omit when HasTemperature is false (0 is a valid value).
	Temperature    float64
	HasTemperature bool
	// ReasoningEffort is Responses API effort: "low" | "medium" | "high".
	ReasoningEffort string
	// ImageOnly prefers image output with minimal caption text.
	ImageOnly bool
	// Refs are optional reference images (data URLs or https), capped at MaxImageGenRefs.
	Refs []ImageRef
}

// ImageRef is one vision/reference image for image-to-image style generation when supported.
type ImageRef struct {
	Name      string
	MediaType string
	// DataURL is data:image/... or https://...
	DataURL string
}

// GeneratedImage is one image result (url and/or base64).
type GeneratedImage struct {
	// URL is an https URL when the provider returns a hosted image.
	URL string
	// B64JSON is raw base64 (no data: prefix) when returned that way.
	B64JSON string
	// RevisedPrompt is optional rewritten prompt from the provider.
	RevisedPrompt string
}

// ImageGenResult is the outcome of GenerateImage.
type ImageGenResult struct {
	Images []GeneratedImage
	// Model is the resolved model id used (when known).
	Model string
}

// ImageGenUnsupportedError means the provider/profile cannot generate images.
// Callers should surface a clear degrade message (no crash).
type ImageGenUnsupportedError struct {
	// Profile is the provider profile id.
	Profile string
	// Reason is operator-facing (never secrets).
	Reason string
}

func (e *ImageGenUnsupportedError) Error() string {
	if e == nil {
		return "image generation is not supported"
	}
	p := e.Profile
	if p == "" {
		p = "provider"
	}
	r := e.Reason
	if r == "" {
		r = "unsupported"
	}
	return fmt.Sprintf(
		"Image generation is not supported by profile %q (%s). Turn off Image gen or use a model/gateway that can enable image generation via `/v1/images/generations` (Console DPoP) or the `/v1/responses` fallback.",
		p, r,
	)
}

// IsImageGenUnsupported reports whether err is ImageGenUnsupportedError.
func IsImageGenUnsupported(err error) bool {
	if err == nil {
		return false
	}
	var ue *ImageGenUnsupportedError
	return errors.As(err, &ue)
}

// CapImageRefs keeps the first MaxImageGenRefs valid image URLs.
func CapImageRefs(in []ImageRef) []ImageRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]ImageRef, 0, MaxImageGenRefs)
	for _, r := range in {
		url := strings.TrimSpace(r.DataURL)
		if url == "" {
			continue
		}
		lower := strings.ToLower(url)
		if !strings.HasPrefix(lower, "data:image/") &&
			!strings.HasPrefix(lower, "https://") &&
			!strings.HasPrefix(lower, "http://") {
			continue
		}
		mt := strings.TrimSpace(r.MediaType)
		if mt == "" && strings.HasPrefix(lower, "data:") {
			if semi := strings.Index(url, ";"); semi > 5 {
				mt = url[5:semi]
			}
		}
		out = append(out, ImageRef{
			Name:      strings.TrimSpace(r.Name),
			MediaType: mt,
			DataURL:   url,
		})
		if len(out) >= MaxImageGenRefs {
			break
		}
	}
	return out
}

// GenerateImage tries gateway image paths in the order the Cloud gateway serves
// them:
//  1. POST /images/generations                              (Console DPoP path via Cloud gateway)
//  2. POST /responses + image_generation tool               (legacy Grok CLI fallback)
//
// Returns ImageGenUnsupportedError when no path yields images so the UI can degrade (VAL-CHAT-025).
func (c *OpenAICompat) GenerateImage(ctx context.Context, req ImageGenRequest) (*ImageGenResult, error) {
	if c == nil {
		return nil, fmt.Errorf("provider: nil client")
	}
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	if err := c.requireKey(); err != nil {
		return nil, err
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("provider: empty image generation prompt")
	}
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > MaxImageGenRefs {
		n = MaxImageGenRefs
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(c.cfg.DefaultModel)
	}
	refs := CapImageRefs(req.Refs)
	size := strings.TrimSpace(req.Size)

	httpClient := c.imageGenHTTPClient()
	var lastErr error

	// 1) Classic OpenAI images API (Console DPoP path via Cloud gateway).
	if res, err := c.generateImageViaImagesAPI(ctx, httpClient, prompt, model, n, size, refs, req); err == nil {
		res.Model = model
		return res, nil
	} else {
		lastErr = err
		if !IsImageGenUnsupported(err) && !isRetryableImagePathErr(err) {
			return nil, err
		}
	}

	// 2) Fallback: Responses API + image_generation tool (legacy Grok CLI — paywalled).
	if res, err := c.generateImageViaResponses(ctx, httpClient, prompt, model, refs, req); err == nil {
		res.Model = model
		return res, nil
	} else {
		lastErr = err
		if !IsImageGenUnsupported(err) && !isRetryableImagePathErr(err) {
			return nil, err
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &ImageGenUnsupportedError{Profile: c.Name(), Reason: "no image endpoint"}
}

func isRetryableImagePathErr(err error) bool {
	if err == nil {
		return false
	}
	if IsImageGenUnsupported(err) {
		return true
	}
	// Transport / parse failures on one path → try the other.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "parse") ||
		strings.Contains(msg, "non-json") ||
		strings.Contains(msg, "no images") ||
		strings.Contains(msg, "http 404") ||
		strings.Contains(msg, "http 405") ||
		strings.Contains(msg, "http 501")
}

func (c *OpenAICompat) imageGenHTTPClient() *http.Client {
	timeout := imageGenHTTPTimeout
	if c.cfg.HTTPTimeoutSeconds > 0 {
		// Allow config to raise, but never go below 60s for image gen.
		cfgT := time.Duration(c.cfg.HTTPTimeoutSeconds) * time.Second
		if cfgT > timeout {
			timeout = cfgT
		} else if cfgT >= 60*time.Second {
			timeout = cfgT
		}
	}
	dial := 30 * time.Second
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   dial,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   dial,
			ResponseHeaderTimeout: timeout,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

// generateImageViaResponses implements the working temp-ai path used by kekasir-blog:
//
//	POST {base}/responses
//	{
//	  "model": "grok-4.5",
//	  "stream": false,
//	  "store": false,
//	  "tools": [{"type": "image_generation"}],
//	  "input": [{"role":"user","content":[{"type":"input_text","text":"Generate an image: …"}]}]
//	}
func (c *OpenAICompat) generateImageViaResponses(
	ctx context.Context,
	httpClient *http.Client,
	prompt, model string,
	refs []ImageRef,
	req ImageGenRequest,
) (*ImageGenResult, error) {
	content := make([]map[string]any, 0, 1+len(refs))
	promptText := buildImageGenUserText(prompt, req)
	content = append(content, map[string]any{
		"type": "input_text",
		"text": promptText,
	})
	for _, r := range refs {
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": r.DataURL,
		})
	}
	payload := map[string]any{
		"stream": false,
		"store":  false,
		"tools":  []map[string]any{{"type": "image_generation"}},
		"input": []map[string]any{
			{"role": "user", "content": content},
		},
	}
	if model != "" {
		payload["model"] = model
	}
	if req.HasTemperature {
		payload["temperature"] = req.Temperature
	}
	// Optional reasoning hint used by some Grok gateways (kekasir).
	effort := strings.TrimSpace(strings.ToLower(req.ReasoningEffort))
	if effort == "" {
		effort = "high"
	}
	if effort != "low" && effort != "medium" && effort != "high" {
		effort = "high"
	}
	payload["reasoning"] = map[string]any{"effort": effort, "summary": "concise"}

	raw, status, err := c.postImageJSON(ctx, httpClient, "/responses", payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		return nil, &ImageGenUnsupportedError{
			Profile: c.Name(),
			Reason:  fmt.Sprintf("HTTP %d on /responses", status),
		}
	}
	if status != http.StatusOK {
		if isLikelyImageUnsupported(status, raw) {
			return nil, &ImageGenUnsupportedError{
				Profile: c.Name(),
				Reason:  shortBodyReason(raw, status),
			}
		}
		return nil, c.mapHTTPError(status, raw)
	}
	if looksLikeNonJSONImageBody(raw) {
		return nil, &ImageGenUnsupportedError{
			Profile: c.Name(),
			Reason:  "non-JSON body on /responses",
		}
	}
	return parseResponsesImageOutput(raw)
}

func (c *OpenAICompat) generateImageViaImagesAPI(
	ctx context.Context,
	httpClient *http.Client,
	prompt, model string,
	n int,
	size string,
	refs []ImageRef,
	req ImageGenRequest,
) (*ImageGenResult, error) {
	m := strings.ToLower(strings.TrimSpace(model))
	// enxapi upstream quirks:
	//   - gpt-image-2 accepts ONLY {model, prompt} (upstream errors on extras).
	//   - gemini-*-image REQUIRES size when response_format is set; default to 1024x1024.
	strictBare := strings.Contains(m, "gpt-image-2")

	payload := map[string]any{
		"prompt": buildImageGenUserText(prompt, req),
	}
	if model != "" {
		payload["model"] = model
	}

	if !strictBare {
		payload["n"] = n
		payload["response_format"] = "b64_json"
		if size == "" && strings.Contains(m, "gemini") && strings.Contains(m, "image") {
			size = "1024x1024"
		}
		if size != "" {
			payload["size"] = size
		}
		if req.HasTemperature {
			payload["temperature"] = req.Temperature
		}
		if len(refs) > 0 {
			urls := make([]string, 0, len(refs))
			for _, r := range refs {
				urls = append(urls, r.DataURL)
			}
			payload["image"] = urls
			payload["reference_images"] = urls
		}
	}

	raw, status, err := c.postImageJSON(ctx, httpClient, "/images/generations", payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		return nil, &ImageGenUnsupportedError{
			Profile: c.Name(),
			Reason:  fmt.Sprintf("HTTP %d on /images/generations", status),
		}
	}
	if status != http.StatusOK {
		if isLikelyImageUnsupported(status, raw) {
			return nil, &ImageGenUnsupportedError{
				Profile: c.Name(),
				Reason:  shortBodyReason(raw, status),
			}
		}
		return nil, c.mapHTTPError(status, raw)
	}
	if looksLikeNonJSONImageBody(raw) {
		return nil, &ImageGenUnsupportedError{
			Profile: c.Name(),
			Reason:  "non-JSON body on /images/generations",
		}
	}
	return parseImageGenResponse(raw)
}

func (c *OpenAICompat) postImageJSON(
	ctx context.Context,
	httpClient *http.Client,
	path string,
	payload map[string]any,
) (raw []byte, status int, err error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path), bytes.NewReader(body))
	if err != nil {
		return nil, 0, classifyTransport(c.cfg.BaseURL, err)
	}
	c.setAuth(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, classifyTransport(c.cfg.BaseURL, err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if readErr != nil {
		return nil, resp.StatusCode, classifyTransport(c.cfg.BaseURL, readErr)
	}
	return raw, resp.StatusCode, nil
}

func isLikelyImageUnsupported(status int, body []byte) bool {
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		lower := strings.ToLower(string(body))
		if strings.Contains(lower, "image") &&
			(strings.Contains(lower, "not support") ||
				strings.Contains(lower, "unsupported") ||
				strings.Contains(lower, "unknown endpoint") ||
				strings.Contains(lower, "not available") ||
				strings.Contains(lower, "does not support")) {
			return true
		}
	}
	return false
}

// looksLikeNonJSONImageBody detects HTML portals / empty non-JSON that should
// degrade as unsupported instead of a raw JSON parse error bubble.
func looksLikeNonJSONImageBody(raw []byte) bool {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") || strings.HasPrefix(s, "<") {
		return true
	}
	return false
}

func shortBodyReason(body []byte, status int) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	if s == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return fmt.Sprintf("HTTP %d: %s", status, s)
}

type imageGenWireResponse struct {
	Data []struct {
		URL           string `json:"url"`
		B64JSON       string `json:"b64_json"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
	// Some gateways wrap as { "images": [ { "url": … } ] }
	Images []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"images"`
	// Error body may appear even on 200 from some proxies.
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func parseImageGenResponse(raw []byte) (*ImageGenResult, error) {
	var parsed imageGenWireResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse image generation response: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return nil, fmt.Errorf("image generation: %s", strings.TrimSpace(parsed.Error.Message))
	}
	out := &ImageGenResult{}
	for _, d := range parsed.Data {
		img := GeneratedImage{
			URL:           strings.TrimSpace(d.URL),
			B64JSON:       strings.TrimSpace(d.B64JSON),
			RevisedPrompt: strings.TrimSpace(d.RevisedPrompt),
		}
		if img.URL == "" && img.B64JSON == "" {
			continue
		}
		out.Images = append(out.Images, img)
	}
	if len(out.Images) == 0 {
		for _, d := range parsed.Images {
			img := GeneratedImage{
				URL:     strings.TrimSpace(d.URL),
				B64JSON: strings.TrimSpace(d.B64JSON),
			}
			if img.URL == "" && img.B64JSON == "" {
				continue
			}
			out.Images = append(out.Images, img)
		}
	}
	if len(out.Images) == 0 {
		// Fallback: some gateways still return Responses-shaped body on images path.
		if res, err := parseResponsesImageOutput(raw); err == nil {
			return res, nil
		}
		return nil, fmt.Errorf("image generation: no images in response")
	}
	return out, nil
}

// parseResponsesImageOutput extracts base64/url from OpenAI Responses API style output
// (used by temp-ai / Grok image_generation tool — same as kekasir-blog ai_client.py).
func parseResponsesImageOutput(raw []byte) (*ImageGenResult, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse responses image output: %w", err)
	}
	if errObj, ok := root["error"].(map[string]any); ok {
		if msg, _ := errObj["message"].(string); strings.TrimSpace(msg) != "" {
			return nil, fmt.Errorf("image generation: %s", strings.TrimSpace(msg))
		}
	}

	out := &ImageGenResult{}
	seen := map[string]struct{}{}
	addB64 := func(b64 string) {
		b64 = strings.TrimSpace(b64)
		if b64 == "" {
			return
		}
		// Strip data-url prefix if gateway returned full data URI.
		if i := strings.Index(b64, "base64,"); i >= 0 {
			b64 = b64[i+len("base64,"):]
		}
		if _, ok := seen["b64:"+b64[:min(32, len(b64))]]; ok {
			return
		}
		seen["b64:"+b64[:min(32, len(b64))]] = struct{}{}
		out.Images = append(out.Images, GeneratedImage{B64JSON: b64})
	}
	addURL := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		if _, ok := seen["url:"+u]; ok {
			return
		}
		seen["url:"+u] = struct{}{}
		out.Images = append(out.Images, GeneratedImage{URL: u})
	}

	// Walk output[] items (Responses API).
	if output, ok := root["output"].([]any); ok {
		for _, item := range output {
			walkResponsesNode(item, addB64, addURL)
		}
	}
	// Some proxies nest under response.output
	if resp, ok := root["response"].(map[string]any); ok {
		if output, ok := resp["output"].([]any); ok {
			for _, item := range output {
				walkResponsesNode(item, addB64, addURL)
			}
		}
	}
	// Markdown / text content with data:image
	for _, key := range []string{"content", "message", "text", "output_text"} {
		if s, ok := root[key].(string); ok {
			if b64 := extractB64FromMarkdown(s); b64 != "" {
				addB64(b64)
			}
		}
	}
	// Classic data[] still accepted
	if data, ok := root["data"].([]any); ok {
		for _, d := range data {
			m, ok := d.(map[string]any)
			if !ok {
				continue
			}
			if u, _ := m["url"].(string); u != "" {
				addURL(u)
			}
			if b, _ := m["b64_json"].(string); b != "" {
				addB64(b)
			}
		}
	}

	if len(out.Images) == 0 {
		return nil, &ImageGenUnsupportedError{
			Profile: "",
			Reason:  "response contained no usable images",
		}
	}
	return out, nil
}

func walkResponsesNode(node any, addB64 func(string), addURL func(string)) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	t, _ := m["type"].(string)
	t = strings.ToLower(t)

	if t == "image_generation_call" || t == "output_image" || t == "image" {
		if r, _ := m["result"].(string); strings.TrimSpace(r) != "" {
			addB64(r)
		}
		if b, _ := m["b64_json"].(string); b != "" {
			addB64(b)
		}
		if u, _ := m["url"].(string); u != "" {
			addURL(u)
		}
		if u, _ := m["image_url"].(string); u != "" {
			if strings.HasPrefix(strings.ToLower(u), "data:image/") {
				if b := extractB64FromMarkdown(u); b != "" {
					addB64(b)
				}
			} else {
				addURL(u)
			}
		}
	}

	if r, _ := m["result"].(string); len(r) > 64 && (t == "" || strings.Contains(t, "image")) {
		addB64(r)
	}
	if b, _ := m["b64_json"].(string); b != "" {
		addB64(b)
	}
	if b, _ := m["image_base64"].(string); b != "" {
		addB64(b)
	}
	if u, _ := m["url"].(string); u != "" && strings.HasPrefix(strings.ToLower(u), "http") {
		addURL(u)
	}

	// Nested content[]
	if content, ok := m["content"].([]any); ok {
		for _, c := range content {
			walkResponsesNode(c, addB64, addURL)
		}
	}
	// Nested output[]
	if output, ok := m["output"].([]any); ok {
		for _, o := range output {
			walkResponsesNode(o, addB64, addURL)
		}
	}
}

func extractB64FromMarkdown(text string) string {
	if text == "" {
		return ""
	}
	// data:image/png;base64,AAAA
	const marker = ";base64,"
	lower := strings.ToLower(text)
	if i := strings.Index(lower, "data:image/"); i >= 0 {
		rest := text[i:]
		if j := strings.Index(strings.ToLower(rest), marker); j >= 0 {
			b64 := rest[j+len(marker):]
			// trim trailing markdown / quotes / whitespace
			b64 = strings.TrimSpace(b64)
			for _, cut := range []string{")", "\"", "'", " ", "\n", "`"} {
				if k := strings.Index(b64, cut); k > 0 {
					b64 = b64[:k]
				}
			}
			if len(b64) > 32 {
				return b64
			}
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func buildImageGenUserText(prompt string, req ImageGenRequest) string {
	var b strings.Builder
	b.WriteString("Generate an image: ")
	b.WriteString(strings.TrimSpace(prompt))
	b.WriteString("\n\nUse the image_generation tool.")
	if size := strings.TrimSpace(req.Size); size != "" {
		b.WriteString("\nPreferred size: ")
		b.WriteString(size)
		b.WriteString(".")
		if ar := aspectHintFromSize(size); ar != "" {
			b.WriteString("\nAspect ratio: ")
			b.WriteString(ar)
			b.WriteString(". Match this aspect ratio exactly in the generated image.")
		}
	}
	if req.ImageOnly {
		b.WriteString("\nOutput image only — no lengthy caption or analysis text.")
	} else {
		b.WriteString("\nYou may include a short caption after the image.")
	}
	return b.String()
}

func aspectHintFromSize(size string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return ""
	}
	var w, h float64
	if _, err := fmt.Sscanf(parts[0], "%f", &w); err != nil || w <= 0 {
		return ""
	}
	if _, err := fmt.Sscanf(parts[1], "%f", &h); err != nil || h <= 0 {
		return ""
	}
	ratio := w / h
	switch {
	case ratio > 0.95 && ratio < 1.05:
		return "1:1"
	case ratio > 1.7 && ratio < 1.85:
		return "16:9"
	case ratio > 0.54 && ratio < 0.6:
		return "9:16"
	case ratio > 1.25 && ratio < 1.4:
		return "4:3"
	case ratio > 0.7 && ratio < 0.8:
		return "3:4"
	default:
		return fmt.Sprintf("%.0f:%.0f", w, h)
	}
}

// FormatGeneratedImagesMarkdown builds assistant markdown with image embeds.
func FormatGeneratedImagesMarkdown(prompt string, res *ImageGenResult) string {
	if res == nil || len(res.Images) == 0 {
		return ""
	}
	var b strings.Builder
	p := strings.TrimSpace(prompt)
	if i := strings.Index(p, "\n## "); i > 0 {
		p = strings.TrimSpace(p[:i])
	}
	if len(p) > 160 {
		p = p[:157] + "…"
	}
	if p != "" {
		b.WriteString("Generated image")
		if len(res.Images) > 1 {
			b.WriteString("s")
		}
		b.WriteString(" for: ")
		b.WriteString(p)
		b.WriteString("\n\n")
	}
	for i, img := range res.Images {
		src := ""
		if img.URL != "" {
			src = img.URL
		} else if img.B64JSON != "" {
			// Assume png when provider returns bare b64.
			src = "data:image/png;base64," + img.B64JSON
		}
		if src == "" {
			continue
		}
		alt := fmt.Sprintf("generated-%d", i+1)
		b.WriteString(fmt.Sprintf("![%s](%s)\n\n", alt, src))
	}
	return strings.TrimSpace(b.String())
}
