package browser

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHumanWaitNotZeroWhenEnabled(t *testing.T) {
	w := NewWaiter(HumanWaitConfig{Enabled: true, MinMs: 5, MaxMs: 15})
	// Override sleep to capture requested duration without real delay.
	var got time.Duration
	w.mu.Lock()
	w.sleepFn = func(ctx context.Context, d time.Duration) error {
		got = d
		return nil
	}
	w.mu.Unlock()

	for i := 0; i < 20; i++ {
		d, err := w.Wait(context.Background())
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		if d <= 0 {
			t.Fatalf("enabled human wait must not be zero-delay, got %v", d)
		}
		if d < 5*time.Millisecond || d > 15*time.Millisecond {
			t.Fatalf("delay %v outside [5ms,15ms]", d)
		}
		if got != d {
			t.Fatalf("sleep saw %v, returned %v", got, d)
		}
	}
}

func TestSanitizeForLLMRedactsCookiesAndAuth(t *testing.T) {
	raw := `title=Hi cookie=sessionid=abc123; auth=secret document.cookie="a=1" Authorization: Bearer supersecrettoken123 storage_state={"cookies":[{"value":"leak"}]}`
	out := SanitizeForLLM(raw)
	if strings.Contains(out, "sessionid=abc123") {
		t.Fatalf("cookie value leaked: %q", out)
	}
	if strings.Contains(out, "supersecrettoken123") {
		t.Fatalf("bearer leaked: %q", out)
	}
	if strings.Contains(out, `"value":"leak"`) {
		t.Fatalf("storage cookie value leaked: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected redaction markers: %q", out)
	}
	// After sanitize, ContainsSensitive should be false for clean redactions
	// of the common patterns (or true only if residual leak remains).
	if ContainsSensitive(out) {
		// Re-sanitize once; residual cookie= prose is OK if values redacted.
		out2 := SanitizeForLLM(out)
		if strings.Contains(out2, "sessionid=abc123") || strings.Contains(out2, "supersecrettoken") {
			t.Fatalf("still sensitive after sanitize: %q", out2)
		}
	}
}

func TestSanitizeEvalResultPageTextOK(t *testing.T) {
	res := EvalResult{Value: "Example Domain", Raw: `"Example Domain"`}
	out := SanitizeEvalResult(res)
	if !strings.Contains(out, "Example Domain") {
		t.Fatalf("page text lost: %q", out)
	}
}

func TestValidateNavigateURLRejectsExec(t *testing.T) {
	cases := []string{
		"https://example.com --exec /bin/sh",
		"https://example.com && rm -rf /",
		"javascript:alert(1)",
		"https://x.example --eval code",
	}
	for _, c := range cases {
		if err := ValidateNavigateURL(c); err == nil {
			t.Fatalf("expected reject for %q", c)
		}
	}
	if err := ValidateNavigateURL("https://example.com/path?q=1"); err != nil {
		t.Fatalf("valid url rejected: %v", err)
	}
}

func TestValidateEvaluateRejectsChildProcess(t *testing.T) {
	if err := ValidateEvaluateExpression("require('child_process').exec('ls')"); err == nil {
		t.Fatal("expected reject")
	}
	if err := ValidateEvaluateExpression("document.title"); err != nil {
		t.Fatalf("valid page js rejected: %v", err)
	}
}

func TestValidateLaunchArgsFromModel(t *testing.T) {
	if err := ValidateLaunchArgsFromModel([]string{"--remote-debugging-port=9222"}); err == nil {
		t.Fatal("model must not supply launch args")
	}
	if err := ValidateLaunchArgsFromModel(nil); err != nil {
		t.Fatal(err)
	}
}

func TestHasShellBridgeTool(t *testing.T) {
	if !HasShellBridgeTool("browser_shell") || !HasShellBridgeTool("browser_exec") {
		t.Fatal("shell bridges must be detected")
	}
	for _, n := range BrowserToolNames {
		if HasShellBridgeTool(n) {
			t.Fatalf("%s is a real tool, not a shell bridge", n)
		}
	}
}

func TestManagerLifecycleEvents(t *testing.T) {
	m := NewManager()
	m.SetHumanWait(HumanWaitConfig{Enabled: true, MinMs: 1, MaxMs: 2})
	// Instant sleep for unit speed.
	m.Wait.mu.Lock()
	m.Wait.sleepFn = func(ctx context.Context, d time.Duration) error { return nil }
	m.Wait.mu.Unlock()

	var events []Status
	var mu sync.Mutex
	m.SetStatusHandler(func(st Status) {
		mu.Lock()
		events = append(events, st)
		mu.Unlock()
	})

	// Fake engine that succeeds.
	m.RegisterEngineFactory(EngineE2E, func() Engine {
		return &fakeEngine{id: EngineE2E, title: "T"}
	})

	ctx := context.Background()
	if err := m.SetEngine(ctx, EngineE2E); err != nil {
		t.Fatalf("set-engine: %v", err)
	}
	if err := m.Navigate(ctx, "https://example.com"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if _, err := m.WaitHuman(ctx, "captcha", 1, 2); err != nil {
		t.Fatalf("wait_human: %v", err)
	}
	if err := m.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Required lifecycle states (VAL-BRW-008).
	wantAny := []StatusState{
		StateSelecting, StateLaunching, StateReady,
		StateNavigating, StateNavigateEnd,
		StateWaitingHuman, StateStopped,
	}
	mu.Lock()
	seen := map[StatusState]bool{}
	for _, e := range events {
		seen[e.State] = true
	}
	hist, _ := m.StatusHistory()
	mu.Unlock()
	for _, w := range wantAny {
		if !seen[w] {
			t.Fatalf("missing lifecycle state %s; events=%v hist=%d", w, events, len(hist))
		}
	}
	if len(hist) < 4 {
		t.Fatalf("history too short: %d", len(hist))
	}
}

func TestManagerNavigateRejectsExecURL(t *testing.T) {
	m := NewManager()
	m.ApplyHumanWaitOnNavigate = false
	m.RegisterEngineFactory(EngineE2E, func() Engine {
		return &fakeEngine{id: EngineE2E}
	})
	ctx := context.Background()
	_ = m.SetEngine(ctx, EngineE2E)
	err := m.Navigate(ctx, "https://example.com --exec /bin/sh")
	if err == nil {
		t.Fatal("expected sandbox rejection")
	}
}

func TestManagerMissingEngineClearError(t *testing.T) {
	m := NewManager()
	m.FallbackEnabled = false
	m.RegisterEngineFactory(EngineStealth, func() Engine {
		e := NewCloakBrowserEngine(NewPortAllocator())
		e.BinaryOverride = "/no/such/cloakbrowser-binary"
		return e
	})
	err := m.SetEngine(context.Background(), EngineStealth)
	if err == nil {
		t.Fatal("expected unavailable")
	}
	if !IsUnavailable(err) {
		t.Fatalf("want ErrUnavailable, got %T %v", err, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "stealth") && !strings.Contains(msg, "cloak") {
		t.Fatalf("engine name missing: %s", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "install") {
		t.Fatalf("install guidance missing: %s", msg)
	}
	// No panic — we're still running.
}

func TestScreenshotResultNeverInlinesCookies(t *testing.T) {
	out := ScreenshotResultForLLM("/tmp/shot.png", 1234, EngineE2E)
	if strings.Contains(strings.ToLower(out), "cookie=") && !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("unexpected cookie material: %q", out)
	}
	if !strings.Contains(out, "/tmp/shot.png") {
		t.Fatalf("path missing: %q", out)
	}
}

func TestPortAllocatorNeverForbidden(t *testing.T) {
	a := NewPortAllocator()
	reserved := []int{}
	for i := 0; i < 40; i++ {
		p, err := a.Allocate()
		if err != nil {
			// Exhaustion in full range is OK when many reserved; release and continue.
			for _, rp := range reserved {
				a.Release(rp)
			}
			reserved = reserved[:0]
			p, err = a.Allocate()
			if err != nil {
				t.Fatalf("allocate: %v", err)
			}
		}
		if p == 5000 || p == 7000 || p == 5599 {
			t.Fatalf("forbidden port allocated: %d", p)
		}
		if !InRange(p) {
			t.Fatalf("out of range: %d", p)
		}
		reserved = append(reserved, p)
	}
	for _, p := range reserved {
		a.Release(p)
	}
}

func TestDebugLaunchArgsRejectOutOfRange(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for out-of-range port")
		}
	}()
	_ = DebugLaunchArgs(5000)
}

func TestFormatUnavailable(t *testing.T) {
	s := FormatUnavailable(EngineStealth, "binary missing")
	if !strings.Contains(s, "stealth") || !strings.Contains(strings.ToLower(s), "install") {
		t.Fatalf("bad format: %s", s)
	}
}

// Ensure fakeEngine from manager_test is usable with CDP range check (fake uses 4111).
func TestFakeEnginePortInRange(t *testing.T) {
	f := &fakeEngine{id: EngineE2E}
	if !InRange(f.CDPPort()) {
		t.Fatalf("fake CDP port %d not in range", f.CDPPort())
	}
	_ = fmt.Sprintf("%v", f.ID())
}
