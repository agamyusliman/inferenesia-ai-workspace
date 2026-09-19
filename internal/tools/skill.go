package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/skills"
)

// Skill-related tool names (VAL-ORCH-004 / VAL-ORCH-005).
const (
	NameSkill            = "skill"
	NameSkillStatus      = "skill_status"
	NameMultibrainRead   = "multibrain_read"
	NameMultibrainAppend = "multibrain_append"
)

// SkillBackend is implemented by a session skills.Loader adapter.
type SkillBackend interface {
	// List returns progressive metadata only.
	List() ([]skills.Meta, error)
	// Load activates a named skill; returns full skill.
	Load(name string) (*skills.Skill, error)
	// Unload deactivates a skill.
	Unload(name string) bool
	// LoadedNames lists activated skill ids.
	LoadedNames() []string
	// SystemPromptOverlay is the injected prompt from loaded skills.
	SystemPromptOverlay() string
	// HasExtraTool reports whether a skill-granted tool is active.
	HasExtraTool(name string) bool
	// IsLoaded reports activation.
	IsLoaded(name string) bool
}

// SkillMCPRegistrar registers/unregisters MCP servers that a skill declares
// in its frontmatter (VAL-CROSS-012). The skills package cannot import
// internal/mcp (cycle), so the tools package adapts MCPServerSpec →
// mcp.ServerConfig here.
//
// RegisterSkillMCP is called from execSkill load after the skill body is
// activated. UnregisterSkillMCP is called from execSkill unload BEFORE the
// skill is deactivated so the namespaced tools disappear from the task tool
// namespace for subsequent task() calls.
type SkillMCPRegistrar interface {
	// RegisterSkillMCP registers every MCP server declared by the named skill
	// on the shared MCP host. Server names are qualified as
	// "<skill-name>:<server>" so two skills cannot collide. Returns the list
	// of fully-qualified server names that were registered (for unload).
	RegisterSkillMCP(ctx context.Context, skillName string, specs []skills.MCPServerSpec) ([]string, error)
	// UnregisterSkillMCP stops and removes every MCP server previously
	// registered for the named skill. Returns the list of server names that
	// were removed (may be shorter than the registered list if some failed
	// to start).
	UnregisterSkillMCP(skillName string) []string
	// RegisteredSkillServers returns the fully-qualified server names
	// currently registered for the named skill (for diagnostics/tests).
	RegisteredSkillServers(skillName string) []string
}

// SetSkillBackend wires the skill tool and progressive loader.
func (r *Registry) SetSkillBackend(sb SkillBackend) {
	if r == nil {
		return
	}
	r.skill = sb
}

// SkillBackend returns the attached skill backend (may be nil).
func (r *Registry) SkillBackend() SkillBackend {
	if r == nil {
		return nil
	}
	return r.skill
}

// SetSkillMCPRegistrar wires the MCP registrar used by execSkill load/unload
// to register/unregister skill-declared MCP servers (VAL-CROSS-012). Pass nil
// to disable; in that case skills with mcp-servers frontmatter load their
// body/prompt/allowed-tools but their MCP servers are not registered (and a
// clear message is returned to the agent).
func (r *Registry) SetSkillMCPRegistrar(reg SkillMCPRegistrar) {
	if r == nil {
		return
	}
	r.skillMCP = reg
}

// SkillMCPRegistrar returns the attached registrar (may be nil).
func (r *Registry) SkillMCPRegistrar() SkillMCPRegistrar {
	if r == nil {
		return nil
	}
	return r.skillMCP
}

// SkillBackendFromLoader adapts *skills.Loader to SkillBackend.
type SkillBackendFromLoader struct {
	Loader *skills.Loader
}

// NewSkillBackend wraps a Loader for tools.Registry.
func NewSkillBackend(l *skills.Loader) *SkillBackendFromLoader {
	return &SkillBackendFromLoader{Loader: l}
}

func (s *SkillBackendFromLoader) List() ([]skills.Meta, error) {
	if s == nil || s.Loader == nil {
		return nil, fmt.Errorf("skill: loader not configured")
	}
	return s.Loader.List()
}

func (s *SkillBackendFromLoader) Load(name string) (*skills.Skill, error) {
	if s == nil || s.Loader == nil {
		return nil, fmt.Errorf("skill: loader not configured")
	}
	return s.Loader.Load(name)
}

func (s *SkillBackendFromLoader) Unload(name string) bool {
	if s == nil || s.Loader == nil {
		return false
	}
	return s.Loader.Unload(name)
}

func (s *SkillBackendFromLoader) LoadedNames() []string {
	if s == nil || s.Loader == nil {
		return nil
	}
	return s.Loader.LoadedNames()
}

func (s *SkillBackendFromLoader) SystemPromptOverlay() string {
	if s == nil || s.Loader == nil {
		return ""
	}
	return s.Loader.SystemPromptOverlay()
}

func (s *SkillBackendFromLoader) HasExtraTool(name string) bool {
	if s == nil || s.Loader == nil {
		return false
	}
	return s.Loader.HasExtraTool(name)
}

func (s *SkillBackendFromLoader) IsLoaded(name string) bool {
	if s == nil || s.Loader == nil {
		return false
	}
	return s.Loader.IsLoaded(name)
}

func skillDefs(r *Registry) []Definition {
	if r == nil || r.skill == nil {
		return nil
	}
	defs := []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameSkill,
				Description: "List available skills (progressive metadata only) or load a named skill to change the session toolset/system prompt. Actions: list | load | unload | loaded | prompt. Loading multi-brain grants multibrain_read/multibrain_append tools.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": { "type": "string", "description": "list | load | unload | loaded | prompt", "enum": ["list", "load", "unload", "loaded", "prompt"] },
    "name": { "type": "string", "description": "Skill name for load/unload (e.g. multi-brain, go-core-worker)" }
  },
  "required": ["action"]
}`),
			},
		},
	}
	// skill_status is only advertised when a loaded skill grants it (observable toolset delta).
	if r.skill.HasExtraTool(NameSkillStatus) {
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameSkillStatus,
				Description: "Report which skills are currently loaded and the system-prompt overlay (granted by go-core-worker skill).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		})
	}
	// multibrain tools only when multi-brain skill is loaded (or backend grants them).
	if r.skill.HasExtraTool(NameMultibrainRead) || r.skill.IsLoaded(skills.NameMultiBrain) {
		defs = append(defs,
			Definition{
				Type: "function",
				Function: FunctionSpec{
					Name:        NameMultibrainRead,
					Description: "Multi Brain protocol read: opens session.md first, then at most 1–2 matching buckets under indexes/, then pointed context files only — never the entire .multibrain/ tree.",
					Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Optional free-text to rank which 1–2 buckets to open" }
  }
}`),
				},
			},
			Definition{
				Type: "function",
				Function: FunctionSpec{
					Name:        NameMultibrainAppend,
					Description: "Multi Brain protocol write: append one newest-first entry (timestamp WIB — agent: summary -> context pointer) to the best-matching bucket via WriteGateway.",
					Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "summary": { "type": "string", "description": "Short one-line summary (required)" },
    "agent": { "type": "string", "description": "Agent display name (default: Agent)" },
    "topic": { "type": "string", "description": "Topic slug for optional context file" },
    "query": { "type": "string", "description": "Free-text to select best bucket" },
    "bucket": { "type": "string", "description": "Force bucket name (creates if missing)" },
    "description": { "type": "string", "description": "Scope text when creating a new bucket" },
    "decision": { "type": "string", "description": "Decision note (triggers context file)" },
    "blocker": { "type": "string", "description": "Blocker note (triggers context file)" },
    "verification": { "type": "string", "description": "Verification evidence (triggers context file)" },
    "files": { "type": "array", "items": { "type": "string" }, "description": "Important file paths (triggers context)" },
    "body": { "type": "string", "description": "Extra free-form context body" }
  },
  "required": ["summary"]
}`),
				},
			},
		)
	}
	return defs
}

type skillArgs struct {
	Action string `json:"action"`
	Name   string `json:"name"`
}

func (r *Registry) execSkill(call Call) Result {
	if r.skill == nil {
		return Result{
			Name:    NameSkill,
			Content: "skill: backend not configured",
			IsError: true,
		}
	}
	var args skillArgs
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return Result{
				Name:    NameSkill,
				Content: fmt.Sprintf("skill: invalid arguments JSON: %v", err),
				IsError: true,
			}
		}
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))
	if action == "" {
		action = "list"
	}
	switch action {
	case "list":
		list, err := r.skill.List()
		if err != nil {
			return Result{Name: NameSkill, Content: err.Error(), IsError: true}
		}
		var b strings.Builder
		b.WriteString("skills (metadata only; bodies load on skill load):\n")
		if len(list) == 0 {
			b.WriteString("  (none)\n")
		}
		for _, m := range list {
			loaded := ""
			if r.skill.IsLoaded(m.Name) {
				loaded = " [loaded]"
			}
			fmt.Fprintf(&b, "  %s  [%s]%s\n    %s\n", m.Name, m.Source, loaded, m.Description)
			if m.Path != "" {
				fmt.Fprintf(&b, "    path: %s\n", m.Path)
			}
		}
		return Result{Name: NameSkill, Content: b.String()}
	case "load":
		name := strings.TrimSpace(args.Name)
		if name == "" {
			return Result{Name: NameSkill, Content: "skill load: name is required", IsError: true}
		}
		sk, err := r.skill.Load(name)
		if err != nil {
			return Result{Name: NameSkill, Content: err.Error(), IsError: true}
		}
		tools := strings.Join(sk.AllowedTools, ", ")
		if tools == "" {
			tools = "(none extra)"
		}
		// VAL-CROSS-012: register any MCP servers the skill declares in its
		// frontmatter so the namespaced tools appear in the task tool
		// namespace for subsequent task() calls. Unload unregisters them.
		mcpNote := registerSkillMCPServers(r, context.Background(), sk)
		return Result{
			Name: NameSkill,
			Content: fmt.Sprintf(
				"loaded skill %q source=%s\nallowed_tools: %s\n%s\nsystem_prompt_overlay_bytes: %d\nskill body preview: %s",
				sk.Name, sk.Source, tools, mcpNote, len(r.skill.SystemPromptOverlay()), truncateStr(sk.Body, 200),
			),
		}
	case "unload":
		name := strings.TrimSpace(args.Name)
		if name == "" {
			return Result{Name: NameSkill, Content: "skill unload: name is required", IsError: true}
		}
		// VAL-CROSS-012: unregister the skill's MCP servers BEFORE deactivating
		// the skill body so the namespaced tools disappear from the task tool
		// namespace for subsequent task() calls. After unregister, calls to
		// mcp.<server>.<tool> return "unknown tool".
		unregisterNote := unregisterSkillMCPServers(r, name)
		if !r.skill.Unload(name) {
			return Result{Name: NameSkill, Content: fmt.Sprintf("skill %q was not loaded%s", name, unregisterNote)}
		}
		return Result{Name: NameSkill, Content: fmt.Sprintf("unloaded skill %q%s", name, unregisterNote)}
	case "loaded":
		names := r.skill.LoadedNames()
		if len(names) == 0 {
			return Result{Name: NameSkill, Content: "no skills loaded"}
		}
		return Result{Name: NameSkill, Content: "loaded: " + strings.Join(names, ", ")}
	case "prompt":
		overlay := r.skill.SystemPromptOverlay()
		if strings.TrimSpace(overlay) == "" {
			return Result{Name: NameSkill, Content: "(no system prompt overlay — load a skill first)"}
		}
		return Result{Name: NameSkill, Content: overlay}
	default:
		return Result{
			Name:    NameSkill,
			Content: fmt.Sprintf("skill: unknown action %q (list|load|unload|loaded|prompt)", action),
			IsError: true,
		}
	}
}

func (r *Registry) execSkillStatus(call Call) Result {
	if r.skill == nil {
		return Result{Name: NameSkillStatus, Content: "skill_status: backend not configured", IsError: true}
	}
	if !r.skill.HasExtraTool(NameSkillStatus) && !r.skill.IsLoaded(skills.NameGoCore) {
		return Result{
			Name:    NameSkillStatus,
			Content: "skill_status: not available (load go-core-worker skill first)",
			IsError: true,
		}
	}
	names := r.skill.LoadedNames()
	return Result{
		Name: NameSkillStatus,
		Content: fmt.Sprintf(
			"skill_status: loaded=[%s]\noverlay:\n%s",
			strings.Join(names, ", "),
			r.skill.SystemPromptOverlay(),
		),
	}
}

type multibrainReadArgs struct {
	Query string `json:"query"`
}

func (r *Registry) execMultibrainRead(call Call) Result {
	if r.skill == nil || (!r.skill.HasExtraTool(NameMultibrainRead) && !r.skill.IsLoaded(skills.NameMultiBrain)) {
		return Result{
			Name:    NameMultibrainRead,
			Content: "multibrain_read: load multi-brain skill first (skill load multi-brain)",
			IsError: true,
		}
	}
	var args multibrainReadArgs
	if strings.TrimSpace(call.Arguments) != "" {
		_ = json.Unmarshal([]byte(call.Arguments), &args)
	}
	ws := ""
	if r.gw != nil {
		ws = r.gw.RootPath()
	}
	if ws == "" {
		return Result{Name: NameMultibrainRead, Content: "multibrain_read: workspace not configured", IsError: true}
	}
	res, err := skills.MultiBrainRead(skills.MultiBrainReadInput{
		Workspace: ws,
		Query:     args.Query,
	})
	if err != nil {
		return Result{Name: NameMultibrainRead, Content: err.Error(), IsError: true}
	}
	return Result{Name: NameMultibrainRead, Content: res.Summary}
}

type multibrainAppendArgs struct {
	Summary      string   `json:"summary"`
	Agent        string   `json:"agent"`
	Topic        string   `json:"topic"`
	Query        string   `json:"query"`
	Bucket       string   `json:"bucket"`
	Description  string   `json:"description"`
	Decision     string   `json:"decision"`
	Blocker      string   `json:"blocker"`
	Verification string   `json:"verification"`
	Files        []string `json:"files"`
	Body         string   `json:"body"`
}

func (r *Registry) execMultibrainAppend(call Call) Result {
	if r.skill == nil || (!r.skill.HasExtraTool(NameMultibrainAppend) && !r.skill.IsLoaded(skills.NameMultiBrain)) {
		return Result{
			Name:    NameMultibrainAppend,
			Content: "multibrain_append: load multi-brain skill first (skill load multi-brain)",
			IsError: true,
		}
	}
	if r.gw == nil {
		return Result{Name: NameMultibrainAppend, Content: "multibrain_append: WriteGateway not configured", IsError: true}
	}
	var args multibrainAppendArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{
			Name:    NameMultibrainAppend,
			Content: fmt.Sprintf("multibrain_append: invalid arguments: %v", err),
			IsError: true,
		}
	}
	if strings.TrimSpace(args.Summary) == "" {
		return Result{Name: NameMultibrainAppend, Content: "multibrain_append: summary is required", IsError: true}
	}
	res, err := skills.MultiBrainAppend(r.gw, skills.MultiBrainAppendInput{
		Workspace:    r.gw.RootPath(),
		Agent:        args.Agent,
		Summary:      args.Summary,
		Topic:        args.Topic,
		Query:        args.Query,
		Bucket:       args.Bucket,
		Description:  args.Description,
		Decision:     args.Decision,
		Blocker:      args.Blocker,
		Verification: args.Verification,
		Files:        args.Files,
		Body:         args.Body,
		When:         time.Time{}, // now
	})
	if err != nil {
		return Result{Name: NameMultibrainAppend, Content: err.Error(), IsError: true}
	}
	msg := fmt.Sprintf(
		"appended to bucket=%s path=%s\nentry: %s\nformat_ok=%v session_updated=%v created_bucket=%v",
		res.BucketName, res.BucketPath, res.EntryLine, res.FormatOK, res.SessionUpdated, res.CreatedBucket,
	)
	if res.ContextPath != "" {
		msg += "\ncontext: " + res.ContextPath
	}
	return Result{Name: NameMultibrainAppend, Content: msg}
}

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// registerSkillMCPServers registers every MCP server declared by the skill on
// the attached SkillMCPRegistrar (VAL-CROSS-012). Returns a human-readable
// note for the skill load result. When no registrar is attached, the skill
// still loads its body/prompt/allowed-tools but its MCP servers are skipped
// with a clear message (graceful degrade, not a crash).
func registerSkillMCPServers(r *Registry, ctx context.Context, sk *skills.Skill) string {
	if r == nil || sk == nil || len(sk.MCPServers) == 0 {
		return "mcp_servers: (none)"
	}
	reg := r.SkillMCPRegistrar()
	if reg == nil {
		// No MCP host wired (e.g. CLI without --mcp). Surface clearly so the
		// agent knows the namespaced tools are not available; do not fail the
		// skill load — the body/prompt/allowed-tools are still active.
		return fmt.Sprintf("mcp_servers: %d declared but no MCP registrar attached (namespaced tools not registered)", len(sk.MCPServers))
	}
	registered, err := reg.RegisterSkillMCP(ctx, sk.Name, sk.MCPServers)
	if err != nil {
		return fmt.Sprintf("mcp_servers: registered %d/%d — %v", len(registered), len(sk.MCPServers), err)
	}
	if len(registered) == 0 {
		return "mcp_servers: 0 registered (all failed; see logs)"
	}
	return fmt.Sprintf("mcp_servers: registered %d (%s)", len(registered), strings.Join(registered, ", "))
}

// unregisterSkillMCPServers removes every MCP server previously registered for
// the named skill (VAL-CROSS-012). Returns a human-readable note for the
// skill unload result. Subsequent task() calls cannot invoke the namespaced
// tools (unknown-tool error from the MCP host).
func unregisterSkillMCPServers(r *Registry, skillName string) string {
	if r == nil {
		return ""
	}
	reg := r.SkillMCPRegistrar()
	if reg == nil {
		return ""
	}
	removed := reg.UnregisterSkillMCP(skillName)
	if len(removed) == 0 {
		return ""
	}
	return fmt.Sprintf(" (unregistered mcp servers: %s)", strings.Join(removed, ", "))
}
