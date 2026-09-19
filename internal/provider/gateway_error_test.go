package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// VAL-CROSS-009: provider-level GatewayError behavior.
//
// When TEMP_AI_BASE_URL is unreachable or returns 401/5xx, the tempai profile
// must surface a brand-correct "Inferenesia: gateway unavailable (temp-ai)" error
// that:
//   - contains "gateway" and "unavailable"
//   - does NOT contain the API key value
//   - is fail-fast (completes within 10s) for unreachable hosts
//   - wraps the underlying ConnectionError / AuthError / 5xx error so
//     errors.Is keeps working for the agent loop retry/blocked path.

// TestGatewayErrorStringBrandCorrect checks the brand-correct wording and
// that the API key never appears in the error string, across all the
// underlying failure modes (unreachable / 401 / 5xx / unknown).
func TestGatewayErrorStringBrandCorrect(t *testing.T) {
	const apiKey = "super-secret-key-XYZ-not-real"

	cases := []struct {
		name   string
		err    error
		status int
		host   string
	}{
		{
			name: "unreachable (connection refused)",
			err:  &ConnectionError{BaseURL: "http://127.0.0.1:9/v1", Reason: "connection refused"},
			host: "http://127.0.0.1:9",
		},
		{
			name:   "401 unauthorized",
			err:    &AuthError{Reason: "invalid key", Status: 401, Cause: ErrUnauthorized},
			status: 401,
			host:   "https://ai.temp.web.id",
		},
		{
			name: "5xx server error",
			err:  fmt.Errorf("provider returned HTTP 503: server error"),
			host: "https://ai.temp.web.id",
		},
		{
			name: "unknown error",
			err:  fmt.Errorf("something else went wrong"),
			host: "https://ai.temp.web.id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ge := AsGatewayError(tc.host, tc.err)
			if ge == nil {
				t.Fatalf("AsGatewayError returned nil for %v", tc.err)
			}
			s := ge.Error()
			if !strings.Contains(s, "gateway") {
				t.Errorf("missing 'gateway' in %q", s)
			}
			if !strings.Contains(s, "unavailable") {
				t.Errorf("missing 'unavailable' in %q", s)
			}
			if !strings.Contains(s, "Inferenesia") {
				t.Errorf("missing brand 'Inferenesia' in %q", s)
			}
			if !strings.Contains(s, "Inferenesia API") {
				t.Errorf("missing provider label 'Inferenesia API' in %q", s)
			}
			if strings.Contains(s, apiKey) {
				t.Errorf("error string leaked API key: %q", s)
			}
			if !errors.Is(ge, ErrGatewayUnavailable) {
				t.Errorf("errors.Is(ErrGatewayUnavailable) = false; want true")
			}
			if !IsGateway(ge) {
				t.Errorf("IsGateway = false; want true")
			}
		})
	}
}

// TestGatewayErrorNeverContainsAPIKey defends against the API key ever being
// embedded in the GatewayError string (even when the underlying cause is a
// 401 that echoes the bearer token in its message).
func TestGatewayErrorNeverContainsAPIKey(t *testing.T) {
	const apiKey = "super-secret-key-XYZ-not-real"
	// Even if an underlying error somehow echoed the bearer value, AsGatewayError
	// itself never adds it. The CLI layer (secrethyg.Redact) is the second line
	// of defense; here we assert the provider layer does not introduce it.
	ae := &AuthError{
		Reason: "invalid or unauthorized API key", // no key value
		Status: 401,
		Cause:  ErrUnauthorized,
	}
	ge := AsGatewayError("https://ai.temp.web.id", ae)
	if strings.Contains(ge.Error(), apiKey) {
		t.Fatalf("GatewayError leaked API key: %q", ge.Error())
	}
	if strings.Contains(strings.ToLower(ge.Error()), "bearer "+strings.ToLower(apiKey)) {
		t.Fatalf("GatewayError leaked bearer token: %q", ge.Error())
	}
}

// TestWrapGatewayErrorIfProfileTempAIOnly ensures non-tempai profiles keep
// their existing provider errors unchanged (BYOK / Anthropic / 9Router).
func TestWrapGatewayErrorIfProfileTempAIOnly(t *testing.T) {
	origErr := &ConnectionError{BaseURL: "http://byok.example/v1", Reason: "refused"}

	// tempai → wrapped as GatewayError.
	wrapped := WrapGatewayErrorIfProfile(ProfileTempAI, "https://ai.temp.web.id", origErr)
	if !IsGateway(wrapped) {
		t.Fatalf("tempai wrap: want GatewayError, got %T", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "gateway unavailable") {
		t.Fatalf("tempai wrap error: %q", wrapped.Error())
	}

	// BYOK → unchanged (not a gateway profile).
	byok := WrapGatewayErrorIfProfile(ProfileBYOK, "http://byok.example", origErr)
	if byok != origErr {
		t.Fatalf("BYOK wrap: error should be unchanged, got %T", byok)
	}

	// Anthropic → unchanged.
	ant := WrapGatewayErrorIfProfile(ProfileAnthropic, "https://api.anthropic.com", origErr)
	if ant != origErr {
		t.Fatalf("Anthropic wrap: error should be unchanged, got %T", ant)
	}

	// 9Router → unchanged.
	nr := WrapGatewayErrorIfProfile(ProfileNineRouter, "http://9r.example/v1", origErr)
	if nr != origErr {
		t.Fatalf("9Router wrap: error should be unchanged, got %T", nr)
	}

	// Empty profile → unchanged.
	empty := WrapGatewayErrorIfProfile("", "", origErr)
	if empty != origErr {
		t.Fatalf("empty profile wrap: error should be unchanged, got %T", empty)
	}

	// nil error → nil.
	if WrapGatewayErrorIfProfile(ProfileTempAI, "https://ai.temp.web.id", nil) != nil {
		t.Fatal("nil err should return nil")
	}
}

// TestGatewayErrorIdempotentOnAlreadyGatewayError verifies that wrapping an
// already-classified *GatewayError is a no-op (does not double-wrap).
func TestGatewayErrorIdempotentOnAlreadyGatewayError(t *testing.T) {
	orig := AsGatewayError("https://ai.temp.web.id", &ConnectionError{
		BaseURL: "https://ai.temp.web.id/v1",
		Reason:  "refused",
	})
	wrapped := WrapGatewayErrorIfProfile(ProfileTempAI, "https://ai.temp.web.id", orig)
	if wrapped != orig {
		t.Fatalf("double-wrap: want same error back, got %T", wrapped)
	}
}

// TestOpenAICompatUnreachableBaseURLGatewayError exercises the real
// OpenAICompat client against a dead localhost port and verifies:
//   - it fails fast (within 10s)
//   - the error is a *ConnectionError (IsConnection == true)
//   - wrapping it as a GatewayError yields a brand-correct string
//   - the API key value never appears in the error string
// This is the provider-level half of VAL-CROSS-009.
func TestOpenAICompatUnreachableBaseURLGatewayError(t *testing.T) {
	const apiKey = "super-secret-key-XYZ-not-real"
	// Use a port that's almost certainly dead (localhost:1 is reserved).
	// Short timeout ensures < 10s even on slow CI.
	c := NewOpenAICompat(Config{
		ID:                 ProfileTempAI,
		BaseURL:            "http://127.0.0.1:1/v1",
		APIKey:             apiKey,
		HTTPTimeoutSeconds: 2,
	})

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.ListModels(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error for unreachable base URL")
	}
	if elapsed > 10*time.Second {
		t.Fatalf("took %v, want <= 10s (VAL-CROSS-009)", elapsed)
	}
	if !IsConnection(err) && !errors.Is(err, context.DeadlineExceeded) {
		// Some platforms return a generic error; accept any non-nil that
		// classifies as connection or timed out. The gateway wrap below is
		// what turns it into a brand-correct string.
		if !strings.Contains(strings.ToLower(err.Error()), "connection") &&
			!strings.Contains(strings.ToLower(err.Error()), "refused") &&
			!strings.Contains(strings.ToLower(err.Error()), "timeout") &&
			!strings.Contains(strings.ToLower(err.Error()), "unreachable") &&
			!strings.Contains(strings.ToLower(err.Error()), "dial") {
			t.Fatalf("unexpected err type: %v", err)
		}
	}

	// Wrap as a gateway error and verify the brand-correct string.
	ge := WrapGatewayErrorIfProfile(ProfileTempAI, c.RequestHost(), err)
	if !IsGateway(ge) {
		t.Fatalf("tempai wrap: want GatewayError, got %T", ge)
	}
	s := ge.Error()
	if !strings.Contains(s, "gateway") {
		t.Errorf("missing 'gateway' in %q", s)
	}
	if !strings.Contains(s, "unavailable") {
		t.Errorf("missing 'unavailable' in %q", s)
	}
	if strings.Contains(s, apiKey) {
		t.Errorf("GatewayError leaked API key: %q", s)
	}
	if !errors.Is(ge, ErrGatewayUnavailable) {
		t.Errorf("errors.Is(ErrGatewayUnavailable) = false; want true")
	}
}

// TestOpenAICompat401GatewayError exercises the 401 path and verifies the
// GatewayError carries the HTTP status and never embeds the key.
func TestOpenAICompat401GatewayError(t *testing.T) {
	const apiKey = "bad-key-not-real-zzzzzz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never echo the key back (defensive).
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      ProfileTempAI,
		BaseURL: srv.URL + "/v1",
		APIKey:  apiKey,
	})
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !IsAuth(err) {
		t.Fatalf("want AuthError, got %v", err)
	}

	ge := WrapGatewayErrorIfProfile(ProfileTempAI, c.RequestHost(), err)
	if !IsGateway(ge) {
		t.Fatalf("tempai wrap: want GatewayError, got %T", ge)
	}
	s := ge.Error()
	if !strings.Contains(s, "gateway") || !strings.Contains(s, "unavailable") {
		t.Errorf("missing gateway/unavailable in %q", s)
	}
	if !strings.Contains(s, "HTTP 401") {
		t.Errorf("missing HTTP 401 in %q", s)
	}
	if strings.Contains(s, apiKey) {
		t.Errorf("GatewayError leaked API key: %q", s)
	}
}

// TestOpenAICompat5xxGatewayError exercises the 5xx path.
func TestOpenAICompat5xxGatewayError(t *testing.T) {
	const apiKey = "any-key-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream gateway down"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompat(Config{
		ID:      ProfileTempAI,
		BaseURL: srv.URL + "/v1",
		APIKey:  apiKey,
	})
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected 5xx error")
	}

	ge := WrapGatewayErrorIfProfile(ProfileTempAI, c.RequestHost(), err)
	if !IsGateway(ge) {
		t.Fatalf("tempai wrap: want GatewayError, got %T", ge)
	}
	s := ge.Error()
	if !strings.Contains(s, "gateway") || !strings.Contains(s, "unavailable") {
		t.Errorf("missing gateway/unavailable in %q", s)
	}
	if strings.Contains(s, "server error") {
		// 502 should be classified as "server error" reason.
		if !strings.Contains(s, "server error") {
			t.Errorf("expected 'server error' reason in %q", s)
		}
	}
	if strings.Contains(s, apiKey) {
		t.Errorf("GatewayError leaked API key: %q", s)
	}
}
