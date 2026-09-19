// Package core hosts the transport-agnostic agent loop (CLI and desktop share this).
// File mutations always go through tools → WriteGateway.
package core

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
)

// DefaultMaxToolRounds is 0 = unlimited tool rounds (stop only when the model
// stops requesting tools, context cancels, or an explicit positive MaxToolRounds).
// Pass a positive MaxToolRounds to enforce a cap for scripts/tests.
const DefaultMaxToolRounds = 0

// Streamer is the subset of provider.ChatClient used by the agent loop.
type Streamer interface {
	ChatStream(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error)
}

// TurnRunner is an optional non-streaming turn API (OpenAICompat implements this).
type TurnRunner interface {
	ChatTurn(ctx context.Context, req provider.ChatRequest) (provider.Message, error)
}

// AgentConfig configures one agent run (single user prompt or continuing session messages).
type AgentConfig struct {
	// Client is required (streaming and/or turn).
	Client Streamer
	// Tools registers write_file / shell (and later more). Nil disables tool calling.
	Tools *tools.Registry
	// Model is the resolved model id (required non-empty for the request).
	Model string
	// Messages is the conversation so far (must include at least one user message).
	Messages []provider.Message
	// SystemPrompt, when non-empty, is prepended as a system message if Messages has none.
	SystemPrompt string
	// MaxToolRounds caps tool-call iterations. 0 or negative = unlimited
	// (DefaultMaxToolRounds). Positive = hard stop after that many tool rounds.
	MaxToolRounds int
	// Stream enables SSE streaming for assistant text when true.
	Stream bool
	// Out receives streamed token deltas and final text lines.
	Out io.Writer
	// EventOut receives human-readable tool events (ToolStart/ToolEnd/ToolError).
	// When nil, events are discarded (tools still execute).
	EventOut io.Writer

	// TokenSavers holds optional RTK / Headroom / Caveman / Ponytail toggles (defaults all off).
	// Applied inside Run: system fragments (Caveman/Ponytail), tool-result egress (RTK),
	// pre-model messages (Headroom). Missing tools pass through and never crash (VAL-SAVER).
	TokenSavers config.TokenSaversConfig
	// RTKCompress optional override for tests / custom compressors. Nil uses binary probe path.
	RTKCompress RTKCompressFunc
	// HeadroomCompress optional override for tests / custom compressors. Nil uses HTTP base_url.
	HeadroomCompress HeadroomCompressFunc
	// Discipline overrides DefaultContextDiscipline when non-nil. Canvas agents
	// (slides/html/…) pass CanvasContextDiscipline so large HTML user prompts
	// are not truncated to MaxUserRunes=4000.
	Discipline *DisciplineOptions
}

// AgentResult is the outcome of one Run.
type AgentResult struct {
	// Messages is the full conversation including assistant and tool turns.
	Messages []provider.Message
	// Final is the last assistant text content (may be empty if the last act was tools-only).
	Final string
	// ToolRounds is how many times the model requested tools.
	ToolRounds int
}

// DefaultSystemPrompt guides the model to use write_file and shell tools.
const DefaultSystemPrompt = `You are Inferenesia, a local coding assistant.

You have tools:
- read_file: read a UTF-8 text file inside the workspace (read-only).
- grep: search file contents under the workspace (read-only).
- list_dir: list directory entries under the workspace (read-only).
- write_file: create or overwrite a file inside the workspace via WriteGateway (paths relative to workspace root). Writes outside the workspace are denied by sandbox. All writes are undoable.
- write_multibrain: write Multi Brain memory files under .multibrain/ (also via WriteGateway, undoable).
- write_diagram: save diagram sources under docs/diagrams/ (via WriteGateway, undoable).
- shell: run a short shell command in the workspace and return exit code, stdout, stderr.
- terminal_start: start a long-running process in a PTY (servers, watchers). Prefer shell for short one-shots.
- terminal_read: read captured PTY output (tail) for a session and reason about logs.
- terminal_write: send stdin to a running PTY session.
- terminal_list: list PTY sessions (id, command, pid, status).
- terminal_stop: stop a PTY session (safe no-op if already stopped).
- git_status, git_diff, git_log, git_stage, git_unstage: read and stage workspace git state.
- git_commit: create a commit — ALWAYS surfaces NeedsApproval; only proceeds if the user approves.
- git_fetch, git_pull, git_push: remote ops. Push requires confirmation; force/force-with-lease always needs explicit approval. Default push is non-force.
- git_branch_list, git_checkout: branch navigation.
- git_reset_hard, git_branch_delete: DESTRUCTIVE — always need explicit user confirmation; cancel leaves the tree unchanged.
- propose_plan: propose a structured plan with title/summary/body and an ordered steps[] checklist for the user to Approve or Reject. After PlanProposed, wait — do not execute mutating work until Approve seeds todos.
- todo_list: list session todos (id, content, status).
- todo_write: replace the full todo checklist (pending | in_progress | completed | cancelled).
- todo_update: update one todo by id (move pending → in_progress → completed as you work).
- get_goal: read current or specified mission/goal (status, blocker, gates, evidence).
- create_goal: create a mission with objective and optional budget_tokens (status pending).
- attach_mission_evidence: attach verification evidence (test output / command result). Required before complete. Secrets are redacted.
- update_goal: set status to complete or blocked only. complete fails with "evidence required" if no verification evidence is attached. blocked needs a reason.
- task: spawn parallel background sub-tasks (explore/implement/verify). Prefer prompts[] with ≥2 explore prompts for concurrent search. Depth default is 1 — a sub-task cannot call task() again (error: orchestrator: max depth 1 reached). After parallel explores complete, call task with synthesize=true to merge results WITHOUT re-running the same full explore search. One sub-task failure does not abort the others (partial result + error). Parallel overlapping writes are blocked.
- skill: list available skills (metadata only) or load a named skill to change the session toolset/system prompt. Loading multi-brain grants multibrain_read and multibrain_append. Bodies are not dumped at boot — load on demand.
- multibrain_read / multibrain_append: available after skill load multi-brain. Read order is session.md → 1–2 buckets → pointed contexts only (never the whole .multibrain/ tree). Append writes one newest-first entry with timestamp "YYYY-MM-DD HH:mm WIB — agent: summary -> .multibrain/context/…".
- browser_set_engine / browser_navigate / browser_screenshot / browser_evaluate / browser_snapshot / browser_wait_human / browser_status / browser_close: multi-engine browser sidecars (debug CDP external window, Playwright e2e, stealth CloakBrowser, camoufox). Evaluate is page JS only — no host shell/exec. Human-like wait is applied on navigate. Never dump cookies, auth tokens, or storage state into answers — only page text and screenshot *paths*.
- MCP tools (when configured under mcp: in config.yaml): namespaced as mcp.<server>.<tool> so they never collide with built-ins (e.g. mcp.docs.ping). Stdio and HTTP servers only. Failures on one server do not stop others. Never request or log Authorization header values.

When Plan Mode is active, only read-only tools are available (including propose_plan and todo_list). Write tools (write_file, shell, git commit/push, todo_write/todo_update, create_goal, task, multibrain_append, …) are denied with "plan mode: writes denied" until the user exits Plan Mode (approve/reject/cancel).
When planning non-trivial work, call propose_plan with concrete steps so the UI can show Approve/Reject. On approve, steps become todos (pending). On reject, no todos are created.

## Plan / multi-step execution (checklist required)
Do **not** free-form execute a multi-step plan without a checklist.
1. **Plan first** when the work is non-trivial: use propose_plan (Plan Mode) or, after Plan Mode exits, ensure a todo list exists via todo_write with clear pending steps (milestones).
2. **Execute only from the checklist**: work one item at a time — set it in_progress with todo_update, do the work, mark completed, then move to the next. Prefer a single in_progress item.
3. **Parallel work**: use task() for independent explores/implements, but still map outcomes back to todos (complete or update the matching checklist items).
4. **Mission path**: for goal-scale work, create_goal / mission milestones + attach_mission_evidence; still keep session todos in sync when the UI shows a checklist.
5. **No silent bulk execution**: do not implement many files/steps without creating or updating todos first. If the user approved a plan, the checklist is the source of truth.
6. **Done means checklist done**: do not claim overall completion while pending/in_progress todos remain unless the user cancelled or reprioritized them.

As you execute after approval, keep todos updated in real time: set one item in_progress, then completed when done.
Never mark a mission complete without verification evidence (tests/gates). Use attach_mission_evidence first. If a gate fails, use update_goal status=blocked with the reason.
When the user asks you to create a file, you MUST call write_file (do not only describe the file).
When the user asks you to update Multi Brain memory, use write_multibrain with a path under .multibrain/.
When the user asks you to save a diagram, use write_diagram under docs/diagrams/.
When the user asks you to run a shell command, you MUST call shell and report the tool output.
When the user asks what a long-running process printed, use terminal_read on the session id.
For multi-area codebase search, use task with prompts[] (category=explore) in parallel, then synthesize=true — do not re-run the same full search as primary.
Never force-push or hard-reset without the user approving the confirmation dialog.
If a tool returns a sandbox denial, plan mode denial, NeedsApproval reject, or error, explain that clearly; do not claim the write or git op succeeded.

## When to write a clear summary (only when finished)
Give a user-facing summary **only after the work for this turn is actually done** — not while you still need more tools, not mid-exploration, not as a premature "done" before the job is complete.
- If you still need tools: call them; do not close with a full summary yet.
- If blocked or waiting on the user: say so briefly and ask — do not invent a completion summary.
- When truly finished (or answering a simple question without tools): end with a clear summary, not only tool logs.
Structure for non-trivial finished work (explore, implement, multi-file, multi-step):
1. Short heading of what you did / found.
2. Numbered or bulleted findings, changes, or next steps (paths and concrete outcomes).
3. Optional short "How to verify" when you changed code.
For simple Q&A, a tight complete paragraph is enough — never an empty bubble.
Prefer the user's language when they write in Indonesian or another language.
Avoid dumping raw tool output as the only reply.

## Decisions and options (when unsure)
If you are unsure about a product/tech choice, trade-off, or next step that needs the user's decision:
1. Briefly state the situation and why a choice is needed.
2. Offer **2–5 concrete options** as a numbered list the user can pick (e.g. "1) … 2) … 3) …").
3. Recommend one option when you have a preference, and say why — still wait for the user.
	4. Tell the user they can reply with a number (1/2/3) or type their own answer.
Do not silently pick a high-impact direction (rewrite architecture, delete data, force-push, large refactors) without options or confirmation when the request is ambiguous.`

// SlidesSystemPrompt is the base prompt for the "slides" agent kind — a
// presentation slides HTML editor. It deliberately omits coding tools / file
// system / shell references so the model stays in slide-editing scope.
const SlidesSystemPrompt = `You are a presentation slides HTML editor. You receive slide HTML (1280×720 artboard, Tailwind CSS) and user instructions. Follow the user's instructions exactly — edit, create, or rearrange slides using SEARCH/REPLACE patches or full section replacements. Output only the HTML changes requested. Do not reference coding tools, file systems, or shell commands. Do not create diagrams, sequence diagrams, or non-slide content.`

// ExcalidrawSystemPrompt is the base prompt for the "excalidraw" agent kind.
// It keeps the model in whiteboard/diagram-editing scope.
const ExcalidrawSystemPrompt = `You are an Excalidraw whiteboard editor. You receive Excalidraw element JSON and user instructions. Follow the user's instructions exactly — create, modify, or rearrange visual elements. Output only the Excalidraw changes requested. Do not reference coding tools, file systems, or shell commands. Do not create slide decks, sequence diagrams, or non-Excalidraw content.`

// DiagramSystemPrompt is the base prompt for the "diagram" agent kind.
// It keeps the model in diagram DSL (Mermaid/PlantUML/Graphviz) scope.
const DiagramSystemPrompt = `You are a diagram source editor. You receive diagram DSL code (Mermaid, PlantUML, or Graphviz) and user instructions. Follow the user's instructions exactly — create, modify, or refactor diagram source code. Output only the diagram DSL changes requested. Do not reference coding tools, file systems, or shell commands. Do not create slides, Excalidraw elements, or non-diagram content.`

// HtmlCanvasSystemPrompt is the base prompt for the "html" agent kind.
// It keeps the model in HTML/CSS web-preview editing scope.
const HtmlCanvasSystemPrompt = `You are an HTML web page editor. You receive HTML source code and user instructions. Follow the user's instructions exactly — create, modify, or rearrange HTML and CSS. Output only the HTML changes requested. Do not reference coding tools, file systems, or shell commands. Do not create slides, diagrams, or non-HTML content.`

// ImageSystemPrompt is the base prompt for the "image" agent kind.
// It keeps the model in image generation scope.
const ImageSystemPrompt = `You are an image generation assistant. You receive image generation instructions and produce images based on the user's request. Follow the user's instructions exactly. Do not reference coding tools, file systems, or shell commands. Do not create slides, diagrams, HTML, or non-image content.`

const MarkdownSystemPrompt = `You are a Markdown document editor. Return only the complete Markdown requested, preserving the user's language, facts, links, and sections not targeted by the instruction. Structure content with meaningful headings and readable paragraphs. Do not invent evidence, citations, quotes, or statistics. Do not reference coding tools, file systems, or shell commands.`

const TableSystemPrompt = `You are a spreadsheet editor working on a workbook that may hold several sheets. Return only the complete workbook in the exact JSON format specified by the request, with one entry per requested sheet — never collapse several requested sheets into one. A cell whose text starts with "=" is a live formula: use formulas for totals, percentages, and any derived value instead of hard-coding computed numbers. Apply formatting the request asks for through the style fields (bold, fill, color, align, numFmt) and set column widths and row heights when they make the report readable. When the user supplies real figures, reproduce them exactly and leave genuinely unknown cells empty. When the user asks you to create, draft, or demonstrate a report, build the full structure with headers, plausible rows, and a formula-driven total row. Do not reference coding tools, file systems, or shell commands.`

const TimelineSystemPrompt = `You are a timeline and milestone editor. Return only the complete timeline in the exact CSV or JSON format specified by the request. Preserve milestone identities and unedited dates, statuses, and notes. Use valid ISO dates and ensure end dates do not precede start dates. When the user gives real commitments, reproduce them exactly. When the user asks you to draft, plan, or demonstrate a schedule, produce the milestones they described with realistic sequencing. Do not reference coding tools, file systems, or shell commands.`

// SystemPromptForKind returns the base system prompt for an agent kind.
// "" / "coding" → DefaultSystemPrompt (coding assistant + all tools).
// "slides" → SlidesSystemPrompt (presentation design, no coding tools).
// "excalidraw" → ExcalidrawSystemPrompt (whiteboard elements, no coding tools).
// "diagram" → DiagramSystemPrompt (diagram DSL, no coding tools).
// "html" → HtmlCanvasSystemPrompt (web page HTML/CSS, no coding tools).
// "image" → ImageSystemPrompt (image generation, no coding tools).
func SystemPromptForKind(kind string) string {
	switch kind {
	case "slides":
		return SlidesSystemPrompt
	case "excalidraw":
		return ExcalidrawSystemPrompt
	case "diagram":
		return DiagramSystemPrompt
	case "html":
		return HtmlCanvasSystemPrompt
	case "image":
		return ImageSystemPrompt
	case "markdown":
		return MarkdownSystemPrompt
	case "table":
		return TableSystemPrompt
	case "timeline":
		return TimelineSystemPrompt
	default:
		return DefaultSystemPrompt
	}
}

// Run executes the agent tool-calling loop until the model stops requesting tools.
// When MaxToolRounds > 0, stops with an error after that many tool rounds.
// When MaxToolRounds <= 0 (default), there is no round cap — stop via model Done,
// context cancel (Stop/Esc), or provider error. Tool errors are returned to the
// model as tool messages (the process does not exit non-zero solely because of a sandbox denial).
func Run(ctx context.Context, cfg AgentConfig) (AgentResult, error) {
	if cfg.Client == nil {
		return AgentResult{}, fmt.Errorf("core: agent client is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return AgentResult{}, fmt.Errorf("core: model is required")
	}
	// Positive = hard cap. 0 / negative = unlimited (DefaultMaxToolRounds).
	maxRounds := cfg.MaxToolRounds
	unlimited := maxRounds <= 0
	out := cfg.Out
	if out == nil {
		out = io.Discard
	}
	eventOut := cfg.EventOut
	if eventOut == nil {
		eventOut = io.Discard
	}

	msgs := append([]provider.Message(nil), cfg.Messages...)
	if len(msgs) == 0 {
		return AgentResult{}, fmt.Errorf("core: no messages")
	}
	// Optional Token Saver system fragments (Caveman / Ponytail) when enabled (VAL-SAVER-005/006).
	sysPrompt := ApplyTokenSaverSystemFragments(cfg.SystemPrompt, cfg.TokenSavers)
	if sys := strings.TrimSpace(sysPrompt); sys != "" && !hasSystem(msgs) {
		msgs = append([]provider.Message{{Role: provider.RoleSystem, Content: sys}}, msgs...)
	} else if hasSystem(msgs) {
		// Reconcile managed fragments so toggling a saver off applies on the next turn.
		msgs = appendSaverFragmentsToExistingSystem(msgs, cfg.TokenSavers)
	}

	var toolDefs []provider.ToolDef
	if cfg.Tools != nil {
		toolDefs = ToolsToProvider(cfg.Tools.Definitions())
	}

	var final string
	toolRounds := 0

	// One core.Run is one agent turn for undo granularity (VAL-UNDO-004/005):
	// all WriteFile calls during tool rounds share a single LIFO stack unit.
	if cfg.Tools != nil {
		cfg.Tools.BeginTurn("")
		defer cfg.Tools.EndTurn()
	}

	for round := 0; ; round++ {
		if err := ctx.Err(); err != nil {
			return AgentResult{Messages: msgs, Final: final, ToolRounds: toolRounds}, err
		}

		// P9-ctx: always-on context discipline (outbound only) — never mutates
		// store history. Runs BEFORE Headroom so Token Saver order stays
		// Discipline (always) → Headroom (optional) → provider.
		discOpts := DefaultContextDiscipline()
		if cfg.Discipline != nil {
			discOpts = *cfg.Discipline
		}
		disciplined := ApplyContextDiscipline(msgs, discOpts)

		// Headroom: compress prompts before model call when enabled+available (VAL-SAVER-004).
		// Failures pass through original messages; never crash chat.
		modelMsgs := CompressMessagesHeadroom(ctx, cfg.TokenSavers, disciplined, cfg.HeadroomCompress, nil)

		req := provider.ChatRequest{
			Model:    cfg.Model,
			Messages: modelMsgs,
			Tools:    toolDefs,
			Stream:   cfg.Stream,
		}

		var assistant provider.Message
		var err error
		if cfg.Stream {
			assistant, err = runStreamingTurn(ctx, cfg.Client, req, out)
		} else if tr, ok := cfg.Client.(TurnRunner); ok {
			assistant, err = tr.ChatTurn(ctx, req)
			if err == nil && assistant.Content != "" {
				if _, werr := io.WriteString(out, assistant.Content); werr != nil {
					return AgentResult{Messages: msgs, Final: final, ToolRounds: toolRounds}, werr
				}
				if _, werr := io.WriteString(out, "\n"); werr != nil {
					return AgentResult{Messages: msgs, Final: final, ToolRounds: toolRounds}, werr
				}
			}
		} else {
			// Fallback: stream without printing if client has no ChatTurn.
			assistant, err = runStreamingTurn(ctx, cfg.Client, req, out)
		}
		if err != nil {
			return AgentResult{Messages: msgs, Final: final, ToolRounds: toolRounds}, err
		}

		// Ensure role is assistant.
		if assistant.Role == "" {
			assistant.Role = provider.RoleAssistant
		}
		msgs = append(msgs, assistant)
		if strings.TrimSpace(assistant.Content) != "" {
			final = assistant.Content
		}

		if len(assistant.ToolCalls) == 0 || cfg.Tools == nil {
			// Done — no more tools.
			return AgentResult{Messages: msgs, Final: final, ToolRounds: toolRounds}, nil
		}

		// Cap only when explicitly requested (positive MaxToolRounds).
		if !unlimited && toolRounds >= maxRounds {
			return AgentResult{Messages: msgs, Final: final, ToolRounds: toolRounds},
				fmt.Errorf("core: max tool rounds (%d) reached", maxRounds)
		}

		toolRounds++
		// Execute each tool call and append tool results.
		// RTK compresses tool output before it enters model context when enabled (VAL-SAVER-003).
		for _, tc := range assistant.ToolCalls {
			call := tools.Call{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
			res := cfg.Tools.Execute(ctx, call, func(ev tools.Event) {
				printToolEvent(eventOut, ev)
			})
			toolBody := CompressToolResultRTK(cfg.TokenSavers, res.Name, res.Content, cfg.RTKCompress)
			msgs = append(msgs, provider.Message{
				Role:       provider.RoleTool,
				Content:    toolBody,
				ToolCallID: res.ID,
				Name:       res.Name,
			})
		}
	}

}

// appendSaverFragmentsToExistingSystem reconciles managed fragments on the first
// system message. It removes stale fragments before adding the currently enabled
// set, so Settings toggles apply on the next turn in both directions.
func appendSaverFragmentsToExistingSystem(msgs []provider.Message, savers config.TokenSaversConfig) []provider.Message {
	out := append([]provider.Message(nil), msgs...)
	for i := range out {
		if out[i].Role != provider.RoleSystem {
			continue
		}
		body := out[i].Content
		body = strings.ReplaceAll(body, CavemanSystemFragment, "")
		body = strings.ReplaceAll(body, PonytailSystemFragment, "")
		body = strings.TrimSpace(body)
		if savers.Caveman.Enabled {
			body = strings.TrimSpace(body + "\n\n" + CavemanSystemFragment)
		}
		if savers.Ponytail.Enabled {
			body = strings.TrimSpace(body + "\n\n" + PonytailSystemFragment)
		}
		out[i].Content = body
		break
	}
	return out
}

func runStreamingTurn(ctx context.Context, client Streamer, req provider.ChatRequest, out io.Writer) (provider.Message, error) {
	ch, err := client.ChatStream(ctx, req)
	if err != nil {
		return provider.Message{}, err
	}

	var content strings.Builder
	var toolCalls []provider.ToolCall
	var streamErr error
	var wrote bool

	for {
		select {
		case <-ctx.Done():
			for range ch {
			}
			return provider.Message{}, ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				if err := ctx.Err(); err != nil {
					return provider.Message{}, err
				}
				if streamErr != nil {
					return provider.Message{}, streamErr
				}
				msg := provider.Message{
					Role:      provider.RoleAssistant,
					Content:   content.String(),
					ToolCalls: toolCalls,
				}
				if wrote {
					_, _ = io.WriteString(out, "\n")
				}
				return applyNonStreamFallback(ctx, client, req, msg, out)
			}
			switch ev.Type {
			case provider.EventDelta:
				if ev.Delta != "" {
					content.WriteString(ev.Delta)
					if _, err := io.WriteString(out, ev.Delta); err != nil {
						return provider.Message{}, err
					}
					wrote = true
				}
			case provider.EventToolCalls:
				if len(ev.ToolCalls) > 0 {
					toolCalls = ev.ToolCalls
				}
				if ev.Final != "" && content.Len() == 0 {
					content.WriteString(ev.Final)
				}
			case provider.EventDone:
				if ev.Final != "" {
					if content.Len() == 0 {
						content.WriteString(ev.Final)
						if !wrote {
							if _, err := io.WriteString(out, ev.Final); err != nil {
								return provider.Message{}, err
							}
							wrote = true
						}
					}
				}
				if len(ev.ToolCalls) > 0 {
					toolCalls = ev.ToolCalls
				}
			case provider.EventError:
				if ev.Err != nil {
					streamErr = ev.Err
				}
			}
		}
	}
}

func isEmptyStreamAssistant(m provider.Message) bool {
	return strings.TrimSpace(m.Content) == "" && len(m.ToolCalls) == 0
}

func applyNonStreamFallback(ctx context.Context, client Streamer, req provider.ChatRequest, msg provider.Message, out io.Writer) (provider.Message, error) {
	if !isEmptyStreamAssistant(msg) {
		return msg, nil
	}
	tr, ok := client.(TurnRunner)
	if !ok {
		return msg, nil
	}
	nonStream := req
	nonStream.Stream = false
	retry, rerr := tr.ChatTurn(ctx, nonStream)
	if rerr != nil {
		return provider.Message{}, fmt.Errorf("core: empty stream; non-stream fallback failed: %w", rerr)
	}
	if retry.Role == "" {
		retry.Role = provider.RoleAssistant
	}
	if isEmptyStreamAssistant(retry) {
		return retry, nil
	}
	if text := strings.TrimSpace(retry.Content); text != "" {
		_, _ = io.WriteString(out, text)
		_, _ = io.WriteString(out, "\n")
	}
	return retry, nil
}

func printToolEvent(w io.Writer, ev tools.Event) {
	if w == nil {
		return
	}
	switch ev.Type {
	case tools.EventStart:
		fmt.Fprintf(w, "[tool_start] %s %s\n", ev.Name, ev.Detail)
	case tools.EventEnd:
		// Keep event lines short for CLI readability.
		detail := ev.Detail
		body := strings.TrimSpace(ev.Content)
		if len(body) > 200 {
			body = body[:200] + "…"
		}
		fmt.Fprintf(w, "[tool_end] %s %s → %s\n", ev.Name, detail, body)
	case tools.EventError:
		body := strings.TrimSpace(ev.Content)
		if len(body) > 300 {
			body = body[:300] + "…"
		}
		fmt.Fprintf(w, "[tool_error] %s %s → %s\n", ev.Name, ev.Detail, body)
	}
}

func hasSystem(msgs []provider.Message) bool {
	for _, m := range msgs {
		if m.Role == provider.RoleSystem {
			return true
		}
	}
	return false
}

// ToolsToProvider maps tools package definitions to provider.ToolDef.
func ToolsToProvider(defs []tools.Definition) []provider.ToolDef {
	out := make([]provider.ToolDef, 0, len(defs))
	for _, d := range defs {
		td := provider.ToolDef{Type: d.Type}
		if td.Type == "" {
			td.Type = "function"
		}
		td.Function.Name = d.Function.Name
		td.Function.Description = d.Function.Description
		td.Function.Parameters = d.Function.Parameters
		out = append(out, td)
	}
	return out
}
