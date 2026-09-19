package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv loads KEY=VALUE pairs from path into the process environment.
// Existing non-empty environment variables are never overridden.
// Blank lines and lines starting with # are ignored. Values may be single- or
// double-quoted. The function is a no-op when the file does not exist.
//
// This is intentionally minimal: enough for repo-local .env without a third-party dep.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Support long API key lines.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Optional leading "export ".
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if cur := strings.TrimSpace(os.Getenv(key)); cur != "" {
			continue
		}
		_ = os.Setenv(key, val)
	}
	return sc.Err()
}

// LoadDotEnvFromCWD tries .env in the current working directory, then walks
// parents a few levels for a repo-root .env (common when invoked from subdirs).
// Never logs file contents.
func LoadDotEnvFromCWD() error {
	wd, err := os.Getwd()
	if err != nil {
		return LoadDotEnv(".env")
	}
	dir := wd
	for i := 0; i < 6; i++ {
		path := filepath.Join(dir, ".env")
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return LoadDotEnv(path)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}
