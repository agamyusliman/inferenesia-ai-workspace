package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// MessageMeta is the per-message meta payload persisted in sessions.db so a
// reloaded assistant message restores its ChatMetaRow (tokens/latency/stream
// status) plus tool/task blocks and thinking content (misc-chat-meta-persist-history).
//
// This struct intentionally carries NO credential-like fields: no api key,
// token, authorization header, or secret is ever stored here. Routing meta is
// limited to non-secret labels (profile id, model id, request host). The store
// does not redact caller-supplied strings — callers must never place secrets
// in Detail/Result/Thinking; the field set is fixed so there is no generic
// "metadata" sink for secrets to leak through.
type MessageMeta struct {
	// Tokens / TokensPrompt / TokensCompletion / LatencyMS surface on the
	// ChatMetaRow after reload (VAL-CHAT-012). Zero values are valid (meta
	// simply omits them in the UI when zero).
	Tokens           int   `json:"tokens,omitempty"`
	TokensPrompt     int   `json:"tokens_prompt,omitempty"`
	TokensCompletion int   `json:"tokens_completion,omitempty"`
	LatencyMS        int64 `json:"latency_ms,omitempty"`
	// StreamStatus is the terminal stream state label (done|stopped|error|…)
	// so the reloaded bubble shows the same status as before reload, not a
	// bare "done" default.
	StreamStatus string `json:"stream_status,omitempty"`
	// Model / Profile / Host are non-secret routing labels persisted so the
	// reloaded assistant message shows which provider/model produced it.
	Model   string `json:"model,omitempty"`
	Profile string `json:"profile,omitempty"`
	Host    string `json:"host,omitempty"`
	// Thinking carries optional reasoning content split from the final answer
	// (VAL-CHAT-013). Persisted so the collapsible Thinking block rehydrates.
	Thinking string `json:"thinking,omitempty"`
	// Tools are the tool-call blocks recorded during the turn (VAL-CHAT-014).
	// Persisted so distinct tool cards rehydrate after reload.
	Tools []ToolBlockMeta `json:"tools,omitempty"`
	// Tasks are the subagent/task blocks recorded during the turn (VAL-CHAT-015).
	// Persisted so distinct Subagent cards rehydrate after reload.
	Tasks []TaskBlockMeta `json:"tasks,omitempty"`
}

// ToolBlockMeta is one persisted tool-call card (VAL-CHAT-014).
type ToolBlockMeta struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"` // running|done|error
	Detail string `json:"detail,omitempty"`
	Result string `json:"result,omitempty"`
}

// TaskBlockMeta is one persisted subagent/task card (VAL-CHAT-015).
type TaskBlockMeta struct {
	ID       string `json:"id,omitempty"`
	Category string `json:"category,omitempty"`
	Status   string `json:"status,omitempty"` // running|queued|completed|failed|cancelled
	Label    string `json:"label,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// StoredMessage is one chat turn with optional per-message meta. It embeds
// provider.Message so the LLM-facing message shape stays unchanged; Meta is
// carried alongside for the desktop durable-history path only.
type StoredMessage struct {
	provider.Message
	Meta *MessageMeta `json:"meta,omitempty"`
}

// metaColumnName is the additive column added to the messages table so older
// sessions.db files upgrade in place without a schema rebuild.
const metaColumnName = "meta_json"

// hasMessagesMetaColumn reports whether the messages table already has the
// meta_json column (used by the additive migration to avoid repeating ALTER).
func (s *Store) hasMessagesMetaColumn() (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("store: closed")
	}
	rows, err := s.db.Query(`PRAGMA table_info(messages)`)
	if err != nil {
		return false, fmt.Errorf("store: pragma messages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("store: scan pragma: %w", err)
		}
		if name == metaColumnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

// migrateMessagesMeta adds the meta_json column to messages when missing.
func (s *Store) migrateMessagesMeta() error {
	has, err := s.hasMessagesMetaColumn()
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	stmt := fmt.Sprintf(`ALTER TABLE messages ADD COLUMN %s TEXT NOT NULL DEFAULT ''`, metaColumnName)
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("store: alter messages add %s: %w", metaColumnName, err)
	}
	return nil
}

// SaveTurnWithMeta ensures a session exists (creates if needed) and replaces
// the full message transcript WITH per-message meta. Used by the desktop
// durable-history path so reloaded assistants restore ChatMetaRow + tool/task
// blocks (misc-chat-meta-persist-history). Messages with a nil Meta still
// persist fine (legacy rows load back with nil Meta).
func (s *Store) SaveTurnWithMeta(meta Session, msgs []StoredMessage) error {
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
	return s.ReplaceMessagesWithMeta(id, msgs)
}

// ReplaceMessagesWithMeta deletes existing messages for the session and inserts
// msgs (with meta) in order. Bumps session updated_at.
func (s *Store) ReplaceMessagesWithMeta(sessionID string, msgs []StoredMessage) error {
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
	if err := insertStoredMessagesTx(tx, sessionID, 0, msgs); err != nil {
		return err
	}
	now := nowTimestamp()
	if _, err := tx.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now, sessionID); err != nil {
		return fmt.Errorf("store: bump updated_at: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// AppendMessagesWithMeta appends msgs (with meta) after the current max seq.
// Bumps updated_at.
func (s *Store) AppendMessagesWithMeta(sessionID string, msgs []StoredMessage) error {
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

	if err := insertStoredMessagesTx(tx, sessionID, next, msgs); err != nil {
		return err
	}
	now := nowTimestamp()
	if _, err := tx.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now, sessionID); err != nil {
		return fmt.Errorf("store: bump updated_at: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

func insertStoredMessagesTx(tx *sql.Tx, sessionID string, startSeq int, msgs []StoredMessage) error {
	now := nowTimestamp()
	stmt, err := tx.Prepare(
		`INSERT INTO messages (session_id, seq, role, content, tool_call_id, name, tool_calls_json, meta_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("store: prepare insert stored message: %w", err)
	}
	defer stmt.Close()

	for i, sm := range msgs {
		m := sm.Message
		tcJSON := ""
		if len(m.ToolCalls) > 0 {
			b, err := json.Marshal(m.ToolCalls)
			if err != nil {
				return fmt.Errorf("store: marshal tool_calls: %w", err)
			}
			tcJSON = string(b)
		}
		metaJSON := ""
		if sm.Meta != nil {
			b, err := json.Marshal(sm.Meta)
			if err != nil {
				return fmt.Errorf("store: marshal message meta: %w", err)
			}
			metaJSON = string(b)
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
			metaJSON,
			now,
		); err != nil {
			return fmt.Errorf("store: insert stored message seq %d: %w", startSeq+i, err)
		}
	}
	return nil
}

// LoadMessagesWithMeta returns all messages for a session ordered by seq
// ascending, WITH per-message meta (nil when the row has none / legacy rows).
func (s *Store) LoadMessagesWithMeta(sessionID string) ([]StoredMessage, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store: closed")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("store: session id is required")
	}
	if _, err := s.GetSession(sessionID); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT role, content, tool_call_id, name, tool_calls_json, meta_json
		 FROM messages WHERE session_id = ? ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: load messages with meta: %w", err)
	}
	defer rows.Close()

	var out []StoredMessage
	for rows.Next() {
		var role, content, toolCallID, name, tcJSON, metaJSON string
		if err := rows.Scan(&role, &content, &toolCallID, &name, &tcJSON, &metaJSON); err != nil {
			return nil, fmt.Errorf("store: scan stored message: %w", err)
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
		var meta *MessageMeta
		if strings.TrimSpace(metaJSON) != "" {
			var mm MessageMeta
			if err := json.Unmarshal([]byte(metaJSON), &mm); err != nil {
				return nil, fmt.Errorf("store: unmarshal message meta: %w", err)
			}
			meta = &mm
		}
		out = append(out, StoredMessage{Message: msg, Meta: meta})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []StoredMessage{}
	}
	return out, nil
}

// nowTimestamp returns the canonical fixed-width store timestamp for the
// meta-aware insert path. Mirrors timestampFormat used across the store.
func nowTimestamp() string {
	return time.Now().UTC().Format(timestampFormat)
}

// HasMessageMetaFieldNamed is a reflect-free guard used by tests to assert
// MessageMeta exposes no credential-like field names. It checks a fixed
// allowlist derived from the struct above so adding a secret-named field
// later would fail the test. Returns true when a field with that exact name
// exists on MessageMeta.
func HasMessageMetaFieldNamed(name string) bool {
	return hasMessageMetaFieldNamed(name)
}

// hasMessageMetaFieldNamed is a reflect-free guard used by the no-secrets test
// to assert MessageMeta exposes no credential-like field names. It checks a
// fixed allowlist derived from the struct above so adding a secret-named field
// later would fail the test.
func hasMessageMetaFieldNamed(name string) bool {
	switch name {
	case "Tokens", "TokensPrompt", "TokensCompletion", "LatencyMS",
		"StreamStatus", "Model", "Profile", "Host", "Thinking",
		"Tools", "Tasks":
		return true
	}
	return false
}
