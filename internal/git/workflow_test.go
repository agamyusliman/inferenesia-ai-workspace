package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSaveWorkflow(t *testing.T) {
	ws := t.TempDir()
	w, err := LoadWorkflow(ws)
	if err != nil {
		t.Fatal(err)
	}
	if w.Exists {
		t.Fatal("expected no file")
	}
	if len(w.Defaults.NeverCommit) == 0 {
		t.Fatal("defaults should include never_commit")
	}
	w.Defaults.Rules = "Only commit after tests pass."
	w.Repos = map[string]WorkflowRules{
		"cms-a": {
			DefaultBranch: "dev",
			Deploy: DeployPolicy{
				Enabled: true,
				When:    "after merge to main",
				Command: "npm run deploy:staging",
				Steps:   []string{"run e2e", "notify team"},
			},
		},
	}
	if err := SaveWorkflow(ws, w); err != nil {
		t.Fatal(err)
	}
	path := WorkflowPath(ws)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadWorkflow(ws)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Exists {
		t.Fatal("expected exists")
	}
	r := loaded.ResolveRules("cms-a")
	if r.DefaultBranch != "dev" {
		t.Fatalf("branch=%q", r.DefaultBranch)
	}
	if !r.Deploy.Enabled || r.Deploy.Command == "" {
		t.Fatalf("deploy: %+v", r.Deploy)
	}
	// Defaults never_commit still present via merge
	if len(r.NeverCommit) == 0 {
		t.Fatal("merged never_commit empty")
	}
	ctx := loaded.AgentContextMarkdown("cms-a", "cms-a", "dev")
	if !strings.Contains(ctx, "Deploy enabled") {
		t.Fatalf("context missing deploy:\n%s", ctx)
	}
	if !strings.Contains(ctx, "Never commit") {
		t.Fatalf("context missing never commit:\n%s", ctx)
	}
	// Ensure path under .inferenesia (preferred) or legacy .yura-ai
	if !strings.Contains(path, filepath.Join(".inferenesia", "git-workflow.yaml")) &&
		!strings.Contains(path, filepath.Join(".yura-ai", "git-workflow.yaml")) {
		t.Fatalf("path=%s", path)
	}
}
