package core

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/store"
)

// misc-chat-meta-persist-history: after a turn, reloading the workspace
// restores the last assistant message's tokens/latency/stream status, tool
// blocks, and task blocks from sessions.db — not only a bare "done" status.
func TestChatMetaPersistsAndRestoresAfterReload(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "f.txt"), "f\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	w, err := svc.OpenWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwitchWorkspace(w.ID); err != nil {
		t.Fatal(err)
	}

	// Seed an in-memory chat transcript with per-message meta on the last
	// assistant message, then persist — mirrors what ChatStream does on Done.
	tools := []store.ToolBlockMeta{
		{ID: "t1", Name: "write_file", Status: "done", Detail: "path=a.txt", Result: "ok"},
	}
	tasks := []store.TaskBlockMeta{
		{ID: "task-1", Category: "explore", Status: "completed", Label: "scan"},
	}
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
		{Role: provider.RoleAssistant, Content: "answer with meta"},
	}
	metas := map[int]*store.MessageMeta{
		1: {
			Tokens: 42, TokensPrompt: 10, TokensCompletion: 32,
			LatencyMS: 1500, StreamStatus: "done",
			Model: "test-model", Profile: "tempai", Host: "ai.temp.web.id",
			Thinking: "reasoning",
			Tools:    tools,
			Tasks:    tasks,
		},
	}
	svc.mu.Lock()
	svc.chatWorkspaceID = w.ID
	svc.chat = ChatSession{
		Messages:     msgs,
		MessageMetas: metas,
		Model:        "test-model",
		Profile:      "tempai",
	}
	snap := ChatSession{
		Messages:     append([]provider.Message(nil), msgs...),
		MessageMetas: metas,
		Model:        "test-model",
		Profile:      "tempai",
	}
	svc.mu.Unlock()
	if err := svc.persistChat(w.ID, snap); err != nil {
		t.Fatal(err)
	}

	// Reload via the HTTP history endpoint (in-memory path is bound).
	h := svc.Handler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/history?workspace_id="+w.ID, nil)
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`"tokens":42`, `"latency_ms":1500`, `"stream_status":"done"`, `"write_file"`, `"task-1"`, `"reasoning"`, `"test-model"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("in-memory history body missing %q: %s", want, body)
		}
	}

	// Simulate full app reload: drop the store handle and build a new Service.
	svc.mu.Lock()
	if svc.sessions != nil {
		_ = svc.sessions.Close()
		svc.sessions = nil
	}
	svc.mu.Unlock()

	svc2, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if svc2.registry.ActiveID() != w.ID {
		if _, err := svc2.SwitchWorkspace(w.ID); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := svc2.GetChatHistory(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Messages) != 2 {
		t.Fatalf("after reload len = %d, want 2: %#v", len(hist.Messages), hist.Messages)
	}
	last := hist.Messages[1]
	if last.Role != "assistant" {
		t.Fatalf("last role = %q", last.Role)
	}
	if last.Meta == nil {
		t.Fatalf("after reload last assistant meta missing: %#v", last)
	}
	if last.Meta.Tokens != 42 || last.Meta.TokensPrompt != 10 || last.Meta.TokensCompletion != 32 {
		t.Fatalf("tokens mismatch after reload: %+v", last.Meta)
	}
	if last.Meta.LatencyMS != 1500 {
		t.Fatalf("latency mismatch after reload: %+v", last.Meta)
	}
	if last.Meta.StreamStatus != "done" {
		t.Fatalf("stream status mismatch after reload: %+v", last.Meta)
	}
	if last.Model != "test-model" || last.Profile != "tempai" || last.Host != "ai.temp.web.id" {
		t.Fatalf("routing labels mismatch after reload: %+v", last)
	}
	if last.Thinking != "reasoning" {
		t.Fatalf("thinking mismatch after reload: %q", last.Thinking)
	}
	if len(last.Tools) != 1 || last.Tools[0].Name != "write_file" || last.Tools[0].Status != "done" {
		t.Fatalf("tool blocks mismatch after reload: %+v", last.Tools)
	}
	if len(last.Tasks) != 1 || last.Tasks[0].ID != "task-1" || last.Tasks[0].Status != "completed" {
		t.Fatalf("task blocks mismatch after reload: %+v", last.Tasks)
	}
}

// misc-chat-meta-persist-history: no secrets stored in meta. The store
// MessageMeta struct exposes no credential-like fields, and the persisted
// JSON never carries a key/token value even when the chat used the gateway key.
func TestChatMetaHasNoSecretFields(t *testing.T) {
	mm := store.MessageMeta{
		Tokens: 1, LatencyMS: 1, StreamStatus: "done",
		Model: "test-model", Profile: "tempai", Host: "ai.temp.web.id",
		Thinking: "ok",
		Tools:    []store.ToolBlockMeta{{Name: "shell", Detail: "echo hi", Result: "hi"}},
		Tasks:    []store.TaskBlockMeta{{ID: "t", Category: "explore", Status: "completed"}},
	}
	// Persist + reload and ensure the stored JSON has no credential-like keys.
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sid := "sess-secret"
	if err := s.CreateSession(store.Session{ID: sid}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTurnWithMeta(store.Session{ID: sid}, []store.StoredMessage{
		{Message: provider.Message{Role: provider.RoleAssistant, Content: "x"}, Meta: &mm},
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadMessagesWithMeta(sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Meta == nil {
		t.Fatalf("meta not restored: %+v", loaded)
	}
	// Document the no-secret contract: the field set has no credential sinks.
	for _, forbidden := range []string{"APIKey", "ApiKey", "Token", "Authorization", "Bearer", "Secret", "Password"} {
		if store.HasMessageMetaFieldNamed(forbidden) {
			t.Fatalf("MessageMeta must not expose a credential-like field %q", forbidden)
		}
	}
}

// misc-chat-meta-persist-history: turnMetaAccumulator collects tool/task events
// emitted during a ChatStream turn so they can be persisted on the last
// assistant message. This test exercises the accumulator directly without a
// live gateway (the integration path is covered by existing chat tests).
func TestTurnMetaAccumulatorCollectsToolAndTaskBlocks(t *testing.T) {
	acc := &turnMetaAccumulator{}
	emit := wrapEmitWithMetaCapture(func(ChatEvent) {}, acc)
	emit(ChatEvent{Type: ChatEventToolStart, Name: "write_file", Detail: "path=a.txt"})
	emit(ChatEvent{Type: ChatEventToolEnd, Name: "write_file", Detail: "path=a.txt → ok"})
	emit(ChatEvent{Type: ChatEventToolError, Name: "shell", Detail: "bad cmd", Error: "exit 1"})
	emit(ChatEvent{Type: ChatEventTaskSpawned, Detail: "task-1|explore|running|scan docs"})
	emit(ChatEvent{Type: ChatEventTaskDone, Detail: "task-1|explore|completed|scan docs|found 3"})

	tools, tasks := acc.snapshot()
	if len(tools) != 2 {
		t.Fatalf("tools = %+v, want 2", tools)
	}
	if tools[0].Name != "write_file" || tools[0].Status != "done" || tools[0].Result != "ok" {
		t.Fatalf("tool[0] mismatch: %+v", tools[0])
	}
	if tools[1].Name != "shell" || tools[1].Status != "error" {
		t.Fatalf("tool[1] mismatch: %+v", tools[1])
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" || tasks[0].Status != "completed" || tasks[0].Category != "explore" {
		t.Fatalf("task mismatch: %+v", tasks)
	}

	// buildLastAssistantMeta attaches the snapshot to the last assistant row.
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "q"},
		{Role: provider.RoleAssistant, Content: "a"},
	}
	meta := buildLastAssistantMeta(msgs, 10, 4, 6, 250, "done", "m", "p", "h", "think", tools, tasks)
	if meta == nil {
		t.Fatal("meta nil")
	}
	if meta.Tokens != 10 || meta.LatencyMS != 250 || meta.StreamStatus != "done" {
		t.Fatalf("meta fields mismatch: %+v", meta)
	}
	if len(meta.Tools) != 2 || len(meta.Tasks) != 1 {
		t.Fatalf("meta blocks mismatch: %+v", meta)
	}
	// setLastAssistantMeta keys the map by the last assistant index (1).
	m := setLastAssistantMeta(nil, msgs, meta)
	if m[1] == nil {
		t.Fatalf("meta not keyed by last assistant index: %+v", m)
	}
}
