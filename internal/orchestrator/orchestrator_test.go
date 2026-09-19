package orchestrator_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// VAL-ORCH-001: task() spawns parallel background explores with TaskSpawned/TaskDone.
func TestSpawnParallelExploreConcurrent(t *testing.T) {
	root := t.TempDir()
	// Seed searchable files so explore has hits for both prompts.
	mustWrite(t, filepath.Join(root, "auth_middleware.go"), "package auth\nfunc Middleware() {}\n")
	mustWrite(t, filepath.Join(root, "session_cookie.go"), "package session\nfunc ValidateCookie() {}\n")

	var (
		mu      sync.Mutex
		spawned []orchestrator.Event
		done    []orchestrator.Event
	)
	mgr := orchestrator.New(orchestrator.Options{
		Emitter: func(ev orchestrator.Event) {
			mu.Lock()
			defer mu.Unlock()
			switch ev.Type {
			case orchestrator.EventTaskSpawned:
				spawned = append(spawned, ev)
			case orchestrator.EventTaskDone:
				if !ev.SynthesisConsumed {
					done = append(done, ev)
				}
			}
		},
	})

	// Force overlap: both runners block until both have started.
	var started int32
	release := make(chan struct{})
	runner := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		atomic.AddInt32(&started, 1)
		// Wait until both have started (or timeout).
		deadline := time.Now().Add(2 * time.Second)
		for atomic.LoadInt32(&started) < 2 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		<-release
		// Minimal summary reflecting the input prompt.
		return "ok:" + run.Prompt, []string{run.Prompt}, nil
	}

	// Open release after both tasks have emitted TaskSpawned.
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			n := len(spawned)
			mu.Unlock()
			if n >= 2 {
				close(release)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(release)
	}()

	prompts := []string{
		"Find auth middleware handlers",
		"Find session cookie validation",
	}
	specs := []orchestrator.Spec{
		{Description: "explore auth", Prompt: prompts[0], Category: orchestrator.CategoryExplore, Depth: 0},
		{Description: "explore session", Prompt: prompts[1], Category: orchestrator.CategoryExplore, Depth: 0},
	}

	start := time.Now()
	merged, runs, err := mgr.SpawnParallel(context.Background(), specs, runner)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("SpawnParallel: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs=%d want 2: %+v", len(runs), runs)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(spawned) != 2 {
		t.Fatalf("TaskSpawned count=%d want 2: %+v", len(spawned), spawned)
	}
	if spawned[0].TaskID == "" || spawned[1].TaskID == "" || spawned[0].TaskID == spawned[1].TaskID {
		t.Fatalf("distinct task_ids required: %q %q", spawned[0].TaskID, spawned[1].TaskID)
	}
	// Wall-clock overlap: both StartedAt within ≤500ms.
	d := spawned[0].StartedAt.Sub(spawned[1].StartedAt)
	if d < 0 {
		d = -d
	}
	if d > 500*time.Millisecond {
		t.Fatalf("TaskSpawned timestamps not concurrent (delta=%v > 500ms)", d)
	}
	if len(done) != 2 {
		t.Fatalf("TaskDone count=%d want 2: %+v", len(done), done)
	}
	// Merged summary reflects both inputs.
	if !strings.Contains(merged, prompts[0]) || !strings.Contains(merged, prompts[1]) {
		t.Fatalf("merged must include both prompts:\n%s", merged)
	}
	for _, r := range runs {
		if r.Status != orchestrator.StatusCompleted {
			t.Fatalf("run status=%s err=%s", r.Status, r.Error)
		}
		if !strings.Contains(r.ResultSummary, "ok:") {
			t.Fatalf("summary=%q", r.ResultSummary)
		}
	}
	// Parallelism should complete well under 2× serial sleep; release gate means wall ≈ max not sum.
	if elapsed > 3*time.Second {
		t.Fatalf("too slow for parallel: %v", elapsed)
	}
	_ = root
}

// VAL-ORCH-002: depth default 1 — nested task() denied, no further TaskSpawned.
func TestDepthDefaultOneDeniesNested(t *testing.T) {
	var (
		mu      sync.Mutex
		spawned int
	)
	mgr := orchestrator.New(orchestrator.Options{
		Emitter: func(ev orchestrator.Event) {
			if ev.Type == orchestrator.EventTaskSpawned {
				mu.Lock()
				spawned++
				mu.Unlock()
			}
		},
	})

	// Outer spawn at depth 0 succeeds.
	outerRunner := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		// Nested attempt from inside sub-task (depth of caller = run.Depth which is 1).
		if err := mgr.CanSpawn(run.Depth); err == nil {
			t.Fatal("CanSpawn at depth 1 must fail")
		} else if !strings.Contains(err.Error(), "max depth") {
			t.Fatalf("error=%q", err)
		}
		// SpawnParallel with Depth=1 must not emit TaskSpawned.
		_, nestedRuns, err := mgr.SpawnParallel(ctx, []orchestrator.Spec{{
			Prompt: "nested should fail", Category: orchestrator.CategoryExplore, Depth: run.Depth,
		}}, func(context.Context, *orchestrator.TaskRun) (string, []string, error) {
			t.Fatal("nested runner must not execute")
			return "", nil, nil
		})
		if err != nil {
			return "", nil, err
		}
		if len(nestedRuns) != 1 || nestedRuns[0].TaskID != "" {
			t.Fatalf("nested runs=%+v", nestedRuns)
		}
		if !strings.Contains(nestedRuns[0].Error, orchestrator.MaxDepthError) &&
			!strings.Contains(nestedRuns[0].Error, "max depth") {
			t.Fatalf("nested error=%q", nestedRuns[0].Error)
		}
		return "outer-ok", []string{"outer"}, nil
	}

	merged, runs, err := mgr.SpawnParallel(context.Background(), []orchestrator.Spec{{
		Prompt: "outer explore", Category: orchestrator.CategoryExplore, Depth: 0,
	}}, outerRunner)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != orchestrator.StatusCompleted {
		t.Fatalf("outer runs=%+v", runs)
	}
	mu.Lock()
	n := spawned
	mu.Unlock()
	// Only outer TaskSpawned — nested denied without TaskSpawned.
	if n != 1 {
		t.Fatalf("TaskSpawned=%d want 1 (no nested)", n)
	}
	if !strings.Contains(merged, "outer") {
		t.Fatalf("merged=%s", merged)
	}

	// Direct CanSpawn(1) returns MaxDepthError string.
	err = mgr.CanSpawn(1)
	if err == nil || err.Error() != orchestrator.MaxDepthError {
		t.Fatalf("CanSpawn(1)=%v want %q", err, orchestrator.MaxDepthError)
	}
}

// VAL-ORCH-003: parallel overlapping writes → one blocked; single mutation on undo stack.
func TestParallelWriteConflictBlocked(t *testing.T) {
	root := t.TempDir()
	gw, err := writegate.New(root)
	if err != nil {
		t.Fatal(err)
	}
	// Pre-dirty content (not HEAD-related).
	mustWrite(t, filepath.Join(root, "shared.txt"), "DIRTY-BEFORE")

	var (
		mu       sync.Mutex
		blocked  int
		spawned  int
		writeOK  int
		writeErr int
	)
	mgr := orchestrator.New(orchestrator.Options{
		Emitter: func(ev orchestrator.Event) {
			mu.Lock()
			defer mu.Unlock()
			switch ev.Type {
			case orchestrator.EventBlocked, orchestrator.EventNeedsApproval:
				blocked++
			case orchestrator.EventTaskSpawned:
				spawned++
			}
		},
	})

	// Hold first lock until second has tried so we guarantee conflict.
	firstHeld := make(chan struct{})
	secondTried := make(chan struct{})
	var once sync.Once

	writeFn := func(path string, content []byte) error {
		// Actual WriteGateway mutation (single path).
		_, err := gw.WriteFile(path, content)
		return err
	}

	// Custom runners so first writer holds the lock long enough for second to conflict.
	runnerA := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		path := "shared.txt"
		if err := mgr.AcquireWriteLock(run.TaskID, path); err != nil {
			mu.Lock()
			writeErr++
			mu.Unlock()
			return "", nil, err
		}
		close(firstHeld)
		// Wait for second to attempt (or timeout).
		select {
		case <-secondTried:
		case <-time.After(2 * time.Second):
		}
		if err := writeFn(path, []byte("AFTER-A")); err != nil {
			mgr.ReleaseAllLocks(run.TaskID)
			return "", nil, err
		}
		mgr.ReleaseAllLocks(run.TaskID)
		mu.Lock()
		writeOK++
		mu.Unlock()
		return "wrote-A", []string{"A"}, nil
	}
	runnerB := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		<-firstHeld
		path := "shared.txt"
		err := mgr.AcquireWriteLock(run.TaskID, path)
		once.Do(func() { close(secondTried) })
		if err != nil {
			mu.Lock()
			writeErr++
			mu.Unlock()
			// Conflict — Blocked event already emitted by AcquireWriteLock.
			return "", nil, err
		}
		// Should not reach here if A still holds.
		if err := writeFn(path, []byte("AFTER-B")); err != nil {
			mgr.ReleaseAllLocks(run.TaskID)
			return "", nil, err
		}
		mgr.ReleaseAllLocks(run.TaskID)
		mu.Lock()
		writeOK++
		mu.Unlock()
		return "wrote-B", []string{"B"}, nil
	}

	// Dispatch both with staggered runners by mapping task order — use a composite runner.
	var gate int32
	composite := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		n := atomic.AddInt32(&gate, 1)
		if n == 1 {
			return runnerA(ctx, run)
		}
		return runnerB(ctx, run)
	}

	_, runs, err := mgr.SpawnParallel(context.Background(), []orchestrator.Spec{
		{Prompt: "WRITE path=shared.txt content=AFTER-A", Category: orchestrator.CategoryImplement, Depth: 0},
		{Prompt: "WRITE path=shared.txt content=AFTER-B", Category: orchestrator.CategoryImplement, Depth: 0},
	}, composite)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs=%d", len(runs))
	}

	mu.Lock()
	okN, errN, blk := writeOK, writeErr, blocked
	mu.Unlock()
	if okN != 1 {
		t.Fatalf("successful writes=%d want 1", okN)
	}
	if errN < 1 {
		t.Fatalf("blocked writes=%d want ≥1", errN)
	}
	if blk < 1 {
		t.Fatalf("Blocked events=%d want ≥1", blk)
	}

	// Exactly one mutation should be on the undo stack (single WriteGateway write).
	if depth := gw.UndoDepth(); depth != 1 {
		t.Fatalf("undo depth=%d want 1 (single mutation)", depth)
	}
	data, err := os.ReadFile(filepath.Join(root, "shared.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "AFTER-A" {
		t.Fatalf("file content=%q want AFTER-A (winner)", data)
	}
	// Undo restores pre-agent dirty.
	ur := gw.Undo()
	if ur.Noop {
		t.Fatal("undo should apply")
	}
	data, _ = os.ReadFile(filepath.Join(root, "shared.txt"))
	if string(data) != "DIRTY-BEFORE" {
		t.Fatalf("after undo=%q want DIRTY-BEFORE", data)
	}

	// One completed, one failed with blocked/conflict message.
	var ok, failed int
	for _, r := range runs {
		switch r.Status {
		case orchestrator.StatusCompleted:
			ok++
		case orchestrator.StatusFailed:
			failed++
			if !strings.Contains(r.Error, "blocked") && !strings.Contains(r.Error, "write") {
				t.Fatalf("failed error should mention blocked/write: %q", r.Error)
			}
		}
	}
	if ok != 1 || failed != 1 {
		t.Fatalf("ok=%d failed=%d runs=%+v", ok, failed, runs)
	}
}

// VAL-ORCH-009: sub-task failure isolation — partial result + error, not abort.
func TestSubTaskFailureIsolation(t *testing.T) {
	var doneOK, doneErr int
	mgr := orchestrator.New(orchestrator.Options{
		Emitter: func(ev orchestrator.Event) {
			if ev.Type != orchestrator.EventTaskDone || ev.SynthesisConsumed {
				return
			}
			if ev.Error != "" {
				doneErr++
			} else {
				doneOK++
			}
		},
	})

	// Force concurrency: slow success + fast error.
	runner := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		if strings.Contains(run.Prompt, "FAIL") {
			return "", nil, fmt.Errorf("injected failure for %s", run.Prompt)
		}
		// Overlap with failing sibling.
		time.Sleep(50 * time.Millisecond)
		return "ok:" + run.Prompt, []string{run.Prompt}, nil
	}

	merged, runs, err := mgr.SpawnParallel(context.Background(), []orchestrator.Spec{
		{Prompt: "FAIL please", Category: orchestrator.CategoryExplore, Depth: 0},
		{Prompt: "SUCCESS path", Category: orchestrator.CategoryExplore, Depth: 0},
	}, runner)
	if err != nil {
		// SpawnParallel itself must not return a batch-level abort.
		t.Fatalf("batch must not abort: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs=%d", len(runs))
	}
	var sawOK, sawFail bool
	for _, r := range runs {
		switch r.Status {
		case orchestrator.StatusCompleted:
			sawOK = true
			if !strings.Contains(r.ResultSummary, "SUCCESS") && !strings.Contains(r.ResultSummary, "ok:") {
				t.Fatalf("ok summary=%q", r.ResultSummary)
			}
		case orchestrator.StatusFailed:
			sawFail = true
			if r.Error == "" {
				t.Fatal("failed run missing error")
			}
		}
	}
	if !sawOK || !sawFail {
		t.Fatalf("need one ok + one fail: %+v", runs)
	}
	// Merged parent result includes both.
	if !strings.Contains(merged, "FAIL") || !strings.Contains(merged, "SUCCESS") {
		t.Fatalf("merged partial:\n%s", merged)
	}
	if doneOK < 1 || doneErr < 1 {
		t.Fatalf("TaskDone ok=%d err=%d", doneOK, doneErr)
	}
}

// VAL-ORCH-011: synthesis consumes task results without re-running explore.
func TestSynthesisConsumesResultsNoDuplicateSearch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n// alpha marker\n")
	mustWrite(t, filepath.Join(root, "b.go"), "package b\n// beta marker\n")

	var exploreCalls int32
	mgr := orchestrator.New(orchestrator.Options{})

	base := orchestrator.NewExploreRunnerFunc(root)
	counting := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		atomic.AddInt32(&exploreCalls, 1)
		return base(ctx, run)
	}

	_, runs, err := mgr.SpawnParallel(context.Background(), []orchestrator.Spec{
		{Prompt: "find alpha marker", Category: orchestrator.CategoryExplore, Depth: 0},
		{Prompt: "find beta marker", Category: orchestrator.CategoryExplore, Depth: 0},
	}, counting)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs=%d", len(runs))
	}
	if n := atomic.LoadInt32(&exploreCalls); n != 2 {
		t.Fatalf("explore calls=%d want 2", n)
	}

	// Synthesis must NOT re-invoke the explore runner.
	summary, consumed, err := mgr.SynthesizeFromResults("merge for primary")
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&exploreCalls); n != 2 {
		t.Fatalf("explore re-ran during synthesis: calls=%d", n)
	}
	if len(consumed) != 2 {
		t.Fatalf("consumed=%v", consumed)
	}
	if !strings.Contains(summary, "synthesis from task results") {
		t.Fatalf("summary=%s", summary)
	}
	// Logs prove synthesis consumed results.
	logs := mgr.SynthesisLog()
	if len(logs) == 0 {
		t.Fatal("expected synthesis log entry")
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "synthesis consumed") {
		t.Fatalf("synth log=%q", joined)
	}
	for _, id := range consumed {
		if !strings.Contains(joined, id) {
			t.Fatalf("log missing task_id %s: %s", id, joined)
		}
	}
	// Summary mentions both task results.
	for _, r := range runs {
		if !strings.Contains(summary, r.TaskID) {
			t.Fatalf("summary missing %s:\n%s", r.TaskID, summary)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
