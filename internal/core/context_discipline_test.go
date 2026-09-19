package core

import (
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

func u(content string) provider.Message   { return provider.Message{Role: provider.RoleUser, Content: content} }
func a(content string) provider.Message   { return provider.Message{Role: provider.RoleAssistant, Content: content} }
func sys(content string) provider.Message { return provider.Message{Role: provider.RoleSystem, Content: content} }
func tool(id, content string) provider.Message {
	return provider.Message{Role: provider.RoleTool, Content: content, ToolCallID: id}
}
func aWithTools(content string, ids ...string) provider.Message {
	m := provider.Message{Role: provider.RoleAssistant, Content: content}
	for _, id := range ids {
		tc := provider.ToolCall{ID: id, Type: "function"}
		tc.Function.Name = "some_tool"
		m.ToolCalls = append(m.ToolCalls, tc)
	}
	return m
}

func TestApplyContextDiscipline_PreservesSystemAndRecent(t *testing.T) {
	msgs := []provider.Message{
		sys("system prompt"),
		u("old question 1"),
		a("old answer 1"),
		u("old question 2"),
		a("old answer 2"),
		u("recent question"),
		a("recent answer"),
	}
	opts := DisciplineOptions{MaxPairs: 1, MaxTotalRunes: 10000, MaxUserRunes: 1000, MaxAssistantRunes: 1000, MaxToolRunes: 1000, MaxSystemRunes: 5000}
	out := ApplyContextDiscipline(msgs, opts)
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if out[0].Role != provider.RoleSystem {
		t.Fatalf("expected first message to be system, got %s", out[0].Role)
	}
	if out[0].Content != "system prompt" {
		t.Fatalf("system content not preserved: %q", out[0].Content)
	}
	// With MaxPairs=1, only the last user/assistant pair should remain.
	lastUserSeen := false
	for _, m := range out[1:] {
		if m.Role == provider.RoleUser {
			if lastUserSeen {
				t.Fatalf("expected at most 1 user message, got extra: %q", m.Content)
			}
			lastUserSeen = true
			if m.Content != "recent question" {
				t.Fatalf("expected recent question kept, got %q", m.Content)
			}
		}
	}
	if !lastUserSeen {
		t.Fatal("expected the recent user message to be kept")
	}
}

func TestApplyContextDiscipline_PreservesToolCallChain(t *testing.T) {
	msgs := []provider.Message{
		sys("s"),
		u("q1"),
		aWithTools("calling tool", "call_1"),
		tool("call_1", "tool result 1"),
		aWithTools("calling another", "call_2"),
		tool("call_2", "tool result 2"),
		a("final answer 1"),
		u("q2"),
		a("final answer 2"),
	}
	opts := DisciplineOptions{MaxPairs: 1, MaxTotalRunes: 100000, MaxUserRunes: 10000, MaxAssistantRunes: 10000, MaxToolRunes: 10000, MaxSystemRunes: 50000}
	out := ApplyContextDiscipline(msgs, opts)

	// Verify no orphan tool messages: every tool message must have a parent
	// assistant tool_call in the output.
	parentIDs := map[string]bool{}
	for _, m := range out {
		if m.Role == provider.RoleAssistant {
			for _, tc := range m.ToolCalls {
				parentIDs[tc.ID] = true
			}
		}
	}
	for _, m := range out {
		if m.Role == provider.RoleTool {
			if !parentIDs[m.ToolCallID] {
				t.Fatalf("orphan tool message %q (id=%s) without parent assistant tool_call", m.Content, m.ToolCallID)
			}
		}
	}
}

func TestApplyContextDiscipline_DropsOrphanToolAtHead(t *testing.T) {
	// If the pair window cut drops the assistant with tool_calls, the trailing
	// tool messages for those IDs must also be dropped (no orphan).
	msgs := []provider.Message{
		sys("s"),
		u("old q"),
		aWithTools("old call", "old_call_1"),
		tool("old_call_1", "old tool result"),
		a("old final"),
		u("new q"),
		a("new final"),
	}
	opts := DisciplineOptions{MaxPairs: 1, MaxTotalRunes: 100000, MaxUserRunes: 10000, MaxAssistantRunes: 10000, MaxToolRunes: 10000, MaxSystemRunes: 50000}
	out := ApplyContextDiscipline(msgs, opts)
	if len(out) == 0 {
		t.Fatal("expected output")
	}
	for _, m := range out {
		if m.Role == provider.RoleTool && m.ToolCallID == "old_call_1" {
			t.Fatalf("orphan tool message for dropped assistant should not be in output: %q", m.Content)
		}
	}
}

func TestApplyContextDiscipline_StripsBloat(t *testing.T) {
	liveBlock := "```live-block\n" + strings.Repeat("x", 500) + "\n```"
	htmlPreview := "```html:preview\n<div>" + strings.Repeat("y", 500) + "</div>\n```"
	bigBase64 := "data:image/png;base64," + strings.Repeat("A", 600)
	msgs := []provider.Message{
		sys("s"),
		u("q with bloat: " + liveBlock + " " + htmlPreview + " " + bigBase64),
		a("answer"),
	}
	out := ApplyContextDiscipline(msgs, DisciplineOptions{MaxPairs: 5, MaxTotalRunes: 100000, MaxUserRunes: 100000, MaxAssistantRunes: 10000, MaxToolRunes: 10000, MaxSystemRunes: 50000})
	if len(out) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(out))
	}
	body := out[1].Content
	if strings.Contains(body, "live-block") && strings.Contains(body, strings.Repeat("x", 100)) {
		t.Errorf("live-block body not stripped: %q", body)
	}
	if strings.Contains(body, "html:preview") && strings.Contains(body, strings.Repeat("y", 100)) {
		t.Errorf("html:preview body not stripped: %q", body)
	}
	if strings.Contains(body, strings.Repeat("A", 100)) {
		t.Errorf("big base64 not stripped: %q", body)
	}
	if !strings.Contains(body, "[stripped") {
		t.Errorf("expected stripped placeholder, got: %q", body)
	}
}

func TestApplyContextDiscipline_PerMessageCap(t *testing.T) {
	long := strings.Repeat("word ", 5000) // ~25k runes
	msgs := []provider.Message{
		sys("s"),
		u(long),
		a(long),
	}
	opts := DisciplineOptions{MaxPairs: 5, MaxTotalRunes: 100000, MaxUserRunes: 4000, MaxAssistantRunes: 8000, MaxToolRunes: 6000, MaxSystemRunes: 50000}
	out := ApplyContextDiscipline(msgs, opts)
	for i, m := range out {
		switch m.Role {
		case provider.RoleUser:
			if len([]rune(m.Content)) > opts.MaxUserRunes+50 {
				t.Errorf("user msg %d not capped: %d runes", i, len([]rune(m.Content)))
			}
		case provider.RoleAssistant:
			if len([]rune(m.Content)) > opts.MaxAssistantRunes+50 {
				t.Errorf("assistant msg %d not capped: %d runes", i, len([]rune(m.Content)))
			}
		}
	}
}

func TestApplyContextDiscipline_TotalBudgetDrops(t *testing.T) {
	msgs := []provider.Message{
		sys("s"),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, u(strings.Repeat("u", 10000)))
		msgs = append(msgs, a(strings.Repeat("a", 10000)))
	}
	opts := DisciplineOptions{MaxPairs: 20, MaxTotalRunes: 30000, MaxUserRunes: 100000, MaxAssistantRunes: 100000, MaxToolRunes: 10000, MaxSystemRunes: 50000}
	out := ApplyContextDiscipline(msgs, opts)
	total := 0
	for _, m := range out {
		if m.Role == provider.RoleSystem {
			continue
		}
		total += len([]rune(m.Content))
	}
	if total > opts.MaxTotalRunes+1000 {
		t.Errorf("total non-system runes %d exceeds budget %d", total, opts.MaxTotalRunes)
	}
}

func TestApplyContextDiscipline_DoesNotMutateInput(t *testing.T) {
	msgs := []provider.Message{
		sys("s"),
		u("original question"),
		a("original answer"),
	}
	original := msgs[1].Content
	_ = ApplyContextDiscipline(msgs, DisciplineOptions{MaxPairs: 1, MaxTotalRunes: 10000, MaxUserRunes: 100, MaxAssistantRunes: 100, MaxToolRunes: 100, MaxSystemRunes: 5000})
	if msgs[1].Content != original {
		t.Errorf("input mutated: %q != %q", msgs[1].Content, original)
	}
}

func TestApplyContextDiscipline_SystemCapped(t *testing.T) {
	bigSys := strings.Repeat("s", 20000)
	msgs := []provider.Message{
		sys(bigSys),
		u("q"),
		a("a"),
	}
	opts := DisciplineOptions{MaxPairs: 5, MaxTotalRunes: 100000, MaxUserRunes: 10000, MaxAssistantRunes: 10000, MaxToolRunes: 10000, MaxSystemRunes: 12000}
	out := ApplyContextDiscipline(msgs, opts)
	if len(out) == 0 {
		t.Fatal("expected output")
	}
	if len([]rune(out[0].Content)) > opts.MaxSystemRunes+50 {
		t.Errorf("system not capped: %d runes", len([]rune(out[0].Content)))
	}
	if !strings.Contains(out[0].Content, "[truncated]") {
		t.Errorf("expected truncated marker on system, got: %q", out[0].Content[:50])
	}
}

func TestStripBloat_PlainTextUnchanged(t *testing.T) {
	in := "just a normal message with code\n```go\nfmt.Println()\n```"
	out := StripBloat(in)
	if out != in {
		t.Errorf("plain text changed: %q != %q", out, in)
	}
}

func TestStripBloat_ShortDataURLOk(t *testing.T) {
	in := "icon: data:image/png;base64,iVBORw0KGgo="
	out := StripBloat(in)
	if out != in {
		t.Errorf("short data url changed: %q != %q", out, in)
	}
}

func TestCapRunes_WhitespaceBoundary(t *testing.T) {
	s := strings.Repeat("a", 50) + " " + strings.Repeat("b", 50)
	out := capRunes(s, 60)
	if !strings.HasSuffix(out, "…[truncated]") {
		t.Errorf("expected truncated marker, got: %q", out)
	}
}

func TestApplyContextDiscipline_Empty(t *testing.T) {
	if out := ApplyContextDiscipline(nil, DefaultContextDiscipline()); out != nil {
		t.Errorf("expected nil for empty input, got %v", out)
	}
}
