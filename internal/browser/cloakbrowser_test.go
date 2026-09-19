package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCloakBrowserUnavailableWhenMissing(t *testing.T) {
	e := NewCloakBrowserEngine(NewPortAllocator())
	// Force a missing path
	e.BinaryOverride = filepath.Join(t.TempDir(), "no-such-cloakbrowser-binary")
	ok, reason := e.Available()
	if ok {
		t.Fatal("expected unavailable")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := e.Launch(ctx)
	if err == nil {
		_ = e.Close(context.Background())
		t.Fatal("expected launch error when binary missing")
	}
	if !IsUnavailable(err) {
		t.Fatalf("expected ErrUnavailable, got %T %v", err, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "stealth") && !strings.Contains(msg, "cloak") && !strings.Contains(strings.ToLower(msg), "unavailable") {
		// Engine id is stealth after normalize
		if !strings.Contains(msg, string(EngineStealth)) {
			t.Fatalf("error should name engine: %s", msg)
		}
	}
	if !strings.Contains(strings.ToLower(msg), "install") && !strings.Contains(msg, "pip") {
		t.Fatalf("error should include install guidance: %s", msg)
	}
}

func TestFormatUnavailableInstallHint(t *testing.T) {
	msg := FormatUnavailable(EngineStealth, "CloakBrowser not installed")
	if !strings.Contains(msg, "stealth") {
		t.Fatalf("missing engine id: %s", msg)
	}
	if !strings.Contains(msg, "pip") && !strings.Contains(msg, "cloakbrowser") {
		t.Fatalf("missing install hint: %s", msg)
	}
}

// TestCloakInstallHintMentionsVenvAndCache ensures the canonical install hint
// tells the user the venv + pip path and the ~/.cloakbrowser binary cache.
func TestCloakInstallHintMentionsVenvAndCache(t *testing.T) {
	hint := CloakInstallHint()
	if !strings.Contains(hint, "venv") {
		t.Fatalf("install hint should mention venv: %s", hint)
	}
	if !strings.Contains(hint, "cloakbrowser[geoip]") {
		t.Fatalf("install hint should mention cloakbrowser[geoip] package: %s", hint)
	}
	if !strings.Contains(hint, ".cloakbrowser") {
		t.Fatalf("install hint should mention ~/.cloakbrowser binary cache: %s", hint)
	}
}

// TestCloakbrowserCandidatesIncludesPipCache verifies the candidate discovery
// includes the ~/.cloakbrowser/chromium-<ver>/Chromium.app path when the
// directory exists. Uses a temp HOME so it does not depend on the host's
// actual install state.
func TestCloakbrowserCandidatesIncludesPipCache(t *testing.T) {
	tmpHome := t.TempDir()
	cbCache := filepath.Join(tmpHome, ".cloakbrowser", "chromium-146.0.7680.177.5")
	appPath := filepath.Join(cbCache, "Chromium.app", "Contents", "MacOS", "Chromium")
	if err := os.MkdirAll(filepath.Dir(appPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(appPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldHome, hadHome := os.LookupEnv("HOME")
	t.Setenv("HOME", tmpHome)
	defer func() {
		if hadHome {
			os.Setenv("HOME", oldHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	cands := cloakbrowserCandidates()
	found := false
	for _, c := range cands {
		if strings.Contains(c, ".cloakbrowser") && strings.Contains(c, "chromium-146") && strings.HasSuffix(c, "Chromium") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("candidates did not include the pip-cache Chromium path: %v", cands)
	}

	// Engine.Available() should find the binary.
	e := NewCloakBrowserEngine(NewPortAllocator())
	ok, detail := e.Available()
	if !ok {
		t.Fatalf("expected Available=true with pip-cache binary present; got reason=%s", detail)
	}
	if !strings.Contains(detail, ".cloakbrowser") {
		t.Fatalf("Available detail should point to .cloakbrowser cache: %s", detail)
	}
}

// TestCloakbrowserCandidatesNewestVersionFirst verifies that when multiple
// chromium-<ver> dirs exist, the newest version is returned first (so the
// engine picks the latest stealth patches).
func TestCloakbrowserCandidatesNewestVersionFirst(t *testing.T) {
	tmpHome := t.TempDir()
	versions := []string{
		"chromium-145.0.7632.109.2",
		"chromium-146.0.7680.177.5",
		"chromium-144.0.7600.100.1",
	}
	for _, v := range versions {
		appPath := filepath.Join(tmpHome, ".cloakbrowser", v, "Chromium.app", "Contents", "MacOS", "Chromium")
		if err := os.MkdirAll(filepath.Dir(appPath), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", v, err)
		}
		if err := os.WriteFile(appPath, []byte("x"), 0o755); err != nil {
			t.Fatalf("write %s: %v", v, err)
		}
	}

	oldHome, hadHome := os.LookupEnv("HOME")
	t.Setenv("HOME", tmpHome)
	defer func() {
		if hadHome {
			os.Setenv("HOME", oldHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	cands := cloakbrowserCandidates()
	// Find the first candidate that lives under .cloakbrowser/chromium-
	var first string
	for _, c := range cands {
		if i := strings.Index(c, ".cloakbrowser"+string(filepath.Separator)+"chromium-"); i >= 0 {
			first = c
			break
		}
	}
	if first == "" {
		t.Fatalf("no .cloakbrowser/chromium- candidate found: %v", cands)
	}
	if !strings.Contains(first, "chromium-146") {
		t.Fatalf("expected newest (146) first, got: %s", first)
	}
}

// TestCloakBrowserLaunchStealthIntegration is an integration test that launches
// the real CloakBrowser stealth Chromium binary (when present) and verifies:
//   - CDP starts on a port in 4100–4199
//   - navigator.webdriver is false (stealth patch baked into binary)
//   - navigator.plugins.length > 0 (real plugin list, not 0 like Playwright)
//   - typeof window.chrome === 'object' (present like real Chrome)
//   - screenshot bytes > 0
//
// Skipped when the CloakBrowser binary is not installed (VAL-BRW-003: when
// absent, TestCloakBrowserUnavailableWhenMissing covers the unavailable path).
func TestCloakBrowserLaunchStealthIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	e := NewCloakBrowserEngine(NewPortAllocator())
	e.Headless = true
	ok, detail := e.Available()
	if !ok {
		t.Skipf("CloakBrowser binary not installed: %s", detail)
	}
	t.Logf("using binary: %s", detail)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := e.Launch(ctx); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() { _ = e.Close(context.Background()) }()

	if !InRange(e.CDPPort()) {
		t.Fatalf("CDP port %d not in 4100–4199", e.CDPPort())
	}
	if e.CDPURL() == "" {
		t.Fatal("CDPURL empty")
	}
	if err := e.Navigate(ctx, "data:text/html,<!doctype html><html><head><title>CloakStealthFixture</title></head><body><h1 id=t>stealth</h1></body></html>"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	res, err := e.Evaluate(ctx, "JSON.stringify({wd: navigator.webdriver, plugins: navigator.plugins.length, chrome: typeof window.chrome, ua: navigator.userAgent})")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	raw := strings.ToLower(res.Raw)
	t.Logf("stealth result: %s", res.Raw)
	if strings.Contains(raw, `"wd":true`) {
		t.Fatalf("navigator.webdriver should be false with CloakBrowser stealth binary; got %s", res.Raw)
	}
	if strings.Contains(raw, `"plugins":0`) {
		t.Fatalf("navigator.plugins.length should be > 0 with CloakBrowser stealth binary; got %s", res.Raw)
	}
	if strings.Contains(raw, `"chrome":"undefined"`) {
		t.Fatalf("window.chrome should be present (object) with CloakBrowser stealth binary; got %s", res.Raw)
	}
	if strings.Contains(raw, "headlesschrome") {
		t.Fatalf("UA should not leak HeadlessChrome; got %s", res.Raw)
	}
	png, err := e.Screenshot(ctx, ScreenshotOpts{Format: "png"})
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if len(png) < 100 {
		t.Fatalf("screenshot too small: %d bytes", len(png))
	}
	t.Logf("OK stealth binary launched: port=%d screenshot_bytes=%d", e.CDPPort(), len(png))
}
