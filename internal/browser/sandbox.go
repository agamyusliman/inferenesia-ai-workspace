package browser

import (
	"fmt"
	"strings"
)

// Forbidden model-controlled flags that must never be accepted from tool args
// (VAL-BRW-009: engine launch args are server-controlled; no exec bridge).
var forbiddenLaunchFragments = []string{
	"--eval",
	"--exec",
	"-e ",
	"--js-flags",
	"--renderer-cmd-prefix",
	"--utility-cmd-prefix",
	"--gpu-launcher",
	"--shell",
	"--crash-dumps-dir=/tmp/exec",
	"$(",
	"`",
	"&&",
	";",
	"|",
	"\n",
}

// ValidateNavigateURL rejects URLs that attempt to smuggle shell/exec launch
// options or browser switch injection (VAL-BRW-009).
func ValidateNavigateURL(raw string) error {
	u := strings.TrimSpace(raw)
	if u == "" {
		return fmt.Errorf("browser: url is required")
	}
	// Block chrome:// and file:// exec-ish abuse; allow data:, http(s), about:
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "javascript:") {
		return fmt.Errorf("browser: javascript: URLs are not allowed (page JS only via evaluate)")
	}
	// Reject obvious shell metacharacters used as "options" appendages.
	for _, frag := range []string{" --exec", " --eval", "&&", "; rm ", "| bash", "$(", "`"} {
		if strings.Contains(lower, strings.ToLower(frag)) {
			return fmt.Errorf("browser: rejected navigate url (forbidden shell/exec fragment)")
		}
	}
	// Reject space-delimited chrome switch injection after a normal URL.
	if i := strings.Index(u, " --"); i >= 0 {
		return fmt.Errorf("browser: rejected navigate url (launch flags are server-controlled)")
	}
	return nil
}

// ValidateEvaluateExpression ensures evaluate stays page-JS only: no model
// request for host shell, no CDP Target.createTarget with shell, etc.
// Note: this is a soft filter; the real boundary is that Evaluate is sent only
// to Page Runtime.evaluate and never to os/exec (VAL-BRW-009).
func ValidateEvaluateExpression(expr string) error {
	e := strings.TrimSpace(expr)
	if e == "" {
		return fmt.Errorf("browser: evaluate expression is required")
	}
	// Reject attempts to reach a non-existent host exec bridge.
	low := strings.ToLower(e)
	for _, banned := range []string{
		"require('child_process')",
		`require("child_process")`,
		"child_process",
		"process.binding",
		"__yura_exec",
		"__yura_shell",
		"runtime.addexec",
		"os/exec",
	} {
		if strings.Contains(low, banned) {
			return fmt.Errorf("browser: evaluate is page JS only — host shell/exec is not exposed")
		}
	}
	return nil
}

// ValidateLaunchArgsFromModel always rejects model-supplied launch args
// (VAL-BRW-009: launch args are server-controlled).
func ValidateLaunchArgsFromModel(args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("browser: launch args are server-controlled; model cannot supply engine flags (got %d arg(s))", len(args))
}

// RejectForbiddenLaunchArg returns an error if a single server-side arg would
// reintroduce shell/exec bridges. Server code should only use fixed allowlists;
// this is a defense-in-depth check for ExtraArgs.
func RejectForbiddenLaunchArg(arg string) error {
	low := strings.ToLower(strings.TrimSpace(arg))
	for _, frag := range forbiddenLaunchFragments {
		if strings.Contains(low, strings.ToLower(frag)) {
			return fmt.Errorf("browser: forbidden launch arg fragment %q", frag)
		}
	}
	return nil
}

// IsBrowserToolName reports whether name is a browser_* tool (no shell namespace).
func IsBrowserToolName(name string) bool {
	n := strings.TrimSpace(name)
	return strings.HasPrefix(n, "browser_")
}

// BrowserToolNames is the closed set of model-facing browser tools (VAL-BRW-009).
// Notably absent: browser_shell, browser_exec, browser_run.
var BrowserToolNames = []string{
	"browser_set_engine",
	"browser_status",
	"browser_navigate",
	"browser_screenshot",
	"browser_evaluate",
	"browser_snapshot",
	"browser_wait_human",
	"browser_close",
}

// HasShellBridgeTool reports whether a tool name is a forbidden shell bridge
// under the browser namespace.
func HasShellBridgeTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "browser_shell", "browser_exec", "browser_run", "browser_system",
		"browser_cmd", "browser_spawn":
		return true
	default:
		return false
	}
}
