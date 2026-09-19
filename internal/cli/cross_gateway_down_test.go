package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// VAL-CROSS-009 (CLI surface): gateway down → clear brand-correct error.
//
// The CLI must surface a clear, brand-correct error when TEMP_AI_BASE_URL is
// unreachable or returns 401/5xx. The error must:
//   - contain "gateway"/"unavailable" (after provider.WrapGatewayErrorIfProfile
//     is applied by the CLI formatProviderErr path);
//   - not hang indefinitely (completes within timeout <= 10s);
//   - not print the API key value;
//   - exit non-zero (not fake success);
//   - not produce a Go panic/stack trace.
//
// Scenarios:
//   1. dead localhost port (unreachable) — `inferenesia models` + `chat`
//   2. 401 unauthorized — `inferenesia models`
//   3. 5xx server error — `inferenesia models`

// runCLIWithGatewayEnv sets YURA_AI_HOME / TEMP_AI_* to isolated values, runs
// the given args via ExecuteWith, and returns (exitCode, stdout, stderr).
// chdir to an empty temp dir prevents repo .env re-injection.
func runCLIWithGatewayEnv(t *testing.T, args []string, baseURL, apiKey string) (int, string, string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("YURA_AI_HOME", home)
	t.Setenv("TEMP_AI_BASE_URL", baseURL)
	t.Setenv("TEMP_AI_API_KEY", apiKey)
	t.Setenv("TEMP_AI_MODEL", "")

	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	var out, errBuf bytes.Buffer
	code := ExecuteWith(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

// TestCrossArea_GatewayDown_UnreachableBaseURL_Models verifies `inferenesia
// models` against a dead localhost port fails fast with a brand-correct
// gateway error and never prints the API key.
func TestCrossArea_GatewayDown_UnreachableBaseURL_Models(t *testing.T) {
	const apiKey = "dead-port-cli-secret-XYZ"
	start := time.Now()
	code, out, errBuf := runCLIWithGatewayEnv(t, []string{"models"}, "http://127.0.0.1:1/v1", apiKey)
	elapsed := time.Since(start)

	if code == 0 {
		t.Fatalf("expected non-zero exit; out=%q err=%q", out, errBuf)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("took %v, want <= 10s (VAL-CROSS-009)", elapsed)
	}
	combined := out + errBuf
	lc := strings.ToLower(combined)
	// Must contain brand-correct "gateway"/"unavailable" wording.
	if !strings.Contains(lc, "gateway") {
		t.Fatalf("missing 'gateway' in error: %q", combined)
	}
	if !strings.Contains(lc, "unavailable") {
		t.Fatalf("missing 'unavailable' in error: %q", combined)
	}
	if !strings.Contains(combined, "Inferenesia") {
		t.Fatalf("missing brand 'Inferenesia' in error: %q", combined)
	}
	// Never print the API key value.
	if strings.Contains(combined, apiKey) {
		t.Fatalf("API key leaked into output: %q", combined)
	}
	// No Go panic/stack trace.
	if strings.Contains(strings.ToLower(combined), "panic:") ||
		strings.Contains(combined, "runtime error") {
		t.Fatalf("panic in output: %q", combined)
	}
}

// TestCrossArea_GatewayDown_UnreachableBaseURL_Chat verifies `inferenesia
// chat` against a dead localhost port fails fast with a brand-correct gateway
// error and never prints the API key.
func TestCrossArea_GatewayDown_UnreachableBaseURL_Chat(t *testing.T) {
	const apiKey = "dead-port-chat-secret-XYZ"
	ws := t.TempDir()
	start := time.Now()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("YURA_AI_HOME", home)
	t.Setenv("TEMP_AI_BASE_URL", "http://127.0.0.1:1/v1")
	t.Setenv("TEMP_AI_API_KEY", apiKey)
	t.Setenv("TEMP_AI_MODEL", "")
	cwd, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{
		"chat", "-p", "hi", "--workspace", ws, "--no-tools", "--no-persist",
	}, &out, &errBuf)
	elapsed := time.Since(start)
	if code == 0 {
		t.Fatalf("expected non-zero exit; out=%q err=%q", out.String(), errBuf.String())
	}
	if elapsed > 10*time.Second {
		t.Fatalf("took %v, want <= 10s (VAL-CROSS-009)", elapsed)
	}
	combined := out.String() + errBuf.String()
	lc := strings.ToLower(combined)
	if !strings.Contains(lc, "gateway") || !strings.Contains(lc, "unavailable") {
		t.Fatalf("missing gateway/unavailable in error: %q", combined)
	}
	if strings.Contains(combined, apiKey) {
		t.Fatalf("API key leaked into output: %q", combined)
	}
	if strings.Contains(strings.ToLower(combined), "panic:") {
		t.Fatalf("panic in output: %q", combined)
	}
}

// TestCrossArea_GatewayDown_401_Models verifies the 401 path surfaces a
// brand-correct gateway error with HTTP 401 and no key leak.
func TestCrossArea_GatewayDown_401_Models(t *testing.T) {
	const apiKey = "401-cli-secret-key-XYZ"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never echo the key back.
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer srv.Close()

	code, out, errBuf := runCLIWithGatewayEnv(t, []string{"models"}, srv.URL+"/v1", apiKey)
	if code == 0 {
		t.Fatalf("expected non-zero exit; out=%q err=%q", out, errBuf)
	}
	combined := out + errBuf
	lc := strings.ToLower(combined)
	if !strings.Contains(lc, "gateway") || !strings.Contains(lc, "unavailable") {
		t.Fatalf("missing gateway/unavailable in error: %q", combined)
	}
	if !strings.Contains(lc, "401") {
		t.Fatalf("missing HTTP 401 in error: %q", combined)
	}
	if strings.Contains(combined, apiKey) {
		t.Fatalf("API key leaked into output: %q", combined)
	}
}

// TestCrossArea_GatewayDown_5xx_Models verifies the 5xx path surfaces a
// brand-correct gateway error and no key leak.
func TestCrossArea_GatewayDown_5xx_Models(t *testing.T) {
	const apiKey = "5xx-cli-secret-key-XYZ"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream gateway down"}}`)
	}))
	defer srv.Close()

	code, out, errBuf := runCLIWithGatewayEnv(t, []string{"models"}, srv.URL+"/v1", apiKey)
	if code == 0 {
		t.Fatalf("expected non-zero exit; out=%q err=%q", out, errBuf)
	}
	combined := out + errBuf
	lc := strings.ToLower(combined)
	if !strings.Contains(lc, "gateway") || !strings.Contains(lc, "unavailable") {
		t.Fatalf("missing gateway/unavailable in error: %q", combined)
	}
	if strings.Contains(combined, apiKey) {
		t.Fatalf("API key leaked into output: %q", combined)
	}
}
