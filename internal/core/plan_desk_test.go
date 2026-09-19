package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestServiceEnterExitPlanMode(t *testing.T) {
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	st := svc.GetPlanMode()
	if st.Active {
		t.Fatal("expected inactive")
	}
	st, err = svc.EnterPlanMode("ui")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Active || st.Mode != string(plan.ModeActive) {
		t.Fatalf("enter: %+v", st)
	}
	st, err = svc.ExitPlanMode("cancel")
	if err != nil {
		t.Fatal(err)
	}
	if st.Active {
		t.Fatalf("exit: %+v", st)
	}
}

// TestPlanModeBlocksWriteFileThroughRegistry is an integration-style case
// matching VAL-PLAN-006 expectedBehaviour: write rejected when active, allowed inactive.
func TestPlanModeBlocksWriteFileThroughRegistry(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetPlanMode(svc.PlanModeController())

	// Active → write denied, no file on disk.
	if _, err := svc.EnterPlanMode("test"); err != nil {
		t.Fatal(err)
	}
	res := reg.Execute(context.Background(), tools.Call{
		ID:        "c1",
		Name:      tools.NameWriteFile,
		Arguments: `{"path":"blocked.txt","content":"nope"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected error, got %q", res.Content)
	}
	if !strings.Contains(res.Content, plan.DenialPrefix) {
		t.Fatalf("expected denial prefix, got %q", res.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "blocked.txt")); err == nil {
		t.Fatal("file must not exist after plan-mode denial")
	}

	// Read tools still work.
	_ = os.WriteFile(filepath.Join(root, "ok.txt"), []byte("hello-plan"), 0o644)
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "c2",
		Name:      tools.NameReadFile,
		Arguments: `{"path":"ok.txt"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("read should succeed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "hello-plan") {
		t.Fatalf("read content=%q", res.Content)
	}

	// Exit → write allowed via WriteGateway.
	if _, err := svc.ExitPlanMode("approve"); err != nil {
		t.Fatal(err)
	}
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "c3",
		Name:      tools.NameWriteFile,
		Arguments: `{"path":"after.txt","content":"yes"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("write after exit: %s", res.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "after.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "yes" {
		t.Fatalf("file=%q", data)
	}
	if gw.UndoDepth() < 1 {
		t.Fatal("expected WriteGateway stack entry after successful write")
	}
}
