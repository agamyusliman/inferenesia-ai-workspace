package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
	"unicode/utf8"
)

// NewSessionID returns a unique session id suitable for CLI listing/resume
// (timestamp + random suffix, e.g. 20260717-abcdef12).
func NewSessionID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to pure timestamp nanoseconds.
		return fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102-150405"), time.Now().UnixNano()%1e6)
	}
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(b[:]))
}

// TitleFromPrompt builds a short session title from the first user prompt.
func TitleFromPrompt(prompt string, max int) string {
	if max <= 0 {
		max = 60
	}
	// Collapse whitespace. Range over the string directly (avoids S1029/SA6003):
	// ranging over a string already yields runes, so []rune(prompt) is unnecessary.
	var out []rune
	space := false
	for _, r := range prompt {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if len(out) > 0 {
				space = true
			}
			continue
		}
		if space {
			out = append(out, ' ')
			space = false
		}
		out = append(out, r)
		if len(out) >= max {
			break
		}
	}
	s := string(out)
	if utf8.RuneCountInString(prompt) > max {
		if len(out) >= 3 {
			s = string(out[:max-1]) + "…"
		}
	}
	if s == "" {
		return "untitled"
	}
	return s
}
