package config

import (
	"fmt"
	"strings"
)

// Chat / role model routing keys (Settings → Mode models + chat mode picker).
const (
	RoleSession       = "session"       // default session model (main chat)
	RoleAgent         = "agent"         // execution / default agent mode
	RolePlan          = "plan"          // plan mode (heavy / large context)
	RoleMission       = "mission"       // mission orchestrator surface
	RoleOrchestrator  = "orchestrator"  // mission/task orchestrator
	RoleWorker        = "worker"        // general worker
	RoleValidation    = "validation"    // validation worker
	RoleExplore       = "explore"       // explore subagent (fast)
	RoleImplement     = "implement"     // implement / executor
	RoleResearch      = "research"      // research / librarian-like
	RoleVerify        = "verify"        // verify subagent
	RoleLight         = "light"         // subagent light
	RoleMedium        = "medium"        // subagent medium
	RoleHeavy         = "heavy"         // subagent heavy
	RoleDefaultMode   = "default_chat_mode"
)

// ModelRef is an optional profile + model override for one role.
// Empty fields mean "inherit session default / sticky picker".
type ModelRef struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Model   string `yaml:"model,omitempty" json:"model,omitempty"`
}

// ModeModelsConfig maps chat modes and subagent roles to default models.
// Persisted under ~/.inferenesia/config.yaml as mode_models.
type ModeModelsConfig struct {
	// DefaultChatMode is agent | plan | mission | explore (UI default on open).
	DefaultChatMode string `yaml:"default_chat_mode,omitempty" json:"default_chat_mode,omitempty"`

	Session      ModelRef `yaml:"session,omitempty" json:"session,omitempty"`
	Agent        ModelRef `yaml:"agent,omitempty" json:"agent,omitempty"`
	Plan         ModelRef `yaml:"plan,omitempty" json:"plan,omitempty"`
	Mission      ModelRef `yaml:"mission,omitempty" json:"mission,omitempty"`
	Orchestrator ModelRef `yaml:"orchestrator,omitempty" json:"orchestrator,omitempty"`
	Worker       ModelRef `yaml:"worker,omitempty" json:"worker,omitempty"`
	Validation   ModelRef `yaml:"validation,omitempty" json:"validation,omitempty"`
	Explore      ModelRef `yaml:"explore,omitempty" json:"explore,omitempty"`
	Implement    ModelRef `yaml:"implement,omitempty" json:"implement,omitempty"`
	Research     ModelRef `yaml:"research,omitempty" json:"research,omitempty"`
	Verify       ModelRef `yaml:"verify,omitempty" json:"verify,omitempty"`
	Light        ModelRef `yaml:"light,omitempty" json:"light,omitempty"`
	Medium       ModelRef `yaml:"medium,omitempty" json:"medium,omitempty"`
	Heavy        ModelRef `yaml:"heavy,omitempty" json:"heavy,omitempty"`
}

// RoleMeta is UI metadata for one configurable role.
type RoleMeta struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Group       string `json:"group"` // session | chat_mode | orchestrator | subagent
}

// KnownModeRoles is the ordered Settings list.
func KnownModeRoles() []RoleMeta {
	return []RoleMeta{
		{ID: RoleSession, Label: "Session default", Description: "Default model for main chat when no mode override applies", Group: "session"},
		{ID: RoleAgent, Label: "Agent (execution)", Description: "Default agent / execution mode", Group: "chat_mode"},
		{ID: RolePlan, Label: "Plan mode", Description: "Plan Mode (prefer heavy / large-context model)", Group: "chat_mode"},
		{ID: RoleMission, Label: "Mission mode", Description: "Mission surface (orchestrated goals)", Group: "chat_mode"},
		{ID: RoleExplore, Label: "Explore mode", Description: "Read-heavy explore mode (prefer fast model)", Group: "chat_mode"},
		{ID: RoleOrchestrator, Label: "Mission orchestrator", Description: "Primary orchestrator for missions / parallel task()", Group: "orchestrator"},
		{ID: RoleWorker, Label: "Worker", Description: "General worker turns", Group: "orchestrator"},
		{ID: RoleValidation, Label: "Validation worker", Description: "Gates / verify-style work", Group: "orchestrator"},
		{ID: RoleExplore + "_task", Label: "Subagent: explore", Description: "task(category=explore) — fast search", Group: "subagent"},
		{ID: RoleImplement, Label: "Subagent: implement", Description: "task(category=implement) — executor", Group: "subagent"},
		{ID: RoleResearch, Label: "Subagent: research", Description: "task(category=research) / librarian-like", Group: "subagent"},
		{ID: RoleVerify, Label: "Subagent: verify", Description: "task(category=verify)", Group: "subagent"},
		{ID: RoleLight, Label: "Subagent light", Description: "Light / cheap models for shallow tasks", Group: "subagent"},
		{ID: RoleMedium, Label: "Subagent medium", Description: "Balanced models", Group: "subagent"},
		{ID: RoleHeavy, Label: "Subagent heavy", Description: "Heavy / large-context models", Group: "subagent"},
	}
}

// DefaultModeModels returns empty overrides (inherit session / sticky picker).
func DefaultModeModels() ModeModelsConfig {
	return ModeModelsConfig{
		DefaultChatMode: "agent",
	}
}

// Normalize trims fields and default chat mode.
func (c *ModeModelsConfig) Normalize() {
	if c == nil {
		return
	}
	c.DefaultChatMode = normalizeChatMode(c.DefaultChatMode)
	trimRef(&c.Session)
	trimRef(&c.Agent)
	trimRef(&c.Plan)
	trimRef(&c.Mission)
	trimRef(&c.Orchestrator)
	trimRef(&c.Worker)
	trimRef(&c.Validation)
	trimRef(&c.Explore)
	trimRef(&c.Implement)
	trimRef(&c.Research)
	trimRef(&c.Verify)
	trimRef(&c.Light)
	trimRef(&c.Medium)
	trimRef(&c.Heavy)
}

func trimRef(r *ModelRef) {
	if r == nil {
		return
	}
	r.Profile = strings.TrimSpace(r.Profile)
	r.Model = strings.TrimSpace(r.Model)
}

func normalizeChatMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "plan":
		return "plan"
	case "mission":
		return "mission"
	case "explore":
		return "explore"
	case "agent", "execution", "execute", "":
		return "agent"
	default:
		return "agent"
	}
}

// Get returns the ModelRef for a role id (including explore_task → explore).
func (c ModeModelsConfig) Get(role string) ModelRef {
	id := strings.ToLower(strings.TrimSpace(role))
	switch id {
	case RoleSession:
		return c.Session
	case RoleAgent, "execution", "execute":
		return c.Agent
	case RolePlan:
		return c.Plan
	case RoleMission:
		return c.Mission
	case RoleOrchestrator:
		return c.Orchestrator
	case RoleWorker:
		return c.Worker
	case RoleValidation:
		return c.Validation
	case RoleExplore, "explore_task", "librarian":
		return c.Explore
	case RoleImplement, "executor":
		return c.Implement
	case RoleResearch:
		return c.Research
	case RoleVerify:
		return c.Verify
	case RoleLight:
		return c.Light
	case RoleMedium:
		return c.Medium
	case RoleHeavy:
		return c.Heavy
	default:
		return ModelRef{}
	}
}

// Set updates one role ref. Unknown role returns error.
func (c ModeModelsConfig) Set(role string, ref ModelRef) (ModeModelsConfig, error) {
	out := c
	out.Normalize()
	trimRef(&ref)
	id := strings.ToLower(strings.TrimSpace(role))
	switch id {
	case RoleSession:
		out.Session = ref
	case RoleAgent, "execution", "execute":
		out.Agent = ref
	case RolePlan:
		out.Plan = ref
	case RoleMission:
		out.Mission = ref
	case RoleOrchestrator:
		out.Orchestrator = ref
	case RoleWorker:
		out.Worker = ref
	case RoleValidation:
		out.Validation = ref
	case RoleExplore, "explore_task", "librarian":
		out.Explore = ref
	case RoleImplement, "executor":
		out.Implement = ref
	case RoleResearch:
		out.Research = ref
	case RoleVerify:
		out.Verify = ref
	case RoleLight:
		out.Light = ref
	case RoleMedium:
		out.Medium = ref
	case RoleHeavy:
		out.Heavy = ref
	case RoleDefaultMode:
		out.DefaultChatMode = normalizeChatMode(ref.Model)
		if out.DefaultChatMode == "" {
			out.DefaultChatMode = normalizeChatMode(ref.Profile)
		}
	default:
		return c, fmt.Errorf("unknown mode role %q", role)
	}
	return out, nil
}

// Resolve picks profile/model for a role with inheritance:
// role override → session override → empty (caller uses sticky/router default).
func (c ModeModelsConfig) Resolve(role string) ModelRef {
	c.Normalize()
	ref := c.Get(role)
	if ref.Profile == "" {
		ref.Profile = c.Session.Profile
	}
	if ref.Model == "" {
		ref.Model = c.Session.Model
	}
	return ref
}

// LoadModeModels reads mode_models from config under home.
func LoadModeModels(home string) (ModeModelsConfig, error) {
	cfg, err := LoadFromDir(home)
	if err != nil {
		return DefaultModeModels(), err
	}
	out := cfg.ModeModels
	if out.DefaultChatMode == "" && out.Session.Model == "" && out.Agent.Model == "" {
		// Absent block → defaults
		out = DefaultModeModels()
	}
	out.Normalize()
	return out, nil
}

// SaveModeModels merges mode_models into config.yaml (mode 0600).
func SaveModeModels(home string, modes ModeModelsConfig) (FileConfig, error) {
	modes.Normalize()
	cfg, err := LoadFromDir(home)
	if err != nil {
		return FileConfig{}, err
	}
	cfg.ModeModels = modes
	if err := SaveToDir(home, cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}
