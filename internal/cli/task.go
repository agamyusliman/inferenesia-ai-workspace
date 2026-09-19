package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/store"
	"github.com/spf13/cobra"
)

// newTaskCmd exposes CLI task cancel/resume/categories/list (VAL-ORCH-006/007/008).
func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Orchestrator task control (categories, cancel, resume, list)",
		Long: `Control background orchestrator tasks.

  inferenesia task categories   — list explore/implement/verify (+ aliases)
  inferenesia task list         — list known tasks from this process store
  inferenesia task cancel <id>  — cancel in-flight task by task_id
  inferenesia task resume <id>  — resume cancelled/failed task preserving context
  inferenesia task get <id>     — show one task snapshot

Note: cancel/resume operate on the shared SQLite store and an ephemeral Manager
hydrated from that store. Tasks spawned in a live chat share the same Manager when
desktop is running; CLI-only cancel works against durable rows + a hydrated Manager.`,
	}
	cmd.AddCommand(newTaskCategoriesCmd())
	cmd.AddCommand(newTaskListCmd())
	cmd.AddCommand(newTaskCancelCmd())
	cmd.AddCommand(newTaskResumeCmd())
	cmd.AddCommand(newTaskGetCmd())
	return cmd
}

func newTaskCategoriesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "categories",
		Short: "List task() categories (explore/implement/verify)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "task categories:")
			for _, c := range orchestrator.KnownCategories() {
				mode := "read-only"
				if c.AllowsWrite {
					mode = "writes via WriteGateway"
				}
				fmt.Fprintf(out, "  %s  %s (%s)\n", c.Name, c.Description, mode)
				fmt.Fprintf(out, "    tools: %s\n", strings.Join(c.Tools, ", "))
				if len(c.Assertions) > 0 {
					fmt.Fprintf(out, "    assertions: %s\n", strings.Join(c.Assertions, ", "))
				}
			}
			return nil
		},
	}
}

func newTaskListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List persisted orchestrator tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openTaskStore()
			if err != nil {
				return err
			}
			defer st.Close()
			list, err := st.ListTasks(100)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(list) == 0 {
				fmt.Fprintln(out, "no tasks")
				return nil
			}
			for _, t := range list {
				fmt.Fprintf(out, "%s  cat=%s  status=%s  %s\n",
					t.TaskID, t.Category, t.Status, truncateCLI(t.Description, 40))
			}
			return nil
		},
	}
}

func newTaskGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <task_id>",
		Short: "Show one task snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openTaskStore()
			if err != nil {
				return err
			}
			defer st.Close()
			t, err := st.GetTask(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "task_id: %s\n", t.TaskID)
			fmt.Fprintf(out, "category: %s\n", t.Category)
			fmt.Fprintf(out, "status: %s\n", t.Status)
			fmt.Fprintf(out, "prompt: %s\n", t.Prompt)
			fmt.Fprintf(out, "summary: %s\n", t.ResultSummary)
			if t.Error != "" {
				fmt.Fprintf(out, "error: %s\n", t.Error)
			}
			fmt.Fprintf(out, "completed_tools: %d\n", len(t.CompletedTools))
			return nil
		},
	}
}

func newTaskCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <task_id>",
		Short: "Cancel an in-flight background task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := strings.TrimSpace(args[0])
			st, err := openTaskStore()
			if err != nil {
				return err
			}
			defer st.Close()

			mgr := orchestrator.New(orchestrator.Options{
				Emitter: func(ev orchestrator.Event) {
					if ev.Type == orchestrator.EventTaskDone {
						fmt.Fprintf(cmd.ErrOrStderr(), "[TaskDone] task_id=%s status=%s\n",
							ev.TaskID, ev.Status)
					}
				},
			})
			wireStorePersist(mgr, st)
			hydrateManagerFromStore(mgr, st, taskID)

			// If store says running/queued but no live cancel, mark cancelled durable.
			snap, ok := mgr.Get(taskID)
			if !ok {
				return fmt.Errorf("task %q not found", taskID)
			}
			if snap.Status == orchestrator.StatusRunning || snap.Status == orchestrator.StatusQueued {
				if err := mgr.Cancel(taskID); err != nil {
					// Fall through to durable mark if cancel target missing live cancel fn.
					_ = err
				}
			}
			// Re-check; if still running without cancel func (hydrated only), force cancelled.
			snap, _ = mgr.Get(taskID)
			if snap.Status == orchestrator.StatusRunning || snap.Status == orchestrator.StatusQueued {
				// Mark cancelled in store for process-boundary cancel of durable row.
				row, err := st.GetTask(taskID)
				if err != nil {
					return err
				}
				row.Status = string(orchestrator.StatusCancelled)
				row.Error = context.Canceled.Error()
				row.EndedAt = time.Now().UTC()
				if row.ResultSummary == "" {
					row.ResultSummary = "cancelled"
				}
				if err := st.UpsertTask(row); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "cancelled task_id=%s (durable)\n", taskID)
				return nil
			}
			// Persist final snapshot from manager.
			if final, ok := mgr.Get(taskID); ok {
				_ = st.UpsertTask(taskRunToStore(final))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cancelled task_id=%s status=%s\n", taskID, snap.Status)
			return nil
		},
	}
}

func newTaskResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <task_id>",
		Short: "Resume a cancelled or failed task preserving context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := strings.TrimSpace(args[0])
			st, err := openTaskStore()
			if err != nil {
				return err
			}
			defer st.Close()

			mgr := orchestrator.New(orchestrator.Options{
				Emitter: func(ev orchestrator.Event) {
					switch ev.Type {
					case orchestrator.EventTaskSpawned:
						fmt.Fprintf(cmd.ErrOrStderr(), "[TaskSpawned] task_id=%s category=%s resume\n",
							ev.TaskID, ev.Category)
					case orchestrator.EventTaskDone:
						fmt.Fprintf(cmd.ErrOrStderr(), "[TaskDone] task_id=%s status=%s\n",
							ev.TaskID, ev.Status)
					}
				},
			})
			wireStorePersist(mgr, st)
			hydrateManagerFromStore(mgr, st, taskID)

			// Category-aware default runner: explore uses ExploreRunner.
			root, _ := os.Getwd()
			cat := orchestrator.CategoryExplore
			if row, err := st.GetTask(taskID); err == nil {
				cat = row.Category
			}
			var runner orchestrator.RunnerFunc
			if cat == orchestrator.CategoryImplement {
				// Implement without full agent loop: finish using prior partial + note.
				runner = func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
					// Skip already-completed tools by just consuming them.
					var b strings.Builder
					for _, tc := range run.CompletedToolCalls {
						b.WriteString(fmt.Sprintf("skipped completed tool %s: %s\n", tc.Name, truncateCLI(tc.Content, 80)))
					}
					prior := run.PartialOutput
					if prior == "" {
						prior = run.ResultSummary
					}
					sum := strings.TrimSpace(prior + "\n" + b.String() + "resume completed (CLI runner)")
					return sum, run.Results, nil
				}
			} else {
				explore := orchestrator.NewExploreRunnerFunc(root)
				runner = func(ctx context.Context, run *orchestrator.TaskRun) (string, []string, error) {
					// Do not re-run tools already completed.
					if len(run.CompletedToolCalls) > 0 {
						var b strings.Builder
						b.WriteString(run.PartialOutput)
						if b.Len() == 0 {
							b.WriteString(run.ResultSummary)
						}
						b.WriteString("\n(resume: skipped ")
						b.WriteString(fmt.Sprintf("%d completed tool calls)", len(run.CompletedToolCalls)))
						// Optional light explore only for new keywords if empty results.
						if len(run.Results) == 0 {
							s, r, err := explore(ctx, run)
							if err != nil && ctx.Err() != nil {
								return b.String(), run.Results, err
							}
							return strings.TrimSpace(b.String() + "\n" + s), append(run.Results, r...), nil
						}
						return strings.TrimSpace(b.String()), run.Results, nil
					}
					return explore(ctx, run)
				}
			}

			snap, err := mgr.Resume(context.Background(), taskID, runner)
			if err != nil {
				return err
			}
			_ = st.UpsertTask(taskRunToStore(snap))
			fmt.Fprintf(cmd.OutOrStdout(), "resumed task_id=%s status=%s\nsummary: %s\n",
				snap.TaskID, snap.Status, snap.ResultSummary)
			return nil
		},
	}
}

func openTaskStore() (*store.Store, error) {
	home, err := config.Home()
	if err != nil {
		return nil, err
	}
	return store.Open(store.DefaultPath(home))
}

func wireStorePersist(mgr *orchestrator.Manager, st *store.Store) {
	if mgr == nil || st == nil {
		return
	}
	mgr.SetPersist(func(run orchestrator.TaskRun) {
		_ = st.UpsertTask(taskRunToStore(run))
	})
}

func hydrateManagerFromStore(mgr *orchestrator.Manager, st *store.Store, taskID string) {
	if mgr == nil || st == nil {
		return
	}
	row, err := st.GetTask(taskID)
	if err != nil {
		// Also load all for list hydrate on cancel of missing in-mem only.
		return
	}
	mgr.LoadFromSnapshot(storeTaskToRun(row))
}

func taskRunToStore(run orchestrator.TaskRun) store.Task {
	tools := make([]store.ToolCallRecord, 0, len(run.CompletedToolCalls))
	for _, t := range run.CompletedToolCalls {
		tools = append(tools, store.ToolCallRecord{
			ID: t.ID, Name: t.Name, Args: t.Args, Content: t.Content, IsError: t.IsError,
		})
	}
	return store.Task{
		TaskID:          run.TaskID,
		ParentSessionID: run.ParentSessionID,
		Category:        run.Category,
		Description:     run.Description,
		Prompt:          run.Prompt,
		Status:          string(run.Status),
		ResultSummary:   run.ResultSummary,
		Error:           run.Error,
		Depth:           run.Depth,
		Results:         run.Results,
		Logs:            run.Logs,
		CompletedTools:  tools,
		CreatedAt:       run.CreatedAt,
		StartedAt:       run.StartedAt,
		EndedAt:         run.EndedAt,
	}
}

func storeTaskToRun(t store.Task) orchestrator.TaskRun {
	tools := make([]orchestrator.ToolCallRecord, 0, len(t.CompletedTools))
	for _, c := range t.CompletedTools {
		tools = append(tools, orchestrator.ToolCallRecord{
			ID: c.ID, Name: c.Name, Args: c.Args, Content: c.Content, IsError: c.IsError,
		})
	}
	partial := t.ResultSummary
	return orchestrator.TaskRun{
		TaskID:             t.TaskID,
		ParentSessionID:    t.ParentSessionID,
		Category:           t.Category,
		Description:        t.Description,
		Prompt:             t.Prompt,
		Status:             orchestrator.TaskStatus(t.Status),
		ResultSummary:      t.ResultSummary,
		Error:              t.Error,
		Depth:              t.Depth,
		Results:            t.Results,
		Logs:               t.Logs,
		PartialOutput:      partial,
		CompletedToolCalls: tools,
		CreatedAt:          t.CreatedAt,
		StartedAt:          t.StartedAt,
		EndedAt:            t.EndedAt,
	}
}
