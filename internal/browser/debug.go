package browser

import (
	"context"
	"fmt"
	"os"
)

// DebugEngine launches an external OS Chromium/Chrome window with CDP on
// 4100–4199. Not an in-app embedded webview (VAL-BRW-001).
type DebugEngine struct {
	BinaryOverride string
	// Headless forces headless (tests only). Production debug is always headed.
	Headless bool
	Ports    *PortAllocator

	launch chromeLauncher
	binary string
}

func NewDebugEngine(ports *PortAllocator) *DebugEngine {
	if ports == nil {
		ports = NewPortAllocator()
	}
	return &DebugEngine{Ports: ports}
}

func (e *DebugEngine) ID() EngineID { return EngineDebug }

func (e *DebugEngine) Available() (bool, string) {
	if e.BinaryOverride != "" {
		if st, err := os.Stat(e.BinaryOverride); err == nil && !st.IsDir() {
			return true, e.BinaryOverride
		}
		return false, "override binary missing: " + e.BinaryOverride
	}
	p, err := resolveHeadedChrome()
	if err != nil {
		return false, err.Error()
	}
	return true, p
}

func (e *DebugEngine) Launch(ctx context.Context) error {
	bin := e.BinaryOverride
	if bin == "" {
		var err error
		bin, err = resolveHeadedChrome()
		if err != nil {
			return &ErrUnavailable{
				Engine:  EngineDebug,
				Reason:  err.Error(),
				Install: "install Google Chrome or: npx playwright install chromium",
			}
		}
	}
	e.binary = bin
	// Debug engine: external OS window by default (headed). Tests may set Headless.
	e.launch = chromeLauncher{
		Binary:   bin,
		Headless: e.Headless,
		ports:    e.Ports,
		// Keep a visible window; avoid automation flag that some sites block.
		ExtraArgs: []string{},
	}
	return e.launch.Launch(ctx)
}

func (e *DebugEngine) Navigate(ctx context.Context, url string) error {
	return e.launch.Navigate(ctx, url)
}

func (e *DebugEngine) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	return e.launch.Screenshot(ctx, opts)
}

func (e *DebugEngine) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	return e.launch.Evaluate(ctx, expression)
}

func (e *DebugEngine) Close(ctx context.Context) error {
	return e.launch.Close(ctx)
}

func (e *DebugEngine) CDPURL() string { return e.launch.CDPURL() }
func (e *DebugEngine) CDPPort() int   { return e.launch.CDPPort() }

// LaunchArgsForTest returns representative chrome args used by debug launch
// (for unit tests asserting --remote-debugging-port in 4100–4199).
func DebugLaunchArgs(port int) []string {
	if !InRange(port) {
		panic(fmt.Sprintf("debug port out of range: %d", port))
	}
	return []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
	}
}
