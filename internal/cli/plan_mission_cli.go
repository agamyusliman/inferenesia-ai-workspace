package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/agamyusliman/inferenesia-app/internal/mission"
	"github.com/spf13/cobra"
)

// newPlanCmd exposes Plan Mode enter/exit + propose/approve/reject and the
// cross-area approve-then-create-mission bridge (VAL-CROSS-004).
//
//	inferenesia plan enter [--reason …]
//	inferenesia plan exit  [--reason …]
//	inferenesia plan status
//	inferenesia plan propose --title "…" [--summary …] [--body …] --step a --step b
//	inferenesia plan approve-and-mission [--budget N]
//	inferenesia plan reject
//
// Plan Mode denies writes throughout (read-only). ApproveAndMission creates a
// Mission fed from the approved plan (goal/budget/acceptance criteria/plan
// snapshot) and exits Plan Mode so writes are re-enabled.
func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan Mode (read-only) and plan → mission bridge",
		Long: `Plan Mode is a read-only agent state: mutating tools (fs write, shell, git
write) are denied with the stable prefix ` + "`plan mode: writes denied`" + ` while read/explore
tools still work. A proposed plan can be approved to seed todos and, via
` + "`approve-and-mission`" + `, feed a Mission with goals/budget/verification gates.

` + brand.Name + ` keeps Plan Mode and Mission distinct: approve exits Plan Mode
(writes re-enabled) and creates a Mission that still requires verification
evidence before it can be marked complete (VAL-CROSS-004).`,
	}
	cmd.AddCommand(newPlanEnterCmd())
	cmd.AddCommand(newPlanExitCmd())
	cmd.AddCommand(newPlanStatusCmd())
	cmd.AddCommand(newPlanProposeCmd())
	cmd.AddCommand(newPlanApproveAndMissionCmd())
	cmd.AddCommand(newPlanRejectCmd())
	return cmd
}

func newPlanEnterCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "enter",
		Short: "Enter read-only Plan Mode (writes denied)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			st, err := svc.EnterPlanMode(reason)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Plan Mode: %s (reason=%s)\n", st.Mode, st.Reason)
			fmt.Fprintln(out, "writes denied — read/explore tools only")
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "user", "Reason for entering Plan Mode")
	return cmd
}

func newPlanExitCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "exit",
		Short: "Exit Plan Mode (re-enable writes)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			st, err := svc.ExitPlanMode(reason)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Plan Mode: %s (reason=%s) — writes re-enabled\n", st.Mode, st.Reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "user", "Reason for exiting Plan Mode (approve|reject|cancel|user)")
	return cmd
}

func newPlanStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current Plan Mode state",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			st := svc.GetPlanMode()
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "active=%v mode=%s", st.Active, st.Mode)
			if st.Reason != "" {
				fmt.Fprintf(out, " reason=%s", st.Reason)
			}
			fmt.Fprintln(out)
			return nil
		},
	}
}

func newPlanProposeCmd() *cobra.Command {
	var (
		title   string
		summary string
		body    string
		steps   []string
	)
	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Propose a plan (pending approval)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(title) == "" {
				return fmt.Errorf("plan propose: --title is required")
			}
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			p, err := svc.ProposePlan(title, summary, body, steps)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "proposed plan id=%s status=%s\n", p.ID, p.Status)
			fmt.Fprintf(out, "title: %s\n", p.Title)
			if len(p.Steps) > 0 {
				fmt.Fprintln(out, "steps:")
				for i, s := range p.Steps {
					fmt.Fprintf(out, "  %d. %s\n", i+1, s)
				}
			}
			fmt.Fprintln(out, "use `plan approve-and-mission` to approve and create a Mission from this plan")
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Plan title (required)")
	cmd.Flags().StringVar(&summary, "summary", "", "Short summary")
	cmd.Flags().StringVar(&body, "body", "", "Plan body (markdown)")
	cmd.Flags().StringSliceVar(&steps, "step", nil, "Plan step (repeatable; becomes todos + mission acceptance criteria)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

// newPlanApproveAndMissionCmd is the cross-area bridge (VAL-CROSS-004):
// approve the pending plan, exit Plan Mode, and create a Mission fed from
// the plan with goals/budget/verification gates + plan snapshot.
func newPlanApproveAndMissionCmd() *cobra.Command {
	var (
		budget       int64
		goalOverride string
	)
	cmd := &cobra.Command{
		Use:   "approve-and-mission",
		Short: "Approve plan, exit Plan Mode, and create a Mission from the plan (VAL-CROSS-004)",
		Long: `Approve the pending Plan Mode proposal, seed todos from its steps, exit
Plan Mode (writes re-enabled), and create a Mission fed from the plan:
goal derived from the plan title/summary, acceptance criteria from the plan
steps, plan snapshot captured, and an optional token budget.

The Mission is NOT marked complete here — Mission complete still requires
verification evidence (use ` + "`mission complete`" + ` after ` + "`mission evidence`" + `).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			res, err := svc.ApprovePlanAndCreateMission(core.MissionFromPlanRequest{
				BudgetTokens: budget,
				GoalOverride: goalOverride,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "plan approved: %s (seeded %d todos)\n", res.Plan.Title, res.Seeded)
			fmt.Fprintf(out, "plan mode: %s (writes re-enabled)\n", res.Mode.Mode)
			fmt.Fprintf(out, "mission created: id=%s status=%s\n", res.Mission.ID, res.Mission.Status)
			fmt.Fprintf(out, "mission goal: %s\n", res.Mission.Goal)
			if res.Mission.BudgetTokens > 0 {
				fmt.Fprintf(out, "mission budget: %d tokens\n", res.Mission.BudgetTokens)
			}
			if res.Mission.PlanID != "" {
				fmt.Fprintf(out, "mission plan_id: %s\n", res.Mission.PlanID)
			}
			fmt.Fprintln(out, "mission complete requires evidence — use `mission evidence` then `mission complete`")
			return nil
		},
	}
	cmd.Flags().Int64Var(&budget, "budget", 0, "Optional mission token budget (0 = unlimited)")
	cmd.Flags().StringVar(&goalOverride, "goal", "", "Override the mission goal (default: plan title/summary)")
	return cmd
}

func newPlanRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reject",
		Short: "Reject the pending plan (no todos, no mission)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			res, err := svc.RejectPlan()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "plan rejected: %s (todos unchanged, no mission created)\n", res.Plan.Title)
			return nil
		},
	}
}

// newMissionCmd exposes Mission lifecycle subcommands (VAL-MISSION-* + VAL-CROSS-004).
//
//	inferenesia mission list
//	inferenesia mission create --goal "…" [--budget N] [--criterion a --criterion b]
//	inferenesia mission complete <id>      # blocked without evidence
//	inferenesia mission evidence <id> --summary "…" [--content …] [--command …] [--exit-code N]
//	inferenesia mission block <id> --reason "…"
//	inferenesia mission show <id>
func newMissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mission",
		Short: "Mission / Goal lifecycle (evidence-gated completion)",
		Long: `Missions are long-running goals with optional budget, verification gates,
and evidence-gated completion. Complete is blocked until verification evidence
is attached (VAL-MISSION-002/003, VAL-CROSS-004).

A Mission may be created directly (` + "`mission create`" + `) or fed from an approved
Plan Mode proposal via ` + "`plan approve-and-mission`" + ` (VAL-CROSS-004).`,
	}
	cmd.AddCommand(newMissionListCmd())
	cmd.AddCommand(newMissionCreateCmd())
	cmd.AddCommand(newMissionCompleteCmd())
	cmd.AddCommand(newMissionEvidenceCmd())
	cmd.AddCommand(newMissionBlockCmd())
	cmd.AddCommand(newMissionShowCmd())
	return cmd
}

func newMissionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List missions (newest first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			list := svc.ListMissions()
			out := cmd.OutOrStdout()
			if list.Count == 0 {
				fmt.Fprintln(out, "(no missions)")
				return nil
			}
			for _, m := range list.Missions {
				line := fmt.Sprintf("%s  [%s]  %s", m.ID, m.Status, m.Goal)
				if m.PlanID != "" {
					line += fmt.Sprintf("  (from plan %s)", m.PlanID)
				}
				fmt.Fprintln(out, line)
			}
			return nil
		},
	}
}

func newMissionCreateCmd() *cobra.Command {
	var (
		goal     string
		budget   int64
		criteria []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a mission directly (no plan link)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(goal) == "" {
				return fmt.Errorf("mission create: --goal is required")
			}
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			m, err := svc.CreateMission(core.MissionCreateRequest{
				Goal:               goal,
				BudgetTokens:       budget,
				AcceptanceCriteria: criteria,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "mission created: id=%s status=%s\n", m.ID, m.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "goal: %s\n", m.Goal)
			fmt.Fprintln(cmd.OutOrStdout(), "complete requires evidence — use `mission evidence` then `mission complete`")
			return nil
		},
	}
	cmd.Flags().StringVar(&goal, "goal", "", "Mission goal (required)")
	cmd.Flags().Int64Var(&budget, "budget", 0, "Optional token budget (0 = unlimited)")
	cmd.Flags().StringSliceVar(&criteria, "criterion", nil, "Acceptance criterion (repeatable)")
	_ = cmd.MarkFlagRequired("goal")
	return cmd
}

func newMissionCompleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "complete <id>",
		Short: "Mark a mission complete (requires evidence; otherwise blocked)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			res := svc.CompleteMission(args[0])
			out := cmd.OutOrStdout()
			if !res.OK {
				// Blocked path: evidence required (or gate failure). Exit non-zero
				// so shell validators can observe the blocked state (VAL-CROSS-004).
				fmt.Fprintf(out, "blocked: %s\n", res.Message)
				fmt.Fprintf(out, "mission %s status=%s (unchanged)\n", res.Mission.ID, res.Mission.Status)
				return fmt.Errorf("mission complete blocked: %s", res.Message)
			}
			fmt.Fprintf(out, "mission %s status=%s\n", res.Mission.ID, res.Mission.Status)
			fmt.Fprintln(out, "complete")
			return nil
		},
	}
}

func newMissionEvidenceCmd() *cobra.Command {
	var (
		kind     string
		summary  string
		content  string
		command  string
		exitCode int
	)
	cmd := &cobra.Command{
		Use:   "evidence <id>",
		Short: "Attach verification evidence to a mission (required before complete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(summary) == "" {
				return fmt.Errorf("mission evidence: --summary is required")
			}
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			code := exitCode
			m, err := svc.AttachMissionEvidence(core.MissionEvidenceRequest{
				ID:       args[0],
				Kind:     kind,
				Summary:  summary,
				Content:  content,
				Command:  command,
				ExitCode: &code,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "evidence attached to mission %s (status=%s, evidence=%d)\n", m.ID, m.Status, len(m.Evidence))
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "command", "Evidence kind (command|note|gate|screenshot|test)")
	cmd.Flags().StringVar(&summary, "summary", "", "Short evidence summary (required)")
	cmd.Flags().StringVar(&content, "content", "", "Evidence content (command output, log, etc.)")
	cmd.Flags().StringVar(&command, "command", "", "Command that produced the evidence")
	cmd.Flags().IntVar(&exitCode, "exit-code", 0, "Exit code of the command (default 0)")
	_ = cmd.MarkFlagRequired("summary")
	return cmd
}

func newMissionBlockCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "block <id>",
		Short: "Mark a mission blocked with a reason",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(reason) == "" {
				return fmt.Errorf("mission block: --reason is required")
			}
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			m, err := svc.BlockMission(args[0], reason)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "mission %s status=%s reason=%s\n", m.ID, m.Status, m.BlockerReason)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Blocker reason (required)")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func newMissionShowCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one mission (status, gates, evidence, plan link)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			m, err := svc.GetMission(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if jsonOut {
				b, _ := json.MarshalIndent(m, "", "  ")
				fmt.Fprintln(out, string(b))
				return nil
			}
			fmt.Fprintf(out, "id: %s\n", m.ID)
			fmt.Fprintf(out, "status: %s\n", m.Status)
			fmt.Fprintf(out, "goal: %s\n", m.Goal)
			if m.PlanID != "" {
				fmt.Fprintf(out, "plan_id: %s\n", m.PlanID)
			}
			if m.PlanSnapshot != "" {
				fmt.Fprintln(out, "plan_snapshot:")
				for _, line := range strings.Split(m.PlanSnapshot, "\n") {
					fmt.Fprintf(out, "  %s\n", line)
				}
			}
			if m.BudgetTokens > 0 {
				fmt.Fprintf(out, "budget: %d tokens\n", m.BudgetTokens)
			}
			if m.BlockerReason != "" {
				fmt.Fprintf(out, "blocker: %s\n", m.BlockerReason)
			}
			if len(m.AcceptanceCriteria) > 0 {
				fmt.Fprintln(out, "acceptance_criteria:")
				for i, c := range m.AcceptanceCriteria {
					fmt.Fprintf(out, "  %d. %s\n", i+1, c)
				}
			}
			if len(m.Evidence) > 0 {
				fmt.Fprintf(out, "evidence: %d item(s)\n", len(m.Evidence))
			}
			if len(m.Gates) > 0 {
				fmt.Fprintf(out, "gates: %d\n", len(m.Gates))
				for _, g := range m.Gates {
					pass := "FAIL"
					if g.Passed {
						pass = "PASS"
					}
					fmt.Fprintf(out, "  %s  %s  (exit %d)\n", g.Name, pass, g.ExitCode)
				}
			}
			if m.CompletedAt != "" {
				fmt.Fprintf(out, "completed_at: %s\n", m.CompletedAt)
			}
			_ = mission.StatusPending // keep import for future status constants
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}
