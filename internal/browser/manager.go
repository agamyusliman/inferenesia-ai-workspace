package browser

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Manager routes set-engine / navigate / screenshot / evaluate across adapters
// and applies Camoufox fallback when the primary engine fails to launch (VAL-BRW-004).
// Human-like wait (VAL-BRW-006), status lifecycle (VAL-BRW-008), and sandbox
// constraints (VAL-BRW-009) are enforced here so tools never bypass policy.
type Manager struct {
	mu sync.Mutex

	ports  *PortAllocator
	engine Engine
	id     EngineID
	// FallbackEnabled: when primary launch fails, try Camoufox (default true).
	FallbackEnabled bool
	// StatusLog receives human fallback lines (non-silent fallback).
	StatusLog io.Writer
	// OnStatus is optional; receives every BrowserStatus emission.
	OnStatus func(Status)
	// Wait applies human-like randomized delays (VAL-BRW-006).
	Wait *Waiter
	// History retains lifecycle events for the desktop panel (VAL-BRW-008).
	History *StatusRing
	// ApplyHumanWaitOnNavigate inserts a delay before navigate when true (default).
	ApplyHumanWaitOnNavigate bool

	// Test hooks: inject engines by id instead of real constructors.
	factory map[EngineID]func() Engine

	// HeadlessOverride when non-nil forces headed/headless on constructed engines.
	// nil = each engine's constructor default (stealth/debug headed, e2e/camoufox headless).
	HeadlessOverride *bool

	// lastFallback marks that the active engine was opened via fallback path.
	lastFallback bool
	// closed after Close
	closed bool
}

// NewManager builds a BrowserManager with default adapters.
func NewManager() *Manager {
	return &Manager{
		ports:                    NewPortAllocator(),
		FallbackEnabled:          true,
		StatusLog:                os.Stderr,
		factory:                  nil,
		Wait:                     NewWaiter(DefaultHumanWait()),
		History:                  NewStatusRing(MaxStatusHistory),
		ApplyHumanWaitOnNavigate: true,
	}
}

// SetStatusHandler registers the BrowserStatus callback (desktop/CLI).
func (m *Manager) SetStatusHandler(fn func(Status)) {
	m.mu.Lock()
	m.OnStatus = fn
	m.mu.Unlock()
}

// RegisterEngineFactory overrides construction for tests (e.g. always-fail primary).
func (m *Manager) RegisterEngineFactory(id EngineID, fn func() Engine) {
	m.mu.Lock()
	if m.factory == nil {
		m.factory = map[EngineID]func() Engine{}
	}
	m.factory[NormalizeEngine(id)] = fn
	m.mu.Unlock()
}

// Ports returns the shared CDP port allocator (for engine factories).
func (m *Manager) Ports() *PortAllocator {
	if m == nil {
		return NewPortAllocator()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ports == nil {
		m.ports = NewPortAllocator()
	}
	return m.ports
}

func (m *Manager) emit(st Status) {
	// Never put cookies/auth into status detail paths — callers sanitize tool results.
	if m.History != nil {
		m.History.Push(st)
	}
	if m.OnStatus != nil {
		m.OnStatus(st)
	}
	if st.LogLine != "" && m.StatusLog != nil {
		fmt.Fprintln(m.StatusLog, st.LogLine)
	} else if st.Fallback && m.StatusLog != nil {
		line := fmt.Sprintf("browser: fallback to engine=%s reason=%s", st.Engine, st.Reason)
		fmt.Fprintln(m.StatusLog, line)
		log.Print(line)
	}
}

// SetHumanWait configures human-like wait bounds (VAL-BRW-006).
func (m *Manager) SetHumanWait(cfg HumanWaitConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Wait == nil {
		m.Wait = NewWaiter(cfg)
		return
	}
	m.Wait.SetConfig(cfg)
}

// HumanWaitConfig returns the active wait config.
func (m *Manager) HumanWaitConfig() HumanWaitConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Wait == nil {
		return DefaultHumanWait()
	}
	return m.Wait.Config()
}

// StatusHistory returns retained BrowserStatus events (VAL-BRW-008).
func (m *Manager) StatusHistory() ([]TimedStatus, Status) {
	m.mu.Lock()
	h := m.History
	m.mu.Unlock()
	if h == nil {
		return nil, m.Status()
	}
	return h.Snapshot()
}

// ActiveEngine returns the currently selected engine id (empty if none).
func (m *Manager) ActiveEngine() EngineID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.id
}

// IsFallback reports whether the active engine was opened via fallback.
func (m *Manager) IsFallback() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastFallback
}

// Status returns a snapshot of current engine state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{Engine: m.id, State: StateIdle, Fallback: m.lastFallback}
	if m.engine != nil {
		st.CDPURL = m.engine.CDPURL()
		st.CDPPort = m.engine.CDPPort()
		st.State = StateReady
	}
	return st
}

// SetEngine selects and launches the engine (VAL-BRW-001/002/003).
// On primary launch failure with FallbackEnabled, tries Camoufox (VAL-BRW-004).
func (m *Manager) SetEngine(ctx context.Context, id EngineID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("browser: manager closed")
	}
	id = NormalizeEngine(id)
	m.emit(Status{Engine: id, State: StateSelecting, Detail: "set-engine"})

	// Close previous.
	if m.engine != nil {
		_ = m.engine.Close(ctx)
		m.engine = nil
		m.id = ""
		m.lastFallback = false
	}

	eng, err := m.construct(id)
	if err != nil {
		m.emit(Status{Engine: id, State: StateUnavailable, Reason: err.Error()})
		return err
	}

	// Available check before launch (clear missing-engine path).
	if ok, reason := eng.Available(); !ok {
		uerr := &ErrUnavailable{
			Engine:  id,
			Reason:  reason,
			Install: installHint(id),
		}
		// For stealth, Manager may still fall back to camoufox.
		if m.FallbackEnabled && id != EngineCamoufox {
			if fbErr := m.tryFallbackLocked(ctx, id, uerr.Error()); fbErr == nil {
				return nil
			}
		}
		m.emit(Status{
			Engine: id,
			State:  StateUnavailable,
			Reason: uerr.Error(),
		})
		return uerr
	}

	m.emit(Status{Engine: id, State: StateLaunching})
	if err := eng.Launch(ctx); err != nil {
		// Primary launch failed → Camoufox fallback when configured.
		if m.FallbackEnabled && id != EngineCamoufox {
			if fbErr := m.tryFallbackLocked(ctx, id, err.Error()); fbErr == nil {
				return nil
			}
		}
		// Surface unavailable with install hint when applicable.
		if IsUnavailable(err) {
			m.emit(Status{Engine: id, State: StateUnavailable, Reason: err.Error()})
			return err
		}
		m.emit(Status{Engine: id, State: StateError, Reason: err.Error()})
		return err
	}

	m.engine = eng
	m.id = eng.ID()
	m.lastFallback = false
	st := Status{
		Engine:  eng.ID(),
		State:   StateReady,
		CDPURL:  eng.CDPURL(),
		CDPPort: eng.CDPPort(),
	}
	m.emit(st)
	return nil
}

func (m *Manager) tryFallbackLocked(ctx context.Context, primary EngineID, reason string) error {
	fb, err := m.construct(EngineCamoufox)
	if err != nil {
		return err
	}
	if ok, why := fb.Available(); !ok {
		return &ErrUnavailable{Engine: EngineCamoufox, Reason: why, Install: CamoufoxInstallHint()}
	}
	line := fmt.Sprintf("browser: primary engine %q failed (%s); falling back to camoufox", primary, reason)
	log.Print(line)
	if m.StatusLog != nil {
		fmt.Fprintln(m.StatusLog, line)
	}
	m.emit(Status{
		Engine:   EngineCamoufox,
		State:    StateLaunching,
		Fallback: true,
		Reason:   reason,
		LogLine:  line,
	})
	if err := fb.Launch(ctx); err != nil {
		m.emit(Status{
			Engine:   EngineCamoufox,
			State:    StateError,
			Fallback: true,
			Reason:   err.Error(),
			LogLine:  "browser: camoufox fallback launch failed: " + err.Error(),
		})
		return err
	}
	m.engine = fb
	m.id = EngineCamoufox
	m.lastFallback = true
	m.emit(Status{
		Engine:   EngineCamoufox,
		State:    StateReady,
		Fallback: true,
		Reason:   reason,
		CDPURL:   fb.CDPURL(),
		CDPPort:  fb.CDPPort(),
		LogLine:  fmt.Sprintf("browser: fallback active engine=camoufox fallback=true (primary=%s)", primary),
	})
	return nil
}

func (m *Manager) construct(id EngineID) (Engine, error) {
	id = NormalizeEngine(id)
	var eng Engine
	if m.factory != nil {
		if fn, ok := m.factory[id]; ok && fn != nil {
			eng = fn()
		}
	}
	if eng == nil {
		switch id {
		case EngineDebug:
			eng = NewDebugEngine(m.ports)
		case EngineE2E:
			eng = NewPlaywrightEngine(m.ports)
		case EngineStealth:
			eng = NewCloakBrowserEngine(m.ports)
		case EngineCamoufox:
			eng = NewCamoufoxEngine(m.ports)
		default:
			return nil, fmt.Errorf("browser: unknown engine %q (try debug, e2e/playwright, stealth, camoufox)", id)
		}
	}
	if m.HeadlessOverride != nil {
		applyHeadless(eng, *m.HeadlessOverride)
	}
	return eng, nil
}

func applyHeadless(eng Engine, headless bool) {
	switch e := eng.(type) {
	case *DebugEngine:
		e.Headless = headless
	case *PlaywrightEngine:
		e.Headless = headless
	case *CloakBrowserEngine:
		e.Headless = headless
	case *CamoufoxEngine:
		e.Headless = headless
	}
}

// Navigate loads url on the active engine (VAL-BRW-005).
// Applies human-like wait before navigation when enabled (VAL-BRW-006).
// Rejects shell/exec smuggling in the URL (VAL-BRW-009).
func (m *Manager) Navigate(ctx context.Context, url string) error {
	if err := ValidateNavigateURL(url); err != nil {
		m.emit(Status{State: StateError, URL: url, Reason: err.Error()})
		return err
	}
	m.mu.Lock()
	eng := m.engine
	id := m.id
	fb := m.lastFallback
	doWait := m.ApplyHumanWaitOnNavigate
	waiter := m.Wait
	m.mu.Unlock()
	if eng == nil {
		return fmt.Errorf("browser: no engine selected (use set-engine first)")
	}
	// Human-like wait before navigate (never zero-delay when enabled).
	if doWait && waiter != nil {
		if d, err := waiter.Wait(ctx); err != nil {
			m.emit(Status{Engine: id, State: StateError, URL: url, Reason: err.Error(), Fallback: fb,
				Detail: fmt.Sprintf("human_wait_ms=%d", d.Milliseconds())})
			return err
		} else if d > 0 {
			// Record wait in a navigating-pre status detail (no secrets).
			m.emit(Status{
				Engine: id, State: StateNavigating, URL: url, Fallback: fb,
				Detail: fmt.Sprintf("human_wait_ms=%d navigate-start", d.Milliseconds()),
			})
		} else {
			m.emit(Status{Engine: id, State: StateNavigating, URL: url, Fallback: fb, Detail: "navigate-start"})
		}
	} else {
		m.emit(Status{Engine: id, State: StateNavigating, URL: url, Fallback: fb, Detail: "navigate-start"})
	}
	if err := eng.Navigate(ctx, url); err != nil {
		m.emit(Status{Engine: id, State: StateError, URL: url, Reason: err.Error(), Fallback: fb, Detail: "navigate-failure"})
		return err
	}
	// Validate CDP port confinement if exposed (VAL-BRW-010).
	if p := eng.CDPPort(); p != 0 && !InRange(p) {
		err := fmt.Errorf("browser: cdp port %d outside 4100–4199", p)
		m.emit(Status{Engine: id, State: StateError, URL: url, Reason: err.Error(), Fallback: fb})
		return err
	}
	m.emit(Status{
		Engine: id, State: StateNavigateEnd, URL: url, Fallback: fb,
		CDPURL: eng.CDPURL(), CDPPort: eng.CDPPort(), Detail: "navigate-end",
	})
	return nil
}

// WaitHuman pauses for a human-like (or longer) delay — captcha/MFA handoff (VAL-BRW-006).
// reason is logged in BrowserStatus only (never secrets). minMs/maxMs override defaults when > 0.
func (m *Manager) WaitHuman(ctx context.Context, reason string, minMs, maxMs int) (time.Duration, error) {
	m.mu.Lock()
	id := m.id
	fb := m.lastFallback
	waiter := m.Wait
	m.mu.Unlock()
	if waiter == nil {
		waiter = NewWaiter(DefaultHumanWait())
	}
	// Optional one-shot override range.
	if minMs > 0 || maxMs > 0 {
		cfg := waiter.Config()
		if minMs > 0 {
			cfg.MinMs = minMs
		}
		if maxMs > 0 {
			cfg.MaxMs = maxMs
		}
		cfg.Enabled = true
		tmp := NewWaiter(cfg)
		// Copy sleepFn not needed; new waiter is fine for one-shot.
		waiter = tmp
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "human handoff"
	}
	// Strip anything that looks like cookie dump from reason before status.
	reason = SanitizeForLLM(reason)
	m.emit(Status{
		Engine: id, State: StateWaitingHuman, Fallback: fb,
		Detail: "wait_human", Reason: reason,
	})
	d, err := waiter.Wait(ctx)
	if err != nil {
		m.emit(Status{Engine: id, State: StateError, Fallback: fb, Reason: err.Error(), Detail: "wait_human"})
		return d, err
	}
	// Return to ready if engine still up.
	state := StateReady
	m.mu.Lock()
	if m.engine == nil {
		state = StateIdle
	}
	m.mu.Unlock()
	m.emit(Status{
		Engine: id, State: state, Fallback: fb,
		Detail: fmt.Sprintf("wait_human_done ms=%d", d.Milliseconds()),
		Reason: reason,
	})
	return d, nil
}

// Screenshot captures the active page (VAL-BRW-005).
// Bytes are for disk/artifact use — callers must not dump base64 cookies to the LLM.
func (m *Manager) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	m.mu.Lock()
	eng := m.engine
	m.mu.Unlock()
	if eng == nil {
		return nil, fmt.Errorf("browser: no engine selected")
	}
	return eng.Screenshot(ctx, opts)
}

// Evaluate runs page JS only on the active page (VAL-BRW-005/009).
// Result must be SanitizeForLLM before inclusion in model tool messages (VAL-BRW-006).
func (m *Manager) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	if err := ValidateEvaluateExpression(expression); err != nil {
		return EvalResult{}, err
	}
	m.mu.Lock()
	eng := m.engine
	m.mu.Unlock()
	if eng == nil {
		return EvalResult{}, fmt.Errorf("browser: no engine selected")
	}
	return eng.Evaluate(ctx, expression)
}

// SnapshotText extracts a short page text summary via evaluate (title + body text).
// Always sanitized for LLM (VAL-BRW-006) — never document.cookie / storage.
func (m *Manager) SnapshotText(ctx context.Context) (string, error) {
	// Intentionally does NOT read document.cookie or storage.
	expr := `(function(){
  var t = document.title || '';
  var body = '';
  try { body = (document.body && document.body.innerText) ? document.body.innerText : ''; } catch(e) {}
  if (body.length > 4000) body = body.slice(0, 4000);
  return JSON.stringify({title: t, text: body});
})()`
	res, err := m.Evaluate(ctx, expr)
	if err != nil {
		return "", err
	}
	return SanitizeEvalResult(res), nil
}

// Close stops the active engine and emits engine-stop (VAL-BRW-008).
// The manager remains usable for a subsequent SetEngine (not permanently closed).
// Use Shutdown to mark the manager permanently closed (CLI one-shot).
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	eng := m.engine
	id := m.id
	m.engine = nil
	m.id = ""
	m.lastFallback = false
	// Clear chat SSE emit path before status fan-out so a dead stream cannot panic.
	// Callers that need status still get History push via emit() when OnStatus is set.
	m.mu.Unlock()

	if eng != nil {
		_ = eng.Close(ctx)
		m.emit(Status{Engine: id, State: StateStopped, Detail: "engine-stop"})
	} else {
		m.emit(Status{State: StateStopped, Detail: "engine-stop"})
	}
	return nil
}

// ClearHistory drops retained BrowserStatus events (workspace switch / panel reset).
func (m *Manager) ClearHistory() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.History != nil {
		m.History = NewStatusRing(MaxStatusHistory)
	}
}

// Shutdown stops the engine and marks the manager closed permanently.
func (m *Manager) Shutdown(ctx context.Context) error {
	_ = m.Close(ctx)
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

// Engine returns the live engine instance (nil if none). For tests.
func (m *Manager) Engine() Engine {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.engine
}

func installHint(id EngineID) string {
	switch NormalizeEngine(id) {
	case EngineDebug:
		return "install Google Chrome or: npx playwright install chromium"
	case EngineE2E:
		return "run: npx playwright@1.61.1 install chromium"
	case EngineStealth:
		return CloakInstallHint()
	case EngineCamoufox:
		return CamoufoxInstallHint()
	default:
		return "see docs/browser.md for engine install paths"
	}
}

// FormatUnavailable formats a clear, user-actionable missing-engine error (VAL-BRW-007).
func FormatUnavailable(id EngineID, reason string) string {
	return (&ErrUnavailable{Engine: id, Reason: reason, Install: installHint(id)}).Error()
}

// ParseEngineID parses a CLI/tool engine name.
func ParseEngineID(s string) (EngineID, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "debug":
		return EngineDebug, nil
	case "e2e", "playwright", "pw":
		return EngineE2E, nil
	case "stealth", "cloak", "cloakbrowser":
		return EngineStealth, nil
	case "camoufox", "stealth_fb", "fallback":
		return EngineCamoufox, nil
	case "light", "obscura":
		return EngineLight, nil
	default:
		return "", fmt.Errorf("unknown engine %q (debug|e2e|stealth|camoufox)", s)
	}
}
