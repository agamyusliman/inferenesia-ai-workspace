package mission

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
)

// DefaultGates are the standard verification gates for a Go + web monorepo.
// Names and commands are fixed so the UI can display them (VAL-MISSION-005).
var DefaultGates = []struct {
	Name    string
	Command string
	Args    []string
}{
	{Name: "go_test", Command: "go", Args: []string{"test", "./..."}},
	{Name: "go_vet", Command: "go", Args: []string{"vet", "./..."}},
	{Name: "typecheck_build", Command: "go", Args: []string{"build", "./..."}},
}

// GateRunner executes verification gate commands in a workspace directory.
// Tests may inject a fake runner via RunFunc.
type GateRunner struct {
	// WorkDir is the workspace root to run gates in.
	WorkDir string
	// Timeout per gate (default 3 minutes).
	Timeout time.Duration
	// RunFunc overrides exec for tests. When nil, uses real exec.
	RunFunc func(ctx context.Context, dir, name string, args []string) (exitCode int, output string, err error)
}

// RunGate executes one named gate and returns a GateResult (never panics).
func (r *GateRunner) RunGate(ctx context.Context, name string) GateResult {
	name = strings.TrimSpace(name)
	var cmdName string
	var args []string
	found := false
	for _, g := range DefaultGates {
		if g.Name == name {
			cmdName, args = g.Command, append([]string(nil), g.Args...)
			found = true
			break
		}
	}
	if !found {
		// Allow ad-hoc "command" as free-form: name is used as shell-like identity.
		return GateResult{
			Name:     name,
			Command:  name,
			Passed:   false,
			ExitCode: -1,
			Output:   fmt.Sprintf("unknown gate %q", name),
			RanAt:    time.Now().UTC(),
		}
	}
	return r.run(ctx, name, cmdName, args)
}

// RunAll executes DefaultGates in order and returns results.
func (r *GateRunner) RunAll(ctx context.Context) []GateResult {
	out := make([]GateResult, 0, len(DefaultGates))
	for _, g := range DefaultGates {
		if ctx.Err() != nil {
			break
		}
		out = append(out, r.run(ctx, g.Name, g.Command, g.Args))
	}
	return out
}

func (r *GateRunner) run(ctx context.Context, gateName, cmdName string, args []string) GateResult {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dir := strings.TrimSpace(r.WorkDir)
	if dir == "" {
		dir = "."
	}
	cmdLine := cmdName + " " + strings.Join(args, " ")
	start := time.Now()
	var exitCode int
	var output string
	var runErr error

	if r.RunFunc != nil {
		exitCode, output, runErr = r.RunFunc(cctx, dir, cmdName, args)
	} else {
		exitCode, output, runErr = defaultRun(cctx, dir, cmdName, args)
	}
	if runErr != nil && output == "" {
		output = runErr.Error()
	}
	// Always redact secrets from captured output (VAL-MISSION-008).
	output = secrethyg.Redact(output)
	passed := exitCode == 0 && runErr == nil
	if runErr != nil && exitCode == 0 {
		// exec error without exit (e.g. binary missing)
		exitCode = -1
		passed = false
	}
	return GateResult{
		Name:       gateName,
		Command:    cmdLine,
		Passed:     passed,
		ExitCode:   exitCode,
		Output:     truncateOutput(output, 32<<10),
		RanAt:      time.Now().UTC(),
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func defaultRun(ctx context.Context, dir, name string, args []string) (int, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err == nil {
		return 0, out, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), out, nil
	}
	return -1, out, err
}

func truncateOutput(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated]"
}

// GateCommandLine returns the display command for a known gate name.
func GateCommandLine(name string) string {
	for _, g := range DefaultGates {
		if g.Name == name {
			return g.Command + " " + strings.Join(g.Args, " ")
		}
	}
	return name
}

// ResolveWorkDir normalizes workspace root for gate execution.
func ResolveWorkDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}
