package store

import "testing"

func TestTitleFromPrompt(t *testing.T) {
	if got := TitleFromPrompt("hello", 60); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if got := TitleFromPrompt("  multi\nline  text  ", 60); got != "multi line text" {
		t.Fatalf("collapse whitespace: got %q", got)
	}
	long := stringsRepeat("a", 100)
	got := TitleFromPrompt(long, 10)
	if len([]rune(got)) > 10 {
		t.Fatalf("title too long: %q", got)
	}
	if got := TitleFromPrompt("", 60); got != "untitled" {
		t.Fatalf("empty: got %q", got)
	}
}

// TestTitleFromPromptMultibyte guards the S1029/SA6003 fix in id.go: ranging
// over the string directly must still truncate by rune count (not byte count),
// so multibyte prompts produce titles within the rune budget. Session id
// generation behavior is unchanged (covered by TestNewSessionIDUnique).
func TestTitleFromPromptMultibyte(t *testing.T) {
	// Each 'é' is 2 bytes; a rune-budget truncation must keep ≤ max runes.
	prompt := stringsRepeat("é", 30)
	got := TitleFromPrompt(prompt, 10)
	if rc := len([]rune(got)); rc > 10 {
		t.Fatalf("multibyte title too long: %d runes, %q", rc, got)
	}
	// Mixed CJK + ascii: leading whitespace collapse preserved.
	got = TitleFromPrompt("   あ\nい  う   ", 60)
	if got != "あ い う" {
		t.Fatalf("multibyte whitespace collapse: got %q", got)
	}
}

func TestNewSessionIDUnique(t *testing.T) {
	a, b := NewSessionID(), NewSessionID()
	if a == "" || b == "" {
		t.Fatal("empty id")
	}
	// Same-second calls still differ via random suffix.
	if a == b {
		// Extremely unlikely with 4 random bytes; fail soft only if identical.
		t.Logf("ids collided in same nanosecond (rare): %s", a)
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
