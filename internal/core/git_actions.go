package core

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/git"
)

// GitActionsResult is the Git panel Actions tab payload (VAL-GHA).
type GitActionsResult = git.ActionsResult

// GitWorkflowRun is one run row.
type GitWorkflowRun = git.WorkflowRun

// GetGitActions lists recent GitHub Actions workflow runs for repoID (VAL-GHA-002/005).
// Uses `gh` CLI when available, else GITHUB_TOKEN/GH_TOKEN. Never returns token values.
func (s *Service) GetGitActions(repoID string, limit int) (GitActionsResult, error) {
	gs, selected, _, err := s.gitServiceFor(repoID)
	if err != nil {
		return GitActionsResult{
			OK:           false,
			Availability: git.ActionsNotRepo,
			Message:      err.Error(),
			Runs:         []GitWorkflowRun{},
			RepoID:       repoID,
		}, err
	}
	res := gs.ListGitHubActions(git.ListActionsOptions{
		Limit:    limit,
		PreferGH: true,
	})
	res.RepoID = selected.ID
	// Extra safety: never surface secret-like detail
	res.Detail = git.RedactSecrets(res.Detail)
	res.Message = git.RedactSecrets(res.Message)
	if res.OK {
		s.setStatus(fmt.Sprintf("actions: %s", res.Message))
	}
	return res, nil
}

// OpenGitActionsRun opens a validated github.com workflow run URL in the OS browser (VAL-GHA-004).
func (s *Service) OpenGitActionsRun(rawURL string) (map[string]any, error) {
	u, err := git.OpenRunURL(rawURL)
	if err != nil {
		return map[string]any{"ok": false, "message": err.Error()}, err
	}
	if err := openBrowserURL(u); err != nil {
		return map[string]any{"ok": false, "message": "failed to open browser", "url": u}, err
	}
	return map[string]any{"ok": true, "url": u, "message": "opened"}, nil
}

func openBrowserURL(u string) error {
	u = strings.TrimSpace(u)
	if u == "" {
		return fmt.Errorf("empty url")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}
