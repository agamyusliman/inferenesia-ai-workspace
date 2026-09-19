package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
)

// ChatEventBrowserStatus is emitted on every BrowserManager lifecycle transition
// (select, launch, navigate-start/end, failure, engine-stop) — VAL-BRW-008.
// Desktop panel is a subscriber only (no renderer-side browser code).
const ChatEventBrowserStatus ChatEventType = "BrowserStatus"

// BrowserStatusView is the JSON shape for the desktop browser status panel.
type BrowserStatusView struct {
	Engine   string `json:"engine,omitempty"`
	State    string `json:"state"`
	CDPURL   string `json:"cdp_url,omitempty"`
	CDPPort  int    `json:"cdp_port,omitempty"`
	Fallback bool   `json:"fallback,omitempty"`
	Reason   string `json:"reason,omitempty"`
	URL      string `json:"url,omitempty"`
	Detail   string `json:"detail,omitempty"`
	At       string `json:"at,omitempty"`
	// WaitMinMs/WaitMaxMs expose human-wait config (no secrets).
	WaitMinMs int `json:"wait_min_ms,omitempty"`
	WaitMaxMs int `json:"wait_max_ms,omitempty"`
}

// BrowserStatusListView is history + current for the panel (VAL-BRW-008).
type BrowserStatusListView struct {
	Product  string              `json:"product"`
	Current  BrowserStatusView   `json:"current"`
	History  []BrowserStatusView `json:"history"`
	// Engines availability (read-only probe; no process launch).
	Engines []BrowserEngineAvail `json:"engines,omitempty"`
}

// BrowserEngineAvail is a static availability row for the panel.
type BrowserEngineAvail struct {
	Engine  string `json:"engine"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail,omitempty"`
	Install string `json:"install,omitempty"`
}

// browserState holds the process-local BrowserManager + status emit (desktop/CLI).
type browserState struct {
	manager *browser.Manager
	backend *tools.ManagerBrowserBackend
	emitMu  sync.Mutex
	emit    func(ChatEvent)
}

func (s *Service) ensureBrowser() *browserState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser == nil {
		s.browser = &browserState{}
	}
	if s.browser.manager == nil {
		m := browser.NewManager()
		// Human wait defaults on; keep status history for panel.
		m.SetHumanWait(browser.DefaultHumanWait())
		bs := s.browser
		m.SetStatusHandler(func(st browser.Status) {
			bs.pushStatus(st)
		})
		art := filepath.Join(s.configHome, "browser", "artifacts")
		_ = os.MkdirAll(art, 0o700)
		s.browser.manager = m
		s.browser.backend = tools.NewManagerBrowserBackend(m, art)
	}
	return s.browser
}

func (bs *browserState) setEmit(emit func(ChatEvent)) {
	if bs == nil {
		return
	}
	bs.emitMu.Lock()
	bs.emit = emit
	bs.emitMu.Unlock()
}

func (bs *browserState) pushStatus(st browser.Status) {
	if bs == nil {
		return
	}
	// Always retain history for the panel; SSE emit is optional (only during chat).
	st.Reason = browser.SanitizeForLLM(st.Reason)
	st.Detail = browser.SanitizeForLLM(st.Detail)
	bs.emitMu.Lock()
	emit := bs.emit
	bs.emitMu.Unlock()
	if emit == nil {
		return
	}
	defer func() { _ = recover() }()
	ev := ChatEvent{
		Type:   ChatEventBrowserStatus,
		Name:   string(st.Engine),
		Detail: fmt.Sprintf("state=%s", st.State),
		Path:   st.URL,
		Text:   statusText(st),
		Error:  st.Reason,
	}
	parts := []string{fmt.Sprintf("engine=%s", st.Engine), fmt.Sprintf("state=%s", st.State)}
	if st.CDPURL != "" {
		parts = append(parts, "cdp_url="+st.CDPURL)
	}
	if st.CDPPort != 0 {
		parts = append(parts, fmt.Sprintf("cdp_port=%d", st.CDPPort))
	}
	if st.Fallback {
		parts = append(parts, "fallback=true")
	}
	if st.Detail != "" {
		parts = append(parts, "detail="+st.Detail)
	}
	if st.Reason != "" {
		parts = append(parts, "reason="+st.Reason)
	}
	ev.Detail = strings.Join(parts, " ")
	emit(ev)
}

// ResetBrowserForWorkspace stops the engine and clears lifecycle history so
// switching/closing a workspace does not leave stale browser UI.
func (s *Service) ResetBrowserForWorkspace() {
	if s == nil {
		return
	}
	bs := s.browser
	if bs == nil {
		return
	}
	bs.setEmit(nil)
	if bs.manager == nil {
		return
	}
	mgr := bs.manager
	mgr.ClearHistory()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		_ = mgr.Close(ctx)
	}()
}

func statusText(st browser.Status) string {
	return fmt.Sprintf("%s:%s", st.Engine, st.State)
}

// BrowserManager returns the live manager (may init). For tests / CLI.
func (s *Service) BrowserManager() *browser.Manager {
	if s == nil {
		return nil
	}
	return s.ensureBrowser().manager
}

// WireBrowserBackend attaches BrowserManager tools to a registry (chat/CLI).
func (s *Service) WireBrowserBackend(reg *tools.Registry, emit func(ChatEvent)) {
	if s == nil || reg == nil {
		return
	}
	bs := s.ensureBrowser()
	if emit != nil {
		bs.setEmit(emit)
	}
	reg.SetBrowserBackend(bs.backend)
}

// GetBrowserStatus returns current + history for the desktop panel (VAL-BRW-008).
// Panel must not run its own browser code — only render this snapshot.
func (s *Service) GetBrowserStatus() BrowserStatusListView {
	out := BrowserStatusListView{
		Product: s.BrandName(),
		Current: BrowserStatusView{State: string(browser.StateIdle)},
		History: []BrowserStatusView{},
		Engines: probeEngines(),
	}
	if s == nil {
		return out
	}
	bs := s.ensureBrowser()
	m := bs.manager
	if m == nil {
		return out
	}
	hist, last := m.StatusHistory()
	// Prefer live Status() over empty history.
	live := m.Status()
	if live.State != "" && live.State != browser.StateIdle {
		last = live
	}
	cfg := m.HumanWaitConfig()
	out.Current = toStatusView(last, time.Now().UTC(), cfg)
	for _, h := range hist {
		out.History = append(out.History, toStatusView(h.Status, h.At, cfg))
	}
	return out
}

func toStatusView(st browser.Status, at time.Time, cfg browser.HumanWaitConfig) BrowserStatusView {
	atStr := ""
	if !at.IsZero() {
		atStr = at.UTC().Format(time.RFC3339)
	}
	return BrowserStatusView{
		Engine:    string(st.Engine),
		State:     string(st.State),
		CDPURL:    st.CDPURL,
		CDPPort:   st.CDPPort,
		Fallback:  st.Fallback,
		Reason:    browser.SanitizeForLLM(st.Reason),
		URL:       st.URL,
		Detail:    browser.SanitizeForLLM(st.Detail),
		At:        atStr,
		WaitMinMs: cfg.MinMs,
		WaitMaxMs: cfg.MaxMs,
	}
}

func probeEngines() []BrowserEngineAvail {
	ports := browser.NewPortAllocator()
	engines := []browser.Engine{
		browser.NewDebugEngine(ports),
		browser.NewPlaywrightEngine(ports),
		browser.NewCloakBrowserEngine(ports),
		browser.NewCamoufoxEngine(ports),
	}
	var out []BrowserEngineAvail
	for _, e := range engines {
		ok, detail := e.Available()
		row := BrowserEngineAvail{
			Engine: string(e.ID()),
			OK:     ok,
			Detail: detail,
		}
		if !ok {
			row.Install = browser.FormatUnavailable(e.ID(), detail)
		}
		out = append(out, row)
	}
	return out
}

// BrowserSetEngine selects an engine via Service (hub API / tests).
// headless nil = engine default; true/false override.
func (s *Service) BrowserSetEngine(ctx context.Context, engine string, headless *bool) (BrowserStatusView, error) {
	bs := s.ensureBrowser()
	if err := bs.backend.SetEngine(ctx, engine, headless); err != nil {
		return BrowserStatusView{}, err
	}
	st := s.GetBrowserStatus()
	return st.Current, nil
}

// BrowserNavigate navigates via Service.
func (s *Service) BrowserNavigate(ctx context.Context, url string) (BrowserStatusView, error) {
	bs := s.ensureBrowser()
	if err := bs.backend.Navigate(ctx, url); err != nil {
		return BrowserStatusView{}, err
	}
	return s.GetBrowserStatus().Current, nil
}

// BrowserClose stops the active engine.
func (s *Service) BrowserClose(ctx context.Context) error {
	bs := s.ensureBrowser()
	bs.setEmit(nil)
	return bs.backend.Close(ctx)
}
