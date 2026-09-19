package browser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// failEngine always fails Launch — used to force Camoufox fallback (VAL-BRW-004).
type failEngine struct {
	id EngineID
}

func (f *failEngine) ID() EngineID                              { return f.id }
func (f *failEngine) Available() (bool, string)                 { return true, "mock" }
func (f *failEngine) Launch(ctx context.Context) error          { return fmt.Errorf("simulated primary launch failure") }
func (f *failEngine) Navigate(ctx context.Context, url string) error {
	return fmt.Errorf("not launched")
}
func (f *failEngine) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	return nil, fmt.Errorf("not launched")
}
func (f *failEngine) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	return EvalResult{}, fmt.Errorf("not launched")
}
func (f *failEngine) Close(ctx context.Context) error { return nil }
func (f *failEngine) CDPURL() string                  { return "" }
func (f *failEngine) CDPPort() int                    { return 0 }

// fakeEngine is a successful in-memory engine for fallback unit tests without browsers.
type fakeEngine struct {
	id     EngineID
	title  string
	shots  int
	closed bool
}

func (f *fakeEngine) ID() EngineID              { return f.id }
func (f *fakeEngine) Available() (bool, string) { return true, "fake" }
func (f *fakeEngine) Launch(ctx context.Context) error {
	return nil
}
func (f *fakeEngine) Navigate(ctx context.Context, url string) error { return nil }
func (f *fakeEngine) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	f.shots++
	return []byte{0x89, 0x50, 0x4e, 0x47}, nil // PNG magic-ish
}
func (f *fakeEngine) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	return EvalResult{Value: f.title, Raw: `"` + f.title + `"`}, nil
}
func (f *fakeEngine) Close(ctx context.Context) error { f.closed = true; return nil }
func (f *fakeEngine) CDPURL() string                  { return "http://127.0.0.1:4111" }
func (f *fakeEngine) CDPPort() int                    { return 4111 }

func TestManagerFallbackToCamoufox(t *testing.T) {
	m := NewManager()
	var logBuf bytes.Buffer
	m.StatusLog = &logBuf
	var events []Status
	var mu sync.Mutex
	m.SetStatusHandler(func(st Status) {
		mu.Lock()
		events = append(events, st)
		mu.Unlock()
	})

	m.RegisterEngineFactory(EngineStealth, func() Engine {
		return &failEngine{id: EngineStealth}
	})
	m.RegisterEngineFactory(EngineCamoufox, func() Engine {
		return &fakeEngine{id: EngineCamoufox, title: "camou-ok"}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.SetEngine(ctx, EngineStealth); err != nil {
		t.Fatalf("SetEngine: %v", err)
	}
	if m.ActiveEngine() != EngineCamoufox {
		t.Fatalf("want camoufox, got %s", m.ActiveEngine())
	}
	if !m.IsFallback() {
		t.Fatal("expected fallback=true")
	}
	logOut := logBuf.String()
	if !strings.Contains(logOut, "fallback") && !strings.Contains(logOut, "camoufox") {
		t.Fatalf("fallback must be logged, got: %q", logOut)
	}
	// Event payload fallback=true + engine=camoufox
	found := false
	mu.Lock()
	for _, ev := range events {
		if ev.Engine == EngineCamoufox && ev.Fallback && ev.State == StateReady {
			found = true
			break
		}
	}
	mu.Unlock()
	if !found {
		t.Fatalf("expected BrowserStatus(engine=camoufox, fallback=true, ready); events=%+v", events)
	}
	_ = m.Close(context.Background())
}

func TestManagerMissingStealthNoFallbackWhenDisabled(t *testing.T) {
	m := NewManager()
	m.FallbackEnabled = false
	m.RegisterEngineFactory(EngineStealth, func() Engine {
		e := NewCloakBrowserEngine(NewPortAllocator())
		e.BinaryOverride = "/no/such/cloakbrowser"
		return e
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := m.SetEngine(ctx, EngineStealth)
	if err == nil {
		_ = m.Close(context.Background())
		t.Fatal("expected unavailable")
	}
	if !IsUnavailable(err) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "install") {
		t.Fatalf("want install hint: %v", err)
	}
}

func TestParseEngineID(t *testing.T) {
	cases := map[string]EngineID{
		"debug":        EngineDebug,
		"e2e":          EngineE2E,
		"playwright":   EngineE2E,
		"stealth":      EngineStealth,
		"cloakbrowser": EngineStealth,
		"camoufox":     EngineCamoufox,
		"stealth_fb":   EngineCamoufox,
	}
	for in, want := range cases {
		got, err := ParseEngineID(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if NormalizeEngine(got) != NormalizeEngine(want) {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
	if _, err := ParseEngineID("nope"); err == nil {
		t.Fatal("expected error")
	}
}
