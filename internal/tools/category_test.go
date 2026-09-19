package tools_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-ORCH-008: explore cannot call write_file; implement can; categories via task tool.
func TestCategoryToolFilterExploreDeniesWrite(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "ok.txt"), "hello")
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetCategoryFilter(orchestrator.CategoryExplore)

	// write_file denied
	res := reg.Execute(context.Background(), tools.Call{
		ID:        "1",
		Name:      tools.NameWriteFile,
		Arguments: `{"path":"x.txt","content":"nope"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("explore should deny write_file")
	}
	if !strings.Contains(res.Content, "denied") && !strings.Contains(res.Content, "read-only") {
		t.Fatalf("denial message: %s", res.Content)
	}

	// read_file allowed
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "2",
		Name:      tools.NameReadFile,
		Arguments: `{"path":"ok.txt"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("read_file should work: %s", res.Content)
	}

	// Definitions must not advertise write_file
	for _, d := range reg.Definitions() {
		if d.Function.Name == tools.NameWriteFile {
			t.Fatal("explore definitions must not include write_file")
		}
	}
}

func TestCategoryImplementAllowsWrite(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetCategoryFilter(orchestrator.CategoryImplement)

	res := reg.Execute(context.Background(), tools.Call{
		ID:        "1",
		Name:      tools.NameWriteFile,
		Arguments: `{"path":"new.txt","content":"via-gateway"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("implement write_file: %s", res.Content)
	}
}

func TestCategoryVerifyAssertions(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "f.txt"), "needle here")
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetCategoryFilter(orchestrator.CategoryVerify)

	// write denied
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: tools.NameWriteFile, Arguments: `{"path":"z.txt","content":"x"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("verify must deny write")
	}

	// assert tools allowed
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "2",
		Name:      tools.NameAssertContains,
		Arguments: `{"path":"f.txt","substring":"needle"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("assert_contains: %s", res.Content)
	}
	if !strings.Contains(res.Content, "PASS") {
		t.Fatalf("want PASS got %s", res.Content)
	}

	foundAssert := false
	for _, d := range reg.Definitions() {
		if d.Function.Name == tools.NameAssertContains {
			foundAssert = true
		}
		if d.Function.Name == tools.NameWriteFile {
			t.Fatal("verify defs include write_file")
		}
	}
	if !foundAssert {
		t.Fatal("verify defs missing assert_contains")
	}
}

func TestTaskToolCategoriesAction(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	mgr := orchestrator.New(orchestrator.Options{})
	reg.SetTaskBackend(tools.NewTaskAdapter(mgr, orchestrator.NewExploreRunnerFunc(root)))

	res := reg.Execute(context.Background(), tools.Call{
		ID: "c", Name: tools.NameTask, Arguments: `{"action":"categories"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	for _, need := range []string{"explore", "implement", "verify"} {
		if !strings.Contains(res.Content, need) {
			t.Fatalf("categories output missing %s: %s", need, res.Content)
		}
	}
}
