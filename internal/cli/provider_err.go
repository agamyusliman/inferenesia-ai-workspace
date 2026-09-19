package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
)

// formatProviderErr turns provider errors into concise, secret-free CLI errors.
// Gateway-down errors (tempai unreachable / 401 / 5xx) are surfaced as
// brand-correct "Inferenesia: gateway unavailable (temp-ai) — <reason>" so the
// operator can retry or switch profile (VAL-CROSS-009).
func formatProviderErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("interrupted")
	}
	// Already a brand-correct GatewayError: keep as-is (still redact defensively).
	var gwErr *provider.GatewayError
	if errors.As(err, &gwErr) && gwErr != nil {
		return fmt.Errorf("%s", secrethyg.Redact(gwErr.Error()))
	}
	msg := secrethyg.Redact(err.Error())

	switch {
	case provider.IsBlocked(err):
		// VAL-PROV-008: clear blocked / key required (Anthropic or any profile).
		return fmt.Errorf("%s", msg)
	case errors.Is(err, provider.ErrMissingAPIKey):
		// Generic message covers temp-ai (TEMP_AI_API_KEY), BYOK, and Anthropic.
		// Avoid "KEY=value" phrasing so secret redaction does not treat trailing text as a leak.
		return fmt.Errorf("blocked: key required (set TEMP_AI_API_KEY, ANTHROPIC_API_KEY, YURA_AI_BYOK_API_KEY, or profile api_key; BYOK is not proxied through temp-ai)")
	case provider.IsAuth(err):
		return fmt.Errorf("authentication failed (invalid or unauthorized API key): %s", msg)
	case provider.IsConnection(err):
		return fmt.Errorf("connection error: %s", msg)
	}

	var me *provider.ModelError
	if errors.As(err, &me) {
		return fmt.Errorf("%s", msg)
	}

	if strings.Contains(strings.ToLower(msg), "context canceled") {
		return fmt.Errorf("interrupted")
	}
	return fmt.Errorf("%s", msg)
}

// wrapGatewayErrIfTempAI wraps err as a brand-correct *provider.GatewayError
// when profileName is the temp-ai gateway profile (VAL-CROSS-009). Non-tempai
// profiles (BYOK / Anthropic / 9Router) return err unchanged. host is the
// request host (no key) included in the error string. Returns nil when err
// is nil.
func wrapGatewayErrIfTempAI(profileName, host string, err error) error {
	if err == nil {
		return nil
	}
	return provider.WrapGatewayErrorIfProfile(profileName, host, err)
}
