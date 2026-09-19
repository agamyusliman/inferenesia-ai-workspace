package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/browser"
	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/spf13/cobra"
)

func newBrowserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browser",
		Short: "Multi-engine browser manager (debug CDP, Playwright, stealth)",
		Long: `BrowserManager drives local browser sidecars (not an in-app webview).

Engines:
  debug       External Chromium/Chrome window + CDP on ports 4100–4199
  e2e         Playwright Chrome for Testing (headless-friendly)
  stealth     CloakBrowser (anti-bot); unavailable when not installed
  camoufox    Camoufox fallback

Safety:
  - Human-like randomized wait on navigate / wait-human
  - Cookies/auth/storage never printed to model/stdout dumps
  - Launch flags are server-controlled (no --exec)

Examples:
  ` + brand.Binary + ` browser set-engine debug
  ` + brand.Binary + ` browser set-engine e2e
  ` + brand.Binary + ` browser navigate https://example.com
  ` + brand.Binary + ` browser screenshot --out /tmp/page.png
  ` + brand.Binary + ` browser evaluate 'document.title'
  ` + brand.Binary + ` browser wait-human --reason captcha --min-ms 100 --max-ms 200
  ` + brand.Binary + ` browser status
  ` + brand.Binary + ` browser close`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newBrowserSetEngineCmd())
	cmd.AddCommand(newBrowserStatusCmd())
	cmd.AddCommand(newBrowserNavigateCmd())
	cmd.AddCommand(newBrowserScreenshotCmd())
	cmd.AddCommand(newBrowserEvaluateCmd())
	cmd.AddCommand(newBrowserWaitHumanCmd())
	cmd.AddCommand(newBrowserCloseCmd())
	cmd.AddCommand(newBrowserInstallCmd())
	return cmd
}

// sessionManager keeps a process-local manager for multi-step CLI within one invocation.
// For separate process invocations, set-engine launches, runs action, and closes.
// Persistent sessions across processes will land with desktop; here we support
// one-shot flags and an optional --keep session file under YURA_AI_HOME.

func withManager(run func(context.Context, *browser.Manager) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	m := browser.NewManager()
	// Stream BrowserStatus lines to stderr (not cookies/secrets).
	m.SetStatusHandler(func(st browser.Status) {
		parts := []string{
			fmt.Sprintf("BrowserStatus engine=%s state=%s", st.Engine, st.State),
		}
		if st.CDPURL != "" {
			parts = append(parts, "cdp_url="+st.CDPURL)
		}
		if st.CDPPort != 0 {
			parts = append(parts, fmt.Sprintf("cdp_port=%d", st.CDPPort))
		}
		if st.Fallback {
			parts = append(parts, "fallback=true")
		}
		if st.Reason != "" {
			parts = append(parts, "reason="+st.Reason)
		}
		if st.URL != "" {
			parts = append(parts, "url="+st.URL)
		}
		fmt.Fprintln(os.Stderr, strings.Join(parts, " "))
	})
	defer func() { _ = m.Close(context.Background()) }()
	return run(ctx, m)
}

func newBrowserSetEngineCmd() *cobra.Command {
	var headless bool
	cmd := &cobra.Command{
		Use:   "set-engine <debug|e2e|stealth|camoufox>",
		Short: "Select and launch a browser engine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := browser.ParseEngineID(args[0])
			if err != nil {
				return err
			}
			return withManager(func(ctx context.Context, m *browser.Manager) error {
				m.FallbackEnabled = false
				ports := m.Ports()
				// Wire headless for all engines when --headless is set.
				// Defaults without flag: debug/stealth headed, e2e/camoufox headless.
				m.RegisterEngineFactory(browser.EngineDebug, func() browser.Engine {
					e := browser.NewDebugEngine(ports)
					e.Headless = headless
					return e
				})
				m.RegisterEngineFactory(browser.EngineE2E, func() browser.Engine {
					e := browser.NewPlaywrightEngine(ports)
					e.Headless = true
					if headless {
						e.Headless = true
					} else if cmd.Flags().Changed("headless") && !headless {
						e.Headless = false
					}
					return e
				})
				m.RegisterEngineFactory(browser.EngineStealth, func() browser.Engine {
					e := browser.NewCloakBrowserEngine(ports)
					e.Headless = headless
					return e
				})
				m.RegisterEngineFactory(browser.EngineCamoufox, func() browser.Engine {
					e := browser.NewCamoufoxEngine(ports)
					if cmd.Flags().Changed("headless") {
						e.Headless = headless
					}
					return e
				})
				if err := m.SetEngine(ctx, id); err != nil {
					return err
				}
				st := m.Status()
				mode := "headed"
				if headless {
					mode = "headless"
				} else if id == browser.EngineE2E || id == browser.EngineCamoufox {
					if !cmd.Flags().Changed("headless") {
						mode = "headless"
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "engine=%s mode=%s state=%s fallback=%v\n", st.Engine, mode, st.State, st.Fallback)
				if st.CDPURL != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "cdp_url=%s\n", st.CDPURL)
				}
				if st.CDPPort != 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "cdp_port=%d\n", st.CDPPort)
					if !browser.InRange(st.CDPPort) {
						return fmt.Errorf("browser: cdp port %d outside 4100–4199", st.CDPPort)
					}
				}
				if (id == browser.EngineDebug || id == browser.EngineStealth) && !headless && os.Getenv("YURA_AI_BROWSER_QUICK") == "" {
					fmt.Fprintln(cmd.OutOrStdout(), "window launched (closing on CLI exit; use hub tools for long sessions)")
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&headless, "headless", false, "no OS window (stealth/debug default headed; e2e default headless)")
	return cmd
}

func newBrowserStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show configured engine availability (no process attach required)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ports := browser.NewPortAllocator()
			engines := []browser.Engine{
				browser.NewDebugEngine(ports),
				browser.NewPlaywrightEngine(ports),
				browser.NewCloakBrowserEngine(ports),
				browser.NewCamoufoxEngine(ports),
			}
			for _, e := range engines {
				ok, detail := e.Available()
				state := "ready"
				if !ok {
					state = "unavailable"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "engine=%-10s state=%-12s detail=%s\n", e.ID(), state, detail)
				if !ok {
					fmt.Fprintf(cmd.OutOrStdout(), "  install: %s\n", browser.FormatUnavailable(e.ID(), detail))
				}
			}
			return nil
		},
	}
}

func newBrowserNavigateCmd() *cobra.Command {
	var engineName string
	var headless bool
	cmd := &cobra.Command{
		Use:   "navigate <url>",
		Short: "Launch engine (if needed) and navigate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			id, err := browser.ParseEngineID(engineName)
			if err != nil {
				return err
			}
			return withManager(func(ctx context.Context, m *browser.Manager) error {
				if id == browser.EngineDebug {
					m.RegisterEngineFactory(browser.EngineDebug, func() browser.Engine {
						e := browser.NewDebugEngine(nil)
						e.Headless = headless || os.Getenv("YURA_AI_BROWSER_HEADLESS") == "1"
						return e
					})
				}
				if id == browser.EngineE2E {
					m.RegisterEngineFactory(browser.EngineE2E, func() browser.Engine {
						e := browser.NewPlaywrightEngine(nil)
						e.Headless = true
						return e
					})
				}
				if err := m.SetEngine(ctx, id); err != nil {
					return err
				}
				if err := m.Navigate(ctx, url); err != nil {
					return err
				}
				title, err := m.Evaluate(ctx, "document.title")
				if err == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "title=%v\n", title.Value)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "navigated=%s engine=%s fallback=%v\n", url, m.ActiveEngine(), m.IsFallback())
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&engineName, "engine", "e2e", "engine id (debug|e2e|stealth|camoufox)")
	cmd.Flags().BoolVar(&headless, "headless", false, "headless mode for debug")
	return cmd
}

func newBrowserScreenshotCmd() *cobra.Command {
	var engineName, outPath string
	var headless bool
	cmd := &cobra.Command{
		Use:   "screenshot",
		Short: "Navigate optional URL then write screenshot bytes",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := browser.ParseEngineID(engineName)
			if err != nil {
				return err
			}
			url, _ := cmd.Flags().GetString("url")
			return withManager(func(ctx context.Context, m *browser.Manager) error {
				if id == browser.EngineDebug {
					m.RegisterEngineFactory(browser.EngineDebug, func() browser.Engine {
						e := browser.NewDebugEngine(nil)
						e.Headless = headless || os.Getenv("YURA_AI_BROWSER_HEADLESS") == "1"
						return e
					})
				}
				if id == browser.EngineE2E {
					m.RegisterEngineFactory(browser.EngineE2E, func() browser.Engine {
						e := browser.NewPlaywrightEngine(nil)
						e.Headless = true
						return e
					})
				}
				if err := m.SetEngine(ctx, id); err != nil {
					return err
				}
				if url != "" {
					if err := m.Navigate(ctx, url); err != nil {
						return err
					}
				}
				png, err := m.Screenshot(ctx, browser.ScreenshotOpts{Format: "png"})
				if err != nil {
					return err
				}
				if len(png) == 0 {
					return fmt.Errorf("browser: screenshot empty")
				}
				if outPath == "" {
					home, _ := config.Home()
					dir := filepath.Join(home, "browser", "artifacts")
					_ = os.MkdirAll(dir, 0o700)
					outPath = filepath.Join(dir, fmt.Sprintf("shot-%d.png", time.Now().Unix()))
				}
				if err := os.WriteFile(outPath, png, 0o600); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "screenshot=%s bytes=%d engine=%s\n", outPath, len(png), m.ActiveEngine())
				// Also print base64 length for tests without dumping image.
				_ = base64.StdEncoding
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&engineName, "engine", "e2e", "engine id")
	cmd.Flags().StringVar(&outPath, "out", "", "output PNG path")
	cmd.Flags().String("url", "data:text/html,<title>YuraShot</title><h1>ok</h1>", "optional URL to open before shot")
	cmd.Flags().BoolVar(&headless, "headless", true, "headless")
	return cmd
}

func newBrowserEvaluateCmd() *cobra.Command {
	var engineName string
	cmd := &cobra.Command{
		Use:   "evaluate <js>",
		Short: "Evaluate JavaScript in the active page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := browser.ParseEngineID(engineName)
			if err != nil {
				return err
			}
			url, _ := cmd.Flags().GetString("url")
			return withManager(func(ctx context.Context, m *browser.Manager) error {
				m.RegisterEngineFactory(browser.EngineE2E, func() browser.Engine {
					e := browser.NewPlaywrightEngine(nil)
					e.Headless = true
					return e
				})
				m.RegisterEngineFactory(browser.EngineDebug, func() browser.Engine {
					e := browser.NewDebugEngine(nil)
					e.Headless = true
					return e
				})
				if err := m.SetEngine(ctx, id); err != nil {
					return err
				}
				if url != "" {
					if err := m.Navigate(ctx, url); err != nil {
						return err
					}
				}
				res, err := m.Evaluate(ctx, args[0])
				if err != nil {
					return err
				}
				// Sanitize before stdout so CLI dumps match LLM policy (VAL-BRW-006).
				fmt.Fprintln(cmd.OutOrStdout(), browser.SanitizeEvalResult(res))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&engineName, "engine", "e2e", "engine id")
	cmd.Flags().String("url", "data:text/html,<title>YuraEval</title>", "page to load first")
	return cmd
}

func newBrowserWaitHumanCmd() *cobra.Command {
	var reason string
	var minMs, maxMs int
	cmd := &cobra.Command{
		Use:   "wait-human",
		Short: "Human-like randomized wait (captcha/MFA handoff; VAL-BRW-006)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withManager(func(ctx context.Context, m *browser.Manager) error {
				// No engine required for pure wait path (status still emits).
				d, err := m.WaitHuman(ctx, reason, minMs, maxMs)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "waited_ms=%d reason=%s\n", d.Milliseconds(), browser.SanitizeForLLM(reason))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "human handoff", "why waiting (no secrets)")
	cmd.Flags().IntVar(&minMs, "min-ms", browser.DefaultHumanWaitMinMs, "minimum wait milliseconds")
	cmd.Flags().IntVar(&maxMs, "max-ms", browser.DefaultHumanWaitMaxMs, "maximum wait milliseconds")
	return cmd
}

func newBrowserCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close",
		Short: "No-op status note (one-shot CLI sessions close automatically)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "browser sessions in CLI one-shot mode close when the command exits")
			return nil
		},
	}
}

func newBrowserInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <stealth|e2e|camoufox>",
		Short: "Print install instructions for an engine sidecar",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := browser.ParseEngineID(args[0])
			if err != nil {
				return err
			}
			switch browser.NormalizeEngine(id) {
			case browser.EngineE2E:
				fmt.Fprintln(cmd.OutOrStdout(), "npx playwright@1.61.1 install chromium")
				fmt.Fprintln(cmd.OutOrStdout(), "Browsers cache: ~/Library/Caches/ms-playwright (macOS)")
			case browser.EngineStealth:
				fmt.Fprintln(cmd.OutOrStdout(), browser.CloakInstallHint())
				fmt.Fprintln(cmd.OutOrStdout(), "Binary cache (auto-downloaded on first launch): ~/.cloakbrowser/chromium-<ver>/")
				fmt.Fprintln(cmd.OutOrStdout(), "Venv (project-local): ~/.inferenesia/browser/engines/cloakbrowser/venv/")
				fmt.Fprintln(cmd.OutOrStdout(), "Verify: inferenesia browser status   # stealth should report ready")
			case browser.EngineCamoufox:
				fmt.Fprintln(cmd.OutOrStdout(), browser.CamoufoxInstallHint())
			case browser.EngineDebug:
				fmt.Fprintln(cmd.OutOrStdout(), "Install Google Chrome, or use Playwright chromium cache.")
			default:
				fmt.Fprintln(cmd.OutOrStdout(), browser.FormatUnavailable(id, "see docs/browser.md"))
			}
			return nil
		},
	}
}
