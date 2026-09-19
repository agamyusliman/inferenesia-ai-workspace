package pty

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// PersistRootEnv overrides where detached session state is stored.
// Preferred: INFERENESIA_PTY_DIR. Typo: INFERNESIA_PTY_DIR. Legacy: YURA_AI_PTY_DIR.
// Default: <config.Home()>/pty
const PersistRootEnv = "INFERENESIA_PTY_DIR"

// PersistRootEnvTypo is the misspelled product PTY dir override.
const PersistRootEnvTypo = "INFERNESIA_PTY_DIR"

// PersistRootEnvLegacy is the pre-rebrand PTY dir override.
const PersistRootEnvLegacy = "YURA_AI_PTY_DIR"

// SessionMeta is filesystem metadata for detached (CLI-surviving) sessions.
// In-process Manager sessions need no disk; the CLI uses this for multi-command
// workflows (start → later stop/read from another process).
type SessionMeta struct {
	ID       string    `json:"id"`
	Command  string    `json:"command"`
	Args     []string  `json:"args"`
	WorkDir  string    `json:"work_dir"`
	Status   Status    `json:"status"`
	PID      int       `json:"pid"`
	ExitCode *int      `json:"exit_code,omitempty"`
	// WorkerPID is the supervisor process that owns the PTY (if detached).
	WorkerPID int       `json:"worker_pid,omitempty"`
	Started   time.Time `json:"started_at"`
	Ended     time.Time `json:"ended_at,omitempty"`
	Error     string    `json:"error,omitempty"`
	// Detached marks CLI-surviving sessions.
	Detached bool `json:"detached"`
}

// PersistRoot resolves the directory for PTY session state.
func PersistRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(PersistRootEnv)); override != "" {
		return ensurePersistPath(override)
	}
	if typo := strings.TrimSpace(os.Getenv(PersistRootEnvTypo)); typo != "" {
		return ensurePersistPath(typo)
	}
	if legacy := strings.TrimSpace(os.Getenv(PersistRootEnvLegacy)); legacy != "" {
		return ensurePersistPath(legacy)
	}
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, "pty")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func ensurePersistPath(override string) (string, error) {
	path := filepath.Clean(override)
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

// SessionDir returns $root/sessions/<id>.
func SessionDir(root, id string) string {
	return filepath.Join(root, "sessions", id)
}

// MetaPath is the meta.json path for a session.
func MetaPath(root, id string) string {
	return filepath.Join(SessionDir(root, id), "meta.json")
}

// OutputPath is the out.log path for a session.
func OutputPath(root, id string) string {
	return filepath.Join(SessionDir(root, id), "out.log")
}

// SaveMeta writes meta.json with mode 0600.
func SaveMeta(root string, meta SessionMeta) error {
	dir := SessionDir(root, meta.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := MetaPath(root, meta.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, MetaPath(root, meta.ID))
}

// LoadMeta reads meta.json for a session.
func LoadMeta(root, id string) (SessionMeta, error) {
	var meta SessionMeta
	data, err := os.ReadFile(MetaPath(root, id))
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

// ListMeta loads all session metas under root (sorted by id).
func ListMeta(root string) ([]SessionMeta, error) {
	dir := filepath.Join(root, "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []SessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := LoadMeta(root, e.Name())
		if err != nil {
			continue
		}
		out = append(out, meta)
	}
	// sort by id
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ID < out[i].ID {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// ReadOutputFile returns the full (or last maxBytes of) out.log content.
func ReadOutputFile(root, id string, maxBytes int) (string, error) {
	path := OutputPath(root, id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if maxBytes > 0 && len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
	}
	return string(data), nil
}

// UpdateMetaStatus is a small helper to patch status/exit fields.
func UpdateMetaStatus(root, id string, status Status, exitCode *int, errMsg string) error {
	meta, err := LoadMeta(root, id)
	if err != nil {
		return err
	}
	meta.Status = status
	if exitCode != nil {
		meta.ExitCode = exitCode
	}
	if errMsg != "" {
		meta.Error = errMsg
	}
	if status == StatusExited || status == StatusStopped || status == StatusError {
		if meta.Ended.IsZero() {
			meta.Ended = time.Now()
		}
	}
	return SaveMeta(root, meta)
}

// FormatMetaLine is a one-line CLI summary.
func FormatMetaLine(m SessionMeta) string {
	code := "-"
	if m.ExitCode != nil {
		code = fmt.Sprintf("%d", *m.ExitCode)
	}
	return fmt.Sprintf("%s  status=%s  pid=%d  exit=%s  cmd=%q",
		m.ID, m.Status, m.PID, code, m.Command)
}
