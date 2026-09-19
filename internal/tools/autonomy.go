package tools

import "strings"

// ToolRisk classifies tools for autonomy gating. Higher values are more dangerous.
type ToolRisk int

const (
	RiskReadOnly ToolRisk = iota // no workspace mutation
	RiskEdit                     // WriteGateway / docs (low+)
	RiskReversible               // git / terminal / MCP (medium+)
	RiskDestructive              // shell + irreversible git (high only)
)

func toolRisk(name string) ToolRisk {
	name = strings.TrimSpace(name)

	if strings.HasPrefix(name, "mcp.") {
		return RiskReversible
	}

	switch name {
	case NameReadFile, NameGrep, NameListDir:
		return RiskReadOnly
	case NameGitStatus, NameGitDiff, NameGitLog, NameGitBranchList:
		return RiskReadOnly
	case NameTerminalRead, NameTerminalList:
		return RiskReadOnly
	case "propose_plan", "todo_write", "todo_update", "todo_list":
		return RiskReadOnly
	case "get_goal", "create_goal", "update_goal", "attach_mission_evidence":
		return RiskReadOnly
	case NameTask:
		return RiskReadOnly
	case "skill", "skill_status":
		return RiskReadOnly
	case NameContext7Query, "multibrain_read":
		return RiskReadOnly

	case NameWriteFile, NameWriteMultibrain, NameWriteDiagram:
		return RiskEdit
	case NameCreateDiagram, NameUpdateDiagram, NameExportDiagram, NameConvertDiagram:
		return RiskEdit
	case "research_save", "multibrain_append", "diagram_export":
		return RiskEdit

	case NameTerminalStart, NameTerminalWrite, NameTerminalStop:
		return RiskReversible
	case NameGitStage, NameGitUnstage, NameGitCommit, NameGitFetch, NameGitPull, NameGitPush, NameGitCheckout:
		return RiskReversible

	case NameShell:
		return RiskDestructive
	case NameGitResetHard, NameGitBranchDelete:
		return RiskDestructive
	}

	return RiskDestructive
}

// autonomyDenies is a hard deny gate (not NeedsApproval). Raise the level to allow the tool.
// Git force-push / commit may still emit separate NeedsApproval when the tool is allowed.
func autonomyDenies(autonomyLevel, toolName string) (denied bool, reason string) {
	level := strings.TrimSpace(strings.ToLower(autonomyLevel))
	if level == "" {
		return false, ""
	}
	risk := toolRisk(toolName)

	switch level {
	case "off":
		if risk > RiskReadOnly {
			return true, "autonomy off: only read-only tools allowed — raise autonomy to Low (edits), Medium (git/terminal/MCP), or High (shell/destructive)"
		}
	case "low":
		if risk > RiskEdit {
			return true, "autonomy low: only read-only + file edits allowed — raise to Medium for git/terminal/MCP, or High for shell/destructive"
		}
	case "medium":
		if risk >= RiskDestructive {
			return true, "autonomy medium: shell and destructive git (reset --hard, branch delete) blocked — raise to High to allow"
		}
	case "high":
	default:
		if risk > RiskEdit {
			return true, "autonomy low: only read-only + file edits allowed — raise to Medium for git/terminal/MCP, or High for shell/destructive"
		}
	}

	return false, ""
}
