package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
)

// mockBrowser implements BrowserBackend for unit tests (no real browser).
type mockBrowser struct {
	engine    string
	state     string
	navigate  string
	evalCalls []string
	waitMs    int
	closed    bool
	setErr    error
	shots     int
}

func (m *mockBrowser) SetEngine(ctx context.Context, id string, headless *bool) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.engine = id
	m.state = "ready"
	return nil
}
func (m *mockBrowser) Status() (string, string, string, int, bool, string) {
	return m.engine, m.state, "", 0, false, ""
}
func (m *mockBrowser) Navigate(ctx context.Context, url string) error {
	if err := browser.ValidateNavigateURL(url); err != nil {
		return err
	}
	m.navigate = url
	return nil
}
func (m *mockBrowser) Screenshot(ctx context.Context, outPath string) (string, int, error) {
	m.shots++
	path := outPath
	if path == "" {
		path = "/tmp/yura-test-shot.png"
	}
	return path, 12, nil
}
func (m *mockBrowser) Evaluate(ctx context.Context, expression string) (string, error) {
	if err := browser.ValidateEvaluateExpression(expression); err != nil {
		return "", err
	}
	m.evalCalls = append(m.evalCalls, expression)
	// Simulate a page that would expose cookies if we naively returned them.
	raw := `{"title":"ok","cookie":"sessionid=secret123"}`
	return browser.SanitizeForLLM(raw), nil
}
func (m *mockBrowser) Snapshot(ctx context.Context) (string, error) {
	return browser.PageTextResultForLLM("Hello page text"), nil
}
func (m *mockBrowser) WaitHuman(ctx context.Context, reason string, minMs, maxMs int) (int, error) {
	m.waitMs = 10
	if minMs > 0 {
		m.waitMs = minMs
	}
	return m.waitMs, nil
}
func (m *mockBrowser) Close(ctx context.Context) error { m.closed = true; return nil }
func (m *mockBrowser) ArtifactDir() string             { return "/tmp" }

func TestBrowserToolsNoShellBridge(t *testing.T) {
	r := NewRegistry(nil)
	mb := &mockBrowser{state: "idle"}
	r.SetBrowserBackend(mb)

	// Registered names must not include shell bridges.
	for _, d := range r.Definitions() {
		if browser.HasShellBridgeTool(d.Function.Name) {
			t.Fatalf("shell bridge registered: %s", d.Function.Name)
		}
		if d.Function.Name == "browser_shell" || d.Function.Name == "browser_exec" {
			t.Fatalf("forbidden tool registered: %s", d.Function.Name)
		}
	}

	// Explicit call to forbidden name is denied.
	res := r.Execute(context.Background(), Call{Name: "browser_exec", Arguments: `{"command":"ls"}`}, nil)
	if !res.IsError {
		t.Fatal("browser_exec must error")
	}
	if !strings.Contains(res.Content, "shell") && !strings.Contains(res.Content, "sandbox") {
		t.Fatalf("expected sandbox message: %s", res.Content)
	}
}

func TestBrowserEvaluateSanitizesCookies(t *testing.T) {
	r := NewRegistry(nil)
	mb := &mockBrowser{}
	r.SetBrowserBackend(mb)
	res := r.Execute(context.Background(), Call{
		Name:      NameBrowserEvaluate,
		Arguments: `{"expression":"document.title"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("eval error: %s", res.Content)
	}
	if strings.Contains(res.Content, "sessionid=secret123") {
		t.Fatalf("cookie leaked to model: %s", res.Content)
	}
	if browser.ContainsSensitive(res.Content) {
		// Values must be redacted.
		if strings.Contains(res.Content, "secret123") {
			t.Fatalf("secret in tool result: %s", res.Content)
		}
	}
}

func TestBrowserEvaluateRejectsExecArgs(t *testing.T) {
	r := NewRegistry(nil)
	r.SetBrowserBackend(&mockBrowser{})
	res := r.Execute(context.Background(), Call{
		Name:      NameBrowserEvaluate,
		Arguments: `{"expression":"1+1","exec":"rm -rf /"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("expected reject of exec field")
	}
}

func TestBrowserSetEngineRejectsLaunchArgs(t *testing.T) {
	r := NewRegistry(nil)
	r.SetBrowserBackend(&mockBrowser{})
	args, _ := json.Marshal(map[string]any{
		"engine":      "e2e",
		"launch_args": []string{"--exec", "/bin/sh"},
	})
	res := r.Execute(context.Background(), Call{Name: NameBrowserSetEngine, Arguments: string(args)}, nil)
	if !res.IsError {
		t.Fatal("expected reject of model launch args")
	}
}

func TestBrowserMissingEngineClearError(t *testing.T) {
	r := NewRegistry(nil)
	r.SetBrowserBackend(&mockBrowser{
		setErr: &browser.ErrUnavailable{
			Engine:  browser.EngineStealth,
			Reason:  "binary not found",
			Install: "pip install cloakbrowser[geoip]",
		},
	})
	res := r.Execute(context.Background(), Call{
		Name:      NameBrowserSetEngine,
		Arguments: `{"engine":"stealth"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("expected error")
	}
	if !strings.Contains(res.Content, "stealth") {
		t.Fatalf("engine name missing: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "install") {
		t.Fatalf("install guidance missing: %s", res.Content)
	}
}

func TestBrowserWaitHuman(t *testing.T) {
	r := NewRegistry(nil)
	mb := &mockBrowser{}
	r.SetBrowserBackend(mb)
	res := r.Execute(context.Background(), Call{
		Name:      NameBrowserWaitHuman,
		Arguments: `{"reason":"captcha","min_ms":5,"max_ms":10}`,
	}, nil)
	if res.IsError {
		t.Fatalf("wait: %s", res.Content)
	}
	if !strings.Contains(res.Content, "waited_ms=") {
		t.Fatalf("missing waited_ms: %s", res.Content)
	}
	if mb.waitMs <= 0 {
		t.Fatal("wait not applied")
	}
}

func TestBrowserNavigateRejectsExecURL(t *testing.T) {
	r := NewRegistry(nil)
	r.SetBrowserBackend(&mockBrowser{})
	res := r.Execute(context.Background(), Call{
		Name:      NameBrowserNavigate,
		Arguments: `{"url":"https://example.com --exec /bin/sh"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("expected navigate sandbox reject")
	}
}

func TestBrowserScreenshotReturnsPathNotBase64(t *testing.T) {
	r := NewRegistry(nil)
	r.SetBrowserBackend(&mockBrowser{})
	res := r.Execute(context.Background(), Call{
		Name:      NameBrowserScreenshot,
		Arguments: `{}`,
	}, nil)
	if res.IsError {
		t.Fatalf("screenshot: %s", res.Content)
	}
	if strings.Contains(res.Content, "iVBOR") || strings.Contains(res.Content, "base64") {
		t.Fatalf("must not inline image bytes: %s", res.Content)
	}
	if !strings.Contains(res.Content, "screenshot_path=") && !strings.Contains(res.Content, ".png") {
		t.Fatalf("expected path reference: %s", res.Content)
	}
}

func TestBrowserToolsRegisteredWhenBackendSet(t *testing.T) {
	r := NewRegistry(nil)
	names := map[string]bool{}
	for _, d := range r.Definitions() {
		names[d.Function.Name] = true
	}
	if names[NameBrowserNavigate] {
		t.Fatal("browser tools must not appear without backend")
	}
	r.SetBrowserBackend(&mockBrowser{})
	names = map[string]bool{}
	for _, d := range r.Definitions() {
		names[d.Function.Name] = true
	}
	for _, n := range browser.BrowserToolNames {
		if !names[n] {
			t.Fatalf("missing tool %s", n)
		}
	}
}

func TestManagerBrowserBackendHumanWait(t *testing.T) {
	m := browser.NewManager()
	m.SetHumanWait(browser.HumanWaitConfig{Enabled: true, MinMs: 1, MaxMs: 2})
	// Instant sleep.
	// Access via WaitHuman which uses Waiter.
	// Use exported SetHumanWait only; override through a one-shot low range.
	b := NewManagerBrowserBackend(m, t.TempDir())
	m.RegisterEngineFactory(browser.EngineE2E, func() browser.Engine {
		return &stubEngine{}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.SetEngine(ctx, "e2e", nil); err != nil {
		// stub may fail Available — register via factory after SetEngine construct path.
		// Register before set:
	}
	// Re-create properly
	m2 := browser.NewManager()
	m2.SetHumanWait(browser.HumanWaitConfig{Enabled: true, MinMs: 1, MaxMs: 1})
	m2.RegisterEngineFactory(browser.EngineE2E, func() browser.Engine { return &stubEngine{} })
	b2 := NewManagerBrowserBackend(m2, t.TempDir())
	// Factory is used for e2e only if we re-register after NewManagerBrowserBackend SetEngine
	// SetEngine re-registers Playwright — override after:
	// Call SetEngine through manager directly.
	if err := m2.SetEngine(ctx, browser.EngineE2E); err != nil {
		t.Fatalf("set: %v", err)
	}
	ms, err := b2.WaitHuman(ctx, "test", 1, 1)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if ms < 1 {
		t.Fatalf("expected ≥1ms wait, got %d", ms)
	}
}

// stubEngine is a minimal Engine for tools package tests.
type stubEngine struct{}

func (s *stubEngine) ID() browser.EngineID                              { return browser.EngineE2E }
func (s *stubEngine) Available() (bool, string)                         { return true, "stub" }
func (s *stubEngine) Launch(ctx context.Context) error                  { return nil }
func (s *stubEngine) Navigate(ctx context.Context, url string) error     { return nil }
func (s *stubEngine) Screenshot(ctx context.Context, opts browser.ScreenshotOpts) ([]byte, error) {
	return []byte{1, 2, 3}, nil
}
func (s *stubEngine) Evaluate(ctx context.Context, expression string) (browser.EvalResult, error) {
	return browser.EvalResult{Value: "ok", Raw: `"ok"`}, nil
}
func (s *stubEngine) Close(ctx context.Context) error { return nil }
func (s *stubEngine) CDPURL() string                  { return "http://127.0.0.1:4112" }
func (s *stubEngine) CDPPort() int                    { return 4112 }

func TestStubEnginePortInRange(t *testing.T) {
	s := &stubEngine{}
	if !browser.InRange(s.CDPPort()) {
		t.Fatalf("port %d", s.CDPPort())
	}
	_ = fmt.Sprintf("%s", s.ID())
}
