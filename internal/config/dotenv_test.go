package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvDoesNotOverrideExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".env")
	body := "FOO_TEST_VAR=fromfile\nBAR_TEST_VAR=barval\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOO_TEST_VAR", "already")
	t.Setenv("BAR_TEST_VAR", "") // empty: should be filled

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("FOO_TEST_VAR"); got != "already" {
		t.Fatalf("overrode existing FOO_TEST_VAR: %q", got)
	}
	if got := os.Getenv("BAR_TEST_VAR"); got != "barval" {
		t.Fatalf("BAR_TEST_VAR = %q, want barval", got)
	}
}

func TestLoadDotEnvMissingIsNoop(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "no-such.env")); err != nil {
		t.Fatalf("missing file should be nil, got %v", err)
	}
}

func TestLoadDotEnvIgnoresCommentsAndExport(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".env")
	body := "# comment\nexport QUIX_TEST_VAR=\"quoted\"\n\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUIX_TEST_VAR", "")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("QUIX_TEST_VAR"); got != "quoted" {
		t.Fatalf("QUIX_TEST_VAR = %q, want quoted", got)
	}
}
