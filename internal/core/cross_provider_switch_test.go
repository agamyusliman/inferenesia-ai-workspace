package core_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// VAL-CROSS-006: provider switch mid-app cross-area flow.
//
// Flow under test (must exercise ALL four expected behaviors of the feature):
//  1. Start on the temp-ai gateway profile (tempai) and run a chat turn that
//     streams TokenDelta + ToolStart events (tools enabled, NOT NoTools).
//  2. Mid-session (no app restart, no service reconstruction) switch the
//     active provider/model to a BYOK openai_compatible profile.
//  3. Run the next chat turn — it must re-route subsequent ChatStream calls to
//     the new BYOK provider:
//       - HTTP request target host == BYOK base_url (NOT ai.temp.web.id / temp-ai mock)
//       - Authorization header on BYOK host carries the BYOK key (direct routing)
//       - temp-ai mock receives ZERO chat requests after the switch (isolation)
//       - TokenDelta events keep flowing on the new route
//       - ToolStart events keep flowing on the new route (tool loop works)
//  4. Session messages persist across the switch — the post-switch turn sees
//     the prior user + assistant messages in its request body; history count
//     only grows (never wiped by SetActiveProvider).
//
// Distinct from TestMidSessionProviderSwitch (which uses NoTools:true and
// switches byok-a → byok-b): this one switches **tempai → BYOK** and asserts
// tool + token events keep flowing after the switch with tools enabled.
func TestCrossArea_ProviderSwitchMidApp_ToolsAndHistoryPersist(t *testing.T) {
	// --- temp-ai gateway mock: serves models + a tool-calling chat turn ---
	var (
		tempChatHits   atomic.Int32
		tempModelsHits atomic.Int32
	)
	tempSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			tempModelsHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "temp-glm-5"}},
			})
			return
		}
		// chat/completions
		tempChatHits.Add(1)
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		writeSSE := func(s string) {
			_, _ = io.WriteString(w, s)
			if fl != nil {
				fl.Flush()
			}
		}
		// Detect turn: if the LAST message is a tool result, this is the post-tool turn.
		// (Checking "any tool" is wrong because history carries prior tool results across turns.)
		hasTool := false
		if len(req.Messages) > 0 {
			hasTool = req.Messages[len(req.Messages)-1].Role == "tool"
		}
		if !hasTool {
			// Turn 1: stream a write_file tool_call with a token delta first.
			writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"Plan\"},\"finish_reason\":null}]}\n\n")
			args, _ := json.Marshal(map[string]string{
				"path":    "from-tempai.txt",
				"content": "created-on-tempai-route",
			})
			toolChunk := map[string]any{
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_temp_1",
							"type":  "function",
							"function": map[string]any{
								"name":      "write_file",
								"arguments": string(args),
							},
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
		// Turn 2: streamed final answer (two token deltas → proves TokenDelta flow).
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"temp-\"},\"finish_reason\":null}]}\n\n")
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"wrote\"},\"finish_reason\":\"stop\"}]}\n\n")
		writeSSE("data: [DONE]\n\n")
	}))
	defer tempSrv.Close()

	// --- BYOK mock: must receive the post-switch traffic. ---
	var (
		byokChatHits   atomic.Int32
		byokModelsHits atomic.Int32
		byokAuthMu     sync.Mutex
		byokAuth       string
		byokHostMu     sync.Mutex
		byokHost       string
		byokLastMsgsMu sync.Mutex
		byokLastMsgs   []string // roles seen on the post-switch request
	)
	byokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		byokHostMu.Lock()
		byokHost = r.Host
		byokHostMu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/models") {
			byokModelsHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "byok-custom-1"}},
			})
			return
		}
		byokChatHits.Add(1)
		byokAuthMu.Lock()
		byokAuth = r.Header.Get("Authorization")
		byokAuthMu.Unlock()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		roles := make([]string, 0, len(req.Messages))
		for _, m := range req.Messages {
			roles = append(roles, m.Role)
		}
		byokLastMsgsMu.Lock()
		byokLastMsgs = roles
		byokLastMsgsMu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		writeSSE := func(s string) {
			_, _ = io.WriteString(w, s)
			if fl != nil {
				fl.Flush()
			}
		}
		// Detect turn: if the LAST message is a tool result, this is the post-tool turn.
		hasTool := false
		if len(req.Messages) > 0 {
			hasTool = req.Messages[len(req.Messages)-1].Role == "tool"
		}
		if !hasTool {
			// Turn 1: stream a write_file tool_call (proves ToolStart keeps flowing post-switch).
			writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"ok \"},\"finish_reason\":null}]}\n\n")
			args, _ := json.Marshal(map[string]string{
				"path":    "from-byok.txt",
				"content": "created-on-byok-route",
			})
			toolChunk := map[string]any{
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"index": 0,
							"id":    "call_byok_1",
							"type":  "function",
							"function": map[string]any{
								"name":      "write_file",
								"arguments": string(args),
							},
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
		// Turn 2: streamed final answer (two token deltas).
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"byok-\"},\"finish_reason\":null}]}\n\n")
		writeSSE("data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\n")
		writeSSE("data: [DONE]\n\n")
	}))
	defer byokSrv.Close()

	// --- Config: tempai profile points at tempSrv; a BYOK profile at byokSrv. ---
	home := t.TempDir()
	cfg := config.FileConfig{
		Version:         1,
		DefaultProvider: "tempai", // start on temp-ai gateway
		Providers: []config.ProviderProfile{
			{
				ID:           "byok-custom",
				Type:         "openai_compatible",
				Name:         "BYOK Custom",
				BaseURL:      byokSrv.URL + "/v1",
				APIKey:       "byok-secret-key-XYZ",
				DefaultModel: "byok-custom-1",
			},
		},
	}
	if err := config.SaveToDir(home, cfg); err != nil {
		t.Fatal(err)
	}
	// env tempai route = tempSrv (so we can observe isolation after switch).
	t.Setenv(provider.EnvBaseURL, tempSrv.URL+"/v1")
	t.Setenv(provider.EnvAPIKey, "temp-gateway-key-value")
	t.Setenv(provider.EnvModel, "temp-glm-5")
	t.Setenv(provider.EnvBYOKBaseURL, "")
	t.Setenv(provider.EnvBYOKAPIKey, "")
	t.Setenv(provider.EnvBYOKModel, "")

	wsRoot := filepath.Join(home, "ws")
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	// --- Phase 1: turn on the gateway route (tools enabled, NOT NoTools). ---
	// config.yaml names the legacy alias "tempai"; the active session must
	// report the canonical registered id so Settings can match a profile row.
	start := svc.GetActiveSession()
	if start.Profile != provider.ProfileInferenesia {
		t.Fatalf("expected start profile=%s, got %q", provider.ProfileInferenesia, start.Profile)
	}
	if !start.IsGateway {
		t.Fatalf("gateway alias did not resolve to the gateway route: %+v", start)
	}

	var (
		mu            sync.Mutex
		tempTokens    []string
		tempToolStart int
		tempDone      core.ChatEvent
	)
	err = svc.ChatStream(context.Background(), core.ChatRequest{
		Prompt: "create from-tempai.txt with created-on-tempai-route",
		// NoTools is FALSE → tool loop is enabled.
	}, func(ev core.ChatEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Type {
		case core.ChatEventToken:
			tempTokens = append(tempTokens, ev.Delta)
		case core.ChatEventToolStart:
			tempToolStart++
		case core.ChatEventDone:
			tempDone = ev
		}
	})
	if err != nil {
		t.Fatalf("phase 1 chat: %v", err)
	}
	if tempChatHits.Load() == 0 {
		t.Fatal("phase 1: temp-ai mock received no chat traffic")
	}
	if tempToolStart == 0 {
		t.Fatalf("phase 1: expected ToolStart events with tools enabled, got %d", tempToolStart)
	}
	if len(tempTokens) == 0 {
		t.Fatal("phase 1: expected TokenDelta events with tools enabled")
	}
	// The write_file tool must have produced a real file via WriteGateway.
	if _, err := os.Stat(filepath.Join(wsRoot, "from-tempai.txt")); err != nil {
		t.Fatalf("phase 1: write_file did not land file via tool loop: %v", err)
	}
	if tempDone.Model != "temp-glm-5" {
		t.Fatalf("phase 1 done.model=%q want temp-glm-5", tempDone.Model)
	}
	if !provider.IsTempAIHost(tempDone.Host) {
		// temp-ai mock host should be classified as the temp-ai route (it is NOT ai.temp.web.id,
		// but it is the configured gateway route). We check that done.Host points at tempSrv host.
		if !strings.Contains(tempSrv.URL, tempDone.Host) && !strings.Contains(tempDone.Host, "127.0.0.1") {
			t.Fatalf("phase 1 done.host=%q unexpected", tempDone.Host)
		}
	}
	// History now contains: system + user + assistant(tool) + tool result + assistant(final)
	histAfterPhase1 := svc.GetChatSession()
	countAfterPhase1 := len(histAfterPhase1.Messages)
	if countAfterPhase1 < 2 {
		t.Fatalf("phase 1: history too short: %d msgs", countAfterPhase1)
	}

	// --- Phase 2: mid-session switch to BYOK (no restart, no history wipe). ---
	switched, err := svc.SetActiveProvider(core.SetActiveProviderRequest{
		Profile: "byok-custom",
		Model:   "byok-custom-1",
	})
	if err != nil {
		t.Fatalf("SetActiveProvider: %v", err)
	}
	if switched.Profile != "byok-custom" {
		t.Fatalf("switched profile=%q want byok-custom", switched.Profile)
	}
	if switched.Model != "byok-custom-1" {
		t.Fatalf("switched model=%q", switched.Model)
	}
	// History MUST persist across switch (no wipe).
	histAfterSwitch := svc.GetChatSession()
	if len(histAfterSwitch.Messages) != countAfterPhase1 {
		t.Fatalf("history wiped by switch: before=%d after=%d",
			countAfterPhase1, len(histAfterSwitch.Messages))
	}

	// --- Phase 3: post-switch turn. Must route to BYOK host, NOT temp-ai. ---
	var (
		byokTokens    []string
		byokToolStart int
		byokDone      core.ChatEvent
	)
	chatHitsBefore := byokChatHits.Load()
	tempChatHitsBefore := tempChatHits.Load()
	err = svc.ChatStream(context.Background(), core.ChatRequest{
		Prompt: "now create from-byok.txt with created-on-byok-route",
		// NoTools still FALSE → tool loop still enabled.
	}, func(ev core.ChatEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Type {
		case core.ChatEventToken:
			byokTokens = append(byokTokens, ev.Delta)
		case core.ChatEventToolStart:
			byokToolStart++
		case core.ChatEventDone:
			byokDone = ev
		}
	})
	if err != nil {
		t.Fatalf("phase 3 chat: %v", err)
	}

	// (a) BYOK mock received new chat traffic.
	if byokChatHits.Load() <= chatHitsBefore {
		t.Fatal("phase 3: BYOK mock received no new chat traffic after switch")
	}
	// (b) temp-ai mock received ZERO new chat traffic after the switch (isolation).
	if tempChatHits.Load() != tempChatHitsBefore {
		t.Fatalf("phase 3: temp-ai mock received %d new chat requests after switch (BYOK must route direct, not via temp-ai)",
			tempChatHits.Load()-tempChatHitsBefore)
	}
	// (c) BYOK Authorization carries the BYOK key (direct routing).
	byokAuthMu.Lock()
	auth := byokAuth
	byokAuthMu.Unlock()
	if !strings.Contains(auth, "byok-secret-key-XYZ") {
		t.Fatalf("phase 3: BYOK Authorization missing BYOK key: %q", auth)
	}
	// (d) Request target host == BYOK base_url host (NOT temp-ai).
	byokHostMu.Lock()
	gotHost := byokHost
	byokHostMu.Unlock()
	if gotHost == "" {
		t.Fatal("phase 3: BYOK mock did not record request host")
	}
	if strings.Contains(gotHost, "ai.temp.web.id") {
		t.Fatalf("phase 3: BYOK request host is temp-ai: %q", gotHost)
	}
	// The host must NOT match the tempSrv host (distinct route).
	tempHost := strings.TrimPrefix(strings.TrimPrefix(tempSrv.URL, "http://"), "https://")
	if i := strings.Index(tempHost, "/"); i >= 0 {
		tempHost = tempHost[:i]
	}
	if gotHost == tempHost {
		t.Fatalf("phase 3: BYOK request host (%q) equals temp-ai route host — switch did not re-route", gotHost)
	}
	// (e) TokenDelta events keep flowing on the new route.
	if len(byokTokens) == 0 {
		t.Fatal("phase 3: no TokenDelta events on BYOK route (tools enabled)")
	}
	// (f) ToolStart events keep flowing on the new route (tool loop still works).
	if byokToolStart == 0 {
		t.Fatalf("phase 3: no ToolStart events on BYOK route (expected write_file tool call), got %d", byokToolStart)
	}
	// (g) The write_file tool actually ran on the BYOK route (file exists).
	if _, err := os.Stat(filepath.Join(wsRoot, "from-byok.txt")); err != nil {
		t.Fatalf("phase 3: write_file did not land file via BYOK tool loop: %v", err)
	}
	// (h) Done metadata reflects the new route.
	if byokDone.Model != "byok-custom-1" {
		t.Fatalf("phase 3 done.model=%q want byok-custom-1", byokDone.Model)
	}
	if provider.IsTempAIHost(byokDone.Host) {
		t.Fatalf("phase 3 done.host=%q is temp-ai (BYOK must route direct)", byokDone.Host)
	}
	// (i) Done.host should equal the BYOK request host.
	if !strings.Contains(byokDone.Host, gotHost) {
		t.Fatalf("phase 3 done.host=%q does not match BYOK request host %q", byokDone.Host, gotHost)
	}

	// --- Phase 4: session history persisted across the switch + grew. ---
	byokLastMsgsMu.Lock()
	roles := byokLastMsgs
	byokLastMsgsMu.Unlock()
	if len(roles) == 0 {
		t.Fatal("phase 3: BYOK mock recorded no messages — did the turn run?")
	}
	// The post-switch request MUST include the prior user + assistant messages from phase 1.
	// (system + user1 + assistant1 + tool + assistant2 + user2 ...)
	hasPriorUser := false
	hasPriorAssistant := false
	for _, r := range roles {
		if r == "user" {
			hasPriorUser = true
		}
		if r == "assistant" {
			hasPriorAssistant = true
		}
	}
	if !hasPriorUser || !hasPriorAssistant {
		t.Fatalf("phase 3: prior history not retained on BYOK route. roles=%v", roles)
	}
	// History count grew (turn 2 appended messages).
	histAfterPhase3 := svc.GetChatSession()
	if len(histAfterPhase3.Messages) <= countAfterPhase1 {
		t.Fatalf("phase 3: history did not grow: phase1=%d phase3=%d",
			countAfterPhase1, len(histAfterPhase3.Messages))
	}

	// --- Phase 5: no secrets leaked into event stream. ---
	// (defensive — BYOK key never appears in any event text).
	mu.Lock()
	allEventText := strings.Join(byokTokens, "") + byokDone.Final
	mu.Unlock()
	if strings.Contains(allEventText, "byok-secret-key-XYZ") {
		t.Fatal("phase 3: BYOK API key leaked into event stream")
	}
}

// TestCrossArea_ProviderSwitchMidApp_DoneHostReflectsRoute is a narrower
// regression test for the Done event host reflection after a switch. It keeps
// NoTools=true for a fast check that the metadata path is correct even when
// no tool loop runs.
func TestCrossArea_ProviderSwitchMidApp_DoneHostReflectsRoute(t *testing.T) {
	// BYOK mock — records its host.
	var byokHostMu sync.Mutex
	var byokHost string
	byokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		byokHostMu.Lock()
		byokHost = r.Host
		byokHostMu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "m1"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok-byok"}},
			},
		})
	}))
	defer byokSrv.Close()

	// temp-ai mock.
	var tempHostMu sync.Mutex
	var tempHost string
	tempSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tempHostMu.Lock()
		tempHost = r.Host
		tempHostMu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"id": "m2"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok-temp"}},
			},
		})
	}))
	defer tempSrv.Close()

	home := t.TempDir()
	_ = config.SaveToDir(home, config.FileConfig{
		Version:         1,
		DefaultProvider: "tempai",
		Providers: []config.ProviderProfile{
			{
				ID:           "byok-hr",
				Type:         "openai_compatible",
				Name:         "BYOK HR",
				BaseURL:      byokSrv.URL + "/v1",
				APIKey:       "k-byok-hr",
				DefaultModel: "m1",
			},
		},
	})
	t.Setenv(provider.EnvBaseURL, tempSrv.URL+"/v1")
	t.Setenv(provider.EnvAPIKey, "k-temp-hr")
	t.Setenv(provider.EnvModel, "m2")

	ws := filepath.Join(home, "ws")
	_ = os.MkdirAll(ws, 0o755)
	svc, err := core.NewService(core.Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	var doneTemp core.ChatEvent
	if err := svc.ChatStream(context.Background(), core.ChatRequest{
		Prompt:  "ping temp",
		NoTools: true,
	}, func(ev core.ChatEvent) {
		if ev.Type == core.ChatEventDone {
			doneTemp = ev
		}
	}); err != nil {
		t.Fatalf("temp chat: %v", err)
	}
	if doneTemp.Profile != "tempai" && doneTemp.Profile != provider.ProfileInferenesia {
		t.Fatalf("doneTemp profile=%q", doneTemp.Profile)
	}

	if _, err := svc.SetActiveProvider(core.SetActiveProviderRequest{
		Profile: "byok-hr",
		Model:   "m1",
	}); err != nil {
		t.Fatal(err)
	}

	var doneByok core.ChatEvent
	if err := svc.ChatStream(context.Background(), core.ChatRequest{
		Prompt:  "ping byok",
		NoTools: true,
	}, func(ev core.ChatEvent) {
		if ev.Type == core.ChatEventDone {
			doneByok = ev
		}
	}); err != nil {
		t.Fatalf("byok chat: %v", err)
	}
	if doneByok.Profile != "byok-hr" {
		t.Fatalf("doneByok profile=%q", doneByok.Profile)
	}
	if doneByok.Model != "m1" {
		t.Fatalf("doneByok model=%q", doneByok.Model)
	}
	// Hosts must differ between routes.
	if doneTemp.Host == doneByok.Host {
		t.Fatalf("temp host == byok host: %q — switch did not re-route", doneByok.Host)
	}
	// byokHost recorded by the mock must match doneByok.Host.
	byokHostMu.Lock()
	gotByokHost := byokHost
	byokHostMu.Unlock()
	if gotByokHost == "" {
		t.Fatal("BYOK mock recorded no request host")
	}
	if !strings.Contains(doneByok.Host, gotByokHost) {
		t.Fatalf("doneByok host=%q does not match BYOK request host %q", doneByok.Host, gotByokHost)
	}
	tempHostMu.Lock()
	gotTempHost := tempHost
	tempHostMu.Unlock()
	if gotByokHost == gotTempHost {
		t.Fatalf("byok host (%q) == temp host (%q) — routes not distinct", gotByokHost, gotTempHost)
	}
}
