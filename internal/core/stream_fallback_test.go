package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

type mockStreamClient struct {
	streamCh   <-chan provider.StreamEvent
	streamErr  error
	turnMsg    provider.Message
	turnErr    error
	streamCalls int
	turnCalls   int
}

func (m *mockStreamClient) ChatStream(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	m.streamCalls++
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	if m.streamCh != nil {
		return m.streamCh, nil
	}
	ch := make(chan provider.StreamEvent)
	close(ch)
	return ch, nil
}

func (m *mockStreamClient) ChatTurn(ctx context.Context, req provider.ChatRequest) (provider.Message, error) {
	m.turnCalls++
	if m.turnErr != nil {
		return provider.Message{}, m.turnErr
	}
	return m.turnMsg, nil
}

type streamOnlyClient struct {
	streamCh    <-chan provider.StreamEvent
	streamErr   error
	streamCalls int
}

func (m *streamOnlyClient) ChatStream(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	m.streamCalls++
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	if m.streamCh != nil {
		return m.streamCh, nil
	}
	ch := make(chan provider.StreamEvent)
	close(ch)
	return ch, nil
}

func newClosedEmptyStream() <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent)
	close(ch)
	return ch
}

func newDoneOnlyStream() <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: provider.EventDone, Final: ""}
	close(ch)
	return ch
}

func newDeltaStream(deltas ...string) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, len(deltas)+1)
	for _, d := range deltas {
		ch <- provider.StreamEvent{Type: provider.EventDelta, Delta: d}
	}
	ch <- provider.StreamEvent{Type: provider.EventDone, Final: strings.Join(deltas, "")}
	close(ch)
	return ch
}

func newErrorStream(err error) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: provider.EventError, Err: err}
	close(ch)
	return ch
}

func TestEmptyStreamFallsBackToChatTurnContent(t *testing.T) {
	client := &mockStreamClient{
		streamCh: newDoneOnlyStream(),
		turnMsg:  provider.Message{Role: provider.RoleAssistant, Content: "hello"},
	}
	var out strings.Builder
	msg, err := runStreamingTurn(context.Background(), client, provider.ChatRequest{Stream: true}, &out)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if client.turnCalls != 1 {
		t.Fatalf("turnCalls=%d want 1", client.turnCalls)
	}
	if msg.Content != "hello" {
		t.Fatalf("content=%q want hello", msg.Content)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("out=%q missing hello", out.String())
	}
	if msg.Role != provider.RoleAssistant {
		t.Fatalf("role=%q", msg.Role)
	}
}

func TestEmptyStreamFallsBackToChatTurnTools(t *testing.T) {
	var tc provider.ToolCall
	tc.ID = "call_1"
	tc.Type = "function"
	tc.Function.Name = "write_file"
	tc.Function.Arguments = `{"path":"x.txt","content":"y"}`
	client := &mockStreamClient{
		streamCh: newClosedEmptyStream(),
		turnMsg:  provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{tc}},
	}
	var out strings.Builder
	msg, err := runStreamingTurn(context.Background(), client, provider.ChatRequest{Stream: true}, &out)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if client.turnCalls != 1 {
		t.Fatalf("turnCalls=%d want 1", client.turnCalls)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "write_file" {
		t.Fatalf("toolCalls=%+v", msg.ToolCalls)
	}
}

func TestNonEmptyStreamDoesNotRetry(t *testing.T) {
	client := &mockStreamClient{
		streamCh: newDeltaStream("partial", " answer"),
		turnMsg:  provider.Message{Role: provider.RoleAssistant, Content: "should not be used"},
	}
	var out strings.Builder
	msg, err := runStreamingTurn(context.Background(), client, provider.ChatRequest{Stream: true}, &out)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if client.turnCalls != 0 {
		t.Fatalf("turnCalls=%d want 0", client.turnCalls)
	}
	if msg.Content != "partial answer" {
		t.Fatalf("content=%q", msg.Content)
	}
}

func TestStreamErrorNoRetry(t *testing.T) {
	streamErr := errors.New("upstream 502")
	client := &mockStreamClient{
		streamCh: newErrorStream(streamErr),
		turnMsg:  provider.Message{Role: provider.RoleAssistant, Content: "should not be used"},
	}
	var out strings.Builder
	_, err := runStreamingTurn(context.Background(), client, provider.ChatRequest{Stream: true}, &out)
	if err == nil {
		t.Fatal("want err")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("err=%q missing 502", err.Error())
	}
	if client.turnCalls != 0 {
		t.Fatalf("turnCalls=%d want 0", client.turnCalls)
	}
}

func TestCancelNoRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan provider.StreamEvent)
	client := &mockStreamClient{
		streamCh: ch,
		turnMsg:  provider.Message{Role: provider.RoleAssistant, Content: "should not be used"},
	}
	var out strings.Builder
	done := make(chan struct{})
	go func() {
		_, err := runStreamingTurn(ctx, client, provider.ChatRequest{Stream: true}, &out)
		if err == nil {
			t.Error("want err")
		}
		close(done)
	}()
	cancel()
	close(ch)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancel")
	}
	if client.turnCalls != 0 {
		t.Fatalf("turnCalls=%d want 0", client.turnCalls)
	}
}

func TestStreamerWithoutTurnRunnerReturnsEmpty(t *testing.T) {
	only := &streamOnlyClient{
		streamCh: newClosedEmptyStream(),
	}
	var out strings.Builder
	msg, err := runStreamingTurn(context.Background(), only, provider.ChatRequest{Stream: true}, &out)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if msg.Content != "" {
		t.Fatalf("content=%q want empty", msg.Content)
	}
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("toolCalls=%+v want empty", msg.ToolCalls)
	}
}

func TestChatTurnFailsAfterEmptyStream(t *testing.T) {
	turnErr := errors.New("503 service unavailable")
	client := &mockStreamClient{
		streamCh: newClosedEmptyStream(),
		turnErr:  turnErr,
	}
	var out strings.Builder
	_, err := runStreamingTurn(context.Background(), client, provider.ChatRequest{Stream: true}, &out)
	if err == nil {
		t.Fatal("want err")
	}
	if !strings.Contains(err.Error(), "non-stream fallback failed") {
		t.Fatalf("err=%q missing fallback marker", err.Error())
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("err=%q missing 503", err.Error())
	}
	if client.turnCalls != 1 {
		t.Fatalf("turnCalls=%d want 1", client.turnCalls)
	}
}
