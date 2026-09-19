package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
)

// WriteSecureFile writes data to path with mode 0600 (FilePerm), creating parent
// directories as needed (DirPerm). Use for any config that may hold secrets.
func WriteSecureFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), DirPerm); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, FilePerm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, FilePerm); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// EnsureSecretFilePerms walks home (config home) and forces FilePerm (0600) on
// regular files that look like they contain key/token material. Non-secret
// files are left alone (may remain 0644). Errors walking individual files are
// collected and returned as a single multi-error style message when any fail.
//
// To keep CLI startup fast (the config home may contain large non-config trees
// such as browser engine installs, PTY session logs, or workspace undo state),
// well-known non-config subdirectories are pruned from the walk. Provider keys
// live in top-level config.yaml or shallow subdirs (e.g. .yura-ai/mcp/servers).
func EnsureSecretFilePerms(home string) error {
	// Subdirectories under home that never hold provider credentials and are
	// expensive to walk (engine installs, session stores, pty logs, etc.).
	pruneDirs := map[string]bool{
		"browser":    true, // CloakBrowser/Playwright venvs + engine binaries
		"pty":        true, // detached PTY session logs + FIFOs
		"workspaces": true, // per-workspace undo stacks + state
		"snippets":   true, // terminal snippet library (no secrets by design)
		"missions":   true, // mission evidence artifacts
		"skills":     true, // skill definitions
	}
	var errs []string
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries rather than aborting the whole walk.
			errs = append(errs, err.Error())
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != home {
				// Prune known non-config subtrees to avoid walking thousands of
				// engine/venv files on every CLI invocation.
				if pruneDirs[filepath.Base(path)] {
					return fs.SkipDir
				}
			}
			return nil
		}
		// Skip symlinks (venv bin links, etc.) — never chmod a symlink target.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		// Cap scan size for hygiene check (typical config is small).
		info, err := d.Info()
		if err != nil {
			errs = append(errs, err.Error())
			return nil
		}
		if info.Size() > 1<<20 { // 1 MiB
			return nil
		}
		// Skip non-text-ish extensions commonly not holding keys.
		name := strings.ToLower(filepath.Base(path))
		switch {
		case strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".jpg"),
			strings.HasSuffix(name, ".jpeg"), strings.HasSuffix(name, ".gif"),
			strings.HasSuffix(name, ".db"), strings.HasSuffix(name, ".sqlite"),
			strings.HasSuffix(name, ".sqlite3"), strings.HasSuffix(name, ".bin"):
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err.Error())
			return nil
		}
		if !secrethyg.ContainsSecretLike(string(data)) {
			return nil
		}
		if info.Mode().Perm()&0o077 != 0 || info.Mode().Perm() != FilePerm {
			if err := os.Chmod(path, FilePerm); err != nil {
				errs = append(errs, fmt.Sprintf("chmod %s: %v", path, err))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("ensure secret perms: %s", strings.Join(errs, "; "))
	}
	return nil
}
