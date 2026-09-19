package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/git"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/store"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/workspace"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// ChatEventType classifies desktop/SSE chat stream events (no TS-side LLM).
type ChatEventType string

const (
	// ChatEventToken is a TokenDelta partial assistant token.
	ChatEventToken ChatEventType = "TokenDelta"
	// ChatEventToolStart marks a tool invocation start.
	ChatEventToolStart ChatEventType = "ToolStart"
	// ChatEventToolEnd marks a tool invocation end.
	ChatEventToolEnd ChatEventType = "ToolEnd"
	// ChatEventToolError marks a tool error (sandbox, etc.).
	ChatEventToolError ChatEventType = "ToolError"
	// ChatEventFileChanged is emitted when WriteGateway mutates a path.
	ChatEventFileChanged ChatEventType = "FileChanged"
	// ChatEventUndoStack is emitted after undo stack depth changes.
	ChatEventUndoStack ChatEventType = "UndoStackChanged"
	// ChatEventDone marks successful chat turn completion.
	ChatEventDone ChatEventType = "Done"
	// ChatEventError marks a fatal chat error.
	ChatEventError ChatEventType = "Error"
	// ChatEventCancelled marks user/server cancel of an in-flight stream (VAL-CHAT-005).
	// Explicit Stop via CancelChatRun cancels the detached run ctx.
	// Browser disconnect alone does NOT cancel detached runs.
	ChatEventCancelled ChatEventType = "Cancelled"
	// ChatEventRunStarted announces a detached run id before tokens (reload-safe).
	ChatEventRunStarted ChatEventType = "RunStarted"
)

// StoppedByUserMarker is appended / used when a stream is cancelled mid-turn (VAL-CHAT-003/004).
const StoppedByUserMarker = "_(stopped by user)_"

// ChatEvent is one SSE / binding stream unit for the desktop chat panel.
type ChatEvent struct {
	Type    ChatEventType `json:"type"`
	Delta   string        `json:"delta,omitempty"`
	Text    string        `json:"text,omitempty"`
	Name    string        `json:"name,omitempty"`
	Path    string        `json:"path,omitempty"`
	Detail  string        `json:"detail,omitempty"`
	Final   string        `json:"final,omitempty"`
	Error   string        `json:"error,omitempty"`
	CanUndo bool          `json:"can_undo,omitempty"`
	CanRedo bool          `json:"can_redo,omitempty"`
	// Profile/Model/Host surface mid-session routing metadata on Done (VAL-PROV-005).
	// Never include API keys.
	Profile string `json:"profile,omitempty"`
	Model   string `json:"model,omitempty"`
	Host    string `json:"host,omitempty"`
	// ApprovalID is set on NeedsApproval events so the UI can ResolveApproval (VAL-GIT-005+).
	ApprovalID string `json:"approval_id,omitempty"`
	// ApprovalKind is the gated op kind (commit, force_push, reset_hard, …).
	ApprovalKind string `json:"approval_kind,omitempty"`
	// ApprovalForce marks force-push dialogs.
	ApprovalForce bool `json:"approval_force,omitempty"`
	// Tokens / TokensPrompt / TokensCompletion / LatencyMS surface assistant
	// meta on Done when available (VAL-CHAT-012). Never include secrets.
	Tokens           int   `json:"tokens,omitempty"`
	TokensPrompt     int   `json:"tokens_prompt,omitempty"`
	TokensCompletion int   `json:"tokens_completion,omitempty"`
	LatencyMS        int64 `json:"latency_ms,omitempty"`
	// Thinking carries optional reasoning/thinking content (VAL-CHAT-013).
	Thinking string `json:"thinking,omitempty"`
	// RunID identifies a detached backend chat run (survives HTTP disconnect).
	RunID string `json:"run_id,omitempty"`
}

// ChatImagePart is one vision/reference image data URL from the desktop paperclip
// path (VAL-CHAT-022). Client may send up to MaxChatImages; excess is ignored.
type ChatImagePart struct {
	// Name is the original file name for logging / text reference (never a path secret).
	Name string `json:"name,omitempty"`
	// MediaType is image/png, image/jpeg, … (optional when data_url has a prefix).
	MediaType string `json:"media_type,omitempty"`
	// DataURL is a data:image/...;base64,... URL (or https: when already remote).
	DataURL string `json:"data_url"`
}

// MaxChatImages is the max vision reference images accepted per turn (VAL-CHAT-022).
const MaxChatImages = 4

// ChatRequest is the desktop/HTTP chat turn input (core.Service only; no TS LLM).
type ChatRequest struct {
	// Prompt is the user message text (may already include client-inlined PDF/text).
	Prompt string `json:"prompt"`
	// Mentions are workspace-relative file paths attached via @mention.
	Mentions []string `json:"mentions,omitempty"`
	// Images are optional vision/reference data URLs (max MaxChatImages).
	// Serialised as multimodal user content parts when the provider supports it.
	// With GenerateImage, these act as reference images (also capped at MaxChatImages).
	Images []ChatImagePart `json:"images,omitempty"`
	// Model overrides the profile default / discovery when non-empty.
	Model string `json:"model,omitempty"`
	// Profile is the provider profile id (tempai, byok, …). Empty uses default.
	Profile string `json:"profile,omitempty"`
	// NoTools disables tool calling (plain stream only).
	NoTools bool `json:"no_tools,omitempty"`
	// MaxToolRounds caps tool rounds; 0 (default) = unlimited. Positive = hard cap.
	MaxToolRounds int `json:"max_tool_rounds,omitempty"`
	// GenerateImage routes the turn through gateway image generation when the
	// active profile supports it (VAL-CHAT-025). Unsupported → clear Error event.
	GenerateImage bool `json:"generate_image,omitempty"`
	// ImageSize is an optional size hint for image gen ("1024x1024", "1792x1024", …).
	ImageSize string `json:"image_size,omitempty"`
	// ImageTemperature is optional sampling temperature for image gen (0–2).
	// Pointer so 0 can be sent deliberately.
	ImageTemperature *float64 `json:"image_temperature,omitempty"`
	// ImageReasoningEffort is optional Responses reasoning effort ("low"|"medium"|"high").
	ImageReasoningEffort string `json:"image_reasoning_effort,omitempty"`
	// ImageOnly prefers image output with minimal caption text.
	ImageOnly bool `json:"image_only,omitempty"`
	// Ephemeral means the prompt is NOT stored as a user history message.
	// Used for auto-continue continue prompts (VAL-CHAT-026).
	Ephemeral bool `json:"ephemeral,omitempty"`
	// Continue marks an auto-continue follow-up turn (client UX); with Ephemeral,
	// the continue prompt is injected into the model request only.
	Continue bool `json:"continue,omitempty"`
	// NoHistory skips loading workspace chat history into the model messages.
	// Used by slides/canvas agents that must be isolated from workspace chat
	// context (VAL-SLIDES-CTX-ISOLATION). Implies Ephemeral for storage.
	NoHistory bool `json:"no_history,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`

	// SessionID explicitly targets a chat session for history load/persist.
	// When set, overrides the default "desk:"+workspaceID scoping for THIS turn.
	// Empty → legacy workspace-scoped behavior (backward compat).
	SessionID string `json:"session_id,omitempty"`

	// PlaygroundID scopes a playground agent's own instruction history.
	// When set, a separate playground-scoped session is loaded/merged/persisted
	// (keyed "pg:<playgroundID>"). Empty → no playground history.
	PlaygroundID string `json:"playground_id,omitempty"`

	// AgentKind selects a non-default system prompt + tool policy.
	// "" | "coding" → DefaultSystemPrompt (legacy).
  // Playground kinds select document-specific prompts without workspace snippets.
	AgentKind string `json:"agent_kind,omitempty"`
}

// ChatSession holds lightweight in-memory conversation for the active hub workspace.
// Durable copy is scoped by workspace_id in sessions.db (VAL-DESK-014).
type ChatSession struct {
	Messages    []provider.Message `json:"messages"`
	Model       string             `json:"model,omitempty"`
	Profile     string             `json:"profile,omitempty"`
	WorkspaceID string             `json:"workspace_id,omitempty"`
	// MessageMetas carries optional per-message meta keyed by index into
	// Messages (assistant rows only). Persisted to sessions.db so a reloaded
	// assistant message restores its ChatMetaRow + tool/task blocks, not only
	// a bare stream-status done (misc-chat-meta-persist-history). Never
	// carries secrets — only tokens/latency/stream status/routing labels/
	// thinking/tool+task block summaries.
	MessageMetas map[int]*store.MessageMeta `json:"-"`
}

// streamWriter emits TokenDelta events for each Write from the agent loop.
type streamWriter struct {
	emit func(ChatEvent)
	mu   sync.Mutex
}

func (w *streamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Drop trailing newlines that agent.go appends for CLI readability; keep content deltas.
	s := string(p)
	if s == "\n" {
		return len(p), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.emit != nil {
		w.emit(ChatEvent{Type: ChatEventToken, Delta: s})
	}
	return len(p), nil
}

// toolEventWriter bridges tools.Event to ChatEvent (unused when tools emit via callback).
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// ChatStream runs one agent turn and streams ChatEvent values to emit.
// All LLM calls go through Go core (VAL-DESK-004/008). Cancel ctx to stop.
// Prefer ChatStreamDetached for desktop SSE so client disconnect does not cancel the turn.
func (s *Service) ChatStream(ctx context.Context, req ChatRequest, emit func(ChatEvent)) error {
	if s == nil {
		return fmt.Errorf("core: service is nil")
	}
	if emit == nil {
		emit = func(ChatEvent) {}
	}
	// Image generation toggle path (VAL-CHAT-025): gateway /images/generations
	// (or responses/image_generation) when supported; clear degrade otherwise.
	if req.GenerateImage {
		return s.chatStreamImageGen(ctx, req, emit)
	}

	// Capture tool/task events emitted during this turn so the final assistant
	// message can persist its tool/task blocks alongside tokens/latency/stream
	// status (misc-chat-meta-persist-history). Never captures secrets — only
	// tool names, statuses, and non-secret detail/result text.
	turnMeta := &turnMetaAccumulator{}
	emit = wrapEmitWithMetaCapture(emit, turnMeta)

	prompt := strings.TrimSpace(req.Prompt)
	images := normalizeChatImages(req.Images)
	if prompt == "" && len(req.Mentions) == 0 && len(images) == 0 {
		return fmt.Errorf("core: empty chat prompt")
	}

	// NoHistory implies Ephemeral: isolated turns must not be persisted either.
	if req.NoHistory {
		req.Ephemeral = true
	}

	turnWorkspaceID := strings.TrimSpace(req.WorkspaceID)
	if turnWorkspaceID == "" {
		if active := s.registry.Active(); active != nil {
			turnWorkspaceID = active.ID
		}
	}
	if turnWorkspaceID == "" {
		s.mu.Lock()
		turnWorkspaceID = s.chatWorkspaceID
		s.mu.Unlock()
	}
	root, err := s.rootFor(turnWorkspaceID)
	if err != nil {
		emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
		return err
	}

	// Enrich user text with @file context (mention picker attaches paths).
	// Image-only turns are allowed: empty prompt + vision data URLs (VAL-CHAT-022).
	// Ephemeral continue may pass prompt without mentions (VAL-CHAT-026).
	userText, err := s.buildUserMessage(root, prompt, req.Mentions)
	if err != nil {
		if len(images) > 0 && len(req.Mentions) == 0 && prompt == "" {
			userText = "[image attachment(s) only]"
		} else if req.Ephemeral && prompt != "" {
			// Ephemeral continue: no file context required — use prompt as-is.
			userText = prompt
		} else {
			emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
			return err
		}
	}

	// Mid-session switch: empty request profile/model falls back to session active
	// selection so the next turn uses the picker without app restart (VAL-PROV-005).
	s.resolveChatProfileModel(&req)

	router, err := s.providerRouter(req.Profile)
	if err != nil {
		emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
		return err
	}
	// GetProfile supports openai_compatible + anthropic (VAL-PROV-007).
	// Missing Anthropic key → BlockedError "key required", never temp-ai proxy (VAL-PROV-008).
	client, err := router.GetProfile(req.Profile)
	if err != nil {
		emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
		return err
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = client.DefaultModel()
	}
	resolved, err := client.ResolveModel(ctx, model)
	if err != nil {
		// Surface blocked/key-required clearly for Anthropic without key.
		// Gateway-down (tempai unreachable / 401 / 5xx) → brand-correct
		// GatewayError + Blocked event so the agent loop offers a retry/blocked
		// path instead of hanging or faking success (VAL-CROSS-009).
		emitGatewayBlockedEvent(emit, client, err)
		emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
		return err
	}
	// Capture routing host for Done metadata / isolation verification.
	requestHost := client.RequestHost()

	var reg *tools.Registry
	var gw *writegate.Gateway
	if !req.NoTools {
		gw, err = s.openGateway(root)
		if err != nil {
			emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
			return err
		}
		gw.SetOnChange(func(ev writegate.ChangeEvent) {
			label := ev.Rel
			if label == "" {
				label = ev.Path
			}
			emit(ChatEvent{
				Type:   ChatEventFileChanged,
				Path:   label,
				Detail: ev.Op,
			})
			// Keep undo toolbar in sync with WriteGateway stack.
			emit(ChatEvent{
				Type:    ChatEventUndoStack,
				CanUndo: gw.UndoDepth() > 0,
				CanRedo: gw.RedoDepth() > 0,
				Detail:  fmt.Sprintf("undo=%d redo=%d", gw.UndoDepth(), gw.RedoDepth()),
			})
		})
		reg = tools.NewRegistry(gw)
		// Diagram tools (VAL-DIAG-002/003/004): create/update/export via WriteGateway.
		tools.EnsureDiagramFromGateway(reg)
		// Builtin Context7 library-docs tool (P9-ctx): bounded fetch, soft-fail.
		tools.EnsureContext7Backend(reg)
		// Plan Mode: share Service controller so enter/exit UI gates the tool loop
		// (VAL-PLAN-001/006/007). Write tools denied while active.
		reg.SetPlanMode(s.PlanModeController())
		reg.SetAutonomyLevel(s.AutonomyLevel())
		// Session todos + propose_plan (VAL-PLAN-003/004/005).
		// Stream PlanProposed / TodoUpdated on the same chat SSE channel.
		pt := s.ensurePlanTodos()
		pt.setEmit(emit)
		reg.SetTodoBackend(s)
		// Mission/Goal tools + MissionStatus SSE (VAL-MISSION-001..006).
		ms := s.ensureMissions()
		ms.setEmit(emit)
		reg.SetMissionBackend(s)
		// Orchestrator task() — parallel explore, depth 1, synthesis (VAL-ORCH-001+).
		s.wireTaskBackend(reg, root, emit)
		// Progressive skill loader (VAL-ORCH-004/005): list metadata only; load changes toolset/prompt.
		home, _ := config.Home()
		skillLoader := skills.NewDefaultLoader(home, root, nil)
		reg.SetSkillBackend(tools.NewSkillBackend(skillLoader))
		// Browser multi-engine tools + BrowserStatus SSE (VAL-BRW-006..010).
		// Panel is subscriber-only; cookies never enter tool results for the model.
		s.WireBrowserBackend(reg, emit)
		// Desktop chat shares the same in-process PTY manager as the
		// integrated terminal panel so the agent can list/read/tail
		// sessions the user started from the UI (VAL-DESK-011).
		// Do not use detached CLI workers here — panel sessions are live
		// under Service.ensurePTY(), not under ~/.inferenesia/pty workers.
		reg.SetTerminalBackend(tools.NewManagerTerminal(s.ensurePTY(), ""))
		// Agent git tools: hooksPath isolation + NeedsApproval gates (VAL-GIT-005..009).
		if gs, gerr := git.New(git.Options{Root: root, AgentMode: true}); gerr == nil {
			ap := s.ensureApprover()
			ap.setEmit(emit)
			reg.SetGitBackend(gs, ap)
		}
		// MCP host: persistent on Service — reused across chat streams (VAL-MCP-001..007).
		// Register-phase errors are isolated; do not fail the chat turn.
		// ToolStart/ToolEnd/ToolError for MCP calls use the same EventOut path as built-ins (VAL-MCP-008).
		mcpHost, mcpErr := s.WireMCPHostIntoRegistry(reg, root)
		if mcpHost != nil {
			// VAL-CROSS-012: wire the skill MCP registrar so a skill with
			// mcp-servers frontmatter can register/unregister namespaced
			// tools on the same host the agent loop already uses. Unloading
			// the skill unregisters them so subsequent task() calls cannot
			// invoke them (unknown-tool error).
			reg.SetSkillMCPRegistrar(tools.NewSkillMCPHostRegistrar(mcpHost))
		} else if mcpErr != nil {
			emit(ChatEvent{Type: ChatEventToolError, Name: "mcp", Detail: mcpErr.Error(), Error: mcpErr.Error()})
		}
	}

	var history []provider.Message
	if !req.NoHistory {
		history, _, _ = s.loadContextHistory(req, turnWorkspaceID)
	}

	msgs := append([]provider.Message(nil), history...)
	sysForRun := SystemPromptForKind(req.AgentKind)
	isCodingAgent := req.AgentKind == "" || req.AgentKind == "coding"
	if isCodingAgent && !req.NoHistory {
		if modeSnippet := strings.TrimSpace(s.interactionModeSystemSnippet(turnWorkspaceID)); modeSnippet != "" {
			sysForRun = strings.TrimSpace(sysForRun) + "\n\n" + modeSnippet
		}
		if gitCtx := strings.TrimSpace(s.gitWorkflowSystemSnippet("")); gitCtx != "" {
			sysForRun = strings.TrimSpace(sysForRun) + "\n\n" + gitCtx
		}
		if mbSnippet := strings.TrimSpace(s.multiBrainSystemSnippet(root, prompt)); mbSnippet != "" {
			sysForRun = strings.TrimSpace(sysForRun) + "\n\n" + mbSnippet
		}
		if reg != nil {
			if sb := reg.SkillBackend(); sb != nil {
				if overlay := strings.TrimSpace(sb.SystemPromptOverlay()); overlay != "" {
					sysForRun = strings.TrimSpace(sysForRun) + "\n\n" + overlay
				}
			}
		}
		sysForRun = strings.TrimSpace(sysForRun) + "\n\n" + WebPreviewSystemFragment
		if s.LiveBlocksConfig().Enabled {
			sysForRun = strings.TrimSpace(sysForRun) + "\n\n" + LiveBlocksSystemFragment
		}
	}
	if os.Getenv("INFERENESIA_DEBUG_CHAT") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG-CHAT] agentKind=%q noHistory=%v sessionID=%q playgroundID=%q workspaceID=%q turnWID=%q sysPromptLen=%d historyLen=%d\n",
			req.AgentKind, req.NoHistory, req.SessionID, req.PlaygroundID, req.WorkspaceID, turnWorkspaceID, len(sysForRun), len(history))
	}
	for _, m := range msgs {
		if m.Role == provider.RoleSystem {
			sysForRun = ""
			break
		}
	}
	// Ephemeral continue: inject prompt for the model only; do NOT append a durable
	// user row. After Run, we strip any echo of this continue prompt from the
	// persisted transcript (VAL-CHAT-026).
	userMsg := provider.Message{Role: provider.RoleUser, Content: userText}
	if len(images) > 0 && !req.Ephemeral {
		// Multimodal user turn: text + image_url data URLs (VAL-CHAT-022).
		// Content remains the full text so durable history and text-only clients work.
		parts := []provider.ContentPart{{Type: "text", Text: userText}}
		for _, img := range images {
			parts = append(parts, provider.ContentPart{
				Type:      "image_url",
				ImageURL:  img.DataURL,
				MediaType: img.MediaType,
			})
		}
		userMsg.Parts = parts
	}
	msgs = append(msgs, userMsg)
	// Snapshot message count before this turn so ephemeral continue can drop the
	// injected user prompt from durable storage after the run.
	historyLenBeforeUser := len(history)

	out := &streamWriter{emit: emit}
	// Tools print via EventOut would dump CLI-style lines; suppress and use
	// tools.Execute callback via wrapping would need core.Run change.
	// core.Run already prints tool events to EventOut as text — map via tee if needed.
	// For desktop we prefer structured TokenDelta; tool lines go as ToolStart/End via a parser writer.
	eventOut := &toolLineWriter{emit: emit}

	// Optional Token Savers from ~/.inferenesia config (defaults off). Pipeline
	// pass-through when off/unavailable; never crash chat (VAL-SAVER-003..008).
	tokenSavers := s.TokenSaversConfig()

	// Wall-clock latency for meta row (VAL-CHAT-012). Tokens estimated from text when
	// the provider does not stream usage (gateway may omit usage on SSE).
	started := time.Now()
	// Canvas agents ship large HTML/JSON in the user prompt — use wider
	// discipline caps so MaxUserRunes=4000 does not truncate deck + intent.
	var discPtr *DisciplineOptions
	if !isCodingAgent {
		d := DisciplineForAgentKind(req.AgentKind)
		discPtr = &d
	}
	result, runErr := Run(ctx, AgentConfig{
		Client:        client,
		Tools:         reg,
		Model:         resolved,
		SystemPrompt:  sysForRun,
		Messages:      msgs,
		MaxToolRounds: req.MaxToolRounds,
		Stream:        true,
		Out:           out,
		EventOut:      eventOut,
		TokenSavers:   tokenSavers,
		Discipline:    discPtr,
	})
	latencyMS := time.Since(started).Milliseconds()

	// Persist gateway stack even on partial runs.
	if gw != nil {
		_ = s.saveGateway(root, gw)
		emit(ChatEvent{
			Type:    ChatEventUndoStack,
			CanUndo: gw.UndoDepth() > 0,
			CanRedo: gw.RedoDepth() > 0,
			Detail:  fmt.Sprintf("undo=%d redo=%d", gw.UndoDepth(), gw.RedoDepth()),
		})
	}

	if runErr != nil {
		// Still keep partial transcript for UI continuity + durable save.
		// User cancel = context.Canceled only (browser Abort / hub cancel).
		// DeadlineExceeded is a timeout/provider hang — surface as Error, not
		// "_(stopped by user)_" (users often never pressed Stop).
		userCancelled := errors.Is(runErr, context.Canceled) ||
			(ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled))
		msgsOut := result.Messages
		if userCancelled {
			msgsOut = markMessagesStoppedByUser(result.Messages)
		}
		if req.Ephemeral {
			if req.Continue {
				msgsOut = dropEphemeralUserFromResult(msgsOut, historyLenBeforeUser, userText)
			} else {
				msgsOut = dropFullyEphemeralTurn(msgsOut, historyLenBeforeUser)
			}
		}
		// Profile/model for sticky session + Done/Cancelled meta (VAL-CHATBUG-001).
		// Prefer explicit request profile (picker) over client.Name() which may be a label.
		routeProfile := firstNonEmpty(strings.TrimSpace(req.Profile), client.Name())
		if len(msgsOut) > 0 {
			// Build per-message meta for the last assistant row so a reloaded
			// assistant message restores its ChatMetaRow + tool/task blocks,
			// not only a bare stream-status (misc-chat-meta-persist-history).
			tools, tasks := turnMeta.snapshot()
			streamStatus := "error"
			if userCancelled {
				streamStatus = "stopped"
			}
			finalText := lastAssistantContent(msgsOut)
			if finalText == "" && userCancelled {
				finalText = StoppedByUserMarker
			}
			tokPrompt, tokComp, tokTotal := estimateTokenUsage(msgsOut, finalText)
			lastMeta := buildLastAssistantMeta(
				msgsOut, tokTotal, tokPrompt, tokComp, latencyMS,
				streamStatus, resolved, routeProfile, requestHost, "",
				tools, tasks,
			)
			snap := ChatSession{
				Messages:     append([]provider.Message(nil), msgsOut...),
				Model:        resolved,
				Profile:      routeProfile,
				MessageMetas: setLastAssistantMeta(nil, msgsOut, lastMeta),
			}
			s.commitTurn(req, turnWorkspaceID, snap, lastMeta, false)
		}
		if userCancelled {
			// Clean cancel: do not surface as a fatal Error bubble (VAL-CHAT-003/005).
			// Message is persisted above so Cancelled never leaves busy lock / empty history
			// without a stop marker (VAL-CHATBUG-003).
			finalText := lastAssistantContent(msgsOut)
			if finalText == "" {
				finalText = StoppedByUserMarker
			}
			tokPrompt, tokComp, tokTotal := estimateTokenUsage(msgsOut, finalText)
			emit(ChatEvent{
				Type:             ChatEventCancelled,
				Text:             finalText,
				Final:            finalText,
				Error:            "cancelled",
				Profile:          routeProfile,
				Model:            resolved,
				Host:             requestHost,
				Tokens:           tokTotal,
				TokensPrompt:     tokPrompt,
				TokensCompletion: tokComp,
				LatencyMS:        latencyMS,
			})
			return runErr
		}
		// Failed chat turns always emit Error for UI bubble (VAL-CHAT-028).
		// Gateway-down (tempai unreachable / 401 / 5xx) → brand-correct
		// GatewayError + Blocked event so the agent loop offers a retry/blocked
		// path instead of hanging or faking success (VAL-CROSS-009).
		emitGatewayBlockedEvent(emit, client, runErr)
		emit(ChatEvent{Type: ChatEventError, Error: runErr.Error()})
		return runErr
	}

	msgsOut := result.Messages
	if req.Ephemeral {
		if req.Continue {
			msgsOut = dropEphemeralUserFromResult(msgsOut, historyLenBeforeUser, userText)
		} else {
			msgsOut = dropFullyEphemeralTurn(msgsOut, historyLenBeforeUser)
		}
	}

	// Prefer request profile id (sticky picker) for next-turn session binding.
	routeProfile := firstNonEmpty(strings.TrimSpace(req.Profile), client.Name())
	tokPrompt, tokComp, tokTotal := estimateTokenUsage(result.Messages, result.Final)
	// Build per-message meta for the last assistant row so a reloaded assistant
	// message restores its ChatMetaRow + tool/task blocks, not only a bare
	// stream-status done (misc-chat-meta-persist-history). Thinking is left
	// empty here — the UI splits <thinking> tags from content client-side, and
	// we persist the raw assistant content (which keeps the tags) so the
	// collapsible Thinking block rehydrates from content on reload too.
	tools, tasks := turnMeta.snapshot()
	lastMeta := buildLastAssistantMeta(
		msgsOut, tokTotal, tokPrompt, tokComp, latencyMS,
		"done", resolved, routeProfile, requestHost, "",
		tools, tasks,
	)
	snap := ChatSession{
		Messages:     append([]provider.Message(nil), msgsOut...),
		Model:        resolved,
		Profile:      routeProfile,
		MessageMetas: setLastAssistantMeta(nil, msgsOut, lastMeta),
	}
	s.commitTurn(req, turnWorkspaceID, snap, lastMeta, true)

	emit(ChatEvent{
		Type:             ChatEventDone,
		Final:            result.Final,
		Text:             result.Final,
		Profile:          routeProfile,
		Model:            resolved,
		Host:             requestHost,
		Tokens:           tokTotal,
		TokensPrompt:     tokPrompt,
		TokensCompletion: tokComp,
		LatencyMS:        latencyMS,
	})
	return nil
}

// commitTurnChat persists the finished turn under turnWorkspaceID always.
// In-memory s.chat is updated only when still bound to that workspace so a
// mid-stream switch cannot clobber the newly loaded thread.
func (s *Service) commitTurnChat(turnWorkspaceID string, snap ChatSession, lastMeta *store.MessageMeta, markDone bool) {
	if s == nil {
		return
	}
	wid := strings.TrimSpace(turnWorkspaceID)
	if wid == "" {
		s.mu.Lock()
		wid = s.chatWorkspaceID
		if wid == "" && s.registry != nil {
			wid = s.registry.ActiveID()
		}
		s.mu.Unlock()
	}
	if wid != "" {
		_ = s.persistChat(wid, snap)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chatWorkspaceID != "" && wid != "" && s.chatWorkspaceID != wid {
		return
	}
	if wid != "" && s.chatWorkspaceID == "" {
		s.chatWorkspaceID = wid
	}
	s.chat.Messages = append([]provider.Message(nil), snap.Messages...)
	s.chat.Model = snap.Model
	s.chat.Profile = snap.Profile
	s.chat.MessageMetas = setLastAssistantMeta(s.chat.MessageMetas, s.chat.Messages, lastMeta)
	if markDone {
		s.statusMsg = "Chat turn complete"
	}
}

// commitTurn persists the finished turn to the correct scope(s) based on the
// 3-mode context binding.
//
//   - Playground turn (PlaygroundID set): persistPlayground writes ONLY to
//     "pg:<playgroundID>". s.chat in-memory workspace chat and workspace
//     durable chat are NOT touched so playground turns never clobber the
//     workspace thread. markDone/lastMeta are irrelevant for playground scope.
//   - Legacy chat (no PlaygroundID): delegates to commitTurnChat unchanged
//     (workspace-scoped persist + in-memory binding + statusMsg).
func (s *Service) commitTurn(req ChatRequest, turnWorkspaceID string, snap ChatSession, lastMeta *store.MessageMeta, markDone bool) {
	if s == nil {
		return
	}
	if strings.TrimSpace(req.PlaygroundID) != "" {
		snap.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
		_ = s.persistPlayground(req.PlaygroundID, snap)
		return
	}
	s.commitTurnChat(turnWorkspaceID, snap, lastMeta, markDone)
}

// estimateTokenUsage provides approximate token counts for meta display when the
// provider does not stream usage. Heuristic: ~4 chars per token (VAL-CHAT-012).
// Never claims billing accuracy — UI labels as tokens only.
func estimateTokenUsage(msgs []provider.Message, final string) (prompt, completion, total int) {
	var promptChars, completionChars int
	for _, m := range msgs {
		n := len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
		switch m.Role {
		case provider.RoleAssistant:
			completionChars += n
		case provider.RoleUser, provider.RoleSystem, provider.RoleTool:
			promptChars += n
		default:
			promptChars += n
		}
	}
	if completionChars == 0 && strings.TrimSpace(final) != "" {
		completionChars = len(final)
	}
	// Floor at 1 when non-empty so meta shows when there was content.
	if promptChars > 0 {
		prompt = (promptChars + 3) / 4
		if prompt < 1 {
			prompt = 1
		}
	}
	if completionChars > 0 {
		completion = (completionChars + 3) / 4
		if completion < 1 {
			completion = 1
		}
	}
	total = prompt + completion
	return prompt, completion, total
}

func dropFullyEphemeralTurn(msgs []provider.Message, historyLen int) []provider.Message {
	if historyLen < 0 {
		historyLen = 0
	}
	if historyLen > len(msgs) {
		historyLen = len(msgs)
	}
	if historyLen == 0 {
		return nil
	}
	return append([]provider.Message(nil), msgs[:historyLen]...)
}

// dropEphemeralUserFromResult removes the injected continue user prompt from the
// Run result so durable history never stores auto-continue prompts as user rows
// (VAL-CHAT-026). Keeps new assistant / tool messages from this turn.
func dropEphemeralUserFromResult(msgs []provider.Message, historyLen int, ephemeralContent string) []provider.Message {
	if len(msgs) == 0 {
		return msgs
	}
	// Prefer: keep history prefix + only non-matching-user messages after it.
	if historyLen >= 0 && historyLen <= len(msgs) {
		out := make([]provider.Message, 0, len(msgs))
		out = append(out, msgs[:historyLen]...)
		for _, m := range msgs[historyLen:] {
			if m.Role == provider.RoleUser && strings.TrimSpace(m.Content) == strings.TrimSpace(ephemeralContent) {
				continue
			}
			// Also drop exact ephemeral prompt text if slightly re-wrapped.
			if m.Role == provider.RoleUser && ephemeralContent != "" && strings.Contains(m.Content, ephemeralContent) {
				continue
			}
			out = append(out, m)
		}
		return out
	}
	// Fallback: strip trailing user messages that equal the ephemeral content.
	out := append([]provider.Message(nil), msgs...)
	for i := 0; i < len(out); i++ {
		if out[i].Role == provider.RoleUser && strings.TrimSpace(out[i].Content) == strings.TrimSpace(ephemeralContent) {
			out = append(out[:i], out[i+1:]...)
			i--
		}
	}
	return out
}

// markMessagesStoppedByUser appends StoppedByUserMarker to the last assistant
// message when the user cancels mid-stream (does not invent an assistant if none).
func markMessagesStoppedByUser(msgs []provider.Message) []provider.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := append([]provider.Message(nil), msgs...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Role != provider.RoleAssistant {
			continue
		}
		c := strings.TrimSpace(out[i].Content)
		if strings.Contains(c, StoppedByUserMarker) {
			return out
		}
		if c == "" {
			out[i].Content = StoppedByUserMarker
		} else {
			out[i].Content = strings.TrimRight(out[i].Content, " \t\n") + "\n\n" + StoppedByUserMarker
		}
		return out
	}
	// No assistant yet — append a short cancelled assistant turn for durability.
	return append(out, provider.Message{Role: provider.RoleAssistant, Content: StoppedByUserMarker})
}

func lastAssistantContent(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant {
			return msgs[i].Content
		}
	}
	return ""
}

// buildUserMessage concatenates the user prompt with @file mention bodies.
func (s *Service) buildUserMessage(root, prompt string, mentions []string) (string, error) {
	var b strings.Builder
	if prompt != "" {
		b.WriteString(prompt)
	}
	seen := map[string]bool{}
	for _, m := range mentions {
		rel := strings.TrimSpace(m)
		rel = strings.TrimPrefix(rel, "@")
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		content, err := s.ReadFile("", rel)
		if err != nil {
			// Attach path reference even when unreadable so the model knows the intent.
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(fmt.Sprintf("[attached file @%s: unreadable: %v]", rel, err))
			continue
		}
		// Cap each attachment to keep the context bounded.
		const maxAttach = 12_000
		if len(content) > maxAttach {
			content = content[:maxAttach] + "\n…(truncated)"
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("### Attached file @%s\n```\n%s\n```", rel, content))
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("core: empty chat prompt")
	}
	return b.String(), nil
}

// normalizeChatImages keeps valid image data URLs, capped at MaxChatImages (VAL-CHAT-022).
func normalizeChatImages(in []ChatImagePart) []ChatImagePart {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChatImagePart, 0, MaxChatImages)
	for _, img := range in {
		url := strings.TrimSpace(img.DataURL)
		if url == "" {
			continue
		}
		// Accept data:image/… or https: remote refs only (no file://).
		lower := strings.ToLower(url)
		if !strings.HasPrefix(lower, "data:image/") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
			continue
		}
		mt := strings.TrimSpace(img.MediaType)
		if mt == "" && strings.HasPrefix(lower, "data:") {
			// data:image/png;base64,...
			if semi := strings.Index(url, ";"); semi > 5 {
				mt = url[5:semi]
			} else if comma := strings.Index(url, ","); comma > 5 {
				mt = url[5:comma]
			}
		}
		out = append(out, ChatImagePart{
			Name:      strings.TrimSpace(img.Name),
			MediaType: mt,
			DataURL:   url,
		})
		if len(out) >= MaxChatImages {
			break
		}
	}
	return out
}

// providerRouter builds ModelRouter from env + config under this service home.
func (s *Service) providerRouter(preferred string) (*provider.Router, error) {
	s.mu.Lock()
	home := s.configHome
	s.mu.Unlock()
	file, err := config.LoadFromDir(home)
	if err != nil {
		// Missing config is fine; Bootstrap still uses env.
		file = config.FileConfig{}
	}
	return provider.Bootstrap(provider.BootstrapOptions{
		File:        file,
		PreferredID: strings.TrimSpace(preferred),
	}), nil
}

func (s *Service) activeRoot() (string, error) {
	if s == nil || s.registry == nil {
		return "", fmt.Errorf("core: no active workspace — open a workspace first")
	}
	active := s.registry.Active()
	if active == nil {
		// Auto-bind a session scratch root so chat/agent can run without a project folder.
		w, err := s.registry.EnsureScratchSession()
		if err != nil {
			return "", fmt.Errorf("core: no active workspace — open a workspace first (%w)", err)
		}
		return w.RootPath, nil
	}
	return active.RootPath, nil
}

// interactionModeSystemSnippet tells the model whether this turn is a Session
// (chat-only scratch) or a project Workspace folder — so replies use the right words.
func (s *Service) interactionModeSystemSnippet(workspaceID string) string {
	if s == nil || s.registry == nil {
		return ""
	}
	wid := strings.TrimSpace(workspaceID)
	var w *workspace.Workspace
	if wid != "" {
		got, err := s.registry.Get(wid)
		if err == nil {
			w = got
		}
	}
	if w == nil {
		w = s.registry.Active()
	}
	if w == nil {
		return `[Interaction mode]
No session or project workspace is bound yet. Prefer neutral language ("this conversation"). Do not invent a project folder name.`
	}
	if workspace.IsSession(*w) {
		return fmt.Sprintf(`[Interaction mode — SESSION]
You are in a chat Session named %q (not a project Workspace folder).
- Speak of this as a Session / conversation. Do NOT call it "the workspace" or "project workspace".
- Files you write land in a private session scratch directory (ephemeral sandbox), not the user's real project unless they open a Workspace.
- Prefer self-contained HTML in chat (fenced html) for landing pages / mockups; optional write_file in this session scratch only.
- When the session is empty, say the session is empty — not "workspace is empty".`, w.Name)
	}
	return fmt.Sprintf(`[Interaction mode — WORKSPACE]
You are in a project Workspace named %q at %s.
- Speak of this as a Workspace / project. Tools read/write under that project root.
- Prefer real project files and normal coding workflow for multi-file work.`, w.Name, w.RootPath)
}

func (s *Service) openGateway(root string) (*writegate.Gateway, error) {
	g, err := writegate.New(root)
	if err != nil {
		return nil, err
	}
	path := writegate.PersistPath(s.ConfigHome(), root)
	if err := g.LoadStack(path); err != nil {
		return nil, fmt.Errorf("load undo stack: %w", err)
	}
	return g, nil
}

func (s *Service) saveGateway(root string, g *writegate.Gateway) error {
	if g == nil {
		return nil
	}
	path := writegate.PersistPath(s.ConfigHome(), root)
	return g.SaveStack(path)
}

// toolLineWriter maps CLI-style tool event lines from core.Run EventOut into ChatEvents.
type toolLineWriter struct {
	emit func(ChatEvent)
}

func (w *toolLineWriter) Write(p []byte) (int, error) {
	if w == nil || w.emit == nil || len(p) == 0 {
		return len(p), nil
	}
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	// Examples from printToolEvent: [tool_start] write_file … / [tool_end] … / [tool_error] …
	// Prefer structured Name + Detail so desktop can render distinct tool blocks (VAL-CHAT-014).
	switch {
	case strings.HasPrefix(line, "[tool_start]"):
		name, detail := splitToolLine(strings.TrimSpace(strings.TrimPrefix(line, "[tool_start]")))
		w.emit(ChatEvent{Type: ChatEventToolStart, Name: name, Detail: detail})
	case strings.HasPrefix(line, "[tool_end]"):
		name, detail := splitToolLine(strings.TrimSpace(strings.TrimPrefix(line, "[tool_end]")))
		w.emit(ChatEvent{Type: ChatEventToolEnd, Name: name, Detail: detail})
	case strings.HasPrefix(line, "[tool_error]"):
		name, detail := splitToolLine(strings.TrimSpace(strings.TrimPrefix(line, "[tool_error]")))
		w.emit(ChatEvent{Type: ChatEventToolError, Name: name, Detail: detail, Error: line})
	case strings.HasPrefix(line, "[file_changed]"):
		// Also emitted structured via WriteGateway OnChange; keep text for CLI parity.
		w.emit(ChatEvent{Type: ChatEventFileChanged, Detail: strings.TrimSpace(strings.TrimPrefix(line, "[file_changed]"))})
	default:
		// Ignore other diagnostic lines.
		_ = io.Discard
	}
	return len(p), nil
}

// splitToolLine pulls the first token as tool name and the rest as detail.
func splitToolLine(rest string) (name, detail string) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", ""
	}
	sp := strings.IndexByte(rest, ' ')
	if sp < 0 {
		return rest, ""
	}
	return rest[:sp], strings.TrimSpace(rest[sp+1:])
}

// ToolLineWriterForTest exposes toolLineWriter for package_test (VAL-CHAT-014).
// Production callers use ChatStream EventOut mapping only.
type ToolLineWriterForTest struct {
	Emit func(ChatEvent)
}

// Write implements io.Writer for test harnesses.
func (w *ToolLineWriterForTest) Write(p []byte) (int, error) {
	inner := &toolLineWriter{emit: w.Emit}
	return inner.Write(p)
}

// EstimateTokenUsageForTest exposes estimateTokenUsage for unit tests (VAL-CHAT-012).
func EstimateTokenUsageForTest(msgs []provider.Message, final string) (prompt, completion, total int) {
	return estimateTokenUsage(msgs, final)
}

// GetChatSession returns a shallow copy of the in-memory chat transcript.
// Includes workspace_id when the active thread is bound (VAL-DESK-014).
func (s *Service) GetChatSession() ChatSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ChatSession{
		Model:       s.chat.Model,
		Profile:     s.chat.Profile,
		WorkspaceID: s.chatWorkspaceID,
	}
	if len(s.chat.Messages) > 0 {
		out.Messages = append([]provider.Message(nil), s.chat.Messages...)
	}
	return out
}

// emitGatewayBlockedEvent wraps err as a brand-correct *provider.GatewayError
// when the active client is the temp-ai gateway profile and emits a Blocked
// ChatEvent so the agent loop / desktop UI offers a retry/blocked path instead
// of hanging or faking success (VAL-CROSS-009).
//
// Non-gateway profiles (BYOK / Anthropic / 9Router) emit no Blocked event here
// so their existing provider-specific error paths stay intact. Missing-key /
// blocked-key errors also emit no Blocked event (the gateway is not "down",
// just unconfigured — VAL-CLI-008 / VAL-PROV-008). The Blocked event text is
// brand-correct ("gateway unavailable") and never contains the API key value
// (secrethyg still redacts defensively in the CLI layer).
//
// Safe to call with a nil client or nil err (no-op).
func emitGatewayBlockedEvent(emit func(ChatEvent), client provider.Profile, err error) {
	if emit == nil || err == nil || client == nil {
		return
	}
	// Only the mission-required temp-ai gateway profile surfaces a GatewayError.
	if !provider.IsGatewayProfile(client.Name()) {
		return
	}
	host := client.RequestHost()
	wrapped := provider.WrapGatewayErrorIfProfile(client.Name(), host, err)
	// Only emit a Blocked event when the wrapped error is actually a
	// *GatewayError (transport / 401 / 5xx). Missing-key errors return the
	// original err unchanged and must not emit a Blocked event.
	var gwErr *provider.GatewayError
	if !errors.As(wrapped, &gwErr) || gwErr == nil {
		return
	}
	emit(ChatEvent{
		Type:   ChatEventBlocked,
		Name:   "gateway",
		Text:   gwErr.Error(),
		Error:  gwErr.Error(),
		Detail: "gateway unavailable — retry or switch profile",
		Host:   host,
	})
}

// turnMetaAccumulator collects ToolStart/ToolEnd/ToolError and
// TaskSpawned/TaskDone events emitted during a single ChatStream turn so the
// final assistant message can persist its tool/task blocks alongside tokens /
// latency / stream status (misc-chat-meta-persist-history). Never stores
// secrets — only tool names, statuses, and non-secret detail/result text.
//
// The accumulator upserts tool blocks by name (running → done/error) and task
// blocks by task_id, mirroring the desktop UI's chatMetaBlocks upsert logic so
// the persisted shape matches what the user saw live.
type turnMetaAccumulator struct {
	mu    sync.Mutex
	tools []store.ToolBlockMeta
	tasks []store.TaskBlockMeta
}

// observe records a chat event into the accumulator (no-op for non-tool/task events).
func (a *turnMetaAccumulator) observe(ev ChatEvent) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	switch ev.Type {
	case ChatEventToolStart, ChatEventToolEnd, ChatEventToolError:
		a.upsertTool(ev)
	case ChatEventTaskSpawned, ChatEventTaskDone:
		a.upsertTask(ev)
	}
}

func (a *turnMetaAccumulator) upsertTool(ev ChatEvent) {
	name := strings.TrimSpace(ev.Name)
	detail := strings.TrimSpace(ev.Detail)
	// toolLineWriter packs tool_end as "name detail → result"; peel the result
	// off the detail so the persisted card matches the live UI block.
	result := ""
	if ev.Type == ChatEventToolEnd || ev.Type == ChatEventToolError {
		if idx := strings.Index(detail, "→"); idx >= 0 {
			result = strings.TrimSpace(detail[idx+len("→"):])
			detail = strings.TrimSpace(detail[:idx])
			// detail may still start with the tool name when Name was empty.
			if name == "" && strings.HasPrefix(detail+" ", name+" ") {
				detail = strings.TrimSpace(strings.TrimPrefix(detail, name))
			}
		}
	}
	if result == "" && ev.Type == ChatEventToolError {
		result = strings.TrimSpace(ev.Error)
	}
	status := "running"
	switch ev.Type {
	case ChatEventToolEnd:
		status = "done"
	case ChatEventToolError:
		status = "error"
	}
	// Match an existing running block with the same name first (open → close).
	for i, t := range a.tools {
		if t.Status == "running" && t.Name == name {
			a.tools[i].Status = status
			if detail != "" {
				a.tools[i].Detail = detail
			}
			if result != "" {
				a.tools[i].Result = result
			}
			return
		}
	}
	id := ev.ApprovalID
	if id == "" {
		id = fmt.Sprintf("tool-%s-%d", name, len(a.tools))
	}
	a.tools = append(a.tools, store.ToolBlockMeta{
		ID:     id,
		Name:    name,
		Status:  status,
		Detail:  detail,
		Result:  result,
	})
}

func (a *turnMetaAccumulator) upsertTask(ev ChatEvent) {
	// Detail shape from orchestrator_desk: "task_id|category|status|label|extra".
	raw := strings.TrimSpace(ev.Detail)
	parts := strings.SplitN(raw, "|", 5)
	id := ""
	category := ""
	status := ""
	label := ""
	extra := ""
	if len(parts) > 0 {
		id = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		category = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		status = strings.TrimSpace(parts[2])
	}
	if len(parts) > 3 {
		label = strings.TrimSpace(parts[3])
	}
	if len(parts) > 4 {
		extra = strings.TrimSpace(parts[4])
	}
	if id == "" {
		id = strings.TrimSpace(ev.Name)
	}
	if id == "" {
		id = fmt.Sprintf("task-%d", len(a.tasks))
	}
	if status == "" {
		status = "running"
		if ev.Type == ChatEventTaskDone {
			status = "completed"
		}
	}
	for i, t := range a.tasks {
		if t.ID == id {
			a.tasks[i].Status = status
			if category != "" {
				a.tasks[i].Category = category
			}
			if label != "" {
				a.tasks[i].Label = label
			}
			if extra != "" {
				a.tasks[i].Detail = extra
			}
			return
		}
	}
	a.tasks = append(a.tasks, store.TaskBlockMeta{
		ID:       id,
		Category: category,
		Status:   status,
		Label:    label,
		Detail:   extra,
	})
}

// snapshot returns copies of the accumulated tool/task blocks (nil-safe).
func (a *turnMetaAccumulator) snapshot() ([]store.ToolBlockMeta, []store.TaskBlockMeta) {
	if a == nil {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var tools []store.ToolBlockMeta
	if len(a.tools) > 0 {
		tools = append([]store.ToolBlockMeta(nil), a.tools...)
	}
	var tasks []store.TaskBlockMeta
	if len(a.tasks) > 0 {
		tasks = append([]store.TaskBlockMeta(nil), a.tasks...)
	}
	return tools, tasks
}

// buildLastAssistantMeta constructs the persisted meta for the last assistant
// message in msgsOut. Returns nil when there is no assistant row to attach to.
// Never includes secrets — only tokens/latency/stream status/routing labels/
// thinking/tool+task blocks. thinking is the caller-resolved reasoning content
// (may be empty).
func buildLastAssistantMeta(
	msgsOut []provider.Message,
	tokens, tokensPrompt, tokensCompletion int,
	latencyMS int64,
	streamStatus, model, profile, host, thinking string,
	tools []store.ToolBlockMeta,
	tasks []store.TaskBlockMeta,
) *store.MessageMeta {
	idx := lastAssistantIndex(msgsOut)
	if idx < 0 {
		return nil
	}
	mm := &store.MessageMeta{
		Tokens:           tokens,
		TokensPrompt:     tokensPrompt,
		TokensCompletion: tokensCompletion,
		LatencyMS:        latencyMS,
		StreamStatus:     streamStatus,
		Model:            model,
		Profile:          profile,
		Host:             host,
		Thinking:         thinking,
		Tools:            tools,
		Tasks:            tasks,
	}
	return mm
}

// lastAssistantIndex returns the index of the last assistant message in msgs, or -1.
func lastAssistantIndex(msgs []provider.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant {
			return i
		}
	}
	return -1
}

// setLastAssistantMeta writes meta into the metas map keyed by the last
// assistant message index in msgsOut. Initializes the map when nil. No-op when
// there is no assistant row or meta is nil.
func setLastAssistantMeta(metas map[int]*store.MessageMeta, msgsOut []provider.Message, meta *store.MessageMeta) map[int]*store.MessageMeta {
	if meta == nil {
		return metas
	}
	idx := lastAssistantIndex(msgsOut)
	if idx < 0 {
		return metas
	}
	if metas == nil {
		metas = make(map[int]*store.MessageMeta, 1)
	}
	metas[idx] = meta
	return metas
}

// wrapEmitWithMetaCapture returns an emit wrapper that forwards events to the
// underlying emit AND records tool/task events into acc for later persistence.
func wrapEmitWithMetaCapture(emit func(ChatEvent), acc *turnMetaAccumulator) func(ChatEvent) {
	if emit == nil {
		emit = func(ChatEvent) {}
	}
	if acc == nil {
		return emit
	}
	return func(ev ChatEvent) {
		acc.observe(ev)
		emit(ev)
	}
}
