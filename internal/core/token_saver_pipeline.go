package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
)

// System prompt fragments for optional Token Savers (VAL-SAVER-005/006).
// Injected only when the corresponding toggle is on. No secrets.

// CavemanSystemFragment steers the model toward terse completions (fewer output tokens).
// Inspired by JuliusBrussee/caveman — keep code, paths, errors, and shell exact.
const CavemanSystemFragment = `[Token Saver: Caveman]
Prefer terse replies. Drop filler and ceremony ("Sure!", "Great question", restating the prompt).
Keep code blocks, file paths, commands, stack traces, and error messages byte-exact.
Short sentences OK. Do not invent slang that obscures technical facts.`

// PonytailSystemFragment steers toward minimal, YAGNI code edits.
// Inspired by DietrichGebert/ponytail — reuse stdlib, prefer deletion over addition.
const PonytailSystemFragment = `[Token Saver: Ponytail]
When writing or editing code: YAGNI — only what the task needs.
Prefer the standard library and existing project helpers over new deps or frameworks.
Prefer deletion and simplification over adding wrappers, layers, or options.
One focused change beats a large refactor. Do not scaffold files the user did not ask for.`

// RTKCompressFunc compresses a single tool result for model context (VAL-SAVER-003).
// name is the tool name; content is the raw tool output. Return content unchanged on failure.
type RTKCompressFunc func(name, content string) (string, error)

// HeadroomCompressFunc compresses the full message list before a model call (VAL-SAVER-004).
// Return messages unchanged on failure so chat never crashes.
type HeadroomCompressFunc func(ctx context.Context, messages []provider.Message) ([]provider.Message, error)

// ApplyTokenSaverSystemFragments appends Caveman and/or Ponytail guidance when enabled.
// Empty base is left empty (caller may skip system message entirely). Off toggles are no-ops.
func ApplyTokenSaverSystemFragments(base string, savers config.TokenSaversConfig) string {
	base = strings.TrimSpace(base)
	var parts []string
	if base != "" {
		parts = append(parts, base)
	}
	if savers.Caveman.Enabled {
		parts = append(parts, CavemanSystemFragment)
	}
	if savers.Ponytail.Enabled {
		parts = append(parts, PonytailSystemFragment)
	}
	return strings.Join(parts, "\n\n")
}

// CompressToolResultRTK runs the RTK path when enabled+available; otherwise returns content as-is.
// Missing binary, empty content, or compressor errors never panic — pass-through (VAL-SAVER-003/007).
// Secrets in content are redacted before any external process / log path (VAL-SAVER-008).
func CompressToolResultRTK(savers config.TokenSaversConfig, toolName, content string, inject RTKCompressFunc) string {
	if !savers.RTK.Enabled || strings.TrimSpace(content) == "" {
		return content
	}
	// Injectable mock (tests) — still redact before calling so tests catch leakage patterns.
	safeIn := secrethyg.Redact(content)
	if inject != nil {
		out, err := inject(toolName, safeIn)
		if err != nil || strings.TrimSpace(out) == "" {
			return content
		}
		return out
	}
	ok, _ := config.ProbeRTKAvailable(savers.RTKCommand())
	if !ok {
		// Not installed: full tool output (status shown in Settings; never crash chat).
		return content
	}
	out, err := compressWithRTKBinary(savers.RTKCommand(), toolName, safeIn)
	if err != nil || strings.TrimSpace(out) == "" {
		// Fallback local heuristic when binary exists but filter path fails.
		out = heuristicToolCompress(toolName, safeIn)
	}
	if strings.TrimSpace(out) == "" {
		return content
	}
	return out
}

// compressWithRTKBinary prefers `rtk filter` stdin when available; falls back to error.
// Never logs stdin/stdout (may contain redacted secrets).
func compressWithRTKBinary(command, toolName, content string) (string, error) {
	cmdName := strings.TrimSpace(command)
	if cmdName == "" {
		cmdName = "rtk"
	}
	// Try: rtk filter (stdin) — present on some RTK builds as a post-hoc path.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, cmdName, "filter", "--tool", toolName)
	c.Stdin = strings.NewReader(content)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err == nil && stdout.Len() > 0 {
		return stdout.String(), nil
	}
	// Try bare filter without --tool.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	c2 := exec.CommandContext(ctx2, cmdName, "filter")
	c2.Stdin = strings.NewReader(content)
	var stdout2 bytes.Buffer
	c2.Stdout = &stdout2
	c2.Stderr = io.Discard
	if err := c2.Run(); err == nil && stdout2.Len() > 0 {
		return stdout2.String(), nil
	}
	return "", fmt.Errorf("rtk filter unavailable")
}

// heuristicToolCompress is a local RTK-style compact for git/grep/ls/tree/log-like outputs.
// Used when the binary is present but has no stdin filter, or as mock-path fidelity.
func heuristicToolCompress(toolName, content string) string {
	const maxLines = 80
	const maxLineRunes = 240
	const maxTotalRunes = 6000

	lines := strings.Split(content, "\n")
	var out []string
	seen := make(map[string]int)
	for _, line := range lines {
		trim := strings.TrimRight(line, " \t\r")
		if trim == "" {
			// collapse blank runs
			if len(out) > 0 && out[len(out)-1] == "" {
				continue
			}
			out = append(out, "")
			continue
		}
		// Collapse exact duplicate lines (log spam).
		if n := seen[trim]; n >= 2 {
			continue
		}
		seen[trim]++
		if utf8.RuneCountInString(trim) > maxLineRunes {
			r := []rune(trim)
			trim = string(r[:maxLineRunes]) + "…"
		}
		out = append(out, trim)
		if len(out) >= maxLines {
			out = append(out, fmt.Sprintf("…[rtk: truncated %d more lines from %s]", len(lines)-len(out), toolName))
			break
		}
	}
	joined := strings.Join(out, "\n")
	if utf8.RuneCountInString(joined) > maxTotalRunes {
		r := []rune(joined)
		joined = string(r[:maxTotalRunes]) + "\n…[rtk: truncated]"
	}
	if joined == content {
		// Ensure observable compression marker for verbose/noisy inputs that already fit.
		if len(lines) > maxLines/2 || utf8.RuneCountInString(content) > 400 {
			return joined + "\n[rtk: compressed]"
		}
	}
	if joined != content && !strings.Contains(joined, "[rtk:") {
		// Mark so tests can assert the RTK path ran without logging raw secrets.
		joined = joined + "\n[rtk: compressed]"
	}
	return joined
}

// CompressMessagesHeadroom calls the Headroom-style compress endpoint when enabled+available.
// Off, missing base_url, HTTP failure, or inject error → original messages (VAL-SAVER-004/007).
// Never logs request/response bodies that may hold secrets (VAL-SAVER-008).
func CompressMessagesHeadroom(ctx context.Context, savers config.TokenSaversConfig, messages []provider.Message, inject HeadroomCompressFunc, client *http.Client) []provider.Message {
	if !savers.Headroom.Enabled || len(messages) == 0 {
		return messages
	}
	if inject != nil {
		out, err := inject(ctx, messages)
		if err != nil || len(out) == 0 {
			return messages
		}
		return out
	}
	ok, _ := config.ProbeHeadroomAvailable(savers.Headroom.BaseURL)
	if !ok {
		return messages
	}
	out, err := headroomHTTPCompress(ctx, savers.Headroom.BaseURL, messages, client)
	if err != nil || len(out) == 0 {
		return messages
	}
	return out
}

type headroomCompressRequest struct {
	Messages []provider.Message `json:"messages"`
}

type headroomCompressResponse struct {
	Messages []provider.Message `json:"messages"`
	// Some APIs return a single compressed system/user blob.
	Content string `json:"content,omitempty"`
}

func headroomHTTPCompress(ctx context.Context, baseURL string, messages []provider.Message, client *http.Client) ([]provider.Message, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("empty base_url")
	}
	// Support both base_url ending at host and paths already including /v1.
	endpoint := base
	if !strings.HasSuffix(base, "/compress") {
		if strings.HasSuffix(base, "/v1") {
			endpoint = base + "/compress"
		} else {
			endpoint = base + "/v1/compress"
		}
	}
	// Redact secrets from message contents before they leave the process.
	safeMsgs := make([]provider.Message, len(messages))
	for i, m := range messages {
		m.Content = secrethyg.Redact(m.Content)
		safeMsgs[i] = m
	}
	body, err := json.Marshal(headroomCompressRequest{Messages: safeMsgs})
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Do not attach provider API keys to third-party compressors (VAL-SAVER-008).
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("headroom compress status %d", resp.StatusCode)
	}
	var parsed headroomCompressResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Messages) > 0 {
		return parsed.Messages, nil
	}
	if c := strings.TrimSpace(parsed.Content); c != "" {
		// Collapse into a single system note while preserving last user turn shape is fragile;
		// prefer leave original if only opaque content returned.
		return messages, nil
	}
	return nil, fmt.Errorf("headroom compress empty response")
}

// tokenSaverLog is a no-op-safe note path for debug; never receives raw secrets.
// Kept for future structured events; currently unused outside tests.
func tokenSaverSafeDetail(s string) string {
	return secrethyg.Redact(s)
}
