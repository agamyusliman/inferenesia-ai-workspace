// Package cli implements the inferenesia cobra command tree.
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
	"github.com/spf13/cobra"
)

// Execute runs the root command with process args and returns the exit code.
func Execute() int {
	return ExecuteWith(os.Args[1:], os.Stdout, os.Stderr)
}

// safeWriter wraps an io.Writer and redacts secret-like material before write.
// CLI must never print TEMP_AI_API_KEY values or Authorization bearer tokens
// (VAL-FOUND-012).
type safeWriter struct {
	w io.Writer
}

func (s safeWriter) Write(p []byte) (int, error) {
	redacted := secrethyg.Redact(string(p))
	n, err := io.WriteString(s.w, redacted)
	// Report full original length so callers do not retry on partial redact.
	if err == nil {
		return len(p), nil
	}
	return n, err
}

func redactf(w io.Writer, format string, args ...any) {
	msg := secrethyg.Redact(fmt.Sprintf(format, args...))
	fmt.Fprint(w, msg)
}

// ExecuteWith runs the CLI with the given args and I/O streams (testable entry).
func ExecuteWith(args []string, stdout, stderr io.Writer) int {
	// Load repo-local .env (if present) without overriding already-set env.
	// Never logs file contents (secrets stay out of stdout/stderr).
	_ = config.LoadDotEnvFromCWD()

	// First-run bootstrap: create ~/.inferenesia (or $YURA_AI_HOME) mode 0700 and
	// config.yaml when missing. Honors YURA_AI_HOME only — never touches the
	// default home when the override is set (VAL-FOUND-003/004).
	// Errors are non-fatal for help/unknown so operators still get usage text;
	// later milestones that require config can treat a missing home as hard fail.
	home, err := config.EnsureHome()
	if err != nil {
		redactf(stderr, "warning: could not ensure config home: %v\n", err)
	} else {
		// Tighten perms on any config file under home that already holds keys.
		_ = config.EnsureSecretFilePerms(home)
	}

	out := safeWriter{w: stdout}
	errOut := safeWriter{w: stderr}

	root := NewRootCmd()
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errOut)

	if err := root.Execute(); err != nil {
		redactf(errOut, "Error: %v\n", err)
		redactf(errOut, "\nRun '%s --help' for usage.\n", brand.Binary)
		return 1
	}
	return 0
}

// NewRootCmd builds the root cobra command and subcommands.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   brand.Binary,
		Short: brand.Name + " — " + brand.ShortDescription,
		Long: fmt.Sprintf(`%s

%s

Config home: ~/.inferenesia (override with INFERENESIA_HOME; legacy YURA_AI_HOME still honored).
Provider: Inferenesia API gateway and/or BYOK profiles.`, brand.Name, brand.ShortDescription),
		SilenceUsage:  true,
		SilenceErrors: true,
		// Startup banner line (product brand only; provider is labeled Inferenesia API).
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Skip banner for pure help/version flags so --help stays clean.
			if cmd.Name() == "help" || cmd.Name() == "version" {
				return
			}
			if len(os.Args) > 1 {
				for _, a := range os.Args[1:] {
					if a == "--help" || a == "-h" || a == "--version" || a == "-v" {
						return
					}
				}
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// No subcommand: print help (exit 0).
			return cmd.Help()
		},
	}

	cmd.Version = "0.0.1-dev"
	cmd.SetVersionTemplate(brand.Name + " {{.Version}}\n")

	cmd.AddCommand(newChatCmd())
	cmd.AddCommand(newSessionsCmd())
	cmd.AddCommand(newOpenCmd())
	cmd.AddCommand(newUndoCmd())
	cmd.AddCommand(newRedoCmd())
	cmd.AddCommand(newRevertCmd())
	cmd.AddCommand(newHistoryCmd())
	cmd.AddCommand(newMemoryCmd())
	cmd.AddCommand(newTerminalCmd())
	cmd.AddCommand(newTaskCmd())
	cmd.AddCommand(newSkillCmd())
	cmd.AddCommand(newBrowserCmd())
	cmd.AddCommand(newPlanCmd())
	cmd.AddCommand(newMissionCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newModelsCmd())

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		redactf(c.ErrOrStderr(), "Error: %v\n\nRun '%s --help' for usage.\n", err, brand.Binary)
		return err
	})

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print " + brand.Name + " version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", brand.Name, cmd.Root().Version)
		},
	}
}
