package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
)

// Assertion-style tool names for verify category (VAL-ORCH-008).
const (
	NameAssertContains  = orchestrator.ToolAssertContains
	NameAssertExitCode  = orchestrator.ToolAssertExitCode
	NameAssertFileExists = orchestrator.ToolAssertFile
)

func assertDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameAssertContains,
				Description: "Assert that a workspace text file contains a substring (verify category). Read-only.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "File path relative to workspace root" },
    "substring": { "type": "string", "description": "Expected substring" }
  },
  "required": ["path", "substring"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameAssertExitCode,
				Description: "Assert an expected exit code (verify). Pass command_exit_code observed from a prior shell run.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "command_exit_code": { "type": "integer", "description": "Observed exit code" },
    "expected": { "type": "integer", "description": "Expected exit code (default 0)" }
  },
  "required": ["command_exit_code"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameAssertFileExists,
				Description: "Assert a workspace path exists (file or dir). Read-only verify tool.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Path relative to workspace root" },
    "is_dir": { "type": "boolean", "description": "When true, require a directory" }
  },
  "required": ["path"]
}`),
			},
		},
	}
}

func (r *Registry) execAssertContains(call Call) Result {
	var args struct {
		Path      string `json:"path"`
		Substring string `json:"substring"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameAssertContains, Content: fmt.Sprintf("assert_contains: invalid args: %v", err), IsError: true}
	}
	args.Path = strings.TrimSpace(args.Path)
	if args.Path == "" || args.Substring == "" {
		return Result{Name: NameAssertContains, Content: "assert_contains: path and substring required", IsError: true}
	}
	if r == nil || r.gw == nil {
		return Result{Name: NameAssertContains, Content: "assert_contains: workspace not configured", IsError: true}
	}
	abs, _, err := r.gw.ResolveInWorkspace(args.Path)
	if err != nil {
		return Result{Name: NameAssertContains, Content: err.Error(), IsError: true}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Result{Name: NameAssertContains, Content: fmt.Sprintf("assert_contains FAIL: read %s: %v", args.Path, err), IsError: true}
	}
	if !strings.Contains(string(data), args.Substring) {
		return Result{
			Name:    NameAssertContains,
			Content: fmt.Sprintf("assert_contains FAIL: %q not found in %s", args.Substring, args.Path),
			IsError: true,
		}
	}
	return Result{
		Name:    NameAssertContains,
		Content: fmt.Sprintf("assert_contains PASS: %q in %s", args.Substring, args.Path),
	}
}

func (r *Registry) execAssertExitCode(call Call) Result {
	var args struct {
		CommandExitCode *int `json:"command_exit_code"`
		Expected        *int `json:"expected"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameAssertExitCode, Content: fmt.Sprintf("assert_exit_code: invalid args: %v", err), IsError: true}
	}
	if args.CommandExitCode == nil {
		return Result{Name: NameAssertExitCode, Content: "assert_exit_code: command_exit_code required", IsError: true}
	}
	expected := 0
	if args.Expected != nil {
		expected = *args.Expected
	}
	got := *args.CommandExitCode
	if got != expected {
		return Result{
			Name: NameAssertExitCode,
			Content: fmt.Sprintf("assert_exit_code FAIL: got %s want %s",
				strconv.Itoa(got), strconv.Itoa(expected)),
			IsError: true,
		}
	}
	return Result{
		Name:    NameAssertExitCode,
		Content: fmt.Sprintf("assert_exit_code PASS: exit_code=%d", got),
	}
}

func (r *Registry) execAssertFileExists(call Call) Result {
	var args struct {
		Path  string `json:"path"`
		IsDir *bool  `json:"is_dir"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{Name: NameAssertFileExists, Content: fmt.Sprintf("assert_file_exists: invalid args: %v", err), IsError: true}
	}
	args.Path = strings.TrimSpace(args.Path)
	if args.Path == "" {
		return Result{Name: NameAssertFileExists, Content: "assert_file_exists: path required", IsError: true}
	}
	if r == nil || r.gw == nil {
		return Result{Name: NameAssertFileExists, Content: "assert_file_exists: workspace not configured", IsError: true}
	}
	abs, _, err := r.gw.ResolveInWorkspace(args.Path)
	if err != nil {
		return Result{Name: NameAssertFileExists, Content: err.Error(), IsError: true}
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Result{
			Name:    NameAssertFileExists,
			Content: fmt.Sprintf("assert_file_exists FAIL: %s does not exist (%v)", args.Path, err),
			IsError: true,
		}
	}
	if args.IsDir != nil && *args.IsDir && !fi.IsDir() {
		return Result{
			Name:    NameAssertFileExists,
			Content: fmt.Sprintf("assert_file_exists FAIL: %s is not a directory", args.Path),
			IsError: true,
		}
	}
	kind := "file"
	if fi.IsDir() {
		kind = "dir"
	}
	return Result{
		Name:    NameAssertFileExists,
		Content: fmt.Sprintf("assert_file_exists PASS: %s (%s) %s", args.Path, kind, filepath.Base(abs)),
	}
}
