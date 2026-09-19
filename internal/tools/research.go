package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/browser"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// Research tool names (VAL-CROSS-005).
// research_save is granted only while the research skill is loaded; it writes
// docs/research/<slug>.md via WriteGateway with the screenshot referenced by
// path (never inlined base64). LLM-facing tool result never contains raw
// cookie/auth (browser sanitizer + research_save scrub).
const (
	NameResearchSave = "research_save"
)

// researchSaveArgs are the model-facing arguments for research_save.
// ScreenshotPath is the path returned by browser_screenshot (relative or
// workspace-absolute). It is referenced by path in the artifact, never
// inlined as base64 into the model context.
type researchSaveArgs struct {
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	Summary        string `json:"summary"`
	ScreenshotPath string `json:"screenshot_path"`
	Findings       string `json:"findings"`
}

// researchDefs returns the research_save tool definition. It is advertised
// only when the research skill is loaded (VAL-CROSS-005, VAL-ORCH-004).
func researchDefs(r *Registry) []Definition {
	if r == nil || r.skill == nil {
		return nil
	}
	if !r.skill.HasExtraTool(NameResearchSave) && !r.skill.IsLoaded(skills.NameResearch) {
		return nil
	}
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameResearchSave,
				Description: "Save a research artifact as docs/research/<slug>.md via WriteGateway (undoable). The screenshot must be referenced by path, never inlined as base64. Cookies/auth are never written. Requires the research skill (read-only browser research scope).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": { "type": "string", "description": "URL-safe slug for the artifact filename (e.g. example-domain-overview). Becomes docs/research/<slug>.md" },
    "title": { "type": "string", "description": "Human-readable research title" },
    "url": { "type": "string", "description": "Source URL researched (the page that was navigated to)" },
    "summary": { "type": "string", "description": "1-3 sentence summary of findings" },
    "screenshot_path": { "type": "string", "description": "Path returned by browser_screenshot (workspace-relative or absolute under workspace). Referenced by path in the artifact, never inlined." },
    "findings": { "type": "string", "description": "Markdown body of key findings, quotes, and notes. No cookies/auth tokens." }
  },
  "required": ["slug", "title", "url", "summary"]
}`),
			},
		},
	}
}

// researchSlugRe restricts slugs to URL-safe characters so the artifact
// filename is predictable and cannot escape docs/research/.
var researchSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// execResearchSave writes docs/research/<slug>.md via WriteGateway and returns
// a path-only tool result (never base64, never cookies).
//
// Preconditions enforced here:
//   - research skill must be loaded (toolset gate)
//   - WriteGateway must be configured (sandbox + undo)
//   - slug must be URL-safe (no path escape)
//   - screenshot_path (when provided) must resolve inside the workspace and
//     be referenced by relative path in the artifact
//   - all fields are sanitized: cookies/auth/bearer tokens redacted
//
// The screenshot is **referenced by path**, not inlined. The tool result
// returned to the model contains only the artifact path + screenshot path
// (VAL-CROSS-005, VAL-BRW-006).
func (r *Registry) execResearchSave(call Call) Result {
	if r == nil {
		return Result{Name: NameResearchSave, Content: "research_save: registry not configured", IsError: true}
	}
	if r.skill == nil || (!r.skill.HasExtraTool(NameResearchSave) && !r.skill.IsLoaded(skills.NameResearch)) {
		return Result{
			Name:    NameResearchSave,
			Content: "research_save: load the research skill first (skill load research)",
			IsError: true,
		}
	}
	if r.gw == nil {
		return Result{Name: NameResearchSave, Content: "research_save: WriteGateway not configured", IsError: true}
	}

	var args researchSaveArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{
			Name:    NameResearchSave,
			Content: fmt.Sprintf("research_save: invalid arguments JSON: %v", err),
			IsError: true,
		}
	}

	slug := strings.ToLower(strings.TrimSpace(args.Slug))
	if slug == "" {
		return Result{Name: NameResearchSave, Content: "research_save: slug is required", IsError: true}
	}
	if !researchSlugRe.MatchString(slug) {
		return Result{
			Name:    NameResearchSave,
			Content: "research_save: slug must be URL-safe (lowercase letters, digits, hyphens; no leading/trailing hyphen)",
			IsError: true,
		}
	}

	title := strings.TrimSpace(args.Title)
	if title == "" {
		return Result{Name: NameResearchSave, Content: "research_save: title is required", IsError: true}
	}
	url := strings.TrimSpace(args.URL)
	if url == "" {
		return Result{Name: NameResearchSave, Content: "research_save: url is required", IsError: true}
	}
	summary := strings.TrimSpace(args.Summary)
	if summary == "" {
		return Result{Name: NameResearchSave, Content: "research_save: summary is required", IsError: true}
	}

	shotPathIn := strings.TrimSpace(args.ScreenshotPath)
	var shotRel string
	if shotPathIn != "" {
		// The screenshot must already exist inside the workspace (browser_screenshot
		// writes it to the artifact dir or a workspace path). Resolve and keep the
		// workspace-relative form for the artifact reference. Never inline bytes.
		absShot, relShot, err := r.gw.ResolveInWorkspace(shotPathIn)
		if err != nil {
			return Result{
				Name:    NameResearchSave,
				Content: fmt.Sprintf("research_save: screenshot_path must resolve inside the workspace: %v", err),
				IsError: true,
			}
		}
		_ = absShot
		shotRel = relShot
	}

	// Sanitize every field that will be written into the artifact (VAL-BRW-006).
	// Defense in depth: even if the model tried to paste a cookie, redact it.
	title = browser.SanitizeForLLM(title)
	url = browser.SanitizeForLLM(url)
	summary = browser.SanitizeForLLM(summary)
	findings := browser.SanitizeForLLM(strings.TrimSpace(args.Findings))

	artifactPath := filepath.ToSlash(filepath.Join("docs", "research", slug+".md"))
	body := composeResearchArtifact(title, url, summary, shotRel, findings)

	entry, err := r.gw.WriteResearch(artifactPath, []byte(body))
	if err != nil {
		if writegate.IsSandboxDenied(err) {
			return Result{
				Name: NameResearchSave,
				Content: fmt.Sprintf(
					"sandbox denied: research artifact must be written under docs/research/ (%s). Error: %v",
					r.gw.RootPath(), err,
				),
				IsError: true,
			}
		}
		return Result{Name: NameResearchSave, Content: fmt.Sprintf("research_save failed: %v", err), IsError: true}
	}

	rel := entry.Snapshot.Rel
	// Tool result to the model: path only, never base64, never cookies.
	msg := fmt.Sprintf("wrote research artifact %s via WriteGateway (source=%s, %d bytes)", rel, entry.Source, len(entry.After))
	if shotRel != "" {
		msg += fmt.Sprintf("; screenshot referenced by path=%s", shotRel)
	} else {
		msg += "; no screenshot path provided"
	}
	// Final defense-in-depth: ensure no sensitive material leaked into the
	// tool result that the model will see.
	if browser.ContainsSensitive(msg) {
		msg = browser.SanitizeForLLM(msg)
	}
	return Result{Name: NameResearchSave, Content: msg}
}

// composeResearchArtifact builds the markdown body for docs/research/<slug>.md.
// The screenshot is referenced by relative path — never inlined as base64.
// Cookies / auth / bearer tokens are already redacted by callers, but we
// run a final sanitize pass here so the on-disk artifact is clean too.
func composeResearchArtifact(title, url, summary, screenshotRel, findings string) string {
	var b strings.Builder
	// Timestamp in a stable, human-readable form (WIB-neutral UTC for determinism).
	ts := time.Now().UTC().Format("2006-01-02 15:04 MST")

	// Header
	b.WriteString("# Research: ")
	b.WriteString(browser.SanitizeForLLM(strings.TrimSpace(title)))
	b.WriteString("\n\n")

	// Metadata block
	b.WriteString("- Source URL: ")
	b.WriteString(browser.SanitizeForLLM(strings.TrimSpace(url)))
	b.WriteString("\n")
	b.WriteString("- Captured: ")
	b.WriteString(ts)
	b.WriteString("\n")
	b.WriteString("- Engine: read-only research skill (browser navigate + screenshot)\n\n")

	// Summary
	b.WriteString("## Summary\n\n")
	b.WriteString(browser.SanitizeForLLM(strings.TrimSpace(summary)))
	b.WriteString("\n\n")

	// Screenshot reference (by path, not inlined base64).
	b.WriteString("## Screenshot\n\n")
	if screenshotRel != "" {
		b.WriteString("![research screenshot](")
		b.WriteString(browser.SanitizeForLLM(strings.TrimSpace(screenshotRel)))
		b.WriteString(")\n\n")
		b.WriteString("Path: `")
		b.WriteString(browser.SanitizeForLLM(strings.TrimSpace(screenshotRel)))
		b.WriteString("`\n\n")
	} else {
		b.WriteString("_No screenshot captured for this research run._\n\n")
	}

	// Findings body (already sanitized by caller; sanitize again as defense).
	b.WriteString("## Findings\n\n")
	body := strings.TrimSpace(findings)
	if body == "" {
		body = "_No additional findings recorded._"
	}
	b.WriteString(browser.SanitizeForLLM(body))
	b.WriteString("\n")

	return b.String()
}
