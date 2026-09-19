// Package pty manages long-running pseudo-terminal sessions for Inferenesia.
//
// The Manager is transport-agnostic and used by CLI and (later) desktop/agent tools.
// Sessions capture streaming output, report exit codes, support clean stop, and
// are fully independent of each other (VAL-TERM-001..005).
package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	creackpty "github.com/creack/pty"
)

// Status is the lifecycle state of a Session.
type Status string

const (
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusExited   Status = "exited"
	StatusStopped  Status = "stopped"
	StatusError    Status = "error"
)

// DefaultMaxOutput is the in-memory ring buffer size for captured PTY bytes.
// Core capture uses this bound; advanced tail may tune further (VAL-TERM-006).
const DefaultMaxOutput = 256 << 10 // 256 KiB

// Session is one long-running PTY process with captured output.
type Session struct {
	ID      string
	Command string // display / reconstruct string
	Args    []string
	WorkDir string

	mu       sync.Mutex
	status   Status
	pid      int
	exitCode int
	exitSet  bool
	started  time.Time
	ended    time.Time
	errMsg   string

	cmd  *exec.Cmd
	ptmx *os.File
	buf  *ringBuffer

	// waitOnce ensures WaitProcess runs once.
	waitOnce sync.Once
	done     chan struct{}

	// onOutput is optional; called with every chunk (for streaming subscribers).
	onOutput func([]byte)
}

// Info is a snapshot of session metadata safe for callers.
type Info struct {
	ID       string    `json:"id"`
	Command  string    `json:"command"`
	WorkDir  string    `json:"work_dir,omitempty"`
	Status   Status    `json:"status"`
	PID      int       `json:"pid"`
	ExitCode *int      `json:"exit_code,omitempty"`
	Started  time.Time `json:"started_at"`
	Ended    time.Time `json:"ended_at,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Info returns a consistent snapshot.
func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := Info{
		ID:      s.ID,
		Command: s.Command,
		WorkDir: s.WorkDir,
		Status:  s.status,
		PID:     s.pid,
		Started: s.started,
		Ended:   s.ended,
		Error:   s.errMsg,
	}
	if s.exitSet {
		c := s.exitCode
		info.ExitCode = &c
	}
	return info
}

// PID returns the OS process id (0 if not started).
func (s *Session) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

// Status returns the current lifecycle status.
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// ExitCode returns the process exit code and whether it is known.
func (s *Session) ExitCode() (code int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode, s.exitSet
}

// Output returns all currently captured output (bounded by the ring).
func (s *Session) Output() string {
	s.mu.Lock()
	buf := s.buf
	s.mu.Unlock()
	if buf == nil {
		return ""
	}
	return buf.String()
}

// Wait blocks until the process exits (or the session is already done).
func (s *Session) Wait() {
	if s == nil || s.done == nil {
		return
	}
	<-s.done
}

// WaitTimeout waits up to d for process exit. Returns true if exited.
func (s *Session) WaitTimeout(d time.Duration) bool {
	if s == nil || s.done == nil {
		return true
	}
	select {
	case <-s.done:
		return true
	case <-time.After(d):
		return false
	}
}

// DefaultWinsize is used when a session is started without an explicit size.
// A zero/unknown size makes zsh emit the partial-line "%" mark at the top of
// integrated terminals when the first prompt is painted.
const (
	DefaultCols = 120
	DefaultRows = 30
)

// start launches the command under a PTY. Must be called once under Manager lock.
func (s *Session) start() error {
	if len(s.Args) == 0 {
		return errors.New("pty: empty command")
	}

	cmd := exec.Command(s.Args[0], s.Args[1:]...)
	if s.WorkDir != "" {
		cmd.Dir = s.WorkDir
	}
	// Inherit env but force a real terminal type + dimensions so interactive
	// shells paint a clean first prompt.
	env := append([]string(nil), os.Environ()...)
	env = setEnvKV(env, "TERM", "xterm-256color")
	env = setEnvKV(env, "COLORTERM", "truecolor")
	env = setEnvKV(env, "COLUMNS", fmt.Sprintf("%d", DefaultCols))
	env = setEnvKV(env, "LINES", fmt.Sprintf("%d", DefaultRows))
	// zsh PROMPT_SP prints a reverse "%" on partial lines (including a poorly
	// sized/bootstrapped PTY). Empty mark suppresses it in integrated terminals
	// the way VS Code / most embbeded shells expect.
	env = setEnvKV(env, "PROMPT_EOL_MARK", "")
	cmd.Env = env
	// creack/pty.Start creates a new session (Setsid) so the child is process-group
	// leader; Stop can then signal -pid for the whole tree.

	// StartWithSize so the first shell paint already has a non-zero winsize.
	ptmx, err := creackpty.StartWithSize(cmd, &creackpty.Winsize{
		Rows: DefaultRows,
		Cols: DefaultCols,
	})
	if err != nil {
		s.mu.Lock()
		s.status = StatusError
		s.errMsg = err.Error()
		s.mu.Unlock()
		close(s.done)
		return fmt.Errorf("pty start: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.ptmx = ptmx
	s.pid = cmd.Process.Pid
	s.status = StatusRunning
	s.started = time.Now()
	s.mu.Unlock()

	// Stream PTY output into ring buffer.
	go s.readLoop()
	// Reap process and record exit code.
	go s.waitLoop()
	return nil
}

// setEnvKV replaces or appends KEY=value in a process env slice.
func setEnvKV(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// SetSize resizes the PTY window (TIOCSWINSZ). cols/rows must be > 0.
// Used by the integrated terminal surface after FitAddon measures the host.
func (s *Session) SetSize(cols, rows uint16) error {
	if s == nil {
		return errors.New("pty: nil session")
	}
	if cols == 0 || rows == 0 {
		return fmt.Errorf("pty: invalid size %dx%d", cols, rows)
	}
	s.mu.Lock()
	ptmx := s.ptmx
	st := s.status
	s.mu.Unlock()
	if ptmx == nil || st != StatusRunning {
		return fmt.Errorf("pty: session %s not running", s.ID)
	}
	return creackpty.Setsize(ptmx, &creackpty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.mu.Lock()
			if s.buf != nil {
				s.buf.Write(chunk)
			}
			cb := s.onOutput
			s.mu.Unlock()
			if cb != nil {
				cb(chunk)
			}
		}
		if err != nil {
			// EOF / closed pty is normal on process exit.
			return
		}
	}
}

func (s *Session) waitLoop() {
	s.waitOnce.Do(func() {
		var waitErr error
		if s.cmd != nil && s.cmd.Process != nil {
			waitErr = s.cmd.Wait()
		}
		code := 0
		if waitErr != nil {
			if ee, ok := waitErr.(*exec.ExitError); ok {
				code = ee.ExitCode()
				if code < 0 {
					if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
						// Convention: 128 + signal for processes killed by signal.
						code = 128 + int(ws.Signal())
					}
				}
			} else {
				code = -1
			}
		}

		s.mu.Lock()
		// Don't overwrite Stopped if Stop() already marked it.
		if s.status == StatusRunning || s.status == StatusStarting {
			s.status = StatusExited
		}
		s.exitCode = code
		s.exitSet = true
		s.ended = time.Now()
		ptmx := s.ptmx
		s.mu.Unlock()

		if ptmx != nil {
			_ = ptmx.Close()
		}
		close(s.done)
	})
}

// Stop sends SIGTERM to the process group, then SIGKILL after grace if needed.
// It is safe to call multiple times. Returns nil when the process is gone.
func (s *Session) Stop(grace time.Duration) error {
	if s == nil {
		return nil
	}
	if grace <= 0 {
		grace = 2 * time.Second
	}

	s.mu.Lock()
	st := s.status
	pid := s.pid
	cmd := s.cmd
	s.mu.Unlock()

	if st == StatusExited || st == StatusStopped || st == StatusError {
		// Already done — safe no-op for session object.
		return nil
	}
	if pid <= 0 || cmd == nil || cmd.Process == nil {
		s.mu.Lock()
		s.status = StatusStopped
		s.ended = time.Now()
		s.mu.Unlock()
		return nil
	}

	// Signal the whole process group (negative pid) when Setpgid was used.
	pgid := -pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
	// Also signal the direct process in case group signal fails.
	_ = cmd.Process.Signal(syscall.SIGTERM)

	if s.WaitTimeout(grace) {
		s.mu.Lock()
		if s.status == StatusRunning || s.status == StatusStarting {
			s.status = StatusStopped
		} else if s.status == StatusExited {
			// Distinguish voluntary exit vs stop: prefer stopped when we requested it.
			s.status = StatusStopped
		}
		s.mu.Unlock()
		return nil
	}

	_ = syscall.Kill(pgid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
	// After SIGKILL, wait only briefly so a slow reaper cannot stall callers
	// (desktop tab close felt like multi-second hangs with full grace ×2).
	killWait := grace
	if killWait > 200*time.Millisecond {
		killWait = 200 * time.Millisecond
	}
	s.WaitTimeout(killWait)
	s.mu.Lock()
	s.status = StatusStopped
	if !s.exitSet {
		s.exitCode = 128 + int(syscall.SIGKILL)
		s.exitSet = true
	}
	if s.ended.IsZero() {
		s.ended = time.Now()
	}
	s.mu.Unlock()
	return nil
}

// WriteInput writes bytes to the PTY (stdin of the child). Used by advanced
// feature tests but provided on Session so core can share one type.
func (s *Session) WriteInput(b []byte) (int, error) {
	s.mu.Lock()
	ptmx := s.ptmx
	st := s.status
	s.mu.Unlock()
	if ptmx == nil || st != StatusRunning {
		return 0, fmt.Errorf("pty: session %s not running", s.ID)
	}
	return ptmx.Write(b)
}

// WriteString is convenience for WriteInput.
func (s *Session) WriteString(str string) (int, error) {
	return s.WriteInput([]byte(str))
}

// ClosePTY closes the master side (best-effort).
func (s *Session) ClosePTY() {
	s.mu.Lock()
	ptmx := s.ptmx
	s.ptmx = nil
	s.mu.Unlock()
	if ptmx != nil {
		_ = ptmx.Close()
	}
}

// Running reports whether the OS process still appears alive.
func (s *Session) Running() bool {
	s.mu.Lock()
	pid := s.pid
	st := s.status
	s.mu.Unlock()
	if st != StatusRunning || pid <= 0 {
		return false
	}
	// Signal 0 checks existence without killing.
	err := syscall.Kill(pid, 0)
	return err == nil
}

// newSession constructs an unstarted session.
func newSession(id, workDir string, args []string, maxOutput int, onOutput func([]byte)) *Session {
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutput
	}
	cmdStr := strings.Join(args, " ")
	return &Session{
		ID:       id,
		Command:  cmdStr,
		Args:     append([]string(nil), args...),
		WorkDir:  workDir,
		status:   StatusStarting,
		buf:      newRingBuffer(maxOutput),
		done:     make(chan struct{}),
		onOutput: onOutput,
	}
}

// TeeOutput writes every captured chunk to w (in addition to the ring buffer).
// Must be set before Start; used by the detached worker to persist logs.
func (s *Session) TeeOutput(w io.Writer) {
	if w == nil {
		return
	}
	s.mu.Lock()
	prev := s.onOutput
	s.onOutput = func(b []byte) {
		if prev != nil {
			prev(b)
		}
		_, _ = w.Write(b)
	}
	s.mu.Unlock()
}
