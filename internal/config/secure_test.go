package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteSecureFileMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix mode bits")
	}
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sub", "keys.yaml")
	body := []byte("api_key: sk-live-abcdefghijklmnopqrstuvwxyz\n")
	if err := WriteSecureFile(path, body); err != nil {
		t.Fatalf("WriteSecureFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != FilePerm {
		t.Fatalf("mode = %o, want %o", got, FilePerm)
	}
}

func TestEnsureSecretFilePermsTightensKeyFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix mode bits")
	}
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, DirPerm); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Non-secret: leave as 0644.
	plain := filepath.Join(home, "notes.txt")
	if err := os.WriteFile(plain, []byte("hello notes\n"), 0o644); err != nil {
		t.Fatalf("write plain: %v", err)
	}
	// Secret-like but loose perms.
	secretPath := filepath.Join(home, "config.yaml")
	secretBody := []byte("api_key: sk-live-abcdefghijklmnopqrstuvwxyz\n")
	if err := os.WriteFile(secretPath, secretBody, 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	if err := EnsureSecretFilePerms(home); err != nil {
		t.Fatalf("EnsureSecretFilePerms: %v", err)
	}

	secInfo, err := os.Stat(secretPath)
	if err != nil {
		t.Fatalf("stat secret: %v", err)
	}
	if got := secInfo.Mode().Perm(); got != FilePerm {
		t.Fatalf("secret file mode = %o, want %o", got, FilePerm)
	}
	plainInfo, err := os.Stat(plain)
	if err != nil {
		t.Fatalf("stat plain: %v", err)
	}
	if got := plainInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("plain file mode changed to %o, want 0644", got)
	}
}

// TestEnsureSecretFilePermsPrunesNonConfigDirs ensures the walk skips known
// non-config subtrees (browser engine installs, pty logs, etc.) so startup
// stays fast even when the config home contains thousands of venv files.
// Regression: previously EnsureSecretFilePerms read every file under home,
// including CloakBrowser's venv (~2500 files), causing multi-second CLI
// startup hangs that broke the PTY detach deadline (TestDetachStartStopAndExitCode).
func TestEnsureSecretFilePermsPrunesNonConfigDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix mode bits")
	}
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, DirPerm); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Simulate a browser engine venv with a secret-like file inside it.
	// The walk must prune the browser/ subtree so this file is NOT chmod'd.
	venvDir := filepath.Join(home, "browser", "engines", "cloakbrowser", "venv", "bin")
	if err := os.MkdirAll(venvDir, DirPerm); err != nil {
		t.Fatalf("mkdir venv: %v", err)
	}
	venvSecret := filepath.Join(venvDir, "script.sh")
	if err := os.WriteFile(venvSecret, []byte("api_key: sk-should-not-be-scanned\n"), 0o644); err != nil {
		t.Fatalf("write venv: %v", err)
	}

	// Top-level config file with a secret — should be tightened.
	topSecret := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(topSecret, []byte("api_key: sk-live-abcdefghijklmnopqrstuvwxyz\n"), 0o644); err != nil {
		t.Fatalf("write top: %v", err)
	}

	if err := EnsureSecretFilePerms(home); err != nil {
		t.Fatalf("EnsureSecretFilePerms: %v", err)
	}

	// Top-level secret file tightened.
	topInfo, err := os.Stat(topSecret)
	if err != nil {
		t.Fatalf("stat top: %v", err)
	}
	if got := topInfo.Mode().Perm(); got != FilePerm {
		t.Fatalf("top secret mode = %o, want %o", got, FilePerm)
	}

	// Pruned subtree file left unchanged (not scanned).
	venvInfo, err := os.Stat(venvSecret)
	if err != nil {
		t.Fatalf("stat venv: %v", err)
	}
	if got := venvInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("pruned venv file mode changed to %o, want 0644 (should be skipped)", got)
	}
}
