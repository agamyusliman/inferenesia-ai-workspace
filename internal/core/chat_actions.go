package core

import (
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/store"
)

// ChatMessageActionResult is returned after a durable message mutation (VAL-CHAT-017/018).
type ChatMessageActionResult struct {
	WorkspaceID string               `json:"workspace_id"`
	SessionID   string               `json:"session_id,omitempty"`
	Messages    []DesktopChatMessage `json:"messages"`
	Removed     int                  `json:"removed"`
	Action      string               `json:"action"`
	Message     string               `json:"message,omitempty"`
}

// SetChatHistory replaces the durable UI thread for a workspace and rebinds in-memory
// chat when the workspace is active. Used by delete / undo-from-here (VAL-CHAT-017/018).
//
// Preserves per-message meta (tokens/latency/tools/tasks) on assistant rows so a
// delete / truncate does not silently drop the ChatMetaRow + tool/task blocks
// from the remaining messages (misc-chat-meta-persist-history).
func (s *Service) SetChatHistory(workspaceID string, messages []DesktopChatMessage) (DesktopChatHistory, error) {
	if s == nil {
		return DesktopChatHistory{}, fmt.Errorf("core: service is nil")
	}
	wid, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return DesktopChatHistory{}, err
	}
	if messages == nil {
		messages = []DesktopChatMessage{}
	}
	// Normalize: user/assistant only, re-index stable desktop ids.
	norm := normalizeDesktopMessages(messages)
	prov, metas := desktopToProviderWithMeta(norm)

	// Persist durable store first.
	s.mu.Lock()
	model, profile := s.chat.Model, s.chat.Profile
	activeWid := s.chatWorkspaceID
	s.mu.Unlock()

	if err := s.persistChat(wid, ChatSession{
		Messages:     prov,
		Model:        model,
		Profile:      profile,
		MessageMetas: metas,
	}); err != nil {
		return DesktopChatHistory{}, err
	}

	// If this workspace is the active memory thread (or none set but matches), rebind.
	s.mu.Lock()
	if activeWid == "" || activeWid == wid {
		s.chatWorkspaceID = wid
		s.chat.Messages = append([]provider.Message(nil), prov...)
		s.chat.MessageMetas = metas
		s.statusMsg = "Chat history updated"
	}
	s.mu.Unlock()

	return DesktopChatHistory{
		WorkspaceID: wid,
		SessionID:   desktopSessionID(wid),
		Model:       model,
		Profile:     profile,
		Messages:    norm,
	}, nil
}

// DeleteChatMessage removes one message by desktop id (or index m-N) and persists (VAL-CHAT-018).
func (s *Service) DeleteChatMessage(workspaceID, messageID string) (ChatMessageActionResult, error) {
	if s == nil {
		return ChatMessageActionResult{}, fmt.Errorf("core: service is nil")
	}
	hist, err := s.GetChatHistory(workspaceID)
	if err != nil {
		return ChatMessageActionResult{}, err
	}
	before := len(hist.Messages)
	idx := findDesktopMessageIndex(hist.Messages, messageID)
	if idx < 0 {
		return ChatMessageActionResult{
			WorkspaceID: hist.WorkspaceID,
			SessionID:   hist.SessionID,
			Messages:    hist.Messages,
			Removed:     0,
			Action:      "delete",
			Message:     "message not found",
		}, nil
	}
	next := append([]DesktopChatMessage{}, hist.Messages[:idx]...)
	next = append(next, hist.Messages[idx+1:]...)
	updated, err := s.SetChatHistory(hist.WorkspaceID, next)
	if err != nil {
		return ChatMessageActionResult{}, err
	}
	return ChatMessageActionResult{
		WorkspaceID: updated.WorkspaceID,
		SessionID:   updated.SessionID,
		Messages:    updated.Messages,
		Removed:     before - len(updated.Messages),
		Action:      "delete",
		Message:     "Message deleted",
	}, nil
}

// TruncateChatFromHere drops the message with messageID and everything after it (undo-from-here).
// Durable store + in-memory transcript are updated (VAL-CHAT-017).
func (s *Service) TruncateChatFromHere(workspaceID, messageID string) (ChatMessageActionResult, error) {
	if s == nil {
		return ChatMessageActionResult{}, fmt.Errorf("core: service is nil")
	}
	hist, err := s.GetChatHistory(workspaceID)
	if err != nil {
		return ChatMessageActionResult{}, err
	}
	before := len(hist.Messages)
	idx := findDesktopMessageIndex(hist.Messages, messageID)
	if idx < 0 {
		return ChatMessageActionResult{
			WorkspaceID: hist.WorkspaceID,
			SessionID:   hist.SessionID,
			Messages:    hist.Messages,
			Removed:     0,
			Action:      "truncate",
			Message:     "message not found",
		}, nil
	}
	next := append([]DesktopChatMessage{}, hist.Messages[:idx]...)
	updated, err := s.SetChatHistory(hist.WorkspaceID, next)
	if err != nil {
		return ChatMessageActionResult{}, err
	}
	return ChatMessageActionResult{
		WorkspaceID: updated.WorkspaceID,
		SessionID:   updated.SessionID,
		Messages:    updated.Messages,
		Removed:     before - len(updated.Messages),
		Action:      "truncate",
		Message:     "History truncated from here",
	}, nil
}

func normalizeDesktopMessages(msgs []DesktopChatMessage) []DesktopChatMessage {
	var out []DesktopChatMessage
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		cp := DesktopChatMessage{
			Role:     role,
			Content:  m.Content,
			Mentions: m.Mentions,
		}
		// Preserve persisted meta so delete/truncate does not drop the
		// ChatMetaRow + tool/task blocks from surviving assistant rows
		// (misc-chat-meta-persist-history).
		if role == "assistant" {
			cp.Meta = m.Meta
			cp.Tools = m.Tools
			cp.Tasks = m.Tasks
			cp.Thinking = m.Thinking
			cp.Model = m.Model
			cp.Profile = m.Profile
			cp.Host = m.Host
		}
		out = append(out, cp)
	}
	for i := range out {
		out[i].ID = fmt.Sprintf("m-%d", i)
	}
	if out == nil {
		out = []DesktopChatMessage{}
	}
	return out
}

// desktopToProviderWithMeta converts desktop bubbles to a provider.Message
// transcript plus a per-message meta map keyed by index. Used by SetChatHistory
// so delete/truncate preserves persisted meta on surviving assistant rows.
func desktopToProviderWithMeta(msgs []DesktopChatMessage) ([]provider.Message, map[int]*store.MessageMeta) {
	out := make([]provider.Message, 0, len(msgs))
	metas := make(map[int]*store.MessageMeta, len(msgs))
	for i, m := range msgs {
		role := provider.Role(strings.TrimSpace(m.Role))
		if role != provider.RoleUser && role != provider.RoleAssistant {
			continue
		}
		out = append(out, provider.Message{Role: role, Content: m.Content})
		if role == provider.RoleAssistant && hasDesktopMeta(&m) {
			metas[i] = desktopToStoreMeta(&m)
		}
	}
	return out, metas
}

// hasDesktopMeta reports whether a desktop message carries any persisted meta
// worth saving (meta row, tool/task blocks, thinking, or routing labels).
func hasDesktopMeta(m *DesktopChatMessage) bool {
	if m == nil {
		return false
	}
	return m.Meta != nil || len(m.Tools) > 0 || len(m.Tasks) > 0 ||
		strings.TrimSpace(m.Thinking) != "" || strings.TrimSpace(m.Model) != "" ||
		strings.TrimSpace(m.Profile) != "" || strings.TrimSpace(m.Host) != ""
}

// desktopToStoreMeta flattens a desktop message's meta fields into a store
// MessageMeta. Never copies secrets — none of the source fields are credential-like.
func desktopToStoreMeta(m *DesktopChatMessage) *store.MessageMeta {
	if m == nil {
		return nil
	}
	mm := &store.MessageMeta{
		Thinking: m.Thinking,
		Model:    m.Model,
		Profile:  m.Profile,
		Host:     m.Host,
	}
	if m.Meta != nil {
		mm.Tokens = m.Meta.Tokens
		mm.TokensPrompt = m.Meta.TokensPrompt
		mm.TokensCompletion = m.Meta.TokensCompletion
		mm.LatencyMS = m.Meta.LatencyMS
		mm.StreamStatus = m.Meta.StreamStatus
	}
	if len(m.Tools) > 0 {
		mm.Tools = make([]store.ToolBlockMeta, 0, len(m.Tools))
		for _, t := range m.Tools {
			mm.Tools = append(mm.Tools, store.ToolBlockMeta{
				ID:     t.ID,
				Name:   t.Name,
				Status: t.Status,
				Detail: t.Detail,
				Result: t.Result,
			})
		}
	}
	if len(m.Tasks) > 0 {
		mm.Tasks = make([]store.TaskBlockMeta, 0, len(m.Tasks))
		for _, tk := range m.Tasks {
			mm.Tasks = append(mm.Tasks, store.TaskBlockMeta{
				ID:       tk.ID,
				Category: tk.Category,
				Status:   tk.Status,
				Label:    tk.Label,
				Detail:   tk.Detail,
			})
		}
	}
	return mm
}

// findDesktopMessageIndex resolves a UI/store message id.
// Accepts m-N index ids from GetChatHistory and exact id matches from the client.
func findDesktopMessageIndex(msgs []DesktopChatMessage, messageID string) int {
	id := strings.TrimSpace(messageID)
	if id == "" {
		return -1
	}
	for i, m := range msgs {
		if m.ID == id {
			return i
		}
	}
	// Fallback: m-N parses as absolute index into current list.
	if strings.HasPrefix(id, "m-") {
		var n int
		if _, err := fmt.Sscanf(id, "m-%d", &n); err == nil && n >= 0 && n < len(msgs) {
			return n
		}
	}
	return -1
}
