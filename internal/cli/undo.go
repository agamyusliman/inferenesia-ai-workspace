package cli

import (
	"fmt"
	"path/filepath"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
	"github.com/spf13/cobra"
)

func newUndoCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "undo",
		Short: "Undo the last agent file write (restore pre-agent dirty content)",
		Long: `Restore the last WriteGateway mutation to its pre-agent snapshot.

This is NOT "restore to git HEAD". Undo returns files to the byte content that
existed immediately before the agent wrote them (including uncommitted dirty
edits). Use a separate git restore command if you intentionally want HEAD.

Stack is persisted under $INFERENESIA_HOME/workspaces/<id>/undo/stack.json.
Empty stack: prints "nothing to undo" and exits 0 (safe no-op).

Subcommand: undo list — same as "history" (agent-change timeline).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}
			g, err := openWorkspaceGateway(ws)
			if err != nil {
				return err
			}
			res := g.Undo()
			fmt.Fprintln(cmd.OutOrStdout(), res.Message)
			for _, ch := range res.Changes {
				label := ch.Rel
				if label == "" {
					label = ch.Path
				}
				if ch.Removed {
					fmt.Fprintf(cmd.OutOrStdout(), "[file_changed] undo removed %s\n", label)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "[file_changed] undo %s\n", label)
				}
			}
			if err := saveWorkspaceGateway(ws, g); err != nil {
				return err
			}
			// Empty stack is still success (no-op).
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root (default: current directory)")

	// undo list → agent-change timeline (VAL-MEM-012 alias).
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List agent-change timeline (alias for history)",
		RunE: func(c *cobra.Command, args []string) error {
			wsFlag, _ := c.Flags().GetString("workspace")
			if wsFlag == "" {
				wsFlag = workspace
			}
			return runHistory(c, wsFlag)
		},
	}
	listCmd.Flags().String("workspace", "", "Workspace root (default: current directory)")
	cmd.AddCommand(listCmd)
	return cmd
}

func newRedoCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "redo",
		Short: "Redo the last undone agent file write",
		Long: `Reapply the most recently undone WriteGateway mutation (agent after content).

Empty redo stack: prints "nothing to redo" and exits 0 (safe no-op, not a crash).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}
			g, err := openWorkspaceGateway(ws)
			if err != nil {
				return err
			}
			res := g.Redo()
			fmt.Fprintln(cmd.OutOrStdout(), res.Message)
			for _, ch := range res.Changes {
				label := ch.Rel
				if label == "" {
					label = ch.Path
				}
				if ch.Removed {
					fmt.Fprintf(cmd.OutOrStdout(), "[file_changed] redo removed %s\n", label)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "[file_changed] redo %s\n", label)
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

// openWorkspaceGateway creates a Gateway for ws and loads the persisted stack.
func openWorkspaceGateway(ws string) (*writegate.Gateway, error) {
	g, err := writegate.New(ws)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	path, err := undoStackPath(ws)
	if err != nil {
		return nil, err
	}
	if err := g.LoadStack(path); err != nil {
		return nil, fmt.Errorf("load undo stack: %w", err)
	}
	return g, nil
}

func saveWorkspaceGateway(ws string, g *writegate.Gateway) error {
	path, err := undoStackPath(ws)
	if err != nil {
		return err
	}
	if err := g.SaveStack(path); err != nil {
		return fmt.Errorf("save undo stack: %w", err)
	}
	return nil
}

func undoStackPath(ws string) (string, error) {
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	// Ensure home exists (mode 0700) before writing under workspaces/.
	if _, err := config.EnsureHome(); err != nil {
		return "", err
	}
	return writegate.PersistPath(home, ws), nil
}

// gatewayForChat builds a WriteGateway for chat that persists stack after each write.
// The returned cleanup function saves the final stack (also after tools finish).
func gatewayForChat(ws string, eventWriter interface{ Write([]byte) (int, error) }) (*writegate.Gateway, func() error, error) {
	g, err := writegate.New(ws)
	if err != nil {
		return nil, nil, err
	}
	path, err := undoStackPath(ws)
	if err != nil {
		return nil, nil, err
	}
	if err := g.LoadStack(path); err != nil {
		return nil, nil, fmt.Errorf("load undo stack: %w", err)
	}
	if eventWriter != nil {
		g.SetOnChange(func(ev writegate.ChangeEvent) {
			label := ev.Rel
			if label == "" {
				label = filepath.Base(ev.Path)
			}
			// file_changed lines for validators / users (VAL-UNDO-009).
			var line string
			switch {
			case ev.Removed:
				line = fmt.Sprintf("[file_changed] %s removed %s\n", ev.Op, label)
			default:
				line = fmt.Sprintf("[file_changed] %s %s\n", ev.Op, label)
			}
			_, _ = eventWriter.Write([]byte(line))
		})
	}
	cleanup := func() error {
		return g.SaveStack(path)
	}
	return g, cleanup, nil
}

