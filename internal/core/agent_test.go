package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// fakeClient drives a scripted multi-turn agent conversation without the network.
type fakeClient struct {
	mu    sync.Mutex
	steps []fakeStep
	idx   int
}

type fakeStep struct {
	// stream events for this turn (when useStream path)
	events []provider.StreamEvent
	// message for ChatTurn path
	msg provider.Message
}

func (f *fakeClient) ChatStream(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.idx >= len(f.steps) {
		ch := make(chan provider.StreamEvent)
		close(ch)
		return ch, nil
	}
	step := f.steps[f.idx]
	f.idx++
	ch := make(chan provider.StreamEvent, len(step.events)+1)
	go func() {
		defer close(ch)
		for _, ev := range step.events {
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
			}
		}
	}()
	return ch, nil
}

func (f *fakeClient) ChatTurn(ctx context.Context, req provider.ChatRequest) (provider.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.idx >= len(f.steps) {
		return provider.Message{Role: provider.RoleAssistant, Content: ""}, nil
	}
	step := f.steps[f.idx]
	f.idx++
	return step.msg, nil
}

func TestAgentWriteFileToolLoop(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	// Turn 1: model requests write_file; turn 2: final answer.
	var writeTC provider.ToolCall
	writeTC.ID = "call_w1"
	writeTC.Type = "function"
	writeTC.Function.Name = tools.NameWriteFile
	writeTC.Function.Arguments = `{"path":"hello.txt","content":"hi"}`

	client := &fakeClient{
		steps: []fakeStep{
			{msg: provider.Message{Role: provider.RoleAssistant, Content: "", ToolCalls: []provider.ToolCall{writeTC}}},
			{msg: provider.Message{Role: provider.RoleAssistant, Content: "Created hello.txt with hi"}},
		},
	}

	var eventBuf strings.Builder
	var outBuf strings.Builder
	res, err := Run(context.Background(), AgentConfig{
		Client: client,
		Tools:  reg,
		Model:  "fake-model",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "create hello.txt containing hi"},
		},
		SystemPrompt: DefaultSystemPrompt,
		Stream:       false,
		Out:          &outBuf,
		EventOut:     &eventBuf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ToolRounds != 1 {
		t.Fatalf("ToolRounds=%d", res.ToolRounds)
	}
	data, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hi" {
		t.Fatalf("file content=%q", data)
	}
	if !strings.Contains(eventBuf.String(), "tool_start") || !strings.Contains(eventBuf.String(), "write_file") {
		t.Fatalf("events missing write: %q", eventBuf.String())
	}
	if !strings.Contains(outBuf.String(), "hello.txt") && !strings.Contains(res.Final, "hello.txt") {
		// Final may say "Created hello.txt"
		if !strings.Contains(res.Final, "Created") {
			t.Fatalf("final=%q out=%q", res.Final, outBuf.String())
		}
	}
}

func TestAgentShellToolLoop(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	var shellTC provider.ToolCall
	shellTC.ID = "call_s1"
	shellTC.Type = "function"
	shellTC.Function.Name = tools.NameShell
	shellTC.Function.Arguments = `{"command":"echo tool-ok"}`

	client := &fakeClient{
		steps: []fakeStep{
			{msg: provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{shellTC}}},
			{msg: provider.Message{Role: provider.RoleAssistant, Content: "The command printed tool-ok"}},
		},
	}

	res, err := Run(context.Background(), AgentConfig{
		Client: client,
		Tools:  reg,
		Model:  "fake-model",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "run echo tool-ok"},
		},
		Stream: false,
		Out:    ioDiscard{},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Final, "tool-ok") {
		t.Fatalf("final=%q", res.Final)
	}
	// Ensure a tool message with stdout was injected.
	foundTool := false
	for _, m := range res.Messages {
		if m.Role == provider.RoleTool && strings.Contains(m.Content, "tool-ok") {
			foundTool = true
		}
	}
	if !foundTool {
		t.Fatalf("tool result missing in messages: %+v", res.Messages)
	}
}

func TestAgentSandboxDenialSurfacesToModel(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	outside := filepath.Join(os.TempDir(), "yura-core-outside-"+filepath.Base(root), "x.txt")
	var writeTC provider.ToolCall
	writeTC.ID = "call_bad"
	writeTC.Type = "function"
	writeTC.Function.Name = tools.NameWriteFile
	writeTC.Function.Arguments = `{"path":"` + filepath.ToSlash(outside) + `","content":"nope"}`

	client := &fakeClient{
		steps: []fakeStep{
			{msg: provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{writeTC}}},
			{msg: provider.Message{Role: provider.RoleAssistant, Content: "I could not write outside the workspace: sandbox denied"}},
		},
	}

	var eventBuf strings.Builder
	res, err := Run(context.Background(), AgentConfig{
		Client: client,
		Tools:  reg,
		Model:  "fake-model",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "write outside workspace"},
		},
		Stream:   false,
		Out:      ioDiscard{},
		EventOut: &eventBuf,
	})
	if err != nil {
		t.Fatalf("Run should not crash on sandbox denial: %v", err)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("outside file must not exist")
	}
	if !strings.Contains(eventBuf.String(), "tool_error") {
		t.Fatalf("expected tool_error event, got %q", eventBuf.String())
	}
	// Tool result message should mention sandbox.
	found := false
	for _, m := range res.Messages {
		if m.Role == provider.RoleTool && strings.Contains(strings.ToLower(m.Content), "sandbox") {
			found = true
		}
	}
	if !found {
		t.Fatalf("tool message missing sandbox denial: %+v", res.Messages)
	}
}

func TestAgentStreamingToolCalls(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	var writeTC provider.ToolCall
	writeTC.ID = "call_stream"
	writeTC.Type = "function"
	writeTC.Function.Name = tools.NameWriteFile
	writeTC.Function.Arguments = `{"path":"s.txt","content":"streamed"}`

	client := &fakeClient{
		steps: []fakeStep{
			{
				events: []provider.StreamEvent{
					{Type: provider.EventToolCalls, ToolCalls: []provider.ToolCall{writeTC}, FinishReason: "tool_calls"},
					{Type: provider.EventDone, ToolCalls: []provider.ToolCall{writeTC}, FinishReason: "tool_calls"},
				},
			},
			{
				events: []provider.StreamEvent{
					{Type: provider.EventDelta, Delta: "done"},
					{Type: provider.EventDone, Final: "done"},
				},
			},
		},
	}

	var out strings.Builder
	res, err := Run(context.Background(), AgentConfig{
		Client: client,
		Tools:  reg,
		Model:  "fake-model",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "write s.txt"},
		},
		Stream: true,
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "s.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "streamed" {
		t.Fatalf("content=%q", data)
	}
	if !strings.Contains(out.String(), "done") {
		t.Fatalf("out=%q", out.String())
	}
	if res.ToolRounds != 1 {
		t.Fatalf("ToolRounds=%d", res.ToolRounds)
	}
}

// ioDiscard is a tiny io.Writer for tests without importing io just for Discard in type position.
type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
