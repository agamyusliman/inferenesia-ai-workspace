// Package workspace manages the multi-project registry (workspace hub).
// Persists to ~/.yura-ai/workspaces.json (or $YURA_AI_HOME/workspaces.json).
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// RegistryFileName is the registry JSON under the config home.
	RegistryFileName = "workspaces.json"
	// DirPerm for workspace state dirs under the config home.
	DirPerm = 0o700
	// FilePerm for the registry file (no secrets by default, keep private).
	FilePerm = 0o600
)

// Kind values for Workspace.Kind.
const (
	KindFolder  = "folder"
	KindSession = "session"
)

// Workspace is a registered project root or session scratch.
type Workspace struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	RootPath     string    `json:"root_path"`
	// Kind is "folder" (default) or "session" (no project folder / scratch).
	Kind         string    `json:"kind,omitempty"`
	Pinned       bool      `json:"pinned,omitempty"`
	LastOpenedAt time.Time `json:"last_opened_at"`
	// Broken is true when RootPath no longer exists on disk.
	Broken bool `json:"broken,omitempty"`
}

// Registry holds the workspace list and active selection.
type Registry struct {
	mu       sync.Mutex
	path     string // absolute path to workspaces.json
	items    []Workspace
	activeID string
}

type registryFile struct {
	Version  int         `json:"version"`
	ActiveID string      `json:"active_id"`
	Items    []Workspace `json:"items"`
}

// Path returns the absolute registry file path for a config home.
func Path(configHome string) string {
	return filepath.Join(filepath.Clean(configHome), RegistryFileName)
}

// Open loads (or creates) the registry at configHome/workspaces.json.
func Open(configHome string) (*Registry, error) {
	home := filepath.Clean(configHome)
	if home == "" || home == "." {
		return nil, fmt.Errorf("workspace: empty config home")
	}
	if err := os.MkdirAll(home, DirPerm); err != nil {
		return nil, fmt.Errorf("workspace: create home: %w", err)
	}
	r := &Registry{path: Path(home)}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// List returns a copy of workspaces (pinned first, then newest last_opened_at).
func (r *Registry) List() []Workspace {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Workspace, len(r.items))
	copy(out, r.items)
	for i := range out {
		out[i].Broken = !dirExists(out[i].RootPath)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		ni, nj := sessionIDNano(out[i].ID), sessionIDNano(out[j].ID)
		if ni != nj {
			return ni > nj
		}
		return out[i].LastOpenedAt.After(out[j].LastOpenedAt)
	})
	return out
}

func sessionIDNano(id string) int64 {
	const p = "ws_sess_"
	if !strings.HasPrefix(id, p) {
		return 0
	}
	var n int64
	for _, c := range id[len(p):] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// ActiveID returns the currently active workspace id (may be empty).
func (r *Registry) ActiveID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeID
}

// Active returns the active workspace or nil.
func (r *Registry) Active() *Workspace {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeID == "" {
		return nil
	}
	for i := range r.items {
		if r.items[i].ID == r.activeID {
			w := r.items[i]
			w.Broken = !dirExists(w.RootPath)
			return &w
		}
	}
	return nil
}

// Get returns a workspace by id.
func (r *Registry) Get(id string) (*Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.items {
		if r.items[i].ID == id {
			w := r.items[i]
			w.Broken = !dirExists(w.RootPath)
			return &w, nil
		}
	}
	return nil, fmt.Errorf("workspace: not found: %s", id)
}

const ScratchSessionID = "ws_session"

const ScratchSessionName = "Session"

// IsSession reports whether the workspace is a session scratch (no project folder).
func IsSession(w Workspace) bool {
	if w.Kind == KindSession {
		return true
	}
	if w.ID == ScratchSessionID {
		return true
	}
	if strings.Contains(filepath.ToSlash(w.RootPath), "/session-scratch") {
		return true
	}
	if strings.Contains(filepath.ToSlash(w.RootPath), "/sessions/") {
		return true
	}
	return false
}

// CreateSession registers a new named session workspace under
// <configHome>/sessions/<id> and makes it active. Each call creates a distinct
// session (rename/delete are independent of other sessions).
func (r *Registry) CreateSession(name string) (Workspace, error) {
	if r == nil {
		return Workspace{}, fmt.Errorf("workspace: nil registry")
	}
	configHome := filepath.Dir(r.path)
	if configHome == "" || configHome == "." {
		return Workspace{}, fmt.Errorf("workspace: empty config home")
	}
	id := fmt.Sprintf("ws_sess_%d", time.Now().UnixNano())
	label := strings.TrimSpace(name)
	if label == "" {
		label = "New chat"
	}
	root := filepath.Join(configHome, "sessions", id)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Workspace{}, fmt.Errorf("workspace: create session dir: %w", err)
	}
	abs, err := resolveRoot(root)
	if err != nil {
		return Workspace{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	w := Workspace{
		ID:           id,
		Name:         label,
		RootPath:     abs,
		Kind:         KindSession,
		LastOpenedAt: now,
	}
	r.items = append([]Workspace{w}, r.items...)
	r.activeID = w.ID
	if err := r.saveLocked(); err != nil {
		return Workspace{}, err
	}
	return w, nil
}

// EnsureScratchSession activates the most recent session, or creates Session.
func (r *Registry) EnsureScratchSession() (Workspace, error) {
	if r == nil {
		return Workspace{}, fmt.Errorf("workspace: nil registry")
	}
	r.mu.Lock()
	var best *Workspace
	for i := range r.items {
		if IsSession(r.items[i]) && dirExists(r.items[i].RootPath) {
			if best == nil || r.items[i].LastOpenedAt.After(best.LastOpenedAt) {
				cp := r.items[i]
				best = &cp
			}
		}
	}
	if best != nil {
		id := best.ID
		r.mu.Unlock()
		return r.Switch(id)
	}
	r.mu.Unlock()
	return r.CreateSession(ScratchSessionName)
}

// Rename sets the display name for a workspace or session entry.
func (r *Registry) Rename(id, name string) (Workspace, error) {
	if r == nil {
		return Workspace{}, fmt.Errorf("workspace: nil registry")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Workspace{}, fmt.Errorf("workspace: empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.items {
		if r.items[i].ID == id {
			r.items[i].Name = name
			if err := r.saveLocked(); err != nil {
				return Workspace{}, err
			}
			return r.items[i], nil
		}
	}
	return Workspace{}, fmt.Errorf("workspace: not found: %s", id)
}

// OpenPath registers a directory as a workspace (dedupe by resolved absolute path),
// marks it active, and persists. Returns the workspace.
func (r *Registry) OpenPath(root string) (Workspace, error) {
	abs, err := resolveRoot(root)
	if err != nil {
		return Workspace{}, err
	}
	if !dirExists(abs) {
		return Workspace{}, fmt.Errorf("workspace: path is not a directory: %s", abs)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Dedupe by absolute root path.
	for i := range r.items {
		if samePath(r.items[i].RootPath, abs) {
			r.items[i].LastOpenedAt = time.Now().UTC()
			r.items[i].Broken = false
			if r.items[i].Kind == "" {
				r.items[i].Kind = KindFolder
			}
			r.activeID = r.items[i].ID
			if err := r.saveLocked(); err != nil {
				return Workspace{}, err
			}
			return r.items[i], nil
		}
	}

	now := time.Now().UTC()
	w := Workspace{
		ID:           newID(),
		Name:         filepath.Base(abs),
		RootPath:     abs,
		Kind:         KindFolder,
		LastOpenedAt: now,
	}
	r.items = append(r.items, w)
	r.activeID = w.ID
	if err := r.saveLocked(); err != nil {
		return Workspace{}, err
	}
	return w, nil
}

// Switch sets the active workspace by id and updates last_opened_at.
func (r *Registry) Switch(id string) (Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.items {
		if r.items[i].ID == id {
			if !dirExists(r.items[i].RootPath) {
				r.items[i].Broken = true
				_ = r.saveLocked()
				return Workspace{}, fmt.Errorf("workspace: path missing: %s", r.items[i].RootPath)
			}
			r.items[i].LastOpenedAt = time.Now().UTC()
			r.items[i].Broken = false
			r.activeID = id
			if err := r.saveLocked(); err != nil {
				return Workspace{}, err
			}
			return r.items[i], nil
		}
	}
	return Workspace{}, fmt.Errorf("workspace: not found: %s", id)
}

// Remove deletes a workspace from the registry (does not delete project files).
func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := -1
	for i := range r.items {
		if r.items[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("workspace: not found: %s", id)
	}
	r.items = append(r.items[:idx], r.items[idx+1:]...)
	if r.activeID == id {
		r.activeID = ""
		if len(r.items) > 0 {
			// Prefer most recently opened remaining.
			best := 0
			for i := range r.items {
				if r.items[i].LastOpenedAt.After(r.items[best].LastOpenedAt) {
					best = i
				}
			}
			r.activeID = r.items[best].ID
		}
	}
	return r.saveLocked()
}

func (r *Registry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.items = nil
			r.activeID = ""
			return r.saveLocked()
		}
		return fmt.Errorf("workspace: read registry: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		r.items = nil
		r.activeID = ""
		return nil
	}
	var file registryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("workspace: parse registry: %w", err)
	}
	r.items = file.Items
	if r.items == nil {
		r.items = []Workspace{}
	}
	r.activeID = file.ActiveID
	for i := range r.items {
		r.items[i].RootPath = filepath.Clean(r.items[i].RootPath)
		if r.items[i].ID == "" {
			r.items[i].ID = newID()
		}
		if r.items[i].Name == "" {
			r.items[i].Name = filepath.Base(r.items[i].RootPath)
		}
		if r.items[i].Kind == "" {
			if IsSession(r.items[i]) {
				r.items[i].Kind = KindSession
			} else {
				r.items[i].Kind = KindFolder
			}
		}
	}
	return nil
}

func (r *Registry) saveLocked() error {
	file := registryFile{
		Version:  1,
		ActiveID: r.activeID,
		Items:    r.items,
	}
	if file.Items == nil {
		file.Items = []Workspace{}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("workspace: marshal: %w", err)
	}
	data = append(data, '\n')
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, FilePerm); err != nil {
		return fmt.Errorf("workspace: write temp: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("workspace: rename: %w", err)
	}
	_ = os.Chmod(r.path, FilePerm)
	return nil
}

func resolveRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("workspace: empty path")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve path: %w", err)
	}
	return filepath.Clean(abs), nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func newID() string {
	// Compact id: timestamp-ish + random-ish path base hash.
	return fmt.Sprintf("ws_%d", time.Now().UnixNano())
}
