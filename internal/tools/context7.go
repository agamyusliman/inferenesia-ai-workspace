package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/context7"
)

// NameContext7Query is the builtin library-docs tool name (P9-ctx).
const NameContext7Query = "context7_query"

// Context7Backend is the subset of *context7.Client the registry needs.
// Injected so tests can use httptest without touching the network.
type Context7Backend interface {
	Search(ctx context.Context, libraryName, query string) ([]context7.Library, error)
	FetchContext(ctx context.Context, libraryID, query string) (string, error)
	FetchDocs(ctx context.Context, libraryName, query, libraryID string) (string, error)
}

// SetContext7Backend wires the builtin library-docs tool. Pass nil to clear.
// When the env gate (YURA_AI_CONTEXT7=0 / CONTEXT7_DISABLED=1) is set, the
// tool stays registered but returns a clear "disabled" message so the model
// knows it cannot fetch external docs this session.
func (r *Registry) SetContext7Backend(b Context7Backend) {
	if r == nil {
		return
	}
	r.context7 = b
}

// Context7Backend returns the attached backend (may be nil).
func (r *Registry) Context7Backend() Context7Backend {
	if r == nil {
		return nil
	}
	return r.context7
}

// Context7Disabled reports whether the builtin Context7 tool is disabled by
// env. YURA_AI_CONTEXT7=0 or CONTEXT7_DISABLED=1 disable it.
func Context7Disabled() bool {
	if v := strings.TrimSpace(os.Getenv("YURA_AI_CONTEXT7")); v == "0" || strings.EqualFold(v, "false") {
		return true
	}
	if v := strings.TrimSpace(os.Getenv("CONTEXT7_DISABLED")); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	return false
}

// ResolveContext7APIKey picks the first non-empty Context7 API key from env.
// Empty = anonymous (rate-limited). Order: YURA_AI_CONTEXT7_API_KEY, CONTEXT7_API_KEY.
func ResolveContext7APIKey() string {
	return context7.ResolveAPIKey(
		os.Getenv("YURA_AI_CONTEXT7_API_KEY"),
		os.Getenv("CONTEXT7_API_KEY"),
	)
}

// EnsureContext7Backend attaches a default *context7.Client when no backend
// is set and the tool is not env-disabled. Used by CLI/desktop bootstrap so
// `context7_query` is available out-of-the-box without extra wiring.
func EnsureContext7Backend(r *Registry) {
	if r == nil || r.context7 != nil {
		return
	}
	if Context7Disabled() {
		return
	}
	r.context7 = context7.New(ResolveContext7APIKey())
}

func context7Defs(r *Registry) []Definition {
	if r == nil || r.context7 == nil {
		return nil
	}
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameContext7Query,
				Description: "Fetch up-to-date library/framework documentation snippets via Context7 (bounded, ~12k chars). Soft-fails when offline, rate-limited, or missing key — returns a clear error, never crashes chat. Prefer this for library/framework API questions over guessing from training data. Does not modify files.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "library_name": { "type": "string", "description": "Library or framework name, e.g. \"react\", \"next.js\", \"tailwindcss\"" },
    "query": { "type": "string", "description": "What to look up (API, hook, config option, usage pattern)" },
    "library_id": { "type": "string", "description": "Optional Context7 library id to skip search, e.g. \"/vercel/next.js\"" }
  },
  "required": ["library_name", "query"]
}`),
			},
		},
	}
}

type context7QueryArgs struct {
	LibraryName string `json:"library_name"`
	Query       string `json:"query"`
	LibraryID   string `json:"library_id"`
}

func (r *Registry) execContext7Query(ctx context.Context, call Call) Result {
	if r == nil || r.context7 == nil {
		return Result{
			ID:      call.ID,
			Name:    NameContext7Query,
			Content: "context7_query: tool not registered (disabled or no backend). Set YURA_AI_CONTEXT7=1 (default) and ensure a backend is wired.",
			IsError: true,
		}
	}
	if Context7Disabled() {
		return Result{
			ID:      call.ID,
			Name:    NameContext7Query,
			Content: "context7_query: disabled by env (YURA_AI_CONTEXT7=0 or CONTEXT7_DISABLED=1). Re-enable to fetch library docs.",
			IsError: true,
		}
	}
	var args context7QueryArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{
			ID:      call.ID,
			Name:    NameContext7Query,
			Content: fmt.Sprintf("context7_query: invalid arguments JSON: %v", err),
			IsError: true,
		}
	}
	if strings.TrimSpace(args.LibraryName) == "" {
		return Result{
			ID:      call.ID,
			Name:    NameContext7Query,
			Content: "context7_query: library_name is required",
			IsError: true,
		}
	}
	if strings.TrimSpace(args.Query) == "" {
		return Result{
			ID:      call.ID,
			Name:    NameContext7Query,
			Content: "context7_query: query is required",
			IsError: true,
		}
	}
	out, err := r.context7.FetchDocs(ctx, args.LibraryName, args.Query, args.LibraryID)
	if err != nil {
		// Soft-fail: clear error string to the model, never crash chat.
		return Result{
			ID:      call.ID,
			Name:    NameContext7Query,
			Content: fmt.Sprintf("context7_query: %v", err),
			IsError: true,
		}
	}
	return Result{
		ID:      call.ID,
		Name:    NameContext7Query,
		Content: out,
	}
}
