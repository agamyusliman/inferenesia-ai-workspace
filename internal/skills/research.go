package skills

import "strings"

// Research skill (VAL-CROSS-005).
//
// Loading this skill scopes the browser engines to **read-only research**
// behavior: navigate + screenshot + snapshot only. The skill grants the
// research_save tool which writes a markdown artifact under docs/research/
// via WriteGateway with the screenshot referenced by **path** (never inlined
// base64 into the model context). Cookies / auth tokens are never returned
// to the model (browser sanitizer + research_save never echoes raw page
// state). The skill never grants browser_evaluate beyond the snapshot path
// and explicitly forbids browser shell/exec bridges (VAL-BRW-009).

// NameResearch is the builtin research skill id.
const NameResearch = "research"

// ToolResearchSave is granted while the research skill is loaded.
// It writes a markdown research artifact under docs/research/<slug>.md via
// WriteGateway (undoable, SourceResearch). The screenshot must be referenced
// by path, not inlined base64.
const ToolResearchSave = "research_save"

// researchBody is the system-prompt overlay injected when the research skill
// is loaded. It scopes the browser to read-only research and prescribes the
// artifact format (screenshot referenced by path).
const researchBody = `# Research skill (read-only browser research)

Use this skill when the user wants to investigate a topic on the web and save
a research artifact with cited sources and a screenshot.

## Browser scope: READ-ONLY research

While this skill is loaded, the browser engines are scoped to **read-only
research**:

- Allowed browser tools: browser_set_engine, browser_navigate, browser_screenshot,
  browser_snapshot, browser_status, browser_wait_human, browser_close.
- Do **not** use browser_evaluate to read document.cookie, localStorage,
  sessionStorage, or any auth/storage state. Use browser_snapshot for page
  text only (it already excludes storage).
- Do **not** attempt logins, form submissions that mutate remote state, or
  any browser_* shell/exec bridge (they are blocked by the sandbox).
- Prefer the e2e/playwright engine for headless research; use stealth only
  when anti-bot blocks require it and the binary is installed.

## Research procedure

1. browser_set_engine (e2e recommended for headless research).
2. browser_navigate to the target URL.
3. browser_snapshot to capture sanitized page title + visible text.
4. browser_screenshot — the tool returns a **path** + byte size. Remember
   the path; never inline base64 image bytes into the model context.
5. research_save with: slug, title, url, summary, screenshot_path, and
   key findings. The tool writes docs/research/<slug>.md via WriteGateway
   (undoable). The artifact must reference the screenshot by **path**,
   not by base64 content.
6. browser_close when done to release the engine.

## Artifact format (docs/research/<slug>.md)

The research_save tool composes the markdown. The screenshot is referenced
by relative path (e.g. ` + "`screenshot: docs/research/shots/<slug>.png`" + `),
never inlined. Cookies / Authorization headers / bearer tokens are never
written into the artifact (browser sanitizer + research_save scrub).

## Secrets

Never include API keys, bearer tokens, cookies, or .env contents in the
research artifact or in tool arguments. If a page exposes credentials in
text, redact them before saving.
`

// ResearchMetas returns discovery-only metadata for the research skill.
func ResearchMetas() []Meta {
	return []Meta{
		{
			Name:          NameResearch,
			Description:   "Read-only browser research: navigate + screenshot + snapshot, then save docs/research/<slug>.md via WriteGateway (screenshot referenced by path, not base64)",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
	}
}

// loadResearchBuiltin returns the fully loaded research skill, or false.
func loadResearchBuiltin(name string) (*Skill, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name != NameResearch {
		return nil, false
	}
	return &Skill{
		Meta: Meta{
			Name:          NameResearch,
			Description:   "Read-only browser research: navigate + screenshot + snapshot, then save docs/research/<slug>.md via WriteGateway (screenshot referenced by path, not base64)",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
		Body: researchBody,
		// Grant research_save while the skill is loaded. browser_* tools are
		// already in the baseline registry when a BrowserBackend is wired;
		// the skill scopes their use to read-only research via the prompt
		// overlay and the research_save writer (which is the only write
		// path research produces — it routes through WriteGateway).
		AllowedTools: []string{ToolResearchSave},
	}, true
}

// IsResearchSkill reports whether name is the research builtin skill.
func IsResearchSkill(name string) bool {
	_, ok := loadResearchBuiltin(name)
	return ok
}
