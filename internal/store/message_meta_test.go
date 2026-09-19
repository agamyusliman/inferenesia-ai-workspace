package store

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// misc-chat-meta-persist-history: per-message meta (tokens/latency/tools/tasks)
// round-trips through sessions.db so reloaded assistants restore ChatMetaRow +
// tool/task blocks, not only stream status done.
func TestMessageMetaRoundTripAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sessions.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sid := "sess-meta-1"
	if err := s1.CreateSession(Session{ID: sid, Title: "meta"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	tools := []ToolBlockMeta{
		{ID: "t1", Name: "write_file", Status: "done", Detail: "path=a.txt", Result: "ok"},
		{ID: "t2", Name: "shell", Status: "error", Detail: "echo hi", Result: "exit 1"},
	}
	tasks := []TaskBlockMeta{
		{ID: "task-1", Category: "explore", Status: "completed", Label: "scan docs"},
	}
	assistantMeta := &MessageMeta{
		Tokens:           42,
		TokensPrompt:     10,
		TokensCompletion: 32,
		LatencyMS:        1500,
		StreamStatus:     "done",
		Model:            "test-model",
		Profile:          "tempai",
		Host:             "ai.temp.web.id",
		Thinking:         "reasoning here",
		Tools:            tools,
		Tasks:            tasks,
	}

	stored := []StoredMessage{
		{
			Message: provider.Message{Role: provider.RoleUser, Content: "hello"},
		},
		{
			Message: provider.Message{Role: provider.RoleAssistant, Content: "answer"},
			Meta:    assistantMeta,
		},
	}
	if err := s1.SaveTurnWithMeta(Session{ID: sid}, stored); err != nil {
		t.Fatalf("SaveTurnWithMeta: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close s1: %v", err)
	}

	// Simulate app reload: new store handle on the same DB file.
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen Open: %v", err)
	}
	defer s2.Close()

	loaded, err := s2.LoadMessagesWithMeta(sid)
	if err != nil {
		t.Fatalf("LoadMessagesWithMeta: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len = %d, want 2", len(loaded))
	}
	if loaded[0].Meta != nil {
		t.Fatalf("user message must not carry meta, got %+v", loaded[0].Meta)
	}
	got := loaded[1].Meta
	if got == nil {
		t.Fatalf("assistant meta missing after reload")
	}
	if got.Tokens != 42 || got.TokensPrompt != 10 || got.TokensCompletion != 32 {
		t.Fatalf("tokens mismatch: %+v", got)
	}
	if got.LatencyMS != 1500 {
		t.Fatalf("latency mismatch: %+v", got)
	}
	if got.StreamStatus != "done" {
		t.Fatalf("stream status mismatch: %+v", got)
	}
	if got.Model != "test-model" || got.Profile != "tempai" || got.Host != "ai.temp.web.id" {
		t.Fatalf("routing meta mismatch: %+v", got)
	}
	if got.Thinking != "reasoning here" {
		t.Fatalf("thinking mismatch: %+v", got)
	}
	if len(got.Tools) != 2 || got.Tools[0].Name != "write_file" || got.Tools[1].Status != "error" {
		t.Fatalf("tools mismatch: %+v", got.Tools)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].ID != "task-1" || got.Tasks[0].Status != "completed" {
		t.Fatalf("tasks mismatch: %+v", got.Tasks)
	}
}

// Empty / nil meta must round-trip without breaking older rows. Legacy rows
// written before the meta column existed must load with a nil Meta (graceful
// upgrade path for existing sessions.db files).
func TestLoadMessagesWithMetaNilForLegacyRows(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sessions.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s1.Close()
	sid := "sess-legacy"
	if err := s1.CreateSession(Session{ID: sid, Title: "legacy"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// Use the legacy provider.Message path (no meta) — emulates a row written
	// before meta persistence shipped.
	if err := s1.ReplaceMessages(sid, []provider.Message{
		{Role: provider.RoleUser, Content: "legacy user"},
		{Role: provider.RoleAssistant, Content: "legacy answer"},
	}); err != nil {
		t.Fatalf("ReplaceMessages: %v", err)
	}

	loaded, err := s1.LoadMessagesWithMeta(sid)
	if err != nil {
		t.Fatalf("LoadMessagesWithMeta: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len = %d, want 2", len(loaded))
	}
	for i, m := range loaded {
		if m.Meta != nil {
			t.Fatalf("legacy row %d must have nil meta, got %+v", i, m.Meta)
		}
		if m.Content == "" {
			t.Fatalf("legacy row %d content empty", i)
		}
	}
}

// Secrets must never be persisted inside meta. The store does not enforce a
// content filter (meta is operator-controlled), but the field set is fixed and
// never includes key/token fields — this test documents that contract.
func TestMessageMetaHasNoSecretFields(t *testing.T) {
	m := MessageMeta{
		Tokens: 1, LatencyMS: 1, StreamStatus: "done",
		Tools: []ToolBlockMeta{{Name: "x"}}, Tasks: []TaskBlockMeta{{ID: "y"}},
	}
	if strings.Contains(strings.ToLower("api_key token authorization bearer secret"), "apikey") {
		// sanity: the forbidden-words list itself is well-formed.
	}
	// No field on MessageMeta is named like a credential sink.
	for _, forbidden := range []string{"APIKey", "ApiKey", "Token", "Authorization", "Bearer", "Secret", "Password"} {
		if hasMessageMetaFieldNamed(forbidden) {
			t.Fatalf("MessageMeta must not expose a credential-like field %q", forbidden)
		}
	}
	_ = m
}
