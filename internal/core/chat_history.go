package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/store"
)

// DesktopChatMessage is one UI-facing chat bubble persisted for a workspace thread.
// Mentions are stored when the UI/recorder provides them; model history uses content only.
//
// Meta / Tools / Tasks / Thinking / Model / Profile / Host / StreamStatus
// rehydrate after reload so a restored assistant message shows its ChatMetaRow
// and tool/task blocks, not only a bare "done" status
// (misc-chat-meta-persist-history). None of these fields ever carry secrets.
type DesktopChatMessage struct {
	ID       string   `json:"id,omitempty"`
	Role     string   `json:"role"`
	Content  string   `json:"content"`
	Mentions []string `json:"mentions,omitempty"`
	// Meta is the per-message ChatMetaRow payload (tokens/latency/stream status).
	// Populated only for assistant messages that completed a turn.
	Meta *DesktopChatMeta `json:"meta,omitempty"`
	// Tools are the persisted tool-call blocks for this assistant message.
	Tools []DesktopToolBlock `json:"tools,omitempty"`
	// Tasks are the persisted subagent/task blocks for this assistant message.
	Tasks []DesktopTaskBlock `json:"tasks,omitempty"`
	// Thinking is the optional reasoning content split from the final answer.
	Thinking string `json:"thinking,omitempty"`
	// Model / Profile / Host are non-secret routing labels for the assistant turn.
	Model   string `json:"model,omitempty"`
	Profile string `json:"profile,omitempty"`
	Host    string `json:"host,omitempty"`
}

// DesktopChatMeta is the persisted per-message meta for the ChatMetaRow
// (VAL-CHAT-012). Never carries secrets.
type DesktopChatMeta struct {
	Tokens           int    `json:"tokens,omitempty"`
	TokensPrompt     int    `json:"tokens_prompt,omitempty"`
	TokensCompletion int    `json:"tokens_completion,omitempty"`
	LatencyMS        int64  `json:"latency_ms,omitempty"`
	StreamStatus     string `json:"stream_status,omitempty"`
	// Mode is Agent / Plan / Image for the turn footer (UI-only; optional).
	Mode string `json:"mode,omitempty"`
	// Profile / Model mirror store routing labels for the footer when needed.
	Profile string `json:"profile,omitempty"`
	Model   string `json:"model,omitempty"`
}

// DesktopToolBlock is one persisted tool-call card (VAL-CHAT-014).
type DesktopToolBlock struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
	Result string `json:"result,omitempty"`
}

// DesktopTaskBlock is one persisted subagent/task card (VAL-CHAT-015).
type DesktopTaskBlock struct {
	ID       string `json:"id,omitempty"`
	Category string `json:"category,omitempty"`
	Status   string `json:"status,omitempty"`
	Label    string `json:"label,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// DesktopChatHistory is the durable one-thread-per-workspace chat payload (VAL-DESK-014).
type DesktopChatHistory struct {
	WorkspaceID string               `json:"workspace_id"`
	SessionID   string               `json:"session_id,omitempty"`
	Model       string               `json:"model,omitempty"`
	Profile     string               `json:"profile,omitempty"`
	Messages    []DesktopChatMessage `json:"messages"`
}

// desktopSessionID is the stable session primary key for v1 one-thread-per-workspace.
func desktopSessionID(workspaceID string) string {
	return "desk:" + strings.TrimSpace(workspaceID)
}

// GetChatHistory returns the durable chat thread for a workspace.
// Empty workspaceID uses the active workspace. Empty history is a successful empty list.
// Prefer store when memory is empty so app reload restores via sessions.db.
//
// Per-message meta (tokens/latency/stream status) and tool/task blocks
// rehydrate from sessions.db so a restored assistant message shows its
// ChatMetaRow + tool/task cards, not only a bare stream-status done
// (misc-chat-meta-persist-history).
func (s *Service) GetChatHistory(workspaceID string) (DesktopChatHistory, error) {
	if s == nil {
		return DesktopChatHistory{}, fmt.Errorf("core: service is nil")
	}
	wid, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return DesktopChatHistory{WorkspaceID: workspaceID, Messages: []DesktopChatMessage{}}, err
	}

	// Prefer non-empty in-memory transcript when bound to this workspace.
	// When memory is empty, fall through to sessions.db so persist-without-memory
	// and cross-handle reloads still return durable messages (VAL-DESK-014).
	// ClearChatSession writes empty to both memory and store, so empty is correct.
	s.mu.Lock()
	if s.chatWorkspaceID == wid && len(s.chat.Messages) > 0 {
		out := DesktopChatHistory{
			WorkspaceID: wid,
			SessionID:   desktopSessionID(wid),
			Model:       s.chat.Model,
			Profile:     s.chat.Profile,
			Messages:    providerMsgsToDesktop(s.chat.Messages, s.chat.MessageMetas),
		}
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	st, err := s.ensureStore()
	if err != nil {
		return DesktopChatHistory{WorkspaceID: wid, Messages: []DesktopChatMessage{}}, err
	}

	// Stable desk session id first (v1 single active thread).
	sid := desktopSessionID(wid)
	if meta, err := st.GetSession(sid); err == nil && meta != nil {
		loaded, loadErr := st.LoadMessagesWithMeta(sid)
		if loadErr != nil {
			return DesktopChatHistory{WorkspaceID: wid, Messages: []DesktopChatMessage{}}, loadErr
		}
		return DesktopChatHistory{
			WorkspaceID: wid,
			SessionID:   sid,
			Model:       meta.Model,
			Profile:     meta.Profile,
			Messages:    storedMsgsToDesktop(loaded),
		}, nil
	}

	// Legacy / other sessions keyed by workspace field.
	latest, err := st.LatestSessionForWorkspace(wid)
	if err != nil {
		return DesktopChatHistory{WorkspaceID: wid, Messages: []DesktopChatMessage{}}, err
	}
	if latest == nil {
		return DesktopChatHistory{WorkspaceID: wid, Messages: []DesktopChatMessage{}}, nil
	}
	loaded, err := st.LoadMessagesWithMeta(latest.ID)
	if err != nil {
		return DesktopChatHistory{WorkspaceID: wid, Messages: []DesktopChatMessage{}}, err
	}
	return DesktopChatHistory{
		WorkspaceID: wid,
		SessionID:   latest.ID,
		Model:       latest.Model,
		Profile:     latest.Profile,
		Messages:    storedMsgsToDesktop(loaded),
	}, nil
}

// ClearChatSession resets the in-memory chat for the active workspace and
// clears durable messages for that workspace thread (still one session row).
func (s *Service) ClearChatSession() {
	if s == nil {
		return
	}
	s.mu.Lock()
	wid := s.chatWorkspaceID
	if wid == "" && s.registry != nil {
		wid = s.registry.ActiveID()
	}
	s.chat = ChatSession{}
	s.statusMsg = "Chat cleared"
	s.mu.Unlock()

	if strings.TrimSpace(wid) == "" {
		return
	}
	st, err := s.ensureStore()
	if err != nil {
		return
	}
	sid := desktopSessionID(wid)
	_ = st.SaveTurn(store.Session{
		ID:        sid,
		Title:     "Desktop chat",
		Workspace: wid,
	}, nil)
}

// resolveWorkspaceID returns a workspace id; empty input uses active.
func (s *Service) resolveWorkspaceID(workspaceID string) (string, error) {
	wid := strings.TrimSpace(workspaceID)
	if wid != "" {
		if _, err := s.registry.Get(wid); err != nil {
			// Allow history for ids not currently registered only if store has data —
			// still return the id for lookup; registry miss is not fatal for pure store reads.
		}
		return wid, nil
	}
	if s.registry == nil {
		return "", fmt.Errorf("core: no active workspace")
	}
	active := s.registry.ActiveID()
	if active == "" {
		return "", fmt.Errorf("core: no active workspace")
	}
	return active, nil
}

// ensureStore opens sessions.db under config home once (shared with CLI store).
func (s *Service) ensureStore() (*store.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions != nil {
		return s.sessions, nil
	}
	path := store.DefaultPath(s.configHome)
	st, err := store.Open(path)
	if err != nil {
		return nil, fmt.Errorf("core: open sessions store: %w", err)
	}
	s.sessions = st
	return s.sessions, nil
}

// persistChat saves messages for workspaceID (does not require mu held for duration).
// Per-message meta (tokens/latency/tools/tasks/thinking/routing) is persisted on
// assistant rows so a reloaded assistant message restores its ChatMetaRow + tool/task
// blocks, not only a bare stream-status done (misc-chat-meta-persist-history).
func (s *Service) persistChat(workspaceID string, sess ChatSession) error {
	wid := strings.TrimSpace(workspaceID)
	if wid == "" {
		return fmt.Errorf("core: persist chat requires workspace id")
	}
	st, err := s.ensureStore()
	if err != nil {
		return err
	}
	// Only persist user/assistant content for durable UI thread (drop system/tool noise),
	// preserving per-message meta keyed by the ORIGINAL message index.
	stored := filterPersistableWithMeta(sess.Messages, sess.MessageMetas)
	return st.SaveTurnWithMeta(store.Session{
		ID:        desktopSessionID(wid),
		Title:     "Desktop chat",
		Model:     sess.Model,
		Profile:   sess.Profile,
		Workspace: wid,
	}, stored)
}


// loadProviderHistory returns durable messages for workspaceID without flipping
// the global active chat binding (safe for concurrent ChatStream runs).
func (s *Service) loadProviderHistory(workspaceID string) (msgs []provider.Message, model, profile string) {
	wid := strings.TrimSpace(workspaceID)
	if s == nil || wid == "" {
		return nil, "", ""
	}
	s.mu.Lock()
	if s.chatWorkspaceID == wid && len(s.chat.Messages) > 0 {
		msgs = append([]provider.Message(nil), s.chat.Messages...)
		model, profile = s.chat.Model, s.chat.Profile
		s.mu.Unlock()
		return msgs, model, profile
	}
	s.mu.Unlock()

	st, err := s.ensureStore()
	if err != nil {
		return nil, "", ""
	}
	sid := desktopSessionID(wid)
	if meta, err := st.GetSession(sid); err == nil && meta != nil {
		if loaded, loadErr := st.LoadMessagesWithMeta(sid); loadErr == nil {
			msgs, _ = storedMsgsToProvider(loaded)
			return msgs, meta.Model, meta.Profile
		}
	}
	if latest, err := st.LatestSessionForWorkspace(wid); err == nil && latest != nil {
		if latest.Workspace == "" || latest.Workspace == wid {
			if loaded, loadErr := st.LoadMessagesWithMeta(latest.ID); loadErr == nil {
				msgs, _ = storedMsgsToProvider(loaded)
				return msgs, latest.Model, latest.Profile
			}
		}
	}
	return nil, "", ""
}

// switchChatWorkspace saves current thread and loads the target workspace thread.
func (s *Service) switchChatWorkspace(newWorkspaceID string) {
	newID := strings.TrimSpace(newWorkspaceID)

	s.mu.Lock()
	oldID := s.chatWorkspaceID
	oldSess := ChatSession{
		Model:        s.chat.Model,
		Profile:      s.chat.Profile,
		MessageMetas: cloneMessageMetas(s.chat.MessageMetas),
	}
	if len(s.chat.Messages) > 0 {
		oldSess.Messages = append([]provider.Message(nil), s.chat.Messages...)
	}
	s.mu.Unlock()

	if oldID != "" && oldID != newID && len(oldSess.Messages) > 0 {
		_ = s.persistChat(oldID, oldSess)
	}
	if newID == "" {
		s.mu.Lock()
		s.chat = ChatSession{}
		s.chatWorkspaceID = ""
		s.mu.Unlock()
		return
	}
	if oldID == newID {
		return
	}

	st, err := s.ensureStore()
	if err != nil {
		s.mu.Lock()
		s.chat = ChatSession{}
		s.chatWorkspaceID = newID
		s.mu.Unlock()
		return
	}
	sid := desktopSessionID(newID)
	var msgs []provider.Message
	var metas map[int]*store.MessageMeta
	var model, profile string
	if meta, err := st.GetSession(sid); err == nil && meta != nil {
		model, profile = meta.Model, meta.Profile
		if loaded, err := st.LoadMessagesWithMeta(sid); err == nil {
			msgs, metas = storedMsgsToProvider(loaded)
		}
	} else if latest, err := st.LatestSessionForWorkspace(newID); err == nil && latest != nil {
		if latest.Workspace == "" || latest.Workspace == newID {
			model, profile = latest.Model, latest.Profile
			if loaded, err := st.LoadMessagesWithMeta(latest.ID); err == nil {
				msgs, metas = storedMsgsToProvider(loaded)
			}
		}
	}
	s.mu.Lock()
	s.chat = ChatSession{Messages: msgs, MessageMetas: metas, Model: model, Profile: profile}
	s.chatWorkspaceID = newID
	s.mu.Unlock()
}

func cloneMessageMetas(in map[int]*store.MessageMeta) map[int]*store.MessageMeta {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int]*store.MessageMeta, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		cp := *v
		out[k] = &cp
	}
	return out
}

// filterPersistableWithMeta keeps user/assistant content for the durable UI
// thread (drops system/tool noise) and preserves per-message meta keyed by the
// ORIGINAL message index. System rows are kept for multi-turn agent quality
// but never carry meta. Returns StoredMessage slices for SaveTurnWithMeta.
func filterPersistableWithMeta(msgs []provider.Message, metas map[int]*store.MessageMeta) []store.StoredMessage {
	var out []store.StoredMessage
	for i, m := range msgs {
		switch m.Role {
		case provider.RoleUser, provider.RoleAssistant:
			// Skip empty assistant shells and tool-call-only intermediate rounds.
			// Final assistant text (and its meta tools/tasks) is what the UI shows.
			if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) == "" {
				var meta *store.MessageMeta
				if metas != nil {
					meta = metas[i]
				}
				if !desktopAssistantHasVisibleBody(m.Content, meta) {
					continue
				}
			}
			cp := provider.Message{Role: m.Role, Content: m.Content}
			sm := store.StoredMessage{Message: cp}
			if m.Role == provider.RoleAssistant && metas != nil {
				if meta, ok := metas[i]; ok && meta != nil {
					sm.Meta = meta
				}
			}
			out = append(out, sm)
		default:
			// system / tool messages stay out of desktop bubble list for v1 UI
			// but are needed for multi-turn agent quality — keep system only.
			if m.Role == provider.RoleSystem {
				out = append(out, store.StoredMessage{Message: provider.Message{Role: m.Role, Content: m.Content}})
			}
		}
	}
	if out == nil {
		out = []store.StoredMessage{}
	}
	return out
}

// providerMsgsToDesktop converts the in-memory provider.Message transcript to
// desktop bubbles, attaching per-message meta from the metas map (keyed by
// original message index) when present.
func providerMsgsToDesktop(msgs []provider.Message, metas map[int]*store.MessageMeta) []DesktopChatMessage {
	var out []DesktopChatMessage
	desktopIdx := 0
	for i, m := range msgs {
		role := string(m.Role)
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		if role == "system" {
			// UI hides system by default; omit from restored bubbles.
			continue
		}
		// Skip intermediate tool-only assistant turns (empty content, tool_calls only).
		// They clutter the chat as blank ASSISTANT rows; tools live on the final bubble meta.
		if role == "assistant" && strings.TrimSpace(m.Content) == "" {
			var meta *store.MessageMeta
			if metas != nil {
				meta = metas[i]
			}
			if !desktopAssistantHasVisibleBody(m.Content, meta) {
				continue
			}
		}
		dm := DesktopChatMessage{
			ID:      fmt.Sprintf("m-%d", desktopIdx),
			Role:    role,
			Content: m.Content,
		}
		if role == "assistant" && metas != nil {
			if meta, ok := metas[i]; ok && meta != nil {
				applyStoreMetaToDesktop(&dm, meta)
			}
		}
		out = append(out, dm)
		desktopIdx++
	}
	if out == nil {
		out = []DesktopChatMessage{}
	}
	return out
}

// storedMsgsToDesktop converts store.StoredMessage rows (with meta) to desktop
// bubbles. Used by the durable store load path so reloaded assistant messages
// restore their ChatMetaRow + tool/task blocks.
func storedMsgsToDesktop(msgs []store.StoredMessage) []DesktopChatMessage {
	var out []DesktopChatMessage
	desktopIdx := 0
	for _, sm := range msgs {
		role := string(sm.Message.Role)
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		if role == "system" {
			continue
		}
		// Drop empty assistant rows (tool-round placeholders without text/tools/thinking).
		if role == "assistant" && !desktopAssistantHasVisibleBody(sm.Message.Content, sm.Meta) {
			continue
		}
		dm := DesktopChatMessage{
			ID:      fmt.Sprintf("m-%d", desktopIdx),
			Role:    role,
			Content: sm.Message.Content,
		}
		if role == "assistant" && sm.Meta != nil {
			applyStoreMetaToDesktop(&dm, sm.Meta)
		}
		out = append(out, dm)
		desktopIdx++
	}
	if out == nil {
		out = []DesktopChatMessage{}
	}
	return out
}

// desktopAssistantHasVisibleBody reports whether an assistant turn should render
// a chat bubble (text, thinking, tools, or tasks). Empty tool-only rounds do not.
func desktopAssistantHasVisibleBody(content string, meta *store.MessageMeta) bool {
	if strings.TrimSpace(content) != "" {
		return true
	}
	if meta == nil {
		return false
	}
	if strings.TrimSpace(meta.Thinking) != "" {
		return true
	}
	if len(meta.Tools) > 0 || len(meta.Tasks) > 0 {
		return true
	}
	return false
}

// storedMsgsToProvider splits store.StoredMessage rows back into the
// provider.Message transcript + the per-message meta map (keyed by index into
// the returned provider.Message slice). Used when loading the durable thread
// into the in-memory ChatSession.
func storedMsgsToProvider(msgs []store.StoredMessage) ([]provider.Message, map[int]*store.MessageMeta) {
	out := make([]provider.Message, 0, len(msgs))
	metas := make(map[int]*store.MessageMeta, len(msgs))
	for i, sm := range msgs {
		out = append(out, sm.Message)
		if sm.Meta != nil {
			metas[i] = sm.Meta
		}
	}
	return out, metas
}

// applyStoreMetaToDesktop copies a store.MessageMeta onto a DesktopChatMessage,
// flattening tool/task blocks and routing labels onto the desktop fields. Never
// copies secrets — store.MessageMeta has no credential-like fields.
func applyStoreMetaToDesktop(dm *DesktopChatMessage, meta *store.MessageMeta) {
	if meta == nil {
		return
	}
	dm.Meta = &DesktopChatMeta{
		Tokens:           meta.Tokens,
		TokensPrompt:     meta.TokensPrompt,
		TokensCompletion: meta.TokensCompletion,
		LatencyMS:        meta.LatencyMS,
		StreamStatus:     meta.StreamStatus,
		Profile:          meta.Profile,
		Model:            meta.Model,
	}
	if strings.TrimSpace(meta.Thinking) != "" {
		dm.Thinking = meta.Thinking
	}
	if strings.TrimSpace(meta.Model) != "" {
		dm.Model = meta.Model
	}
	if strings.TrimSpace(meta.Profile) != "" {
		dm.Profile = meta.Profile
	}
	if strings.TrimSpace(meta.Host) != "" {
		dm.Host = meta.Host
	}
	if len(meta.Tools) > 0 {
		dm.Tools = make([]DesktopToolBlock, 0, len(meta.Tools))
		for _, t := range meta.Tools {
			dm.Tools = append(dm.Tools, DesktopToolBlock{
				ID:     t.ID,
				Name:   t.Name,
				Status: t.Status,
				Detail: t.Detail,
				Result: t.Result,
			})
		}
	}
	if len(meta.Tasks) > 0 {
		dm.Tasks = make([]DesktopTaskBlock, 0, len(meta.Tasks))
		for _, tk := range meta.Tasks {
			dm.Tasks = append(dm.Tasks, DesktopTaskBlock{
				ID:       tk.ID,
				Category: tk.Category,
				Status:   tk.Status,
				Label:    tk.Label,
				Detail:   tk.Detail,
			})
		}
	}
}

func desktopToProvider(msgs []DesktopChatMessage) []provider.Message {
	var out []provider.Message
	for _, m := range msgs {
		role := provider.Role(strings.TrimSpace(m.Role))
		if role != provider.RoleUser && role != provider.RoleAssistant {
			continue
		}
		out = append(out, provider.Message{Role: role, Content: m.Content})
	}
	if out == nil {
		out = []provider.Message{}
	}
	return out
}

var _ = desktopToProvider // retained for backward-compat; superseded by desktopToProviderWithMeta

// loadContextHistory assembles provider messages for the turn based on
// session/playground scope. Returns the merged history (oldest→newest).
// Pure read; does not touch s.chat in-memory binding.
//
// Binding modes (3-mode context binding):
//   - Legacy (no SessionID, no PlaygroundID): workspace-scoped via loadProviderHistory.
//   - Session-bound (SessionID + PlaygroundID): merge(session, playground).
//   - Workspace-bound (no SessionID + PlaygroundID): merge(workspace, playground).
//   - Library (no SessionID + PlaygroundID + no workspace): playground only.
//
// When PlaygroundID is set the playground thread is appended LAST so the most
// relevant instruction context is nearest the user turn. Owner history (session
// or workspace) is never mutated by this read.
func (s *Service) loadContextHistory(req ChatRequest, turnWorkspaceID string) (msgs []provider.Message, model, profile string) {
	hasSession := strings.TrimSpace(req.SessionID) != ""
	hasPlayground := strings.TrimSpace(req.PlaygroundID) != ""

	if !hasSession && !hasPlayground {
		return s.loadProviderHistory(turnWorkspaceID)
	}

	if hasSession {
		ownerMsgs, ownerModel, ownerProfile := s.loadBySession(req.SessionID)
		if hasPlayground {
			pgMsgs, pgModel, pgProfile := s.loadPlaygroundHistory(req.PlaygroundID)
			merged := mergeHistories(ownerMsgs, pgMsgs)
			model, profile = pickRoutingLabels(ownerModel, ownerProfile, pgModel, pgProfile)
			return merged, model, profile
		}
		return ownerMsgs, ownerModel, ownerProfile
	}

	// hasPlayground only (no SessionID).
	// Use the ORIGINAL req.WorkspaceID, not the auto-resolved turnWorkspaceID.
	// If frontend didn't send workspace_id, this is library mode — playground history only.
	reqWid := strings.TrimSpace(req.WorkspaceID)
	if reqWid != "" {
		ownerMsgs, ownerModel, ownerProfile := s.loadProviderHistory(reqWid)
		pgMsgs, pgModel, pgProfile := s.loadPlaygroundHistory(req.PlaygroundID)
		merged := mergeHistories(ownerMsgs, pgMsgs)
		model, profile = pickRoutingLabels(ownerModel, ownerProfile, pgModel, pgProfile)
		return merged, model, profile
	}

	// Library: no workspace, no session — playground only.
	pgMsgs, pgModel, pgProfile := s.loadPlaygroundHistory(req.PlaygroundID)
	if os.Getenv("INFERENESIA_DEBUG_CHAT") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG-CHAT-HIST] library mode: pgMsgs=%d pgModel=%q pgProfile=%q\n", len(pgMsgs), pgModel, pgProfile)
	}
	return pgMsgs, pgModel, pgProfile
}

// loadBySession loads a session by explicit ID (no "desk:" prefix logic).
// Used by Session-bound mode. Does not consult s.chat in-memory binding.
func (s *Service) loadBySession(sessionID string) (msgs []provider.Message, model, profile string) {
	sid := strings.TrimSpace(sessionID)
	if s == nil || sid == "" {
		return nil, "", ""
	}
	st, err := s.ensureStore()
	if err != nil {
		return nil, "", ""
	}
	meta, err := st.GetSession(sid)
	if err != nil || meta == nil {
		if os.Getenv("INFERENESIA_DEBUG_CHAT") != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG-CHAT-HIST] loadBySession(%q) → not found: %v\n", sid, err)
		}
		return nil, "", ""
	}
	loaded, loadErr := st.LoadMessagesWithMeta(sid)
	if loadErr != nil {
		return nil, "", ""
	}
	msgs, _ = storedMsgsToProvider(loaded)
	if os.Getenv("INFERENESIA_DEBUG_CHAT") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG-CHAT-HIST] loadBySession(%q) → %d msgs, model=%q\n", sid, len(msgs), meta.Model)
	}
	return msgs, meta.Model, meta.Profile
}

// loadPlaygroundHistory loads the playground-scoped thread ("pg:"+playgroundID).
// Returns empty if the playground has no prior turns.
func (s *Service) loadPlaygroundHistory(playgroundID string) (msgs []provider.Message, model, profile string) {
	pid := strings.TrimSpace(playgroundID)
	if s == nil || pid == "" {
		return nil, "", ""
	}
	return s.loadBySession("pg:" + pid)
}

// mergeHistories concatenates owner history then playground history.
// Playground history comes LAST (most relevant). Trims to ~50 messages max by
// dropping oldest owner rows first so the playground tail is preserved.
func mergeHistories(owner, playground []provider.Message) []provider.Message {
	const maxMessages = 50
	combined := make([]provider.Message, 0, len(owner)+len(playground))
	combined = append(combined, owner...)
	combined = append(combined, playground...)
	if len(combined) <= maxMessages {
		return combined
	}
	// Drop oldest owner rows first; keep the entire playground tail.
	overflow := len(combined) - maxMessages
	if overflow >= len(owner) {
		// Owner is entirely surplus; keep only the playground tail, trimmed.
		tail := playground
		if len(tail) > maxMessages {
			tail = tail[len(tail)-maxMessages:]
		}
		return append([]provider.Message(nil), tail...)
	}
	trimmedOwner := owner[overflow:]
	out := make([]provider.Message, 0, maxMessages)
	out = append(out, trimmedOwner...)
	out = append(out, playground...)
	return out
}

// pickRoutingLabels prefers non-empty owner labels, falling back to playground.
func pickRoutingLabels(ownerModel, ownerProfile, pgModel, pgProfile string) (model, profile string) {
	model = strings.TrimSpace(ownerModel)
	if model == "" {
		model = strings.TrimSpace(pgModel)
	}
	profile = strings.TrimSpace(ownerProfile)
	if profile == "" {
		profile = strings.TrimSpace(pgProfile)
	}
	return model, profile
}

// persistPlayground saves a playground-scoped turn under "pg:"+playgroundID.
// Same mechanics as persistChat but with a playground session ID and title.
// Never touches s.chat or workspace chat history (callers must avoid
// commitTurnChat for playground turns).
func (s *Service) persistPlayground(playgroundID string, sess ChatSession) error {
	pid := strings.TrimSpace(playgroundID)
	if pid == "" {
		return fmt.Errorf("core: persist playground requires playground id")
	}
	st, err := s.ensureStore()
	if err != nil {
		return err
	}
	stored := filterPersistableWithMeta(sess.Messages, sess.MessageMetas)
	return st.SaveTurnWithMeta(store.Session{
		ID:        "pg:" + pid,
		Title:     "Playground: " + pid,
		Model:     sess.Model,
		Profile:   sess.Profile,
		Workspace: strings.TrimSpace(sess.WorkspaceID),
	}, stored)
}
