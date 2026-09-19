package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/memory"
	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/store"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/spf13/cobra"
)

func newChatCmd() *cobra.Command {
	var (
		prompt      string
		modelFlag   string
		profileFlag string
		noStream    bool
		showModel   bool
		showTarget  bool
		workspace    string
		noTools      bool
		maxRounds    int
		systemExtra  string
		sessionID    string
		continueSess bool
		noPersist    bool
		verboseMem   bool
		loadSkills   []string
	)
	cmd := &cobra.Command{
		Use:   "chat [prompt...]",
		Short: "Chat with the agent (stream + tools)",
		Long: `Start or resume a chat session with ` + brand.Name + ` against a configured
provider profile (Inferenesia Cloud gateway via INFERENESIA_* and/or BYOK openai_compatible).

Profiles:
  - default inferenesia from INFERENESIA_BASE_URL + INFERENESIA_API_KEY
    (legacy TEMP_AI_* / YURA_AI_* env still honored via dual-read)
  - env BYOK: INFERENESIA_BYOK_BASE_URL + INFERENESIA_BYOK_API_KEY (profile id "byok")
  - ~/.inferenesia/config.yaml providers[] (type openai_compatible)
  - select with --profile <id>

BYOK requests go directly to the profile base_url; Authorization is never
sent to the Inferenesia Cloud host for a non-inferenesia profile (VAL-CLI-011).
Use --show-target to print the request host (never the API key).

Dual modes (VAL-CLI-013): gateway-only OR BYOK-only both work without the other.

Model discovery: when model is unset, the CLI lists models from
GET {base}/models and selects one automatically.

Session persistence (VAL-CLI-014): each chat turn is saved to SQLite under
$INFERENESIA_HOME/sessions.db (legacy $YURA_AI_HOME still read). After quit/restart, ` + brand.Binary + ` sessions
lists prior threads; ` + brand.Binary + ` chat --session <id> reloads messages
and continues the same transcript. Use --continue for the most recent session.

Tools: write_file (via WriteGateway, sandboxed to the workspace), shell, and
terminal_start/read/write/list/stop for long-running PTY sessions.
Tool events print as [tool_start]/[tool_end]/[tool_error] lines on stderr.
File mutations also emit [file_changed] lines and land on the undo stack.
After chat, use ` + "`" + brand.Binary + " undo`" + ` to restore pre-agent dirty content
(not git HEAD) and ` + "`" + brand.Binary + " redo`" + ` to reapply.

Sandbox denials are tool errors (process still exits 0 when the agent finishes).

Streaming tokens are written to stdout as they arrive. SIGINT cancels the
upstream request cleanly.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.TrimSpace(prompt)
			if text == "" {
				text = strings.TrimSpace(strings.Join(args, " "))
			}
			if text == "" {
				return fmt.Errorf("missing prompt: pass args or -p/--prompt")
			}

			router, err := loadProviderRouter(profileFlag)
			if err != nil {
				return err
			}
			// GetProfile supports openai_compatible + anthropic (VAL-PROV-007).
			client, err := router.GetProfile(profileFlag)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			if showTarget {
				// Verbose host only — never print API key (VAL-CLI-011 / VAL-FOUND-012).
				fmt.Fprintf(cmd.OutOrStdout(), "profile: %s\n", client.Name())
				fmt.Fprintf(cmd.OutOrStdout(), "target: %s\n", client.RequestHost())
				fmt.Fprintf(cmd.OutOrStdout(), "type: %s\n", provider.AdapterType(client))
			}

			model := strings.TrimSpace(modelFlag)
			if model == "" {
				model = client.DefaultModel()
			}
			resolved, err := client.ResolveModel(ctx, model)
			if err != nil {
				// VAL-CROSS-009: wrap tempai gateway errors as brand-correct
				// "Inferenesia: gateway unavailable (temp-ai)" so the operator can
				// retry or switch profile; non-tempai profiles keep their
				// existing error text. Never prints the API key.
				return formatProviderErr(wrapGatewayErrIfTempAI(client.Name(), client.RequestHost(), err))
			}
			if showModel {
				fmt.Fprintf(cmd.OutOrStdout(), "model: %s\n", resolved)
			}

			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}

			// Multi Brain detection at chat startup (VAL-MEM-001/002).
			// File-based only: session.md first, then at most 1–2 buckets (no RAG).
			mbSnippet, err := reportMultiBrainStartup(cmd, ws, text, verboseMem)
			if err != nil {
				// Detection failure is non-fatal for chat (sandbox path issues rare).
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: multi-brain: %v\n", err)
			}

			// Tool events go to stderr so stdout stays the assistant transcript.
			eventWriter := cmd.ErrOrStderr()

			var reg *tools.Registry
			var saveStack func() error
			if !noTools {
				gw, cleanup, err := gatewayForChat(ws, eventWriter)
				if err != nil {
					return fmt.Errorf("workspace: %w", err)
				}
				saveStack = cleanup
			reg = tools.NewRegistry(gw)
			if home, herr := config.Home(); herr == nil {
				if level, lerr := config.LoadAutonomy(home); lerr == nil {
					reg.SetAutonomyLevel(level)
				} else {
					reg.SetAutonomyLevel(config.DefaultAutonomy())
				}
			} else {
				reg.SetAutonomyLevel(config.DefaultAutonomy())
			}
			// Diagram tools (VAL-DIAG-002/003/004): create/update/export
			// route through WriteGateway; export renders to PNG/SVG.
			tools.EnsureDiagramFromGateway(reg)
			// Builtin Context7 library-docs tool (P9-ctx): bounded fetch,
			// soft-fail. Disabled by env YURA_AI_CONTEXT7=0.
			tools.EnsureContext7Backend(reg)
				// PTY terminal tools (VAL-TERM-007+): default to detached sessions under
				// config home. In-process foreground sessions register cleanup separately.
				if tb, terr := tools.NewDetachedTerminal(ws); terr == nil {
					reg.SetTerminalBackend(tb)
				}
				// Orchestrator task() (VAL-ORCH-001+): parallel explore, depth 1, synthesis.
				// Events print as [TaskSpawned]/[TaskDone]/[Blocked] on stderr.
				orchMgr := orchestrator.New(orchestrator.Options{
					Emitter: func(ev orchestrator.Event) {
						switch ev.Type {
						case orchestrator.EventTaskSpawned:
							fmt.Fprintf(eventWriter, "[TaskSpawned] task_id=%s category=%s %s\n",
								ev.TaskID, ev.Category, ev.Detail)
						case orchestrator.EventTaskDone:
							if ev.SynthesisConsumed {
								fmt.Fprintf(eventWriter, "[TaskDone] synthesis_consumed=true summary=%s\n",
									truncateCLI(ev.Summary, 200))
								return
							}
							fmt.Fprintf(eventWriter, "[TaskDone] task_id=%s status=%s %s\n",
								ev.TaskID, ev.Status, truncateCLI(ev.Summary, 160))
						case orchestrator.EventBlocked:
							fmt.Fprintf(eventWriter, "[Blocked] task_id=%s %s\n", ev.TaskID, ev.Error)
						}
					},
				})
				reg.SetTaskBackend(tools.NewTaskAdapter(orchMgr, orchestrator.NewExploreRunnerFunc(ws)))
				// Progressive skill loader (VAL-ORCH-004/005): metadata at boot; bodies on load.
				home, _ := config.Home()
				skillLoader := skills.NewDefaultLoader(home, ws, nil)
				reg.SetSkillBackend(tools.NewSkillBackend(skillLoader))
				for _, sn := range loadSkills {
					sn = strings.TrimSpace(sn)
					if sn == "" {
						continue
					}
					if _, lerr := skillLoader.Load(sn); lerr != nil {
						fmt.Fprintf(eventWriter, "warning: skill load %q: %v\n", sn, lerr)
					} else {
						fmt.Fprintf(eventWriter, "[skill_loaded] %s\n", sn)
					}
				}
				// Persist stack after chat even on tool-only partial success.
				defer func() {
					if saveStack != nil {
						_ = saveStack()
					}
				}()
				// Inject skill system-prompt overlay after skills pre-loaded.
				if overlay := skillLoader.SystemPromptOverlay(); overlay != "" {
					// Applied below when building sys.
					systemExtra = joinSystem(systemExtra, overlay)
				}
				// MCP host: env > project .inferenesia/ > ~/.inferenesia/config.yaml (VAL-MCP-001..007).
				// Failures are isolated; unavailable servers log clear errors, live tools stay.
				// MCP tool calls emit [tool_start]/[tool_end]/[tool_error] with namespaced names (VAL-MCP-008).
				cfgHomeMCP, _ := config.Home()
				if mcpHost, merr := tools.AttachMCPFromWorkspace(ctx, reg, cfgHomeMCP, ws, eventWriter); mcpHost != nil {
					defer mcpHost.Close()
					if merr != nil {
						fmt.Fprintf(eventWriter, "warning: mcp: %v\n", merr)
					}
					if names := mcpHost.NamespacedNames(); len(names) > 0 {
						fmt.Fprintf(eventWriter, "[mcp] tools: %s\n", strings.Join(names, ", "))
					}
				} else if merr != nil {
					fmt.Fprintf(eventWriter, "warning: mcp: %v\n", merr)
				}
			}

			sys := core.DefaultSystemPrompt
			if mbSnippet != "" {
				sys = sys + "\n\n" + mbSnippet
			}
			if extra := strings.TrimSpace(systemExtra); extra != "" {
				sys = sys + "\n\n" + extra
			}

			// Session load/create (optional when --no-persist).
			var (
				st       *store.Store
				sessID   string
				history  []provider.Message
				newSess  bool
			)
			if !noPersist {
				st, err = openSessionStore()
				if err != nil {
					return err
				}
				defer st.Close()

				sessID = strings.TrimSpace(sessionID)
				if continueSess && sessID == "" {
					list, lerr := st.ListSessions(1)
					if lerr != nil {
						return lerr
					}
					if len(list) == 0 {
						return fmt.Errorf("no prior sessions to continue; start a new chat first")
					}
					sessID = list[0].ID
				}
				if sessID != "" {
					meta, gerr := st.GetSession(sessID)
					if gerr != nil {
						return gerr
					}
					history, err = st.LoadMessages(sessID)
					if err != nil {
						return err
					}
					// Prefer stored model when CLI did not override.
					if strings.TrimSpace(modelFlag) == "" && strings.TrimSpace(meta.Model) != "" {
						resolved = meta.Model
					}
					fmt.Fprintf(cmd.OutOrStdout(), "session: %s (resumed)\n", sessID)
				} else {
					sessID = store.NewSessionID()
					newSess = true
					fmt.Fprintf(cmd.OutOrStdout(), "session: %s\n", sessID)
				}
			}

			// Build message list for this turn.
			msgs := append([]provider.Message(nil), history...)
			// When history already has a system message, skip re-prepend in core.Run.
			sysForRun := sys
			for _, m := range msgs {
				if m.Role == provider.RoleSystem {
					sysForRun = ""
					break
				}
			}
			msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: text})

			// Optional Token Savers (RTK/Headroom/Caveman/Ponytail) from config home.
			// Defaults off; missing tools pass through and never crash chat.
			cfgHome, _ := config.Home()
			tokenSavers, _ := config.LoadTokenSavers(cfgHome)
			result, runErr := core.Run(ctx, core.AgentConfig{
				Client:        client,
				Tools:         reg,
				Model:         resolved,
				SystemPrompt:  sysForRun,
				Messages:      msgs,
				MaxToolRounds: maxRounds,
				Stream:        !noStream,
				Out:           cmd.OutOrStdout(),
				EventOut:      eventWriter,
				TokenSavers:   tokenSavers,
			})
			if runErr != nil {
				// Still try to persist partial transcript if we have messages.
				if st != nil && sessID != "" && len(result.Messages) > 0 {
					_ = st.SaveTurn(store.Session{
						ID:        sessID,
						Title:     store.TitleFromPrompt(text, 60),
						Model:     resolved,
						Profile:   client.Name(),
						Workspace: ws,
					}, result.Messages)
				}
				// VAL-CROSS-009: wrap tempai gateway errors as brand-correct
				// "Inferenesia: gateway unavailable (temp-ai)" so the operator can
				// retry or switch profile; non-tempai profiles keep their
				// existing error text. Never prints the API key.
				return formatProviderErr(wrapGatewayErrIfTempAI(client.Name(), client.RequestHost(), runErr))
			}

			if st != nil && sessID != "" {
				title := store.TitleFromPrompt(text, 60)
				if !newSess {
					// Keep original title when resuming.
					if meta, gerr := st.GetSession(sessID); gerr == nil && meta.Title != "" {
						title = meta.Title
					}
				}
				if err := st.SaveTurn(store.Session{
					ID:        sessID,
					Title:     title,
					Model:     resolved,
					Profile:   client.Name(),
					Workspace: ws,
				}, result.Messages); err != nil {
					return fmt.Errorf("persist session: %w", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "User prompt (alternative to positional args)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "Model id override (default: profile/TEMP_AI_MODEL or auto from /models)")
	cmd.Flags().StringVar(&profileFlag, "profile", "", "Provider profile id (tempai, byok, or config.yaml providers)")
	cmd.Flags().BoolVar(&noStream, "no-stream", false, "Use non-streaming chat completions")
	cmd.Flags().BoolVar(&showModel, "show-model", false, "Print the resolved model id before streaming")
	cmd.Flags().BoolVar(&showTarget, "show-target", false, "Print profile id and request host (no API key)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root for tools/sandbox (default: current directory)")
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "Disable tool calling (plain chat only)")
	cmd.Flags().IntVar(&maxRounds, "max-tool-rounds", core.DefaultMaxToolRounds, "Max tool-call rounds (0=unlimited, default)")
	cmd.Flags().StringVar(&systemExtra, "system", "", "Extra system prompt appended after defaults")
	cmd.Flags().StringVar(&sessionID, "session", "", "Resume a persisted session id (loads prior messages)")
	cmd.Flags().BoolVar(&continueSess, "continue", false, "Resume the most recently updated session")
	cmd.Flags().BoolVar(&noPersist, "no-persist", false, "Do not save or load sessions from SQLite")
	cmd.Flags().BoolVar(&verboseMem, "verbose-memory", false, "Trace Multi Brain file open order at startup (session.md first)")
	cmd.Flags().StringSliceVar(&loadSkills, "skill", nil, "Pre-load skill(s) for this chat (repeatable; e.g. multi-brain)")
	return cmd
}

func joinSystem(base, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if base == "" {
		return extra
	}
	if extra == "" {
		return base
	}
	return base + "\n\n" + extra
}

// reportMultiBrainStartup prints a memory status line and optionally a verbose
// open-order trace. Reads session.md first then at most 1–2 buckets matching the prompt.
// Returns an optional system-prompt snippet with bounded Multi Brain context (not the whole tree).
func reportMultiBrainStartup(cmd *cobra.Command, ws, prompt string, verbose bool) (string, error) {
	st, err := memory.Detect(ws)
	if err != nil {
		return "", err
	}
	// Status line always on stderr so stdout remains the transcript/stream.
	fmt.Fprintln(cmd.ErrOrStderr(), memory.FormatStatusLine(st))
	if !st.Present {
		if verbose {
			fmt.Fprintln(cmd.ErrOrStderr(), "[memory] no files opened (multi-brain absent)")
		}
		return "", nil
	}
	res, err := memory.Load(ws, prompt)
	if err != nil {
		return "", err
	}
	if verbose {
		fmt.Fprintln(cmd.ErrOrStderr(), "[memory] open order (session.md must be first):")
		memory.WriteTrace(cmd.ErrOrStderr(), res.Trace)
		if len(res.Trace.Opened) > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "[memory] first read: %s\n", filepath.Base(res.Trace.Opened[0]))
		}
	}
	return multiBrainSystemSnippet(res), nil
}

// multiBrainSystemSnippet builds a short system prompt addition: read-order rules
// plus the master index and selected buckets only (never full tree dump).
func multiBrainSystemSnippet(res memory.LoadResult) string {
	if !res.Status.Present {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Multi Brain memory (this workspace)\n")
	b.WriteString("Project memory lives under `.multibrain/`. Rules:\n")
	b.WriteString("1. Treat `session.md` as the master index (already loaded below).\n")
	b.WriteString("2. Only 1–2 bucket files are loaded; do not request the entire `.multibrain/` tree.\n")
	b.WriteString("3. Open additional context files only when a bucket entry points to them.\n")
	b.WriteString("4. Memory is file-based Multi Brain only — no RAG/vector store.\n\n")
	if res.Session != "" {
		b.WriteString("### .multibrain/session.md\n")
		b.WriteString(truncateRunes(res.Session, 4000))
		b.WriteString("\n\n")
	}
	for name, content := range res.Buckets {
		b.WriteString("### .multibrain/indexes/")
		b.WriteString(name)
		b.WriteString(".md\n")
		b.WriteString(truncateRunes(content, 3000))
		b.WriteString("\n\n")
	}
	for rel, content := range res.Contexts {
		b.WriteString("### ")
		b.WriteString(rel)
		b.WriteString("\n")
		b.WriteString(truncateRunes(content, 2000))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func truncateRunes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// byte-safe enough for ASCII-heavy markdown indexes; cap hard.
	if max > 3 && len(s) > max {
		return s[:max-3] + "..."
	}
	return s[:max]
}

// truncateCLI shortens event lines for stderr (orchestrator TaskDone, etc.).
func truncateCLI(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func resolveWorkspace(flag string) (string, error) {
	ws := strings.TrimSpace(flag)
	if ws == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve workspace: %w", err)
		}
		ws = wd
	}
	abs, err := filepath.Abs(ws)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("workspace %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace %q is not a directory", abs)
	}
	return abs, nil
}
