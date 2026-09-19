package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveFileMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := FileConfig{
		Version: 1,
		Providers: []ProviderProfile{
			{ID: "x", Type: "openai_compatible", Name: "X", BaseURL: "http://127.0.0.1:4112/v1", APIKey: "secret-local"},
		},
	}
	if err := SaveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "secret-local") {
		t.Fatal("expected key in file")
	}
}

func TestUpsertProviderReplaceAndMask(t *testing.T) {
	dir := t.TempDir()
	_, err := UpsertProvider(dir, ProviderProfile{
		ID: "a", Type: "openai_compatible", Name: "A", BaseURL: "https://example.com/v1", APIKey: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := UpsertProvider(dir, ProviderProfile{
		ID: "a", Type: "openai_compatible", Name: "A2", BaseURL: "https://example.com/v1", APIKey: "k2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "A2" || cfg.Providers[0].APIKey != "k2" {
		t.Fatalf("%+v", cfg.Providers)
	}
	// Preserve key when omitted.
	cfg, err = UpsertProvider(dir, ProviderProfile{
		ID: "a", Type: "openai_compatible", Name: "A3", BaseURL: "https://example.com/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers[0].APIKey != "k2" {
		t.Fatalf("key not preserved: %q", cfg.Providers[0].APIKey)
	}
}

func TestMaskBaseURL(t *testing.T) {
	got := MaskBaseURL("https://user:pass@api.example.com/v1?token=1")
	if strings.Contains(got, "pass") || strings.Contains(got, "token") || strings.Contains(got, "user") {
		t.Fatalf("leaked credentials: %q", got)
	}
	if !strings.Contains(got, "api.example.com") {
		t.Fatalf("host missing: %q", got)
	}
}

func TestSanitizeProviderID(t *testing.T) {
	if got := SanitizeProviderID("My OpenAI!"); got != "my-openai" {
		t.Fatalf("got %q", got)
	}
	if got := SanitizeProviderID(""); got != "byok" {
		t.Fatalf("empty → %q", got)
	}
}
