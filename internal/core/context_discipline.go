package core

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// P9-ctx: always-on context window discipline (outbound only).
//
// ApplyContextDiscipline produces a *model-bound* view of the conversation
// history that is capped by pair count and total rune budget, with bloat
// (Live Blocks / html:preview fences, large base64 data URLs) stripped to
// short placeholders and per-role per-message caps applied at rune boundary.
//
// It MUST NOT mutate the durable store history. core.Run passes the result
// to CompressMessagesHeadroom (optional) and then to the provider; the
// original msgs slice (with full tool results / UI history) is what gets
// persisted and shown in the UI.
//
// Ordering invariant (do not break Token Savers):
//
//	Discipline (always) → Headroom (optional) → provider
//
// Discipline is always-on and does not depend on any Token Saver toggle.
// Headroom remains opt-in and may further compress whatever discipline
// produced; failures there pass through the disciplined view unchanged.

// DisciplineOptions tunes ApplyContextDiscipline. Zero values fall back to
// DefaultContextDiscipline() at call time.
type DisciplineOptions struct {
	// MaxPairs caps the number of user/assistant exchanges kept in the
	// outbound view. Default 20. System messages are never counted.
	MaxPairs int
	// MaxTotalRunes caps the total rune count of non-system content in
	// the outbound view. Default 48000.
	MaxTotalRunes int
	// MaxUserRunes caps a single user message content. Default 4000.
	MaxUserRunes int
	// MaxAssistantRunes caps a single assistant message content (text
	// body; tool_calls are preserved intact). Default 8000.
	MaxAssistantRunes int
	// MaxToolRunes caps a single tool-role message content. Default 6000.
	MaxToolRunes int
	// MaxSystemRunes caps each leading system message. Default 12000.
	MaxSystemRunes int
}

// DefaultContextDiscipline returns coding-friendly defaults. These are
// intentionally conservative so a long session never silently overflows a
// 128k-class context window after bloat accumulates.
func DefaultContextDiscipline() DisciplineOptions {
	return DisciplineOptions{
		MaxPairs:          20,
		MaxTotalRunes:     48000,
		MaxUserRunes:      4000,
		MaxAssistantRunes: 8000,
		MaxToolRunes:      6000,
		MaxSystemRunes:    12000,
	}
}

// CanvasContextDiscipline preserves full Playground documents in outbound prompts.
// The coding cap would otherwise truncate the document after its editing rules.
func CanvasContextDiscipline() DisciplineOptions {
	return DisciplineOptions{
		MaxPairs:          8,
		MaxTotalRunes:     200000,
		MaxUserRunes:      150000,
		MaxAssistantRunes: 80000,
		MaxToolRunes:      8000,
		MaxSystemRunes:    16000,
	}
}

// DisciplineForAgentKind selects outbound context caps by agent kind.
// Coding ("" / "coding") keeps DefaultContextDiscipline; canvas kinds use
// CanvasContextDiscipline so large HTML payloads are not truncated mid-prompt.
func DisciplineForAgentKind(kind string) DisciplineOptions {
	switch strings.TrimSpace(kind) {
	case "slides", "excalidraw", "diagram", "html", "image", "markdown", "table", "timeline":
		return CanvasContextDiscipline()
	default:
		return DefaultContextDiscipline()
	}
}

// ApplyContextDiscipline returns a new outbound message slice that is the
// disciplined view of msgs. The input slice is never mutated.
//
// Algorithm:
//  1. Leading system messages are kept (each capped to MaxSystemRunes).
//  2. Non-system messages have StripBloat applied per content.
//  3. Sliding window from the end keeps at most MaxPairs user messages
//     while preserving tool_call→tool_result chains (an assistant with
//     tool_calls drags its following tool messages; an orphan tool message
//     without a parent assistant is dropped from the head).
//  4. Per-role per-message rune caps applied at rune boundary with a
//     "…[truncated]" marker.
//  5. If total non-system runes still exceed MaxTotalRunes, oldest kept
//     non-system messages are dropped (pair-aligned, preserving chains)
//     until under budget.
func ApplyContextDiscipline(msgs []provider.Message, opts DisciplineOptions) []provider.Message {
	if len(msgs) == 0 {
		return nil
	}
	if opts.MaxPairs <= 0 {
		opts.MaxPairs = 20
	}
	if opts.MaxTotalRunes <= 0 {
		opts.MaxTotalRunes = 48000
	}
	if opts.MaxUserRunes <= 0 {
		opts.MaxUserRunes = 4000
	}
	if opts.MaxAssistantRunes <= 0 {
		opts.MaxAssistantRunes = 8000
	}
	if opts.MaxToolRunes <= 0 {
		opts.MaxToolRunes = 6000
	}
	if opts.MaxSystemRunes <= 0 {
		opts.MaxSystemRunes = 12000
	}

	// 1. Split leading system messages from the rest. Only a contiguous
	//    leading run is treated as "system preamble"; a system message
	//    appearing later (rare) stays in-place with the non-system flow.
	leadingSystem := 0
	for leadingSystem < len(msgs) && msgs[leadingSystem].Role == provider.RoleSystem {
		leadingSystem++
	}
	out := make([]provider.Message, 0, len(msgs))
	for i := 0; i < leadingSystem; i++ {
		m := cloneMessage(msgs[i])
		m.Content = capRunes(m.Content, opts.MaxSystemRunes)
		out = append(out, m)
	}
	rest := msgs[leadingSystem:]

	// 2. Strip bloat on each non-system message (content + tool content).
	stripped := make([]provider.Message, len(rest))
	for i, m := range rest {
		stripped[i] = cloneMessage(m)
		stripped[i].Content = StripBloat(m.Content)
	}

	// 3. Sliding window by pairs, preserving tool_call→tool_result chains.
	kept := selectByPairs(stripped, opts.MaxPairs)

	// 4. Per-role per-message rune caps.
	for i := range kept {
		kept[i].Content = capByRole(kept[i], opts)
	}

	// 5. Total budget: drop oldest pair-aligned (chain-safe) until under.
	kept = enforceTotalBudget(kept, opts.MaxTotalRunes)

	return append(out, kept...)
}

// selectByPairs walks from the end and keeps messages until MaxPairs user
// messages have been collected, while guaranteeing that:
//   - an assistant message with tool_calls keeps its following tool messages;
//   - a tool message whose parent assistant was dropped is itself dropped
//     (never leave an orphan tool message — providers reject it).
//
// The window is always aligned to start at a user message: any assistant
// (with or without tool_calls) that loses its preceding user is dropped,
// along with its trailing tool messages, so no orphan tool_call / tool_result
// can leak into the outbound view.
func selectByPairs(msgs []provider.Message, maxPairs int) []provider.Message {
	if len(msgs) == 0 {
		return nil
	}
	userSeen := 0
	cut := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleUser {
			userSeen++
			if userSeen > maxPairs {
				cut = i + 1
				break
			}
			cut = i
		}
	}
	if cut < 0 {
		cut = 0
	}
	kept := msgs[cut:]
	// Align: if the window does not start at a user message, drop leading
	// non-user messages so we never keep an assistant / tool chain whose
	// user turn was excluded.
	for len(kept) > 0 && kept[0].Role != provider.RoleUser {
		kept = kept[1:]
	}
	return dropLeadingOrphanTools(kept)
}

// dropLeadingOrphanTools removes leading tool messages whose parent
// assistant tool_calls are not in the kept window. It also repairs the
// edge case where the first kept message is a tool result with no
// preceding assistant tool_calls.
func dropLeadingOrphanTools(msgs []provider.Message) []provider.Message {
	// Build the set of tool_call IDs present in kept assistant messages.
	parentIDs := make(map[string]bool)
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant {
			for _, tc := range m.ToolCalls {
				parentIDs[tc.ID] = true
			}
		}
	}
	// Drop leading tool messages that have no parent in the window.
	start := 0
	for start < len(msgs) {
		m := msgs[start]
		if m.Role != provider.RoleTool {
			break
		}
		if !parentIDs[m.ToolCallID] {
			start++
			continue
		}
		break
	}
	if start >= len(msgs) {
		return nil
	}
	// Also drop any trailing tool messages whose parent assistant was
	// itself dropped — handled implicitly because we only drop from the
	// head here; mid-window orphans do not occur given the pair window.
	return msgs[start:]
}

// enforceTotalBudget drops oldest pair-aligned messages until the total
// rune count of the slice is under max. Tool chains are preserved: when
// dropping an assistant with tool_calls, its trailing tool messages are
// dropped together. The leading system messages are NOT in this slice
// (they were prepended by the caller).
func enforceTotalBudget(msgs []provider.Message, max int) []provider.Message {
	for totalRunes(msgs) > max && len(msgs) > 1 {
		// Drop the first message; if it was an assistant with tool_calls,
		// also drop the contiguous run of tool messages that follow and
		// belong to those call IDs.
		first := msgs[0]
		msgs = msgs[1:]
		if first.Role == provider.RoleAssistant && len(first.ToolCalls) > 0 {
			callIDs := make(map[string]bool, len(first.ToolCalls))
			for _, tc := range first.ToolCalls {
				callIDs[tc.ID] = true
			}
			for len(msgs) > 0 && msgs[0].Role == provider.RoleTool && callIDs[msgs[0].ToolCallID] {
				msgs = msgs[1:]
			}
		}
		// Re-strip any new leading orphan tool (defensive — should not
		// happen because of the chain drop above, but keeps invariant).
		msgs = dropLeadingOrphanTools(msgs)
		if len(msgs) == 0 {
			break
		}
	}
	return msgs
}

func totalRunes(msgs []provider.Message) int {
	n := 0
	for _, m := range msgs {
		n += utf8.RuneCountInString(m.Content)
	}
	return n
}

func capByRole(m provider.Message, opts DisciplineOptions) string {
	switch m.Role {
	case provider.RoleUser:
		return capRunes(m.Content, opts.MaxUserRunes)
	case provider.RoleAssistant:
		return capRunes(m.Content, opts.MaxAssistantRunes)
	case provider.RoleTool:
		return capRunes(m.Content, opts.MaxToolRunes)
	default:
		return m.Content
	}
}

// capRunes returns s truncated to at most max runes. When truncation
// happens, it tries to break on the last whitespace within the final
// 10% of the budget for readability and appends " …[truncated]".
func capRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := max
	// Try to break on whitespace in the trailing 10% for readability.
	searchStart := max - max/10
	if searchStart < 0 {
		searchStart = 0
	}
	for i := max - 1; i >= searchStart; i-- {
		ch := r[i]
		if ch == ' ' || ch == '\n' || ch == '\t' {
			cut = i
			break
		}
	}
	return string(r[:cut]) + " …[truncated]"
}

// cloneMessage returns a shallow copy of m. ToolCalls and Parts slices
// are reused (never mutated by discipline); only Content is replaced.
func cloneMessage(m provider.Message) provider.Message {
	out := m
	return out
}

// ---- StripBloat ---------------------------------------------------------

var (
	// liveBlockFenceRe matches fenced ```live-block, ```yura-live, and
	// ```html:preview blocks (case-insensitive info string, non-greedy body).
	// Replaced with a short placeholder. Languages MUST stay in sync with
	// config.IsLiveBlockLanguage and web/src/features/chat/liveBlocks.ts.
	liveBlockFenceRe = regexp.MustCompile("(?si)```(?:live-block|yura-live|html:preview)\\b.*?```")
	// dataURLBase64Re matches long base64 data URLs (image/audio/etc).
	// We only strip when the base64 payload is "very long" to avoid
	// touching short inline data: icons. Bounded by a min length.
	dataURLBase64Re = regexp.MustCompile("(?s)data:[a-zA-Z0-9.+-]+/[a-zA-Z0-9.+-]+;base64,[A-Za-z0-9+/=]{400,}")
)

// StripBloat replaces large Live Block / html:preview fenced blocks and
// huge base64 data URLs with short summary placeholders. It is safe on
// empty / plain text (returns input unchanged). It never mutates the
// input string.
func StripBloat(s string) string {
	if s == "" {
		return s
	}
	// Fenced Live Blocks / html:preview → placeholder with approx size.
	out := liveBlockFenceRe.ReplaceAllStringFunc(s, func(match string) string {
		return "[stripped live-block/html preview ~" + itoa(len(match)) + " chars]"
	})
	// Huge base64 data URLs → short placeholder preserving the mime hint.
	out = dataURLBase64Re.ReplaceAllStringFunc(out, func(match string) string {
		mime := "data"
		if i := strings.Index(match, ";base64,"); i > 5 {
			mime = match[5:i]
		}
		return "[stripped base64 " + mime + " ~" + itoa(len(match)) + " chars]"
	})
	return out
}

// itoa is a tiny allocation-free int → string for placeholder sizes.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
