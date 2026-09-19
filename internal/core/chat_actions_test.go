package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

func seedFourMessages(t *testing.T, svc *Service, workspaceID string) {
	t.Helper()
	if err := svc.persistChat(workspaceID, ChatSession{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "user-one"},
			{Role: provider.RoleAssistant, Content: "asst-one"},
			{Role: provider.RoleUser, Content: "user-two"},
			{Role: provider.RoleAssistant, Content: "asst-two"},
		},
		Model:   "test-model",
		Profile: "tempai",
	}); err != nil {
		t.Fatal(err)
	}
	// Bind memory so GetChatHistory / actions see the thread.
	svc.mu.Lock()
	svc.chatWorkspaceID = workspaceID
	svc.chat = ChatSession{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "user-one"},
			{Role: provider.RoleAssistant, Content: "asst-one"},
			{Role: provider.RoleUser, Content: "user-two"},
			{Role: provider.RoleAssistant, Content: "asst-two"},
		},
		Model:   "test-model",
		Profile: "tempai",
	}
	svc.mu.Unlock()
}

// VAL-CHAT-017: truncate from message id removes that message and everything after, durably.
func TestTruncateChatFromHere(t *testing.T) {
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
	seedFourMessages(t, svc, w.ID)

	res, err := svc.TruncateChatFromHere(w.ID, "m-2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 2 {
		t.Fatalf("removed=%d want 2, msgs=%#v", res.Removed, res.Messages)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("len=%d want 2", len(res.Messages))
	}
	if res.Messages[0].Content != "user-one" || res.Messages[1].Content != "asst-one" {
		t.Fatalf("unexpected msgs: %#v", res.Messages)
	}

	// Durable after new Service reload.
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
	hist, err := svc2.GetChatHistory(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Messages) != 2 {
		t.Fatalf("reloaded len=%d want 2: %#v", len(hist.Messages), hist.Messages)
	}
	if desktopHistoryContains(hist, "user-two") || desktopHistoryContains(hist, "asst-two") {
		t.Fatalf("truncated messages still present: %#v", hist.Messages)
	}
	if !desktopHistoryContains(hist, "user-one") {
		t.Fatalf("kept message missing: %#v", hist.Messages)
	}
}

// VAL-CHAT-018: single-message delete for user and assistant.
func TestDeleteChatMessage(t *testing.T) {
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
	seedFourMessages(t, svc, w.ID)

	// Delete assistant m-1.
	res, err := svc.DeleteChatMessage(w.ID, "m-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 {
		t.Fatalf("removed=%d want 1", res.Removed)
	}
	if len(res.Messages) != 3 {
		t.Fatalf("len=%d want 3: %#v", len(res.Messages), res.Messages)
	}
	for _, m := range res.Messages {
		if m.Content == "asst-one" {
			t.Fatalf("assistant still present: %#v", res.Messages)
		}
	}

	// Delete user (now re-indexed: original m-2 becomes m-1 after prior delete).
	// Use content-stable path: re-fetch and delete user-two.
	hist, err := svc.GetChatHistory(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	var userTwoID string
	for _, m := range hist.Messages {
		if m.Content == "user-two" {
			userTwoID = m.ID
			break
		}
	}
	if userTwoID == "" {
		t.Fatalf("user-two not found: %#v", hist.Messages)
	}
	res2, err := svc.DeleteChatMessage(w.ID, userTwoID)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Removed != 1 {
		t.Fatalf("removed user=%d want 1", res2.Removed)
	}
	if desktopHistoryContains(DesktopChatHistory{Messages: res2.Messages}, "user-two") {
		t.Fatalf("user-two still present: %#v", res2.Messages)
	}
}

func TestChatMessageActionsHTTP(t *testing.T) {
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
	seedFourMessages(t, svc, w.ID)
	h := svc.Handler()

	// Delete via HTTP.
	body := `{"workspace_id":"` + w.ID + `","message_id":"m-3"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/message/delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("delete status %d %s", rr.Code, rr.Body.String())
	}
	var del ChatMessageActionResult
	if err := json.Unmarshal(rr.Body.Bytes(), &del); err != nil {
		t.Fatal(err)
	}
	if del.Removed != 1 || len(del.Messages) != 3 {
		t.Fatalf("delete result: %#v", del)
	}

	// Truncate from m-1 (asst-one → drops asst-one, user-two).
	body2 := `{"workspace_id":"` + w.ID + `","message_id":"m-1"}`
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/chat/message/truncate", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("truncate status %d %s", rr2.Code, rr2.Body.String())
	}
	var tr ChatMessageActionResult
	if err := json.Unmarshal(rr2.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Removed != 2 || len(tr.Messages) != 1 {
		t.Fatalf("truncate result: %#v", tr)
	}
	if tr.Messages[0].Content != "user-one" {
		t.Fatalf("want user-one kept: %#v", tr.Messages)
	}

	// History GET matches.
	rr3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/chat/history?workspace_id="+w.ID, nil)
	h.ServeHTTP(rr3, req3)
	if !strings.Contains(rr3.Body.String(), "user-one") {
		t.Fatalf("history missing user-one: %s", rr3.Body.String())
	}
	if strings.Contains(rr3.Body.String(), "user-two") {
		t.Fatalf("history still has truncated msgs: %s", rr3.Body.String())
	}
}
