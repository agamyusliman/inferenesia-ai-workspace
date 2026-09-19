package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// VAL-DESK-014: one durable thread per workspace; A→B→A restores A; reload restores A.
func TestChatHistoryPerWorkspaceSwitchAndReload(t *testing.T) {
	home := t.TempDir()
	wsA := t.TempDir()
	wsB := t.TempDir()
	mustWrite(t, filepath.Join(wsA, "a.txt"), "A\n")
	mustWrite(t, filepath.Join(wsB, "b.txt"), "B\n")

	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	a, err := svc.OpenWorkspace(wsA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.OpenWorkspace(wsB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwitchWorkspace(a.ID); err != nil {
		t.Fatal(err)
	}

	// Seed workspace A transcript as agent would after a turn.
	svc.mu.Lock()
	svc.chatWorkspaceID = a.ID
	svc.chat = ChatSession{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hello from A"},
			{Role: provider.RoleAssistant, Content: "reply A unique"},
		},
		Model:   "test-model",
		Profile: "tempai",
	}
	snapA := ChatSession{
		Messages: append([]provider.Message(nil), svc.chat.Messages...),
		Model:    svc.chat.Model,
		Profile:  svc.chat.Profile,
	}
	svc.mu.Unlock()
	if err := svc.persistChat(a.ID, snapA); err != nil {
		t.Fatal(err)
	}

	// Switch to B with independent thread.
	if _, err := svc.SwitchWorkspace(b.ID); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.chat = ChatSession{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hello from B"},
			{Role: provider.RoleAssistant, Content: "reply B unique"},
		},
	}
	snapB := ChatSession{
		Messages: append([]provider.Message(nil), svc.chat.Messages...),
	}
	svc.chatWorkspaceID = b.ID
	svc.mu.Unlock()
	if err := svc.persistChat(b.ID, snapB); err != nil {
		t.Fatal(err)
	}

	// Back to A — must restore A's messages, not B.
	if _, err := svc.SwitchWorkspace(a.ID); err != nil {
		t.Fatal(err)
	}
	histA, err := svc.GetChatHistory(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !desktopHistoryContains(histA, "hello from A") || !desktopHistoryContains(histA, "reply A unique") {
		t.Fatalf("workspace A history missing A messages: %#v", histA.Messages)
	}
	if desktopHistoryContains(histA, "reply B unique") {
		t.Fatalf("workspace A history leaked B: %#v", histA.Messages)
	}

	histB, err := svc.GetChatHistory(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !desktopHistoryContains(histB, "reply B unique") {
		t.Fatalf("workspace B history missing B: %#v", histB.Messages)
	}
	if desktopHistoryContains(histB, "reply A unique") {
		t.Fatalf("workspace B history leaked A: %#v", histB.Messages)
	}

	// Simulate full app reload: new Service with same config home + registry.
	// Registry keeps active id; NewService should load active workspace thread.
	if _, err := svc.SwitchWorkspace(a.ID); err != nil {
		t.Fatal(err)
	}
	// Drop store handle from first process.
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
	// Active should still be A (workspace registry).
	if svc2.registry.ActiveID() != a.ID {
		// Registry may restore last active — force switch if not.
		if _, err := svc2.SwitchWorkspace(a.ID); err != nil {
			t.Fatal(err)
		}
	}
	histReload, err := svc2.GetChatHistory(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !desktopHistoryContains(histReload, "reply A unique") {
		t.Fatalf("after reload A missing messages: %#v", histReload.Messages)
	}
	// Confirm sessions.db exists on disk.
	dbPath := filepath.Join(home, "sessions.db")
	if st, err := os.Stat(dbPath); err != nil || st.Size() == 0 {
		t.Fatalf("sessions.db missing or empty: %v", err)
	}
}

func TestChatHistoryHTTP(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "n.txt"), "n\n")
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	w, err := svc.OpenWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.persistChat(w.ID, ChatSession{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "persist-me"},
			{Role: provider.RoleAssistant, Content: "ok-persist"},
		},
	})

	h := svc.Handler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/history?workspace_id="+w.ID, nil)
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, part := range []string{"persist-me", "ok-persist", w.ID} {
		if !strings.Contains(body, part) {
			t.Fatalf("history body missing %q: %s", part, body)
		}
	}
}

func desktopHistoryContains(h DesktopChatHistory, needle string) bool {
	for _, m := range h.Messages {
		if m.Content == needle {
			return true
		}
	}
	return false
}


func TestCreateSessionStartsEmptyChat(t *testing.T) {
	home := t.TempDir()
	folder := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.OpenWorkspace(folder)
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.chatWorkspaceID = a.ID
	svc.chat = ChatSession{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "from folder"},
			{Role: provider.RoleAssistant, Content: "folder reply"},
		},
	}
	snap := ChatSession{Messages: append([]provider.Message(nil), svc.chat.Messages...)}
	svc.mu.Unlock()
	if err := svc.persistChat(a.ID, snap); err != nil {
		t.Fatal(err)
	}

	sess, err := svc.CreateSessionWorkspace("")
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == a.ID {
		t.Fatal("session id must differ from folder workspace")
	}
	hist, err := svc.GetChatHistory(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Messages) != 0 {
		t.Fatalf("new session must have empty chat, got %d msgs: %+v", len(hist.Messages), hist.Messages)
	}
	// folder history still intact
	if _, err := svc.SwitchWorkspace(a.ID); err != nil {
		t.Fatal(err)
	}
	histA, err := svc.GetChatHistory(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(histA.Messages) != 2 {
		t.Fatalf("folder history lost: %d", len(histA.Messages))
	}
}
