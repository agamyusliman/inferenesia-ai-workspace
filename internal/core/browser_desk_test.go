package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
)

func TestGetBrowserStatusSnapshot(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{ConfigHome: dir})
	if err != nil {
		t.Fatal(err)
	}
	st := svc.GetBrowserStatus()
	if st.Product == "" || !strings.Contains(st.Product, "Inferenesia") {
		t.Fatalf("product brand: %+v", st)
	}
	if st.Current.State == "" {
		t.Fatal("current state empty")
	}
	if len(st.Engines) == 0 {
		t.Fatal("expected engine availability rows")
	}
	cfg := svc.BrowserManager().HumanWaitConfig()
	if !cfg.Enabled || cfg.MinMs <= 0 {
		t.Fatalf("human wait not configured: %+v", cfg)
	}
}

func TestBrowserStatusHTTP(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{ConfigHome: dir})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/browser/status", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
	var body BrowserStatusListView
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Current.State == "" {
		t.Fatal("empty current")
	}
}

func TestBrowserSetEngineMissingClearError(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{ConfigHome: dir})
	if err != nil {
		t.Fatal(err)
	}
	m := svc.BrowserManager()
	m.FallbackEnabled = false
	m.RegisterEngineFactory(browser.EngineStealth, func() browser.Engine {
		e := browser.NewCloakBrowserEngine(browser.NewPortAllocator())
		e.BinaryOverride = "/definitely/missing/cloakbrowser"
		return e
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = svc.BrowserSetEngine(ctx, "stealth", nil)
	if err == nil {
		t.Fatal("expected missing engine error")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "install") {
		t.Fatalf("install hint missing: %s", msg)
	}
	if !strings.Contains(msg, "stealth") && !strings.Contains(msg, "cloak") {
		t.Fatalf("engine name missing: %s", msg)
	}
}

func TestWireBrowserBackendTools(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{ConfigHome: dir})
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(nil)
	svc.WireBrowserBackend(reg, nil)
	found := false
	for _, d := range reg.Definitions() {
		if d.Function.Name == "browser_navigate" {
			found = true
		}
		if d.Function.Name == "browser_shell" || d.Function.Name == "browser_exec" {
			t.Fatalf("forbidden tool registered: %s", d.Function.Name)
		}
	}
	if !found {
		t.Fatal("browser_navigate not registered after WireBrowserBackend")
	}
	// Shell bridge execute denial
	res := reg.Execute(context.Background(), tools.Call{Name: "browser_exec", Arguments: `{}`}, nil)
	if !res.IsError {
		t.Fatal("browser_exec must be denied")
	}
}

func TestBrowserLifecycleHistoryViaManager(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{ConfigHome: dir})
	if err != nil {
		t.Fatal(err)
	}
	m := svc.BrowserManager()
	m.SetHumanWait(browser.HumanWaitConfig{Enabled: true, MinMs: 1, MaxMs: 1})
	// Instant wait for unit speed: disable navigate wait and use tiny wait_human.
	m.ApplyHumanWaitOnNavigate = false
	m.RegisterEngineFactory(browser.EngineE2E, func() browser.Engine {
		return &coreFakeEngine{id: browser.EngineE2E}
	})
	ctx := context.Background()
	if err := m.SetEngine(ctx, browser.EngineE2E); err != nil {
		t.Fatal(err)
	}
	if err := m.Navigate(ctx, "data:text/html,<title>ok</title>"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.WaitHuman(ctx, "test", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	snap := svc.GetBrowserStatus()
	states := map[string]bool{}
	for _, h := range snap.History {
		states[h.State] = true
	}
	for _, want := range []string{"selecting", "ready", "navigating", "navigate_end", "waiting_human", "stopped"} {
		if !states[want] {
			// launching may be present too
			if want == "ready" && (states["launching"] || snap.Current.State == "stopped") {
				// still require ready somewhere
			}
			if !states[want] {
				t.Fatalf("missing state %s in history keys=%v current=%+v", want, states, snap.Current)
			}
		}
	}
}

type coreFakeEngine struct {
	id browser.EngineID
}

func (f *coreFakeEngine) ID() browser.EngineID              { return f.id }
func (f *coreFakeEngine) Available() (bool, string)         { return true, "fake" }
func (f *coreFakeEngine) Launch(ctx context.Context) error  { return nil }
func (f *coreFakeEngine) Navigate(ctx context.Context, url string) error {
	return nil
}
func (f *coreFakeEngine) Screenshot(ctx context.Context, opts browser.ScreenshotOpts) ([]byte, error) {
	return []byte{1}, nil
}
func (f *coreFakeEngine) Evaluate(ctx context.Context, expression string) (browser.EvalResult, error) {
	return browser.EvalResult{Value: "ok", Raw: `"ok"`}, nil
}
func (f *coreFakeEngine) Close(ctx context.Context) error { return nil }
func (f *coreFakeEngine) CDPURL() string                  { return "http://127.0.0.1:4115" }
func (f *coreFakeEngine) CDPPort() int                    { return 4115 }
