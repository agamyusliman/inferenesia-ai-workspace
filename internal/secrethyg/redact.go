// Package secrethyg provides redaction helpers so API keys and bearer tokens
// never appear in CLI output or logs. Named "secrethyg" (not "secrets") so
// the directory is not caught by the root .gitignore *secrets* pattern.
package secrethyg

import (
	"os"
	"regexp"
	"strings"
)

// Common patterns that may appear in logs or error messages.
var (
	reBearer      = regexp.MustCompile(`(?i)(Authorization\s*[:=]\s*Bearer\s+)(\S+)`)
	reBearerValue = regexp.MustCompile(`(?i)(Bearer\s+)([A-Za-z0-9._\-]{8,})`)
	reAPIKeyLine  = regexp.MustCompile(`(?i)(TEMP_AI_API_KEY|API[_-]?KEY|api[_-]?key)\s*[=:]\s*([^\s"'\\]+)`)
	reSkPrefix    = regexp.MustCompile(`\b(sk-[A-Za-z0-9_\-]{8,})\b`)
	reTempKey     = regexp.MustCompile(`\b(temp-[A-Za-z0-9]{16,})\b`)
)

// Redact replaces sensitive token/key material in s with redacted placeholders.
// It is safe to call on empty strings and already-redacted input.
func Redact(s string) string {
	if s == "" {
		return s
	}
	// Prefer env-known values first (exact substring) before pattern masking.
	// Includes NINEROUTER_KEY so optional 9Router kit tools never echo secrets (VAL-KIT-010).
	for _, envName := range []string{
		"TEMP_AI_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENROUTER_API_KEY",
		"NINEROUTER_KEY", "YURA_AI_BYOK_API_KEY",
	} {
		if v := strings.TrimSpace(os.Getenv(envName)); v != "" && len(v) >= 8 {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	s = reBearer.ReplaceAllString(s, `${1}[REDACTED]`)
	s = reBearerValue.ReplaceAllString(s, `${1}[REDACTED]`)
	s = reAPIKeyLine.ReplaceAllString(s, `${1}=[REDACTED]`)
	s = reSkPrefix.ReplaceAllString(s, `[REDACTED]`)
	s = reTempKey.ReplaceAllString(s, `[REDACTED]`)
	return s
}

// ContainsSecretLike reports whether text appears to hold a key or bearer token.
// Used to decide whether a config file should be mode 0600.
func ContainsSecretLike(s string) bool {
	if s == "" {
		return false
	}
	// Inline key assignments that look like real values (not empty / placeholders).
	if reAPIKeyLine.MatchString(s) {
		// Ignore empty or clearly placeholder values.
		matches := reAPIKeyLine.FindAllStringSubmatch(s, -1)
		for _, m := range matches {
			if len(m) < 3 {
				continue
			}
			val := strings.TrimSpace(m[2])
			if val == "" || val == `""` || val == `''` || strings.EqualFold(val, "null") ||
				strings.HasPrefix(val, "${") || strings.EqualFold(val, "changeme") {
				continue
			}
			// Pure env-var reference: api_key_env: TEMP_AI_API_KEY is not a secret value.
			if strings.EqualFold(m[1], "api_key_env") || strings.HasSuffix(strings.ToLower(m[1]), "api_key_env") {
				continue
			}
			if !strings.Contains(val, "API_KEY") && len(val) >= 8 {
				return true
			}
		}
	}
	if reBearer.MatchString(s) || reBearerValue.MatchString(s) {
		return true
	}
	if reSkPrefix.MatchString(s) || reTempKey.MatchString(s) {
		return true
	}
	// YAML-style api_key: <value>
	reYAMLKey := regexp.MustCompile(`(?m)^\s*(api_key|apiKey|token|access_token|secret)\s*:\s*['"]?([^\s#'"]{8,})['"]?`)
	if reYAMLKey.MatchString(s) {
		for _, m := range reYAMLKey.FindAllStringSubmatch(s, -1) {
			if len(m) < 3 {
				continue
			}
			val := m[2]
			if strings.HasPrefix(val, "${") || strings.Contains(strings.ToUpper(val), "API_KEY") {
				continue
			}
			return true
		}
	}
	return false
}
