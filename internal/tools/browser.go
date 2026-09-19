package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
)

// Browser tool names exposed to the model (VAL-BRW-006/007/009).
// Closed set — no browser_shell / browser_exec (sandbox VAL-BRW-009).
const (
	NameBrowserSetEngine  = "browser_set_engine"
	NameBrowserStatus     = "browser_status"
	NameBrowserNavigate   = "browser_navigate"
	NameBrowserScreenshot = "browser_screenshot"
	NameBrowserEvaluate   = "browser_evaluate"
	NameBrowserSnapshot   = "browser_snapshot"
	NameBrowserWaitHuman  = "browser_wait_human"
	NameBrowserClose      = "browser_close"
)

// BrowserBackend is implemented by core.Service's BrowserManager adapter.
// Engine launch args are server-controlled; tools never accept model ExtraArgs.
type BrowserBackend interface {
	// SetEngine selects and launches an engine. Missing binary → clear ErrUnavailable.
	// headless: nil = engine default (stealth/debug headed; e2e headless); true/false override.
	SetEngine(ctx context.Context, id string, headless *bool) error
	// Status returns a human-readable status snapshot (no cookies).
	Status() (engine, state string, cdpURL string, cdpPort int, fallback bool, detail string)
	// Navigate loads url (with human wait + URL sandbox).
	Navigate(ctx context.Context, url string) error
	// Screenshot writes PNG to path (or default artifact dir) and returns path + bytes.
	// LLM tool result must reference path only — never base64 or cookies.
	Screenshot(ctx context.Context, outPath string) (path string, nbytes int, err error)
	// Evaluate runs page JS only; result is already sanitized for LLM.
	Evaluate(ctx context.Context, expression string) (string, error)
	// Snapshot returns sanitized page title+text (no cookies/storage).
	Snapshot(ctx context.Context) (string, error)
	// WaitHuman applies a configurable human-like delay (VAL-BRW-006).
	WaitHuman(ctx context.Context, reason string, minMs, maxMs int) (waitedMs int, err error)
	// Close stops the active engine.
	Close(ctx context.Context) error
	// ArtifactDir returns a directory for screenshots (may be empty → /tmp).
	ArtifactDir() string
}

// SetBrowserBackend wires browser_* tools.
func (r *Registry) SetBrowserBackend(b BrowserBackend) {
	if r == nil {
		return
	}
	r.browser = b
}

// BrowserBackend returns the attached backend (may be nil).
func (r *Registry) BrowserBackend() BrowserBackend {
	if r == nil {
		return nil
	}
	return r.browser
}

func browserDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserSetEngine,
				Description: "Select and launch a browser engine: debug (headed Chromium+CDP window), e2e/playwright (tests), stealth/CloakBrowser (scrape/anti-bot), camoufox (fallback). Use headless=true for no OS window (stealth default is headed so you can see the browser). Launch flags are server-controlled. Missing binary → clear install hint.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "engine": { "type": "string", "description": "Engine id: debug | e2e | playwright | stealth | camoufox" },
    "headless": { "type": "boolean", "description": "true = no OS window (--headless=new). false = visible window. Omit for engine default: stealth/debug headed, e2e headless, camoufox headless." }
  },
  "required": ["engine"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserStatus,
				Description: "Report the active browser engine state and CDP port (if any). Does not dump cookies, profiles, or storage.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserNavigate,
				Description: "Navigate the active browser engine to a URL. Applies a human-like randomized wait. Rejects shell/exec smuggling in the URL. Launch args cannot be passed.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "url": { "type": "string", "description": "http(s) or data: URL to open" }
  },
  "required": ["url"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserScreenshot,
				Description: "Capture a PNG screenshot of the active page. Returns a file path + byte size for the model (never base64 image bytes, never cookies).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Optional output PNG path (workspace or artifact dir)" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserEvaluate,
				Description: "Run page JavaScript only via the browser engine (Runtime.evaluate). No host shell/exec bridge. Results are sanitized: cookies/auth/storage tokens are redacted before the model sees them.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "expression": { "type": "string", "description": "JavaScript expression to evaluate in the page" }
  },
  "required": ["expression"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserSnapshot,
				Description: "Extract page title + visible text for the model (sanitized). Never returns document.cookie or storage state.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserWaitHuman,
				Description: "Pause with a human-like randomized delay (captcha / MFA handoff). Configurable min_ms/max_ms. Does not dump page secrets.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "reason": { "type": "string", "description": "Why the agent is waiting (e.g. captcha)" },
    "min_ms": { "type": "integer", "description": "Optional minimum wait in ms (default ~80)" },
    "max_ms": { "type": "integer", "description": "Optional maximum wait in ms (default ~350)" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameBrowserClose,
				Description: "Stop the active browser engine and emit engine-stop status.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
	}
}

func (r *Registry) execBrowser(ctx context.Context, call Call) Result {
	name := strings.TrimSpace(call.Name)
	if browser.HasShellBridgeTool(name) {
		return Result{
			Name:    name,
			Content: "browser: shell/exec tools are not exposed under the browser namespace (sandbox)",
			IsError: true,
		}
	}
	if r == nil || r.browser == nil {
		return Result{
			Name:    name,
			Content: "browser: BrowserManager not configured",
			IsError: true,
		}
	}
	b := r.browser

	switch name {
	case NameBrowserSetEngine:
		var args struct {
			Engine   string `json:"engine"`
			Headless *bool  `json:"headless"`
			// LaunchArgs intentionally omitted from schema; reject if model smuggles them.
			LaunchArgs []string `json:"launch_args"`
			ExtraArgs  []string `json:"extra_args"`
			Args       []string `json:"args"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return Result{Name: name, Content: "browser_set_engine: invalid arguments JSON: " + err.Error(), IsError: true}
		}
		if err := browser.ValidateLaunchArgsFromModel(args.LaunchArgs); err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		if err := browser.ValidateLaunchArgsFromModel(args.ExtraArgs); err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		if err := browser.ValidateLaunchArgsFromModel(args.Args); err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		eng := strings.TrimSpace(args.Engine)
		if eng == "" {
			return Result{Name: name, Content: "browser_set_engine: engine is required (debug|e2e|stealth|camoufox)", IsError: true}
		}
		if err := b.SetEngine(ctx, eng, args.Headless); err != nil {
			// ErrUnavailable already includes install guidance (VAL-BRW-007).
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		engine, state, cdp, port, fb, detail := b.Status()
		mode := "headed"
		if args.Headless != nil {
			if *args.Headless {
				mode = "headless"
			} else {
				mode = "headed"
			}
		} else {
			// Defaults when omit: e2e/camoufox headless, stealth/debug headed.
			switch strings.ToLower(eng) {
			case "e2e", "playwright", "camoufox":
				mode = "headless"
			default:
				mode = "headed"
			}
		}
		msg := fmt.Sprintf("engine=%s mode=%s state=%s fallback=%v", engine, mode, state, fb)
		if cdp != "" {
			msg += " cdp_url=" + cdp
		}
		if port != 0 {
			msg += fmt.Sprintf(" cdp_port=%d", port)
			if !browser.InRange(port) {
				return Result{Name: name, Content: fmt.Sprintf("browser: cdp port %d outside 4100–4199", port), IsError: true}
			}
		}
		if detail != "" {
			msg += " " + detail
		}
		return Result{Name: name, Content: browser.SanitizeForLLM(msg)}

	case NameBrowserStatus:
		engine, state, cdp, port, fb, detail := b.Status()
		msg := fmt.Sprintf("engine=%s state=%s fallback=%v", engine, state, fb)
		if cdp != "" {
			msg += " cdp_url=" + cdp
		}
		if port != 0 {
			msg += fmt.Sprintf(" cdp_port=%d", port)
		}
		if detail != "" {
			msg += " detail=" + detail
		}
		return Result{Name: name, Content: browser.SanitizeForLLM(msg)}

	case NameBrowserNavigate:
		var args struct {
			URL        string   `json:"url"`
			LaunchArgs []string `json:"launch_args"`
			Exec       string   `json:"exec"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return Result{Name: name, Content: "browser_navigate: invalid arguments: " + err.Error(), IsError: true}
		}
		if strings.TrimSpace(args.Exec) != "" {
			return Result{Name: name, Content: "browser: exec is not allowed via navigate (page JS only via browser_evaluate)", IsError: true}
		}
		if err := browser.ValidateLaunchArgsFromModel(args.LaunchArgs); err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		if err := b.Navigate(ctx, args.URL); err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		return Result{Name: name, Content: browser.SanitizeForLLM("navigated " + strings.TrimSpace(args.URL))}

	case NameBrowserScreenshot:
		var args struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &args)
		path, n, err := b.Screenshot(ctx, args.Path)
		if err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		// Path reference only — never base64 / cookies (VAL-BRW-006).
		return Result{Name: name, Content: browser.ScreenshotResultForLLM(path, n, "")}

	case NameBrowserEvaluate:
		var args struct {
			Expression string `json:"expression"`
			// reject shell bridges
			Exec    string `json:"exec"`
			Command string `json:"command"`
			Shell   string `json:"shell"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return Result{Name: name, Content: "browser_evaluate: invalid arguments: " + err.Error(), IsError: true}
		}
		if args.Exec != "" || args.Command != "" || args.Shell != "" {
			return Result{Name: name, Content: "browser: evaluate is page JS only — no shell/exec bridge", IsError: true}
		}
		out, err := b.Evaluate(ctx, args.Expression)
		if err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		// Double-sanitize defense in depth.
		san := browser.SanitizeForLLM(out)
		if browser.ContainsSensitive(san) {
			san = browser.SanitizeForLLM(san)
		}
		return Result{Name: name, Content: san}

	case NameBrowserSnapshot:
		out, err := b.Snapshot(ctx)
		if err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		return Result{Name: name, Content: browser.PageTextResultForLLM(out)}

	case NameBrowserWaitHuman:
		var args struct {
			Reason string `json:"reason"`
			MinMs  int    `json:"min_ms"`
			MaxMs  int    `json:"max_ms"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &args)
		ms, err := b.WaitHuman(ctx, args.Reason, args.MinMs, args.MaxMs)
		if err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		return Result{Name: name, Content: fmt.Sprintf("waited_ms=%d reason=%s", ms, browser.SanitizeForLLM(args.Reason))}

	case NameBrowserClose:
		if err := b.Close(ctx); err != nil {
			return Result{Name: name, Content: err.Error(), IsError: true}
		}
		return Result{Name: name, Content: "browser closed"}

	default:
		return Result{Name: name, Content: fmt.Sprintf("unknown browser tool %q", name), IsError: true}
	}
}

// ManagerBrowserBackend adapts *browser.Manager to tools.BrowserBackend.
type ManagerBrowserBackend struct {
	M           *browser.Manager
	Artifacts   string
	// HeadlessE2E forces e2e headless when constructing (desktop tools).
	HeadlessE2E bool
}

// NewManagerBrowserBackend wraps a Manager.
func NewManagerBrowserBackend(m *browser.Manager, artifactDir string) *ManagerBrowserBackend {
	return &ManagerBrowserBackend{M: m, Artifacts: artifactDir, HeadlessE2E: true}
}

func (b *ManagerBrowserBackend) ArtifactDir() string {
	if b == nil {
		return ""
	}
	return b.Artifacts
}

func (b *ManagerBrowserBackend) SetEngine(ctx context.Context, id string, headless *bool) error {
	if b == nil || b.M == nil {
		return fmt.Errorf("browser: manager not configured")
	}
	eng, err := browser.ParseEngineID(id)
	if err != nil {
		return err
	}
	eng = browser.NormalizeEngine(eng)

	// Explicit headless overrides engine defaults for this launch only.
	if headless != nil {
		b.M.HeadlessOverride = headless
	} else if eng == browser.EngineE2E {
		// Desktop tools default e2e to headless when unspecified.
		hl := b.HeadlessE2E
		b.M.HeadlessOverride = &hl
	} else {
		b.M.HeadlessOverride = nil
	}
	return b.M.SetEngine(ctx, eng)
}

func (b *ManagerBrowserBackend) Status() (engine, state string, cdpURL string, cdpPort int, fallback bool, detail string) {
	if b == nil || b.M == nil {
		return "", string(browser.StateIdle), "", 0, false, "not configured"
	}
	st := b.M.Status()
	return string(st.Engine), string(st.State), st.CDPURL, st.CDPPort, st.Fallback, st.Detail
}

func (b *ManagerBrowserBackend) Navigate(ctx context.Context, url string) error {
	if b == nil || b.M == nil {
		return fmt.Errorf("browser: manager not configured")
	}
	return b.M.Navigate(ctx, url)
}

func (b *ManagerBrowserBackend) Screenshot(ctx context.Context, outPath string) (string, int, error) {
	if b == nil || b.M == nil {
		return "", 0, fmt.Errorf("browser: manager not configured")
	}
	png, err := b.M.Screenshot(ctx, browser.ScreenshotOpts{Format: "png"})
	if err != nil {
		return "", 0, err
	}
	if len(png) == 0 {
		return "", 0, fmt.Errorf("browser: screenshot empty")
	}
	path := strings.TrimSpace(outPath)
	if path == "" {
		dir := b.Artifacts
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "yura-browser-artifacts")
		}
		_ = os.MkdirAll(dir, 0o700)
		path = filepath.Join(dir, fmt.Sprintf("shot-%d.png", time.Now().UnixNano()))
	}
	if err := os.WriteFile(path, png, 0o600); err != nil {
		return "", 0, err
	}
	return path, len(png), nil
}

func (b *ManagerBrowserBackend) Evaluate(ctx context.Context, expression string) (string, error) {
	if b == nil || b.M == nil {
		return "", fmt.Errorf("browser: manager not configured")
	}
	res, err := b.M.Evaluate(ctx, expression)
	if err != nil {
		return "", err
	}
	return browser.SanitizeEvalResult(res), nil
}

func (b *ManagerBrowserBackend) Snapshot(ctx context.Context) (string, error) {
	if b == nil || b.M == nil {
		return "", fmt.Errorf("browser: manager not configured")
	}
	return b.M.SnapshotText(ctx)
}

func (b *ManagerBrowserBackend) WaitHuman(ctx context.Context, reason string, minMs, maxMs int) (int, error) {
	if b == nil || b.M == nil {
		return 0, fmt.Errorf("browser: manager not configured")
	}
	d, err := b.M.WaitHuman(ctx, reason, minMs, maxMs)
	return int(d.Milliseconds()), err
}

func (b *ManagerBrowserBackend) Close(ctx context.Context) error {
	if b == nil || b.M == nil {
		return nil
	}
	return b.M.Close(ctx)
}
