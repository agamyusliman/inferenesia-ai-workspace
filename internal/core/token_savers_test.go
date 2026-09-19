package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

func TestGetTokenSaversDefaultsOff(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	view := svc.GetTokenSavers()
	if view.Product == "" || !strings.Contains(view.Product, "Inferenesia") {
		t.Fatalf("product brand: %q", view.Product)
	}
	if len(view.Savers) != 4 {
		t.Fatalf("want 4 savers, got %d", len(view.Savers))
	}
	wantIDs := []string{"rtk", "headroom", "caveman", "ponytail"}
	for i, id := range wantIDs {
		if view.Savers[i].ID != id {
			t.Fatalf("saver[%d] id = %q, want %q", i, view.Savers[i].ID, id)
		}
		if view.Savers[i].Enabled {
			t.Fatalf("%s should default off", id)
		}
		if view.Savers[i].Description == "" || view.Savers[i].Name == "" {
			t.Fatalf("%s missing name/description", id)
		}
	}
	// RTK/Headroom typically not installed in CI → not_installed status.
	for _, s := range view.Savers {
		if s.ID == "rtk" || s.ID == "headroom" {
			if s.Status != "not_installed" && s.Installed {
				// installed is fine if binary exists on host
				continue
			}
			if !s.Installed && s.Status != "not_installed" {
				t.Fatalf("%s status=%q installed=%v", s.ID, s.Status, s.Installed)
			}
		}
		if s.ID == "caveman" || s.ID == "ponytail" {
			if !s.Installed || s.Status != "ready" {
				t.Fatalf("%s should be ready (prompt bias): %+v", s.ID, s)
			}
		}
	}
}

// saverEnabled is a literal helper for the optional Enabled field.
func saverEnabled(v bool) *bool { return &v }

func TestSetTokenSaverPersistsAndIndependent(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}

	// Enable missing RTK — must not error (VAL-SAVER-007).
	v, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "rtk", Enabled: saverEnabled(true)})
	if err != nil {
		t.Fatalf("enable missing rtk: %v", err)
	}
	var rtk TokenSaverStatus
	for _, s := range v.Savers {
		if s.ID == "rtk" {
			rtk = s
		}
		if s.ID == "headroom" && s.Enabled {
			t.Fatal("headroom should stay off when only rtk toggled")
		}
	}
	if !rtk.Enabled {
		t.Fatal("rtk should be enabled")
	}
	// Missing binary still not_installed.
	if rtk.Installed && rtk.Status != "ready" {
		// host may have rtk — OK
	} else if !rtk.Installed && rtk.Status != "not_installed" {
		t.Fatalf("missing rtk status: %+v", rtk)
	}

	// Enable caveman independently.
	v2, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "caveman", Enabled: saverEnabled(true)})
	if err != nil {
		t.Fatal(err)
	}
	var cav, pon TokenSaverStatus
	for _, s := range v2.Savers {
		switch s.ID {
		case "caveman":
			cav = s
		case "ponytail":
			pon = s
		case "rtk":
			if !s.Enabled {
				t.Fatal("rtk should remain enabled")
			}
		}
	}
	if !cav.Enabled || pon.Enabled {
		t.Fatalf("caveman on / ponytail off expected: cav=%+v pon=%+v", cav, pon)
	}

	// Persist across Service reload (simulates app restart).
	svc2, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	reloaded := svc2.GetTokenSavers()
	on := map[string]bool{}
	for _, s := range reloaded.Savers {
		on[s.ID] = s.Enabled
	}
	if !on["rtk"] || !on["caveman"] || on["headroom"] || on["ponytail"] {
		t.Fatalf("persist mismatch: %+v", on)
	}

	// Config file mode 0600.
	st, err := os.Stat(filepath.Join(home, config.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
}

func TestSetTokenSaverPersistsEditableSetupAndCanClear(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	rtkCommand := "/opt/inferenesia/bin/rtk"
	headroomURL := "https://compress.example.test/custom"
	if _, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "rtk", Enabled: saverEnabled(false), Command: &rtkCommand}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "headroom", Enabled: saverEnabled(true), BaseURL: &headroomURL})
	if err != nil {
		t.Fatal(err)
	}
	for _, saver := range view.Savers {
		switch saver.ID {
		case "rtk":
			if saver.Command != rtkCommand {
				t.Fatalf("rtk command = %q", saver.Command)
			}
		case "headroom":
			if saver.BaseURL != headroomURL || !saver.Enabled {
				t.Fatalf("headroom = %+v", saver)
			}
		}
	}
	empty := ""
	cleared, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "headroom", Enabled: saverEnabled(false), BaseURL: &empty})
	if err != nil {
		t.Fatal(err)
	}
	for _, saver := range cleared.Savers {
		if saver.ID == "headroom" && saver.BaseURL != "" {
			t.Fatalf("headroom base_url not cleared: %q", saver.BaseURL)
		}
	}
}

func TestSetTokenSaverRejectsInvalidHeadroomURLWithoutChangingConfig(t *testing.T) {
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	bad := "file:///tmp/compress"
	if _, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "headroom", Enabled: saverEnabled(true), BaseURL: &bad}); err == nil {
		t.Fatal("expected invalid Headroom URL to be rejected")
	}
	view := svc.GetTokenSavers()
	for _, saver := range view.Savers {
		if saver.ID == "headroom" && (saver.Enabled || saver.BaseURL != "") {
			t.Fatalf("invalid update changed Headroom: %+v", saver)
		}
	}
}

// Saving a setup value is a separate action from toggling. Omitting enabled
// must leave the toggle exactly as persisted — previously the zero-value bool
// silently disabled an enabled saver whenever the user saved its command/URL.
func TestSetTokenSaverSetupSaveKeepsToggleState(t *testing.T) {
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "rtk", Enabled: saverEnabled(true)}); err != nil {
		t.Fatal(err)
	}

	command := "/opt/inferenesia/bin/rtk"
	view, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "rtk", Command: &command})
	if err != nil {
		t.Fatal(err)
	}
	for _, saver := range view.Savers {
		if saver.ID != "rtk" {
			continue
		}
		if !saver.Enabled {
			t.Fatal("setup-only save disabled an enabled saver")
		}
		if saver.Command != command {
			t.Fatalf("rtk command = %q, want %q", saver.Command, command)
		}
	}
	if !svc.TokenSaversConfig().IsEnabled("rtk") {
		t.Fatal("persisted config lost the enabled toggle")
	}
}

// A setup field that belongs to another saver is a malformed request; applying
// it would write a value the caller cannot see on the row it edited.
func TestSetTokenSaverRejectsMismatchedSetupField(t *testing.T) {
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	url := "https://compress.example.test"
	if _, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "rtk", BaseURL: &url}); err == nil {
		t.Fatal("expected base_url on rtk to be rejected")
	}
	for _, saver := range svc.GetTokenSavers().Savers {
		if saver.ID == "headroom" && saver.BaseURL != "" {
			t.Fatalf("rejected request still wrote headroom base_url: %q", saver.BaseURL)
		}
	}
}

func TestSetTokenSaverUnknown(t *testing.T) {
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetTokenSaver(SetTokenSaverRequest{ID: "magic", Enabled: saverEnabled(true)}); err == nil {
		t.Fatal("expected error for unknown saver")
	}
}

func TestTokenSaversHTTP(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(Options{ConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/token-savers", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status %d body %s", rr.Code, rr.Body.String())
	}
	var getView TokenSaversView
	if err := json.Unmarshal(rr.Body.Bytes(), &getView); err != nil {
		t.Fatal(err)
	}
	if len(getView.Savers) != 4 {
		t.Fatalf("got %d savers", len(getView.Savers))
	}

	// Toggle ponytail on.
	body := `{"id":"ponytail","enabled":true}`
	rr2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/token-savers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("POST status %d body %s", rr2.Code, rr2.Body.String())
	}
	var setView TokenSaversView
	if err := json.Unmarshal(rr2.Body.Bytes(), &setView); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range setView.Savers {
		if s.ID == "ponytail" && s.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("ponytail not enabled after POST: %+v", setView.Savers)
	}

	// Reload still on.
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, httptest.NewRequest(http.MethodGet, "/api/token-savers", nil))
	var again TokenSaversView
	_ = json.Unmarshal(rr3.Body.Bytes(), &again)
	for _, s := range again.Savers {
		if s.ID == "ponytail" && !s.Enabled {
			t.Fatal("ponytail not persisted via HTTP")
		}
	}
}

func TestGetSettingsIncludesTokenSaverSection(t *testing.T) {
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	view := svc.GetSettings("")
	found := false
	for _, sec := range view.Sections {
		if sec.ID == "token-savers" {
			found = true
			if !strings.Contains(strings.ToLower(sec.Title), "token") {
				t.Fatalf("title: %q", sec.Title)
			}
		}
	}
	if !found {
		t.Fatalf("settings sections missing token-savers: %+v", view.Sections)
	}
	if view.TokenSavers == nil || len(view.TokenSavers.Savers) != 4 {
		t.Fatalf("TokenSavers embedded view missing: %+v", view.TokenSavers)
	}
}
