package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/pty"
)

// Terminal tool names exposed to the model (VAL-TERM-007/008/012).
const (
	NameTerminalRead  = "terminal_read"
	NameTerminalWrite = "terminal_write"
	NameTerminalList  = "terminal_list"
	NameTerminalStart = "terminal_start"
	NameTerminalStop  = "terminal_stop"
)

// TerminalBackend abstracts PTY session ops so tools can use the on-disk detached
// store and/or an in-process Manager.
type TerminalBackend interface {
	Start(opts pty.StartOpts) (pty.SessionMeta, error)
	Stop(id string) (pty.SessionMeta, string, error)
	Read(id string, maxBytes int) (meta pty.SessionMeta, output string, err error)
	Write(id string, data string) error
	List() ([]pty.SessionMeta, error)
}

// DetachedTerminal is the default backend: sessions live under PersistRoot and
// survive CLI process exit when explicitly detached (worker process).
type DetachedTerminal struct {
	Root       string
	Binary     string // path to inferenesia for DetachStart
	MaxRead    int
	WorkDir    string
	Grace      time.Duration
}

// NewDetachedTerminal builds a backend using YURA_AI_PTY_DIR / ~/.inferenesia/pty.
func NewDetachedTerminal(workDir string) (*DetachedTerminal, error) {
	root, err := pty.PersistRoot()
	if err != nil {
		return nil, err
	}
	bin, err := os.Executable()
	if err != nil {
		bin = ""
	}
	return &DetachedTerminal{
		Root:    root,
		Binary:  bin,
		MaxRead: 64 << 10,
		WorkDir: workDir,
		Grace:   2 * time.Second,
	}, nil
}

func (d *DetachedTerminal) Start(opts pty.StartOpts) (pty.SessionMeta, error) {
	if d == nil {
		return pty.SessionMeta{}, fmt.Errorf("terminal: nil backend")
	}
	if opts.WorkDir == "" {
		opts.WorkDir = d.WorkDir
	}
	if d.Binary == "" {
		return pty.SessionMeta{}, fmt.Errorf("terminal: worker binary not resolved")
	}
	return pty.DetachStart(d.Root, d.Binary, opts)
}

func (d *DetachedTerminal) Stop(id string) (pty.SessionMeta, string, error) {
	meta, err := pty.DetachStop(d.Root, id, d.Grace)
	if err != nil {
		if pty.IsNotFound(err) {
			return pty.SessionMeta{}, fmt.Sprintf("session %q not found (already stopped or never existed)", id), nil
		}
		return pty.SessionMeta{}, "", err
	}
	if meta.Status == pty.StatusExited || meta.Status == pty.StatusStopped {
		// DetachStop returns existing meta for already-stopped — treat as no-op message.
		alive := pty.ProcessAlive(meta.PID)
		if !alive {
			return meta, fmt.Sprintf("stopped session=%s status=%s pid=%d", meta.ID, meta.Status, meta.PID), nil
		}
	}
	return meta, fmt.Sprintf("stopped session=%s status=%s pid=%d", meta.ID, meta.Status, meta.PID), nil
}

func (d *DetachedTerminal) Read(id string, maxBytes int) (pty.SessionMeta, string, error) {
	if maxBytes <= 0 {
		maxBytes = d.MaxRead
	}
	if maxBytes <= 0 {
		maxBytes = 64 << 10
	}
	meta, err := pty.LoadMeta(d.Root, id)
	if err != nil {
		if os.IsNotExist(err) {
			return pty.SessionMeta{}, "", fmt.Errorf("session %q not found", id)
		}
		return pty.SessionMeta{}, "", err
	}
	out, err := pty.ReadOutputFile(d.Root, id, maxBytes)
	if err != nil {
		return meta, "", err
	}
	return meta, out, nil
}

func (d *DetachedTerminal) Write(id string, data string) error {
	return pty.WriteInputToSession(d.Root, id, []byte(data))
}

func (d *DetachedTerminal) List() ([]pty.SessionMeta, error) {
	return pty.ListMeta(d.Root)
}

// ManagerTerminal is an in-process TerminalBackend (tests + foreground).
// Sessions do not survive process exit; CleanupNow stops them (VAL-TERM-009).
type ManagerTerminal struct {
	Mgr     *pty.Manager
	Root    string // optional FIFO root for Write when needed
	MaxRead int
}

// NewManagerTerminal wraps a PTY manager and registers it for exit cleanup.
func NewManagerTerminal(mgr *pty.Manager, fifoRoot string) *ManagerTerminal {
	if mgr != nil {
		pty.RegisterCleanup(mgr)
		pty.SetDefaultManager(mgr)
	}
	return &ManagerTerminal{Mgr: mgr, Root: fifoRoot, MaxRead: 64 << 10}
}

func (m *ManagerTerminal) Start(opts pty.StartOpts) (pty.SessionMeta, error) {
	if m == nil || m.Mgr == nil {
		return pty.SessionMeta{}, fmt.Errorf("terminal: nil manager")
	}
	// Tools pass shell-form commands via StartOpts.Shell from terminal_start.
	// If only Shell is set, resolveArgs in Manager.Start handles it.
	s, err := m.Mgr.Start(opts)
	if err != nil {
		return pty.SessionMeta{}, err
	}
	info := s.Info()
	var exit *int
	if info.ExitCode != nil {
		exit = info.ExitCode
	}
	return pty.SessionMeta{
		ID:       info.ID,
		Command:  info.Command,
		WorkDir:  info.WorkDir,
		Status:   info.Status,
		PID:      info.PID,
		ExitCode: exit,
		Started:  info.Started,
		Ended:    info.Ended,
		Error:    info.Error,
		Detached: false,
	}, nil
}

func (m *ManagerTerminal) Stop(id string) (pty.SessionMeta, string, error) {
	if m == nil || m.Mgr == nil {
		return pty.SessionMeta{}, "", fmt.Errorf("terminal: nil manager")
	}
	msg, stopped := m.Mgr.SafeStop(id, 2*time.Second)
	s := m.Mgr.Get(id)
	if s == nil {
		return pty.SessionMeta{}, msg, nil
	}
	info := s.Info()
	meta := pty.SessionMeta{
		ID: info.ID, Command: info.Command, Status: info.Status, PID: info.PID,
	}
	if info.ExitCode != nil {
		meta.ExitCode = info.ExitCode
	}
	if !stopped {
		return meta, msg, nil
	}
	return meta, msg, nil
}

func (m *ManagerTerminal) Read(id string, maxBytes int) (pty.SessionMeta, string, error) {
	if m == nil || m.Mgr == nil {
		return pty.SessionMeta{}, "", fmt.Errorf("terminal: nil manager")
	}
	s := m.Mgr.Get(id)
	if s == nil {
		// Fall back to disk meta if Root set.
		if m.Root != "" {
			meta, err := pty.LoadMeta(m.Root, id)
			if err != nil {
				return pty.SessionMeta{}, "", fmt.Errorf("session %q not found", id)
			}
			out, err := pty.ReadOutputFile(m.Root, id, maxBytes)
			return meta, out, err
		}
		return pty.SessionMeta{}, "", fmt.Errorf("session %q not found", id)
	}
	info := s.Info()
	out := s.Output()
	if maxBytes <= 0 {
		maxBytes = m.MaxRead
	}
	if maxBytes > 0 && len(out) > maxBytes {
		out = out[len(out)-maxBytes:]
	}
	meta := pty.SessionMeta{
		ID: info.ID, Command: info.Command, Status: info.Status, PID: info.PID,
		WorkDir: info.WorkDir, Started: info.Started, Ended: info.Ended, Error: info.Error,
	}
	if info.ExitCode != nil {
		meta.ExitCode = info.ExitCode
	}
	return meta, out, nil
}

func (m *ManagerTerminal) Write(id string, data string) error {
	if m == nil || m.Mgr == nil {
		return fmt.Errorf("terminal: nil manager")
	}
	s := m.Mgr.Get(id)
	if s == nil {
		if m.Root != "" {
			return pty.WriteInputToSession(m.Root, id, []byte(data))
		}
		return fmt.Errorf("session %q not found", id)
	}
	_, err := s.WriteString(data)
	return err
}

func (m *ManagerTerminal) List() ([]pty.SessionMeta, error) {
	if m == nil || m.Mgr == nil {
		return nil, nil
	}
	infos := m.Mgr.List()
	out := make([]pty.SessionMeta, 0, len(infos))
	for _, info := range infos {
		meta := pty.SessionMeta{
			ID: info.ID, Command: info.Command, Status: info.Status, PID: info.PID,
			WorkDir: info.WorkDir, Started: info.Started, Ended: info.Ended, Error: info.Error,
		}
		if info.ExitCode != nil {
			meta.ExitCode = info.ExitCode
		}
		out = append(out, meta)
	}
	return out, nil
}

// SetTerminalBackend injects a TerminalBackend on the registry (chat/CLI).
func (r *Registry) SetTerminalBackend(tb TerminalBackend) {
	if r == nil {
		return
	}
	r.terminal = tb
}

// TerminalBackend returns the configured backend (may be nil).
func (r *Registry) TerminalBackend() TerminalBackend {
	if r == nil {
		return nil
	}
	return r.terminal
}

func terminalDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameTerminalRead,
				Description: "Read the current captured output (tail) of a long-running PTY terminal session started with terminal_start or `inferenesia terminal start`. Returns status, pid, exit code if known, and recent log lines. Use this to answer what a dev server or background process printed.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "session_id": { "type": "string", "description": "PTY session id from terminal_start or terminal list" },
    "max_bytes": { "type": "integer", "description": "Max output bytes to return (default 65536, tail)" }
  },
  "required": ["session_id"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameTerminalWrite,
				Description: "Send input (stdin) to a running PTY session. Example: start `cat`, then terminal_write with text \"hello-pty\\n\" and later terminal_read to see the echo.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "session_id": { "type": "string", "description": "PTY session id" },
    "data": { "type": "string", "description": "Bytes/text to write to the PTY (include \\n for Enter)" }
  },
  "required": ["session_id", "data"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameTerminalList,
				Description: "List active and known PTY sessions with id, command, pid, and status. Empty when no sessions exist.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameTerminalStart,
				Description: "Start a long-running process in a PTY session (non-blocking). Prefer this for npm run dev, servers, watchers. Short one-shot commands should use the shell tool instead. Returns session id and pid.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": { "type": "string", "description": "Shell command string (run via sh -c), e.g. npm run dev" },
    "id": { "type": "string", "description": "Optional forced session id" }
  },
  "required": ["command"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameTerminalStop,
				Description: "Stop a PTY session by id (SIGTERM then SIGKILL). Already-stopped or unknown ids are a safe no-op with a clear message.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "session_id": { "type": "string", "description": "PTY session id to stop" }
  },
  "required": ["session_id"]
}`),
			},
		},
	}
}

func (r *Registry) ensureTerminal() (TerminalBackend, error) {
	if r == nil {
		return nil, fmt.Errorf("terminal: registry is nil")
	}
	if r.terminal != nil {
		return r.terminal, nil
	}
	// Lazy default: detached backend under workspace cwd.
	tb, err := NewDetachedTerminal(r.shellWD)
	if err != nil {
		return nil, err
	}
	r.terminal = tb
	return tb, nil
}

type terminalReadArgs struct {
	SessionID string `json:"session_id"`
	MaxBytes  int    `json:"max_bytes"`
}

type terminalWriteArgs struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"`
}

type terminalStartArgs struct {
	Command string `json:"command"`
	ID      string `json:"id"`
}

type terminalStopArgs struct {
	SessionID string `json:"session_id"`
}

func (r *Registry) execTerminalRead(call Call) Result {
	tb, err := r.ensureTerminal()
	if err != nil {
		return Result{Name: NameTerminalRead, Content: err.Error(), IsError: true}
	}
	var args terminalReadArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameTerminalRead, Content: fmt.Sprintf("terminal_read: invalid JSON: %v", err), IsError: true}
	}
	id := strings.TrimSpace(args.SessionID)
	if id == "" {
		return Result{Name: NameTerminalRead, Content: "terminal_read: session_id is required", IsError: true}
	}
	meta, out, err := tb.Read(id, args.MaxBytes)
	if err != nil {
		return Result{Name: NameTerminalRead, Content: err.Error(), IsError: true}
	}
	code := "-"
	if meta.ExitCode != nil {
		code = fmt.Sprintf("%d", *meta.ExitCode)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "session=%s status=%s pid=%d exit=%s cmd=%q\n",
		meta.ID, meta.Status, meta.PID, code, meta.Command)
	fmt.Fprintf(&b, "--- output ---\n%s", out)
	if out != "" && !strings.HasSuffix(out, "\n") {
		b.WriteByte('\n')
	}
	return Result{Name: NameTerminalRead, Content: b.String()}
}

func (r *Registry) execTerminalWrite(call Call) Result {
	tb, err := r.ensureTerminal()
	if err != nil {
		return Result{Name: NameTerminalWrite, Content: err.Error(), IsError: true}
	}
	var args terminalWriteArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameTerminalWrite, Content: fmt.Sprintf("terminal_write: invalid JSON: %v", err), IsError: true}
	}
	id := strings.TrimSpace(args.SessionID)
	if id == "" {
		return Result{Name: NameTerminalWrite, Content: "terminal_write: session_id is required", IsError: true}
	}
	if err := tb.Write(id, args.Data); err != nil {
		return Result{Name: NameTerminalWrite, Content: fmt.Sprintf("terminal_write failed: %v", err), IsError: true}
	}
	return Result{
		Name:    NameTerminalWrite,
		Content: fmt.Sprintf("wrote %d bytes to session %s", len(args.Data), id),
	}
}

func (r *Registry) execTerminalList(_ Call) Result {
	tb, err := r.ensureTerminal()
	if err != nil {
		return Result{Name: NameTerminalList, Content: err.Error(), IsError: true}
	}
	list, err := tb.List()
	if err != nil {
		return Result{Name: NameTerminalList, Content: err.Error(), IsError: true}
	}
	if len(list) == 0 {
		return Result{Name: NameTerminalList, Content: "(no sessions)"}
	}
	var b strings.Builder
	for _, m := range list {
		fmt.Fprintf(&b, "%s\n", pty.FormatMetaLine(m))
	}
	return Result{Name: NameTerminalList, Content: strings.TrimSpace(b.String())}
}

func (r *Registry) execTerminalStart(call Call) Result {
	tb, err := r.ensureTerminal()
	if err != nil {
		return Result{Name: NameTerminalStart, Content: err.Error(), IsError: true}
	}
	var args terminalStartArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameTerminalStart, Content: fmt.Sprintf("terminal_start: invalid JSON: %v", err), IsError: true}
	}
	cmd := strings.TrimSpace(args.Command)
	if cmd == "" {
		return Result{Name: NameTerminalStart, Content: "terminal_start: command is required", IsError: true}
	}
	meta, err := tb.Start(pty.StartOpts{
		Shell: cmd,
		ID:    strings.TrimSpace(args.ID),
	})
	if err != nil {
		return Result{Name: NameTerminalStart, Content: fmt.Sprintf("terminal_start failed: %v", err), IsError: true}
	}
	return Result{
		Name: NameTerminalStart,
		Content: fmt.Sprintf("started session=%s pid=%d status=%s cmd=%q",
			meta.ID, meta.PID, meta.Status, meta.Command),
	}
}

func (r *Registry) execTerminalStop(call Call) Result {
	tb, err := r.ensureTerminal()
	if err != nil {
		return Result{Name: NameTerminalStop, Content: err.Error(), IsError: true}
	}
	var args terminalStopArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameTerminalStop, Content: fmt.Sprintf("terminal_stop: invalid JSON: %v", err), IsError: true}
	}
	id := strings.TrimSpace(args.SessionID)
	if id == "" {
		return Result{Name: NameTerminalStop, Content: "terminal_stop: session_id is required", IsError: true}
	}
	meta, msg, err := tb.Stop(id)
	if err != nil {
		return Result{Name: NameTerminalStop, Content: err.Error(), IsError: true}
	}
	if msg == "" {
		msg = fmt.Sprintf("session=%s status=%s", meta.ID, meta.Status)
	}
	return Result{Name: NameTerminalStop, Content: msg}
}
