package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/store"
)

// VAL-CLI-014: after a chat turn is written to the config-home SQLite store,
// a restarted CLI (new process = new ExecuteWith call) can list the session
// and show the same message content.
func TestSessionListAndShowAfterRestart(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("YURA_AI_HOME", home)

	// Simulate a completed chat turn persisting to the store (process 1).
	dbPath := store.DefaultPath(home)
	s1, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const sid = "20260717-sessdemo"
	const userText = "Reply with the single word: PONG"
	const assistantText = "PONG"
	if err := s1.CreateSession(store.Session{
		ID:    sid,
		Title: store.TitleFromPrompt(userText, 60),
		Model: "demo-model",
	}); err != nil {
		t.Fatal(err)
	}
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are Inferenesia"},
		{Role: provider.RoleUser, Content: userText},
		{Role: provider.RoleAssistant, Content: assistantText},
	}
	if err := s1.ReplaceMessages(sid, msgs); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	// Process 2: restart CLI, list sessions.
	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"sessions"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("sessions exit %d: stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	listOut := out.String()
	if !strings.Contains(listOut, sid) {
		t.Fatalf("sessions list missing id %q; output:\n%s", sid, listOut)
	}
	if !strings.Contains(listOut, "PONG") && !strings.Contains(listOut, "Reply with") {
		// Title is derived from prompt; should appear in list.
		t.Fatalf("sessions list missing title fragment; output:\n%s", listOut)
	}

	// Process 3: show messages — content must match.
	out.Reset()
	errBuf.Reset()
	code = ExecuteWith([]string{"sessions", "show", sid}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("sessions show exit %d: stderr=%q stdout=%q", code, errBuf.String(), out.String())
	}
	show := out.String()
	if !strings.Contains(show, userText) {
		t.Fatalf("show missing user message; output:\n%s", show)
	}
	if !strings.Contains(show, assistantText) {
		t.Fatalf("show missing assistant message; output:\n%s", show)
	}
	if !strings.Contains(show, "session: "+sid) {
		t.Fatalf("show missing session id; output:\n%s", show)
	}
}

func TestSessionsEmptyList(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))

	var out, errBuf bytes.Buffer
	code := ExecuteWith([]string{"sessions"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "no sessions") {
		t.Fatalf("want empty sessions message; got %q", out.String())
	}
}

func TestHelpListsSessions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("YURA_AI_HOME", filepath.Join(tmp, "home"))

	var out, errBuf bytes.Buffer
	if code := ExecuteWith([]string{"--help"}, &out, &errBuf); code != 0 {
		t.Fatalf("help exit %d: %s", code, errBuf.String())
	}
	combined := out.String() + errBuf.String()
	if !strings.Contains(combined, "sessions") {
		t.Fatalf("help missing sessions subcommand; output:\n%s", combined)
	}
}
