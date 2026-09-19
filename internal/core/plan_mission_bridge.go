package core

import (
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
)

// MissionFromPlanRequest drives ApprovePlanAndCreateMission: it approves the
// pending Plan Mode proposal and creates a Mission fed from that plan
// (goal/budget/acceptance criteria/verification gates + plan snapshot).
//
// This is the cross-area bridge VAL-CROSS-004 requires: a plan produced in
// read-only Plan Mode, once approved, feeds a Mission with goals/budget/
// verification gates; Plan Mode denies writes throughout, and Mission
// complete requires verification evidence.
type MissionFromPlanRequest struct {
	// BudgetTokens is the optional mission token budget (0 = unlimited).
	BudgetTokens int64 `json:"budget_tokens,omitempty"`
	// GoalOverride, when non-empty, replaces the plan-derived goal. When
	// empty, the goal is derived from the approved plan title + summary.
	GoalOverride string `json:"goal_override,omitempty"`
}

// PlanMissionBridgeResult is returned by ApprovePlanAndCreateMission so the
// UI / CLI / validator can observe the full cross-area outcome in one shot:
// the approved plan, seeded todos, exited Plan Mode, and the created Mission.
type PlanMissionBridgeResult struct {
	Plan    PlanProposalView  `json:"plan"`
	Todos   TodoListView      `json:"todos"`
	Mode    PlanModeState     `json:"mode"`
	Mission MissionView       `json:"mission"`
	Seeded  int               `json:"seeded"`
	Hub     HubState          `json:"hub"`
}

// ApprovePlanAndCreateMission is the cross-area bridge (VAL-CROSS-004):
//
//  1. Approve the pending Plan Mode proposal (seeds todos with pending status).
//  2. Exit Plan Mode so write tools are re-enabled (writes were denied
//     throughout Plan Mode).
//  3. Create a Mission fed from the approved plan: goal derived from the
//     plan title/summary, acceptance criteria from the plan steps, plan
//     snapshot captured, optional budget from the request.
//
// Returns the bridge result. Mission complete still requires verification
// evidence (CompleteMission) — the bridge only creates the mission; it does
// not mark it complete.
func (s *Service) ApprovePlanAndCreateMission(req MissionFromPlanRequest) (PlanMissionBridgeResult, error) {
	if s == nil {
		return PlanMissionBridgeResult{}, fmt.Errorf("core: service is nil")
	}
	pt := s.ensurePlanTodos()
	// 1. Approve the pending proposal (seeds todos pending, returns approved plan).
	approved, err := pt.proposals.Approve()
	if err != nil {
		return PlanMissionBridgeResult{}, fmt.Errorf("core: approve plan: %w", err)
	}
	seeded := pt.todos.SeedPending(approved.Steps)
	s.emitTodoUpdated(pt, seeded)

	// 2. Exit Plan Mode (writes re-enabled). ApprovePlan exits with reason
	//    "approve"; we mirror that here.
	mode, _ := s.ExitPlanMode("approve")

	// 3. Derive mission goal + acceptance criteria from the approved plan.
	goal := missionGoalFromPlan(approved, req.GoalOverride)
	acceptance := missionAcceptanceFromPlan(approved)
	snapshot := missionPlanSnapshot(approved)

	// Workspace id: active workspace if any.
	wsID := ""
	if active := s.GetActiveWorkspace(); active != nil {
		wsID = active.ID
	}

	ms := s.ensureMissions()
	m, err := ms.store.CreateFromPlan(goal, req.BudgetTokens, wsID, acceptance, approved.ID, snapshot)
	if err != nil {
		// Mission creation failed after approve; surface a clear error but the
		// approve + todo seed already happened (they are separate concerns).
		return PlanMissionBridgeResult{
			Plan:   *proposalToView(&approved),
			Todos:  TodoListView{Todos: seeded, Count: len(seeded), Product: s.BrandName()},
			Mode:   mode,
			Seeded: len(seeded),
			Hub:    s.GetHubState(),
		}, fmt.Errorf("core: create mission from plan: %w", err)
	}
	s.emitMissionStatus(m)
	s.setStatus(fmt.Sprintf("Plan approved → Mission %s created (from plan %s)", shortID(m.ID), shortID(approved.ID)))

	return PlanMissionBridgeResult{
		Plan:    *proposalToView(&approved),
		Todos:   TodoListView{Todos: seeded, Count: len(seeded), Product: s.BrandName()},
		Mode:    mode,
		Mission: missionToView(m, s.BrandName()),
		Seeded:  len(seeded),
		Hub:     s.GetHubState(),
	}, nil
}

// missionGoalFromPlan derives the Mission goal from the approved plan.
// GoalOverride wins; otherwise prefer the plan title; fall back to summary;
// finally a stable default so goal is never empty.
func missionGoalFromPlan(p plan.ProposedPlan, override string) string {
	if g := strings.TrimSpace(override); g != "" {
		return g
	}
	if t := strings.TrimSpace(p.Title); t != "" {
		return t
	}
	if sm := strings.TrimSpace(p.Summary); sm != "" {
		return sm
	}
	return "Mission from approved plan"
}

// missionAcceptanceFromPlan turns plan steps into mission acceptance criteria
// (one criterion per step). Steps are already trimmed/non-empty by ProposalStore.Set.
func missionAcceptanceFromPlan(p plan.ProposedPlan) []string {
	out := make([]string, 0, len(p.Steps))
	for _, step := range p.Steps {
		if s := strings.TrimSpace(step); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// missionPlanSnapshot captures the approved plan body (and summary header) so
// the mission always carries the plan that fed it (docs/plan-mission.md
// "plan_snapshot"). Empty body falls back to summary then title.
func missionPlanSnapshot(p plan.ProposedPlan) string {
	if b := strings.TrimSpace(p.Body); b != "" {
		if sm := strings.TrimSpace(p.Summary); sm != "" {
			return sm + "\n\n" + b
		}
		return b
	}
	if sm := strings.TrimSpace(p.Summary); sm != "" {
		return sm
	}
	return strings.TrimSpace(p.Title)
}
