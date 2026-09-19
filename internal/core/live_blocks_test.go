package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetLiveBlocksDefaultsOff(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	view := svc.GetLiveBlocks()
	if view.Enabled {
		t.Fatalf("Live Blocks must default off: %+v", view)
	}
	if !strings.Contains(view.Product, "Inferenesia") {
		t.Fatalf("product brand: %q", view.Product)
	}
	if view.Note == "" {
		t.Fatal("note should be populated for UI")
	}
}

func TestSetLiveBlocksPersists(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.SetLiveBlocks(SetLiveBlocksRequest{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Enabled {
		t.Fatal("should be enabled after set")
	}

	// Persist across Service reload.
	svc2, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if got := svc2.GetLiveBlocks(); !got.Enabled {
		t.Fatalf("not persisted: %+v", got)
	}

	// Config file mode 0600.
	st, err := os.Stat(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}

	// Turn back off.
	v2, err := svc.SetLiveBlocks(SetLiveBlocksRequest{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if v2.Enabled {
		t.Fatal("should be off after disable")
	}
	if got := svc.GetLiveBlocks(); got.Enabled {
		t.Fatal("disable not persisted")
	}
}

func TestLiveBlocksHTTP(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/live-blocks", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status %d body %s", rr.Code, rr.Body.String())
	}
	var getView LiveBlocksView
	if err := json.Unmarshal(rr.Body.Bytes(), &getView); err != nil {
		t.Fatal(err)
	}
	if getView.Enabled {
		t.Fatal("default must be off")
	}

	body := `{"enabled":true}`
	rr2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/live-blocks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("POST status %d body %s", rr2.Code, rr2.Body.String())
	}
	var setView LiveBlocksView
	if err := json.Unmarshal(rr2.Body.Bytes(), &setView); err != nil {
		t.Fatal(err)
	}
	if !setView.Enabled {
		t.Fatal("should be enabled after POST")
	}

	// Reload still on.
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, httptest.NewRequest(http.MethodGet, "/api/live-blocks", nil))
	var again LiveBlocksView
	_ = json.Unmarshal(rr3.Body.Bytes(), &again)
	if !again.Enabled {
		t.Fatal("not persisted via HTTP")
	}
}

func TestGetSettingsIncludesLiveBlocksSection(t *testing.T) {
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	view := svc.GetSettings("")
	found := false
	for _, sec := range view.Sections {
		if sec.ID == "live-blocks" {
			found = true
			if !strings.Contains(strings.ToLower(sec.Title), "live") {
				t.Fatalf("title: %q", sec.Title)
			}
		}
	}
	if !found {
		t.Fatalf("settings sections missing live-blocks: %+v", view.Sections)
	}
	if view.LiveBlocks == nil {
		t.Fatal("LiveBlocks embedded view missing")
	}
}

func TestLiveBlocksSystemFragmentOnlyWhenEnabled(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	// Off by default → fragment not in system prompt path. We can't easily
	// inspect sysForRun directly; assert the config-driven branch instead.
	if svc.LiveBlocksConfig().Enabled {
		t.Fatal("default must be off")
	}
	if _, err := svc.SetLiveBlocks(SetLiveBlocksRequest{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if !svc.LiveBlocksConfig().Enabled {
		t.Fatal("should be on after set")
	}
	// Fragment constant sanity: short, no secrets, mentions fence + CDN.
	if !strings.Contains(LiveBlocksSystemFragment, "live-block") {
		t.Fatal("fragment must mention live-block fence")
	}
	if !strings.Contains(LiveBlocksSystemFragment, "cdn.jsdelivr.net") {
		t.Fatal("fragment must mention CDN allowlist")
	}
	if !strings.Contains(LiveBlocksSystemFragment, "secrets") {
		t.Fatal("fragment must warn about secrets")
	}
	if !strings.Contains(WebPreviewSystemFragment, "Web Preview") {
		t.Fatal("web preview fragment missing title")
	}
	if !strings.Contains(WebPreviewSystemFragment, "http.server") {
		t.Fatal("web preview fragment must discourage user-run http.server")
	}
}
