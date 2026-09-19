package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/mcp"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// buildEchoServer compiles the fixture MCP stdio server into t.TempDir.
func buildEchoServer(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "echo_mcp_server")
	cmd := exec.Command("go", "build", "-o", out, "./testdata/echo_server.go")
	cmd.Dir = filepath.Join(".", "") // package dir
	// Resolve absolute package path for robustness when go test cwd is package dir.
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Dir = pkgDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	outb, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build echo server: %v\n%s", err, outb)
	}
	return out
}

func TestHost_RegisterStdio_ToolsAndCall(t *testing.T) {
	// VAL-MCP-001: spawn stdio, list tools, call echo.
	bin := buildEchoServer(t)
	h := mcp.NewHost(mcp.ClientInfo{Name: "test", Version: "0"})
	defer h.Close()

	var logs []string
	h.Logf = func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err := h.Register(ctx, mcp.ServerConfig{
		Name:    "echo",
		Type:    "stdio",
		Command: bin,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	names := h.NamespacedNames()
	foundEcho := false
	foundFS := false
	for _, n := range names {
		if n == "mcp.echo.echo" {
			foundEcho = true
		}
		if n == "mcp.echo.fs.read" {
			foundFS = true
		}
	}
	if !foundEcho {
		t.Fatalf("missing mcp.echo.echo in %v", names)
	}
	if !foundFS {
		t.Fatalf("missing mcp.echo.fs.read in %v", names)
	}

	content, isErr := h.Call(ctx, "mcp.echo.echo", `{"message":"ping-payload"}`)
	if isErr {
		t.Fatalf("call error: %s", content)
	}
	if !strings.Contains(content, "echo:ping-payload") {
		t.Fatalf("content=%q", content)
	}
	_ = logs
}

func TestHost_ToolNameIsolation(t *testing.T) {
	// VAL-MCP-005: MCP fs.read becomes mcp.<server>.fs.read; built-in read_file unchanged.
	bin := buildEchoServer(t)
	root := t.TempDir()
	// seed a real file for built-in read_file
	if err := os.WriteFile(filepath.Join(root, "built-in.txt"), []byte("builtin-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	ctx := context.Background()
	if err := h.Register(ctx, mcp.ServerConfig{Name: "srv", Type: "stdio", Command: bin}); err != nil {
		t.Fatal(err)
	}
	reg.SetMCPHost(h)

	// Built-in name still present.
	var builtin, namespaced bool
	for _, d := range reg.Definitions() {
		if d.Function.Name == tools.NameReadFile {
			builtin = true
		}
		if d.Function.Name == "mcp.srv.fs.read" {
			namespaced = true
		}
		if d.Function.Name == "fs.read" {
			t.Fatalf("bare fs.read must not appear in agent tool list")
		}
	}
	if !builtin {
		t.Fatal("built-in read_file missing")
	}
	if !namespaced {
		t.Fatal("namespaced mcp.srv.fs.read missing")
	}

	// Call namespaced MCP tool.
	res := reg.Execute(ctx, tools.Call{
		ID: "1", Name: "mcp.srv.fs.read", Arguments: `{"path":"secret.txt"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("mcp call: %s", res.Content)
	}
	if !strings.Contains(res.Content, "mcp-fs-read:secret.txt") {
		t.Fatalf("mcp content=%q", res.Content)
	}

	// Built-in still works distinctly.
	res2 := reg.Execute(ctx, tools.Call{
		ID: "2", Name: tools.NameReadFile, Arguments: `{"path":"built-in.txt"}`,
	}, nil)
	if res2.IsError {
		t.Fatalf("builtin: %s", res2.Content)
	}
	if !strings.Contains(res2.Content, "builtin-data") {
		t.Fatalf("builtin content=%q", res2.Content)
	}
}

func TestHost_HTTPConfigAndHeadersNotLogged(t *testing.T) {
	// VAL-MCP-002: remote HTTP MCP against local fixture in 4100–4199; headers never logged.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if port < 4100 || port > 4199 {
		// Port allocator not required; tests may land outside range when 0 is used.
		// For contract: use an explicit port in range if free.
		_ = ln.Close()
		ln, err = net.Listen("tcp", "127.0.0.1:4117")
		if err != nil {
			// fall back to whatever free port we got earlier path — ok for functional test
			ln, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
		}
		defer ln.Close()
		port = ln.Addr().(*net.TCPAddr).Port
	}

	secret := "Bearer super-secret-auth-token-never-log-me"
	var idSeq int64
	var mu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		// Require Authorization so 401 path is testable elsewhere.
		if got := r.Header.Get("Authorization"); got != secret {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"unauthorized"}}`))
			return
		}
		var rq struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &rq)
		mu.Lock()
		idSeq++
		mu.Unlock()
		switch rq.Method {
		case "initialize":
			writeJSON(w, map[string]any{
				"jsonrpc": "2.0", "id": rq.ID,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]string{"name": "http-fixture", "version": "1"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeJSON(w, map[string]any{
				"jsonrpc": "2.0", "id": rq.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "ping",
							"description": "returns pong",
							"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
						},
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

	var logBuf strings.Builder
	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	h.Logf = func(format string, args ...any) {
		logBuf.WriteString(fmt.Sprintf(format, args...))
		logBuf.WriteByte('\n')
	}

	base := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = h.Register(ctx, mcp.ServerConfig{
		Name:    "httpdocs",
		Type:    "http",
		BaseURL: base,
		Headers: map[string]string{"Authorization": secret},
	})
	if err != nil {
		t.Fatalf("register http: %v", err)
	}
	names := h.NamespacedNames()
	found := false
	for _, n := range names {
		if n == "mcp.httpdocs.ping" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tools=%v", names)
	}
	content, isErr := h.Call(ctx, "mcp.httpdocs.ping", `{}`)
	if isErr || content != "pong" {
		t.Fatalf("call=%q err=%v", content, isErr)
	}

	// Headers must never be logged.
	logged := logBuf.String()
	if strings.Contains(logged, secret) || strings.Contains(logged, "super-secret-auth-token") {
		t.Fatalf("authorization leaked into logs: %q", logged)
	}
	// Also check SafeHeadersForLog only returns names.
	namesOnly := mcp.SafeHeadersForLog(map[string]string{"Authorization": secret, "X-Test": "v"})
	for _, n := range namesOnly {
		if strings.Contains(n, "secret") || strings.Contains(n, "Bearer") {
			t.Fatalf("header value in name list: %q", n)
		}
	}
}

func TestHost_FailureIsolation(t *testing.T) {
	// VAL-MCP-003: dead server clear error; live server still works; host continues.
	bin := buildEchoServer(t)
	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()

	var logs []string
	var logMu sync.Mutex
	h.Logf = func(format string, args ...any) {
		logMu.Lock()
		logs = append(logs, fmt.Sprintf(format, args...))
		logMu.Unlock()
	}

	ctx := context.Background()
	// Dead stdio: non-existent binary.
	errDead := h.Register(ctx, mcp.ServerConfig{
		Name:    "dead",
		Type:    "stdio",
		Command: filepath.Join(t.TempDir(), "does-not-exist-mcp-server"),
	})
	if errDead == nil {
		t.Fatal("expected dead server register error")
	}
	if !strings.Contains(errDead.Error(), `server "dead"`) || !strings.Contains(errDead.Error(), "unavailable") {
		t.Fatalf("dead error shape: %v", errDead)
	}

	// Live server.
	if err := h.Register(ctx, mcp.ServerConfig{Name: "live", Type: "stdio", Command: bin}); err != nil {
		t.Fatalf("live register: %v", err)
	}

	// Live tools present.
	if !h.HasTool("mcp.live.echo") {
		t.Fatal("live tool missing after dead register")
	}
	content, isErr := h.Call(ctx, "mcp.live.echo", `{"message":"ok"}`)
	if isErr || !strings.Contains(content, "echo:ok") {
		t.Fatalf("live call failed: %q isErr=%v", content, isErr)
	}

	// Dead tools absent; call returns clear error without panic.
	content, isErr = h.Call(ctx, "mcp.dead.echo", `{}`)
	if !isErr {
		t.Fatalf("expected error for dead tool, got %q", content)
	}
	if !strings.Contains(content, `server "dead"`) {
		t.Fatalf("dead call msg=%q", content)
	}

	// Log must have clear unavailable for dead.
	logMu.Lock()
	joined := strings.Join(logs, "\n")
	logMu.Unlock()
	if !strings.Contains(joined, `server "dead"`) || !strings.Contains(joined, "unavailable") {
		t.Fatalf("logs missing unavailable: %q", joined)
	}

	// HTTP 401 path also clear.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"unauthorized"}}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	err401 := h.Register(ctx, mcp.ServerConfig{
		Name:    "authfail",
		Type:    "http",
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d/mcp", port),
	})
	if err401 == nil {
		t.Fatal("expected 401 register error")
	}
	if !strings.Contains(err401.Error(), `server "authfail"`) ||
		!(strings.Contains(err401.Error(), "401") || strings.Contains(err401.Error(), "auth")) {
		t.Fatalf("401 error shape: %v", err401)
	}

	// Live still works after 401 peer failed.
	content, isErr = h.Call(ctx, "mcp.live.echo", `{"message":"still"}`)
	if isErr || !strings.Contains(content, "echo:still") {
		t.Fatalf("live after 401: %q %v", content, isErr)
	}
}

func TestHost_NoRAGWiringSource(t *testing.T) {
	// VAL-MCP-004 negative: package source + package API reject rag type.
	// Schema rejection covered in config_test; here ensure Host.RegisterAll fails on rag
	// without connecting anything.
	h := mcp.NewHost(mcp.DefaultClientInfo)
	defer h.Close()
	err := h.RegisterAll(context.Background(), map[string]mcp.ServerConfig{
		"bad": {Name: "bad", Type: "rag", URL: "http://127.0.0.1:6333"},
	})
	if err == nil {
		t.Fatal("expected rag rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rag") {
		t.Fatalf("err=%v", err)
	}
	if len(h.Status()) != 0 {
		t.Fatalf("no servers should be registered on schema reject: %+v", h.Status())
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
