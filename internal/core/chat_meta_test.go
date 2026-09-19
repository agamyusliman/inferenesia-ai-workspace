package core_test

import (
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// VAL-CHAT-012: token estimate + meta fields are non-zero when content present.
func TestEstimateTokenUsageFromMessages(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: strings.Repeat("hello world ", 20)},
		{Role: provider.RoleAssistant, Content: strings.Repeat("answer text ", 15)},
	}
	prompt, completion, total := core.EstimateTokenUsageForTest(msgs, msgs[1].Content)
	if prompt <= 0 {
		t.Fatalf("prompt tokens = %d, want > 0", prompt)
	}
	if completion <= 0 {
		t.Fatalf("completion tokens = %d, want > 0", completion)
	}
	if total != prompt+completion {
		t.Fatalf("total=%d prompt=%d completion=%d", total, prompt, completion)
	}
	ev := core.ChatEvent{
		Type:             core.ChatEventDone,
		Final:            "ok",
		Tokens:           total,
		TokensPrompt:     prompt,
		TokensCompletion: completion,
		LatencyMS:        150,
	}
	if ev.Tokens <= 0 || ev.LatencyMS != 150 {
		t.Fatalf("ChatEvent meta fields not set: %+v", ev)
	}
}

// VAL-CHAT-014: tool line split yields name + detail for UI blocks.
func TestSplitToolLineViaWriter(t *testing.T) {
	var got []core.ChatEvent
	w := &core.ToolLineWriterForTest{Emit: func(ev core.ChatEvent) {
		got = append(got, ev)
	}}
	// toolLineWriter is unexported; use the test export below.
	n, err := w.Write([]byte("[tool_start] write_file path=hello.txt\n"))
	if err != nil || n == 0 {
		t.Fatalf("write: n=%d err=%v", n, err)
	}
	n, err = w.Write([]byte("[tool_end] shell echo hi → exit 0\nhi\n"))
	if err != nil || n == 0 {
		t.Fatalf("write end: n=%d err=%v", n, err)
	}
	n, err = w.Write([]byte("[tool_error] write_file sandbox → denied\n"))
	if err != nil || n == 0 {
		t.Fatalf("write err: n=%d err=%v", n, err)
	}
	if len(got) != 3 {
		t.Fatalf("events=%d want 3: %+v", len(got), got)
	}
	if got[0].Type != core.ChatEventToolStart || got[0].Name != "write_file" {
		t.Fatalf("start: %+v", got[0])
	}
	if got[0].Detail == "" || !strings.Contains(got[0].Detail, "hello") {
		t.Fatalf("start detail: %q", got[0].Detail)
	}
	if got[1].Type != core.ChatEventToolEnd || got[1].Name != "shell" {
		t.Fatalf("end: %+v", got[1])
	}
	if got[2].Type != core.ChatEventToolError || got[2].Name != "write_file" {
		t.Fatalf("error: %+v", got[2])
	}
}
