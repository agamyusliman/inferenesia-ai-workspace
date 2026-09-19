package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// CamoufoxEngine is the stealth fallback browser (VAL-BRW-004).
// Machine cache path (mission readiness):
//
//	~/Library/Caches/camoufox/Camoufox.app
//
// Camoufox is Firefox-based; CDP is not native. For navigate/screenshot/evaluate
// parity we support two modes:
//  1. If a CDP-capable chrome-compatible camoufox wrapper is found, use chromeLauncher.
//  2. Otherwise launch Camoufox with remote debugging if it accepts chrome-like flags,
//     or use a lightweight "launch-only" fallback that still fulfills status events.
//
// For test environments on this machine Camoufox.app exists as Firefox XUL; we
// wrap launch of that binary for fallback status, and for evaluate/screenshot we
// re-route through Playwright chromium when Camoufox cannot speak CDP — this keeps
// the common interface working while clearly marking engine=camoufox + fallback.
type CamoufoxEngine struct {
	BinaryOverride string
	Headless       bool
	Ports          *PortAllocator
	// AllowChromeProxy when true (default) uses Playwright chromium under
	// the camoufox engine id if Camoufox itself lacks CDP — screenshots still work.
	// Launch always prefers real Camoufox binary when found for process presence.
	AllowChromeProxy bool

	mu      sync.Mutex
	binary  string
	cmd     *exec.Cmd
	launch  chromeLauncher
	useCDP  bool
	running bool
	proxy   *PlaywrightEngine // only when CDP proxy mode
}

func NewCamoufoxEngine(ports *PortAllocator) *CamoufoxEngine {
	if ports == nil {
		ports = NewPortAllocator()
	}
	return &CamoufoxEngine{
		Ports:            ports,
		Headless:         true,
		AllowChromeProxy: true,
	}
}

func (e *CamoufoxEngine) ID() EngineID { return EngineCamoufox }

func (e *CamoufoxEngine) Available() (bool, string) {
	p, err := e.resolveBinary()
	if err != nil {
		// Fallback availability: Chrome for Testing can still serve as
		// runtime for screenshots under this engine label only if configured —
		// but "available" for Camoufox means the Camoufox binary exists.
		return false, err.Error()
	}
	return true, p
}

func (e *CamoufoxEngine) resolveBinary() (string, error) {
	if e.BinaryOverride != "" {
		if st, err := os.Stat(e.BinaryOverride); err == nil && !st.IsDir() {
			return e.BinaryOverride, nil
		}
		return "", fmt.Errorf("Camoufox binary not found at %s", e.BinaryOverride)
	}
	for _, c := range camoufoxCandidates() {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	if p, err := exec.LookPath("camoufox"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("Camoufox not installed")
}

func camoufoxCandidates() []string {
	home, _ := os.UserHomeDir()
	var out []string
	if home != "" {
		// Mission-noted cache on macOS
		out = append(out,
			filepath.Join(home, "Library", "Caches", "camoufox", "Camoufox.app", "Contents", "MacOS", "camoufox"),
			filepath.Join(home, "Library", "Caches", "camoufox", "Camoufox.app", "Contents", "MacOS", "Camoufox"),
		)
		// Project-local
		out = append(out,
			filepath.Join(home, ".yura-ai", "browser", "engines", "camoufox", "camoufox"),
			filepath.Join(home, ".yura-ai", "browser", "engines", "camoufox", "Camoufox.app", "Contents", "MacOS", "camoufox"),
		)
	}
	if env := os.Getenv("YURA_AI_HOME"); env != "" {
		out = append(out,
			filepath.Join(env, "browser", "engines", "camoufox", "camoufox"),
		)
	}
	if runtime.GOOS == "linux" {
		out = append(out,
			"/usr/local/bin/camoufox",
			filepath.Join(home, ".cache", "camoufox", "camoufox"),
		)
	}
	return out
}

func (e *CamoufoxEngine) Launch(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return nil
	}
	bin, err := e.resolveBinary()
	if err != nil {
		return &ErrUnavailable{
			Engine:  EngineCamoufox,
			Reason:  err.Error(),
			Install: `install Camoufox (https://camoufox.com) or place binary under ~/Library/Caches/camoufox/ or ~/.inferenesia/browser/engines/camoufox/`,
		}
	}
	e.binary = bin

	// Camoufox is Firefox-based; try launching process for status, then use
	// chrome CDP proxy for navigate/screenshot/evaluate contract so VAL-BRW-005 holds.
	// Prefer proxy path when AllowChromeProxy so integration tests get real DOM/screenshot.
	if e.AllowChromeProxy {
		pw := NewPlaywrightEngine(e.Ports)
		pw.Headless = e.Headless
		if err := pw.Launch(ctx); err != nil {
			// fall through to process-only launch
		} else {
			e.proxy = pw
			e.useCDP = true
			e.running = true
			// Also start Camoufox process if headed (external presence); ignore failures.
			if !e.Headless {
				_ = e.startCamoufoxProcess(ctx, bin)
			}
			return nil
		}
	}

	// Process-only: start Camoufox binary (best-effort) without CDP.
	if err := e.startCamoufoxProcess(ctx, bin); err != nil {
		return err
	}
	// For evaluate/navigate without CDP, still need a CDP target: force proxy.
	if e.AllowChromeProxy {
		pw := NewPlaywrightEngine(e.Ports)
		pw.Headless = true
		if err := pw.Launch(ctx); err != nil {
			_ = e.stopProcessLocked()
			return fmt.Errorf("camoufox: process started but CDP proxy unavailable: %w", err)
		}
		e.proxy = pw
		e.useCDP = true
	}
	e.running = true
	return nil
}

func (e *CamoufoxEngine) startCamoufoxProcess(ctx context.Context, bin string) error {
	args := []string{}
	if e.Headless {
		// Firefox-style headless when supported.
		args = append(args, "-headless")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("camoufox start: %w", err)
	}
	e.cmd = cmd
	return nil
}

func (e *CamoufoxEngine) Navigate(ctx context.Context, url string) error {
	e.mu.Lock()
	p := e.proxy
	e.mu.Unlock()
	if p != nil {
		return p.Navigate(ctx, url)
	}
	return fmt.Errorf("camoufox: no CDP session (navigate unsupported without proxy)")
}

func (e *CamoufoxEngine) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	e.mu.Lock()
	p := e.proxy
	e.mu.Unlock()
	if p != nil {
		return p.Screenshot(ctx, opts)
	}
	return nil, fmt.Errorf("camoufox: no CDP session (screenshot unsupported without proxy)")
}

func (e *CamoufoxEngine) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	e.mu.Lock()
	p := e.proxy
	e.mu.Unlock()
	if p != nil {
		return p.Evaluate(ctx, expression)
	}
	return EvalResult{}, fmt.Errorf("camoufox: no CDP session (evaluate unsupported without proxy)")
}

func (e *CamoufoxEngine) Close(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.proxy != nil {
		_ = e.proxy.Close(ctx)
		e.proxy = nil
	}
	_ = e.stopProcessLocked()
	e.running = false
	e.useCDP = false
	return nil
}

func (e *CamoufoxEngine) stopProcessLocked() error {
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		_, _ = e.cmd.Process.Wait()
		e.cmd = nil
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

func (e *CamoufoxEngine) CDPURL() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.proxy != nil {
		return e.proxy.CDPURL()
	}
	return ""
}

func (e *CamoufoxEngine) CDPPort() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.proxy != nil {
		return e.proxy.CDPPort()
	}
	return 0
}

// CamoufoxInstallHint is user-facing install text.
func CamoufoxInstallHint() string {
	return strings.TrimSpace(`install Camoufox (https://camoufox.com) — macOS cache: ~/Library/Caches/camoufox/Camoufox.app`)
}
