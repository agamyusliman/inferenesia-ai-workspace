package browser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Sensitive patterns must never appear in LLM-facing tool results (VAL-BRW-006).
// Only page text / screenshot *paths* are returned to the model — never cookies,
// auth tokens, storage state, or raw profile dumps.

var (
	// cookiePair matches "name=value" cookie segments and "cookie: …" header-ish lines.
	reCookieAssign = regexp.MustCompile(`(?i)\b(?:set-)?cookie\s*[:=]\s*[^\s;,"']+`)
	reCookieKV     = regexp.MustCompile(`(?i)\b(?:sessionid|sid|ssid|auth|token|jwt|access_token|refresh_token|csrf|csrftoken|__session|connect\.sid)\s*=\s*[^\s;,"']+`)
	// authorization headers / bearer tokens
	reAuthHeader = regexp.MustCompile(`(?i)\b(?:authorization|proxy-authorization)\s*[:=]\s*(?:bearer\s+)?[A-Za-z0-9._\-+/=]{8,}`)
	reBearer     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._\-+/=]{8,}`)
	// storage state / localStorage dumps
	reStorageKey = regexp.MustCompile(`(?i)\b(?:localStorage|sessionStorage|indexedDB|storage_state|storageState)\b`)
	// document.cookie explicit dumps
	reDocumentCookie = regexp.MustCompile(`(?i)document\.cookie\s*=\s*['"][^'"]*['"]`)
	// generic high-entropy secret-looking tokens near auth words
	reSecretish = regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|password|passwd|pwd)\s*[:=]\s*['"]?[^\s,'"]{6,}`)
)

// SanitizeForLLM redacts cookies, auth tokens, and storage-state dumps from a
// string that will be handed to the agent loop / model (VAL-BRW-006).
// Page text and non-sensitive content are preserved; only sensitive fragments
// are replaced with redaction markers.
func SanitizeForLLM(s string) string {
	if s == "" {
		return s
	}
	out := s
	out = reDocumentCookie.ReplaceAllString(out, "document.cookie=[REDACTED]")
	out = reCookieAssign.ReplaceAllString(out, "cookie=[REDACTED]")
	out = reCookieKV.ReplaceAllStringFunc(out, func(m string) string {
		// Keep key name for debugging, strip value.
		if i := strings.Index(m, "="); i >= 0 {
			return m[:i+1] + "[REDACTED]"
		}
		return "[REDACTED]"
	})
	out = reAuthHeader.ReplaceAllString(out, "authorization=[REDACTED]")
	out = reBearer.ReplaceAllString(out, "Bearer [REDACTED]")
	out = reSecretish.ReplaceAllStringFunc(out, func(m string) string {
		if i := strings.IndexAny(m, ":="); i >= 0 {
			return m[:i+1] + " [REDACTED]"
		}
		return "[REDACTED]"
	})
	// If storage APIs are mentioned together with large base64-ish blobs, soften.
	if reStorageKey.MatchString(out) {
		// Collapse obvious JSON storage dumps (keys like cookies arrays).
		out = redactStorageJSON(out)
	}
	return out
}

// redactStorageJSON replaces JSON-ish blobs that look like Playwright storageState.
func redactStorageJSON(s string) string {
	// Heuristic: if payload looks like {"cookies":[...]} strip cookies array values.
	if !strings.Contains(strings.ToLower(s), `"cookies"`) &&
		!strings.Contains(strings.ToLower(s), `"origins"`) {
		return s
	}
	// Replace cookie value fields inside JSON.
	reJSONCookieVal := regexp.MustCompile(`(?i)("(?:value|cookie|token)"\s*:\s*)"[^"]*"`)
	return reJSONCookieVal.ReplaceAllString(s, `${1}"[REDACTED]"`)
}

// SanitizeEvalResult prepares Evaluate output for the model: stringifies then
// redacts sensitive fragments. Binary/complex values become a short type note.
func SanitizeEvalResult(res EvalResult) string {
	if res.Raw != "" {
		return SanitizeForLLM(res.Raw)
	}
	if res.Value == nil {
		return "null"
	}
	switch v := res.Value.(type) {
	case string:
		return SanitizeForLLM(v)
	case float64, float32, int, int64, int32, bool:
		return fmt.Sprint(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return SanitizeForLLM(fmt.Sprint(v))
		}
		return SanitizeForLLM(string(b))
	}
}

// ContainsSensitive reports whether s looks like it still carries cookies/auth
// (used in tests / defensive checks before tool results leave the sandbox).
func ContainsSensitive(s string) bool {
	if s == "" {
		return false
	}
	// After sanitize we expect these markers only as redactions; detect real leaks.
	if reCookieKV.MatchString(s) && !strings.Contains(s, "[REDACTED]") {
		return true
	}
	if reAuthHeader.MatchString(s) && !strings.Contains(s, "[REDACTED]") {
		return true
	}
	if reBearer.MatchString(s) && !strings.Contains(s, "[REDACTED]") {
		return true
	}
	// literal cookie=name=value without redaction
	low := strings.ToLower(s)
	if strings.Contains(low, "cookie=") && !strings.Contains(s, "[REDACTED]") {
		// Allow the word "cookie" in prose; require assignment-like shape.
		if reCookieAssign.MatchString(s) {
			return true
		}
	}
	return false
}

// ScreenshotResultForLLM returns a tool payload for screenshots: path + byte size,
// never base64 image bytes or cookies (VAL-BRW-006).
func ScreenshotResultForLLM(path string, nbytes int, engine EngineID) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return SanitizeForLLM(fmt.Sprintf("screenshot captured (%d bytes) engine=%s", nbytes, engine))
	}
	return SanitizeForLLM(fmt.Sprintf("screenshot_path=%s bytes=%d engine=%s", path, nbytes, engine))
}

// PageTextResultForLLM wraps extracted page text after sanitization.
func PageTextResultForLLM(text string) string {
	return SanitizeForLLM(text)
}
