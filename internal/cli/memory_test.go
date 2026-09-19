package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-MEM-001: workspace with .multibrain/ is recognized; absent reports no multi-brain without crash.
func TestMemoryStatusDetectsMultiBrain(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))

	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(filepath.Join(ws, ".multibrain", "indexes"), 0o755); err != nil {
		t.Fatal(err)
	}
	session := `# Multi Brain Master Index

## Buckets

- ` + "`architecture`" + ` — monorepo. Last updated: 2026-07-17 20:05 WIB -> .multibrain/indexes/architecture.md
- ` + "`agents`" + ` — workflows. Last updated: 2026-07-17 16:05 WIB -> .multibrain/indexes/agents.md
`
	if err := os.WriteFile(filepath.Join(ws, ".multibrain", "session.md"), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".multibrain", "indexes", "architecture.md"), []byte("# architecture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".multibrain", "indexes", "agents.md"), []byte("# agents\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"memory", "status", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	combined := out.String()
	if !strings.Contains(strings.ToLower(combined), "multi-brain detected") {
		t.Fatalf("want multi-brain detected, got:\n%s", combined)
	}
	if !strings.Contains(combined, "session.md") {
		t.Fatalf("want session.md mention:\n%s", combined)
	}
	if !strings.Contains(strings.ToLower(combined), "no rag") {
		t.Fatalf("want no RAG note:\n%s", combined)
	}
}

func TestMemoryStatusAbsentNoCrash(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))
	ws := filepath.Join(tmp, "empty-ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"memory", "status", "--workspace", ws}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d stderr=%q", code, errBuf.String())
	}
	if !strings.Contains(strings.ToLower(out.String()), "no multi-brain") {
		t.Fatalf("want no multi-brain, got:\n%s", out.String())
	}
}

// VAL-MEM-002: --verbose shows session.md first; not whole tree.
func TestMemoryStatusVerboseSessionFirst(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))

	ws := filepath.Join(tmp, "ws")
	mb := filepath.Join(ws, ".multibrain")
	if err := os.MkdirAll(filepath.Join(mb, "indexes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mb, "context"), 0o755); err != nil {
		t.Fatal(err)
	}
	session := `## Buckets

- ` + "`architecture`" + ` — layout -> .multibrain/indexes/architecture.md
- ` + "`agents`" + ` — notes -> .multibrain/indexes/agents.md
- ` + "`providers`" + ` — gateway -> .multibrain/indexes/providers.md
`
	if err := os.WriteFile(filepath.Join(mb, "session.md"), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"architecture.md", "agents.md", "providers.md"} {
		body := "- note -> .multibrain/context/" + strings.TrimSuffix(name, ".md") + ".md\n"
		if err := os.WriteFile(filepath.Join(mb, "indexes", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mb, "context", strings.TrimSuffix(name, ".md")+".md"), []byte("ctx"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Extra files that must not all open.
	for i := 0; i < 15; i++ {
		p := filepath.Join(mb, "context", "extra-"+itoaPad(i)+".md")
		_ = os.WriteFile(p, []byte("noise"), 0o600)
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"memory", "status", "--workspace", ws, "--verbose", "--query", "architecture"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	s := out.String()
	if !strings.Contains(s, "[memory] opened[0]:") {
		t.Fatalf("want verbose open order:\n%s", s)
	}
	// First opened path line must mention session.md
	idx := strings.Index(s, "[memory] opened[0]:")
	lineEnd := strings.Index(s[idx:], "\n")
	firstLine := s[idx : idx+lineEnd]
	if !strings.Contains(firstLine, "session.md") {
		t.Fatalf("first opened must be session.md, got %q", firstLine)
	}
	if !strings.Contains(s, "[memory] first read: session.md") {
		t.Fatalf("want first read callout:\n%s", s)
	}
	// Should not open every extra context file
	openCount := strings.Count(s, "[memory] opened[")
	if openCount > 1+2+8 { // session + buckets + max contexts
		t.Fatalf("opened too many files: count=%d\n%s", openCount, s)
	}
}

func TestHelpListsMemory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "memory") {
		t.Fatalf("help missing memory:\n%s", out.String())
	}
}

// VAL-MEM-003/004/005/011 via CLI append surface.
func TestMemoryAppendWritesBucketAndSession(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))

	ws := filepath.Join(tmp, "ws")
	mb := filepath.Join(ws, ".multibrain")
	if err := os.MkdirAll(filepath.Join(mb, "indexes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mb, "context"), 0o755); err != nil {
		t.Fatal(err)
	}
	session := `# Multi Brain Master Index

## Buckets

- ` + "`architecture`" + ` — monorepo. Last updated: 2026-07-17 20:05 WIB -> .multibrain/indexes/architecture.md
- ` + "`agents`" + ` — workflows. Last updated: 2026-07-17 16:05 WIB -> .multibrain/indexes/agents.md
`
	if err := os.WriteFile(filepath.Join(mb, "session.md"), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}
	archBefore := `# architecture

- 2026-07-17 19:05 WIB — Droid: prior -> .multibrain/context/prior.md
`
	if err := os.WriteFile(filepath.Join(mb, "indexes", "architecture.md"), []byte(archBefore), 0o600); err != nil {
		t.Fatal(err)
	}
	agentsBefore := `# agents

- old agents entry
`
	if err := os.WriteFile(filepath.Join(mb, "indexes", "agents.md"), []byte(agentsBefore), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{
		"memory", "append",
		"--workspace", ws,
		"--summary", "CLI append multibrain write path",
		"--query", "architecture",
		"--agent", "Droid",
		"--topic", "cli-append",
		"--decision", "route all MB writes via WriteGateway",
		"--file", "internal/memory/write.go",
		"--verification", "go test ./internal/memory/...",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	s := out.String()
	if !strings.Contains(s, `bucket "architecture"`) && !strings.Contains(s, "architecture") {
		t.Fatalf("want architecture bucket mention:\n%s", s)
	}
	if !strings.Contains(s, "context:") {
		t.Fatalf("want context line:\n%s", s)
	}

	archAfter, err := os.ReadFile(filepath.Join(mb, "indexes", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(archAfter), "CLI append multibrain write path") {
		t.Fatalf("architecture missing new entry:\n%s", archAfter)
	}
	if !strings.Contains(string(archAfter), "prior -> .multibrain/context/prior.md") {
		t.Fatal("prior entry lost")
	}
	// Other bucket unchanged
	agentsAfter, err := os.ReadFile(filepath.Join(mb, "indexes", "agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(agentsAfter) != agentsBefore {
		t.Fatalf("agents bucket should be unchanged:\n%s", agentsAfter)
	}
	// session Last updated refreshed
	sessionAfter, err := os.ReadFile(filepath.Join(mb, "session.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sessionAfter), "`architecture`") {
		t.Fatal("architecture line gone")
	}
	// context file created under context/
	ents, err := os.ReadDir(filepath.Join(mb, "context"))
	if err != nil {
		t.Fatal(err)
	}
	foundCtx := false
	for _, e := range ents {
		if strings.Contains(e.Name(), "cli-append") || strings.Contains(e.Name(), "droid") {
			foundCtx = true
			data, _ := os.ReadFile(filepath.Join(mb, "context", e.Name()))
			if len(data) == 0 {
				t.Fatal("empty context")
			}
			if strings.Contains(string(data), "WriteGateway") {
				// good
			}
		}
	}
	if !foundCtx {
		t.Fatalf("no context file written; dir=%v", ents)
	}
}

func itoaPad(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
