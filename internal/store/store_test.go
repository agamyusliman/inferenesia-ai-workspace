package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// VAL-CLI-014: after a chat turn, a new store handle (simulating process restart)
// must list the session and return identical message content.
func TestSessionPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sessions.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	meta := Session{
		ID:       "sess-test-1",
		Title:    "ping pong",
		Model:    "fake-model",
		Profile:  "tempai",
		Workspace: dir,
	}
	if err := s1.CreateSession(meta); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are Inferenesia"},
		{Role: provider.RoleUser, Content: "Reply with the single word: PONG"},
		{Role: provider.RoleAssistant, Content: "PONG"},
	}
	if err := s1.ReplaceMessages(meta.ID, msgs); err != nil {
		t.Fatalf("ReplaceMessages: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close first handle: %v", err)
	}

	// Simulate CLI restart: new process opens the same DB file.
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen Open: %v", err)
	}
	defer s2.Close()

	list, err := s2.ListSessions(0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListSessions len = %d, want 1", len(list))
	}
	if list[0].ID != meta.ID {
		t.Fatalf("listed id = %q, want %q", list[0].ID, meta.ID)
	}
	if list[0].Title != meta.Title {
		t.Fatalf("listed title = %q, want %q", list[0].Title, meta.Title)
	}

	loaded, err := s2.LoadMessages(meta.ID)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(loaded) != len(msgs) {
		t.Fatalf("LoadMessages len = %d, want %d", len(loaded), len(msgs))
	}
	for i := range msgs {
		if loaded[i].Role != msgs[i].Role {
			t.Errorf("msg[%d].Role = %q, want %q", i, loaded[i].Role, msgs[i].Role)
		}
		if loaded[i].Content != msgs[i].Content {
			t.Errorf("msg[%d].Content = %q, want %q", i, loaded[i].Content, msgs[i].Content)
		}
	}
}

func TestAppendMessagesAndToolCallsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id := "sess-tools"
	if err := s.CreateSession(Session{ID: id, Title: "tools"}); err != nil {
		t.Fatal(err)
	}

	var tc provider.ToolCall
	tc.ID = "call_1"
	tc.Type = "function"
	tc.Function.Name = "write_file"
	tc.Function.Arguments = `{"path":"a.txt","content":"x"}`

	initial := []provider.Message{
		{Role: provider.RoleUser, Content: "write a.txt"},
		{Role: provider.RoleAssistant, Content: "", ToolCalls: []provider.ToolCall{tc}},
	}
	if err := s.ReplaceMessages(id, initial); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendMessages(id, []provider.Message{
		{Role: provider.RoleTool, Content: "ok", ToolCallID: "call_1", Name: "write_file"},
		{Role: provider.RoleAssistant, Content: "done"},
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadMessages(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 4 {
		t.Fatalf("len = %d, want 4", len(loaded))
	}
	if len(loaded[1].ToolCalls) != 1 || loaded[1].ToolCalls[0].Function.Name != "write_file" {
		t.Fatalf("tool calls not restored: %+v", loaded[1].ToolCalls)
	}
	if loaded[2].ToolCallID != "call_1" || loaded[2].Name != "write_file" {
		t.Fatalf("tool result not restored: %+v", loaded[2])
	}
}

func TestGetSessionNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_, err = s.GetSession("missing")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestListSessionsOrderNewestFirst(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for _, id := range []string{"a", "b", "c"} {
		if err := s.CreateSession(Session{ID: id, Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	// Bump "a" updated_at by rewriting messages.
	if err := s.ReplaceMessages("a", []provider.Message{
		{Role: provider.RoleUser, Content: "later"},
	}); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListSessions(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d", len(list))
	}
	if list[0].ID != "a" {
		t.Fatalf("newest first want a, got %s", list[0].ID)
	}
}

// TestListSessionsOrderDeterministicCollision is a regression guard for the
// flake fixed by switching from time.RFC3339Nano to timestampFormat and from
// (updated_at DESC, id DESC) to (updated_at DESC, rowid DESC).
//
// Two failure modes were addressed:
//  1. RFC3339Nano trims trailing zeros (".5Z" vs ".500000000Z"), so a 500ms
//     timestamp sorted *newer* than a 500ms+5ns one under lexicographic TEXT
//     comparison in ORDER BY, even though it is chronologically older.
//  2. The id DESC tie-breaker sorted by id string, so when two sessions shared
//     the same updated_at, the alphabetically-larger (but possibly older) id
//     won instead of the more-recently-touched one.
//
// This test forces collisions deterministically with explicit timestamps (no
// reliance on wall-clock resolution), so it must remain stable under
// -count=50 / -race without timing luck.
func TestListSessionsOrderDeterministicCollision(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// base is a whole second; sub-second offsets exercise trailing-zero cases.
	base := time.Date(2026, 7, 19, 2, 15, 0, 0, time.UTC)

	// Create in a non-sorted id order so a naive (updated_at DESC, id DESC)
	// tie-breaker would surface the wrong session first.
	//
	//   id="z"  updated = base + 500ms            -> .500000000Z (trim bug: ".5Z")
	//   id="a"  updated = base + 500ms + 5ns      -> .500000005Z (chronologically later)
	//   id="m"  updated = base + 500ms            -> identical timestamp to "z"
	//
	// Expected newest-first by chronological + insertion tie-break:
	//   1) "a" (500ms+5ns, the latest)
	//   2) "m" (500ms, inserted after "z" → rowid wins on tie)
	//   3) "z" (500ms, inserted first)
	cases := []struct {
		id      string
		updated time.Time
	}{
		{"z", base.Add(500 * time.Millisecond)},
		{"a", base.Add(500*time.Millisecond + 5*time.Nanosecond)},
		{"m", base.Add(500 * time.Millisecond)},
	}
	for _, c := range cases {
		if err := s.CreateSession(Session{
			ID:        c.id,
			Title:     c.id,
			CreatedAt: base,
			UpdatedAt: c.updated,
		}); err != nil {
			t.Fatalf("CreateSession %s: %v", c.id, err)
		}
	}

	list, err := s.ListSessions(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	wantOrder := []string{"a", "m", "z"}
	for i, want := range wantOrder {
		if list[i].ID != want {
			t.Fatalf("list[%d].ID = %q, want %q (full order: %s)",
				i, list[i].ID, want, sessionIDs(list))
		}
	}

	// Bump "z" to the latest timestamp via a fresh session-bump path that
	// honors an explicit timestamp. We cannot set updated_at directly without
	// a setter, so instead re-create a deterministic scenario: create a 4th
	// session "newest" with an explicitly later updated_at and confirm it lands
	// first, proving rowid does not override a strictly-later updated_at.
	if err := s.CreateSession(Session{
		ID:        "newest",
		Title:     "newest",
		CreatedAt: base,
		UpdatedAt: base.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("CreateSession newest: %v", err)
	}
	list, err = s.ListSessions(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("len = %d, want 4", len(list))
	}
	if list[0].ID != "newest" {
		t.Fatalf("newest session must sort first, got %s (full order: %s)",
			list[0].ID, sessionIDs(list))
	}
}

// TestListSessionsTimestampFormatLexicographic is a focused unit-level guard:
// the stored updated_at string must be fixed-width so that two timestamps
// within the same second sort lexicographically the same as chronologically.
// This directly catches the RFC3339Nano trailing-zero regression.
func TestListSessionsTimestampFormatLexicographic(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
	}{
		{"whole_second", time.Date(2026, 7, 19, 2, 15, 0, 0, time.UTC)},
		{"half_second", time.Date(2026, 7, 19, 2, 15, 0, 500_000_000, time.UTC)},
		{"five_ns", time.Date(2026, 7, 19, 2, 15, 0, 500_000_005, time.UTC)},
		{"trailing_zeros", time.Date(2026, 7, 19, 2, 15, 0, 123_000_000, time.UTC)},
		{"one_ns", time.Date(2026, 7, 19, 2, 15, 0, 1, time.UTC)},
	}
	// Build the formatted strings and ensure chronological order == lex order.
	formatted := make([]string, len(cases))
	for i, c := range cases {
		formatted[i] = c.t.UTC().Format(timestampFormat)
		// Guard: must not contain a bare ".5Z"-style truncation; must have 9 digits.
		if !strings.Contains(formatted[i], ".") {
			t.Fatalf("%s: missing fractional seconds in %q", c.name, formatted[i])
		}
		// Fractional part (between '.' and 'Z'/offset) must be exactly 9 digits.
		frac := formatted[i]
		dot := strings.Index(frac, ".")
		if dot < 0 {
			t.Fatalf("%s: no dot in %q", c.name, formatted[i])
		}
		tail := frac[dot+1:]
		// strip trailing 'Z' or +hh:mm offset
		if zi := strings.IndexAny(tail, "Z+"); zi >= 0 {
			tail = tail[:zi]
		}
		if len(tail) != 9 {
			t.Fatalf("%s: fractional part %q has %d digits, want 9 (trailing-zero bug)", c.name, tail, len(tail))
		}
	}
	// Chronological sort must equal lexicographic sort of the formatted strings.
	byTime := append([]time.Time(nil), func() []time.Time {
		out := make([]time.Time, len(cases))
		for i, c := range cases {
			out[i] = c.t
		}
		return out
	}()...)
	// stable sort by time
	for i := 1; i < len(byTime); i++ {
		for j := i; j > 0 && byTime[j].Before(byTime[j-1]); j-- {
			byTime[j], byTime[j-1] = byTime[j-1], byTime[j]
		}
	}
	byLex := append([]string(nil), formatted...)
	for i := 1; i < len(byLex); i++ {
		for j := i; j > 0 && byLex[j] < byLex[j-1]; j-- {
			byLex[j], byLex[j-1] = byLex[j-1], byLex[j]
		}
	}
	// Re-derive lex-sorted times from byLex order mapping and compare with byTime.
	// Map formatted string -> original time via cases (assume unique).
	strToTime := make(map[string]time.Time, len(cases))
	for _, c := range cases {
		strToTime[c.t.UTC().Format(timestampFormat)] = c.t
	}
	for i, fs := range byLex {
		if !strToTime[fs].Equal(byTime[i]) {
			t.Fatalf("lex order != chronological order at index %d: lex=%v (%s) chrono=%v",
				i, strToTime[fs], fs, byTime[i])
		}
	}
}

func sessionIDs(list []Session) string {
	ids := make([]string, 0, len(list))
	for _, s := range list {
		ids = append(ids, s.ID)
	}
	return strings.Join(ids, ",")
}

func TestOpenCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "store", "sessions.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open nested: %v", err)
	}
	defer s.Close()
}
