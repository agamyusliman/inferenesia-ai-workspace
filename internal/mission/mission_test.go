package mission_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/mission"
)

// VAL-MISSION-001 / create: goal + optional budget → list with pending status.
func TestCreateMissionAppearsInList(t *testing.T) {
	st := mission.NewStore()
	m, err := st.Create("Ship prepaid pack API", 50_000, "ws-1", []string{"tests green"})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" {
		t.Fatal("expected id")
	}
	if m.Status != mission.StatusPending {
		t.Fatalf("status=%s want pending", m.Status)
	}
	if m.Goal != "Ship prepaid pack API" {
		t.Fatalf("goal=%q", m.Goal)
	}
	if m.BudgetTokens != 50_000 {
		t.Fatalf("budget=%d", m.BudgetTokens)
	}
	list := st.List()
	if len(list) != 1 || list[0].ID != m.ID {
		t.Fatalf("list=%+v", list)
	}
}

// VAL-MISSION-002 / VAL-MISSION-007(a): complete without evidence is rejected.
func TestCompleteRejectedWithoutEvidence(t *testing.T) {
	st := mission.NewStore()
	m, err := st.Create("No evidence mission", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Complete(m.ID)
	if err == nil {
		t.Fatal("expected complete without evidence to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "evidence required") {
		t.Fatalf("error should mention evidence required: %v", err)
	}
	got, err := st.Get(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == mission.StatusCompleted {
		t.Fatal("status must remain non-complete")
	}
}

// VAL-MISSION-003: complete succeeds after evidence attached.
func TestCompleteWithEvidenceSucceeds(t *testing.T) {
	st := mission.NewStore()
	m, err := st.Create("With evidence", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	code := 0
	m, err = st.AttachEvidence(m.ID, "command", "go test ./... ok", "PASS\nok", "go test ./...", &code, "go_test")
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasEvidence() {
		t.Fatal("expected evidence")
	}
	m, err = st.Complete(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != mission.StatusCompleted {
		t.Fatalf("status=%s", m.Status)
	}
	if m.CompletedAt == nil {
		t.Fatal("expected completed_at")
	}
}

// VAL-MISSION-004 / VAL-MISSION-007(b): blocked is settable and queryable.
func TestBlockedStatusSettableAndQueryable(t *testing.T) {
	st := mission.NewStore()
	m, err := st.Create("Will block", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err = st.SetBlocked(m.ID, "missing TEMP_AI gateway (key absent)")
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != mission.StatusBlocked {
		t.Fatalf("status=%s", m.Status)
	}
	if !strings.Contains(m.BlockerReason, "missing") {
		t.Fatalf("blocker=%q", m.BlockerReason)
	}
	got, err := st.Get(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != mission.StatusBlocked || got.BlockerReason == "" {
		t.Fatalf("queryable blocked failed: %+v", got)
	}
}

// VAL-MISSION-005: failing gate prevents completion.
func TestFailingGatePreventsCompletion(t *testing.T) {
	st := mission.NewStore()
	m, err := st.Create("Gate fail", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Attach evidence so only gate blocks complete.
	if _, err := st.AttachEvidence(m.ID, "note", "manual note", "looks good", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	m, err = st.RecordGate(m.ID, mission.GateResult{
		Name:     "go_test",
		Command:  "go test ./...",
		Passed:   false,
		ExitCode: 1,
		Output:   "--- FAIL: TestX",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != mission.StatusBlocked {
		t.Fatalf("status after fail gate=%s want blocked", m.Status)
	}
	_, err = st.Complete(m.ID)
	if err == nil {
		t.Fatal("complete must fail when gate failed")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "gate") {
		t.Fatalf("error=%v", err)
	}
}

// VAL-MISSION-005: passing gates + evidence allow complete.
func TestPassingGatesAllowComplete(t *testing.T) {
	st := mission.NewStore()
	m, err := st.Create("Gates green", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AttachEvidence(m.ID, "gate", "all green", "PASS", "go test ./...", intPtr(0), "go_test"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go_test", "go_vet", "typecheck_build"} {
		if _, err := st.RecordGate(m.ID, mission.GateResult{
			Name: name, Command: mission.GateCommandLine(name), Passed: true, ExitCode: 0, Output: "ok",
		}); err != nil {
			t.Fatal(err)
		}
	}
	m, err = st.Complete(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != mission.StatusCompleted {
		t.Fatalf("status=%s", m.Status)
	}
}

// VAL-MISSION-008: evidence content is redacted (no API keys / bearer).
func TestEvidenceRedactsSecrets(t *testing.T) {
	st := mission.NewStore()
	m, err := st.Create("Secret hygiene", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	leaky := "TEMP_AI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz123456 Bearer secret-token-value"
	m, err = st.AttachEvidence(m.ID, "note", "gate log", leaky, "echo leak", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Evidence) != 1 {
		t.Fatalf("evidence=%+v", m.Evidence)
	}
	ev := m.Evidence[0]
	blob := ev.Summary + " " + ev.Content + " " + ev.Command
	if strings.Contains(blob, "sk-abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("secret key leaked into evidence: %q", blob)
	}
	if strings.Contains(blob, "TEMP_AI_API_KEY=sk-") {
		t.Fatalf("key assignment leaked: %q", blob)
	}
	if strings.Contains(strings.ToLower(blob), "bearer secret-token-value") {
		t.Fatalf("bearer leaked: %q", blob)
	}
	// Redacted form should appear or key patterns gone.
	if !strings.Contains(ev.Content, "[REDACTED]") && strings.Contains(leaky, "sk-") {
		// secrethyg may redact sk- to [REDACTED]
		t.Logf("content after redact: %q", ev.Content)
	}
}

// Gate runner with injected RunFunc (no real go test required).
func TestGateRunnerRecordsPassFail(t *testing.T) {
	r := &mission.GateRunner{
		WorkDir: t.TempDir(),
		RunFunc: func(ctx context.Context, dir, name string, args []string) (int, string, error) {
			if len(args) > 0 && args[0] == "test" {
				return 1, "FAIL", nil
			}
			return 0, "ok", nil
		},
	}
	fail := r.RunGate(context.Background(), "go_test")
	if fail.Passed {
		t.Fatal("go_test should fail")
	}
	pass := r.RunGate(context.Background(), "go_vet")
	if !pass.Passed {
		t.Fatal("go_vet should pass")
	}
}

func intPtr(n int) *int { return &n }
