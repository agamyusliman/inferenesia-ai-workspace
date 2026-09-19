package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-MCP-006: stopping/removing an MCP server terminates the stdio process,
// unregisters tools, and subsequent calls return "unknown tool" within timeout.
func TestHost_UnregisterStdio_TerminatesProcessAndUnknownTool(t *testing.T) {
	bin := buildEchoServer(t)
	h := mcp.NewHost(mcp.ClientInfo{Name: "test", Version: "0"})
	defer h.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := h.Register(ctx, mcp.ServerConfig{
		Name:    "life",
		Type:    "stdio",
		Command: bin,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if !h.HasTool("mcp.life.echo") {
		t.Fatal("expected mcp.life.echo registered")
	}
	pid := h.ServerPID("life")
	if pid <= 0 {
		t.Fatalf("expected live stdio pid, got %d", pid)
	}
	if !processAlive(pid) {
		t.Fatalf("pid %d not alive after register", pid)
	}

	// Successful call before stop.
	content, isErr := h.Call(ctx, "mcp.life.echo", `{"message":"before-stop"}`)
	if isErr || !strings.Contains(content, "echo:before-stop") {
		t.Fatalf("pre-stop call: content=%q isErr=%v", content, isErr)
	}

	// Stop/unregister: process must die, tools gone.
	if err := h.Stop("life"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if h.HasTool("mcp.life.echo") {
		t.Fatal("tool still registered after stop")
	}
	if h.ServerPID("life") != 0 {
		t.Fatalf("ServerPID after stop = %d want 0", h.ServerPID("life"))
	}

	// Wait for OS reaper (process gone).
	deadline := time.Now().Add(3 * time.Second)
	for processAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if processAlive(pid) {
		t.Fatalf("stdio pid %d still alive after Stop", pid)
	}

	// Subsequent call returns clear unknown-tool (not hang).
	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()
	start := time.Now()
	content, isErr = h.Call(callCtx, "mcp.life.echo", `{"message":"after"}`)
	elapsed := time.Since(start)
	if !isErr {
		t.Fatalf("expected error after stop, got %q", content)
	}
	if !strings.Contains(strings.ToLower(content), "unknown tool") {
		t.Fatalf("want unknown tool message, got %q", content)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("call after stop took %v (must not hang)", elapsed)
	}

	// Registry path also reports unknown tool after host stop + still-wired registry.
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	// Fresh host re-register then stop via registry helper.
	h2 := mcp.NewHost(mcp.DefaultClientInfo)
	defer h2.Close()
	if err := h2.Register(ctx, mcp.ServerConfig{Name: "life", Type: "stdio", Command: bin}); err != nil {
		t.Fatal(err)
	}
	reg.SetMCPHost(h2)
	res := reg.Execute(ctx, tools.Call{ID: "1", Name: "mcp.life.echo", Arguments: `{"message":"x"}`}, nil)
	if res.IsError {
		t.Fatalf("pre-stop reg execute: %s", res.Content)
	}
	if err := tools.StopMCPServer(reg, "life"); err != nil {
		t.Fatalf("StopMCPServer: %v", err)
	}
	// Tool definitions must not include the stopped tool.
	for _, d := range reg.Definitions() {
		if d.Function.Name == "mcp.life.echo" {
			t.Fatal("mcp.life.echo still in Definitions after StopMCPServer")
		}
	}
	res2 := reg.Execute(ctx, tools.Call{ID: "2", Name: "mcp.life.echo", Arguments: `{"message":"y"}`}, nil)
	if !res2.IsError {
		t.Fatalf("expected unknown tool after stop, got %q", res2.Content)
	}
	if !strings.Contains(strings.ToLower(res2.Content), "unknown tool") {
		t.Fatalf("registry post-stop: %q", res2.Content)
	}
}

// VAL-MCP-006: HTTP server stop closes client and unregisters tools.
func TestHost_UnregisterHTTP_ClosesAndUnknownTool(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var rq struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &rq)
		switch rq.Method {
		case "initialize":
			writeJSON(w, map[string]any{
				"jsonrpc": "2.0", "id": rq.ID,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]string{"name": "http-stop", "version": "1"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeJSON(w, map[string]any{
				"jsonrpc": "2.0", "id": rq.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "ping", "description": "pong", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
					},
				},
			})
		case "tools/call":
			writeJSON(w, map[string]any{
				"jsonrpc": "2.0", "id": rq.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "pong"}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	base := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	if err := h.Register(ctx, mcp.ServerConfig{
		Name:    "httpstop",
		Type:    "http",
		BaseURL: base,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if !h.HasTool("mcp.httpstop.ping") {
		t.Fatalf("tools=%v", h.NamespacedNames())
	}
	content, isErr := h.Call(ctx, "mcp.httpstop.ping", `{}`)
	if isErr || content != "pong" {
		t.Fatalf("pre-stop call=%q isErr=%v", content, isErr)
	}

	if err := h.Unregister("httpstop"); err != nil {
		t.Fatalf("unregister: %v", err)
	}
	if h.HasTool("mcp.httpstop.ping") {
		t.Fatal("tool still present after unregister")
	}
	content, isErr = h.Call(ctx, "mcp.httpstop.ping", `{}`)
	if !isErr || !strings.Contains(strings.ToLower(content), "unknown tool") {
		t.Fatalf("post-stop call=%q isErr=%v", content, isErr)
	}
}

// VAL-MCP-008: MCP tool calls emit ToolStart / ToolEnd (or ToolError) with namespaced name.
func TestMCP_ToolCallTelemetryEvents(t *testing.T) {
	bin := buildEchoServer(t)
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	ctx := context.Background()
	if err := h.Register(ctx, mcp.ServerConfig{Name: "tele", Type: "stdio", Command: bin}); err != nil {
		t.Fatal(err)
	}
	reg.SetMCPHost(h)

	var events []tools.Event
	res := reg.Execute(ctx, tools.Call{
		ID: "tc1", Name: "mcp.tele.echo", Arguments: `{"message":"telemetry"}`,
	}, func(ev tools.Event) {
		events = append(events, ev)
	})
	if res.IsError {
		t.Fatalf("call: %s", res.Content)
	}
	if !strings.Contains(res.Content, "echo:telemetry") {
		t.Fatalf("content=%q", res.Content)
	}
	if len(events) < 2 {
		t.Fatalf("expected start+end events, got %+v", events)
	}
	if events[0].Type != tools.EventStart || events[0].Name != "mcp.tele.echo" {
		t.Fatalf("start event=%+v", events[0])
	}
	last := events[len(events)-1]
	if last.Type != tools.EventEnd || last.Name != "mcp.tele.echo" {
		t.Fatalf("end event=%+v", last)
	}

	// Error path: unknown tool after stop → ToolStart + ToolError with namespaced name.
	_ = h.Stop("tele")
	events = nil
	resErr := reg.Execute(ctx, tools.Call{
		ID: "tc2", Name: "mcp.tele.echo", Arguments: `{}`,
	}, func(ev tools.Event) {
		events = append(events, ev)
	})
	if !resErr.IsError {
		t.Fatal("expected error after stop")
	}
	if len(events) < 2 {
		t.Fatalf("error events=%+v", events)
	}
	if events[0].Type != tools.EventStart || events[0].Name != "mcp.tele.echo" {
		t.Fatalf("err start=%+v", events[0])
	}
	last = events[len(events)-1]
	if last.Type != tools.EventError || last.Name != "mcp.tele.echo" {
		t.Fatalf("err end=%+v", last)
	}
	if !strings.Contains(strings.ToLower(last.Content), "unknown tool") {
		t.Fatalf("error content=%q", last.Content)
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 probes existence without killing (POSIX).
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
