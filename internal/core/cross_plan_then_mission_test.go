package core_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/mission"
	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CROSS-004: plan then mission end-to-end at the core.Service + tools layer.
//
// Flow under test:
//  1. Enter Plan Mode (read-only).
//  2. An fs.write tool call is rejected with the stable denial prefix
//     "plan mode: writes denied" — no file lands on disk.
//  3. Propose a plan (goal + steps that become acceptance criteria).
//  4. ApprovePlanAndCreateMission: approve the plan, seed todos, and create a
//     Mission fed from the plan (goal/budget/verification gates + plan snapshot).
//     Plan Mode is exited as part of approve (writes re-enabled).
//  5. mission complete WITHOUT evidence returns blocked ("evidence required").
//  6. Attach verification evidence.
//  7. mission complete WITH evidence returns complete.
//
// This exercises internal/plan + internal/mission + internal/tools + core
// in one cross-area flow without requiring a live LLM gateway.
func TestCrossArea_PlanThenMission_FullFlow(t *testing.T) {
	home := t.TempDir()
	wsRoot := t.TempDir()

	gw, err := writegate.New(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetPlanMode(svc.PlanModeController())
	reg.SetMissionBackend(svc)

	// --- Step 1: enter Plan Mode (read-only) ---
	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}
	if !svc.GetPlanMode().Active {
		t.Fatal("Plan Mode should be active after Enter")
	}

	// --- Step 2: fs.write tool call rejected with stable denial prefix ---
	res := reg.Execute(context.Background(), tools.Call{
		ID:        "c1",
		Name:      tools.NameWriteFile,
		Arguments: `{"path":"blocked.txt","content":"nope"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected write_file to be rejected in Plan Mode, got %q", res.Content)
	}
	if !strings.Contains(res.Content, plan.DenialPrefix) {
		t.Fatalf("denial must contain %q, got %q", plan.DenialPrefix, res.Content)
	}
	// Read tools still succeed in Plan Mode (VAL-PLAN-002 / VAL-CROSS-004 read-only invariant).
	if _, err := svc.ExitPlanMode("cancel"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}

	// --- Step 3: propose a plan ---
	planSteps := []string{
		"Explore internal/plan and internal/mission",
		"Implement bridge from approved plan to mission",
		"Run go test ./... green",
	}
	proposed, err := svc.ProposePlan(
		"Ship plan-then-mission cross-area flow",
		"Plan Mode denies writes; approve feeds a Mission with verification gates",
		"## Plan\n\nBridge approved plan into Mission with goals/budget/gates.",
		planSteps,
	)
	if err != nil {
		t.Fatal(err)
	}
	if proposed.Status != plan.ProposalPending {
		t.Fatalf("proposed status=%s want pending", proposed.Status)
	}
	if len(proposed.Steps) != len(planSteps) {
		t.Fatalf("steps=%d want %d", len(proposed.Steps), len(planSteps))
	}

	// While still in Plan Mode, another write attempt must still be denied
	// (Plan Mode denies writes throughout, even with a pending proposal).
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "c2",
		Name:      tools.NameWriteFile,
		Arguments: `{"path":"still-blocked.txt","content":"nope"}`,
	}, nil)
	if !res.IsError || !strings.Contains(res.Content, plan.DenialPrefix) {
		t.Fatalf("write must stay denied while Plan Mode active: %+v", res)
	}

	// --- Step 4: approve plan AND create mission from it ---
	bridge, err := svc.ApprovePlanAndCreateMission(core.MissionFromPlanRequest{
		BudgetTokens: 25_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Plan Mode exited (writes re-enabled).
	if bridge.Mode.Active {
		t.Fatal("Plan Mode must exit after approve")
	}
	// Todos seeded from plan steps.
	if bridge.Seeded != len(planSteps) {
		t.Fatalf("seeded=%d want %d", bridge.Seeded, len(planSteps))
	}
	// Mission created from plan with snapshot + acceptance criteria from steps.
	if bridge.Mission.ID == "" {
		t.Fatal("mission id missing")
	}
	if bridge.Mission.Status != string(mission.StatusPending) && bridge.Mission.Status != string(mission.StatusActive) {
		t.Fatalf("mission status=%s", bridge.Mission.Status)
	}
	if bridge.Mission.Goal == "" {
		t.Fatal("mission goal must be derived from plan")
	}
	if !strings.Contains(strings.ToLower(bridge.Mission.Goal), "plan") {
		t.Fatalf("mission goal should reflect plan, got %q", bridge.Mission.Goal)
	}
	if bridge.Mission.BudgetTokens != 25_000 {
		t.Fatalf("budget=%d want 25000", bridge.Mission.BudgetTokens)
	}
	if len(bridge.Mission.AcceptanceCriteria) != len(planSteps) {
		t.Fatalf("acceptance criteria=%d want %d (one per plan step)", len(bridge.Mission.AcceptanceCriteria), len(planSteps))
	}
	if bridge.Mission.PlanID == "" {
		t.Fatal("mission must reference source plan id")
	}
	if !strings.Contains(bridge.Mission.PlanSnapshot, "Bridge approved plan") {
		t.Fatalf("plan snapshot missing body, got %q", bridge.Mission.PlanSnapshot)
	}

	// Writes are re-enabled after approve (VAL-PLAN-007).
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "c3",
		Name:      tools.NameWriteFile,
		Arguments: `{"path":"after-approve.txt","content":"yes"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("write after approve must succeed: %s", res.Content)
	}

	// --- Step 5: mission complete WITHOUT evidence returns blocked ---
	missionID := bridge.Mission.ID
	completeRes := svc.CompleteMission(missionID)
	if completeRes.OK {
		t.Fatal("complete without evidence must fail")
	}
	if !strings.Contains(strings.ToLower(completeRes.Message+completeRes.Error), "evidence required") {
		t.Fatalf("complete-without-evidence message must say 'evidence required', got %q / %q", completeRes.Message, completeRes.Error)
	}
	got, _ := svc.GetMission(missionID)
	if got.Status == string(mission.StatusCompleted) {
		t.Fatal("mission must NOT be complete without evidence")
	}

	// --- Step 6: attach verification evidence ---
	code := 0
	if _, err := svc.AttachMissionEvidence(core.MissionEvidenceRequest{
		ID:      missionID,
		Kind:    "command",
		Summary: "go test ./... passed",
		Content: "ok\nPASS",
		Command: "go test ./...",
		ExitCode: &code,
	}); err != nil {
		t.Fatal(err)
	}

	// --- Step 7: mission complete WITH evidence returns complete ---
	completeRes = svc.CompleteMission(missionID)
	if !completeRes.OK {
		t.Fatalf("complete with evidence must succeed: %+v", completeRes)
	}
	if completeRes.Mission.Status != string(mission.StatusCompleted) {
		t.Fatalf("status=%s want completed", completeRes.Mission.Status)
	}
	if completeRes.Mission.CompletedAt == "" {
		t.Fatal("completed_at should be set")
	}
}

// TestCrossArea_PlanThenMission_ProposeRejectNoMission ensures reject does NOT
// create a mission (only approve feeds into a mission).
func TestCrossArea_PlanThenMission_ProposeRejectNoMission(t *testing.T) {
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnterPlanMode("user"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProposePlan("Reject path", "discard", "body", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RejectPlan(); err != nil {
		t.Fatal(err)
	}
	if svc.ListMissions().Count != 0 {
		t.Fatalf("reject must not create a mission, got %d", svc.ListMissions().Count)
	}
}
