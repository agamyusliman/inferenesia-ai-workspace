package plan_test

import (
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
)

// TestWriteToolRejectedWhenPlanModeActive covers VAL-PLAN-006:
// write tools are rejected while Plan Mode is active and allowed when inactive.
func TestWriteToolRejectedWhenPlanModeActive(t *testing.T) {
	st := plan.New()
	if st.Active() {
		t.Fatal("expected inactive by default")
	}

	// Inactive: write tools allowed.
	if err := st.AllowTool("write_file"); err != nil {
		t.Fatalf("inactive should allow write_file: %v", err)
	}
	if err := st.AllowTool("shell"); err != nil {
		t.Fatalf("inactive should allow shell: %v", err)
	}
	if err := st.AllowTool("git_commit"); err != nil {
		t.Fatalf("inactive should allow git_commit: %v", err)
	}

	st.Enter("user")
	if !st.Active() {
		t.Fatal("expected active after Enter")
	}
	if st.Mode() != plan.ModeActive {
		t.Fatalf("mode=%s", st.Mode())
	}

	for _, name := range []string{"write_file", "shell", "git_commit", "git_stage", "write_multibrain"} {
		err := st.AllowTool(name)
		if err == nil {
			t.Fatalf("plan mode active: expected denial for %q", name)
		}
		if !strings.Contains(err.Error(), plan.DenialPrefix) {
			t.Fatalf("denial for %q must contain %q, got %q", name, plan.DenialPrefix, err.Error())
		}
		if !strings.Contains(strings.ToLower(err.Error()), "writes denied") {
			t.Fatalf("denial for %q should say writes denied: %q", name, err.Error())
		}
	}
}

// TestReadToolsAllowedWhenPlanModeActive covers VAL-PLAN-002 / VAL-PLAN-006 read set.
func TestReadToolsAllowedWhenPlanModeActive(t *testing.T) {
	st := plan.New()
	st.Enter("user")

	for _, name := range []string{
		"read_file", "grep", "list_dir",
		"git_status", "git_diff", "git_log", "git_branch_list",
		"terminal_read", "terminal_list",
		"propose_plan", "todo_list",
	} {
		if err := st.AllowTool(name); err != nil {
			t.Fatalf("plan mode should allow read tool %q: %v", name, err)
		}
		if !plan.IsReadTool(name) {
			t.Fatalf("%q should classify as read", name)
		}
	}
	// todo_write / todo_update remain write while plan mode active.
	for _, name := range []string{"todo_write", "todo_update"} {
		if err := st.AllowTool(name); err == nil {
			t.Fatalf("plan mode should deny %q", name)
		}
	}
}

// TestExitPlanModeReEnablesWrites covers VAL-PLAN-007 at the state machine layer.
func TestExitPlanModeReEnablesWrites(t *testing.T) {
	st := plan.New()
	st.Enter("user")
	if err := st.AllowTool("write_file"); err == nil {
		t.Fatal("expected denial while active")
	}

	st.Exit("cancel")
	if st.Active() {
		t.Fatal("expected inactive after Exit")
	}
	if err := st.AllowTool("write_file"); err != nil {
		t.Fatalf("after exit write_file must be allowed: %v", err)
	}
	if err := st.AllowTool("git_commit"); err != nil {
		t.Fatalf("after exit git_commit must be allowed: %v", err)
	}

	// Approve path also exits.
	st.Enter("user")
	st.Exit("approve")
	if err := st.AllowTool("shell"); err != nil {
		t.Fatalf("after approve exit shell must be allowed: %v", err)
	}
}

func TestClassifyToolKnownSets(t *testing.T) {
	writes := []string{
		"write_file", "write_multibrain", "write_diagram", "shell",
		"git_stage", "git_unstage", "git_commit", "git_push", "git_pull", "git_fetch",
		"git_checkout", "git_reset_hard", "git_branch_delete",
		"terminal_write", "terminal_start", "terminal_stop",
		"create_diagram", "update_diagram", "export_diagram", "convert_diagram",
	}
	for _, n := range writes {
		if !plan.IsWriteTool(n) {
			t.Fatalf("%q should be write", n)
		}
	}
	reads := []string{
		"read_file", "grep", "git_status", "git_diff", "git_log", "git_branch_list",
		"terminal_read", "terminal_list", "list_dir",
	}
	for _, n := range reads {
		if !plan.IsReadTool(n) {
			t.Fatalf("%q should be read", n)
		}
	}
	// Unknown defaults fail-closed (write).
	if !plan.IsWriteTool("mystery_tool") {
		t.Fatal("unknown tool should be treated as write (fail-closed)")
	}
}

func TestFilterDefinitions(t *testing.T) {
	type def struct{ Name string }
	defs := []def{
		{Name: "write_file"},
		{Name: "read_file"},
		{Name: "git_status"},
		{Name: "git_commit"},
		{Name: "grep"},
	}
	got := plan.FilterDefinitions(true, defs, func(d def) string { return d.Name })
	if len(got) != 3 {
		t.Fatalf("expected 3 read defs, got %d: %+v", len(got), got)
	}
	for _, d := range got {
		if plan.IsWriteTool(d.Name) {
			t.Fatalf("filtered set still has write %q", d.Name)
		}
	}
	// Inactive: pass-through.
	all := plan.FilterDefinitions(false, defs, func(d def) string { return d.Name })
	if len(all) != len(defs) {
		t.Fatalf("inactive filter should keep all, got %d", len(all))
	}
}

func TestSnapshot(t *testing.T) {
	st := plan.New()
	snap := st.Snapshot()
	if snap.Active || snap.Mode != string(plan.ModeInactive) {
		t.Fatalf("inactive snapshot: %+v", snap)
	}
	st.Enter("ui")
	snap = st.Snapshot()
	if !snap.Active || snap.Mode != string(plan.ModeActive) || snap.Reason != "ui" {
		t.Fatalf("active snapshot: %+v", snap)
	}
}

func TestDenialMessageStablePrefix(t *testing.T) {
	msg := plan.DenialMessage("write_file")
	if !strings.HasPrefix(msg, plan.DenialPrefix) {
		t.Fatalf("message=%q", msg)
	}
}
