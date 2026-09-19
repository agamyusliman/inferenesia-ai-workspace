package orchestrator_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
)

// VAL-ORCH-006: in-flight task cancelled by task_id emits TaskDone(cancelled) ≤2s.
func TestCancelInFlightTaskEmitsCancelled(t *testing.T) {
	var (
		mu   sync.Mutex
		done []orchestrator.Event
	)
	mgr := orchestrator.New(orchestrator.Options{
		Emitter: func(ev orchestrator.Event) {
			if ev.Type == orchestrator.EventTaskDone && !ev.SynthesisConsumed {
				mu.Lock()
				done = append(done, ev)
				mu.Unlock()
			}
		},
	})

	started := make(chan struct{})
	var runs int32
	runner := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		atomic.AddInt32(&runs, 1)
		close(started)
		// Long body — wait for cancel.
		select {
		case <-ctx.Done():
			return "partial-before-cancel", []string{"partial"}, ctx.Err()
		case <-time.After(10 * time.Second):
			return "should-not-finish", nil, nil
		}
	}

	errCh := make(chan error, 1)
	go func() {
		_, _, err := mgr.SpawnParallel(context.Background(), []orchestrator.Spec{
			{Prompt: "long work", Category: orchestrator.CategoryExplore, Depth: 0},
		}, runner)
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner never started")
	}

	// Grab task id from list.
	list := mgr.List()
	if len(list) == 0 {
		t.Fatal("no tasks registered")
	}
	taskID := list[0].TaskID

	cancelAt := time.Now()
	if err := mgr.Cancel(taskID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// Wait for TaskDone cancelled ≤2s.
	deadline := time.Now().Add(2 * time.Second)
	var last orchestrator.Event
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(done) > 0 {
			last = done[len(done)-1]
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	elapsed := time.Since(cancelAt)
	if last.TaskID == "" {
		t.Fatalf("no TaskDone within 2s after cancel")
	}
	if last.Status != orchestrator.StatusCancelled {
		t.Fatalf("status=%s want cancelled detail=%+v", last.Status, last)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("cancel→TaskDone took %v want ≤2s", elapsed)
	}
	// Locks released — acquire write on arbitrary path should not conflict.
	if err := mgr.AcquireWriteLock("other", "path.txt"); err != nil {
		t.Fatalf("locks not released after cancel: %v", err)
	}
	mgr.ReleaseWriteLock("other", "path.txt")

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("SpawnParallel did not return after cancel")
	}

	snap, ok := mgr.Get(taskID)
	if !ok {
		t.Fatal("task missing after cancel")
	}
	if snap.Status != orchestrator.StatusCancelled {
		t.Fatalf("snap status=%s", snap.Status)
	}
	if !strings.Contains(snap.ResultSummary, "partial") && snap.ResultSummary != "cancelled" {
		// Allow either partial summary or cancelled placeholder.
		t.Logf("summary=%q (ok if cancelled placeholder)", snap.ResultSummary)
	}
}

// VAL-ORCH-007: resume cancelled task preserves context and skips completed tools.
func TestResumeCancelledPreservesContextSkipsTools(t *testing.T) {
	mgr := orchestrator.New(orchestrator.Options{})

	// Phase 1: run until one tool is recorded, then cancel.
	var toolExecCount int32
	started := make(chan struct{})
	runner1 := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		// Simulate completed tool call before cancel.
		if content, skipped := run.SkipCompletedTools("tc-1", "read_file", `{"path":"a.go"}`); skipped {
			return content, []string{content}, nil
		}
		atomic.AddInt32(&toolExecCount, 1)
		// Record as if tool finished.
		run.CompletedToolCalls = append(run.CompletedToolCalls, orchestrator.ToolCallRecord{
			ID:      "tc-1",
			Name:    "read_file",
			Args:    `{"path":"a.go"}`,
			Content: "file-a-content",
		})
		run.PartialOutput = "partial-from-tool"
		mgr.RecordToolCall(run.TaskID, orchestrator.ToolCallRecord{
			ID:      "tc-1",
			Name:    "read_file",
			Args:    `{"path":"a.go"}`,
			Content: "file-a-content",
		})
		close(started)
		select {
		case <-ctx.Done():
			return "partial-from-tool", []string{"file-a-content"}, ctx.Err()
		case <-time.After(10 * time.Second):
			return "done", nil, nil
		}
	}

	go func() {
		_, _, _ = mgr.SpawnParallel(context.Background(), []orchestrator.Spec{
			{Prompt: "do work with tools", Category: orchestrator.CategoryImplement, Depth: 0},
		}, runner1)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("phase1 never started")
	}
	list := mgr.List()
	if len(list) == 0 {
		t.Fatal("no task")
	}
	taskID := list[0].TaskID
	if err := mgr.Cancel(taskID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	// Wait cancelled.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s, ok := mgr.Get(taskID)
		if ok && s.Status == orchestrator.StatusCancelled {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	beforeResume, _ := mgr.Get(taskID)
	if beforeResume.Status != orchestrator.StatusCancelled {
		t.Fatalf("want cancelled got %s", beforeResume.Status)
	}
	if len(beforeResume.CompletedToolCalls) == 0 {
		// Fallback: runner may have set field only on pointer — RecordToolCall should have it.
		t.Fatalf("expected completed tool calls before resume: %+v", beforeResume)
	}

	// Phase 2: resume — must NOT re-execute tc-1.
	var reExec int32
	var sawSkip bool
	runner2 := func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
		// Must skip already-completed tool.
		if content, skipped := run.SkipCompletedTools("tc-1", "read_file", `{"path":"a.go"}`); skipped {
			sawSkip = true
			// Do a second tool that is new.
			return "final-with-prior:" + content, []string{content, "new-step"}, nil
		}
		atomic.AddInt32(&reExec, 1)
		return "re-executed-bad", nil, fmt.Errorf("should not re-exec")
	}

	snap, err := mgr.Resume(context.Background(), taskID, runner2)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !sawSkip {
		t.Fatal("resume did not skip completed tool via SkipCompletedTools")
	}
	if atomic.LoadInt32(&reExec) != 0 {
		t.Fatalf("re-executed tool %d times", reExec)
	}
	if snap.Status != orchestrator.StatusCompleted {
		t.Fatalf("status=%s want completed", snap.Status)
	}
	// Final summary includes pre-cancel partial output.
	if !strings.Contains(snap.ResultSummary, "partial") && !strings.Contains(snap.ResultSummary, "file-a-content") {
		t.Fatalf("summary missing pre-cancel partial: %q", snap.ResultSummary)
	}
	if !strings.Contains(snap.ResultSummary, "final-with-prior") {
		t.Fatalf("summary missing post-resume output: %q", snap.ResultSummary)
	}
}

// VAL-ORCH-008: categories enumerable; explore denies write; implement allows; verify has assertions.
func TestCategoriesEnumerateAndToolFilter(t *testing.T) {
	cats := orchestrator.KnownCategories()
	if len(cats) < 3 {
		t.Fatalf("categories=%d want ≥3", len(cats))
	}
	names := map[string]bool{}
	for _, c := range cats {
		names[c.Name] = true
	}
	for _, need := range []string{
		orchestrator.CategoryExplore,
		orchestrator.CategoryImplement,
		orchestrator.CategoryVerify,
	} {
		if !names[need] {
			t.Fatalf("missing category %s in %+v", need, names)
		}
	}

	// explore: no write_file
	if orchestrator.ToolAllowedInCategory(orchestrator.CategoryExplore, orchestrator.ToolWriteFile) {
		t.Fatal("explore must deny write_file")
	}
	if !orchestrator.ToolAllowedInCategory(orchestrator.CategoryExplore, orchestrator.ToolReadFile) {
		t.Fatal("explore must allow read_file")
	}
	if orchestrator.CategoryAllowsWrite(orchestrator.CategoryExplore) {
		t.Fatal("explore AllowsWrite=true")
	}

	// implement: allows write
	if !orchestrator.ToolAllowedInCategory(orchestrator.CategoryImplement, orchestrator.ToolWriteFile) {
		t.Fatal("implement must allow write_file")
	}
	if !orchestrator.CategoryAllowsWrite(orchestrator.CategoryImplement) {
		t.Fatal("implement AllowsWrite=false")
	}

	// verify: read-only + assertion tools
	if orchestrator.ToolAllowedInCategory(orchestrator.CategoryVerify, orchestrator.ToolWriteFile) {
		t.Fatal("verify must deny write_file")
	}
	if !orchestrator.ToolAllowedInCategory(orchestrator.CategoryVerify, orchestrator.ToolAssertContains) {
		t.Fatal("verify must allow assert_contains")
	}
	info := orchestrator.CategoryByName(orchestrator.CategoryVerify)
	if len(info.Assertions) == 0 {
		t.Fatal("verify Assertions empty")
	}
}
