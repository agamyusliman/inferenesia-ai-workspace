// Package store persists chat sessions and messages in SQLite (CGO-free modernc).
// Config home default path: ~/.inferenesia/sessions.db (override via Open path).
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
	_ "modernc.org/sqlite"
)

// DBFileName is the default SQLite file under the config home.
const DBFileName = "sessions.db"

// timestampFormat is a fixed-width RFC3339 variant with always-9-digit
// fractional seconds. It must be used instead of time.RFC3339Nano for any
// timestamp stored in SQLite and later compared via ORDER BY: RFC3339Nano
// trims trailing zeros (e.g. ".5Z" instead of ".500000000Z"), which breaks
// lexicographic TEXT comparison — a 500ms timestamp would sort *newer* than a
// 500ms+5ns one because 'Z' > '0'. Fixed width guarantees that chronological
// order matches lexicographic order. parseTime still accepts both shapes.
const timestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

// Session is one chat thread metadata row.
type Session struct {
	ID        string
	Title     string
	Model     string
	Profile   string
	Workspace string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Store is a SQLite-backed session/message store.
type Store struct {
	db   *sql.DB
	path string
}

// DefaultPath returns {configHome}/sessions.db.
func DefaultPath(configHome string) string {
	return filepath.Join(configHome, DBFileName)
}

// Open opens (or creates) a SQLite store at path. Parent directories are created
// with mode 0700 when missing. WAL is enabled for multi-reader friendliness.
func Open(path string) (*Store, error) {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return nil, fmt.Errorf("store: empty database path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create parent dir: %w", err)
	}

	// modernc DSN: file path. Enable foreign keys.
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// Single writer process is fine; keep pool small for SQLite.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Restrict DB file to owner when possible (session content is private).
	_ = os.Chmod(path, 0o600)
	return s, nil
}

// Path returns the absolute (cleaned) database file path.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  profile TEXT NOT NULL DEFAULT '',
  workspace TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  tool_calls_json TEXT NOT NULL DEFAULT '',
  meta_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  UNIQUE(session_id, seq),
  FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_messages_session_seq ON messages(session_id, seq);
CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	// Orchestrator tasks table (VAL-ORCH-006/007) — additive migration.
	if _, err := s.db.Exec(tasksSchema()); err != nil {
		return fmt.Errorf("store: migrate tasks: %w", err)
	}
	if err := s.ensureTasksWorkspaceColumn(); err != nil {
		return fmt.Errorf("store: migrate tasks workspace_id: %w", err)
	}
	// Per-message meta column (misc-chat-meta-persist-history): additive so
	// existing sessions.db files upgrade in place without a schema rebuild.
	if err := s.migrateMessagesMeta(); err != nil {
		return fmt.Errorf("store: migrate messages meta: %w", err)
	}
	return nil
}

// CreateSession inserts a new session. ID is required. CreatedAt/UpdatedAt
// default to now when zero.
func (s *Store) CreateSession(meta Session) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	id := strings.TrimSpace(meta.ID)
	if id == "" {
		return fmt.Errorf("store: session id is required")
	}
	now := time.Now().UTC()
	created := meta.CreatedAt
	if created.IsZero() {
		created = now
	}
	updated := meta.UpdatedAt
	if updated.IsZero() {
		updated = created
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, title, model, profile, workspace, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		meta.Title,
		meta.Model,
		meta.Profile,
		meta.Workspace,
		created.UTC().Format(timestampFormat),
		updated.UTC().Format(timestampFormat),
	)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// TouchSession updates metadata fields and bumps updated_at. Empty strings leave
// the previous value for title/model/profile/workspace.
func (s *Store) TouchSession(id string, model, profile, title, workspace string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("store: session id is required")
	}
	cur, err := s.GetSession(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(model) != "" {
		cur.Model = model
	}
	if strings.TrimSpace(profile) != "" {
		cur.Profile = profile
	}
	if strings.TrimSpace(title) != "" {
		cur.Title = title
	}
	if strings.TrimSpace(workspace) != "" {
		cur.Workspace = workspace
	}
	now := time.Now().UTC().Format(timestampFormat)
	_, err = s.db.Exec(
		`UPDATE sessions SET title=?, model=?, profile=?, workspace=?, updated_at=? WHERE id=?`,
		cur.Title, cur.Model, cur.Profile, cur.Workspace, now, id,
	)
	if err != nil {
		return fmt.Errorf("store: touch session: %w", err)
	}
	return nil
}

// GetSession returns one session by id.
func (s *Store) GetSession(id string) (*Session, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store: closed")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("store: session id is required")
	}
	row := s.db.QueryRow(
		`SELECT id, title, model, profile, workspace, created_at, updated_at
		 FROM sessions WHERE id = ?`, id,
	)
	var sess Session
	var created, updated string
	if err := row.Scan(
		&sess.ID, &sess.Title, &sess.Model, &sess.Profile, &sess.Workspace,
		&created, &updated,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: session %q not found", id)
		}
		return nil, fmt.Errorf("store: get session: %w", err)
	}
	sess.CreatedAt = parseTime(created)
	sess.UpdatedAt = parseTime(updated)
	return &sess, nil
}

// ListSessions returns sessions ordered by updated_at descending.
// limit <= 0 means no limit.
//
// Ordering uses (updated_at DESC, rowid DESC): rowid reflects insertion order
// (sessions.id is TEXT PRIMARY KEY, so SQLite assigns an implicit auto rowid),
// giving a deterministic, chronologically-correct tie-breaker when two sessions
// share the same updated_at timestamp (clock collisions / same-second creates).
// The previous (updated_at DESC, id DESC) tie-breaker sorted by id string,
// which could put an alphabetically-larger but older session first.
func (s *Store) ListSessions(limit int) ([]Session, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store: closed")
	}
	q := `SELECT id, title, model, profile, workspace, created_at, updated_at
	      FROM sessions ORDER BY updated_at DESC, rowid DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(q+` LIMIT ?`, limit)
	} else {
		rows, err = s.db.Query(q)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		var created, updated string
		if err := rows.Scan(
			&sess.ID, &sess.Title, &sess.Model, &sess.Profile, &sess.Workspace,
			&created, &updated,
		); err != nil {
			return nil, fmt.Errorf("store: list scan: %w", err)
		}
		sess.CreatedAt = parseTime(created)
		sess.UpdatedAt = parseTime(updated)
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Session{}
	}
	return out, nil
}

// ListSessionsByWorkspace returns sessions for a workspace id, newest first.
// Used for desktop one-active-thread-per-workspace history (VAL-DESK-014).
// limit <= 0 means no limit. Tie-breaker is rowid DESC (see ListSessions).
func (s *Store) ListSessionsByWorkspace(workspace string, limit int) ([]Session, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store: closed")
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("store: workspace is required")
	}
	// Playground threads ("pg:<id>") store a workspace hint for scope display
	// but are NOT workspace chat. Excluding them keeps canvas / Image Studio
	// turns out of the coding transcript (docs/canvas-gallery.md scope rules).
	q := `SELECT id, title, model, profile, workspace, created_at, updated_at
	      FROM sessions WHERE workspace = ? AND id NOT LIKE 'pg:%'
	      ORDER BY updated_at DESC, rowid DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(q+` LIMIT ?`, workspace, limit)
	} else {
		rows, err = s.db.Query(q, workspace)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list sessions by workspace: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		var created, updated string
		if err := rows.Scan(
			&sess.ID, &sess.Title, &sess.Model, &sess.Profile, &sess.Workspace,
			&created, &updated,
		); err != nil {
			return nil, fmt.Errorf("store: list by workspace scan: %w", err)
		}
		sess.CreatedAt = parseTime(created)
		sess.UpdatedAt = parseTime(updated)
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Session{}
	}
	return out, nil
}

// LatestSessionForWorkspace returns the newest session for workspace, or nil
// when none exists (not an error).
func (s *Store) LatestSessionForWorkspace(workspace string) (*Session, error) {
	list, err := s.ListSessionsByWorkspace(workspace, 1)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

// DeleteWorkspaceThread removes the desktop chat thread (desk:<workspaceID>),
// any sessions rows keyed to that workspace, their messages, and tasks.
func (s *Store) DeleteWorkspaceThread(workspaceID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("store: workspace is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin delete workspace thread: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	deskID := "desk:" + workspaceID
	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, deskID); err != nil {
		return fmt.Errorf("store: delete desk messages: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, deskID); err != nil {
		return fmt.Errorf("store: delete desk session: %w", err)
	}
	rows, err := tx.Query(`SELECT id FROM sessions WHERE workspace = ?`, workspaceID)
	if err != nil {
		return fmt.Errorf("store: list workspace sessions: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store: scan session id: %w", err)
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, id); err != nil {
			return fmt.Errorf("store: delete messages for %s: %w", id, err)
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
			return fmt.Errorf("store: delete session %s: %w", id, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM tasks WHERE workspace_id = ?`, workspaceID); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return fmt.Errorf("store: delete tasks: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit delete workspace thread: %w", err)
	}
	return nil
}

// ReplaceMessages deletes existing messages for the session and inserts msgs
// in order (seq 0..n-1). Bumps session updated_at.
func (s *Store) ReplaceMessages(sessionID string, msgs []provider.Message) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("store: session id is required")
	}
	if _, err := s.GetSession(sessionID); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("store: clear messages: %w", err)
	}
	if err := insertMessagesTx(tx, sessionID, 0, msgs); err != nil {
		return err
	}
	now := time.Now().UTC().Format(timestampFormat)
	if _, err := tx.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now, sessionID); err != nil {
		return fmt.Errorf("store: bump updated_at: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// AppendMessages appends msgs after the current max seq. Bumps updated_at.
func (s *Store) AppendMessages(sessionID string, msgs []provider.Message) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("store: session id is required")
	}
	if len(msgs) == 0 {
		return nil
	}
	if _, err := s.GetSession(sessionID); err != nil {
		return err
	}

	var maxSeq sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(seq) FROM messages WHERE session_id = ?`, sessionID,
	).Scan(&maxSeq)
	if err != nil {
		return fmt.Errorf("store: max seq: %w", err)
	}
	next := 0
	if maxSeq.Valid {
		next = int(maxSeq.Int64) + 1
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertMessagesTx(tx, sessionID, next, msgs); err != nil {
		return err
	}
	now := time.Now().UTC().Format(timestampFormat)
	if _, err := tx.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now, sessionID); err != nil {
		return fmt.Errorf("store: bump updated_at: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

func insertMessagesTx(tx *sql.Tx, sessionID string, startSeq int, msgs []provider.Message) error {
	now := time.Now().UTC().Format(timestampFormat)
	stmt, err := tx.Prepare(
		`INSERT INTO messages (session_id, seq, role, content, tool_call_id, name, tool_calls_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("store: prepare insert message: %w", err)
	}
	defer stmt.Close()

	for i, m := range msgs {
		tcJSON := ""
		if len(m.ToolCalls) > 0 {
			b, err := json.Marshal(m.ToolCalls)
			if err != nil {
				return fmt.Errorf("store: marshal tool_calls: %w", err)
			}
			tcJSON = string(b)
		}
		role := string(m.Role)
		if role == "" {
			role = string(provider.RoleUser)
		}
		if _, err := stmt.Exec(
			sessionID,
			startSeq+i,
			role,
			m.Content,
			m.ToolCallID,
			m.Name,
			tcJSON,
			now,
		); err != nil {
			return fmt.Errorf("store: insert message seq %d: %w", startSeq+i, err)
		}
	}
	return nil
}

// LoadMessages returns all messages for a session ordered by seq ascending.
func (s *Store) LoadMessages(sessionID string) ([]provider.Message, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store: closed")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("store: session id is required")
	}
	// Ensure session exists (clear not-found vs empty transcript).
	if _, err := s.GetSession(sessionID); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT role, content, tool_call_id, name, tool_calls_json
		 FROM messages WHERE session_id = ? ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: load messages: %w", err)
	}
	defer rows.Close()

	var out []provider.Message
	for rows.Next() {
		var role, content, toolCallID, name, tcJSON string
		if err := rows.Scan(&role, &content, &toolCallID, &name, &tcJSON); err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		msg := provider.Message{
			Role:       provider.Role(role),
			Content:    content,
			ToolCallID: toolCallID,
			Name:       name,
		}
		if strings.TrimSpace(tcJSON) != "" {
			var tcs []provider.ToolCall
			if err := json.Unmarshal([]byte(tcJSON), &tcs); err != nil {
				return nil, fmt.Errorf("store: unmarshal tool_calls: %w", err)
			}
			msg.ToolCalls = tcs
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []provider.Message{}
	}
	return out, nil
}

// SaveTurn ensures a session exists (creates if needed) and replaces the full
// message transcript. Convenient for agent turns that rebuild the full history.
func (s *Store) SaveTurn(meta Session, msgs []provider.Message) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store: closed")
	}
	id := strings.TrimSpace(meta.ID)
	if id == "" {
		return fmt.Errorf("store: session id is required")
	}
	_, err := s.GetSession(id)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return err
		}
		if err := s.CreateSession(meta); err != nil {
			return err
		}
	} else {
		if err := s.TouchSession(id, meta.Model, meta.Profile, meta.Title, meta.Workspace); err != nil {
			return err
		}
	}
	return s.ReplaceMessages(id, msgs)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// RFC3339Nano accepts both fixed-width (.000000000) and trimmed (.5) forms,
	// so a single parse covers both legacy rows and rows written with
	// timestampFormat. Keep the RFC3339 fallback for second-precision rows.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(timestampFormat, s); err == nil {
		return t
	}
	return time.Time{}
}
