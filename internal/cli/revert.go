package cli

import (
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
	"github.com/spf13/cobra"
)

// newRevertCmd: revert last agent turn, or revert a single file.
//
//	inferenesia revert           → last turn (files + multibrain memory)
//	inferenesia revert <file>    → single path to pre-agent dirty
//
// Restores pre-agent dirty snapshots, never git HEAD by default
// (VAL-MEM-007/008, VAL-CROSS-013).
func newRevertCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "revert [file]",
		Short: "Revert last agent turn or a single file to pre-agent dirty state",
		Long: `Revert agent file mutations using WriteGateway shadow snapshots.

Without arguments, reverts the last agent turn: all files written in that turn
return to their pre-agent dirty content (including Multi Brain memory files).
This is NOT "restore to git HEAD".

With a path argument, reverts only that file to its most recent pre-agent
snapshot. Other files and memory for the turn are left unchanged.
Reverting a file the agent never changed is a safe no-op with a clear message.

Stack: $INFERENESIA_HOME/workspaces/<id>/undo/stack.json
See also: undo, redo, history`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}
			g, err := openWorkspaceGateway(ws)
			if err != nil {
				return err
			}
			var res writegate.UndoResult
			if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
				res = g.RevertFile(args[0])
			} else {
				res = g.Revert()
			}
			fmt.Fprintln(cmd.OutOrStdout(), res.Message)
			for _, ch := range res.Changes {
				label := ch.Rel
				if label == "" {
					label = ch.Path
				}
				if ch.Removed {
					fmt.Fprintf(cmd.OutOrStdout(), "[file_changed] revert removed %s\n", label)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "[file_changed] revert %s\n", label)
				}
			}
			if err := saveWorkspaceGateway(ws, g); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root (default: current directory)")
	return cmd
}

// newHistoryCmd lists agent-change timeline (VAL-MEM-012).
// Also registered as "undo list" via undo subcommand wiring in root.
func newHistoryCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:     "history",
		Aliases: []string{"changes"},
		Short:   "Show agent-change timeline available for revert/undo",
		Long: `List agent write turns recorded by WriteGateway (newest first).

Each entry shows turn id, time, file paths, and mutation sources so you can
choose revert (last turn) or revert <file>. On a fresh workspace with no
agent writes, prints a clear empty message.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(cmd, workspace)
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root (default: current directory)")
	return cmd
}

func runHistory(cmd *cobra.Command, workspace string) error {
	ws, err := resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	g, err := openWorkspaceGateway(ws)
	if err != nil {
		return err
	}
	h := g.History()
	fmt.Fprint(cmd.OutOrStdout(), writegate.FormatHistory(h))
	return nil
}
