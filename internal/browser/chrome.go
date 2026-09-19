package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// chromeLauncher starts a Chromium-based binary with remote debugging in 4100–4199.
type chromeLauncher struct {
	Binary   string
	Headless bool
	// ExtraArgs are appended after base args (engine-controlled, never model-controlled).
	ExtraArgs []string
	// UserDataDir when empty uses a temp dir cleaned on Close.
	UserDataDir string

	ports *PortAllocator

	mu      sync.Mutex
	cmd     *exec.Cmd
	port    int
	cdpURL  string
	client  *cdpClient
	tmpData string
	running bool
}

func (l *chromeLauncher) Launch(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return nil
	}
	if l.Binary == "" {
		return fmt.Errorf("chrome: binary path empty")
	}
	if l.ports == nil {
		l.ports = NewPortAllocator()
	}
	port, err := l.ports.Allocate()
	if err != nil {
		return err
	}

	userData := l.UserDataDir
	if userData == "" {
		tmp, err := os.MkdirTemp("", "yura-browser-*")
		if err != nil {
			l.ports.Release(port)
			return err
		}
		userData = tmp
		l.tmpData = tmp
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--user-data-dir=" + userData,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-features=Translate,MediaRouter",
		"--disable-component-update",
		"about:blank",
	}
	if l.Headless {
		args = append([]string{"--headless=new", "--disable-gpu"}, args...)
	}
	args = append(args, l.ExtraArgs...)

	cmd := exec.CommandContext(ctx, l.Binary, args...)
	// Detach from Go test stdin; keep stderr for debug if needed.
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		l.ports.Release(port)
		if l.tmpData != "" {
			_ = os.RemoveAll(l.tmpData)
			l.tmpData = ""
		}
		return fmt.Errorf("chrome start: %w", err)
	}
	l.cmd = cmd
	l.port = port
	l.cdpURL = cdpBrowserURL(port)

	// Wait for CDP endpoint then attach.
	wsURL, err := discoverWSDebuggerURL(ctx, port)
	if err != nil {
		_ = l.killLocked()
		return err
	}
	client, err := connectCDP(ctx, wsURL)
	if err != nil {
		_ = l.killLocked()
		return err
	}
	l.client = client
	l.running = true
	return nil
}

func (l *chromeLauncher) Navigate(ctx context.Context, url string) error {
	l.mu.Lock()
	c := l.client
	l.mu.Unlock()
	if c == nil {
		return fmt.Errorf("chrome: not launched")
	}
	return c.Navigate(ctx, url)
}

func (l *chromeLauncher) Screenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	l.mu.Lock()
	c := l.client
	l.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("chrome: not launched")
	}
	return c.Screenshot(ctx, opts)
}

func (l *chromeLauncher) Evaluate(ctx context.Context, expression string) (EvalResult, error) {
	l.mu.Lock()
	c := l.client
	l.mu.Unlock()
	if c == nil {
		return EvalResult{}, fmt.Errorf("chrome: not launched")
	}
	return c.Evaluate(ctx, expression)
}

func (l *chromeLauncher) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.killLocked()
}

func (l *chromeLauncher) killLocked() error {
	if l.client != nil {
		_ = l.client.Close()
		l.client = nil
	}
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
		_, _ = l.cmd.Process.Wait()
		l.cmd = nil
	}
	if l.port != 0 && l.ports != nil {
		l.ports.Release(l.port)
	}
	port := l.port
	l.port = 0
	l.cdpURL = ""
	l.running = false
	if l.tmpData != "" {
		_ = os.RemoveAll(l.tmpData)
		l.tmpData = ""
	}
	_ = port
	// Give OS a moment to free the TCP port for subsequent tests.
	time.Sleep(50 * time.Millisecond)
	return nil
}

func (l *chromeLauncher) CDPURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cdpURL
}

func (l *chromeLauncher) CDPPort() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.port
}

// discoverChromeBinary finds a Chromium / Chrome binary for debug or e2e.
// Prefer Playwright-cached Chrome for Testing, then system Chrome.
func discoverChromeBinary(preferPlaywright bool) (string, error) {
	var candidates []string

	if preferPlaywright {
		candidates = append(candidates, playwrightChromiumCandidates()...)
	}
	// System / debug candidates
	candidates = append(candidates, systemChromeCandidates()...)
	if !preferPlaywright {
		candidates = append(candidates, playwrightChromiumCandidates()...)
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("chrome binary not found (install Google Chrome or run: npx playwright install chromium)")
}

func playwrightChromiumCandidates() []string {
	home, _ := os.UserHomeDir()
	var out []string
	// Latest known revisions first (mission readiness: Playwright 1.61).
	revs := []string{"chromium-1228", "chromium-1223", "chromium-1200"}
	for _, rev := range revs {
		base := filepath.Join(home, "Library", "Caches", "ms-playwright", rev)
		// macOS arm64 / intel layouts
		out = append(out,
			filepath.Join(base, "chrome-mac-arm64", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing"),
			filepath.Join(base, "chrome-mac", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing"),
			filepath.Join(base, "chrome-linux", "chrome"),
			filepath.Join(base, "chrome-win64", "chrome.exe"),
		)
		// headless shell (good for e2e tests)
		hs := strings.Replace(rev, "chromium-", "chromium_headless_shell-", 1)
		hsBase := filepath.Join(home, "Library", "Caches", "ms-playwright", hs)
		out = append(out,
			filepath.Join(hsBase, "chrome-headless-shell-mac-arm64", "chrome-headless-shell"),
			filepath.Join(hsBase, "chrome-headless-shell-mac", "chrome-headless-shell"),
			filepath.Join(hsBase, "chrome-headless-shell-linux64", "chrome-headless-shell"),
		)
	}
	// Linux XDG cache
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		for _, rev := range revs {
			out = append(out, filepath.Join(xdg, "ms-playwright", rev, "chrome-linux", "chrome"))
		}
	} else if home != "" {
		for _, rev := range revs {
			out = append(out, filepath.Join(home, ".cache", "ms-playwright", rev, "chrome-linux", "chrome"))
		}
	}
	return out
}

func systemChromeCandidates() []string {
	var out []string
	switch runtime.GOOS {
	case "darwin":
		out = append(out,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
		)
	case "linux":
		out = append(out,
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		)
	case "windows":
		out = append(out,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		)
	}
	// PATH lookup
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// resolveHeadedChrome prefers system Chrome for a real external OS window,
// then Playwright Chrome for Testing.
func resolveHeadedChrome() (string, error) {
	return discoverChromeBinary(false)
}

// resolvePlaywrightChrome prefers Playwright-cached browsers for e2e/headless.
func resolvePlaywrightChrome() (string, error) {
	// Prefer full Chrome for Testing over headless shell (more complete CDP).
	home, _ := os.UserHomeDir()
	revs := []string{"chromium-1228", "chromium-1223", "chromium-1200"}
	for _, rev := range revs {
		for _, p := range []string{
			filepath.Join(home, "Library", "Caches", "ms-playwright", rev, "chrome-mac-arm64", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing"),
			filepath.Join(home, "Library", "Caches", "ms-playwright", rev, "chrome-mac", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing"),
			filepath.Join(home, "Library", "Caches", "ms-playwright", rev, "chrome-linux", "chrome"),
		} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
	}
	return discoverChromeBinary(true)
}
