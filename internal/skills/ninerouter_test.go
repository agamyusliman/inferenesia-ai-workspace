package skills_test

import (
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/skills"
)

// VAL-KIT-001: 9router pack appears in discovery as metadata only (no body dump).
func TestNineRouterPackDiscoveryMetadataOnly(t *testing.T) {
	list, err := skills.Discover(skills.DiscoverOptions{
		Home:           t.TempDir(),
		Workspace:      t.TempDir(),
		IncludeBuiltin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]skills.Meta{}
	for _, m := range list {
		byName[m.Name] = m
	}
	for _, name := range skills.NineRouterPackNames() {
		m, ok := byName[name]
		if !ok {
			t.Fatalf("missing pack skill %q in discovery", name)
		}
		if m.Source != skills.SourceBuiltin {
			t.Fatalf("%s source=%s want builtin", name, m.Source)
		}
		if m.Description == "" {
			t.Fatalf("%s empty description", name)
		}
		// Progressive: list Meta must not carry full procedure bodies.
		if strings.Contains(m.Description, "POST $NINEROUTER") {
			t.Fatalf("%s description leaked body content: %q", name, m.Description)
		}
		if m.Path != "" {
			t.Fatalf("%s should be pure builtin (path empty), path=%q", name, m.Path)
		}
	}
	// Must mention 9router-chat or kit-style ids without requiring Qdrant.
	if _, ok := byName[skills.NameNineRouterChat]; !ok {
		t.Fatal("expected 9router-chat in skill list")
	}
	if _, ok := byName[skills.NameNineRouterEmbeddings]; !ok {
		t.Fatal("expected 9router-embeddings (no Qdrant hard dep)")
	}
}

// VAL-KIT-002: loading a 9router skill changes toolset/prompt; not loading leaves tools off.
func TestNineRouterSkillLoadOptInToolset(t *testing.T) {
	loader := skills.NewDefaultLoader(t.TempDir(), t.TempDir(), nil)

	// Before load: no 9router tools.
	for _, tool := range []string{
		skills.ToolNineRouterImage,
		skills.ToolNineRouterEmbeddings,
		skills.ToolNineRouterWebSearch,
		skills.ToolNineRouterWebFetch,
		skills.ToolNineRouterSTT,
	} {
		if loader.HasExtraTool(tool) {
			t.Fatalf("tool %s must not be granted before skill load", tool)
		}
	}
	if overlay := loader.SystemPromptOverlay(); overlay != "" {
		t.Fatalf("overlay should be empty before load, got %q", overlay)
	}

	// Load image skill → tool + prompt delta.
	sk, err := loader.Load(skills.NameNineRouterImage)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != skills.NameNineRouterImage {
		t.Fatalf("name=%s", sk.Name)
	}
	if !loader.HasExtraTool(skills.ToolNineRouterImage) {
		t.Fatalf("after load tools=%v", loader.AllowedToolsUnion())
	}
	if !loader.HasExtraTool(skills.ToolNineRouterImage) {
		t.Fatal("ninerouter_image should be granted")
	}
	// Other pack tools stay off until their skill is loaded.
	if loader.HasExtraTool(skills.ToolNineRouterWebSearch) {
		t.Fatal("web_search tool should not appear without 9router-web-search load")
	}
	overlay := loader.SystemPromptOverlay()
	if !strings.Contains(overlay, "9Router") && !strings.Contains(overlay, "9router") {
		t.Fatalf("overlay missing 9Router content:\n%s", overlay)
	}
	if !strings.Contains(overlay, "NINEROUTER") {
		t.Fatalf("overlay should document NINEROUTER env:\n%s", overlay)
	}
	// Image pack must not hard-require vector infra.
	if strings.Contains(strings.ToLower(sk.Body), "must install qdrant") ||
		strings.Contains(strings.ToLower(sk.Body), "requires qdrant") {
		t.Fatal("image skill must not require Qdrant")
	}

	// Unload removes tool.
	if !loader.Unload(skills.NameNineRouterImage) {
		t.Fatal("unload failed")
	}
	if loader.HasExtraTool(skills.ToolNineRouterImage) {
		t.Fatal("ninerouter_image should be gone after unload")
	}

	// Embeddings skill does not force Qdrant/enowx-rag (VAL-KIT-007 partial).
	emb, err := loader.Load(skills.NameNineRouterEmbeddings)
	if err != nil {
		t.Fatal(err)
	}
	bodyLower := strings.ToLower(emb.Body)
	if strings.Contains(bodyLower, "must install qdrant") ||
		strings.Contains(bodyLower, "must install enowx-rag") ||
		strings.Contains(bodyLower, "requires qdrant") {
		t.Fatal("embeddings skill must not hard-require Qdrant/enowx-rag")
	}
	// Explicit opt-out language in pack docs.
	if !strings.Contains(bodyLower, "qdrant") || !strings.Contains(bodyLower, "does not") {
		t.Fatalf("embeddings body should disclaim Qdrant/enowx-rag:\n%s", emb.Body)
	}
	if !loader.HasExtraTool(skills.ToolNineRouterEmbeddings) {
		t.Fatal("embeddings tool not granted after load")
	}

	// Chat skill: prompt only, no extra tool (provider profile path).
	loader.ClearLoaded()
	chat, err := loader.Load(skills.NameNineRouterChat)
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.AllowedTools) != 0 {
		t.Fatalf("chat skill should not grant agent tools, got %v", chat.AllowedTools)
	}
	if !strings.Contains(loader.SystemPromptOverlay(), "openai_compatible") {
		t.Fatalf("chat skill should document openai_compatible profile:\n%s", loader.SystemPromptOverlay())
	}
	if !strings.Contains(loader.SystemPromptOverlay(), "NINEROUTER_URL") {
		t.Fatalf("chat skill should document NINEROUTER_URL:\n%s", loader.SystemPromptOverlay())
	}
	// Still no ninerouter_* tools after chat-only load.
	if loader.HasExtraTool(skills.ToolNineRouterImage) {
		t.Fatal("chat load must not expose image tool")
	}
}

// List after pack present does not activate skills (progressive).
func TestNineRouterListDoesNotActivate(t *testing.T) {
	loader := skills.NewDefaultLoader(t.TempDir(), t.TempDir(), nil)
	list, err := loader.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if strings.HasPrefix(m.Name, "9router-") {
			found = true
		}
	}
	if !found {
		t.Fatal("list missing 9router pack entries")
	}
	if len(loader.Loaded()) != 0 {
		t.Fatal("list must not activate skills")
	}
}
