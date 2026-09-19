package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-ORCH-004: CLI skill list + skill load.
func TestSkillListAndLoadCLI(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	t.Setenv("YURA_AI_HOME", home)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	// Personal disk skill under home
	dir := filepath.Join(home, "skills", "demo-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: demo-skill
description: Demo for CLI list
allowed-tools: ["read_file"]
---

# Demo skill body UNIQUE_MARKER
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// list
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"skill", "list", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("list exit %d: %s", code, errBuf.String())
	}
	listOut := out.String()
	if !strings.Contains(listOut, "multi-brain") {
		t.Fatalf("list missing multi-brain:\n%s", listOut)
	}
	if !strings.Contains(listOut, "demo-skill") {
		t.Fatalf("list missing demo-skill:\n%s", listOut)
	}
	if !strings.Contains(listOut, "metadata only") {
		t.Fatalf("list should say metadata only:\n%s", listOut)
	}
	// Body not dumped on list
	if strings.Contains(listOut, "UNIQUE_MARKER") {
		t.Fatal("list dumped skill body")
	}

	// load multi-brain → toolset delta
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"skill", "load", "multi-brain", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("load exit %d: %s", code, errBuf.String())
	}
	loadOut := out.String()
	if !strings.Contains(loadOut, "loaded: multi-brain") {
		t.Fatalf("load output:\n%s", loadOut)
	}
	if !strings.Contains(loadOut, "multibrain_read") {
		t.Fatalf("load should show multibrain tools:\n%s", loadOut)
	}
	if !strings.Contains(loadOut, "toolset_after_load") {
		t.Fatalf("missing toolset_after_load:\n%s", loadOut)
	}
	if !strings.Contains(loadOut, "system_prompt_overlay_bytes") {
		t.Fatalf("missing overlay bytes:\n%s", loadOut)
	}

	// load go-core-worker
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"skill", "load", "go-core-worker", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("load go-core exit %d: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "skill_status") && !strings.Contains(out.String(), "Go core") {
		t.Fatalf("go-core load:\n%s", out.String())
	}
}

func TestSkillHelpInRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
	if !strings.Contains(out.String(), "skill") {
		t.Fatalf("root help missing skill:\n%s", out.String())
	}
}

// VAL-ORCH-005: CLI multibrain-read respects open order + bounds.
func TestSkillMultibrainReadCLI(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))
	ws := filepath.Join(tmp, "ws")
	writeCLIMultiBrain(t, ws)

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{
		"skill", "multibrain-read",
		"--workspace", ws,
		"--query", "architecture",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	s := out.String()
	if !strings.Contains(s, "session.md") {
		t.Fatalf("expected session.md in output:\n%s", s)
	}
	if !strings.Contains(s, "[0]") {
		t.Fatalf("expected open order:\n%s", s)
	}
}

func writeCLIMultiBrain(t *testing.T, ws string) {
	t.Helper()
	mb := filepath.Join(ws, ".multibrain")
	_ = os.MkdirAll(filepath.Join(mb, "indexes"), 0o755)
	_ = os.MkdirAll(filepath.Join(mb, "context"), 0o755)
	session := `# Multi Brain

## Buckets

- ` + "`architecture`" + ` — arch. Last updated: 2026-07-18 10:00 WIB -> .multibrain/indexes/architecture.md
- ` + "`agents`" + ` — agents. Last updated: 2026-07-17 16:00 WIB -> .multibrain/indexes/agents.md
`
	_ = os.WriteFile(filepath.Join(mb, "session.md"), []byte(session), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "indexes", "architecture.md"), []byte("# architecture\n\n## Entries\n\n"), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "indexes", "agents.md"), []byte("# agents\n\n## Entries\n\n"), 0o600)
}
