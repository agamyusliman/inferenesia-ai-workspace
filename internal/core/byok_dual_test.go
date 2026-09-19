package core_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-CLI-013: chat+tools turn with only openai_compatible BYOK credentials
// (TEMP_AI_API_KEY unset). Requests must not target ai.temp.web.id.
func TestChatToolsBYOKOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.Host), "ai.temp.web.id") {
			t.Errorf("request host is temp-ai: %s", r.Host)
		}
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "tool-model"}},
			})
			return
		}
		var reqBody struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if len(reqBody.Messages) > 0 {
			last := reqBody.Messages[len(reqBody.Messages)-1]
			if last["role"] == "tool" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]string{"role": "assistant", "content": "file-written"}},
					},
				})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]string{
									"name":      "write_file",
									"arguments": `{"path":"byok-dual.txt","content":"hello-byok"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(provider.EnvAPIKey, "")
	t.Setenv(provider.EnvBaseURL, "")
	t.Setenv(provider.EnvModel, "")
	t.Setenv(provider.EnvBYOKBaseURL, srv.URL+"/v1")
	t.Setenv(provider.EnvBYOKAPIKey, "only-byok-key-for-dual")
	t.Setenv(provider.EnvBYOKModel, "tool-model")

	r := provider.Bootstrap(provider.BootstrapOptions{})
	if r.DefaultID() != provider.ProfileBYOK {
		t.Fatalf("default=%q want byok", r.DefaultID())
	}
	client, err := r.GetOpenAI("")
	if err != nil {
		t.Fatal(err)
	}
	if provider.IsTempAIHost(client.RequestHost()) {
		t.Fatalf("target host is temp-ai: %s", client.RequestHost())
	}

	ws := t.TempDir()
	gw, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	var out strings.Builder
	res, err := core.Run(context.Background(), core.AgentConfig{
		Client:       client,
		Tools:        reg,
		Model:        "tool-model",
		SystemPrompt: core.DefaultSystemPrompt,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "create byok-dual.txt with hello-byok"},
		},
		Stream: false,
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("agent run: %v", err)
	}
	if res.ToolRounds < 1 {
		t.Fatalf("expected tool rounds, got %d final=%q", res.ToolRounds, res.Final)
	}
	path := filepath.Join(ws, "byok-dual.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hello-byok" {
		t.Fatalf("file content=%q", data)
	}
	if strings.Contains(out.String(), "only-byok-key") {
		t.Fatalf("output leaked API key")
	}
}

// VAL-CLI-013: chat+tools with only TEMP_AI_* credentials (no BYOK env).
func TestChatToolsTempAIOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "temp-tool-model"}},
			})
			return
		}
		var reqBody struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if len(reqBody.Messages) > 0 {
			last := reqBody.Messages[len(reqBody.Messages)-1]
			if last["role"] == "tool" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{"message": map[string]string{"role": "assistant", "content": "shell-ok"}},
					},
				})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_shell",
								"type": "function",
								"function": map[string]string{
									"name":      "shell",
									"arguments": `{"command":"echo tool-ok"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	t.Setenv(provider.EnvBYOKBaseURL, "")
	t.Setenv(provider.EnvBYOKAPIKey, "")
	t.Setenv(provider.EnvAPIKey, "only-temp-ai-key-for-dual")
	t.Setenv(provider.EnvBaseURL, srv.URL+"/v1")
	t.Setenv(provider.EnvModel, "temp-tool-model")

	r := provider.Bootstrap(provider.BootstrapOptions{})
	if r.DefaultID() != provider.ProfileInferenesia {
		t.Fatalf("default=%q want inferenesia", r.DefaultID())
	}
	if _, err := r.Get(provider.ProfileBYOK); err == nil {
		t.Fatal("byok must not be registered without BYOK base url")
	}
	client, err := r.GetOpenAI(provider.ProfileInferenesia)
	if err != nil {
		t.Fatal(err)
	}

	ws := t.TempDir()
	gw, err := writegate.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	var out strings.Builder
	res, err := core.Run(context.Background(), core.AgentConfig{
		Client:       client,
		Tools:        reg,
		Model:        "temp-tool-model",
		SystemPrompt: core.DefaultSystemPrompt,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "run echo tool-ok"},
		},
		Stream: false,
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("agent run: %v", err)
	}
	if res.ToolRounds < 1 {
		t.Fatalf("expected tool rounds, got %d", res.ToolRounds)
	}
	if strings.Contains(out.String()+res.Final, "only-temp-ai-key") {
		t.Fatalf("output leaked API key")
	}
}
