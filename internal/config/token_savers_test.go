package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultTokenSaversAllOff(t *testing.T) {
	d := DefaultTokenSavers()
	if d.RTK.Enabled || d.Headroom.Enabled || d.Caveman.Enabled || d.Ponytail.Enabled {
		t.Fatalf("defaults must be off: %+v", d)
	}
}

func TestTokenSaversPersistRoundTrip(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultTokenSavers()
	cfg.RTK.Enabled = true
	cfg.Caveman.Enabled = true
	cfg.Headroom.BaseURL = "http://127.0.0.1:4120"
	if _, err := SaveTokenSavers(home, cfg); err != nil {
		t.Fatal(err)
	}
	// File mode 0600 (secrets hygiene when config may hold keys).
	path := filepath.Join(home, ConfigFileName)
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 0600", st.Mode().Perm())
	}

	loaded, err := LoadTokenSavers(home)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.RTK.Enabled || !loaded.Caveman.Enabled {
		t.Fatalf("enabled flags not persisted: %+v", loaded)
	}
	if loaded.Headroom.Enabled {
		t.Fatal("headroom should stay off")
	}
	if loaded.Headroom.BaseURL != "http://127.0.0.1:4120" {
		t.Fatalf("base_url = %q", loaded.Headroom.BaseURL)
	}
	if loaded.Ponytail.Enabled {
		t.Fatal("ponytail should stay off")
	}

	// Reload via LoadFromDir and confirm YAML contains token_savers.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "token_savers:") {
		t.Fatalf("config missing token_savers block:\n%s", raw)
	}
	if strings.Contains(string(raw), "TEMP_AI_API_KEY=") {
		t.Fatal("config must not embed env secret values")
	}
}

func TestSetEnabledIndependent(t *testing.T) {
	c := DefaultTokenSavers()
	c, err := c.SetEnabled(TokenSaverPonytail, true)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Ponytail.Enabled || c.RTK.Enabled || c.Headroom.Enabled || c.Caveman.Enabled {
		t.Fatalf("only ponytail should be on: %+v", c)
	}
	if _, err := c.SetEnabled("nope", true); err == nil {
		t.Fatal("expected error for unknown saver")
	}
}

func TestProbeRTKMissing(t *testing.T) {
	ok, detail := ProbeRTKAvailable("rtk-binary-that-does-not-exist-xyz")
	if ok {
		t.Fatalf("expected not available, got %s", detail)
	}
	if detail == "" {
		t.Fatal("expected detail")
	}
}

func TestProbeHeadroomMissingURL(t *testing.T) {
	ok, detail := ProbeHeadroomAvailable("")
	if ok {
		t.Fatal("empty base_url must be unavailable")
	}
	if detail == "" {
		t.Fatal("expected detail")
	}
	ok, _ = ProbeHeadroomAvailable("http://127.0.0.1:4120")
	if !ok {
		t.Fatal("configured base_url should count as available")
	}
}

func TestLoadTokenSaversAbsentDefaultsOff(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}
	// defaultConfigYAML has no token_savers key.
	loaded, err := LoadTokenSavers(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RTK.Enabled || loaded.Headroom.Enabled || loaded.Caveman.Enabled || loaded.Ponytail.Enabled {
		t.Fatalf("absent config must default off: %+v", loaded)
	}
}
