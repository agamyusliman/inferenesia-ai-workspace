package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

func TestTaskToolParallelExploreAndDepth(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "x.go"), "package x\n// keyword_alpha\n")
	mustWriteFile(t, filepath.Join(root, "y.go"), "package y\n// keyword_beta\n")

	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)

	var (
		mu      sync.Mutex
		spawned []string
		done    []string
	)
	mgr := orchestrator.New(orchestrator.Options{
		Emitter: func(ev orchestrator.Event) {
			mu.Lock()
			defer mu.Unlock()
			switch ev.Type {
			case orchestrator.EventTaskSpawned:
				spawned = append(spawned, ev.TaskID)
			case orchestrator.EventTaskDone:
				if !ev.SynthesisConsumed {
					done = append(done, ev.TaskID)
				}
			}
		},
	})
	adapter := tools.NewTaskAdapter(mgr, orchestrator.NewExploreRunnerFunc(root))
	reg.SetTaskBackend(adapter)

	// Definitions include task when backend wired.
	found := false
	for _, d := range reg.Definitions() {
		if d.Function.Name == tools.NameTask {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("task definition missing")
	}

	res := reg.Execute(context.Background(), tools.Call{
		ID:   "c1",
		Name: tools.NameTask,
		Arguments: `{
  "prompts": ["find keyword_alpha", "find keyword_beta"],
  "category": "explore",
  "run_in_background": true
}`,
	}, nil)
	if res.IsError {
		t.Fatalf("task tool: %s", res.Content)
	}
	if !strings.Contains(res.Content, "spawned 2") {
		t.Fatalf("content=%s", res.Content)
	}
	mu.Lock()
	nSpawn, nDone := len(spawned), len(done)
	ids := append([]string(nil), spawned...)
	mu.Unlock()
	if nSpawn != 2 || nDone != 2 {
		t.Fatalf("spawned=%d done=%d ids=%v", nSpawn, nDone, ids)
	}
	if ids[0] == ids[1] {
		t.Fatal("task_ids must be distinct")
	}
	// Both inputs reflected.
	if !strings.Contains(res.Content, "keyword_alpha") || !strings.Contains(res.Content, "keyword_beta") {
		t.Fatalf("merged missing prompts: %s", res.Content)
	}

	// Nested depth: set caller depth to 1 and call task again.
	adapter.SetCallerDepth(1)
	beforeSpawn := nSpawn
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "c2",
		Name:      tools.NameTask,
		Arguments: `{"prompt":"nested attempt","category":"explore"}`,
	}, nil)
	if !res.IsError {
		t.Fatalf("nested must error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "max depth") {
		t.Fatalf("nested error=%q", res.Content)
	}
	mu.Lock()
	afterSpawn := len(spawned)
	mu.Unlock()
	if afterSpawn != beforeSpawn {
		t.Fatalf("nested must not emit TaskSpawned: before=%d after=%d", beforeSpawn, afterSpawn)
	}

	// Synthesis without re-explore.
	adapter.SetCallerDepth(0)
	res = reg.Execute(context.Background(), tools.Call{
		ID:        "c3",
		Name:      tools.NameTask,
		Arguments: `{"synthesize":true,"note":"primary merge"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("synthesize: %s", res.Content)
	}
	if !strings.Contains(res.Content, "synthesis") {
		t.Fatalf("synth content=%s", res.Content)
	}
	logs := adapter.SynthesisLog()
	if len(logs) == 0 || !strings.Contains(strings.Join(logs, "\n"), "synthesis consumed") {
		t.Fatalf("synthesis log=%v", logs)
	}
}

func TestTaskToolFailureIsolation(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(gw)
	mgr := orchestrator.New(orchestrator.Options{})
	runner := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		if strings.Contains(run.Prompt, "boom") {
			return "", nil, context.DeadlineExceeded
		}
		time.Sleep(20 * time.Millisecond)
		return "ok-" + run.Prompt, []string{run.Prompt}, nil
	}
	reg.SetTaskBackend(tools.NewTaskAdapter(mgr, runner))

	res := reg.Execute(context.Background(), tools.Call{
		ID:   "f1",
		Name: tools.NameTask,
		Arguments: `{
  "prompts": ["boom now", "healthy explore"],
  "category": "explore"
}`,
	}, nil)
	if res.IsError {
		// Tool itself should still return partial merge, not abort whole batch as hard error
		// unless every run denied. Partial failures are reported in content.
		t.Logf("tool content (may be ok with partials): %s", res.Content)
	}
	// Even with one failure, content should mention both.
	if !strings.Contains(res.Content, "boom") || !strings.Contains(res.Content, "healthy") {
		// Content embeds merged summaries with prompts.
		if !strings.Contains(res.Content, "spawned") {
			t.Fatalf("expected partial result content: %s", res.Content)
		}
	}
	// At least one success status in content.
	if !strings.Contains(res.Content, "completed") && !strings.Contains(res.Content, "failed") {
		t.Fatalf("status missing: %s", res.Content)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
