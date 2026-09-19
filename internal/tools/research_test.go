package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CROSS-005: skill load research -> browser navigate + screenshot ->
// research_save writes docs/research/<slug>.md via WriteGateway; file exists,
// references screenshot path; LLM-facing tool result does not contain raw
// cookie/auth (per VAL-BRW-006).
//
// This test exercises the cross-area flow end-to-end at the tool layer using a
// stub browser backend (no real browser process required). It verifies:
//  1. Before research skill load: research_save is NOT advertised.
//  2. After skill load: research_save IS advertised (toolset delta).
//  3. browser_navigate + browser_screenshot produce a screenshot file path.
//  4. research_save writes docs/research/<slug>.md via WriteGateway (undoable).
//  5. The artifact references the screenshot by path, not inlined base64.
//  6. The LLM-facing tool result contains no raw cookie/auth.
//  7. Undo restores the pre-research state (VAL-UNDO-007).
func TestCrossSkillBrowserResearchFlow(t *testing.T) {
	root := t.TempDir()
	// Pre-create docs/research so WriteResearch can write (MkdirAll handles this
	// anyway, but the test asserts the directory is workspace-scoped).
	_ = os.MkdirAll(filepath.Join(root, "docs", "research"), 0o755)

	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}

	// Wire a stub browser backend that writes a real PNG file to the artifact
	// dir, mimicking ManagerBrowserBackend.Screenshot. The stub resolves
	// relative paths against the workspace root (as the real backend does
	// when the CLI runs with cwd=workspace root).
	mb := &researchStubBrowser{
		workspaceRoot: root,
		artifactsDir:  filepath.Join(root, "docs", "research", "shots"),
		shotBytes:     []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3},
	}
	_ = os.MkdirAll(mb.artifactsDir, 0o755)

	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))
	reg.SetBrowserBackend(mb)

	// (1) research_save must NOT be advertised before skill load.
	for _, d := range reg.Definitions() {
		if d.Function.Name == tools.NameResearchSave {
			t.Fatalf("research_save must not be advertised before skill load")
		}
	}

	// Load the research skill — this grants research_save and scopes the
	// browser to read-only research (VAL-CROSS-005).
	if _, err := loader.Load(skills.NameResearch); err != nil {
		t.Fatalf("load research skill: %v", err)
	}
	if !loader.IsLoaded(skills.NameResearch) {
		t.Fatal("research skill not loaded")
	}

	// (2) research_save IS advertised after skill load.
	advertised := map[string]bool{}
	for _, d := range reg.Definitions() {
		advertised[d.Function.Name] = true
	}
	if !advertised[tools.NameResearchSave] {
		t.Fatalf("research_save must be advertised after skill load; got: %v", advertised)
	}
	// Browser read-only tools must also be available (navigate + screenshot).
	if !advertised[tools.NameBrowserNavigate] {
		t.Fatal("browser_navigate must be advertised when browser backend wired")
	}
	if !advertised[tools.NameBrowserScreenshot] {
		t.Fatal("browser_screenshot must be advertised when browser backend wired")
	}

	ctx := context.Background()

	// (3) browser_navigate to a fixture URL.
	navURL := "data:text/html,<html><head><title>Example Research</title></head><body><h1>Hello</h1></body></html>"
	navRes := reg.Execute(ctx, tools.Call{
		ID:        "nav",
		Name:      tools.NameBrowserNavigate,
		Arguments: `{"url":"` + navURL + `"}`,
	}, nil)
	if navRes.IsError {
		t.Fatalf("navigate failed: %s", navRes.Content)
	}

	// browser_screenshot — returns path + bytes (never base64).
	shotArgs := `{"path":"docs/research/shots/example.png"}`
	shotRes := reg.Execute(ctx, tools.Call{
		ID:        "shot",
		Name:      tools.NameBrowserScreenshot,
		Arguments: shotArgs,
	}, nil)
	if shotRes.IsError {
		t.Fatalf("screenshot failed: %s", shotRes.Content)
	}
	// LLM-facing screenshot result must reference path, not base64.
	if strings.Contains(shotRes.Content, "iVBOR") || strings.Contains(strings.ToLower(shotRes.Content), "base64") {
		t.Fatalf("screenshot result must not inline base64: %s", shotRes.Content)
	}
	if !strings.Contains(shotRes.Content, "screenshot_path=") && !strings.Contains(shotRes.Content, ".png") {
		t.Fatalf("screenshot result must reference path: %s", shotRes.Content)
	}
	// Screenshot file must exist on disk.
	shotPath := filepath.Join(root, "docs", "research", "shots", "example.png")
	if _, err := os.Stat(shotPath); err != nil {
		t.Fatalf("screenshot file not on disk: %v", err)
	}

	// (4) research_save writes docs/research/<slug>.md via WriteGateway.
	saveArgs := `{"slug":"example-domain-overview","title":"Example Domain Overview","url":"https://example.com","summary":"A brief overview of the example domain.","screenshot_path":"docs/research/shots/example.png","findings":"The page title is 'Example Research'. Key heading: Hello."}`
	saveRes := reg.Execute(ctx, tools.Call{
		ID:        "save",
		Name:      tools.NameResearchSave,
		Arguments: saveArgs,
	}, nil)
	if saveRes.IsError {
		t.Fatalf("research_save failed: %s", saveRes.Content)
	}

	// (5) The artifact file exists and references the screenshot by path.
	artifactPath := filepath.Join(root, "docs", "research", "example-domain-overview.md")
	body, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("artifact not written: %v", err)
	}
	artBody := string(body)
	if !strings.Contains(artBody, "Example Domain Overview") {
		t.Fatalf("artifact missing title: %s", artBody)
	}
	if !strings.Contains(artBody, "https://example.com") {
		t.Fatalf("artifact missing url: %s", artBody)
	}
	if !strings.Contains(artBody, "docs/research/shots/example.png") {
		t.Fatalf("artifact must reference screenshot by path: %s", artBody)
	}
	// Must NOT inline base64 image bytes.
	if strings.Contains(artBody, "iVBOR") || strings.Contains(artBody, "data:image/png;base64") {
		t.Fatalf("artifact must not inline base64 screenshot: %s", artBody)
	}

	// (6) The LLM-facing research_save tool result contains no raw cookie/auth.
	if strings.Contains(saveRes.Content, "cookie=") && !strings.Contains(saveRes.Content, "[REDACTED]") {
		t.Fatalf("research_save result leaked cookie: %s", saveRes.Content)
	}
	if browser.ContainsSensitive(saveRes.Content) {
		t.Fatalf("research_save result contains sensitive material: %s", saveRes.Content)
	}

	// (7) Undo restores pre-research state (VAL-UNDO-007: research write is
	// undoable through WriteGateway, source=research).
	undoRes := gw.Undo()
	if undoRes.Noop {
		t.Fatalf("undo was a no-op: %s", undoRes.Message)
	}
	// After undo, the artifact file should be gone (it did not exist pre-research).
	if _, err := os.Stat(artifactPath); err == nil {
		t.Fatalf("artifact must be removed after undo: %s", artifactPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error after undo: %v", err)
	}
	// Screenshot file remains (browser_screenshot wrote it directly, not via
	// WriteGateway — that's expected; the artifact is the WriteGateway mutation).
}

// TestResearchSaveDeniedWithoutSkill verifies that research_save errors when
// the research skill is not loaded (toolset gate).
func TestResearchSaveDeniedWithoutSkill(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))
	reg.SetBrowserBackend(&researchStubBrowser{})

	// Not loading research skill — research_save must not be advertised.
	for _, d := range reg.Definitions() {
		if d.Function.Name == tools.NameResearchSave {
			t.Fatal("research_save must not be advertised without skill load")
		}
	}

	// Even if the model forces a call, the handler denies.
	res := reg.Execute(context.Background(), tools.Call{
		ID:        "x",
		Name:      tools.NameResearchSave,
		Arguments: `{"slug":"x","title":"x","url":"https://x","summary":"x"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("research_save must error without skill load: %s", res.Content)
	}
	if !strings.Contains(res.Content, "research skill") {
		t.Fatalf("error must mention research skill: %s", res.Content)
	}
}

// TestResearchSaveSanitizesCookies verifies that even if the model tries to
// pass cookies/auth in research_save arguments, they are redacted from both
// the on-disk artifact and the LLM-facing tool result (VAL-BRW-006).
func TestResearchSaveSanitizesCookies(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	if _, err := loader.Load(skills.NameResearch); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))

	// Inject cookie + bearer into findings and summary. Use clearly-fake
	// placeholder secrets that still trigger the sanitizer regex (cookie
	// assignment + bearer token >= 8 chars) but are not real credentials.
	args := `{"slug":"leaky-research","title":"Leak Test","url":"https://example.com","summary":"Summary with cookie sessionid=fakesecret123","findings":"Authorization: Bearer fakebearersecret12345678\ncookie: auth=fakeauth456"}`
	res := reg.Execute(context.Background(), tools.Call{
		ID:        "s",
		Name:      tools.NameResearchSave,
		Arguments: args,
	}, nil)
	if res.IsError {
		t.Fatalf("research_save failed: %s", res.Content)
	}
	// LLM-facing tool result must not contain the raw secret.
	if strings.Contains(res.Content, "fakesecret123") {
		t.Fatalf("tool result leaked sessionid value: %s", res.Content)
	}
	if strings.Contains(res.Content, "fakebearersecret12345678") {
		t.Fatalf("tool result leaked bearer token: %s", res.Content)
	}

	// On-disk artifact must also be sanitized.
	artifactPath := filepath.Join(root, "docs", "research", "leaky-research.md")
	body, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("artifact not written: %v", err)
	}
	artBody := string(body)
	if strings.Contains(artBody, "fakesecret123") {
		t.Fatalf("artifact leaked sessionid value: %s", artBody)
	}
	if strings.Contains(artBody, "fakebearersecret12345678") {
		t.Fatalf("artifact leaked bearer token: %s", artBody)
	}
	// Redaction markers expected.
	if !strings.Contains(artBody, "[REDACTED]") {
		t.Fatalf("artifact should contain redaction markers: %s", artBody)
	}
}

// TestResearchSaveSlugValidation verifies slug must be URL-safe and cannot
// escape docs/research/.
func TestResearchSaveSlugValidation(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	if _, err := loader.Load(skills.NameResearch); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))

	for _, bad := range []string{
		`{"slug":"../escape","title":"x","url":"https://x","summary":"x"}`,
		`{"slug":"a/b","title":"x","url":"https://x","summary":"x"}`,
		`{"slug":"with space","title":"x","url":"https://x","summary":"x"}`,
		`{"slug":"-leading","title":"x","url":"https://x","summary":"x"}`,
		`{"slug":"trailing-","title":"x","url":"https://x","summary":"x"}`,
		`{"slug":"under_score","title":"x","url":"https://x","summary":"x"}`,
		`{"slug":"dot.in","title":"x","url":"https://x","summary":"x"}`,
	} {
		res := reg.Execute(context.Background(), tools.Call{
			ID: "x", Name: tools.NameResearchSave, Arguments: bad,
		}, nil)
		if !res.IsError {
			t.Fatalf("expected slug validation error for %q: %s", bad, res.Content)
		}
	}
}

// TestResearchSaveScreenshotPathMustResolveInsideWorkspace verifies the
// screenshot_path must resolve inside the workspace sandbox.
func TestResearchSaveScreenshotPathMustResolveInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewDefaultLoader(t.TempDir(), root, nil)
	if _, err := loader.Load(skills.NameResearch); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	reg.SetSkillBackend(tools.NewSkillBackend(loader))

	res := reg.Execute(context.Background(), tools.Call{
		ID: "x", Name: tools.NameResearchSave,
		Arguments: `{"slug":"ok","title":"x","url":"https://x","summary":"x","screenshot_path":"/etc/passwd"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("expected sandbox deny for screenshot_path outside workspace: %s", res.Content)
	}
}

// researchStubBrowser is a stub BrowserBackend that writes a real PNG-like
// file to the requested path so the research flow can reference it by path.
// Relative paths are resolved against workspaceRoot (mirroring real backend
// behavior when the CLI cwd is the workspace root).
type researchStubBrowser struct {
	workspaceRoot string
	artifactsDir  string
	shotBytes     []byte
	navigated     string
	closed        bool
}

func (s *researchStubBrowser) SetEngine(ctx context.Context, id string, headless *bool) error {
	return nil
}
func (s *researchStubBrowser) Status() (string, string, string, int, bool, string) {
	return "e2e", "ready", "", 0, false, ""
}
func (s *researchStubBrowser) Navigate(ctx context.Context, url string) error {
	if err := browser.ValidateNavigateURL(url); err != nil {
		return err
	}
	s.navigated = url
	return nil
}
func (s *researchStubBrowser) Screenshot(ctx context.Context, outPath string) (string, int, error) {
	path := outPath
	if path == "" {
		path = filepath.Join(s.artifactsDir, "shot.png")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.workspaceRoot, path)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, s.shotBytes, 0o600); err != nil {
		return "", 0, err
	}
	return path, len(s.shotBytes), nil
}
func (s *researchStubBrowser) Evaluate(ctx context.Context, expression string) (string, error) {
	return browser.SanitizeForLLM(`{"title":"Example Research"}`), nil
}
func (s *researchStubBrowser) Snapshot(ctx context.Context) (string, error) {
	return browser.PageTextResultForLLM("Hello page text"), nil
}
func (s *researchStubBrowser) WaitHuman(ctx context.Context, reason string, minMs, maxMs int) (int, error) {
	return 1, nil
}
func (s *researchStubBrowser) Close(ctx context.Context) error {
	s.closed = true
	return nil
}
func (s *researchStubBrowser) ArtifactDir() string { return s.artifactsDir }
