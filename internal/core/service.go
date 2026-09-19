// Package core hosts the transport-agnostic agent loop and Service facade.
// CLI and desktop (Wails) both use the same Service — no parallel agent loop in TS.
package core

import (
	"fmt"
	"sync"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/pty"
	"github.com/agamyusliman/inferenesia-app/internal/store"
	"github.com/agamyusliman/inferenesia-app/internal/workspace"
)

// HubState is the desktop shell snapshot for the active session.
type HubState struct {
	// Product is the user-facing brand string (must contain "Inferenesia").
	Product       string                `json:"product"`
	ActiveID      string                `json:"active_id"`
	Active        *workspace.Workspace  `json:"active,omitempty"`
	Workspaces    []workspace.Workspace `json:"workspaces"`
	ConfigHome    string                `json:"config_home"`
	SelectedFile  string                `json:"selected_file,omitempty"`
	ChatContext   string                `json:"chat_context,omitempty"`
	StatusMessage string                `json:"status_message,omitempty"`
	// PlanModeActive is true when the agent is in read-only Plan Mode (VAL-PLAN-001).
	PlanModeActive bool `json:"plan_mode_active"`
}

// Service is the shared core facade for CLI and desktop bindings.
// Methods are safe for concurrent use from Wails/dev-HTTP handlers.
type Service struct {
	mu           sync.Mutex
	configHome   string
	registry     *workspace.Registry
	selectedFile string
	statusMsg    string
	// chat is the in-memory desktop chat transcript for chatWorkspaceID.
	// Durable copy lives in sessions.db (one active thread per workspace).
	chat            ChatSession
	chatWorkspaceID string
	sessions        *store.Store

	// Integrated terminal (desktop panel) — in-process internal/pty manager.
	// Lazy-init via ensurePTY(); not a parallel terminal implementation.
	ptys       *pty.Manager // from internal/pty; typed in terminal.go methods
	termMu     sync.Mutex
	termTitles map[string]string
	termFans   map[string]*termFan

	// gitApprover bridges agent git tools to NeedsApproval UI (VAL-GIT-005+).
	gitApprover *gitApproverBridge

	// planMode is the shared Plan Mode state machine (read-only agent tools).
	// Lazy-init via ensurePlanMode(); nil means inactive.
	planMode *plan.State

	// planTodos holds PlanProposed + session todos (VAL-PLAN-003/004/005).
	// Lazy-init via ensurePlanTodos().
	planTodos *planTodoState

	// missions holds Mission/Goal store + MissionStatus emit (VAL-MISSION-*).
	// Lazy-init via ensureMissions().
	missions *missionState

	// orch holds orchestrator.Manager for task() (VAL-ORCH-*).
	// Lazy-init via ensureOrch().
	orch *orchState

	// browser holds BrowserManager + BrowserStatus stream (VAL-BRW-*).
	// Lazy-init via ensureBrowser().
	browser *browserState

	// fileWatcher watches the active workspace root for external writes (CLI /
	// OS / editor) and broadcasts FileChangeEvent values to SSE subscribers
	// so the desktop explorer refreshes via push events, not polling
	// (VAL-CROSS-003). Lazily started; nil-safe when disabled.
	fileWatcher *fileWatcher

	// chatRuns tracks backend-owned chat generations so browser disconnect /
	// reload does not cancel the agent turn. Explicit Stop uses CancelChatRun.
	chatRunsMu sync.Mutex
	chatRuns   map[string]*chatRun

	// mcpState holds the persistent MCP host (survives across chat streams).
	// Lazy-init via ensureMCPState().
	mcpState *mcpState
}

// Options configures NewService.
type Options struct {
	// ConfigHome overrides config.Home() when non-empty (tests / desktop isolation).
	ConfigHome string
}

// NewService constructs the shared core Service used by CLI and desktop.
// VAL-DESK-009: cmd/desktop and CLI both reference this constructor.
func NewService(opts Options) (*Service, error) {
	var home string
	var err error
	if opts.ConfigHome != "" {
		home, err = config.EnsureHomeAt(opts.ConfigHome)
	} else {
		home, err = config.EnsureHome()
	}
	if err != nil {
		return nil, fmt.Errorf("core: ensure config home: %w", err)
	}

	reg, err := workspace.Open(home)
	if err != nil {
		return nil, fmt.Errorf("core: open workspace registry: %w", err)
	}
	svc := &Service{
		configHome: home,
		registry:   reg,
		statusMsg:  "Ready",
	}
	// Restore durable chat for the last active workspace on app start (VAL-DESK-014).
	if id := reg.ActiveID(); id != "" {
		svc.switchChatWorkspace(id)
	}
	return svc, nil
}

// BrandName returns the product brand (Inferenesia).
func (s *Service) BrandName() string {
	return brand.Name
}

// ConfigHome returns the resolved config home path.
func (s *Service) ConfigHome() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configHome
}

// GetHubState returns the desktop hub snapshot.
func (s *Service) GetHubState() HubState {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.registry.List()
	active := s.registry.Active()
	chatCtx := "No workspace selected"
	if active != nil {
		if workspace.IsSession(*active) {
			chatCtx = fmt.Sprintf("Session: %s", active.Name)
		} else {
			chatCtx = fmt.Sprintf("Workspace: %s (%s)", active.Name, active.RootPath)
		}
	}
	planActive := s.planMode != nil && s.planMode.Active()
	return HubState{
		Product:        brand.Name,
		ActiveID:       s.registry.ActiveID(),
		Active:         active,
		Workspaces:     list,
		ConfigHome:     s.configHome,
		SelectedFile:   s.selectedFile,
		ChatContext:    chatCtx,
		StatusMessage:  s.statusMsg,
		PlanModeActive: planActive,
	}
}

// ListWorkspaces returns registered workspaces.
func (s *Service) ListWorkspaces() []workspace.Workspace {
	return s.registry.List()
}

// OpenWorkspace registers (or focuses) a project path and makes it active.
// Switches the durable per-workspace chat thread (VAL-DESK-014).
func (s *Service) OpenWorkspace(path string) (workspace.Workspace, error) {
	w, err := s.registry.OpenPath(path)
	if err != nil {
		return workspace.Workspace{}, err
	}
	s.mu.Lock()
	s.selectedFile = ""
	s.statusMsg = fmt.Sprintf("Opened workspace %s", w.Name)
	s.mu.Unlock()
	s.onWorkspaceBound(w.ID, w.RootPath)
	return w, nil
}

// SwitchWorkspace one-click switches the active workspace by id.
// Loads that workspace's durable chat thread (VAL-DESK-014).
func (s *Service) SwitchWorkspace(id string) (workspace.Workspace, error) {
	w, err := s.registry.Switch(id)
	if err != nil {
		return workspace.Workspace{}, err
	}
	s.mu.Lock()
	s.selectedFile = ""
	s.statusMsg = fmt.Sprintf("Active workspace: %s", w.Name)
	s.mu.Unlock()
	s.onWorkspaceBound(w.ID, w.RootPath)
	return w, nil
}

// onWorkspaceBound runs after active workspace changes: chat thread, todos,
// browser engine/history. Mission list is filtered by workspace_id on read.
func (s *Service) onWorkspaceBound(id, root string) {
	s.switchChatWorkspace(id)
	s.BindTodosToWorkspace(id)
	go s.ResetBrowserForWorkspace()
	go s.watchWorkspaceRoot(root)
}

// watchWorkspaceRoot attaches the file watcher to the active workspace root
// so external writes (CLI agent, OS, editor) push FileChanged events to the
// desktop explorer via SSE (VAL-CROSS-003). Best-effort: a watcher init
// failure does not break workspace open.
func (s *Service) watchWorkspaceRoot(root string) {
	s.mu.Lock()
	fw := s.fileWatcher
	if fw == nil {
		fw = newFileWatcher()
		s.fileWatcher = fw
	}
	s.mu.Unlock()
	_ = fw.setRoot(root)
}

// FileWatcherSubscribe returns a channel of FileChangeEvent values for SSE
// streaming to the desktop explorer (VAL-CROSS-003). The caller MUST call
// FileWatcherUnsubscribe when the SSE client disconnects to avoid leaks.
// Returns a receive-only channel so callers cannot accidentally write to it.
func (s *Service) FileWatcherSubscribe() <-chan FileChangeEvent {
	s.mu.Lock()
	fw := s.fileWatcher
	s.mu.Unlock()
	if fw == nil {
		// No workspace open yet: return a closed channel so SSE exits cleanly.
		ch := make(chan FileChangeEvent)
		close(ch)
		return ch
	}
	if err := fw.ensureStarted(); err != nil {
		ch := make(chan FileChangeEvent)
		close(ch)
		return ch
	}
	return fw.subscribe()
}

// FileWatcherUnsubscribe removes a previously-subscribed channel. Pass the
// same receive-only channel returned by FileWatcherSubscribe.
func (s *Service) FileWatcherUnsubscribe(ch <-chan FileChangeEvent) {
	s.mu.Lock()
	fw := s.fileWatcher
	s.mu.Unlock()
	if fw == nil {
		return
	}
	fw.unsubscribeByRecv(ch)
}

// suppressFile marks an absolute path as "our own WriteGateway mutation" for a
// short window so the fsnotify echo does not double-fire a FileChanged event
// to the explorer (the chat SSE already pushed one). CLI writes are external
// and never suppressed (VAL-CROSS-003). Safe to call with empty path.
func (s *Service) suppressFile(abs string) {
	if abs == "" {
		return
	}
	s.mu.Lock()
	fw := s.fileWatcher
	s.mu.Unlock()
	if fw == nil {
		return
	}
	fw.suppressPath(abs)
}

// Close releases resources held by the Service (file watcher goroutine +
// fsnotify watcher). Safe to call multiple times. Called on desktop/CLI
// shutdown. Best-effort: errors are ignored.
func (s *Service) Close() {
	s.mu.Lock()
	fw := s.fileWatcher
	s.mu.Unlock()
	if fw != nil {
		fw.close()
	}
}

// ListDir lists files/dirs under the active workspace (or an explicit root when path given).
// When workspaceID is empty, uses the active workspace.
func (s *Service) ListDir(workspaceID, rel string) ([]workspace.DirEntry, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return nil, err
	}
	return workspace.ListDir(root, rel)
}

// SelectFile sets the selected file path for the hub main pane.
func (s *Service) SelectFile(rel string) (HubState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selectedFile = rel
	if rel != "" {
		s.statusMsg = fmt.Sprintf("Selected %s", rel)
	}
	// Build state without re-locking registry via nested calls.
	list := s.registry.List()
	active := s.registry.Active()
	chatCtx := "No workspace selected"
	if active != nil {
		if workspace.IsSession(*active) {
			chatCtx = fmt.Sprintf("Session: %s", active.Name)
		} else {
			chatCtx = fmt.Sprintf("Workspace: %s (%s)", active.Name, active.RootPath)
		}
	}
	planActive := s.planMode != nil && s.planMode.Active()
	return HubState{
		Product:        brand.Name,
		ActiveID:       s.registry.ActiveID(),
		Active:         active,
		Workspaces:     list,
		ConfigHome:     s.configHome,
		SelectedFile:   s.selectedFile,
		ChatContext:    chatCtx,
		StatusMessage:  s.statusMsg,
		PlanModeActive: planActive,
	}, nil
}

// ReadFile reads a text file relative to the active (or specified) workspace.
// Large files are truncated with a continuation hint (never hard-rejected for size).
func (s *Service) ReadFile(workspaceID, rel string) (string, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return "", err
	}
	res, err := workspace.ReadFileWindow(root, rel, 0, workspace.DefaultReadMaxBytes)
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// ListMentionFiles returns workspace files for the chat @file mention picker.
func (s *Service) ListMentionFiles(query string, limit int) ([]workspace.FileMention, error) {
	root, err := s.activeRoot()
	if err != nil {
		return nil, err
	}
	return workspace.ListFiles(root, query, limit)
}

// GetActiveWorkspace returns the active workspace or nil.
func (s *Service) GetActiveWorkspace() *workspace.Workspace {
	if s == nil || s.registry == nil {
		return nil
	}
	return s.registry.Active()
}

// EnsureSessionWorkspace activates the most recent session (or creates one).
func (s *Service) EnsureSessionWorkspace() (workspace.Workspace, error) {
	if s == nil || s.registry == nil {
		return workspace.Workspace{}, fmt.Errorf("core: service not ready")
	}
	w, err := s.registry.EnsureScratchSession()
	if err != nil {
		return workspace.Workspace{}, err
	}
	s.mu.Lock()
	s.selectedFile = ""
	s.statusMsg = "Session mode (no project folder)"
	s.mu.Unlock()
	s.onWorkspaceBound(w.ID, w.RootPath)
	return w, nil
}

// CreateSessionWorkspace creates a new named session and activates it.
func (s *Service) CreateSessionWorkspace(name string) (workspace.Workspace, error) {
	if s == nil || s.registry == nil {
		return workspace.Workspace{}, fmt.Errorf("core: service not ready")
	}
	w, err := s.registry.CreateSession(name)
	if err != nil {
		return workspace.Workspace{}, err
	}
	s.mu.Lock()
	s.selectedFile = ""
	s.statusMsg = fmt.Sprintf("Session: %s", w.Name)
	s.mu.Unlock()
	s.onWorkspaceBound(w.ID, w.RootPath)
	return w, nil
}

// RenameWorkspace sets the display name of a workspace or session.
func (s *Service) RenameWorkspace(id, name string) (workspace.Workspace, error) {
	if s == nil || s.registry == nil {
		return workspace.Workspace{}, fmt.Errorf("core: service not ready")
	}
	w, err := s.registry.Rename(id, name)
	if err != nil {
		return workspace.Workspace{}, err
	}
	s.mu.Lock()
	s.statusMsg = fmt.Sprintf("Renamed to %s", w.Name)
	s.mu.Unlock()
	return w, nil
}

func (s *Service) rootFor(workspaceID string) (string, error) {
	if workspaceID == "" {
		root, err := s.activeRoot()
		if err != nil {
			return "", err
		}
		return root, nil
	}
	w, err := s.registry.Get(workspaceID)
	if err != nil {
		// Allow session id even if not yet registered.
		if workspaceID == workspace.ScratchSessionID {
			sw, e2 := s.registry.EnsureScratchSession()
			if e2 != nil {
				return "", err
			}
			return sw.RootPath, nil
		}
		return "", err
	}
	return w.RootPath, nil
}

