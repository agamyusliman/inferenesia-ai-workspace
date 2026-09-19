package secrethyg

import (
	"strings"
	"testing"
)

func TestRedactBearerAndAPIKey(t *testing.T) {
	in := "Authorization: Bearer super-secret-token-value\nTEMP_AI_API_KEY=temp-abcdef1234567890deadbeef\n"
	out := Redact(in)
	if strings.Contains(out, "super-secret-token-value") {
		t.Fatalf("bearer value not redacted: %q", out)
	}
	if strings.Contains(out, "temp-abcdef1234567890deadbeef") {
		t.Fatalf("api key value not redacted: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %q", out)
	}
}

func TestRedactEnvKnownValue(t *testing.T) {
	const secret = "unique-live-key-xyz-9999"
	t.Setenv("TEMP_AI_API_KEY", secret)
	in := "using key " + secret + " in request"
	out := Redact(in)
	if strings.Contains(out, secret) {
		t.Fatalf("env key ***** still present: %q", out)
	}
}

// VAL-KIT-010: NINEROUTER_KEY must be redacted from any echoed text.
func TestRedactNineRouterKey(t *testing.T) {
	const secret = "nr-kit-secret-key-abcdef12"
	t.Setenv("NINEROUTER_KEY", secret)
	in := "auth was Bearer " + secret + " and raw=" + secret
	out := Redact(in)
	if strings.Contains(out, secret) {
		t.Fatalf("NINEROUTER_KEY still present: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] marker: %q", out)
	}
}

func TestRedactSkPrefix(t *testing.T) {
	in := "openai key sk-abcdefghijklmnopqrstuvwxyz123456"
	out := Redact(in)
	if strings.Contains(out, "sk-abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("sk- key not redacted: %q", out)
	}
}

func TestContainsSecretLike(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"# no secrets", false},
		{"TEMP_AI_API_KEY=\n", false},
		{"api_key_env: TEMP_AI_API_KEY", false},
		{"TEMP_AI_API_KEY=temp-abcdef1234567890dead", true},
		{"api_key: sk-live-abcdefghijklmnopqrstuv", true},
		{"token: supersecrettokenvalue99", true},
		{"Authorization: Bearer abcdefghijklmnop", true},
	}
	for _, tc := range cases {
		if got := ContainsSecretLike(tc.in); got != tc.want {
			t.Errorf("ContainsSecretLike(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}
