package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/mission"
)

// Mission tools (model-facing, limited set per docs/plan-mission.md).
const (
	NameGetGoal    = "get_goal"
	NameCreateGoal = "create_goal"
	NameUpdateGoal = "update_goal"
	// Attach verification evidence so complete can succeed (VAL-MISSION-003).
	NameAttachMissionEvidence = "attach_mission_evidence"
)

// MissionBackend is implemented by core.Service mission store.
type MissionBackend interface {
	CreateGoal(objective string, budgetTokens int64) (mission.Mission, error)
	GetGoal(id string) (mission.Mission, error)
	UpdateGoalComplete(id string) (mission.Mission, error)
	UpdateGoalBlocked(id, reason string) (mission.Mission, error)
	AttachGoalEvidence(id, kind, summary, content, command string, exitCode *int) (mission.Mission, error)
}

// SetMissionBackend wires create_goal / get_goal / update_goal / attach evidence.
func (r *Registry) SetMissionBackend(mb MissionBackend) {
	if r == nil {
		return
	}
	r.mission = mb
}

// MissionBackend returns the attached backend (may be nil).
func (r *Registry) MissionBackend() MissionBackend {
	if r == nil {
		return nil
	}
	return r.mission
}

func missionDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name: NameGetGoal,
				Description: "Get the current or specified mission/goal (id, goal, status, blocker, gates, evidence count). " +
					"Read-only. Use before update_goal.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "Optional mission id; omit for latest non-completed" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name: NameCreateGoal,
				Description: "Create a new mission/goal with an objective and optional token budget. " +
					"Initial status is pending. Prefer one unfinished goal at a time for long work.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "objective": { "type": "string", "description": "Mission goal text" },
    "budget_tokens": { "type": "integer", "description": "Optional token budget (0 = unlimited)" }
  },
  "required": ["objective"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name: NameUpdateGoal,
				Description: "Update a mission status to complete or blocked only. " +
					"complete is REJECTED without verification evidence (error contains 'evidence required'). " +
					"blocked requires a reason. Pause/resume/budget are not available to the model.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "Mission id" },
    "status": { "type": "string", "enum": ["complete", "blocked"], "description": "Only complete or blocked" },
    "reason": { "type": "string", "description": "Required when status=blocked" }
  },
  "required": ["id", "status"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name: NameAttachMissionEvidence,
				Description: "Attach verification evidence to a mission (test output, command result, gate log). " +
					"Required before complete. Secrets (API keys, tokens) are redacted automatically.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": { "type": "string", "description": "Mission id" },
    "kind": { "type": "string", "description": "command | note | gate | test | screenshot" },
    "summary": { "type": "string", "description": "Short description of the evidence" },
    "content": { "type": "string", "description": "Log/output body (will be redacted)" },
    "command": { "type": "string", "description": "Optional command that produced the output" },
    "exit_code": { "type": "integer", "description": "Optional process exit code" }
  },
  "required": ["id", "summary"]
}`),
			},
		},
	}
}

type createGoalArgs struct {
	Objective    string `json:"objective"`
	BudgetTokens int64  `json:"budget_tokens"`
}

type getGoalArgs struct {
	ID string `json:"id"`
}

type updateGoalArgs struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type attachEvidenceArgs struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Summary  string `json:"summary"`
	Content  string `json:"content"`
	Command  string `json:"command"`
	ExitCode *int   `json:"exit_code"`
}

func (r *Registry) execGetGoal(call Call) Result {
	if r.mission == nil {
		return Result{Name: NameGetGoal, Content: "get_goal: not configured", IsError: true}
	}
	var args getGoalArgs
	_ = json.Unmarshal([]byte(call.Arguments), &args)
	m, err := r.mission.GetGoal(args.ID)
	if err != nil {
		return Result{Name: NameGetGoal, Content: err.Error(), IsError: true}
	}
	b, _ := json.Marshal(m)
	return Result{Name: NameGetGoal, Content: string(b)}
}

func (r *Registry) execCreateGoal(call Call) Result {
	if r.mission == nil {
		return Result{Name: NameCreateGoal, Content: "create_goal: not configured", IsError: true}
	}
	var args createGoalArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameCreateGoal, Content: fmt.Sprintf("create_goal: invalid arguments: %v", err), IsError: true}
	}
	if strings.TrimSpace(args.Objective) == "" {
		return Result{Name: NameCreateGoal, Content: "create_goal: objective is required", IsError: true}
	}
	m, err := r.mission.CreateGoal(args.Objective, args.BudgetTokens)
	if err != nil {
		return Result{Name: NameCreateGoal, Content: err.Error(), IsError: true}
	}
	return Result{
		Name: NameCreateGoal,
		Content: fmt.Sprintf(
			"MissionCreated id=%s status=%s goal=%q budget=%d — attach verification evidence before update_goal(complete).",
			m.ID, m.Status, m.Goal, m.BudgetTokens,
		),
	}
}

func (r *Registry) execUpdateGoal(call Call) Result {
	if r.mission == nil {
		return Result{Name: NameUpdateGoal, Content: "update_goal: not configured", IsError: true}
	}
	var args updateGoalArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameUpdateGoal, Content: fmt.Sprintf("update_goal: invalid arguments: %v", err), IsError: true}
	}
	if strings.TrimSpace(args.ID) == "" {
		return Result{Name: NameUpdateGoal, Content: "update_goal: id is required", IsError: true}
	}
	st := strings.ToLower(strings.TrimSpace(args.Status))
	switch st {
	case "complete", "completed":
		m, err := r.mission.UpdateGoalComplete(args.ID)
		if err != nil {
			// Surface evidence-required / gate messages to the model (VAL-MISSION-002).
			return Result{Name: NameUpdateGoal, Content: err.Error(), IsError: true}
		}
		return Result{
			Name:    NameUpdateGoal,
			Content: fmt.Sprintf("MissionCompleted id=%s status=%s", m.ID, m.Status),
		}
	case "blocked", "block":
		if strings.TrimSpace(args.Reason) == "" {
			return Result{Name: NameUpdateGoal, Content: "update_goal: reason is required when status=blocked", IsError: true}
		}
		m, err := r.mission.UpdateGoalBlocked(args.ID, args.Reason)
		if err != nil {
			return Result{Name: NameUpdateGoal, Content: err.Error(), IsError: true}
		}
		return Result{
			Name:    NameUpdateGoal,
			Content: fmt.Sprintf("MissionBlocked id=%s reason=%q", m.ID, m.BlockerReason),
		}
	default:
		return Result{
			Name:    NameUpdateGoal,
			Content: "update_goal: status must be complete or blocked (pause/resume/budget are not model-facing)",
			IsError: true,
		}
	}
}

func (r *Registry) execAttachMissionEvidence(call Call) Result {
	if r.mission == nil {
		return Result{Name: NameAttachMissionEvidence, Content: "attach_mission_evidence: not configured", IsError: true}
	}
	var args attachEvidenceArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameAttachMissionEvidence, Content: fmt.Sprintf("attach_mission_evidence: invalid arguments: %v", err), IsError: true}
	}
	if strings.TrimSpace(args.ID) == "" {
		return Result{Name: NameAttachMissionEvidence, Content: "attach_mission_evidence: id is required", IsError: true}
	}
	if strings.TrimSpace(args.Summary) == "" {
		return Result{Name: NameAttachMissionEvidence, Content: "attach_mission_evidence: summary is required", IsError: true}
	}
	m, err := r.mission.AttachGoalEvidence(args.ID, args.Kind, args.Summary, args.Content, args.Command, args.ExitCode)
	if err != nil {
		return Result{Name: NameAttachMissionEvidence, Content: err.Error(), IsError: true}
	}
	return Result{
		Name: NameAttachMissionEvidence,
		Content: fmt.Sprintf(
			"EvidenceAttached mission=%s count=%d status=%s",
			m.ID, len(m.Evidence), m.Status,
		),
	}
}
