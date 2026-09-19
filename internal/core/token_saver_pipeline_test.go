package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestCavemanFragmentOnlyWhenOn(t *testing.T) {
	base := "You are Inferenesia."
	off := config.DefaultTokenSavers()
	got := ApplyTokenSaverSystemFragments(base, off)
	if strings.Contains(got, "Caveman") || strings.Contains(got, CavemanSystemFragment) {
		t.Fatalf("caveman should be absent when off:\n%s", got)
	}
	if !strings.Contains(got, base) {
		t.Fatalf("base missing: %q", got)
	}

	on := off
	on.Caveman.Enabled = true
	got = ApplyTokenSaverSystemFragments(base, on)
	if !strings.Contains(got, CavemanSystemFragment) {
		t.Fatalf("caveman fragment missing when on:\n%s", got)
	}
	if !strings.Contains(got, "terse") {
		t.Fatalf("expected terse guidance:\n%s", got)
	}
}

func TestPonytailFragmentOnlyWhenOn(t *testing.T) {
	base := "You are Inferenesia."
	off := config.DefaultTokenSavers()
	got := ApplyTokenSaverSystemFragments(base, off)
	if strings.Contains(got, "Ponytail") || strings.Contains(got, "YAGNI") {
		t.Fatalf("ponytail should be absent when off:\n%s", got)
	}

	on := off
	on.Ponytail.Enabled = true
	got = ApplyTokenSaverSystemFragments(base, on)
	if !strings.Contains(got, PonytailSystemFragment) {
		t.Fatalf("ponytail fragment missing when on:\n%s", got)
	}
	if !strings.Contains(got, "YAGNI") || !strings.Contains(got, "deletion") {
		t.Fatalf("expected YAGNI/deletion bias:\n%s", got)
	}
}

func TestBothFragmentsIndependent(t *testing.T) {
	s := config.DefaultTokenSavers()
	s.Caveman.Enabled = true
	s.Ponytail.Enabled = true
	got := ApplyTokenSaverSystemFragments("base", s)
	if !strings.Contains(got, CavemanSystemFragment) || !strings.Contains(got, PonytailSystemFragment) {
		t.Fatalf("both should be present:\n%s", got)
	}
}

func TestExistingSystemRemovesDisabledSaverFragments(t *testing.T) {
	messages := []provider.Message{{Role: provider.RoleSystem, Content: "Base rules\n\n" + CavemanSystemFragment + "\n\n" + PonytailSystemFragment}}
	var savers config.TokenSaversConfig
	savers.Ponytail.Enabled = true
	got := appendSaverFragmentsToExistingSystem(messages, savers)
	if strings.Contains(got[0].Content, "[Token Saver: Caveman]") {
		t.Fatal("disabled Caveman fragment survived")
	}
	if !strings.Contains(got[0].Content, "[Token Saver: Ponytail]") {
		t.Fatal("enabled Ponytail fragment missing")
	}
	if !strings.Contains(got[0].Content, "Base rules") {
		t.Fatal("base system prompt was lost")
	}
}

func TestRTKOffUsesFullOutput(t *testing.T) {
	s := config.DefaultTokenSavers()
	raw := strings.Repeat("line of tool output\n", 100)
	var called int32
	out := CompressToolResultRTK(s, "shell", raw, func(name, content string) (string, error) {
		atomic.AddInt32(&called, 1)
		return "compressed", nil
	})
	if out != raw {
		t.Fatalf("off should pass full output")
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatal("injector must not run when RTK off")
	}
}

func TestRTKOnCompressesWithMock(t *testing.T) {
	s := config.DefaultTokenSavers()
	s.RTK.Enabled = true
	raw := strings.Repeat("git status noise\n", 50)
	out := CompressToolResultRTK(s, "shell", raw, func(name, content string) (string, error) {
		if name != "shell" {
			t.Fatalf("tool name %q", name)
		}
		if !strings.Contains(content, "git status") {
			t.Fatal("expected tool content")
		}
		return "M path/a.go\n[rtk: mock compressed]", nil
	})
	if out == raw {
		t.Fatal("expected compressed mock output")
	}
	if !strings.Contains(out, "rtk: mock") {
		t.Fatalf("got %q", out)
	}
}

func TestRTKUnavailablePassThrough(t *testing.T) {
	s := config.DefaultTokenSavers()
	s.RTK.Enabled = true
	s.RTK.Command = "/definitely/missing/rtk-binary-xyz"
	raw := "full tool output body"
	// No inject → probe fails → full output (VAL-SAVER-003/007).
	out := CompressToolResultRTK(s, "grep", raw, nil)
	if out != raw {
		t.Fatalf("missing RTK must pass through, got %q", out)
	}
}

func TestRTKInjectErrorPassThrough(t *testing.T) {
	s := config.DefaultTokenSavers()
	s.RTK.Enabled = true
	raw := "keep-me"
	out := CompressToolResultRTK(s, "list_dir", raw, func(name, content string) (string, error) {
		return "", context.Canceled
	})
	if out != raw {
		t.Fatalf("error must pass through, got %q", out)
	}
}

func TestHeadroomOffSkips(t *testing.T) {
	s := config.DefaultTokenSavers()
	msgs := []provider.Message{{Role: provider.RoleUser, Content: "hello"}}
	var called int32
	out := CompressMessagesHeadroom(context.Background(), s, msgs, func(ctx context.Context, m []provider.Message) ([]provider.Message, error) {
		atomic.AddInt32(&called, 1)
		return []provider.Message{{Role: provider.RoleUser, Content: "compressed"}}, nil
	}, nil)
	if atomic.LoadInt32(&called) != 0 {
		t.Fatal("headroom inject must not run when off")
	}
	if out[0].Content != "hello" {
		t.Fatalf("got %+v", out)
	}
}

func TestHeadroomOnCallsCompress(t *testing.T) {
	s := config.DefaultTokenSavers()
	s.Headroom.Enabled = true
	s.Headroom.BaseURL = "http://example.invalid" // inject overrides HTTP
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "sys"},
		{Role: provider.RoleUser, Content: "long prompt " + strings.Repeat("x", 100)},
	}
	var called int32
	out := CompressMessagesHeadroom(context.Background(), s, msgs, func(ctx context.Context, m []provider.Message) ([]provider.Message, error) {
		atomic.AddInt32(&called, 1)
		return []provider.Message{
			{Role: provider.RoleSystem, Content: "sys"},
			{Role: provider.RoleUser, Content: "short"},
		}, nil
	}, nil)
	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("compress calls = %d", called)
	}
	if out[1].Content != "short" {
		t.Fatalf("got %+v", out)
	}
}

func TestHeadroomMissingURLPassThrough(t *testing.T) {
	s := config.DefaultTokenSavers()
	s.Headroom.Enabled = true
	// empty base_url → not available
	msgs := []provider.Message{{Role: provider.RoleUser, Content: "keep"}}
	out := CompressMessagesHeadroom(context.Background(), s, msgs, nil, nil)
	if out[0].Content != "keep" {
		t.Fatalf("got %+v", out)
	}
}

func TestHeadroomHTTPCompressHook(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/compress" {
			t.Errorf("path %s", r.URL.Path)
		}
		var body headroomCompressRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(headroomCompressResponse{
			Messages: []provider.Message{{Role: provider.RoleUser, Content: "ok-compressed"}},
		})
	}))
	t.Cleanup(srv.Close)

	s := config.DefaultTokenSavers()
	s.Headroom.Enabled = true
	s.Headroom.BaseURL = srv.URL
	out := CompressMessagesHeadroom(context.Background(), s, []provider.Message{
		{Role: provider.RoleUser, Content: "verbose prompt"},
	}, nil, srv.Client())
	if len(out) != 1 || out[0].Content != "ok-compressed" {
		t.Fatalf("got %+v", out)
	}
	// Never attach provider API keys to Headroom (VAL-SAVER-008).
	if gotAuth != "" {
		t.Fatalf("unexpected Authorization header %q", gotAuth)
	}
}

func TestHeadroomHTTPFailurePassThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	s := config.DefaultTokenSavers()
	s.Headroom.Enabled = true
	s.Headroom.BaseURL = srv.URL
	msgs := []provider.Message{{Role: provider.RoleUser, Content: "original"}}
	out := CompressMessagesHeadroom(context.Background(), s, msgs, nil, srv.Client())
	if out[0].Content != "original" {
		t.Fatalf("got %+v", out)
	}
}

func TestSaversNeverLogSecretsInRedactPath(t *testing.T) {
	secret := "sk-test-secret-value-abc12345"
	s := config.DefaultTokenSavers()
	s.RTK.Enabled = true
	var seen string
	_ = CompressToolResultRTK(s, "shell", "export KEY="+secret, func(name, content string) (string, error) {
		seen = content
		return "ok", nil
	})
	if strings.Contains(seen, secret) {
		t.Fatalf("secret leaked into RTK inject input: %q", seen)
	}
	if strings.Contains(tokenSaverSafeDetail("Bearer "+secret), secret) {
		t.Fatal("safe detail must redact")
	}
}

func TestAgentLoopAppliesSavers(t *testing.T) {
	// Full agent loop: RTK on tool results + Headroom on pre-model + Caveman/Ponytail in system.
	savers := config.DefaultTokenSavers()
	savers.RTK.Enabled = true
	savers.Headroom.Enabled = true
	savers.Caveman.Enabled = true
	savers.Ponytail.Enabled = true

	var (
		gotSystem   string
		toolContent string
		headroomN   int32
	)

	// Turn 1: request write_file; turn 2: final after seeing tool result.
	var writeTC provider.ToolCall
	writeTC.ID = "c1"
	writeTC.Type = "function"
	writeTC.Function.Name = tools.NameWriteFile
	writeTC.Function.Arguments = `{"path":"hello.txt","content":"hi from agent"}`

	client := &saverCapturingClient{
		turns: []provider.Message{
			{
				Role:      provider.RoleAssistant,
				Content:   "",
				ToolCalls: []provider.ToolCall{writeTC},
			},
			{
				Role:    provider.RoleAssistant,
				Content: "done",
			},
		},
		onRequest: func(req provider.ChatRequest) {
			if len(req.Messages) > 0 && req.Messages[0].Role == provider.RoleSystem {
				gotSystem = req.Messages[0].Content
			}
			for _, m := range req.Messages {
				if m.Role == provider.RoleTool {
					toolContent = m.Content
				}
			}
		},
	}

	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	result, err := Run(context.Background(), AgentConfig{
		Client:       client,
		Tools:        reg,
		Model:        "test-model",
		SystemPrompt: "You are Inferenesia.",
		Messages:     []provider.Message{{Role: provider.RoleUser, Content: "create hello.txt"}},
		Stream:       false,
		TokenSavers:  savers,
		RTKCompress: func(name, content string) (string, error) {
			return "[rtk mock] compressed-tool-out", nil
		},
		HeadroomCompress: func(ctx context.Context, messages []provider.Message) ([]provider.Message, error) {
			atomic.AddInt32(&headroomN, 1)
			return messages, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ToolRounds < 1 {
		t.Fatalf("expected tool round, got %+v", result)
	}
	if !strings.Contains(gotSystem, CavemanSystemFragment) {
		t.Fatalf("system missing caveman:\n%s", gotSystem)
	}
	if !strings.Contains(gotSystem, PonytailSystemFragment) {
		t.Fatalf("system missing ponytail:\n%s", gotSystem)
	}
	if atomic.LoadInt32(&headroomN) < 1 {
		t.Fatal("headroom compress should run at least once before model")
	}
	if !strings.Contains(toolContent, "[rtk mock]") {
		t.Fatalf("tool message should be RTK-compressed, got %q", toolContent)
	}
	// File still written with full content (RTK only affects model-bound tool message).
	data, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hi from agent" {
		t.Fatalf("file content %q", data)
	}

	// Off path: fragments must not appear with defaults.
	gotSystem = ""
	client2 := &saverCapturingClient{
		turns: []provider.Message{{Role: provider.RoleAssistant, Content: "hi"}},
		onRequest: func(req provider.ChatRequest) {
			if len(req.Messages) > 0 && req.Messages[0].Role == provider.RoleSystem {
				gotSystem = req.Messages[0].Content
			}
		},
	}
	_, err = Run(context.Background(), AgentConfig{
		Client:       client2,
		Model:        "test-model",
		SystemPrompt: "You are Inferenesia.",
		Messages:     []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
		Stream:       false,
		TokenSavers:  config.DefaultTokenSavers(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotSystem, "Caveman") || strings.Contains(gotSystem, "Ponytail") {
		t.Fatalf("fragments must be off by default:\n%s", gotSystem)
	}
}

func TestAgentLoopMissingSaversNeverCrash(t *testing.T) {
	// Enable all savers with missing RTK/Headroom — chat/loop must still complete.
	s := config.DefaultTokenSavers()
	s.RTK.Enabled = true
	s.RTK.Command = "/no/such/rtk"
	s.Headroom.Enabled = true
	// empty base_url
	s.Caveman.Enabled = true
	s.Ponytail.Enabled = true

	var writeTC provider.ToolCall
	writeTC.ID = "c1"
	writeTC.Type = "function"
	writeTC.Function.Name = tools.NameWriteFile
	writeTC.Function.Arguments = `{"path":"x.txt","content":"y"}`

	client := &saverCapturingClient{
		turns: []provider.Message{
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{writeTC}},
			{Role: provider.RoleAssistant, Content: "ok"},
		},
	}
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	res, err := Run(context.Background(), AgentConfig{
		Client:       client,
		Tools:        reg,
		Model:        "m",
		SystemPrompt: "sys",
		Messages:     []provider.Message{{Role: provider.RoleUser, Content: "write"}},
		Stream:       false,
		TokenSavers:  s,
	})
	if err != nil {
		t.Fatalf("must not crash: %v", err)
	}
	if res.ToolRounds != 1 {
		t.Fatalf("tool rounds %d", res.ToolRounds)
	}
	// Full tool result present (no RTK) somewhere in messages.
	foundTool := false
	for _, m := range res.Messages {
		if m.Role == provider.RoleTool {
			foundTool = true
			if m.Content == "" {
				t.Fatal("empty tool content")
			}
		}
	}
	if !foundTool {
		t.Fatal("expected tool message")
	}
}

// saverCapturingClient implements Streamer + TurnRunner for agent loop unit tests.
type saverCapturingClient struct {
	turns     []provider.Message
	idx       int
	onRequest func(provider.ChatRequest)
}

func (c *saverCapturingClient) ChatStream(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	msg, err := c.ChatTurn(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan provider.StreamEvent, 2)
	go func() {
		defer close(ch)
		if msg.Content != "" {
			ch <- provider.StreamEvent{Type: provider.EventDelta, Delta: msg.Content}
		}
		if len(msg.ToolCalls) > 0 {
			ch <- provider.StreamEvent{Type: provider.EventToolCalls, ToolCalls: msg.ToolCalls}
		}
		ch <- provider.StreamEvent{Type: provider.EventDone, Final: msg.Content, ToolCalls: msg.ToolCalls}
	}()
	return ch, nil
}

func (c *saverCapturingClient) ChatTurn(ctx context.Context, req provider.ChatRequest) (provider.Message, error) {
	if c.onRequest != nil {
		c.onRequest(req)
	}
	if c.idx >= len(c.turns) {
		return provider.Message{Role: provider.RoleAssistant, Content: ""}, nil
	}
	m := c.turns[c.idx]
	c.idx++
	return m, nil
}
