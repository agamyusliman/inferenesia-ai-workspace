package orchestrator

import (
	"fmt"
	"strings"
)

// CategoryInfo describes one task() category (VAL-ORCH-008).
type CategoryInfo struct {
	// Name is the category id: explore | implement | verify | review | research.
	Name string `json:"name"`
	// Description is a short human-readable label.
	Description string `json:"description"`
	// ReadOnly is true when write tools are denied.
	ReadOnly bool `json:"read_only"`
	// AllowsWrite is true when WriteGateway mutation tools are allowed.
	AllowsWrite bool `json:"allows_write"`
	// Tools lists default tool names available for this category.
	Tools []string `json:"tools"`
	// Assertions lists assertion-style tools (verify category).
	Assertions []string `json:"assertions,omitempty"`
}

// Builtin tool name constants shared with tools package (avoid import cycle).
const (
	ToolReadFile      = "read_file"
	ToolGrep          = "grep"
	ToolListDir       = "list_dir"
	ToolWriteFile     = "write_file"
	ToolWriteMultibrain = "write_multibrain"
	ToolWriteDiagram  = "write_diagram"
	ToolShell         = "shell"
	// Diagram tools (VAL-DIAG-002/003/004/007): create/update/convert route
	// through WriteGateway; export renders to PNG/SVG. Allowed only for
	// implement category.
	ToolCreateDiagram = "create_diagram"
	ToolUpdateDiagram = "update_diagram"
	ToolExportDiagram = "export_diagram"
	ToolConvertDiagram = "convert_diagram"
	// Assertion-style tools for verify category (VAL-ORCH-008).
	ToolAssertContains = "assert_contains"
	ToolAssertExitCode = "assert_exit_code"
	ToolAssertFile     = "assert_file_exists"
	// Browser tool names (read-only research subset, VAL-CROSS-005).
	// Mirrors internal/browser.BrowserToolNames read-only subset.
	ToolBrowserSetEngine  = "browser_set_engine"
	ToolBrowserStatus     = "browser_status"
	ToolBrowserNavigate   = "browser_navigate"
	ToolBrowserScreenshot = "browser_screenshot"
	ToolBrowserSnapshot   = "browser_snapshot"
	ToolBrowserWaitHuman  = "browser_wait_human"
	ToolBrowserClose      = "browser_close"
	// Research artifact save tool (VAL-CROSS-005). Writes docs/research/<slug>.md
	// via WriteGateway; granted only while the research skill is loaded.
	ToolResearchSave = "research_save"
	// Builtin Context7 library-docs tool (P9-ctx). Read-only network fetch;
	// safe in every read-only category (explore/research/review/verify).
	ToolContext7Query = "context7_query"
)

// KnownCategories is the ordered enumerable set for `task categories` (VAL-ORCH-008).
// Primary product categories: explore / implement / verify; review & research kept as aliases.
func KnownCategories() []CategoryInfo {
	readTools := []string{ToolReadFile, ToolGrep, ToolListDir}
	context7Docs := []string{ToolContext7Query}
	writeTools := []string{ToolWriteFile, ToolWriteMultibrain, ToolWriteDiagram, ToolCreateDiagram, ToolUpdateDiagram, ToolExportDiagram, ToolConvertDiagram}
	assertTools := []string{ToolAssertContains, ToolAssertExitCode, ToolAssertFile}
	// Browser read-only research tools (VAL-CROSS-005). The research skill
	// scopes the browser to navigate + screenshot + snapshot only; these
	// tool names are the closed read-only subset (no shell/exec bridge,
	// no browser_evaluate to read storage — VAL-BRW-009).
	researchBrowserTools := []string{
		ToolBrowserSetEngine, ToolBrowserStatus, ToolBrowserNavigate,
		ToolBrowserScreenshot, ToolBrowserSnapshot, ToolBrowserWaitHuman,
		ToolBrowserClose,
	}
	// exploreBrowserTools is the read-only browser subset allowed for the
	// explore category (VAL-CROSS-011): a parallel task(explore) sub-task may
	// use the browser engine (read-only) to gather info. The closed read-only
	// subset mirrors the research browser tools (no shell/exec bridge, no
	// browser_evaluate to read storage — VAL-BRW-009). Explore remains
	// read-only: no write tools (write_file/create_diagram/etc.) are granted.
	exploreBrowserTools := researchBrowserTools

	return []CategoryInfo{
		{
			Name:        CategoryExplore,
			Description: "Read-only explore: search/read workspace files + read-only browser (navigate/screenshot/snapshot) for info gathering (no write tools)",
			ReadOnly:    true,
			AllowsWrite: false,
			Tools: append(append(append([]string(nil), readTools...), exploreBrowserTools...), context7Docs...),
		},
		{
			Name:        CategoryImplement,
			Description: "Implement: reads + writes through WriteGateway + shell",
			ReadOnly:    false,
			AllowsWrite: true,
			Tools:       append(append(append(append([]string(nil), readTools...), writeTools...), ToolShell), context7Docs...),
		},
		{
			Name:        CategoryVerify,
			Description: "Verify: read-only with assertion-style tools (no write tools)",
			ReadOnly:    true,
			AllowsWrite: false,
			Tools:       append(append(append([]string(nil), readTools...), assertTools...), context7Docs...),
			Assertions:  append([]string(nil), assertTools...),
		},
		{
			Name:        CategoryReview,
			Description: "Review: read-only code review (alias of explore + shell diagnostics)",
			ReadOnly:    true,
			AllowsWrite: false,
			Tools:       append(append(append([]string(nil), readTools...), ToolShell), context7Docs...),
		},
		{
			Name:        CategoryResearch,
			Description: "Research: read-only browser research (navigate + screenshot + snapshot) then save docs/research/<slug>.md via WriteGateway",
			ReadOnly:    true,
			AllowsWrite: false,
			// research_save routes through WriteGateway (undoable) but is the
			// only write path permitted for research — it writes under
			// docs/research/ only. Browser tools are read-only (VAL-BRW-009).
			Tools: func() []string {
				out := append([]string(nil), readTools...)
				out = append(out, researchBrowserTools...)
				out = append(out, ToolResearchSave, ToolContext7Query)
				return out
			}(),
		},
	}
}

// CategoryByName returns the CategoryInfo for name (case-insensitive).
// Unknown names default to explore (read-only, safest).
func CategoryByName(name string) CategoryInfo {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		n = CategoryExplore
	}
	for _, c := range KnownCategories() {
		if c.Name == n {
			return c
		}
	}
	// Unknown → explore defaults (fail closed for writes).
	for _, c := range KnownCategories() {
		if c.Name == CategoryExplore {
			out := c
			out.Name = n
			out.Description = fmt.Sprintf("unknown category %q — treated as explore (read-only)", n)
			return out
		}
	}
	return CategoryInfo{Name: n, ReadOnly: true, Tools: []string{ToolReadFile, ToolGrep, ToolListDir}}
}

// CategoryAllowsWrite reports whether category may use WriteGateway mutations.
func CategoryAllowsWrite(name string) bool {
	return CategoryByName(name).AllowsWrite
}

// CategoryIsReadOnly reports whether category denies all write tools.
func CategoryIsReadOnly(name string) bool {
	return CategoryByName(name).ReadOnly
}

// ToolsForCategory returns the default allowed tool name set for a category.
func ToolsForCategory(name string) []string {
	return append([]string(nil), CategoryByName(name).Tools...)
}

// ToolAllowedInCategory reports whether toolName is permitted under category.
func ToolAllowedInCategory(category, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	// Nested task() always denied at max depth by orchestrator; tool itself is
	// not category-gated here beyond write deny.
	for _, t := range ToolsForCategory(category) {
		if t == toolName {
			return true
		}
	}
	// Explicit write denials for read-only categories even if not listed.
	// This is the defense-in-depth gate: even if a write tool name were
	// accidentally added to a read-only category's tool list, it is denied
	// here. Covers every WriteGateway mutation path (fs, multibrain, diagram,
	// research) so a read-only sub-task can never write (VAL-CROSS-011,
	// VAL-ORCH-008).
	if CategoryIsReadOnly(category) {
		switch toolName {
		case ToolWriteFile, ToolWriteMultibrain, ToolWriteDiagram,
			ToolCreateDiagram, ToolUpdateDiagram, ToolExportDiagram, ToolConvertDiagram,
			ToolResearchSave, ToolShell:
			return false
		}
	}
	return false
}

// DenyToolError is the stable message when a category blocks a tool (VAL-ORCH-008).
func DenyToolError(category, toolName string) error {
	return fmt.Errorf("orchestrator: tool %q denied for category %q (read-only or not in tool set)", toolName, category)
}
