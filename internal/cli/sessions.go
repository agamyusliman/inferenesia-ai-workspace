package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/store"
	"github.com/spf13/cobra"
)

func newSessionsCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:     "sessions",
		Aliases: []string{"session"},
		Short:   "List and inspect persisted chat sessions",
		Long: `List chat sessions stored under the config home SQLite database
(` + store.DBFileName + `). After a chat turn, restart the CLI and use
` + brand.Binary + ` sessions to list prior threads, then
` + brand.Binary + ` chat --session <id> to resume with the same messages.

Database path: $INFERENESIA_HOME/` + store.DBFileName + ` (default ~/.inferenesia/).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listSessions(cmd, limit)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "Maximum sessions to list (0 = all)")
	cmd.AddCommand(newSessionsShowCmd())
	cmd.AddCommand(newSessionsListCmd(&limit))
	return cmd
}

func newSessionsListCmd(limit *int) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List chat sessions (newest first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listSessions(cmd, *limit)
		},
	}
}

func newSessionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <session-id>",
		Short: "Print message transcript for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openSessionStore()
			if err != nil {
				return err
			}
			defer st.Close()

			id := strings.TrimSpace(args[0])
			meta, err := st.GetSession(id)
			if err != nil {
				return err
			}
			msgs, err := st.LoadMessages(id)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "session: %s\n", meta.ID)
			if meta.Title != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "title: %s\n", meta.Title)
			}
			if meta.Model != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "model: %s\n", meta.Model)
			}
			if meta.Profile != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "profile: %s\n", meta.Profile)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated: %s\n", meta.UpdatedAt.UTC().Format(time.RFC3339))
			fmt.Fprintf(cmd.OutOrStdout(), "messages: %d\n", len(msgs))
			fmt.Fprintln(cmd.OutOrStdout(), "---")
			for i, m := range msgs {
				role := string(m.Role)
				if role == "" {
					role = "?"
				}
				content := m.Content
				// Keep show output readable; tool args stay available via role line.
				if len(m.ToolCalls) > 0 {
					names := make([]string, 0, len(m.ToolCalls))
					for _, tc := range m.ToolCalls {
						names = append(names, tc.Function.Name)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "[%d] %s tools=%s\n", i, role, strings.Join(names, ","))
					if content != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "%s\n", content)
					}
					continue
				}
				if m.ToolCallID != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "[%d] %s tool_call_id=%s name=%s\n%s\n", i, role, m.ToolCallID, m.Name, content)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[%d] %s\n%s\n", i, role, content)
			}
			return nil
		},
	}
}

func listSessions(cmd *cobra.Command, limit int) error {
	st, err := openSessionStore()
	if err != nil {
		return err
	}
	defer st.Close()

	list, err := st.ListSessions(limit)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no sessions)")
		return nil
	}
	for _, s := range list {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}
		// Compact one-line list for shell grepping after restart.
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n",
			s.ID,
			s.UpdatedAt.UTC().Format(time.RFC3339),
			s.Model,
			title,
		)
	}
	return nil
}

func openSessionStore() (*store.Store, error) {
	home, err := config.EnsureHome()
	if err != nil {
		return nil, fmt.Errorf("config home: %w", err)
	}
	st, err := store.Open(store.DefaultPath(home))
	if err != nil {
		return nil, err
	}
	return st, nil
}
