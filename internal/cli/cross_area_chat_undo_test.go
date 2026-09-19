package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-CROSS-001: first-run CLI chat then undo write end-to-end on a fresh
// ~/.yura-ai without desktop.
//
// This exercises the real cross-area flow:
//   CLI chat → config bootstrap (YURA_AI_HOME) → provider HTTP (temp-ai mock)
//   → core agent loop → tools write_file → WriteGateway snapshot → undo CLI
//   restores pre-write bytes byte-identically.
//
// The mock gateway is a recorded fixture (VAL-CROSS-001 allows recorded
// fixture when live key is gated): it serves /v1/models and a two-turn
// /v1/chat/completions SSE stream (turn 1 = write_file tool call, turn 2 =
// final streamed answer). No live key is used and no key is ever printed.
func TestCrossArea_FirstRunChatThenUndoWrite_E2E(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-existing dirty file content (the bytes that must be restored by undo).
	const dirtyBefore = "DIRTY-BEFORE-CONTENT"
	fPath := filepath.Join(ws, "target.txt")
	if err := os.WriteFile(fPath, []byte(dirtyBefore), 0o644); err != nil {
		t.Fatal(err)
	}
	preBytes, err := os.ReadFile(fPath)
	if err != nil {
		t.Fatal(err)
	}

	// Mock temp-ai gateway: /models + two-turn /chat/completions SSE.
	const (
		writePath    = "target.txt"
		writeContent = "AGENT-AFTER-CONTENT"
		finalAnswer  = "wrote target.txt"
	)
	var chatCallCount int
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "mock-glm-5", "owned_by": "temp-ai"},
				{"id": "mock-grok-4", "owned_by": "temp-ai"},
			},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		chatCallCount++
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		writeSSE := func(s string) {
			_, _ = io.WriteString(w, s)
			if flusher != nil {
				flusher.Flush()
			}
		}

		// Detect turn: if any message has role "tool", this is the post-tool turn.
		hasToolResult := false
		for _, m := range req.Messages {
			if m.Role == "tool" {
				hasToolResult = true
				break
			}
		}

		if !hasToolResult {
			// Turn 1: stream a write_file tool call.
			args, _ := json.Marshal(map[string]string{
				"path":    writePath,
				"content": writeContent,
			})
			toolChunk := map[string]any{
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"index": 0,
									"id":   "call_write_1",
									"type": "function",
									"function": map[string]any{
										"name":      "write_file",
										"arguments": string(args),
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			b, _ := json.Marshal(toolChunk)
			writeSSE("data: " + string(b) + "\n\n")
			writeSSE("data: [DONE]\n\n")
			return
		}

		// Turn 2: stream the final answer in two chunks to prove streaming.
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"" + finalAnswer[:5] + "\"},\"finish_reason\":null}]}\n\n")
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"" + finalAnswer[5:] + "\"},\"finish_reason\":\"stop\"}]}\n\n")
		writeSSE("data: [DONE]\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Fresh ~/.yura-ai (override) + mock gateway env. Key is a non-secret marker;
	// the test asserts it is never printed.
	const fakeKey = "mock-key-not-a-real-secret"
	t.Setenv("YURA_AI_HOME", home)
	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", fakeKey)
	t.Setenv("TEMP_AI_MODEL", "")

	// --- Phase 1: CLI chat (first run) ---
	var chatOut, chatErr bytes.Buffer
	chatCode := ExecuteWith(
		[]string{"chat", "-p", "create target.txt with AGENT-AFTER-CONTENT", "--workspace", ws, "--no-persist"},
		&chatOut, &chatErr,
	)
	if chatCode != 0 {
		t.Fatalf("chat exit=%d\nstdout=%q\nstderr=%q", chatCode, chatOut.String(), chatErr.String())
	}

	// (a) Fresh home bootstrapped: ~/.yura-ai exists mode 0700 with config.yaml.
	info, err := os.Stat(home)
	if err != nil {
		t.Fatalf("config home not created after first-run chat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("config home is not a directory")
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("home mode = %o, want 0700", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatalf("config.yaml not bootstrapped: %v", err)
	}

	// (b) Gateway was actually contacted (models + at least one chat call).
	if chatCallCount == 0 {
		t.Fatalf("gateway was never contacted by chat (chatCallCount=0)\nstdout=%q\nstderr=%q",
			chatOut.String(), chatErr.String())
	}

	// (c) Response was streamed to stdout (final answer present).
	combined := chatOut.String()
	if !strings.Contains(combined, finalAnswer) {
		t.Fatalf("stdout missing streamed final answer %q\nstdout=%q\nstderr=%q",
			finalAnswer, chatOut.String(), chatErr.String())
	}

	// (d) No API key leaked into stdout/stderr (VAL-FOUND-012 cross-cutting).
	if strings.Contains(combined+chatErr.String(), fakeKey) {
		t.Fatalf("API key leaked into chat output:\nstdout=%q\nstderr=%q",
			chatOut.String(), chatErr.String())
	}

	// (e) Agent wrote the file via the write_file tool (through WriteGateway).
	afterBytes, err := os.ReadFile(fPath)
	if err != nil {
		t.Fatalf("target file missing after chat: %v", err)
	}
	if string(afterBytes) != writeContent {
		t.Fatalf("file content after chat = %q, want %q", afterBytes, writeContent)
	}
	if chatCallCount < 2 {
		t.Fatalf("expected >=2 chat completion calls (tool + final), got %d", chatCallCount)
	}
	// (f) FileChanged event observed on stderr (VAL-UNDO-009).
	if !strings.Contains(chatErr.String(), "[file_changed]") {
		t.Fatalf("stderr missing [file_changed] event:\n%s", chatErr.String())
	}

	// --- Phase 2: CLI undo (separate process, same workspace + home) ---
	var undoOut, undoErr bytes.Buffer
	undoCode := ExecuteWith(
		[]string{"undo", "--workspace", ws},
		&undoOut, &undoErr,
	)
	if undoCode != 0 {
		t.Fatalf("undo exit=%d\nstdout=%q\nstderr=%q", undoCode, undoOut.String(), undoErr.String())
	}
	undoMsg := undoOut.String() + undoErr.String()
	if !strings.Contains(strings.ToLower(undoMsg), "pre-agent") &&
		!strings.Contains(strings.ToLower(undoMsg), "undid") {
		t.Fatalf("undo missing clear message: %q", undoMsg)
	}

	// (g) Undo restores pre-write bytes byte-identically (NOT git HEAD, NOT AFTER).
	restoredBytes, err := os.ReadFile(fPath)
	if err != nil {
		t.Fatalf("target file missing after undo: %v", err)
	}
	if string(restoredBytes) != dirtyBefore {
		t.Fatalf("after undo content = %q, want pre-write %q (byte-identical)",
			restoredBytes, dirtyBefore)
	}
	if !bytes.Equal(restoredBytes, preBytes) {
		t.Fatalf("after undo bytes != pre-write bytes (byte compare failed)")
	}
	if string(restoredBytes) == writeContent {
		t.Fatal("undo left AFTER content — stack did not restore pre-write")
	}
	// (h) No key leaked from undo path either.
	if strings.Contains(undoMsg, fakeKey) {
		t.Fatalf("API key leaked into undo output: %q", undoMsg)
	}

	// (i) undo emitted a [file_changed] event for the restored path (VAL-UNDO-009).
	if !strings.Contains(undoOut.String(), "[file_changed]") {
		t.Fatalf("undo stdout missing [file_changed] event: %q", undoOut.String())
	}
}

// VAL-CROSS-001 variant: the same flow but with a pre-existing file that did
// not exist before the agent (pre-absent). Undo must remove the file.
func TestCrossArea_FirstRunChatThenUndoWrite_PreAbsent_E2E(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	const writePath = "fresh.txt"
	const writeContent = "NEW-FILE-BY-AGENT"
	fPath := filepath.Join(ws, writePath)

	// Confirm the file does not exist before the agent turn.
	if _, err := os.Stat(fPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s should not exist, got err=%v", fPath, err)
	}

	var chatCallCount int
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "mock-glm-5", "owned_by": "temp-ai"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		chatCallCount++
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		writeSSE := func(s string) {
			_, _ = io.WriteString(w, s)
			if flusher != nil {
				flusher.Flush()
			}
		}
		hasToolResult := false
		for _, m := range req.Messages {
			if m.Role == "tool" {
				hasToolResult = true
				break
			}
		}
		if !hasToolResult {
			args, _ := json.Marshal(map[string]string{"path": writePath, "content": writeContent})
			toolChunk := map[string]any{
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"index": 0, "id": "call_w", "type": "function",
							"function": map[string]any{"name": "write_file", "arguments": string(args)},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}
			b, _ := json.Marshal(toolChunk)
			writeSSE("data: " + string(b) + "\n\n")
			writeSSE("data: [DONE]\n\n")
			return
		}
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"created\"},\"finish_reason\":\"stop\"}]}\n\n")
		writeSSE("data: [DONE]\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	const fakeKey = "mock-key-not-a-real-secret"
	t.Setenv("YURA_AI_HOME", home)
	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", fakeKey)
	t.Setenv("TEMP_AI_MODEL", "")

	var chatOut, chatErr bytes.Buffer
	if code := ExecuteWith(
		[]string{"chat", "-p", "create fresh.txt", "--workspace", ws, "--no-persist"},
		&chatOut, &chatErr,
	); code != 0 {
		t.Fatalf("chat exit=%d\nstdout=%q\nstderr=%q", code, chatOut.String(), chatErr.String())
	}
	if chatCallCount < 2 {
		t.Fatalf("expected >=2 chat calls, got %d", chatCallCount)
	}
	// File now exists.
	data, err := os.ReadFile(fPath)
	if err != nil {
		t.Fatalf("fresh file missing after chat: %v", err)
	}
	if string(data) != writeContent {
		t.Fatalf("fresh file content=%q want %q", data, writeContent)
	}

	// Undo → file removed (pre-absent case).
	var undoOut, undoErr bytes.Buffer
	if code := ExecuteWith(
		[]string{"undo", "--workspace", ws},
		&undoOut, &undoErr,
	); code != 0 {
		t.Fatalf("undo exit=%d %s %s", code, undoOut.String(), undoErr.String())
	}
	if _, err := os.Stat(fPath); !os.IsNotExist(err) {
		t.Fatalf("after undo fresh file should be removed (pre-absent), got err=%v content=%q",
			err, mustReadFile(t, fPath))
	}
}

// Smoke: the cross-area flow also works without tools (plain chat) and still
// bootstraps the config home + streams from the gateway. Ensures the chat
// happy path is not broken by the cross-area wiring.
func TestCrossArea_FirstRunChatNoTools_E2E(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "mock-glm-5", "owned_by": "temp-ai"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		writeSSE := func(s string) {
			_, _ = io.WriteString(w, s)
			if flusher != nil {
				flusher.Flush()
			}
		}
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n")
		writeSSE("data: [DONE]\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	const fakeKey = "mock-key-not-a-real-secret"
	t.Setenv("YURA_AI_HOME", home)
	t.Setenv("TEMP_AI_BASE_URL", srv.URL+"/v1")
	t.Setenv("TEMP_AI_API_KEY", fakeKey)

	var out, errBuf bytes.Buffer
	code := ExecuteWith(
		[]string{"chat", "-p", "hi", "--workspace", ws, "--no-tools", "--no-persist"},
		&out, &errBuf,
	)
	if code != 0 {
		t.Fatalf("chat exit=%d\nstdout=%q\nstderr=%q", code, out.String(), errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatalf("config not bootstrapped: %v", err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("streamed answer missing: %q", out.String())
	}
	if strings.Contains(out.String()+errBuf.String(), fakeKey) {
		t.Fatalf("key leaked: %q %q", out.String(), errBuf.String())
	}
}

// ensure fmt import is used even if future edits drop a log line.
var _ = fmt.Sprintf
