package provider

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
)

// AuthError is a human-readable authentication failure that never embeds the key.
type AuthError struct {
	// Reason is a short operator-facing explanation (no secrets).
	Reason string
	// Status is the HTTP status when known (0 if not HTTP).
	Status int
	// Cause is the underlying error (optional).
	Cause error
}

func (e *AuthError) Error() string {
	if e == nil {
		return "authentication error"
	}
	if e.Status > 0 {
		return fmt.Sprintf("authentication failed (HTTP %d): %s", e.Status, e.Reason)
	}
	return fmt.Sprintf("authentication failed: %s", e.Reason)
}

func (e *AuthError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Cause != nil {
		return e.Cause
	}
	return ErrUnauthorized
}

// ConnectionError is a fail-fast network/DNS/timeout error to an unreachable base URL.
type ConnectionError struct {
	// BaseURL is the attempted host root (no secrets).
	BaseURL string
	// Reason is operator-facing.
	Reason string
	Cause  error
}

func (e *ConnectionError) Error() string {
	if e == nil {
		return "connection error"
	}
	if e.BaseURL != "" {
		return fmt.Sprintf("could not reach provider at %s: %s", e.BaseURL, e.Reason)
	}
	return fmt.Sprintf("could not reach provider: %s", e.Reason)
}

func (e *ConnectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ModelError reports an invalid or missing model selection.
type ModelError struct {
	Model  string
	Reason string
	Cause  error
}

func (e *ModelError) Error() string {
	if e == nil {
		return "model error"
	}
	if e.Model != "" {
		return fmt.Sprintf("model %q: %s", e.Model, e.Reason)
	}
	return fmt.Sprintf("model error: %s", e.Reason)
}

func (e *ModelError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Cause != nil {
		return e.Cause
	}
	return ErrModelNotFound
}

// classifyTransport maps low-level transport failures into ConnectionError.
func classifyTransport(baseURL string, err error) error {
	if err == nil {
		return nil
	}
	// Preserve already-classified errors.
	var ae *AuthError
	if errors.As(err, &ae) {
		return err
	}
	var ce *ConnectionError
	if errors.As(err, &ce) {
		return err
	}
	var me *ModelError
	if errors.As(err, &me) {
		return err
	}

	msg := err.Error()
	lower := strings.ToLower(msg)

	// url.Error wrapping dial/DNS/timeout.
	var ue *url.Error
	if errors.As(err, &ue) {
		return &ConnectionError{
			BaseURL: baseURL,
			Reason:  humanNetReason(ue.Err, lower),
			Cause:   err,
		}
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return &ConnectionError{
			BaseURL: baseURL,
			Reason:  humanNetReason(ne, lower),
			Cause:   err,
		}
	}
	if strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "i/o timeout") ||
		strings.Contains(lower, "deadline exceeded") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "tls") ||
		strings.Contains(lower, "certificate") ||
		strings.Contains(lower, "dial tcp") {
		return &ConnectionError{
			BaseURL: baseURL,
			Reason:  humanNetReason(err, lower),
			Cause:   err,
		}
	}
	return err
}

func humanNetReason(err error, lower string) string {
	if err == nil {
		return "connection failed"
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return "connection timed out"
	}
	switch {
	case strings.Contains(lower, "no such host"):
		return "DNS lookup failed (no such host)"
	case strings.Contains(lower, "connection refused"):
		return "connection refused"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "connection timed out"
	case strings.Contains(lower, "network is unreachable"):
		return "network unreachable"
	default:
		// Do not echo raw URLs with credentials; strip likely key fragments.
		return "connection failed"
	}
}

// IsAuth reports whether err is an auth / missing-key failure.
func IsAuth(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrMissingAPIKey) || errors.Is(err, ErrUnauthorized) {
		return true
	}
	var ae *AuthError
	return errors.As(err, &ae)
}

// IsConnection reports whether err is a transport / unreachable base URL failure.
func IsConnection(err error) bool {
	if err == nil {
		return false
	}
	var ce *ConnectionError
	return errors.As(err, &ce)
}

// IsGateway reports whether err is a gateway-down / gateway-unavailable failure
// for the temp-ai (or registered gateway) profile. The error string is
// brand-correct and contains "gateway"/"unavailable"; the API key value is
// never embedded (VAL-CROSS-009).
func IsGateway(err error) bool {
	if err == nil {
		return false
	}
	var ge *GatewayError
	return errors.As(err, &ge)
}

// GatewayError is a brand-correct "gateway unavailable" error for the Inferenesia
// API gateway profile (the mission-required gateway). It wraps the underlying
// transport/auth/5xx failure so the agent loop can offer a retry/blocked path
// without hanging, faking success, or printing the API key (VAL-CROSS-009).
//
// The provider label (brand.ProviderGateway = "Inferenesia API") appears only as
// the provider label; the product brand is Inferenesia. Error string contains
// "gateway"/"unavailable".
type GatewayError struct {
	// Reason is the underlying failure mode in operator-facing language.
	// One of: "unreachable", "unauthorized", "server error", "unknown".
	Reason string
	// Status is the HTTP status code when known (0 for transport failures).
	Status int
	// Host is the request host (no path, no key) for verbose/trace output.
	Host string
	// Cause is the underlying provider error (ConnectionError / AuthError /
	// generic HTTP 5xx error). Wrapped for errors.Is compatibility.
	Cause error
}

func (e *GatewayError) Error() string {
	if e == nil {
		return brand.Name + ": gateway unavailable (" + brand.ProviderGateway + ")"
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "unavailable"
	}
	// Brand-correct: "Inferenesia: gateway unavailable (Inferenesia API): <reason>".
	// Always contains "gateway" + "unavailable"; never embeds the API key.
	host := strings.TrimSpace(e.Host)
	if e.Status > 0 {
		if host != "" {
			return fmt.Sprintf("%s: gateway unavailable (%s) — %s (HTTP %d, host %s)",
				brand.Name, brand.ProviderGateway, reason, e.Status, host)
		}
		return fmt.Sprintf("%s: gateway unavailable (%s) — %s (HTTP %d)",
			brand.Name, brand.ProviderGateway, reason, e.Status)
	}
	if host != "" {
		return fmt.Sprintf("%s: gateway unavailable (%s) — %s (host %s)",
			brand.Name, brand.ProviderGateway, reason, host)
	}
	return fmt.Sprintf("%s: gateway unavailable (%s) — %s", brand.Name, brand.ProviderGateway, reason)
}

func (e *GatewayError) Unwrap() []error {
	if e == nil {
		return nil
	}
	out := make([]error, 0, 2)
	if e.Cause != nil {
		out = append(out, e.Cause)
	}
	// Always expose ErrGatewayUnavailable so errors.Is(ge, ErrGatewayUnavailable)
	// is true for the agent loop retry/blocked path (VAL-CROSS-009).
	out = append(out, ErrGatewayUnavailable)
	return out
}

// AsGatewayError returns a *GatewayError describing err for the temp-ai
// gateway profile (host optional, used for verbose/trace output). Returns nil
// when err is nil or when err is a missing-key / blocked-key error (those stay
// as their original config errors — the gateway is not "down", just
// unconfigured). The returned error is safe to print: it never contains the
// API key value (secrethyg still redacts defensively in the CLI layer).
//
// Use this to surface brand-correct gateway errors when the active profile is
// the temp-ai gateway (VAL-CROSS-009). Non-gateway profile failures keep their
// existing provider errors (BYOK / Anthropic).
func AsGatewayError(host string, err error) *GatewayError {
	if err == nil {
		return nil
	}
	// Missing-key / blocked-key errors are config issues, not gateway-down.
	// Keep them as their original errors so the operator sees "key required"
	// rather than "gateway unavailable" (VAL-CLI-008 / VAL-PROV-008).
	if errors.Is(err, ErrMissingAPIKey) || IsBlocked(err) {
		return nil
	}
	ge := &GatewayError{Host: host, Cause: err}
	switch {
	case IsConnection(err):
		ge.Reason = "unreachable"
	case IsAuth(err):
		// 401/403 from the gateway is still a "gateway unavailable" from the
		// user's perspective (the mission-required gateway is unusable until
		// the key is fixed). Keep "unauthorized" so the operator knows it is
		// a credential problem.
		ge.Reason = "unauthorized"
		if ae, ok := err.(*AuthError); ok && ae != nil {
			ge.Status = ae.Status
		}
	case errors.Is(err, ErrGatewayUnavailable):
		ge.Reason = "unavailable"
	default:
		// HTTP 5xx (or any other) → server error / unknown.
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "http 5"), strings.Contains(msg, "server error"),
			strings.Contains(msg, "502"), strings.Contains(msg, "503"),
			strings.Contains(msg, "504"), strings.Contains(msg, "500"):
			ge.Reason = "server error"
		default:
			ge.Reason = "unknown"
		}
	}
	return ge
}

// WrapGatewayErrorIfProfile wraps err as a brand-correct *GatewayError when
// profileID (or the profile's Name when id is empty) is the temp-ai gateway
// profile. Non-tempai profiles (BYOK / Anthropic / 9Router) return the
// original err unchanged so their provider-specific errors stay intact.
//
// host is the request host (no key) included in the error string for
// verbose/trace output. Returns nil when err is nil.
//
// Use this at the core.Service / CLI boundary to surface clear gateway errors
// for the mission-required temp-ai profile (VAL-CROSS-009). Never embeds the
// API key value.
func WrapGatewayErrorIfProfile(profileID, host string, err error) error {
	if err == nil {
		return nil
	}
	if !IsGatewayProfile(profileID) {
		return err
	}
	// Already a GatewayError: keep as-is (idempotent).
	var ge *GatewayError
	if errors.As(err, &ge) {
		return err
	}
	// Missing-key / blocked-key errors are config issues, not gateway-down:
	// AsGatewayError returns nil for them and we return the original err so
	// the operator sees "key required" rather than "gateway unavailable"
	// (VAL-CLI-008 / VAL-PROV-008). Transport / 401 / 5xx failures wrap as a
	// brand-correct GatewayError (VAL-CROSS-009).
	wrapped := AsGatewayError(host, err)
	if wrapped == nil {
		return err
	}
	return wrapped
}

// IsGatewayProfile reports whether profileID is the temp-ai gateway profile
// (the mission-required gateway). Used to decide whether to wrap provider
// errors as brand-correct GatewayError values (VAL-CROSS-009). BYOK,
// Anthropic, and 9Router profiles return false so their provider-specific
// errors stay intact.
func IsGatewayProfile(profileID string) bool {
	id := strings.TrimSpace(profileID)
	if id == "" {
		return false
	}
	return id == ProfileInferenesia || id == ProfileInfernesia || id == ProfileTempAI
}
