package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// CanvasAIMode selects generate vs edit behaviour (D5b).
type CanvasAIMode string

const (
	// CanvasAIGenerate asks the model to invent a fresh diagram from a prompt.
	CanvasAIGenerate CanvasAIMode = "generate"
	// CanvasAIEdit asks the model to return a COMPLETE updated elements array
	// given existing elements as context (not a patch).
	CanvasAIEdit CanvasAIMode = "edit"
)

// CanvasAIRequest is the input for the D5b canvas AI generate/edit path.
// It is a dedicated non-stream, no-tools turn — never pollutes chat history
// (unlike ChatStream which is session-bound; VAL-CHAT-026 ephemeral still
// appends assistant rows). Canvas AI keeps the canvas buffer authoritative.
type CanvasAIRequest struct {
	// Prompt is the user instruction (e.g. "Draw auth flow" or "recolor Database box").
	Prompt string `json:"prompt"`
	// Mode is generate (invent) or edit (modify existing). Defaults to generate.
	Mode CanvasAIMode `json:"mode,omitempty"`
	// ExistingElementsJSON is the current Excalidraw elements array as JSON text.
	// Required for edit mode; ignored for generate mode (may be empty).
	ExistingElementsJSON string `json:"existing_elements_json,omitempty"`
	// Profile is the provider profile id (empty = session default).
	Profile string `json:"profile,omitempty"`
	// Model overrides the profile default / discovery when non-empty.
	Model string `json:"model,omitempty"`
}

// CanvasAIResult is the outcome of a CanvasAI turn. Never includes secrets.
type CanvasAIResult struct {
	// OK is true when the model returned a parseable elements array.
	OK bool `json:"ok"`
	// ElementsJSON is the canonical JSON array of Excalidraw elements (minted
	// ids for any missing). Empty when OK is false.
	ElementsJSON string `json:"elements_json,omitempty"`
	// RawText is the raw assistant content (post-fence-strip) for debugging.
	RawText string `json:"raw_text,omitempty"`
	// Mode echoes the effective mode (generate|edit).
	Mode CanvasAIMode `json:"mode"`
	// Profile is the resolved provider profile id used.
	Profile string `json:"profile,omitempty"`
	// Model is the resolved model id used.
	Model string `json:"model,omitempty"`
	// Error is a human-readable failure (no secrets).
	Error string `json:"error,omitempty"`
	// LatencyMS is the round-trip latency for the model call.
	LatencyMS int64 `json:"latency_ms,omitempty"`
}

// canvasAIContextCapRunes bounds the existing-elements context sent to the
// model. ~24k runes keeps most diagrams under typical context budgets while
// preventing pathological huge-element payloads from blowing the request.
const canvasAIContextCapRunes = 24_000

// canvasAITimeout is the upper bound for the non-stream ChatTurn. Non-stream
// completions are slower than SSE first-token, so we allow 90s.
const canvasAITimeout = 90 * time.Second

// CanvasAI runs a dedicated non-stream, no-tools chat turn to generate or
// context-aware edit an Excalidraw elements array. It does NOT use ChatStream,
// does NOT enable tools, and does NOT append to the durable chat thread — the
// canvas buffer is the sole source of truth (D5b).
//
// On parse failure it returns a structured error (never panics); the caller
// surfaces the error inline under the prompt bar.
func (s *Service) CanvasAI(req CanvasAIRequest) CanvasAIResult {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return CanvasAIResult{Error: "prompt is required"}
	}
	mode := strings.TrimSpace(string(req.Mode))
	if mode == "" {
		mode = string(CanvasAIGenerate)
	}
	switch CanvasAIMode(mode) {
	case CanvasAIGenerate, CanvasAIEdit:
	default:
		return CanvasAIResult{Error: fmt.Sprintf("mode must be %q or %q", CanvasAIGenerate, CanvasAIEdit)}
	}

	// Resolve provider profile (session default when empty). We deliberately do
	// NOT call resolveChatProfileModel — CanvasAI is not a chat turn and must
	// not be bound to the chat session's profile/model (per D5b "dedicated
	// path"). Empty profile falls back to the router default.
	router, err := s.providerRouter(req.Profile)
	if err != nil {
		return CanvasAIResult{Mode: CanvasAIMode(mode), Error: err.Error()}
	}
	client, err := router.GetProfile(req.Profile)
	if err != nil {
		return CanvasAIResult{Mode: CanvasAIMode(mode), Error: err.Error()}
	}
	if !client.APIKeyConfigured() {
		return CanvasAIResult{
			Mode: CanvasAIMode(mode),
			Error: (&provider.BlockedError{
				Profile: client.Name(),
				Reason:  "key required",
				Cause:   provider.ErrMissingAPIKey,
			}).Error(),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), canvasAITimeout)
	defer cancel()

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = client.DefaultModel()
	}
	// Prefer resolved chat model when discovery works; ignore errors and pass empty.
	if oc, ok := client.(*provider.OpenAICompat); ok {
		if resolved, rerr := oc.ResolveModel(ctx, model); rerr == nil && resolved != "" {
			model = resolved
		}
	}

	system := canvasAISystemPrompt(CanvasAIMode(mode), req.ExistingElementsJSON)
	userMsg := prompt

	started := time.Now()
	msg, err := client.ChatTurn(ctx, provider.ChatRequest{
		Model: model,
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: system},
			{Role: provider.RoleUser, Content: userMsg},
		},
		// No tools, no stream — D5b explicit constraints.
		Stream: false,
	})
	latency := time.Since(started).Milliseconds()
	if err != nil {
		// provider errors already redact; double-check no raw key shapes.
		return CanvasAIResult{
			Mode:      CanvasAIMode(mode),
			Profile:   client.Name(),
			Model:     model,
			Error:     err.Error(),
			LatencyMS: latency,
		}
	}

	raw := strings.TrimSpace(msg.Content)
	elements, parseErr := parseCanvasAIElements(raw)
	if parseErr != nil {
		return CanvasAIResult{
			Mode:      CanvasAIMode(mode),
			Profile:   client.Name(),
			Model:     model,
			RawText:   raw,
			Error:     parseErr.Error(),
			LatencyMS: latency,
		}
	}

	out, err := json.Marshal(elements)
	if err != nil {
		return CanvasAIResult{
			Mode:      CanvasAIMode(mode),
			Profile:   client.Name(),
			Model:     model,
			RawText:   raw,
			Error:     fmt.Sprintf("canvas AI: marshal elements: %v", err),
			LatencyMS: latency,
		}
	}

	return CanvasAIResult{
		OK:          true,
		ElementsJSON: string(out),
		RawText:     raw,
		Mode:        CanvasAIMode(mode),
		Profile:     client.Name(),
		Model:       model,
		LatencyMS:   latency,
	}
}

// canvasAISystemPrompt builds the system instruction for the canvas AI turn.
// The model is told to output ONLY a JSON array of Excalidraw elements (or an
// object with an `elements` key), with required fields per element type. For
// edit mode, the existing elements are included as context and the model is
// told to return a COMPLETE updated array (not a patch).
func canvasAISystemPrompt(mode CanvasAIMode, existingJSON string) string {
	var b strings.Builder
	b.WriteString("You are an Excalidraw diagram generator for the Inferenesia canvas.\n")
	b.WriteString("Output ONLY a JSON array of Excalidraw elements.\n")
	b.WriteString("Alternatively, you may output a JSON object with an \"elements\" key.\n")
	b.WriteString("Do NOT output markdown, prose, or code fences — only JSON.\n\n")
	b.WriteString("Each element MUST include:\n")
	b.WriteString("- id: string (unique; use stable ids like \"n1\", \"e1\")\n")
	b.WriteString("- type: one of rectangle | ellipse | diamond | arrow | line | text | freedraw\n")
	b.WriteString("- x, y: number (canvas position in pixels)\n")
	b.WriteString("- width, height: number (for rectangle/ellipse/diamond/text)\n")
	b.WriteString("- points: [[x,y],...] for arrow/line/freedraw (relative to x,y)\n")
	b.WriteString("- text: string (required for type=text)\n")
	b.WriteString("- fontSize: number (for text; default 20)\n")
	b.WriteString("- strokeColor, backgroundColor, fillStyle: optional strings\n")
	b.WriteString("- startBinding, endBinding: optional {elementId, focus, gap} for arrows\n")
	b.WriteString("- seed: integer (for stable rendering; pick any number)\n")
	b.WriteString("- version, versionNonce: integer (use 1)\n")
	b.WriteString("- isDeleted: false\n")
	b.WriteString("- groupIds: []\n")
	b.WriteString("- frameId: null\n")
	b.WriteString("- roundness: null or {type: 3}\n")
	b.WriteString("- boundElements: []\n")
	b.WriteString("- updated: integer (unix ms; use 1)\n")
	b.WriteString("- link: null\n")
	b.WriteString("- locked: false\n\n")

	switch mode {
	case CanvasAIEdit:
		b.WriteString("MODE: edit. The user wants to modify the existing diagram.\n")
		b.WriteString("EXISTING ELEMENTS (JSON array) — use as context:\n")
		existing := strings.TrimSpace(existingJSON)
		if existing == "" {
			b.WriteString("[]\n")
		} else {
			// Cap context to canvasAIContextCapRunes runes; truncate middle if needed.
			if len([]rune(existing)) > canvasAIContextCapRunes {
				existing = truncateRunesMiddle(existing, canvasAIContextCapRunes)
			}
			b.WriteString(existing)
			b.WriteString("\n")
		}
		b.WriteString("\nReturn a COMPLETE updated elements array (not a patch). Keep existing ids stable when modifying; only mint new ids for genuinely new elements. Preserve unchanged elements verbatim.\n")
	default:
		b.WriteString("MODE: generate. Invent a reasonable layout with readable spacing (~120px between nodes), unique stable ids, and clear labels. Prefer rectangle nodes with text labels and arrow edges with startBinding/endBinding where natural.\n")
	}
	b.WriteString("\nReturn ONLY the JSON. No markdown fences, no commentary.")
	return b.String()
}

// truncateRunesMiddle keeps the head and tail of a string, inserting a marker
// in the middle, so the model still sees both the start and end of a large
// elements array. Operates on runes (not bytes) to avoid splitting multibyte.
func truncateRunesMiddle(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	half := maxRunes / 2
	head := r[:half]
	tail := r[len(r)-half:]
	return string(head) + "\n…[truncated]…\n" + string(tail)
}

// fenceRe matches a leading ```json / ``` (or bare ```) fence and its closing
// counterpart. It is tolerant of optional language tag and surrounding
// whitespace/newlines. Non-greedy on the inner content.
var fenceRe = regexp.MustCompile("(?s)^\\s*```(?:json|excalidraw|jsonc)?\\s*\\n?(.*?)\\n?```\\s*$")

// parseCanvasAIElements extracts the Excalidraw elements array from the model
// output. It strips markdown fences if present, accepts either a bare JSON
// array or an object with an `elements` key, validates the elements array, and
// mints missing ids for any element lacking one.
//
// Returns a normalized []any (each element is a map[string]any). Never panics.
func parseCanvasAIElements(raw string) ([]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("canvas AI: empty model output")
	}

	// Strip a single surrounding fence if present.
	if m := fenceRe.FindStringSubmatch(trimmed); m != nil {
		trimmed = strings.TrimSpace(m[1])
	}

	// Some models wrap in fences mid-text; try to extract the first JSON array
	// or object if the whole string is not pure JSON.
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		extracted := extractFirstJSONBlock(trimmed)
		if extracted == "" {
			return nil, fmt.Errorf("canvas AI: model output is not valid JSON: %v", err)
		}
		if err2 := json.Unmarshal([]byte(extracted), &parsed); err2 != nil {
			return nil, fmt.Errorf("canvas AI: model output is not valid JSON: %v", err2)
		}
		trimmed = extracted
	}

	var elements []any
	switch v := parsed.(type) {
	case []any:
		elements = v
	case map[string]any:
		if rawElements, ok := v["elements"]; ok {
			arr, ok := rawElements.([]any)
			if !ok {
				return nil, fmt.Errorf("canvas AI: \"elements\" must be an array")
			}
			elements = arr
		} else {
			// Tolerate a single element object by wrapping it.
			elements = []any{v}
		}
	default:
		return nil, fmt.Errorf("canvas AI: expected JSON array or object, got %T", parsed)
	}

	if len(elements) == 0 {
		return nil, fmt.Errorf("canvas AI: elements array is empty")
	}

	// Validate + mint missing ids. Each element must be an object with a type.
	for i, el := range elements {
		m, ok := el.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("canvas AI: element %d is not an object", i)
		}
		if _, ok := m["type"]; !ok {
			return nil, fmt.Errorf("canvas AI: element %d missing \"type\"", i)
		}
		id, _ := m["id"].(string)
		if strings.TrimSpace(id) == "" {
			m["id"] = mintCanvasElementID()
		}
		elements[i] = m
	}
	return elements, nil
}

// extractFirstJSONBlock scans the string for the first balanced JSON array or
// object substring. Used when the model surrounds JSON with prose. Returns ""
// if no balanced block is found.
func extractFirstJSONBlock(s string) string {
	startArr := strings.Index(s, "[")
	startObj := strings.Index(s, "{")
	start := -1
	open := byte(0)
	close := byte(0)
	switch {
	case startArr < 0 && startObj < 0:
		return ""
	case startArr < 0:
		start, open, close = startObj, '{', '}'
	case startObj < 0:
		start, open, close = startArr, '[', ']'
	default:
		if startArr < startObj {
			start, open, close = startArr, '[', ']'
		} else {
			start, open, close = startObj, '{', '}'
		}
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// mintCanvasElementID returns a short random hex id for elements missing one.
// Uses crypto/rand so ids are stable-shaped and collision-resistant within a
// single turn (8 hex chars = 32 bits of entropy; plenty for one diagram).
func mintCanvasElementID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback: timestamp-based. Never returns empty.
		return fmt.Sprintf("el%d", time.Now().UnixNano()&0xffffffff)
	}
	return "el" + hex.EncodeToString(b[:])
}
