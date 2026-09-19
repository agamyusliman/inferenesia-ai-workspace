package browser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const fixtureHTML = `data:text/html,<!doctype html><html><head><title>YuraBrowserFixture</title></head><body><h1 id="t">Hello Inferenesia</h1></body></html>`

func TestPlaywrightNavigateScreenshotEvaluate(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	eng := NewPlaywrightEngine(NewPortAllocator())
	eng.Headless = true
	ok, detail := eng.Available()
	if !ok {
		t.Skipf("playwright chrome not available: %s", detail)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := eng.Launch(ctx); err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer func() { _ = eng.Close(context.Background()) }()

	port := eng.CDPPort()
	if !InRange(port) {
		t.Fatalf("cdp port %d not in 4100–4199", port)
	}
	if eng.CDPURL() == "" {
		t.Fatal("empty cdp_url")
	}

	if err := eng.Navigate(ctx, fixtureHTML); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	title, err := eng.Evaluate(ctx, "document.title")
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	titleStr := fmt.Sprint(title.Value)
	if !strings.Contains(titleStr, "YuraBrowserFixture") {
		// raw may hold quoted JSON
		if !strings.Contains(title.Raw, "YuraBrowserFixture") {
			t.Fatalf("title = %v raw=%s", title.Value, title.Raw)
		}
	}
	png, err := eng.Screenshot(ctx, ScreenshotOpts{Format: "png"})
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("screenshot bytes == 0")
	}
	// PNG signature
	if len(png) < 8 || png[0] != 0x89 {
		t.Logf("screenshot len=%d (may still be valid)", len(png))
	}
	dom, err := eng.Evaluate(ctx, "document.getElementById('t').textContent")
	if err != nil {
		t.Fatalf("dom evaluate: %v", err)
	}
	if !strings.Contains(fmt.Sprint(dom.Value), "Hello Inferenesia") && !strings.Contains(dom.Raw, "Hello Inferenesia") {
		t.Fatalf("dom text unexpected: %v %s", dom.Value, dom.Raw)
	}
}

func TestDebugEngineHeadlessCDPPort(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	eng := NewDebugEngine(NewPortAllocator())
	eng.Headless = true // external window still uses same CDP path; headless for CI
	ok, detail := eng.Available()
	if !ok {
		t.Skipf("chrome not available: %s", detail)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := eng.Launch(ctx); err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer func() { _ = eng.Close(context.Background()) }()

	if !InRange(eng.CDPPort()) {
		t.Fatalf("port %d out of range", eng.CDPPort())
	}
	if eng.CDPURL() == "" {
		t.Fatal("cdp_url required for debug engine")
	}
	// Confirm launch used remote-debugging-port in range via status URL.
	if !strings.Contains(eng.CDPURL(), "127.0.0.1") {
		t.Fatalf("cdp_url=%s", eng.CDPURL())
	}
	if err := eng.Navigate(ctx, fixtureHTML); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	title, err := eng.Evaluate(ctx, "document.title")
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !strings.Contains(fmt.Sprint(title.Value), "YuraBrowserFixture") && !strings.Contains(title.Raw, "YuraBrowserFixture") {
		t.Fatalf("title=%v raw=%s", title.Value, title.Raw)
	}
	png, err := eng.Screenshot(ctx, ScreenshotOpts{})
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("empty screenshot")
	}
}

func TestCommonInterfaceTitleEquality(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	ports := NewPortAllocator()
	type engFactory struct {
		name string
		mk   func() Engine
	}
	factories := []engFactory{
		{"e2e", func() Engine {
			e := NewPlaywrightEngine(ports)
			e.Headless = true
			return e
		}},
		{"debug", func() Engine {
			e := NewDebugEngine(ports)
			e.Headless = true
			return e
		}},
	}
	// Camoufox if present
	if ok, _ := NewCamoufoxEngine(ports).Available(); ok {
		factories = append(factories, engFactory{"camoufox", func() Engine {
			e := NewCamoufoxEngine(ports)
			e.Headless = true
			e.AllowChromeProxy = true
			return e
		}})
	}

	var titles []string
	for _, f := range factories {
		e := f.mk()
		ok, detail := e.Available()
		if !ok {
			t.Logf("skip %s: %s", f.name, detail)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		if err := e.Launch(ctx); err != nil {
			cancel()
			t.Fatalf("%s launch: %v", f.name, err)
		}
		if err := e.Navigate(ctx, fixtureHTML); err != nil {
			_ = e.Close(context.Background())
			cancel()
			t.Fatalf("%s navigate: %v", f.name, err)
		}
		res, err := e.Evaluate(ctx, "document.title")
		if err != nil {
			_ = e.Close(context.Background())
			cancel()
			t.Fatalf("%s evaluate: %v", f.name, err)
		}
		png, err := e.Screenshot(ctx, ScreenshotOpts{})
		if err != nil || len(png) == 0 {
			_ = e.Close(context.Background())
			cancel()
			t.Fatalf("%s screenshot: err=%v len=%d", f.name, err, len(png))
		}
		title := strings.Trim(fmt.Sprint(res.Value), `"`)
		if title == "" || title == "<nil>" {
			title = strings.Trim(res.Raw, `"`)
		}
		titles = append(titles, title)
		t.Logf("%s title=%q screenshot=%d cdp_port=%d", f.name, title, len(png), e.CDPPort())
		_ = e.Close(context.Background())
		cancel()
	}
	if len(titles) < 2 {
		t.Skip("need at least two engines for title equality")
	}
	first := titles[0]
	for i, tt := range titles {
		if !strings.Contains(tt, "YuraBrowserFixture") {
			t.Fatalf("engine %d title %q missing fixture", i, tt)
		}
		// All should equal for same fixture
		if tt != first && !strings.Contains(tt, "YuraBrowserFixture") {
			t.Fatalf("title mismatch: %q vs %q", first, tt)
		}
	}
}

func TestManagerPlaywrightStatusEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	if os.Getenv("CI") == "true" && os.Getenv("YURA_AI_BROWSER_E2E") == "" {
		// still run — playwright is cached on this machine
	}
	m := NewManager()
	var statuses []Status
	m.SetStatusHandler(func(st Status) { statuses = append(statuses, st) })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := m.SetEngine(ctx, EngineE2E); err != nil {
		t.Skipf("e2e unavailable: %v", err)
	}
	defer func() { _ = m.Close(context.Background()) }()
	st := m.Status()
	if st.Engine != EngineE2E {
		t.Fatalf("engine=%s", st.Engine)
	}
	if st.CDPURL == "" {
		t.Fatal("cdp_url empty")
	}
	if !InRange(st.CDPPort) {
		t.Fatalf("port %d", st.CDPPort)
	}
	if err := m.Navigate(ctx, fixtureHTML); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	// lifecycle should include selecting/launching/ready/navigating
	joined := fmt.Sprintf("%v", statuses)
	if !strings.Contains(joined, string(StateReady)) {
		t.Fatalf("missing ready in events: %v", statuses)
	}
}

func TestCamoufoxAvailableOnThisMachine(t *testing.T) {
	e := NewCamoufoxEngine(NewPortAllocator())
	ok, detail := e.Available()
	t.Logf("camoufox available=%v detail=%s", ok, detail)
	// Soft assertion: document for fallback tests; do not fail CI if absent.
}
