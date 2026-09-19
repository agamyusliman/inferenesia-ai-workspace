package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CloakBrowserEngine is the stealth adapter (VAL-BRW-003).
//
// CloakBrowser is a Playwright/Puppeteer drop-in that ships a patched Chromium
// binary with source-level stealth patches (navigator.webdriver=false, real
// plugin list, window.chrome present, no HeadlessChrome UA leak). The patches
// are compiled into the binary, so launching the binary directly with
// --remote-debugging-port retains stealth properties without going through the
// Python wrapper.
//
// Binary discovery order:
//  1. ~/.cloakbrowser/chromium-<ver>/Chromium.app/Contents/MacOS/Chromium (macOS, auto-downloaded by `pip install cloakbrowser`)
//  2. ~/.cloakbrowser/chromium-<ver>/chrome-linux/chrome (Linux)
//  3. ~/.cloakbrowser/chromium-<ver>/chrome-win64/chrome.exe (Windows)
//  4. YURA_AI_HOME/browser/engines/cloakbrowser/... (project-local override)
//  5. PATH executables: cloakbrowser, cloak-browser
//
// When the binary/package is missing, Available returns false and Launch returns
// ErrUnavailable with install guidance (no panic).
//
// Installation (project-local venv, preferred):
//
//	python3 -m venv ~/.inferenesia/browser/engines/cloakbrowser/venv
//	~/.inferenesia/browser/engines/cloakbrowser/venv/bin/pip install 'cloakbrowser[geoip]'
//	# first launch auto-downloads the stealth Chromium binary to ~/.cloakbrowser/
type CloakBrowserEngine struct {
	// BinaryOverride forces a specific path (tests inject missing path).
	BinaryOverride string
	// Headless: CloakBrowser often prefers headed for auth; tests may headless.
	Headless bool
	// PreferChromeFallback: when CloakBrowser is missing, still report unavailable
	// unless this is set for emergency local-only testing (not used by Manager).
	Ports *PortAllocator

	// When CloakBrowser binary is present it is expected to support CDP on a port.
	// The stealth patches are baked into the Chromium binary at the C++ source
	// level, so launching it directly with CDP flags retains stealth properties.
	launch chromeLauncher
	binary string
}

func NewCloakBrowserEngine(ports *PortAllocator) *CloakBrowserEngine {
	if ports == nil {
		ports = NewPortAllocator()
	}
	return &CloakBrowserEngine{Ports: ports, Headless: false}
}

func (e *CloakBrowserEngine) ID() EngineID { return EngineStealth }

func (e *CloakBrowserEngine) Available() (bool, string) {
	p, err := e.resolveBinary()
	if err != nil {
		return false, err.Error()
	}
	return true, p
}

func (e *CloakBrowserEngine) resolveBinary() (string, error) {
	if e.BinaryOverride != "" {
		if st, err := os.Stat(e.BinaryOverride); err == nil && !st.IsDir() {
			return e.BinaryOverride, nil
		}
		return "", fmt.Errorf("CloakBrowser binary not found at %s", e.BinaryOverride)
	}
	candidates := cloakbrowserCandidates()
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	// Also try PATH executable name.
	if p, err := exec.LookPath("cloakbrowser"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("cloak-browser"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("CloakBrowser not installed")
}

func cloakbrowserCandidates() []string {
	var out []string
	// 1. CloakBrowser pip cache: ~/.cloakbrowser/chromium-<ver>/...
	// The Python wrapper `pip install cloakbrowser` auto-downloads the patched
	// stealth Chromium binary here on first launch. Stealth patches are baked
	// into this binary, so direct launch retains them.
	if home, err := os.UserHomeDir(); err == nil {
		cbCache := filepath.Join(home, ".cloakbrowser")
		if entries, err := os.ReadDir(cbCache); err == nil {
			// Iterate chromium-<ver> dirs (newest-first by name desc).
			var dirs []string
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), "chromium-") {
					dirs = append(dirs, e.Name())
				}
			}
			sortStringsDesc(dirs)
			for _, d := range dirs {
				base := filepath.Join(cbCache, d)
				if runtime.GOOS == "darwin" {
					out = append(out, filepath.Join(base, "Chromium.app", "Contents", "MacOS", "Chromium"))
				} else if runtime.GOOS == "linux" {
					out = append(out, filepath.Join(base, "chrome-linux", "chrome"))
				} else if runtime.GOOS == "windows" {
					out = append(out, filepath.Join(base, "chrome-win64", "chrome.exe"))
				}
			}
		}
	}
	// 2. Project-local engine dir override (YURA_AI_HOME or default ~/.yura-ai).
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, ".yura-ai", "browser", "engines", "cloakbrowser", "cloakbrowser"),
			filepath.Join(home, ".yura-ai", "browser", "engines", "cloakbrowser", "bin", "cloakbrowser"),
		)
		if runtime.GOOS == "darwin" {
			out = append(out,
				filepath.Join(home, ".yura-ai", "browser", "engines", "cloakbrowser", "Chromium.app", "Contents", "MacOS", "Chromium"),
				filepath.Join(home, ".yura-ai", "browser", "engines", "cloakbrowser", "CloakBrowser.app", "Contents", "MacOS", "CloakBrowser"),
			)
		}
	}
	if env := os.Getenv("YURA_AI_HOME"); env != "" {
		out = append(out,
			filepath.Join(env, "browser", "engines", "cloakbrowser", "cloakbrowser"),
			filepath.Join(env, "browser", "engines", "cloakbrowser", "bin", "cloakbrowser"),
		)
		if runtime.GOOS == "darwin" {
			out = append(out, filepath.Join(env, "browser", "engines", "cloakbrowser", "Chromium.app", "Contents", "MacOS", "Chromium"))
		}
	}
	// 3. Well-known PATH locations (pip --user installs, homebrew).
	out = append(out,
		"/usr/local/bin/cloakbrowser",
		"/opt/homebrew/bin/cloakbrowser",
	)
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".local", "bin", "cloakbrowser"))
	}
	return out
}

// sortStringsDesc sorts in place, descending (newest version first).
func sortStringsDesc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] > s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func (e *CloakBrowserEngine) Launch(ctx context.Context) error {
	bin, err := e.resolveBinary()
	if err != nil {
		return &ErrUnavailable{
			Engine:  EngineStealth,
			Reason:  err.Error(),
			Install: `install CloakBrowser (stealth Chromium): python3 -m venv ~/.inferenesia/browser/engines/cloakbrowser/venv && ~/.inferenesia/browser/engines/cloakbrowser/venv/bin/pip install 'cloakbrowser[geoip]' (first launch auto-downloads the patched Chromium binary to ~/.cloakbrowser/; then: inferenesia browser install stealth)`,
		}
	}
	e.binary = bin
	// Stealth patches are baked into the CloakBrowser Chromium binary at the
	// C++ source level (navigator.webdriver=false, real plugins, window.chrome
	// present, no HeadlessChrome UA leak). We launch it directly with CDP flags;
	// the additional --disable-blink-features=AutomationControlled flag is a
	// belt-and-suspenders guard that is harmless on the patched binary.
	stealthArgs := []string{
		"--disable-blink-features=AutomationControlled",
		"--exclude-switches=enable-automation",
	}
	// Some cloak builds are wrappers; if not chromium-compatible CDP launch fails
	// with a clear error and Manager may fall back to Camoufox.
	e.launch = chromeLauncher{
		Binary:    bin,
		Headless:  e.Headless,
		ports:     e.Ports,
		ExtraArgs: stealthArgs,
	}
	if err := e.launch.Launch(ctx); err != nil {
		// If the binary is not chrome-compatible, surface unavailable/actionable text.
		if strings.Contains(err.Error(), "chrome start") || strings.Contains(err.Error(), "executable") {
			return &ErrUnavailable{
				Engine:  EngineStealth,
				Reason:  err.Error(),
				Install: `CloakBrowser binary present but failed to launch with CDP. Reinstall: pip install -U 'cloakbrowser[geoip]' or use Camoufox fallback.`,
			}
		}
		return err
	}
	return nil
}

func (e *CloakBrowserEngine) Navigate(ctx context.Context, url string) error {
	return e.launch.Navigate(ctx, url)
}

func (e *CloakBrowserEngine) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	return e.launch.Screenshot(ctx, opts)
}

func (e *CloakBrowserEngine) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	return e.launch.Evaluate(ctx, expression)
}

func (e *CloakBrowserEngine) Close(ctx context.Context) error {
	return e.launch.Close(ctx)
}

func (e *CloakBrowserEngine) CDPURL() string { return e.launch.CDPURL() }
func (e *CloakBrowserEngine) CDPPort() int   { return e.launch.CDPPort() }

// CloakInstallHint is the canonical message fragment for missing CloakBrowser.
func CloakInstallHint() string {
	return strings.TrimSpace(`install CloakBrowser (stealth Chromium):
  python3 -m venv ~/.inferenesia/browser/engines/cloakbrowser/venv
  ~/.inferenesia/browser/engines/cloakbrowser/venv/bin/pip install 'cloakbrowser[geoip]'
  # first launch auto-downloads the stealth Chromium binary to ~/.cloakbrowser/`)
}
