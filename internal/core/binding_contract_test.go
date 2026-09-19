package core

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// moduleRoot walks up from this test file to the go.mod directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above " + file)
		}
		dir = parent
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// walkTS collects .ts/.tsx source under web/src (skipping node_modules/dist).
func walkTS(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == "dist" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".ts" || ext == ".tsx" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

// TestDesktopBindsSameCoreServiceAsCLI asserts VAL-DESK-009: both transport
// entrypoints under cmd/ reference core.NewService / core.Service rather than
// a parallel agent implementation.
func TestDesktopBindsSameCoreServiceAsCLI(t *testing.T) {
	root := moduleRoot(t)

	// Type identity: NewService returns *Service used by Wails App bindings.
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil Service")
	}
	// Sanity: shared facade methods used by both CLI open and desktop hub.
	if got := svc.BrandName(); got != "Inferenesia" {
		t.Fatalf("BrandName = %q", got)
	}

	desktopMain := readFile(t, filepath.Join(root, "cmd", "desktop", "main.go"))
	cliMain := readFile(t, filepath.Join(root, "cmd", "inferenesia", "main.go"))
	desktopApp := readFile(t, filepath.Join(root, "cmd", "desktop", "app.go"))

	// Same constructor / type under both cmd entrypoints (rg evidence shape).
	re := regexp.MustCompile(`core\.(NewService|Service)`)
	if !re.MatchString(desktopMain) {
		t.Fatal("cmd/desktop/main.go must reference core.NewService or core.Service (VAL-DESK-009)")
	}
	if !re.MatchString(cliMain) {
		t.Fatal("cmd/inferenesia/main.go must reference core.NewService or core.Service (VAL-DESK-009)")
	}
	if !strings.Contains(desktopMain, "core.NewService") {
		t.Fatal("desktop must call core.NewService (not a fork)")
	}
	if !strings.Contains(cliMain, "core.NewService") {
		t.Fatal("CLI entry must reference core.NewService so both transports share the constructor")
	}
	if !strings.Contains(desktopApp, "*core.Service") {
		t.Fatal("Wails App must hold *core.Service")
	}
	// App must not invent a second agent/LLM package.
	if strings.Contains(desktopApp, "provider.") && !strings.Contains(desktopApp, "core.") {
		t.Fatal("Wails App must not talk to provider package directly")
	}

	// Chat agent loop lives in this package (core.Run / Service.ChatStream).
	chatSrc := readFile(t, filepath.Join(root, "internal", "core", "chat.go"))
	if !strings.Contains(chatSrc, "Run(ctx, AgentConfig{") {
		t.Fatal("Service.ChatStream must call core.Run (single agent loop)")
	}
	cliChat := readFile(t, filepath.Join(root, "internal", "cli", "chat.go"))
	if !strings.Contains(cliChat, "core.Run(") {
		t.Fatal("CLI chat must call core.Run (same agent loop as desktop ChatStream)")
	}
	cliOpen := readFile(t, filepath.Join(root, "internal", "cli", "open.go"))
	if !strings.Contains(cliOpen, "core.NewService") {
		t.Fatal("CLI open must instantiate core.NewService (hub registry shared with desktop)")
	}
}

// TestNoSecondAgentLoopInTypeScript asserts VAL-DESK-008: the renderer never
// embeds an LLM client; model calls route only through Go core.Service (hub).
func TestNoSecondAgentLoopInTypeScript(t *testing.T) {
	root := moduleRoot(t)
	webSrc := filepath.Join(root, "web", "src")
	if st, err := os.Stat(webSrc); err != nil || !st.IsDir() {
		t.Fatalf("web/src missing: %v", err)
	}

	// Patterns from VAL-DESK-008 evidence:
	//   rg -n "fetch\(.*ai.temp.web.id|createOpenAI|streamText|chat.completions" web/src
	// We also flag other common SDK entry points if introduced.
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`fetch\s*\([^)]*ai\.temp\.web\.id`),
		regexp.MustCompile(`\bcreateOpenAI\b`),
		regexp.MustCompile(`\bstreamText\b`),
		regexp.MustCompile(`chat\.completions`),
		regexp.MustCompile(`from\s+['"]openai['"]`),
		regexp.MustCompile(`from\s+['"]@ai-sdk/`),
		regexp.MustCompile(`from\s+['"]@anthropic-ai/`),
		regexp.MustCompile(`https?://ai\.temp\.web\.id`),
	}

	var violations []string
	for _, path := range walkTS(t, webSrc) {
		body := readFile(t, path)
		// Strip block and line comments so docstrings referencing the forbid list
		// (e.g. "Never fetches ai.temp.web.id") do not count as client code.
		stripped := stripTSComments(body)
		rel, _ := filepath.Rel(root, path)
		for _, re := range forbidden {
			if re.MatchString(stripped) {
				violations = append(violations, rel+": matches "+re.String())
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("renderer LLM/agent client violations (VAL-DESK-008):\n  %s",
			strings.Join(violations, "\n  "))
	}

	// Positive: chat must go through hub streamChat → /api/chat/stream only.
	apiPath := filepath.Join(webSrc, "lib", "api.ts")
	api := readFile(t, apiPath)
	if !strings.Contains(api, "/api/chat/stream") {
		t.Fatal("web/src/lib/api.ts must stream via /api/chat/stream (core.Service hub)")
	}
	if !strings.Contains(api, "streamChat") {
		t.Fatal("web/src/lib/api.ts must export streamChat bound to core hub")
	}

	// Agent loop unit coverage lives in this package (agent_test.go).
	agentTest := filepath.Join(root, "internal", "core", "agent_test.go")
	if _, err := os.Stat(agentTest); err != nil {
		t.Fatalf("internal/core must cover agent loop (agent_test.go): %v", err)
	}
}

// stripTSComments removes // and /* */ comments (naive, string-safe enough for
// this contract scan — not a full TS parser).
func stripTSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		// Line comment
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			if i+1 < len(src) {
				i += 2
			}
			continue
		}
		// Single-quoted string
		if src[i] == '\'' {
			b.WriteByte(src[i])
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					b.WriteByte(src[i])
					b.WriteByte(src[i+1])
					i += 2
					continue
				}
				b.WriteByte(src[i])
				if src[i] == '\'' {
					i++
					break
				}
				i++
			}
			continue
		}
		// Double-quoted string
		if src[i] == '"' {
			b.WriteByte(src[i])
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					b.WriteByte(src[i])
					b.WriteByte(src[i+1])
					i += 2
					continue
				}
				b.WriteByte(src[i])
				if src[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}
		// Template literal (keep content; rare for LLM SDK imports)
		if src[i] == '`' {
			b.WriteByte(src[i])
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					b.WriteByte(src[i])
					b.WriteByte(src[i+1])
					i += 2
					continue
				}
				b.WriteByte(src[i])
				if src[i] == '`' {
					i++
					break
				}
				i++
			}
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}
