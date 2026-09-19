package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLiveBlocksOff(t *testing.T) {
	d := DefaultLiveBlocks()
	if d.Enabled {
		t.Fatalf("Live Blocks must default off: %+v", d)
	}
}

func TestLiveBlocksPersistRoundTrip(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}
	cfg := LiveBlocksConfig{Enabled: true}
	if _, err := SaveLiveBlocks(home, cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ConfigFileName)
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 0600", st.Mode().Perm())
	}

	loaded, err := LoadLiveBlocks(home)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Enabled {
		t.Fatalf("enabled flag not persisted: %+v", loaded)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "live_blocks:") {
		t.Fatalf("config missing live_blocks block:\n%s", raw)
	}
	if strings.Contains(string(raw), "TEMP_AI_API_KEY=") {
		t.Fatal("config must not embed env secret values")
	}
}

func TestLoadLiveBlocksAbsentDefaultsOff(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHomeAt(home); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLiveBlocks(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Enabled {
		t.Fatalf("absent config must default off: %+v", loaded)
	}
}

func TestIsLiveBlockLanguage(t *testing.T) {
	cases := []struct {
		lang string
		want bool
	}{
		{"live-block", true},
		{"LIVE-BLOCK", true},
		{"  live-block ", true},
		{"yura-live", true},
		{"Yura-Live", true},
		{"html:preview", true},
		{"HTML:Preview", true},
		{"html", false},
		{"", false},
		{"mermaid", false},
		{"javascript", false},
		{"liveblock", false},
		{"yuralive", false},
	}
	for _, c := range cases {
		if got := IsLiveBlockLanguage(c.lang); got != c.want {
			t.Errorf("IsLiveBlockLanguage(%q) = %v, want %v", c.lang, got, c.want)
		}
	}
}
