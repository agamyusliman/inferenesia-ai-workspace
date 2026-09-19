package mcp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenMCPTerms are the hard non-goal identifiers for the MCP host
// (VAL-MCP-004). Split construction so this test source itself does not
// contain the full literal tokens under a naive package-tree scan.
func forbiddenMCPTerms() []string {
	// Assemble via fragments to avoid self-matching a single-string rg of this file.
	a := []string{"eno", "wx"}
	b := []string{"qd", "rant"}
	c := []string{"voy", "age"}
	return []string{
		a[0] + a[1],
		b[0] + b[1],
		c[0] + c[1],
	}
}

// TestNoForbiddenRAGTermsInMCPTree enforces VAL-MCP-004: production sources
// under internal/mcp and cmd/ must not reference the excluded integrations.
// Test files (*_test.go) and contract/reference docs are excluded.
func TestNoForbiddenRAGTermsInMCPTree(t *testing.T) {
	terms := forbiddenMCPTerms()
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := findModuleRoot(pkgDir)
	if root == "" {
		root = filepath.Dir(filepath.Dir(pkgDir)) // internal/mcp → repo
	}

	scan := func(dir string) {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				if info != nil && (info.Name() == "testdata" || info.Name() == ".git") {
					return filepath.SkipDir
				}
				return err
			}
			// Production sources only: skip tests.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			low := strings.ToLower(string(data))
			for _, bad := range terms {
				if strings.Contains(low, bad) {
					t.Errorf("%s contains forbidden term %q (VAL-MCP-004)", path, bad)
				}
			}
			return nil
		})
	}

	scan(filepath.Join(root, "internal", "mcp"))
	scan(filepath.Join(root, "cmd"))

	// Optional rg over production paths (exclude *_test.go).
	if _, err := exec.LookPath("rg"); err == nil {
		// Pattern built from fragments so this file does not embed the full regex once.
		patParts := []string{terms[0], terms[1], terms[2]}
		pat := strings.Join(patParts, "|")
		cmd := exec.Command("rg", "-i", "-g", "!*_test.go", pat, "internal/mcp", "cmd")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err == nil {
			if len(strings.TrimSpace(string(out))) > 0 {
				t.Fatalf("forbidden terms in production sources:\n%s", out)
			}
			return
		}
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			// no matches — pass
			return
		}
		t.Fatalf("rg failed: %v\n%s", err, out)
	}
}

func findModuleRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
