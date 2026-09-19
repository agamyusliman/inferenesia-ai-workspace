package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/pty"
	"github.com/agamyusliman/inferenesia-app/internal/workspace"
)

// TerminalSessionView is a desktop-safe snapshot of an integrated terminal session.
type TerminalSessionView struct {
	ID      string     `json:"id"`
	Title   string     `json:"title,omitempty"`
	Command string     `json:"command,omitempty"`
	Cwd     string     `json:"cwd"`
	Status  pty.Status `json:"status"`
	PID     int        `json:"pid"`
	ExitCode *int      `json:"exit_code,omitempty"`
}

// TerminalStartRequest opens a new interactive integrated terminal (PTY shell).
type TerminalStartRequest struct {
	// Cwd is the absolute or workspace-relative working directory.
	// Empty uses the active workspace root.
	Cwd string `json:"cwd,omitempty"`
	// WorkspaceID selects the workspace for relative cwd resolution (empty = active).
	WorkspaceID string `json:"workspace_id,omitempty"`
	// Title optional tab label.
	Title string `json:"title,omitempty"`
	// Shell overrides $SHELL (defaults to env SHELL or /bin/sh).
	Shell string `json:"shell,omitempty"`
}

// TerminalWriteRequest sends input bytes to a running PTY session.
type TerminalWriteRequest struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

// TerminalResizeRequest sets the PTY window size for an integrated terminal.
// Called from the xterm FitAddon so the shell knows real cols/rows (avoids
// zsh partial-line "%" at the top of new tabs).
type TerminalResizeRequest struct {
	ID   string `json:"id"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// TerminalReadResult is captured output + session meta.
type TerminalReadResult struct {
	Session TerminalSessionView `json:"session"`
	Output  string              `json:"output"`
}

// TerminalStopResult reports stop outcome.
type TerminalStopResult struct {
	Session TerminalSessionView `json:"session"`
	Message string              `json:"message"`
	Stopped bool                `json:"stopped"`
}

// ensurePTY returns the process-local PTY manager for integrated terminals.
// Desktop uses an in-process Manager (not detached CLI workers) so stdin/out
// stream live into the panel without a second terminal implementation.
func (s *Service) ensurePTY() *pty.Manager {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptys == nil {
		// WorkDir left empty; each session sets its own cwd.
		s.ptys = pty.NewManager("")
		s.ptys.IDPrefix = "term"
		pty.RegisterCleanup(s.ptys)
		// Also expose as default for tools that want the same manager.
		if pty.DefaultManager() == nil {
			pty.SetDefaultManager(s.ptys)
		}
	}
	return s.ptys
}

// resolveTerminalCwd resolves cwd for Open in Integrated Terminal / panel open.
// Relative paths are joined under the selected workspace root.
// Empty cwd → workspace root absolute path.
//
// When workspaceID is empty, this never calls EnsureScratchSession / activeRoot
// auto-bind: opening Terminal alone must not land under sessions/ws_sess_*.
// Preference: active folder workspace root → $HOME → process cwd.
func (s *Service) resolveTerminalCwd(workspaceID, cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	// Absolute path: use as-is after Clean (still best-effort under workspace).
	if cwd != "" && filepath.IsAbs(cwd) {
		abs, err := filepath.Abs(cwd)
		if err != nil {
			return "", err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("terminal: cwd %q: %w", abs, err)
		}
		if !st.IsDir() {
			// If a file was selected, open in its parent folder.
			abs = filepath.Dir(abs)
		}
		return abs, nil
	}

	root, err := s.terminalWorkspaceRoot(workspaceID)
	if err != nil {
		// Fall back to process cwd when no workspace is open.
		if cwd == "" {
			return terminalFallbackHome()
		}
		return "", err
	}
	if cwd == "" || cwd == "." {
		return root, nil
	}
	// Workspace-relative folder.
	joined := filepath.Join(root, filepath.FromSlash(cwd))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	// Sandbox: must stay under workspace root.
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("terminal: cwd %q escapes workspace", cwd)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("terminal: cwd %q: %w", abs, err)
	}
	if !st.IsDir() {
		abs = filepath.Dir(abs)
	}
	return abs, nil
}

// terminalWorkspaceRoot resolves a workspace root for the integrated terminal
// without auto-creating session scratch when workspaceID is empty.
func (s *Service) terminalWorkspaceRoot(workspaceID string) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID != "" {
		return s.rootFor(workspaceID)
	}
	if s == nil || s.registry == nil {
		return "", fmt.Errorf("core: no active workspace")
	}
	active := s.registry.Active()
	if active == nil {
		return "", fmt.Errorf("core: no active workspace")
	}
	if workspace.IsSession(*active) {
		// Prefer a registered folder workspace when hub active is a session
		// but the UI opened Terminal unbound (no workspace_id).
		if folder := s.firstFolderWorkspaceRoot(); folder != "" {
			return folder, nil
		}
		return "", fmt.Errorf("core: no folder workspace for terminal")
	}
	return active.RootPath, nil
}

func (s *Service) firstFolderWorkspaceRoot() string {
	if s == nil || s.registry == nil {
		return ""
	}
	for _, w := range s.registry.List() {
		if !workspace.IsSession(w) && strings.TrimSpace(w.RootPath) != "" {
			return w.RootPath
		}
	}
	return ""
}

func terminalFallbackHome() (string, error) {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if st, err := os.Stat(home); err == nil && st.IsDir() {
			return home, nil
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("terminal: no workspace and no cwd: %w", err)
	}
	return wd, nil
}

func defaultInteractiveShell(override string) []string {
	sh := strings.TrimSpace(override)
	if sh == "" {
		sh = strings.TrimSpace(os.Getenv("SHELL"))
	}
	if sh == "" {
		// Portable fallbacks.
		for _, cand := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				sh = cand
				break
			}
		}
	}
	if sh == "" {
		sh = "/bin/sh"
	}
	// Interactive (-i) so prompts work; avoid -l so login profiles don't fight cwd.
	// For zsh, pre-set PROMPT_EOL_MARK empty via env in Session.start; also avoid
	// hanging on incomplete-line mark when the surface paints slightly later.
	return []string{sh, "-i"}
}

func viewFromSession(sess *pty.Session, title string) TerminalSessionView {
	info := sess.Info()
	v := TerminalSessionView{
		ID:       info.ID,
		Title:    title,
		Command:  info.Command,
		Cwd:      info.WorkDir,
		Status:   info.Status,
		PID:      info.PID,
		ExitCode: info.ExitCode,
	}
	if v.Title == "" {
		base := filepath.Base(info.WorkDir)
		if base == "" || base == "." || base == string(filepath.Separator) {
			base = "Terminal"
		}
		v.Title = base
	}
	return v
}

// TerminalStart opens a new interactive PTY shell bound to internal/pty (VAL-IDE-023/024).
func (s *Service) TerminalStart(req TerminalStartRequest) (TerminalSessionView, error) {
	cwd, err := s.resolveTerminalCwd(req.WorkspaceID, req.Cwd)
	if err != nil {
		return TerminalSessionView{}, err
	}
	mgr := s.ensurePTY()

	sess, err := mgr.Start(pty.StartOpts{
		Command: defaultInteractiveShell(req.Shell),
		WorkDir: cwd,
	})
	if err != nil {
		return TerminalSessionView{}, err
	}

	// Attach live fan-out after Start so the session id is known (no race on empty id).
	id := sess.ID
	s.termMu.Lock()
	if s.termTitles == nil {
		s.termTitles = make(map[string]string)
	}
	if s.termFans == nil {
		s.termFans = make(map[string]*termFan)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = filepath.Base(cwd)
	}
	s.termTitles[id] = title
	if s.termFans[id] == nil {
		s.termFans[id] = newTermFan()
	}
	s.termMu.Unlock()
	sess.TeeOutput(&termChunkWriter{onChunk: func(b []byte) {
		s.broadcastTerm(id, string(b))
	}})

	s.mu.Lock()
	s.statusMsg = fmt.Sprintf("Terminal opened · %s", cwd)
	s.mu.Unlock()

	return viewFromSession(sess, title), nil
}

// TerminalList returns all integrated terminal sessions.
func (s *Service) TerminalList() []TerminalSessionView {
	mgr := s.ensurePTY()
	list := mgr.List()
	out := make([]TerminalSessionView, 0, len(list))
	s.termMu.Lock()
	titles := s.termTitles
	s.termMu.Unlock()
	for _, info := range list {
		title := ""
		if titles != nil {
			title = titles[info.ID]
		}
		v := TerminalSessionView{
			ID:       info.ID,
			Title:    title,
			Command:  info.Command,
			Cwd:      info.WorkDir,
			Status:   info.Status,
			PID:      info.PID,
			ExitCode: info.ExitCode,
		}
		if v.Title == "" {
			v.Title = filepath.Base(info.WorkDir)
			if v.Title == "" || v.Title == "." {
				v.Title = "Terminal"
			}
		}
		out = append(out, v)
	}
	return out
}

// TerminalWrite sends data to a session's PTY stdin.
func (s *Service) TerminalWrite(req TerminalWriteRequest) error {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return fmt.Errorf("terminal: id required")
	}
	if req.Data == "" {
		return fmt.Errorf("terminal: empty data")
	}
	mgr := s.ensurePTY()
	sess := mgr.Get(id)
	if sess == nil {
		return fmt.Errorf("terminal: session %q not found", id)
	}
	_, err := sess.WriteString(req.Data)
	return err
}

// TerminalResize applies cols×rows to a running integrated terminal PTY.
func (s *Service) TerminalResize(req TerminalResizeRequest) error {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return fmt.Errorf("terminal: id required")
	}
	if req.Cols <= 0 || req.Rows <= 0 {
		return fmt.Errorf("terminal: invalid size %dx%d", req.Cols, req.Rows)
	}
	// Clamp to a sane TTY range (avoid overflow / ioctl rejection).
	cols := req.Cols
	rows := req.Rows
	if cols > 1000 {
		cols = 1000
	}
	if rows > 500 {
		rows = 500
	}
	mgr := s.ensurePTY()
	sess := mgr.Get(id)
	if sess == nil {
		return fmt.Errorf("terminal: session %q not found", id)
	}
	return sess.SetSize(uint16(cols), uint16(rows))
}

// TerminalRead returns full captured output and session meta.
func (s *Service) TerminalRead(id string) (TerminalReadResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TerminalReadResult{}, fmt.Errorf("terminal: id required")
	}
	mgr := s.ensurePTY()
	sess := mgr.Get(id)
	if sess == nil {
		return TerminalReadResult{}, fmt.Errorf("terminal: session %q not found", id)
	}
	s.termMu.Lock()
	title := ""
	if s.termTitles != nil {
		title = s.termTitles[id]
	}
	s.termMu.Unlock()
	return TerminalReadResult{
		Session: viewFromSession(sess, title),
		Output:  sess.Output(),
	}, nil
}

// TerminalStop stops one session without affecting others (VAL-IDE-024).
func (s *Service) TerminalStop(id string) (TerminalStopResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TerminalStopResult{}, fmt.Errorf("terminal: id required")
	}
	mgr := s.ensurePTY()
	sess := mgr.Get(id)
	if sess == nil {
		return TerminalStopResult{
			Message: fmt.Sprintf("session %q not found (already stopped or never existed)", id),
			Stopped: false,
		}, nil
	}
	s.termMu.Lock()
	title := ""
	if s.termTitles != nil {
		title = s.termTitles[id]
	}
	s.termMu.Unlock()

	// Short grace so tab close feels snappy; SIGKILL still follows if needed.
	// Interactive shells usually exit on SIGTERM/PTY close well under this.
	msg, stopped := mgr.SafeStop(id, 350*time.Millisecond)
	// Keep meta in map briefly so UI can show exit; allow Remove later.
	view := viewFromSession(sess, title)
	// Close fans
	s.termMu.Lock()
	if f := s.termFans[id]; f != nil {
		f.closeAll()
		delete(s.termFans, id)
	}
	s.termMu.Unlock()

	return TerminalStopResult{
		Session: view,
		Message: msg,
		Stopped: stopped,
	}, nil
}

// --- live output fan-out (SSE) ---

// termChunkWriter adapts PTY TeeOutput (io.Writer) into a chunk callback.
type termChunkWriter struct {
	onChunk func([]byte)
}

func (w *termChunkWriter) Write(p []byte) (int, error) {
	if w != nil && w.onChunk != nil && len(p) > 0 {
		// copy: ring path may reuse buffer
		cp := make([]byte, len(p))
		copy(cp, p)
		w.onChunk(cp)
	}
	return len(p), nil
}

type termFan struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
	done bool
}

func newTermFan() *termFan {
	return &termFan{subs: make(map[chan string]struct{})}
}

func (f *termFan) subscribe() chan string {
	ch := make(chan string, 64)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		close(ch)
		return ch
	}
	f.subs[ch] = struct{}{}
	return ch
}

func (f *termFan) unsubscribe(ch chan string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.subs[ch]; ok {
		delete(f.subs, ch)
		close(ch)
	}
}

func (f *termFan) broadcast(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	for ch := range f.subs {
		select {
		case ch <- s:
		default:
			// drop if slow consumer
		}
	}
}

func (f *termFan) closeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = true
	for ch := range f.subs {
		close(ch)
	}
	f.subs = make(map[chan string]struct{})
}

func (s *Service) broadcastTerm(id, chunk string) {
	if id == "" || chunk == "" {
		return
	}
	s.termMu.Lock()
	if s.termFans == nil {
		s.termFans = make(map[string]*termFan)
	}
	f := s.termFans[id]
	if f == nil {
		f = newTermFan()
		s.termFans[id] = f
	}
	s.termMu.Unlock()
	f.broadcast(chunk)
}

// SubscribeTerminalOutput registers for live chunks of a session. Caller must
// unsubscribe via the returned cancel function.
func (s *Service) SubscribeTerminalOutput(id string) (ch <-chan string, cancel func(), err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, fmt.Errorf("terminal: id required")
	}
	if s.ensurePTY().Get(id) == nil {
		return nil, nil, fmt.Errorf("terminal: session %q not found", id)
	}
	s.termMu.Lock()
	if s.termFans == nil {
		s.termFans = make(map[string]*termFan)
	}
	f := s.termFans[id]
	if f == nil {
		f = newTermFan()
		s.termFans[id] = f
	}
	s.termMu.Unlock()
	sub := f.subscribe()
	return sub, func() { f.unsubscribe(sub) }, nil
}
