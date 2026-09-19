package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/plan"
	"github.com/agamyusliman/inferenesia-app/internal/workspace"
)

// DevPortMin/Max confine any local HTTP debug/dev servers to mission ports.
const (
	DevPortMin = 4100
	DevPortMax = 4199
	// DefaultDevPort is the preferred desktop/dev hub HTTP port.
	DefaultDevPort = 4110
)

// ValidateDevPort returns an error if port is outside 4100–4199.
func ValidateDevPort(port int) error {
	if port < DevPortMin || port > DevPortMax {
		return fmt.Errorf("core: port %d outside allowed range %d-%d", port, DevPortMin, DevPortMax)
	}
	return nil
}

// Handler returns an http.Handler exposing hub APIs for desktop dev and agent-browser.
// Optional staticFS (SPA dist) is served for non-/api paths when non-nil.
// Routes (JSON / SSE):
//
//	GET  /api/health
//	GET  /api/hub
//	GET  /api/workspaces
//	POST /api/workspaces/open   {"path":"/abs"}
//	POST|GET /api/workspaces/browse  — native folder picker {path, cancelled}
//	POST /api/workspaces/switch {"id":"ws_…"}
//	GET  /api/fs/list?rel=&workspace_id=
//	POST /api/fs/select         {"path":"rel/file"}
//	GET  /api/fs/read?path=&workspace_id=
//	POST /api/fs/write          {"workspace_id","path","content"}  — editor save (user edit)
//	POST /api/fs/create-file    {"workspace_id","parent","name"}
//	POST /api/fs/create-folder  {"workspace_id","parent","name"}
//	POST /api/fs/rename         {"workspace_id","path","new_name"}
//	POST /api/fs/delete         {"workspace_id","path"}
//	POST /api/fs/copy           {"workspace_id","src","dest_parent"}  — clipboard paste (copy)
//	POST /api/fs/move           {"workspace_id","src","dest_parent"}  — clipboard paste (cut)
//	GET  /api/fs/find?workspace_id=&folder=&q=&limit=  — Find in Folder scoped search
//	GET  /api/fs/resolve?path=&workspace_id=
//	POST /api/fs/reveal         {"workspace_id","path"}
//	GET  /api/fs/events         SSE FileChangeEvent push (external CLI/OS writes; VAL-CROSS-003)
//	POST /api/workspaces/remove {"id"}
//	POST /api/workspaces/add-folder {"workspace_id","path"}
//	POST /api/chat/stream       SSE TokenDelta via core.Service (no TS LLM)
//	GET  /api/chat/session
//	DELETE /api/chat/session
//	GET  /api/chat/history?workspace_id=  — durable one thread per workspace (VAL-DESK-014)
//	POST /api/chat/message/delete   {"workspace_id","message_id"}  — single message delete (VAL-CHAT-018)
//	POST /api/chat/message/truncate {"workspace_id","message_id"}  — undo-from-here (VAL-CHAT-017)
//	POST /api/chat/history          {"workspace_id","messages"}    — replace durable thread
//	GET  /api/mentions?q=
//	GET  /api/undo/stack
//	POST /api/undo | /api/redo | /api/revert-agent
//	POST /api/restore-to-head   {"path":"rel"}  — git HEAD, not Revert agent
//	GET  /api/agent-changes     — WriteGateway agent-change timeline (not git history; VAL-DESK-012)
//	GET  /api/settings?profile=
//	GET  /api/token-savers        — Token Saver toggles + install status (VAL-SAVER-001)
//	POST /api/token-savers        {"id","enabled"} — persist toggle in ~/.inferenesia (VAL-SAVER-002/007)
//	POST /api/providers           — add BYOK openai_compatible profile (VAL-PROV-003)
//	POST /api/providers/test      {"profile":"tempai"}  — minimal chat completion (VAL-PROV-004)
//	GET  /api/session/active      — mid-session active provider/model (VAL-PROV-005)
//	POST /api/session/active      — switch active provider/model mid-session (VAL-PROV-005/011)
//	GET  /api/onboarding          — first-run dual path state (VAL-PROV-010)
//	POST /api/onboarding/complete {"path":"tempai"|"byok"}
//	POST /api/terminal/start   {"cwd","workspace_id","title"}  — integrated PTY shell
//	GET  /api/terminal/list
//	GET  /api/terminal/read?id=
//	POST /api/terminal/write   {"id","data"}
//	POST /api/terminal/resize  {"id","cols","rows"}
//	POST /api/terminal/stop    {"id"}
//	GET  /api/terminal/stream?id=  — SSE live PtyOutput chunks
//	GET  /api/snippets/terminal                — global terminal snippets (~/.inferenesia)
//	POST /api/snippets/terminal                {"name","body"} or {"id","name","body"}
//	POST /api/snippets/terminal/delete         {"id"}
//	POST /api/canvas/ai                        {"prompt","mode","existing_elements_json"?,"profile"?,"model"?} — D5b canvas AI generate/edit (non-stream, no-tools, no chat history)
//	GET  /api/git/repos                        — discover nested git repos (multi source-control)
//	GET  /api/git/status?repo_id=              — branch + porcelain files (VAL-GIT-001)
//	GET  /api/git/diff?repo_id=&path=&staged=  — file diff (VAL-GIT-002)
//	GET  /api/git/graph?repo_id=&limit=        — commit graph for Graph tab
//	POST /api/git/stage    {"repo_id","path"}  — stage (VAL-GIT-003)
//	POST /api/git/unstage  {"repo_id","path"}  — unstage (VAL-GIT-003)
//	POST /api/git/commit   {"repo_id","message"} — commit index (VAL-GIT-004)
//	POST /api/git/fetch    {"repo_id"}          — fetch (VAL-GIT-010)
//	POST /api/git/pull     {"repo_id"}          — pull (VAL-GIT-010)
//	POST /api/git/push     {"repo_id","confirmed", "force?"} — push (VAL-GIT-011); force gated (VAL-GIT-007)
//	POST /api/git/destructive {"repo_id","kind","ref","confirmed"} — reset_hard/branch_delete (VAL-GIT-006)
//	GET  /api/approvals/pending               — open NeedsApproval request
//	POST /api/approvals/resolve {"id","approved"} — resolve NeedsApproval (VAL-GIT-005+)
//	GET  /api/git/branches?repo_id=            — local/remote branches (checkout)
//	POST /api/git/checkout {"repo_id","name","create?"} — switch/create branch
//	GET  /api/git/workflow                     — per-workspace git/deploy rules
//	POST /api/git/workflow                     — save workflow YAML
//	POST /api/git/workflow/ensure              — write default file if missing
//	GET  /api/git/actions?repo_id=&limit=      — GitHub Actions runs (VAL-GHA)
//	POST /api/git/actions/open {"url"}        — open run on github.com (VAL-GHA-004)
//	GET  /api/plan/mode                        — Plan Mode snapshot (VAL-PLAN-001)
//	POST /api/plan/enter                       {"reason"?} — enter read-only Plan Mode
//	POST /api/plan/exit                        {"reason"?} — exit Plan Mode (approve|reject|cancel|user)
//	GET  /api/plan/pending                     — pending PlanProposed (VAL-PLAN-003)
//	POST /api/plan/propose                     {"title","summary","body","steps":[]} — seed PlanProposed (tools / hub / tests)
//	POST /api/plan/approve                     — approve seeds todos pending (VAL-PLAN-003)
//	POST /api/plan/approve-and-mission         — approve + create Mission from plan (VAL-CROSS-004)
//	POST /api/plan/reject                      — reject discards, no todos (VAL-PLAN-004)
//	GET  /api/todos                            — session todo checklist (VAL-PLAN-005)
//	POST /api/todos                            — replace full list (optional UI/agent)
//	POST /api/todos/update                     {"id","status"?,"content"?}
//	GET  /api/missions                         — mission list (VAL-MISSION-001)
//	POST /api/missions                         {"goal","budget_tokens"?} — create mission
//	GET  /api/missions/get?id=                 — one mission
//	POST /api/missions/activate                {"id"}
//	POST /api/missions/block                   {"id","reason"} — blocked status (VAL-MISSION-004)
//	POST /api/missions/evidence                {"id","summary","content"?,"command"?} (VAL-MISSION-003)
//	POST /api/missions/complete                {"id"} — evidence required (VAL-MISSION-002)
//	POST /api/missions/gates                   {"id","gates"?[]} — run verification gates (VAL-MISSION-005)
//	GET  /api/tasks                            — orchestrator task list for panel (VAL-ORCH-010)
//	POST /api/tasks/cancel                     {"id"} — cancel by task_id (VAL-ORCH-006)
//	POST /api/tasks/resume                     {"id"} — resume cancelled/failed (VAL-ORCH-007)
//	GET  /api/tasks/categories                 — enumerate categories (VAL-ORCH-008)
//	GET  /api/browser/status                   — BrowserStatus lifecycle snapshot (VAL-BRW-008)
//	POST /api/browser/set-engine               {"engine":"e2e"} — select/launch engine
//	POST /api/browser/navigate                 {"url":"…"}
//	POST /api/browser/close                    — stop active engine
func (s *Service) Handler() http.Handler {
	return s.HandlerWithStatic(nil)
}

// HandlerWithStatic is like Handler but also serves static SPA files from staticFS.
func (s *Service) HandlerWithStatic(staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"product": s.BrandName(),
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/api/hub", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.GetHubState())
	})
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.ListWorkspaces())
	})
	mux.HandleFunc("/api/workspaces/open", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ws, err := s.OpenWorkspace(body.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspace": ws, "hub": s.GetHubState()})
	})
	mux.HandleFunc("/api/workspaces/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Name   string `json:"name"`
			Create bool   `json:"create"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var (
			ws  workspace.Workspace
			err error
		)
		if body.Create || strings.TrimSpace(body.Name) != "" {
			ws, err = s.CreateSessionWorkspace(body.Name)
		} else {
			ws, err = s.EnsureSessionWorkspace()
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspace": ws, "hub": s.GetHubState()})
	})
	mux.HandleFunc("/api/workspaces/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ws, err := s.RenameWorkspace(body.ID, body.Name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspace": ws, "hub": s.GetHubState()})
	})
	mux.HandleFunc("/api/workspaces/browse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path, err := s.PickDirectory()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": path, "cancelled": path == ""})
	})
	mux.HandleFunc("/api/workspaces/switch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ws, err := s.SwitchWorkspace(body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspace": ws, "hub": s.GetHubState()})
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rel := r.URL.Query().Get("rel")
		wid := r.URL.Query().Get("workspace_id")
		entries, err := s.ListDir(wid, rel)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, entries)
	})
	mux.HandleFunc("/api/fs/select", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		hub, err := s.SelectFile(body.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, hub)
	})
	mux.HandleFunc("/api/fs/read", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Query().Get("path")
		wid := r.URL.Query().Get("workspace_id")
		text, err := s.ReadFile(wid, path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"path": path, "content": text})
	})
	mux.HandleFunc("/api/fs/write", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Path        string `json:"path"`
			Content     string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.SaveFile(body.WorkspaceID, body.Path, body.Content)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// --- Explorer FS mutations (UI Pack A; route via core.Service / WriteGateway) ---
	mux.HandleFunc("/api/fs/create-file", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Parent      string `json:"parent"`
			Name        string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.CreateFile(body.WorkspaceID, body.Parent, body.Name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/fs/create-folder", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Parent      string `json:"parent"`
			Name        string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.CreateFolder(body.WorkspaceID, body.Parent, body.Name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/fs/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Path        string `json:"path"`
			NewName     string `json:"new_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.RenamePath(body.WorkspaceID, body.Path, body.NewName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/fs/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Path        string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.DeletePath(body.WorkspaceID, body.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	// Explorer clipboard paste — copy creates duplicate; move is cut+paste (VAL-IDE-013).
	mux.HandleFunc("/api/fs/copy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Src         string `json:"src"`
			DestParent  string `json:"dest_parent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.CopyPath(body.WorkspaceID, body.Src, body.DestParent)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/fs/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Src         string `json:"src"`
			DestParent  string `json:"dest_parent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.MovePath(body.WorkspaceID, body.Src, body.DestParent)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	// Find in Folder — results scoped under selected folder only (VAL-IDE-008).
	mux.HandleFunc("/api/fs/find", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		wid := r.URL.Query().Get("workspace_id")
		folder := r.URL.Query().Get("folder")
		q := r.URL.Query().Get("q")
		limit := 0
		if lim := r.URL.Query().Get("limit"); lim != "" {
			if n, err := strconv.Atoi(lim); err == nil {
				limit = n
			}
		}
		hits, err := s.FindInFolder(wid, folder, q, limit)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, hits)
	})
	mux.HandleFunc("/api/fs/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Query().Get("path")
		wid := r.URL.Query().Get("workspace_id")
		info, err := s.ResolvePath(wid, path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	})
	mux.HandleFunc("/api/fs/reveal", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Path        string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.RevealInOS(body.WorkspaceID, body.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	// --- File change events SSE (VAL-CROSS-003) ---
	// GET /api/fs/events streams FileChangeEvent values pushed from the fsnotify
	// watcher when an external process (CLI agent, OS, editor) mutates a file
	// in the active workspace. The desktop explorer subscribes and refreshes
	// on push, not via polling. Source="external" for CLI/OS writes;
	// Source="internal" for desktop WriteGateway echoes.
	mux.HandleFunc("/api/fs/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ctx := r.Context()
		ch := s.FileWatcherSubscribe()
		defer s.FileWatcherUnsubscribe(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					// Watcher closed (workspace closed / service shutdown).
					return
				}
				payload, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("/api/workspaces/remove", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.RemoveWorkspace(body.ID); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "hub": s.GetHubState()})
	})
	mux.HandleFunc("/api/workspaces/add-folder", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			Path        string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ws, err := s.AddFolderToWorkspace(body.WorkspaceID, body.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspace": ws, "hub": s.GetHubState()})
	})

	// --- Chat (TokenDelta SSE via core.Service; no renderer LLM) ---
	// Detached run: agent continues after browser disconnect/reload/navigation.
	// Explicit Stop: POST /api/chat/cancel {run_id}.
	mux.HandleFunc("/api/chat/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		runID, events, unsub, err := s.StartChatRun(body)
		if err != nil {
			payload, _ := json.Marshal(ChatEvent{Type: ChatEventError, Error: err.Error()})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
			return
		}
		defer unsub()
		_ = runID

		reqCtx := r.Context()
		for {
			select {
			case <-reqCtx.Done():
				// Client left (nav/reload/close). Keep agent running; drop this subscriber only.
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				payload, merr := json.Marshal(ev)
				if merr != nil {
					continue
				}
				if _, werr := fmt.Fprintf(w, "data: %s\n\n", payload); werr != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("/api/chat/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			RunID string `json:"run_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ok := s.CancelChatRun(body.RunID)
		writeJSON(w, http.StatusOK, map[string]any{
			"cancelled": ok,
			"run_id":    body.RunID,
		})
	})
	mux.HandleFunc("/api/chat/session", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetChatSession())
		case http.MethodDelete:
			s.ClearChatSession()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// Durable per-workspace chat history (one active thread; VAL-DESK-014).
	mux.HandleFunc("/api/chat/history", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			wid := r.URL.Query().Get("workspace_id")
			hist, err := s.GetChatHistory(wid)
			if err != nil {
				// Still return empty payload with error for missing workspace.
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, hist)
		case http.MethodPost:
			// Full replace for durable thread (optional bulk path for undo/delete clients).
			var body struct {
				WorkspaceID string               `json:"workspace_id"`
				Messages    []DesktopChatMessage `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			hist, err := s.SetChatHistory(body.WorkspaceID, body.Messages)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, hist)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// Single-message delete (VAL-CHAT-018) — no confirm gate (UI shows clear feedback).
	mux.HandleFunc("/api/chat/message/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			MessageID   string `json:"message_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.DeleteChatMessage(body.WorkspaceID, body.MessageID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	// Undo-from-here: truncate from message id to end, durable (VAL-CHAT-017).
	mux.HandleFunc("/api/chat/message/truncate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			WorkspaceID string `json:"workspace_id"`
			MessageID   string `json:"message_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.TruncateChatFromHere(body.WorkspaceID, body.MessageID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// --- Mentions (@file picker) ---
	mux.HandleFunc("/api/mentions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query().Get("q")
		limit := 0
		if lim := r.URL.Query().Get("limit"); lim != "" {
			if n, err := strconv.Atoi(lim); err == nil {
				limit = n
			}
		}
		files, err := s.ListMentionFiles(q, limit)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, files)
	})

	// --- Undo toolbar (pre-agent dirty) vs Restore to HEAD ---
	mux.HandleFunc("/api/undo/stack", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		st, err := s.GetUndoStack()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("/api/undo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := s.UndoLast()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/redo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := s.RedoLast()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/revert-agent", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := s.RevertAgent()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/restore-to-head", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.RestoreToHEAD(body.Path)
		if err != nil {
			// Still return JSON body with labels for UI evidence.
			writeJSON(w, http.StatusBadRequest, res)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	mux.HandleFunc("/api/agent-changes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		hist, err := s.GetAgentChangeHistory()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, hist)
	})
	mux.HandleFunc("/api/file-timeline", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Query().Get("path")
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		tl, err := s.GetFileTimeline(path, limit)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, tl)
	})
	mux.HandleFunc("/api/file-timeline/change", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		ch, err := s.GetFileTimelineChange(
			q.Get("path"),
			q.Get("source"),
			q.Get("unit_id"),
			q.Get("sha"),
		)
		if err != nil && ch.Original == "" && ch.Modified == "" {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, ch)
	})

	// --- Settings panel ---
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		profile := r.URL.Query().Get("profile")
		writeJSON(w, http.StatusOK, s.GetSettings(profile))
	})

	// --- Token Savers (M5c optional; VAL-SAVER-001/002/007) ---
	mux.HandleFunc("/api/token-savers", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetTokenSavers())
		case http.MethodPost:
			var body SetTokenSaverRequest
			// Unknown fields are a client bug: silently ignoring them would
			// report success for a setting that was never applied.
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			view, err := s.SetTokenSaver(body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, view)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// --- Live Blocks (P9-live opt-in generative UI previews) ---
	mux.HandleFunc("/api/live-blocks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetLiveBlocks())
		case http.MethodPost:
			var body SetLiveBlocksRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			view, err := s.SetLiveBlocks(body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, view)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// --- Autonomy level (off/low/medium/high) ---
	mux.HandleFunc("/api/autonomy", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetAutonomy())
		case http.MethodPost:
			var body struct {
				Level string `json:"level"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			view, err := s.SetAutonomy(body.Level)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, view)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// --- Providers (M6: profiles / BYOK / test; VAL-PROV-001..004) ---
	mux.HandleFunc("/api/providers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body AddProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		view, err := s.AddProvider(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	})
	mux.HandleFunc("/api/providers/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Profile string `json:"profile"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			// Empty body is OK — test default profile.
			if !errors.Is(err, io.EOF) {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		res := s.TestProvider(body.Profile)
		// Always 200 with structured ok/error so the UI can show the message;
		// HTTP 4xx is reserved for request parsing failures.
		writeJSON(w, http.StatusOK, res)
	})
	// Mid-session provider/model switch (VAL-PROV-005, VAL-PROV-011).
	mux.HandleFunc("/api/session/active", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetActiveSession())
		case http.MethodPost:
			var body SetActiveProviderRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			view, err := s.SetActiveProvider(body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, view)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/onboarding", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.GetOnboarding())
	})
	mux.HandleFunc("/api/onboarding/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		state, err := s.CompleteOnboarding(body.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})

	// --- Integrated terminal (internal/pty; VAL-IDE-023/024) ---
	mux.HandleFunc("/api/terminal/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body TerminalStartRequest
		// Empty body is OK (defaults to workspace root shell).
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			if !errors.Is(err, io.EOF) {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		sess, err := s.TerminalStart(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, sess)
	})
	mux.HandleFunc("/api/terminal/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.TerminalList())
	})
	mux.HandleFunc("/api/terminal/read", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		res, err := s.TerminalRead(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/terminal/write", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body TerminalWriteRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.TerminalWrite(body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/terminal/resize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body TerminalResizeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.TerminalResize(body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/terminal/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.TerminalStop(body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/terminal/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		// Seed with current capture so late subscribers still see history.
		readRes, err := s.TerminalRead(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ch, cancel, err := s.SubscribeTerminalOutput(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		defer cancel()

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		// Initial snapshot (full buffer so far).
		if snap, err := json.Marshal(map[string]any{
			"type":    "snapshot",
			"id":      id,
			"output":  readRes.Output,
			"session": readRes.Session,
		}); err == nil {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", snap)
			flusher.Flush()
		}
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, open := <-ch:
				if !open {
					return
				}
				payload, err := json.Marshal(map[string]any{
					"type":  "delta",
					"id":    id,
					"delta": chunk,
				})
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		}
	})

	// --- Git panel (VAL-GIT-001/002/003/004/010 + multi-repo + graph) ---
	mux.HandleFunc("/api/git/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		repos, err := s.ListGitRepos()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"repos": repos})
	})
	mux.HandleFunc("/api/git/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		repoID := r.URL.Query().Get("repo_id")
		st, err := s.GetGitStatus(repoID)
		if err != nil {
			// Still return body when we have a structured status (e.g. no workspace).
			if st.Message != "" {
				writeJSON(w, http.StatusOK, st)
				return
			}
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("/api/git/diff", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		repoID := r.URL.Query().Get("repo_id")
		path := r.URL.Query().Get("path")
		staged := r.URL.Query().Get("staged") == "1" || strings.EqualFold(r.URL.Query().Get("staged"), "true")
		diff, err := s.GetGitDiff(repoID, path, staged)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, diff)
	})
	mux.HandleFunc("/api/git/graph", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		repoID := r.URL.Query().Get("repo_id")
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			fmt.Sscanf(raw, "%d", &limit)
		}
		g, err := s.GetGitGraph(repoID, limit)
		if err != nil {
			if g.Message != "" {
				writeJSON(w, http.StatusOK, g)
				return
			}
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, g)
	})
	mux.HandleFunc("/api/git/stage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body GitStageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.GitStage(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, res)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/unstage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body GitStageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.GitUnstage(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, res)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/commit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body GitCommitRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.GitCommit(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, res)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/fetch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			RepoID string `json:"repo_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		res, err := s.GitFetch(body.RepoID)
		// Always return structured result so UI can show success/failure clearly.
		_ = err
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/pull", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			RepoID string `json:"repo_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		res, err := s.GitPull(body.RepoID)
		_ = err
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body GitPushRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.GitPush(body)
		_ = err
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/destructive", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body GitDestructiveRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.GitDestructive(body)
		_ = err
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/plan/mode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.GetPlanMode())
	})
	mux.HandleFunc("/api/plan/enter", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Reason string `json:"reason"`
		}
		// Empty body is OK (default reason).
		_ = json.NewDecoder(r.Body).Decode(&body)
		st, err := s.EnterPlanMode(body.Reason)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"plan": st, "hub": s.GetHubState()})
	})
	mux.HandleFunc("/api/plan/exit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		st, err := s.ExitPlanMode(body.Reason)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"plan": st, "hub": s.GetHubState()})
	})
	// Pending PlanProposed + Approve seeds todos / Reject discards (VAL-PLAN-003/004).
	mux.HandleFunc("/api/plan/pending", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := s.GetPendingPlan()
		if p == nil {
			writeJSON(w, http.StatusOK, map[string]any{"pending": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pending": p})
	})
	// Hub/dev propose path (same Service.ProposePlan as propose_plan tool / PlanProposed).
	mux.HandleFunc("/api/plan/propose", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Title   string   `json:"title"`
			Summary string   `json:"summary"`
			Body    string   `json:"body"`
			Steps   []string `json:"steps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if len(body.Steps) == 0 {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("plan: steps are required"))
			return
		}
		p, err := s.ProposePlan(body.Title, body.Summary, body.Body, body.Steps)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"pending": proposalToView(&p),
			"hub":     s.GetHubState(),
		})
	})
	mux.HandleFunc("/api/plan/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := s.ApprovePlan()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	// POST /api/plan/approve-and-mission — cross-area bridge (VAL-CROSS-004):
	// approve the pending plan, exit Plan Mode, and create a Mission fed from
	// the plan (goal/budget/acceptance criteria/plan snapshot).
	mux.HandleFunc("/api/plan/approve-and-mission", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			BudgetTokens int64  `json:"budget_tokens,omitempty"`
			GoalOverride string `json:"goal_override,omitempty"`
		}
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		res, err := s.ApprovePlanAndCreateMission(MissionFromPlanRequest{
			BudgetTokens: body.BudgetTokens,
			GoalOverride: body.GoalOverride,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/plan/reject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := s.RejectPlan()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	// Session todos panel (VAL-PLAN-003/005).
	mux.HandleFunc("/api/todos", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetTodos())
		case http.MethodPost:
			var body struct {
				Todos []plan.TodoItem `json:"todos"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			out, err := s.ReplaceTodos(body.Todos)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, TodoListView{
				Todos:   out,
				Count:   len(out),
				Product: s.BrandName(),
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/todos/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID      string  `json:"id"`
			Status  *string `json:"status"`
			Content *string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		var st *plan.TodoStatus
		if body.Status != nil {
			v := plan.TodoStatus(*body.Status)
			st = &v
		}
		item, err := s.UpdateTodo(body.ID, body.Content, st)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"todo": item, "todos": s.GetTodos()})
	})
	// Mission panel (VAL-MISSION-001..006).
	mux.HandleFunc("/api/missions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.ListMissions())
		case http.MethodPost:
			var body MissionCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			m, err := s.CreateMission(body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"mission": m, "missions": s.ListMissions()})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/missions/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		m, err := s.GetMission(r.URL.Query().Get("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, m)
	})
	mux.HandleFunc("/api/missions/activate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		m, err := s.ActivateMission(body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mission": m})
	})
	mux.HandleFunc("/api/missions/block", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		m, err := s.BlockMission(body.ID, body.Reason)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mission": m})
	})
	mux.HandleFunc("/api/missions/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		m, err := s.CancelMission(body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mission": m, "missions": s.ListMissions()})
	})
	mux.HandleFunc("/api/missions/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.DeleteMission(body.ID); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "missions": s.ListMissions()})
	})
	mux.HandleFunc("/api/missions/evidence", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body MissionEvidenceRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		m, err := s.AttachMissionEvidence(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mission": m})
	})
	mux.HandleFunc("/api/missions/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		// Always 200 with ok=false when evidence missing so UI can show the message (VAL-MISSION-002).
		res := s.CompleteMission(body.ID)
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/missions/gates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body MissionGateRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		m, err := s.RunMissionGates(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mission": m})
	})
	// Orchestrator tasks panel API (VAL-ORCH-006/007/008/010) — subscriber surface only.
	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.ListTasks())
	})
	mux.HandleFunc("/api/tasks/categories", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.TaskCategories())
	})
	mux.HandleFunc("/api/tasks/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		tv, err := s.CancelTask(body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": tv, "tasks": s.ListTasks()})
	})
	mux.HandleFunc("/api/tasks/resume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		tv, err := s.ResumeTask(r.Context(), body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": tv, "tasks": s.ListTasks()})
	})
	mux.HandleFunc("/api/approvals/pending", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := s.GetPendingApproval()
		if p == nil {
			writeJSON(w, http.StatusOK, map[string]any{"pending": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pending": p})
	})
	mux.HandleFunc("/api/approvals/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body ApprovalDecision
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.ResolveApproval(body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": body.ID, "approved": body.Approved})
	})
	mux.HandleFunc("/api/git/branches", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		list, err := s.ListGitBranches(r.URL.Query().Get("repo_id"))
		if err != nil && list.Message == "" {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("/api/git/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body GitCheckoutRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.GitCheckout(body)
		if err != nil {
			writeJSON(w, http.StatusOK, res) // clear failure for UI
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/workflow", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			wf, err := s.GetGitWorkflow()
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, wf)
		case http.MethodPost:
			var body GitWorkflow
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			wf, err := s.SaveGitWorkflow(body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, wf)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/git/workflow/ensure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		wf, err := s.EnsureGitWorkflow()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, wf)
	})
	mux.HandleFunc("/api/git/actions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		repoID := r.URL.Query().Get("repo_id")
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			fmt.Sscanf(raw, "%d", &limit)
		}
		res, err := s.GetGitActions(repoID, limit)
		// Always return structured payload so UI can render empty/guidance (no silent fail).
		_ = err
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("/api/git/actions/open", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res, err := s.OpenGitActionsRun(body.URL)
		if err != nil {
			writeJSON(w, http.StatusOK, res)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// --- Browser status panel (VAL-BRW-008) — subscriber-only, no TS browser runtime ---
	mux.HandleFunc("/api/browser/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.GetBrowserStatus())
	})
	mux.HandleFunc("/api/browser/set-engine", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Engine   string `json:"engine"`
			Headless *bool  `json:"headless"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		st, err := s.BrowserSetEngine(ctx, body.Engine, body.Headless)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"current": st, "status": s.GetBrowserStatus()})
	})
	mux.HandleFunc("/api/browser/navigate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		st, err := s.BrowserNavigate(ctx, body.URL)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"current": st, "status": s.GetBrowserStatus()})
	})
	mux.HandleFunc("/api/browser/close", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := s.BrowserClose(ctx); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, s.GetBrowserStatus())
	})

	// --- Global terminal snippet library under ~/.inferenesia (VAL-IDE-036) ---
	mux.HandleFunc("/api/snippets/terminal", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list, err := s.ListTerminalSnippets()
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, list)
		case http.MethodPost:
			var body TerminalSnippetSaveRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			sn, err := s.SaveTerminalSnippet(body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, sn)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/snippets/terminal/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body TerminalSnippetDeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		list, err := s.DeleteTerminalSnippet(body.ID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})

	// --- Canvas AI generate/edit (D5b) ---
	// POST /api/canvas/ai runs a dedicated non-stream, no-tools chat turn to
	// generate or context-aware edit an Excalidraw elements array. Does NOT use
	// ChatStream, does NOT enable tools, does NOT append to durable chat
	// history — the canvas buffer is the sole source of truth (D5b).
	mux.HandleFunc("/api/canvas/ai", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body CanvasAIRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		res := s.CanvasAI(body)
		// Always 200 with structured ok/error so the UI can show the message
		// inline under the prompt bar (consistent with /api/providers/test).
		writeJSON(w, http.StatusOK, res)
	})

	mux.HandleFunc("/api/playground/image", servePlaygroundImage)

	// --- MCP status + management (VAL-MCP-001..007) ---
	mux.HandleFunc("/api/mcp/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, s.GetMCPStatus())
	})

	mux.HandleFunc("/api/mcp/servers", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetMCPStatus())
		case http.MethodPost:
			var body struct {
				Action  string                  `json:"action"`
				Name    string                  `json:"name"`
				Enabled *bool                   `json:"enabled,omitempty"`
				Server  *config.MCPServerConfig `json:"server,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			// A server owned by an env override or project config is a
			// conflict, not a malformed request: the client asked for
			// something well-formed that this layer may not change.
			fail := func(err error) {
				if errors.Is(err, ErrMCPReadOnlyLayer) {
					writeErr(w, http.StatusConflict, err)
					return
				}
				writeErr(w, http.StatusBadRequest, err)
			}
			switch body.Action {
			case "toggle":
				if body.Enabled == nil {
					writeErr(w, http.StatusBadRequest, fmt.Errorf("enabled field required for toggle"))
					return
				}
				if err := s.ToggleMCPServer(body.Name, *body.Enabled); err != nil {
					fail(err)
					return
				}
				writeJSON(w, http.StatusOK, s.GetMCPStatus())
			case "add":
				if body.Server == nil {
					writeErr(w, http.StatusBadRequest, fmt.Errorf("server config required for add"))
					return
				}
				if err := s.AddMCPServer(body.Name, *body.Server); err != nil {
					fail(err)
					return
				}
				writeJSON(w, http.StatusOK, s.GetMCPStatus())
			case "remove":
				if err := s.RemoveMCPServer(body.Name); err != nil {
					fail(err)
					return
				}
				writeJSON(w, http.StatusOK, s.GetMCPStatus())
			case "reload":
				ws := ""
				if active := s.registry.Active(); active != nil {
					ws = active.RootPath
				}
				if err := s.ReloadMCP(ws); err != nil {
					writeErr(w, http.StatusBadRequest, err)
					return
				}
				writeJSON(w, http.StatusOK, s.GetMCPStatus())
			default:
				writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown action %q (use toggle|add|remove|reload)", body.Action))
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	if staticFS != nil {
		fileServer := http.FileServer(http.FS(staticFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			// SPA fallback: try file, else index.html
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			if f, err := staticFS.Open(path); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			// Fallback index for client routes
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/index.html"
			fileServer.ServeHTTP(w, r2)
		})
	}
	return withCORS(mux)
}

// ListenAndServe starts the hub HTTP API on 127.0.0.1:port (must be 4100–4199).
func (s *Service) ListenAndServe(port int) (string, error) {
	return s.ListenAndServeStatic(port, nil)
}

// ListenAndServeStatic starts hub API (+ optional SPA) on 127.0.0.1:port (4100–4199).
func (s *Service) ListenAndServeStatic(port int, staticFS fs.FS) (string, error) {
	if err := ValidateDevPort(port); err != nil {
		return "", err
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("core: listen %s: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           s.HandlerWithStatic(staticFS),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	return "http://" + addr, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Allow localhost Vite dev servers in 4100–4199 only.
		if origin != "" && isLocalDevOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalDevOrigin(origin string) bool {
	// http://localhost:41xx or http://127.0.0.1:41xx
	if !strings.HasPrefix(origin, "http://localhost:") && !strings.HasPrefix(origin, "http://127.0.0.1:") {
		return false
	}
	parts := strings.Split(origin, ":")
	if len(parts) < 3 {
		return false
	}
	p, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return false
	}
	return p >= DevPortMin && p <= DevPortMax
}
