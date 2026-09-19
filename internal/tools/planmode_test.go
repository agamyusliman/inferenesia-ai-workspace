package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestPlanModeWriteRejectedAllowedWhenInactive(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	st := plan.New()
	reg := tools.NewRegistry(gw)
	reg.SetPlanMode(st)

	// Inactive: write ok.
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: tools.NameWriteFile,
		Arguments: `{"path":"a.txt","content":"v1"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("inactive write: %s", res.Content)
	}

	st.Enter("test")
	res = reg.Execute(context.Background(), tools.Call{
		ID: "2", Name: tools.NameWriteFile,
		Arguments: `{"path":"b.txt","content":"blocked"}`,
	}, nil)
	if !res.IsError || !strings.Contains(res.Content, plan.DenialPrefix) {
		t.Fatalf("active write: %q", res.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err == nil {
		t.Fatal("b.txt must not exist")
	}

	// Shell denied.
	res = reg.Execute(context.Background(), tools.Call{
		ID: "3", Name: tools.NameShell,
		Arguments: `{"command":"echo no"}`,
	}, nil)
	if !res.IsError || !strings.Contains(res.Content, plan.DenialPrefix) {
		t.Fatalf("active shell: %q", res.Content)
	}

	// Read still works.
	res = reg.Execute(context.Background(), tools.Call{
		ID: "4", Name: tools.NameReadFile,
		Arguments: `{"path":"a.txt"}`,
	}, nil)
	if res.IsError || !strings.Contains(res.Content, "v1") {
		t.Fatalf("active read: %q", res.Content)
	}

	// Grep works.
	res = reg.Execute(context.Background(), tools.Call{
		ID: "5", Name: tools.NameGrep,
		Arguments: `{"pattern":"v1"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("grep: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.txt") {
		t.Fatalf("grep hits: %q", res.Content)
	}

	// Definitions filtered.
	defs := reg.Definitions()
	for _, d := range defs {
		if plan.IsWriteTool(d.Function.Name) {
			t.Fatalf("plan mode definitions still include write tool %q", d.Function.Name)
		}
	}

	st.Exit("cancel")
	res = reg.Execute(context.Background(), tools.Call{
		ID: "6", Name: tools.NameWriteFile,
		Arguments: `{"path":"c.txt","content":"after"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("after exit write: %s", res.Content)
	}
	data, _ := os.ReadFile(filepath.Join(root, "c.txt"))
	if string(data) != "after" {
		t.Fatalf("c.txt=%q", data)
	}
}
