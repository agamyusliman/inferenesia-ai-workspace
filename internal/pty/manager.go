package pty

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manager owns independent PTY sessions (VAL-TERM-005).
//
// It is safe for concurrent use. Sessions do not share buffers or process
// groups — stopping one never stops another.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	// WorkDir is the default cwd for new sessions (workspace root).
	WorkDir string
	// MaxOutput bounds each session's in-memory capture (bytes).
	MaxOutput int
	// IDPrefix optional prefix for generated ids (tests).
	IDPrefix string
}

// NewManager returns an empty session manager.
func NewManager(workDir string) *Manager {
	return &Manager{
		sessions:  make(map[string]*Session),
		WorkDir:   workDir,
		MaxOutput: DefaultMaxOutput,
	}
}

// StartOpts configures a new PTY session.
type StartOpts struct {
	// Command is argv. Either Command or Shell must be set.
	Command []string
	// Shell, when non-empty, runs as: sh -c Shell (overrides Command).
	Shell string
	// WorkDir overrides Manager.WorkDir for this session.
	WorkDir string
	// ID forces a session id (empty = auto).
	ID string
	// OnOutput optional stream callback for each read chunk.
	OnOutput func([]byte)
}

// Start creates and starts a long-running PTY session. Returns immediately
// with a running Session (does not wait for process exit) — VAL-TERM-001.
func (m *Manager) Start(opts StartOpts) (*Session, error) {
	if m == nil {
		return nil, fmt.Errorf("pty: nil manager")
	}
	args, cmdDisplay, err := resolveArgs(opts)
	if err != nil {
		return nil, err
	}
	wd := strings.TrimSpace(opts.WorkDir)
	if wd == "" {
		wd = m.WorkDir
	}
	if wd == "" {
		wd, _ = os.Getwd()
	}
	if wd != "" {
		if abs, err := filepath.Abs(wd); err == nil {
			wd = abs
		}
	}

	id := strings.TrimSpace(opts.ID)
	if id == "" {
		id = m.newID()
	}

	max := m.MaxOutput
	if max <= 0 {
		max = DefaultMaxOutput
	}

	s := newSession(id, wd, args, max, opts.OnOutput)
	s.Command = cmdDisplay

	m.mu.Lock()
	if _, exists := m.sessions[id]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("pty: session %q already exists", id)
	}
	m.sessions[id] = s
	m.mu.Unlock()

	if err := s.start(); err != nil {
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
		return nil, err
	}
	return s, nil
}

func resolveArgs(opts StartOpts) (args []string, display string, err error) {
	shell := strings.TrimSpace(opts.Shell)
	if shell != "" {
		return []string{"sh", "-c", shell}, shell, nil
	}
	if len(opts.Command) == 0 {
		return nil, "", fmt.Errorf("pty: command or shell required")
	}
	// Copy argv.
	args = append([]string(nil), opts.Command...)
	display = strings.Join(args, " ")
	return args, display, nil
}

func (m *Manager) newID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	prefix := m.IDPrefix
	if prefix == "" {
		prefix = "pty"
	}
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("150405"), hex.EncodeToString(b[:]))
}

// Get returns a session by id, or nil if unknown.
func (m *Manager) Get(id string) *Session {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Stop stops a session by id. Unknown id returns ErrNotFound.
// Already-exited sessions are a safe no-op (nil error) — VAL-TERM-011.
func (m *Manager) Stop(id string, grace time.Duration) error {
	s := m.Get(id)
	if s == nil {
		return ErrNotFound
	}
	return s.Stop(grace)
}

// SafeStop is like Stop but never returns an error for unknown or already-stopped
// sessions. Returns a short human message suitable for CLI (exit 0) — VAL-TERM-011.
func (m *Manager) SafeStop(id string, grace time.Duration) (message string, stopped bool) {
	s := m.Get(id)
	if s == nil {
		return fmt.Sprintf("session %q not found (already stopped or never existed)", id), false
	}
	st := s.Status()
	if st == StatusExited || st == StatusStopped || st == StatusError {
		return fmt.Sprintf("session %q already %s (no-op)", id, st), false
	}
	_ = s.Stop(grace)
	return fmt.Sprintf("stopped session %q", id), true
}

// Remove drops a finished session from the map (optional cleanup).
func (m *Manager) Remove(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

// List returns snapshots of all sessions sorted by id.
func (m *Manager) List() []Info {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	out := make([]Info, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.Info())
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// StopAll stops every session (used on CLI exit cleanup).
func (m *Manager) StopAll(grace time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Stop(id, grace)
	}
}

// ErrNotFound is returned when a session id is unknown.
var ErrNotFound = fmt.Errorf("pty: session not found")

// IsNotFound reports whether err is ErrNotFound.
func IsNotFound(err error) bool {
	return err == ErrNotFound
}
