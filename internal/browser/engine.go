package browser

import (
	"context"
	"fmt"
)

// EngineID identifies a browser adapter (docs/browser.md routing table).
type EngineID string

const (
	EngineDebug       EngineID = "debug"
	EngineE2E         EngineID = "e2e"
	EnginePlaywright  EngineID = "playwright" // alias of e2e
	EngineStealth     EngineID = "stealth"
	EngineCloak       EngineID = "cloakbrowser"
	EngineCamoufox    EngineID = "camoufox"
	EngineStealthFB   EngineID = "stealth_fb"
	EngineLight       EngineID = "light" // Obscura optional
)

// NormalizeEngine maps aliases to the canonical engine id used in status events.
func NormalizeEngine(id EngineID) EngineID {
	switch EngineID(string(id)) {
	case EnginePlaywright, "pw":
		return EngineE2E
	case EngineCloak:
		return EngineStealth
	case EngineStealthFB:
		return EngineCamoufox
	default:
		return id
	}
}

// EngineName returns a human-readable name for errors/install hints.
func EngineName(id EngineID) string {
	switch NormalizeEngine(id) {
	case EngineDebug:
		return "debug (Chromium + CDP external window)"
	case EngineE2E:
		return "e2e (Playwright / Chrome for Testing)"
	case EngineStealth:
		return "stealth (CloakBrowser)"
	case EngineCamoufox:
		return "camoufox (Camoufox fallback)"
	case EngineLight:
		return "light (Obscura)"
	default:
		return string(id)
	}
}

// StatusState is the engine/session lifecycle state.
type StatusState string

const (
	StateIdle        StatusState = "idle"
	StateSelecting   StatusState = "selecting"
	StateLaunching   StatusState = "launching"
	StateReady       StatusState = "ready"
	StateNavigating  StatusState = "navigating"
	StateNavigateEnd StatusState = "navigate_end"
	StateWaitingHuman StatusState = "waiting_human"
	StateUnavailable StatusState = "unavailable"
	StateError       StatusState = "error"
	StateStopped     StatusState = "stopped"
)

// Status is emitted as BrowserStatus for CLI/desktop subscribers (VAL-BRW-001/004/008).
type Status struct {
	Engine   EngineID    `json:"engine"`
	State    StatusState `json:"state"`
	CDPURL   string      `json:"cdp_url,omitempty"`
	CDPPort  int         `json:"cdp_port,omitempty"`
	Fallback bool        `json:"fallback,omitempty"`
	Reason   string      `json:"reason,omitempty"`
	URL      string      `json:"url,omitempty"`
	Detail   string      `json:"detail,omitempty"`
	LogLine  string      `json:"log_line,omitempty"`
}

// ScreenshotOpts controls Page.captureScreenshot.
type ScreenshotOpts struct {
	FullPage bool
	Format   string // png|jpeg; default png
	Quality  int    // jpeg only
}

// EvalResult is the JSON-encoded Runtime.evaluate result.
type EvalResult struct {
	Value any
	Raw   string
}

// Engine is the common browser adapter contract (VAL-BRW-005).
// Implementations are sidecars: never embedded Chromium in the Go binary.
type Engine interface {
	// ID is the canonical engine id reported in BrowserStatus.
	ID() EngineID
	// Available reports whether the sidecar binary/package is present.
	// When false, ErrUnavailable is returned from Launch with an install hint.
	Available() (bool, string)
	// Launch starts the engine process (headed/headless per implementation).
	Launch(ctx context.Context) error
	// Navigate loads url in the active page.
	Navigate(ctx context.Context, url string) error
	// Screenshot returns image bytes (non-empty on success).
	Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error)
	// Evaluate runs page JS and returns a JSON-serializable result.
	Evaluate(ctx context.Context, expression string) (EvalResult, error)
	// Close stops the engine and releases ports.
	Close(ctx context.Context) error
	// CDPURL returns the debugger URL when CDP is exposed (debug/e2e).
	CDPURL() string
	// CDPPort returns the remote debugging port (0 if none).
	CDPPort() int
}

// ErrUnavailable is returned when an engine binary/package is missing (VAL-BRW-003/007).
type ErrUnavailable struct {
	Engine  EngineID
	Reason  string
	Install string
}

func (e *ErrUnavailable) Error() string {
	if e == nil {
		return "browser: engine unavailable"
	}
	msg := fmt.Sprintf("browser: engine %q unavailable: %s", e.Engine, e.Reason)
	if e.Install != "" {
		msg += " — " + e.Install
	}
	return msg
}

// IsUnavailable reports whether err is or wraps ErrUnavailable.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*ErrUnavailable)
	return ok
}
