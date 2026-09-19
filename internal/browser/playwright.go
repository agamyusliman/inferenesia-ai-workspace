package browser

import (
	"context"
	"fmt"
	"os"
)

// PlaywrightEngine drives Chrome for Testing from the Playwright browser cache
// (mission readiness: Playwright 1.61). Works headless for tests (VAL-BRW-002).
// This adapter uses the cached chromium binary via CDP — it does not shell out
// to the Playwright Node package at runtime (sidecar binary path only).
type PlaywrightEngine struct {
	BinaryOverride string
	// Headless defaults to true for CI/tests; set false for headed e2e.
	Headless bool
	Ports    *PortAllocator

	launch chromeLauncher
	binary string
}

func NewPlaywrightEngine(ports *PortAllocator) *PlaywrightEngine {
	if ports == nil {
		ports = NewPortAllocator()
	}
	return &PlaywrightEngine{
		Ports:    ports,
		Headless: true,
	}
}

func (e *PlaywrightEngine) ID() EngineID { return EngineE2E }

func (e *PlaywrightEngine) Available() (bool, string) {
	if e.BinaryOverride != "" {
		if st, err := os.Stat(e.BinaryOverride); err == nil && !st.IsDir() {
			return true, e.BinaryOverride
		}
		return false, "override binary missing: " + e.BinaryOverride
	}
	p, err := resolvePlaywrightChrome()
	if err != nil {
		return false, err.Error()
	}
	return true, p
}

func (e *PlaywrightEngine) Launch(ctx context.Context) error {
	bin := e.BinaryOverride
	if bin == "" {
		var err error
		bin, err = resolvePlaywrightChrome()
		if err != nil {
			return &ErrUnavailable{
				Engine:  EngineE2E,
				Reason:  err.Error(),
				Install: "run: npx playwright@1.61.1 install chromium  (browsers cache under ~/Library/Caches/ms-playwright or $XDG_CACHE_HOME/ms-playwright)",
			}
		}
	}
	e.binary = bin
	e.launch = chromeLauncher{
		Binary:    bin,
		Headless:  e.Headless,
		ports:     e.Ports,
		ExtraArgs: []string{"--disable-dev-shm-usage"},
	}
	if err := e.launch.Launch(ctx); err != nil {
		return fmt.Errorf("playwright engine launch: %w", err)
	}
	return nil
}

func (e *PlaywrightEngine) Navigate(ctx context.Context, url string) error {
	return e.launch.Navigate(ctx, url)
}

func (e *PlaywrightEngine) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	return e.launch.Screenshot(ctx, opts)
}

func (e *PlaywrightEngine) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	return e.launch.Evaluate(ctx, expression)
}

func (e *PlaywrightEngine) Close(ctx context.Context) error {
	return e.launch.Close(ctx)
}

func (e *PlaywrightEngine) CDPURL() string { return e.launch.CDPURL() }
func (e *PlaywrightEngine) CDPPort() int   { return e.launch.CDPPort() }

// BinaryPath returns the resolved chromium path after Launch (empty before).
func (e *PlaywrightEngine) BinaryPath() string { return e.binary }
