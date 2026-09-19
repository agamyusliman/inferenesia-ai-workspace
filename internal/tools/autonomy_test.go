package tools

import "testing"

func TestToolRisk_Classification(t *testing.T) {
	cases := []struct {
		name string
		want ToolRisk
	}{
		{"read_file", RiskReadOnly},
		{"grep", RiskReadOnly},
		{"list_dir", RiskReadOnly},
		{"git_status", RiskReadOnly},
		{"git_diff", RiskReadOnly},
		{"git_log", RiskReadOnly},
		{"git_branch_list", RiskReadOnly},
		{"terminal_read", RiskReadOnly},
		{"terminal_list", RiskReadOnly},
		{"context7_query", RiskReadOnly},
		{"task", RiskReadOnly},
		{"skill", RiskReadOnly},
		{"propose_plan", RiskReadOnly},
		{"todo_list", RiskReadOnly},
		{"multibrain_read", RiskReadOnly},

		{"write_file", RiskEdit},
		{"write_multibrain", RiskEdit},
		{"write_diagram", RiskEdit},
		{"create_diagram", RiskEdit},
		{"update_diagram", RiskEdit},
		{"export_diagram", RiskEdit},
		{"research_save", RiskEdit},
		{"multibrain_append", RiskEdit},

		{"git_commit", RiskReversible},
		{"git_push", RiskReversible},
		{"git_stage", RiskReversible},
		{"git_checkout", RiskReversible},
		{"terminal_start", RiskReversible},
		{"terminal_write", RiskReversible},
		{"mcp.echo.echo", RiskReversible},
		{"mcp.cocoindex.search", RiskReversible},

		{"shell", RiskDestructive},
		{"git_reset_hard", RiskDestructive},
		{"git_branch_delete", RiskDestructive},
		{"unknown_tool_xyz", RiskDestructive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolRisk(tc.name); got != tc.want {
				t.Fatalf("toolRisk(%q)=%v want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestAutonomyDenies_Matrix(t *testing.T) {
	// level -> tool -> denied?
	matrix := []struct {
		level string
		tool  string
		deny  bool
	}{
		// off: only read-only
		{"off", "read_file", false},
		{"off", "git_status", false},
		{"off", "write_file", true},
		{"off", "git_commit", true},
		{"off", "shell", true},
		{"off", "mcp.x.y", true},

		// low: read + edit
		{"low", "read_file", false},
		{"low", "write_file", false},
		{"low", "create_diagram", false},
		{"low", "git_commit", true},
		{"low", "terminal_start", true},
		{"low", "mcp.x.y", true},
		{"low", "shell", true},
		{"low", "git_reset_hard", true},

		// medium: + reversible, still no destructive
		{"medium", "write_file", false},
		{"medium", "git_commit", false},
		{"medium", "git_push", false},
		{"medium", "terminal_start", false},
		{"medium", "mcp.x.y", false},
		{"medium", "shell", true},
		{"medium", "git_reset_hard", true},
		{"medium", "git_branch_delete", true},

		// high: all
		{"high", "shell", false},
		{"high", "git_reset_hard", false},
		{"high", "write_file", false},
		{"high", "mcp.x.y", false},

		// empty level: no gate
		{"", "shell", false},

		// unknown level: treat as low
		{"weird", "write_file", false},
		{"weird", "shell", true},
		{"weird", "git_commit", true},
	}
	for _, tc := range matrix {
		name := tc.level + "/" + tc.tool
		if tc.level == "" {
			name = "empty/" + tc.tool
		}
		t.Run(name, func(t *testing.T) {
			denied, reason := autonomyDenies(tc.level, tc.tool)
			if denied != tc.deny {
				t.Fatalf("autonomyDenies(%q,%q)=%v want %v (reason=%q)", tc.level, tc.tool, denied, tc.deny, reason)
			}
			if denied && reason == "" {
				t.Fatal("denied but empty reason")
			}
			if !denied && reason != "" {
				t.Fatalf("allowed but reason=%q", reason)
			}
		})
	}
}

func TestAutonomyDenies_LowVsMediumDiffer(t *testing.T) {
	// The audit bug: low and medium must not be identical.
	for _, tool := range []string{"git_commit", "git_push", "terminal_start", "mcp.foo.bar"} {
		lowDeny, _ := autonomyDenies("low", tool)
		medDeny, _ := autonomyDenies("medium", tool)
		if !lowDeny {
			t.Fatalf("low should deny %s", tool)
		}
		if medDeny {
			t.Fatalf("medium should allow %s", tool)
		}
	}
}

func TestRegistry_AutonomyGate_Execute(t *testing.T) {
	reg := NewRegistry(nil)
	reg.SetAutonomyLevel("low")
	res := reg.Execute(t.Context(), Call{ID: "1", Name: NameShell, Arguments: `{"command":"echo hi"}`}, nil)
	if !res.IsError {
		t.Fatal("expected shell denied at low")
	}
	if res.Content == "" {
		t.Fatal("expected deny reason")
	}

	reg.SetAutonomyLevel("high")
	// shell will still error for other reasons (no workspace) but must not be autonomy-denied
	res = reg.Execute(t.Context(), Call{ID: "2", Name: NameShell, Arguments: `{"command":"true"}`}, nil)
	if res.IsError && stringsHasPrefix(res.Content, "autonomy ") {
		t.Fatalf("high should not autonomy-deny shell: %s", res.Content)
	}
}

func stringsHasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
