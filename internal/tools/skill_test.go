package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-ORCH-004: skill list metadata then skill load changes Definitions toolset.
func TestSkillListAndLoadToolsetDelta(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))

	// Before load: skill tool present; multibrain tools absent.
	before := toolNames(reg)
	if !contains(before, tools.NameSkill) {
		t.Fatal("skill tool must be advertised when backend wired")
	}
	if contains(before, tools.NameMultibrainRead) {
		t.Fatal("multibrain_read must not appear before multi-brain load")
	}
	if contains(before, tools.NameSkillStatus) {
		t.Fatal("skill_status must not appear before go-core-worker load")
	}

	// skill list
	res := reg.Execute(context.Background(), tools.Call{
		ID: "1", Name: tools.NameSkill, Arguments: `{"action":"list"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "multi-brain") {
		t.Fatalf("list missing multi-brain: %s", res.Content)
	}
	if !strings.Contains(res.Content, "metadata only") {
		t.Fatalf("list should note progressive metadata: %s", res.Content)
	}

	// load multi-brain
	res = reg.Execute(context.Background(), tools.Call{
		ID: "2", Name: tools.NameSkill, Arguments: `{"action":"load","name":"multi-brain"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "loaded skill") {
		t.Fatalf("load output: %s", res.Content)
	}

	after := toolNames(reg)
	if !contains(after, tools.NameMultibrainRead) {
		t.Fatalf("after load toolset missing multibrain_read: %v", after)
	}
	if !contains(after, tools.NameMultibrainAppend) {
		t.Fatalf("after load toolset missing multibrain_append: %v", after)
	}

	// Prompt action shows overlay
	res = reg.Execute(context.Background(), tools.Call{
		ID: "3", Name: tools.NameSkill, Arguments: `{"action":"prompt"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "session.md") {
		t.Fatalf("prompt overlay missing protocol: %s", res.Content)
	}

	// go-core adds skill_status
	res = reg.Execute(context.Background(), tools.Call{
		ID: "4", Name: tools.NameSkill, Arguments: `{"action":"load","name":"go-core-worker"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	after2 := toolNames(reg)
	if !contains(after2, tools.NameSkillStatus) {
		t.Fatalf("skill_status missing after go-core-worker: %v", after2)
	}
}

// VAL-ORCH-005: multibrain_read/append tools respect protocol after skill load.
func TestMultibrainToolsProtocol(t *testing.T) {
	root := t.TempDir()
	writeMB(t, root)
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	// Poison tree
	for i := 0; i < 20; i++ {
		_ = os.WriteFile(filepath.Join(root, ".multibrain", "indexes", "x"+itoa(i)+".md"), []byte("- n\n"), 0o600)
		_ = os.WriteFile(filepath.Join(root, ".multibrain", "context", "c"+itoa(i)+".md"), []byte("n"), 0o600)
	}

	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	if _, err := loader.Load("multi-brain"); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))

	// Denied without... actually skill is loaded
	res := reg.Execute(context.Background(), tools.Call{
		ID: "r", Name: tools.NameMultibrainRead, Arguments: `{"query":"architecture"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "session.md") && !strings.Contains(res.Content, "[0]") {
		t.Fatalf("read summary: %s", res.Content)
	}
	// Open count line present and small
	if strings.Contains(res.Content, "opened 30") || strings.Contains(res.Content, "opened 40") {
		t.Fatalf("opened too many: %s", res.Content)
	}

	res = reg.Execute(context.Background(), tools.Call{
		ID: "a", Name: tools.NameMultibrainAppend,
		Arguments: `{"summary":"test append via tool","agent":"Droid","query":"architecture","decision":"ok","topic":"t"}`,
	}, nil)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content, "format_ok=true") {
		t.Fatalf("append: %s", res.Content)
	}
	if !strings.Contains(res.Content, "WIB") {
		t.Fatalf("timestamp missing: %s", res.Content)
	}
	if !strings.Contains(res.Content, "-> .multibrain/context/") && !strings.Contains(res.Content, "context:") {
		t.Fatalf("pointer missing: %s", res.Content)
	}
}

func TestMultibrainToolsDeniedWithoutSkill(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	// Do not load multi-brain — tools not in Definitions; Execute should error if forced.
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))

	if contains(toolNames(reg), tools.NameMultibrainRead) {
		t.Fatal("multibrain_read should not be in Definitions without skill")
	}
	// Even if called, handler denies
	res := reg.Execute(context.Background(), tools.Call{
		ID: "x", Name: tools.NameMultibrainRead, Arguments: `{}`,
	}, nil)
	if !res.IsError {
		t.Fatal("expected error without skill load")
	}
}

func toolNames(reg *tools.Registry) []string {
	var out []string
	for _, d := range reg.Definitions() {
		out = append(out, d.Function.Name)
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func writeMB(t *testing.T, root string) {
	t.Helper()
	mb := filepath.Join(root, ".multibrain")
	_ = os.MkdirAll(filepath.Join(mb, "indexes"), 0o755)
	_ = os.MkdirAll(filepath.Join(mb, "context"), 0o755)
	session := `# Multi Brain

## Buckets

- ` + "`architecture`" + ` — arch. Last updated: 2026-07-18 10:00 WIB -> .multibrain/indexes/architecture.md
- ` + "`agents`" + ` — agents. Last updated: 2026-07-17 16:00 WIB -> .multibrain/indexes/agents.md
`
	_ = os.WriteFile(filepath.Join(mb, "session.md"), []byte(session), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "indexes", "architecture.md"), []byte("# architecture\n\n## Entries\n\n- 2026-07-18 10:00 WIB — Droid: prior -> .multibrain/context/p.md\n"), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "indexes", "agents.md"), []byte("# agents\n\n## Entries\n\n"), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "context", "p.md"), []byte("# p\n"), 0o600)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digs []byte
	for i > 0 {
		digs = append([]byte{byte('0' + i%10)}, digs...)
		i /= 10
	}
	return string(digs)
}
