package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileParsesProviders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const body = `
version: 1
default_provider: byok-local
providers:
  - id: byok-local
    type: openai_compatible
    name: Local BYOK
    base_url: http://127.0.0.1:4111/v1
    api_key_env: MY_BYOK_KEY
    default_model: mock-model
  - id: tempai
    type: openai_compatible
    name: temp-ai
    base_url: https://ai.temp.web.id/v1
    api_key_env: TEMP_AI_API_KEY
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProvider != "byok-local" {
		t.Fatalf("default_provider=%q", cfg.DefaultProvider)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("len(providers)=%d", len(cfg.Providers))
	}
	if cfg.Providers[0].BaseURL != "http://127.0.0.1:4111/v1" {
		t.Fatalf("base_url=%q", cfg.Providers[0].BaseURL)
	}
	t.Setenv("MY_BYOK_KEY", "byok-secret-not-real")
	if got := cfg.Providers[0].ResolvedAPIKey(); got != "byok-secret-not-real" {
		t.Fatalf("ResolvedAPIKey=%q", got)
	}
}

func TestLoadFileMissingReturnsEmpty(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProvider != "" || len(cfg.Providers) != 0 {
		t.Fatalf("want empty, got %+v", cfg)
	}
}

func TestResolvedAPIKeyPrefersInline(t *testing.T) {
	t.Setenv("SOME_KEY_ENV", "from-env")
	p := ProviderProfile{APIKey: "inline-key", APIKeyEnv: "SOME_KEY_ENV"}
	if got := p.ResolvedAPIKey(); got != "inline-key" {
		t.Fatalf("got %q", got)
	}
}
