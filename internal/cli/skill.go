package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/skills"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
	"github.com/spf13/cobra"
)

// newSkillCmd exposes progressive skill discovery and load (VAL-ORCH-004/005).
//
//	inferenesia skill list
//	inferenesia skill load <name>
//	inferenesia skill show <name>
//	inferenesia skill multibrain-read [--query …]
//	inferenesia skill multibrain-append --summary "…"
func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "List and load skills (progressive SKILL.md discovery)",
		Long: `Discover skills as metadata only (name + description) and load a named skill
to change the agent toolset / system prompt for a session.

Discovery roots (priority):
  1. builtin (multi-brain, go-core-worker, …)
  2. ~/.inferenesia/skills/**/SKILL.md
  3. <workspace>/.inferenesia/skills/**
  4. <workspace>/.claude/skills/**
  5. <workspace>/.agents/skills/**

Bodies are loaded only on ` + "`skill load <name>`" + ` — boot does not dump every skill
(VAL-ORCH-004). The multi-brain skill implements Multi Brain read/write protocol
(VAL-ORCH-005).`,
	}
	cmd.AddCommand(newSkillListCmd())
	cmd.AddCommand(newSkillLoadCmd())
	cmd.AddCommand(newSkillShowCmd())
	cmd.AddCommand(newSkillMultibrainReadCmd())
	cmd.AddCommand(newSkillMultibrainAppendCmd())
	return cmd
}

func newSkillListCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered skills (metadata only, progressive)",
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := newCLISkillLoader(workspace)
			if err != nil {
				return err
			}
			list, err := loader.List()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "skills (metadata only; use `skill load <name>` to load body):")
			if len(list) == 0 {
				fmt.Fprintln(out, "  (none)")
				return nil
			}
			for _, m := range list {
				fmt.Fprintf(out, "  %s  [%s]\n", m.Name, m.Source)
				if m.Description != "" {
					fmt.Fprintf(out, "    %s\n", m.Description)
				}
				if m.Path != "" {
					fmt.Fprintf(out, "    path: %s\n", m.Path)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root for project skills (default: cwd)")
	return cmd
}

func newSkillLoadCmd() *cobra.Command {
	var (
		workspace string
		showTools bool
	)
	cmd := &cobra.Command{
		Use:   "load <name>",
		Short: "Load a named skill (body + toolset/system-prompt delta)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := newCLISkillLoader(workspace)
			if err != nil {
				return err
			}
			name := strings.TrimSpace(args[0])
			sk, err := loader.Load(name)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "loaded: %s  source=%s\n", sk.Name, sk.Source)
			if sk.Path != "" {
				fmt.Fprintf(out, "path: %s\n", sk.Path)
			}
			fmt.Fprintf(out, "description: %s\n", sk.Description)
			toolsList := sk.AllowedTools
			if len(toolsList) == 0 {
				fmt.Fprintln(out, "allowed_tools: (none extra — system prompt changed)")
			} else {
				fmt.Fprintf(out, "allowed_tools: %s\n", strings.Join(toolsList, ", "))
			}
			overlay := loader.SystemPromptOverlay()
			fmt.Fprintf(out, "system_prompt_overlay_bytes: %d\n", len(overlay))
			// Observable toolset delta via a transient registry (VAL-ORCH-004).
			if showTools || true {
				reg := tools.NewRegistry(nil)
				reg.SetSkillBackend(tools.NewSkillBackend(loader))
				// Before/after semantics: list tool names after load.
				var names []string
				for _, d := range reg.Definitions() {
					names = append(names, d.Function.Name)
				}
				fmt.Fprintf(out, "toolset_after_load: %s\n", strings.Join(names, ", "))
			}
			// Show a short body preview so load is visibly not just metadata.
			preview := strings.TrimSpace(sk.Body)
			if preview != "" {
				lines := strings.Split(preview, "\n")
				max := 8
				if len(lines) < max {
					max = len(lines)
				}
				fmt.Fprintln(out, "body_preview:")
				for i := 0; i < max; i++ {
					fmt.Fprintf(out, "  %s\n", lines[i])
				}
				if len(lines) > max {
					fmt.Fprintln(out, "  …")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root for project skills (default: cwd)")
	cmd.Flags().BoolVar(&showTools, "show-tools", true, "Print toolset after load")
	return cmd
}

func newSkillShowCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show full skill body (loads without session activation side-channel)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := newCLISkillLoader(workspace)
			if err != nil {
				return err
			}
			sk, err := loader.Load(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "# %s\n\n", sk.Name)
			fmt.Fprintf(out, "source: %s\n", sk.Source)
			if sk.Path != "" {
				fmt.Fprintf(out, "path: %s\n", sk.Path)
			}
			fmt.Fprintf(out, "description: %s\n\n", sk.Description)
			fmt.Fprintln(out, sk.Body)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root (default: cwd)")
	return cmd
}

func newSkillMultibrainReadCmd() *cobra.Command {
	var (
		workspace string
		query     string
	)
	cmd := &cobra.Command{
		Use:   "multibrain-read",
		Short: "Run Multi Brain protocol read (session.md → ≤2 buckets → pointed contexts)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}
			res, err := skills.MultiBrainRead(skills.MultiBrainReadInput{
				Workspace: ws,
				Query:     query,
			})
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), res.Summary)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root (default: cwd)")
	cmd.Flags().StringVar(&query, "query", "", "Rank which 1–2 buckets to open")
	return cmd
}

func newSkillMultibrainAppendCmd() *cobra.Command {
	var (
		workspace    string
		summary      string
		agent        string
		topic        string
		query        string
		bucket       string
		description  string
		decision     string
		blocker      string
		verification string
		files        []string
		body         string
	)
	cmd := &cobra.Command{
		Use:   "multibrain-append",
		Short: "Append Multi Brain entry (newest-first, timestamp format, optional context pointer)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(summary) == "" {
				return fmt.Errorf("--summary is required")
			}
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}
			gw, err := writegate.New(ws)
			if err != nil {
				return err
			}
			res, err := skills.MultiBrainAppend(gw, skills.MultiBrainAppendInput{
				Workspace:    ws,
				Agent:        agent,
				Summary:      summary,
				Topic:        topic,
				Query:        query,
				Bucket:       bucket,
				Description:  description,
				Decision:     decision,
				Blocker:      blocker,
				Verification: verification,
				Files:        files,
				Body:         body,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "bucket: %s (%s)\n", res.BucketName, res.BucketPath)
			fmt.Fprintf(out, "entry: %s\n", res.EntryLine)
			fmt.Fprintf(out, "format_ok: %v\n", res.FormatOK)
			if res.ContextPath != "" {
				fmt.Fprintf(out, "context: %s\n", res.ContextPath)
			}
			fmt.Fprintf(out, "session_updated: %v created_bucket: %v\n", res.SessionUpdated, res.CreatedBucket)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root (default: cwd)")
	cmd.Flags().StringVar(&summary, "summary", "", "Short one-line summary (required)")
	cmd.Flags().StringVar(&agent, "agent", "Droid", "Agent display name")
	cmd.Flags().StringVar(&topic, "topic", "", "Topic slug for context filename")
	cmd.Flags().StringVar(&query, "query", "", "Free-text to select best bucket")
	cmd.Flags().StringVar(&bucket, "bucket", "", "Force bucket name")
	cmd.Flags().StringVar(&description, "description", "", "Scope when creating bucket")
	cmd.Flags().StringVar(&decision, "decision", "", "Decision note (triggers context)")
	cmd.Flags().StringVar(&blocker, "blocker", "", "Blocker note (triggers context)")
	cmd.Flags().StringVar(&verification, "verification", "", "Verification evidence (triggers context)")
	cmd.Flags().StringSliceVar(&files, "file", nil, "Important file path (repeatable)")
	cmd.Flags().StringVar(&body, "body", "", "Extra context body")
	_ = cmd.MarkFlagRequired("summary")
	return cmd
}

func newCLISkillLoader(workspace string) (*skills.Loader, error) {
	home, err := config.Home()
	if err != nil {
		// Non-fatal: discover without personal skills.
		home = ""
	}
	ws, err := resolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	// Ensure personal skills dir may exist (optional).
	if home != "" {
		_ = filepath.Join(home, "skills") // documented path
	}
	return skills.NewDefaultLoader(home, ws, nil), nil
}
