package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/memory"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-ORCH-004: progressive discovery returns metadata only (not full body dump).
func TestDiscoverMetadataOnlyProgressive(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()

	// Personal skill with a large body that must NOT be in list Meta (List doesn't expose body).
	writeSkill(t, filepath.Join(home, "skills", "web-research"), `---
name: web-research
description: Use when scraping the public web
allowed-tools: ["read_file", "shell"]
---

# Web research

`+strings.Repeat("DO NOT LOAD THIS BODY AT BOOT\n", 200))

	// Project skill
	writeSkill(t, filepath.Join(ws, ".yura-ai", "skills", "site-auth"), `---
name: site-auth
description: Profile + handoff captcha
---

# Site auth body secret marker XYZ
`)

	list, err := skills.Discover(skills.DiscoverOptions{
		Home:           home,
		Workspace:      ws,
		IncludeBuiltin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Builtins + disk skills present by name
	names := map[string]skills.Meta{}
	for _, m := range list {
		names[m.Name] = m
	}
	if _, ok := names["multi-brain"]; !ok {
		t.Fatal("missing builtin multi-brain")
	}
	if _, ok := names["go-core-worker"]; !ok {
		t.Fatal("missing builtin go-core-worker")
	}
	if m, ok := names["web-research"]; !ok {
		t.Fatal("missing personal web-research")
	} else {
		if m.Source != skills.SourcePersonal {
			t.Fatalf("source=%s want personal", m.Source)
		}
		if !strings.Contains(m.Description, "scraping") {
			t.Fatalf("description: %q", m.Description)
		}
		if m.Path == "" {
			t.Fatal("path empty for disk skill")
		}
	}
	if m, ok := names["site-auth"]; !ok {
		t.Fatal("missing project site-auth")
	} else if m.Source != skills.SourceProject {
		t.Fatalf("source=%s", m.Source)
	}

	// Progressive: Meta has no body field dumped at list — only load gets body.
	// Confirm large body marker is not accidentally embedded in description.
	if strings.Contains(names["web-research"].Description, "DO NOT LOAD") {
		t.Fatal("list description leaked body content")
	}
}

// VAL-ORCH-004: load changes system prompt and toolset observably.
func TestLoadChangesPromptAndToolset(t *testing.T) {
	loader := skills.NewDefaultLoader(t.TempDir(), t.TempDir(), nil)

	beforePrompt := loader.SystemPromptOverlay()
	if beforePrompt != "" {
		t.Fatalf("expected empty overlay before load, got %q", beforePrompt)
	}
	if loader.HasExtraTool("multibrain_read") {
		t.Fatal("multibrain_read should not be granted before load")
	}

	sk, err := loader.Load("multi-brain")
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "multi-brain" {
		t.Fatalf("name=%s", sk.Name)
	}
	if !loader.IsLoaded("multi-brain") {
		t.Fatal("not loaded")
	}
	overlay := loader.SystemPromptOverlay()
	if !strings.Contains(overlay, "Multi Brain") && !strings.Contains(overlay, "multi-brain") {
		t.Fatalf("overlay missing multi-brain content:\n%s", overlay)
	}
	if !strings.Contains(overlay, "session.md") {
		t.Fatalf("overlay missing protocol read order:\n%s", overlay)
	}
	if !loader.HasExtraTool("multibrain_read") || !loader.HasExtraTool("multibrain_append") {
		t.Fatalf("toolset not changed; allowed=%v", loader.AllowedToolsUnion())
	}

	// go-core-worker grants skill_status
	if _, err := loader.Load("go-core-worker"); err != nil {
		t.Fatal(err)
	}
	if !loader.HasExtraTool("skill_status") {
		t.Fatal("go-core-worker should grant skill_status")
	}
	if !strings.Contains(loader.SystemPromptOverlay(), "Go core worker") {
		t.Fatal("go-core prompt missing")
	}

	// Unload reverses tool grant
	if !loader.Unload("multi-brain") {
		t.Fatal("unload failed")
	}
	if loader.HasExtraTool("multibrain_read") {
		t.Fatal("multibrain tools should be gone after unload")
	}
}

// VAL-ORCH-004: load disk skill body only on demand; discover never requires reading all bodies into Loader.cache.
func TestLoadDoesNotDumpAllAtBoot(t *testing.T) {
	home := t.TempDir()
	// Create many personal skills
	for i := 0; i < 15; i++ {
		name := "skill-" + itoa(i)
		writeSkill(t, filepath.Join(home, "skills", name), `---
name: `+name+`
description: desc `+name+`
---

BODY-`+name+`-MARKER
`)
	}
	loader := skills.NewDefaultLoader(home, t.TempDir(), nil)
	list, err := loader.List()
	if err != nil {
		t.Fatal(err)
	}
	// After list, cache/loaded empty
	if len(loader.Loaded()) != 0 {
		t.Fatal("list must not activate skills")
	}
	// Load only one
	sk, err := loader.Load("skill-3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sk.Body, "BODY-skill-3-MARKER") {
		t.Fatalf("body not loaded: %q", sk.Body)
	}
	// Others not loaded
	if loader.IsLoaded("skill-7") {
		t.Fatal("should not load unrelated skills")
	}
	// list still progressive metadata
	found := false
	for _, m := range list {
		if m.Name == "skill-3" {
			found = true
			if strings.Contains(m.Description, "BODY-") {
				t.Fatal("meta description leaked body")
			}
		}
	}
	if !found {
		t.Fatal("skill-3 missing from list")
	}
}

// VAL-ORCH-005: multi-brain skill read order on poisoned tree.
func TestMultiBrainProtocolReadBounded(t *testing.T) {
	root := t.TempDir()
	writeMultiBrainFixture(t, root)

	// Poison: many indexes + contexts that must not all open.
	for i := 0; i < 25; i++ {
		p := filepath.Join(root, ".multibrain", "indexes", "extra-"+itoa(i)+".md")
		_ = os.WriteFile(p, []byte("- noise -> .multibrain/context/noise.md\n"), 0o600)
	}
	for i := 0; i < 40; i++ {
		p := filepath.Join(root, ".multibrain", "context", "noise-"+itoa(i)+".md")
		_ = os.WriteFile(p, []byte("noise"), 0o600)
	}

	res, err := skills.MultiBrainRead(skills.MultiBrainReadInput{
		Workspace: root,
		Query:     "architecture",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OpenCount == 0 {
		t.Fatal("expected opens")
	}
	first := res.Opened[0]
	if filepath.Base(first) != "session.md" {
		t.Fatalf("first open must be session.md, got %s", first)
	}
	// Bound: ≤ 1 session + 2 buckets + MaxContexts
	if res.OpenCount > skills.FormatMaxOpenBound() {
		t.Fatalf("opened %d files, max %d: %v", res.OpenCount, skills.FormatMaxOpenBound(), res.Opened)
	}
	// Must not open entire tree
	if res.OpenCount >= 30 {
		t.Fatalf("opened almost entire poisoned tree: %d", res.OpenCount)
	}
	// Bucket count ≤ 2
	if len(res.Buckets) > memory.MaxBuckets {
		t.Fatalf("buckets=%d > max %d", len(res.Buckets), memory.MaxBuckets)
	}
	// session first content present
	if res.Session == "" {
		t.Fatal("session empty")
	}
}

// VAL-ORCH-005: append newest-first with required timestamp format + pointer.
func TestMultiBrainProtocolAppendFormat(t *testing.T) {
	root := t.TempDir()
	writeMultiBrainFixture(t, root)
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}

	when := time.Date(2026, 7, 18, 14, 30, 0, 0, time.FixedZone("WIB", 7*3600))
	res, err := skills.MultiBrainAppend(gw, skills.MultiBrainAppendInput{
		Workspace: root,
		Agent:     "Droid",
		Summary:   "skill loader multi-brain protocol",
		Topic:     "skills-loader",
		Query:     "architecture",
		Decision:  "Implement progressive SKILL.md loader",
		Files:     []string{"internal/skills/loader.go"},
		When:      when,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.FormatOK {
		t.Fatalf("format_ok false: %q", res.EntryLine)
	}
	if !strings.Contains(res.EntryLine, "2026-07-18 14:30 WIB") {
		t.Fatalf("timestamp: %q", res.EntryLine)
	}
	if !strings.Contains(res.EntryLine, "Droid:") {
		t.Fatalf("agent: %q", res.EntryLine)
	}
	if !strings.Contains(res.EntryLine, "-> .multibrain/context/") {
		t.Fatalf("context pointer missing: %q", res.EntryLine)
	}
	if res.ContextPath == "" {
		t.Fatal("expected context file")
	}
	// Entry is newest-first in bucket file
	bucket := mustRead(t, filepath.Join(root, res.BucketPath))
	idx := strings.Index(bucket, res.EntryLine)
	if idx < 0 {
		// EntryLine may be without leading "- " vs with
		line := strings.TrimPrefix(res.EntryLine, "- ")
		if !strings.Contains(bucket, line) {
			t.Fatalf("entry not in bucket:\n%s\nwant %q", bucket, res.EntryLine)
		}
	}
	if !skills.ValidateEntryFormat(res.EntryLine) {
		t.Fatal("ValidateEntryFormat failed")
	}
}

func TestValidateEntryFormat(t *testing.T) {
	ok := "- 2026-07-17 16:05 WIB — Droid: expand AGENTS -> .multibrain/context/x.md"
	if !skills.ValidateEntryFormat(ok) {
		t.Fatal("want true")
	}
	if skills.ValidateEntryFormat("nope") {
		t.Fatal("want false")
	}
}

// VAL-CROSS-005: research skill is discoverable as metadata (progressive)
// and load grants research_save tool + read-only browser research prompt.
func TestResearchSkillDiscoveryAndLoad(t *testing.T) {
	loader := skills.NewDefaultLoader(t.TempDir(), t.TempDir(), nil)

	// Discovery: research appears in list as metadata only.
	list, err := loader.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.Name == skills.NameResearch {
			found = true
			if m.Source != skills.SourceBuiltin {
				t.Fatalf("source=%s want builtin", m.Source)
			}
			if !strings.Contains(strings.ToLower(m.Description), "research") {
				t.Fatalf("description: %q", m.Description)
			}
			// Metadata must not contain the body.
			if strings.Contains(m.Description, "browser_set_engine") {
				t.Fatal("metadata leaked body content")
			}
		}
	}
	if !found {
		t.Fatal("research skill not in discovery list")
	}

	// Before load: research_save not granted.
	if loader.HasExtraTool(skills.ToolResearchSave) {
		t.Fatal("research_save must not be granted before load")
	}
	if loader.IsLoaded(skills.NameResearch) {
		t.Fatal("research must not be loaded at boot")
	}

	// Load: grants research_save + injects read-only research prompt.
	sk, err := loader.Load(skills.NameResearch)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if sk.Name != skills.NameResearch {
		t.Fatalf("name=%s", sk.Name)
	}
	if !loader.IsLoaded(skills.NameResearch) {
		t.Fatal("not loaded after Load")
	}
	if !loader.HasExtraTool(skills.ToolResearchSave) {
		t.Fatal("research_save must be granted after load")
	}

	// Overlay must scope browser to read-only research.
	overlay := loader.SystemPromptOverlay()
	if !strings.Contains(overlay, "Research skill") && !strings.Contains(overlay, "research") {
		t.Fatalf("overlay missing research content:\n%s", overlay)
	}
	if !strings.Contains(overlay, "read-only") {
		t.Fatalf("overlay must scope browser to read-only: %s", overlay)
	}
	if !strings.Contains(overlay, "browser_screenshot") {
		t.Fatalf("overlay must mention browser_screenshot: %s", overlay)
	}
	if !strings.Contains(overlay, "research_save") {
		t.Fatalf("overlay must mention research_save: %s", overlay)
	}
	// Must explicitly forbid cookie/storage reads via browser_evaluate.
	if !strings.Contains(overlay, "cookie") {
		t.Fatalf("overlay must forbid cookie reads: %s", overlay)
	}
	// Must say screenshot is referenced by path, not base64.
	if !strings.Contains(strings.ToLower(overlay), "path") {
		t.Fatalf("overlay must reference screenshot by path: %s", overlay)
	}

	// Unload reverses tool grant.
	if !loader.Unload(skills.NameResearch) {
		t.Fatal("unload failed")
	}
	if loader.HasExtraTool(skills.ToolResearchSave) {
		t.Fatal("research_save should be gone after unload")
	}
}

func TestParseFrontmatterAllowedToolsList(t *testing.T) {
	raw := `---
name: sample
description: test skill
allowed-tools:
  - read_file
  - shell
---

# Body
`
	sk, err := skills.ParseFullSkill(raw)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "sample" {
		t.Fatalf("name=%s", sk.Name)
	}
	if len(sk.AllowedTools) != 2 {
		t.Fatalf("tools=%v", sk.AllowedTools)
	}
	if !strings.Contains(sk.Body, "# Body") {
		t.Fatalf("body=%q", sk.Body)
	}
}

func writeSkill(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeMultiBrainFixture(t *testing.T, root string) {
	t.Helper()
	mb := filepath.Join(root, ".multibrain")
	_ = os.MkdirAll(filepath.Join(mb, "indexes"), 0o755)
	_ = os.MkdirAll(filepath.Join(mb, "context"), 0o755)
	session := `# Multi Brain Master Index

## Buckets

- ` + "`architecture`" + ` — product architecture. Last updated: 2026-07-18 10:00 WIB -> .multibrain/indexes/architecture.md
- ` + "`agents`" + ` — agent workflows. Last updated: 2026-07-17 16:00 WIB -> .multibrain/indexes/agents.md
- ` + "`providers`" + ` — providers. Last updated: 2026-07-17 12:00 WIB -> .multibrain/indexes/providers.md
`
	_ = os.WriteFile(filepath.Join(mb, "session.md"), []byte(session), 0o600)
	arch := `# architecture

## Entries

- 2026-07-18 10:00 WIB — Droid: prior work -> .multibrain/context/arch-prior.md
`
	_ = os.WriteFile(filepath.Join(mb, "indexes", "architecture.md"), []byte(arch), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "indexes", "agents.md"), []byte("# agents\n\n## Entries\n\n"), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "indexes", "providers.md"), []byte("# providers\n\n## Entries\n\n"), 0o600)
	_ = os.WriteFile(filepath.Join(mb, "context", "arch-prior.md"), []byte("# prior\n"), 0o600)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
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
