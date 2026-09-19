package main

import (
	"context"
	"fmt"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/workspace"
)

// App is the Wails binding surface. All methods delegate to core.Service
// (no parallel agent/LLM loop in the renderer).
type App struct {
	ctx context.Context
	svc *core.Service
}

// NewApp wraps an existing core.Service for Wails.
func NewApp(svc *core.Service) *App {
	return &App{svc: svc}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// BrandName returns the product brand string (Inferenesia).
func (a *App) BrandName() string {
	return a.svc.BrandName()
}

// GetHubState returns the shell hub snapshot.
func (a *App) GetHubState() core.HubState {
	return a.svc.GetHubState()
}

// ListWorkspaces returns registered workspaces.
func (a *App) ListWorkspaces() []workspace.Workspace {
	return a.svc.ListWorkspaces()
}

// OpenWorkspace opens or focuses a project path.
func (a *App) OpenWorkspace(path string) (workspace.Workspace, error) {
	return a.svc.OpenWorkspace(path)
}

// PickDirectory opens a native OS folder chooser. Empty string = user cancelled.
func (a *App) PickDirectory() (string, error) {
	return a.svc.PickDirectory()
}

// SwitchWorkspace one-click switches the active workspace.
func (a *App) SwitchWorkspace(id string) (workspace.Workspace, error) {
	return a.svc.SwitchWorkspace(id)
}

// ListDir lists explorer entries for the active (or specified) workspace.
func (a *App) ListDir(workspaceID, rel string) ([]workspace.DirEntry, error) {
	return a.svc.ListDir(workspaceID, rel)
}

// SelectFile selects a file in the main pane.
func (a *App) SelectFile(rel string) (core.HubState, error) {
	return a.svc.SelectFile(rel)
}

// ReadFile reads a workspace-relative file for preview.
func (a *App) ReadFile(workspaceID, rel string) (string, error) {
	return a.svc.ReadFile(workspaceID, rel)
}

// SaveFile writes editor buffer content as a user edit (WriteGateway SourceUser).
func (a *App) SaveFile(workspaceID, rel, content string) (core.FSResult, error) {
	return a.svc.SaveFile(workspaceID, rel, content)
}

// StatusMessage is a short helper for the status bar.
func (a *App) StatusMessage() string {
	return a.svc.GetHubState().StatusMessage
}

// GetUndoStack returns undo toolbar state (pre-agent dirty stack).
func (a *App) GetUndoStack() (core.UndoStackState, error) {
	return a.svc.GetUndoStack()
}

// UndoLast restores pre-agent dirty content (not git HEAD).
func (a *App) UndoLast() (core.UndoActionResult, error) {
	return a.svc.UndoLast()
}

// RedoLast re-applies the last undone agent mutation.
func (a *App) RedoLast() (core.UndoActionResult, error) {
	return a.svc.RedoLast()
}

// RevertAgent is the explicit "Revert agent" action (pre-agent dirty).
func (a *App) RevertAgent() (core.UndoActionResult, error) {
	return a.svc.RevertAgent()
}

// RestoreToHEAD restores a path via git (distinct from Revert agent).
func (a *App) RestoreToHEAD(path string) (core.RestoreToHEADResult, error) {
	return a.svc.RestoreToHEAD(path)
}

// GetAgentChangeHistory returns the WriteGateway agent-changes timeline
// (not raw git history; VAL-DESK-012).
func (a *App) GetAgentChangeHistory() (core.AgentChangeHistory, error) {
	return a.svc.GetAgentChangeHistory()
}

// GetSettings returns settings panel sections (profiles, models, token savers, workspace prefs).
func (a *App) GetSettings(profile string) core.SettingsView {
	return a.svc.GetSettings(profile)
}

// GetTokenSavers returns Settings → Token Saver toggles + install status (VAL-SAVER-001).
func (a *App) GetTokenSavers() core.TokenSaversView {
	return a.svc.GetTokenSavers()
}

// SetTokenSaver persists one Token Saver toggle under ~/.inferenesia (VAL-SAVER-002/007).
// Enabling a missing binary does not fail; status stays Not installed.
func (a *App) SetTokenSaver(req core.SetTokenSaverRequest) (core.TokenSaversView, error) {
	return a.svc.SetTokenSaver(req)
}

// GetLiveBlocks returns the Settings → Live Blocks opt-in toggle (P9-live).
func (a *App) GetLiveBlocks() core.LiveBlocksView {
	return a.svc.GetLiveBlocks()
}

// SetLiveBlocks persists the Live Blocks toggle under ~/.inferenesia (P9-live).
func (a *App) SetLiveBlocks(req core.SetLiveBlocksRequest) (core.LiveBlocksView, error) {
	return a.svc.SetLiveBlocks(req)
}

// AddProvider registers a BYOK openai_compatible profile in config (mode 0600).
func (a *App) AddProvider(req core.AddProviderRequest) (core.SettingsView, error) {
	return a.svc.AddProvider(req)
}

// TestProvider runs a minimal chat completion against a profile (VAL-PROV-004).
func (a *App) TestProvider(profile string) core.TestProviderResult {
	return a.svc.TestProvider(profile)
}

// GetOnboarding returns first-run dual-path state (Inferenesia API gateway vs BYOK).
func (a *App) GetOnboarding() core.OnboardingState {
	return a.svc.GetOnboarding()
}

// CompleteOnboarding marks first-run done after either path.
func (a *App) CompleteOnboarding(path string) (core.OnboardingState, error) {
	return a.svc.CompleteOnboarding(path)
}

// GetActiveSession returns the mid-session active provider/model (VAL-PROV-005).
func (a *App) GetActiveSession() core.ActiveSessionView {
	return a.svc.GetActiveSession()
}

// SetActiveProvider switches provider/model mid-session without restart (VAL-PROV-005/011).
func (a *App) SetActiveProvider(req core.SetActiveProviderRequest) (core.ActiveSessionView, error) {
	return a.svc.SetActiveProvider(req)
}

// ListMentionFiles returns files for the @file mention picker.
func (a *App) ListMentionFiles(query string, limit int) ([]workspace.FileMention, error) {
	return a.svc.ListMentionFiles(query, limit)
}

// CreateFile creates an empty file (explorer Pack A New File).
func (a *App) CreateFile(workspaceID, parent, name string) (core.FSResult, error) {
	return a.svc.CreateFile(workspaceID, parent, name)
}

// CreateFolder creates a directory (explorer Pack A New Folder).
func (a *App) CreateFolder(workspaceID, parent, name string) (core.FSResult, error) {
	return a.svc.CreateFolder(workspaceID, parent, name)
}

// RenamePath renames a file or folder (collision rejected).
func (a *App) RenamePath(workspaceID, rel, newName string) (core.FSResult, error) {
	return a.svc.RenamePath(workspaceID, rel, newName)
}

// DeletePath deletes a file or folder recursively (after UI confirm).
func (a *App) DeletePath(workspaceID, rel string) (core.FSResult, error) {
	return a.svc.DeletePath(workspaceID, rel)
}

// CopyPath duplicates a file/folder into dest parent (clipboard paste copy).
func (a *App) CopyPath(workspaceID, src, destParent string) (core.FSResult, error) {
	return a.svc.CopyPath(workspaceID, src, destParent)
}

// MovePath moves a file/folder into dest parent (clipboard paste cut).
func (a *App) MovePath(workspaceID, src, destParent string) (core.FSResult, error) {
	return a.svc.MovePath(workspaceID, src, destParent)
}

// FindInFolder searches under a folder (path + content); scoped results only.
func (a *App) FindInFolder(workspaceID, folder, query string, limit int) ([]core.FindHit, error) {
	return a.svc.FindInFolder(workspaceID, folder, query, limit)
}

// ResolvePath returns absolute/relative path info for copy-path actions.
func (a *App) ResolvePath(workspaceID, rel string) (core.PathInfo, error) {
	return a.svc.ResolvePath(workspaceID, rel)
}

// RevealInOS opens the OS file manager focused on the path.
func (a *App) RevealInOS(workspaceID, rel string) (core.FSResult, error) {
	return a.svc.RevealInOS(workspaceID, rel)
}

// RemoveWorkspace drops a registry root without deleting files from disk.
func (a *App) RemoveWorkspace(id string) error {
	return a.svc.RemoveWorkspace(id)
}

// AddFolderToWorkspace registers a folder as an additional workspace root.
func (a *App) AddFolderToWorkspace(workspaceID, path string) (workspace.Workspace, error) {
	return a.svc.AddFolderToWorkspace(workspaceID, path)
}

// TerminalStart opens an interactive integrated terminal (internal/pty).
func (a *App) TerminalStart(req core.TerminalStartRequest) (core.TerminalSessionView, error) {
	return a.svc.TerminalStart(req)
}

// TerminalList lists integrated terminal sessions.
func (a *App) TerminalList() []core.TerminalSessionView {
	return a.svc.TerminalList()
}

// TerminalWrite sends input to a PTY session.
func (a *App) TerminalWrite(req core.TerminalWriteRequest) error {
	return a.svc.TerminalWrite(req)
}

// TerminalResize sets PTY cols×rows after xterm FitAddon measures the surface.
func (a *App) TerminalResize(req core.TerminalResizeRequest) error {
	return a.svc.TerminalResize(req)
}

// TerminalRead returns captured output + session meta.
func (a *App) TerminalRead(id string) (core.TerminalReadResult, error) {
	return a.svc.TerminalRead(id)
}

// TerminalStop stops one terminal session without killing others.
func (a *App) TerminalStop(id string) (core.TerminalStopResult, error) {
	return a.svc.TerminalStop(id)
}

// ListTerminalSnippets returns the global snippet library under config home (VAL-IDE-036).
func (a *App) ListTerminalSnippets() (core.TerminalSnippetList, error) {
	return a.svc.ListTerminalSnippets()
}

// SaveTerminalSnippet creates or updates a global terminal snippet.
func (a *App) SaveTerminalSnippet(req core.TerminalSnippetSaveRequest) (core.TerminalSnippet, error) {
	return a.svc.SaveTerminalSnippet(req)
}

// DeleteTerminalSnippet removes a global terminal snippet by id.
func (a *App) DeleteTerminalSnippet(id string) (core.TerminalSnippetList, error) {
	return a.svc.DeleteTerminalSnippet(id)
}

// ListGitRepos discovers nested git repos under the active workspace (multi source-control).
func (a *App) ListGitRepos() ([]core.GitRepoRef, error) {
	return a.svc.ListGitRepos()
}

// GetGitStatus returns branch + porcelain-derived file list for the Git panel (VAL-GIT-001).
// repoID selects nested repo ("." or folder path); empty = first discovered.
func (a *App) GetGitStatus(repoID string) (core.GitStatus, error) {
	return a.svc.GetGitStatus(repoID)
}

// GetGitDiff returns unified diff for a path (VAL-GIT-002).
func (a *App) GetGitDiff(repoID, path string, staged bool) (core.GitDiffResult, error) {
	return a.svc.GetGitDiff(repoID, path, staged)
}

// GetGitGraph returns commit graph for the Graph tab.
func (a *App) GetGitGraph(repoID string, limit int) (core.GitGraphResult, error) {
	return a.svc.GetGitGraph(repoID, limit)
}

// GitStage stages one or more paths (VAL-GIT-003).
func (a *App) GitStage(req core.GitStageRequest) (core.GitOpResult, error) {
	return a.svc.GitStage(req)
}

// GitUnstage unstages one or more paths (VAL-GIT-003).
func (a *App) GitUnstage(req core.GitStageRequest) (core.GitOpResult, error) {
	return a.svc.GitUnstage(req)
}

// GitCommit commits the index with a message (VAL-GIT-004).
func (a *App) GitCommit(req core.GitCommitRequest) (core.GitOpResult, error) {
	return a.svc.GitCommit(req)
}

// GitFetch runs git fetch with clear success/failure (VAL-GIT-010).
func (a *App) GitFetch(repoID string) (core.GitOpResult, error) {
	return a.svc.GitFetch(repoID)
}

// GitPull runs git pull with clear success/failure (VAL-GIT-010).
func (a *App) GitPull(repoID string) (core.GitOpResult, error) {
	return a.svc.GitPull(repoID)
}

// GitPush runs UI push (confirm required; force gated separately — VAL-GIT-007/011).
func (a *App) GitPush(req core.GitPushRequest) (core.GitOpResult, error) {
	return a.svc.GitPush(req)
}

// GitDestructive runs confirmed reset --hard / branch delete (VAL-GIT-006).
func (a *App) GitDestructive(req core.GitDestructiveRequest) (core.GitOpResult, error) {
	return a.svc.GitDestructive(req)
}

// ResolveApproval resolves a NeedsApproval dialog (VAL-GIT-005+).
func (a *App) ResolveApproval(dec core.ApprovalDecision) error {
	return a.svc.ResolveApproval(dec)
}

// GetPendingApproval returns the open NeedsApproval request if any.
func (a *App) GetPendingApproval() *core.PendingApproval {
	return a.svc.GetPendingApproval()
}

// ListGitBranches returns local/remote branches for checkout UI.
func (a *App) ListGitBranches(repoID string) (core.GitBranchList, error) {
	return a.svc.ListGitBranches(repoID)
}

// GitCheckout switches or creates a branch.
func (a *App) GitCheckout(req core.GitCheckoutRequest) (core.GitOpResult, error) {
	return a.svc.GitCheckout(req)
}

// GetGitWorkflow loads per-workspace git/deploy workflow.
func (a *App) GetGitWorkflow() (core.GitWorkflow, error) {
	return a.svc.GetGitWorkflow()
}

// SaveGitWorkflow persists per-workspace git/deploy workflow.
func (a *App) SaveGitWorkflow(w core.GitWorkflow) (core.GitWorkflow, error) {
	return a.svc.SaveGitWorkflow(w)
}

// EnsureGitWorkflow writes default workflow file if missing.
func (a *App) EnsureGitWorkflow() (core.GitWorkflow, error) {
	return a.svc.EnsureGitWorkflow()
}

// GetGitActions lists recent GitHub Actions workflow runs for the Actions tab (VAL-GHA).
func (a *App) GetGitActions(repoID string, limit int) (core.GitActionsResult, error) {
	return a.svc.GetGitActions(repoID, limit)
}

// OpenGitActionsRun opens a validated github.com workflow run URL in the OS browser (VAL-GHA-004).
func (a *App) OpenGitActionsRun(url string) (map[string]any, error) {
	return a.svc.OpenGitActionsRun(url)
}

// GetChatSession returns the in-memory chat transcript.
func (a *App) GetChatSession() core.ChatSession {
	return a.svc.GetChatSession()
}

// GetChatHistory returns the durable per-workspace chat thread (VAL-DESK-014).
func (a *App) GetChatHistory(workspaceID string) (core.DesktopChatHistory, error) {
	return a.svc.GetChatHistory(workspaceID)
}

// DeleteChatMessage removes one message and persists the workspace thread (VAL-CHAT-018).
func (a *App) DeleteChatMessage(workspaceID, messageID string) (core.ChatMessageActionResult, error) {
	return a.svc.DeleteChatMessage(workspaceID, messageID)
}

// TruncateChatFromHere truncates history from message id to end (VAL-CHAT-017).
func (a *App) TruncateChatFromHere(workspaceID, messageID string) (core.ChatMessageActionResult, error) {
	return a.svc.TruncateChatFromHere(workspaceID, messageID)
}

// SetChatHistory replaces the durable workspace chat thread.
func (a *App) SetChatHistory(workspaceID string, messages []core.DesktopChatMessage) (core.DesktopChatHistory, error) {
	return a.svc.SetChatHistory(workspaceID, messages)
}

// ClearChatSession clears the in-memory chat transcript and durable empty thread.
func (a *App) ClearChatSession() {
	a.svc.ClearChatSession()
}

// CanvasAI runs a dedicated non-stream, no-tools canvas AI turn (D5b). Does not
// touch chat history; the canvas buffer is the sole source of truth.
func (a *App) CanvasAI(req core.CanvasAIRequest) core.CanvasAIResult {
	return a.svc.CanvasAI(req)
}

// Service exposes the underlying core.Service for tests (not typically bound to UI).
func (a *App) Service() *core.Service {
	return a.svc
}

// String is for logging.
func (a *App) String() string {
	return fmt.Sprintf("Inferenesia desktop App (svc=%p)", a.svc)
}
