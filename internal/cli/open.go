package cli

import (
	"fmt"

	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/spf13/cobra"
)

// newOpenCmd registers a project path in the workspace hub (same core.Service as desktop).
func newOpenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open <path>",
		Short: "Open or register a workspace path (hub registry)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.NewService(core.Options{})
			if err != nil {
				return err
			}
			w, err := svc.OpenWorkspace(args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "opened workspace %s\nid: %s\npath: %s\n", w.Name, w.ID, w.RootPath)
			return nil
		},
	}
	return cmd
}
