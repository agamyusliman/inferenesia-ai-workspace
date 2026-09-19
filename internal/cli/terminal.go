package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/pty"
	"github.com/spf13/cobra"
)

func newTerminalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "terminal",
		Short: "Long-running PTY sessions (start, stop, read, list)",
		Long: `Manage long-running processes in pseudo-terminals (PTY).

Examples:
  ` + brand.Binary + ` terminal start -- sleep 3600
  ` + brand.Binary + ` terminal start --shell 'while true; do echo MARK; sleep 1; done'
  ` + brand.Binary + ` terminal stop <id>
  ` + brand.Binary + ` terminal read <id>
  ` + brand.Binary + ` terminal list

Sessions run detached under $INFERENESIA_HOME/pty so the CLI returns immediately
while the process keeps running (background/session mode). No debug ports
are opened by default (any future control port must stay in 4100–4199).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newTerminalStartCmd())
	cmd.AddCommand(newTerminalStopCmd())
	cmd.AddCommand(newTerminalReadCmd())
	cmd.AddCommand(newTerminalWriteCmd())
	cmd.AddCommand(newTerminalListCmd())
	cmd.AddCommand(newTerminalWorkerCmd()) // internal detached supervisor
	return cmd
}

func newTerminalStartCmd() *cobra.Command {
	var (
		shell     string
		workspace string
		sessionID string
		foreground bool
	)
	cmd := &cobra.Command{
		Use:   "start [--] <command> [args...]",
		Short: "Start a long-running process in a PTY (non-blocking by default)",
		Long: `Start a command under a real PTY.

By default the session is detached: the CLI prints session id + pid and exits
while the process continues (VAL-TERM-001). Use --foreground to block until
exit (still captures exit code).

Command may be given as argv after --, or as a shell string via --shell.

  ` + brand.Binary + ` terminal start -- sleep 30
  ` + brand.Binary + ` terminal start --shell 'echo hello; exit 7'`,
		// Allow unknown flags to pass through? No — use `--`.
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := ptyRoot()
			if err != nil {
				return err
			}
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}

			opts := pty.StartOpts{
				ID:      strings.TrimSpace(sessionID),
				WorkDir: ws,
			}
			if strings.TrimSpace(shell) != "" {
				opts.Shell = shell
			} else {
				if len(args) == 0 {
					return fmt.Errorf("terminal start: provide a command after -- or use --shell")
				}
				opts.Command = args
			}

			if foreground {
				return runTerminalForeground(cmd, opts)
			}

			bin, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable for detached worker: %w", err)
			}
			// Prefer the built binary path as-is.
			meta, err := pty.DetachStart(root, bin, opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "started session=%s pid=%d status=%s cmd=%q\n",
				meta.ID, meta.PID, meta.Status, meta.Command)
			if meta.WorkerPID > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "worker_pid=%d\n", meta.WorkerPID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "", "Run as sh -c '<shell>' instead of argv")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Working directory (default: cwd)")
	cmd.Flags().StringVar(&sessionID, "id", "", "Force session id (default: auto)")
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Block until process exits (in-process PTY)")
	return cmd
}

func runTerminalForeground(cmd *cobra.Command, opts pty.StartOpts) error {
	mgr := pty.NewManager(opts.WorkDir)
	// Register for SIGINT/exit cleanup (VAL-TERM-009). Detached workers are not registered.
	pty.RegisterCleanup(mgr)
	defer pty.UnregisterCleanup(mgr)
	defer pty.CleanupNow()

	s, err := mgr.Start(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "started session=%s pid=%d status=%s cmd=%q\n",
		s.ID, s.PID(), s.Status(), s.Command)
	s.Wait()
	code, ok := s.ExitCode()
	if !ok {
		code = -1
	}
	fmt.Fprintf(cmd.OutOrStdout(), "exited session=%s exit_code=%d\n", s.ID, code)
	out := s.Output()
	if out != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "--- output ---\n%s", out)
		if !strings.HasSuffix(out, "\n") {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	if code != 0 {
		// Surface non-zero as CLI failure for foreground mode.
		return fmt.Errorf("process exit_code=%d", code)
	}
	return nil
}

func newTerminalWriteCmd() *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "write <session-id> [--data TEXT]",
		Short: "Send input to a running PTY session",
		Long: `Write bytes to the session PTY (stdin of the child process).

Example (echo with cat):
  ` + brand.Binary + ` terminal start --id echo-cat -- cat
  ` + brand.Binary + ` terminal write echo-cat --data $'hello-pty\n'
  ` + brand.Binary + ` terminal read echo-cat`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := ptyRoot()
			if err != nil {
				return err
			}
			id := strings.TrimSpace(args[0])
			payload := data
			if payload == "" {
				// Allow remaining via flag only; empty is invalid.
				return fmt.Errorf("terminal write: provide --data (include newline with $'\\n' if needed)")
			}
			if err := pty.WriteInputToSession(root, id, []byte(payload)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %d bytes to session=%s\n", len(payload), id)
			return nil
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "Text/bytes to send to the PTY (use $'\\n' for Enter)")
	return cmd
}

func newTerminalStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <session-id>",
		Short: "Stop a running PTY session",
		Long: `Stop a PTY session by id (SIGTERM then SIGKILL).

Already-stopped or unknown ids are safe no-ops with a clear message and exit 0
(VAL-TERM-011).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := ptyRoot()
			if err != nil {
				return err
			}
			id := strings.TrimSpace(args[0])
			// Pre-load to distinguish unknown vs already stopped for messaging.
			prev, loadErr := pty.LoadMeta(root, id)
			if loadErr != nil && os.IsNotExist(loadErr) {
				fmt.Fprintf(cmd.OutOrStdout(), "session %q not found (already stopped or never existed)\n", id)
				return nil // exit 0 — VAL-TERM-011
			}
			if loadErr == nil && (prev.Status == pty.StatusExited || prev.Status == pty.StatusStopped) {
				fmt.Fprintf(cmd.OutOrStdout(), "session %q already %s (no-op)\n", id, prev.Status)
				return nil
			}
			meta, err := pty.DetachStop(root, id, 2*time.Second)
			if err != nil {
				if pty.IsNotFound(err) {
					fmt.Fprintf(cmd.OutOrStdout(), "session %q not found (already stopped or never existed)\n", id)
					return nil
				}
				return err
			}
			// Confirm process is gone.
			alive := pty.ProcessAlive(meta.PID)
			fmt.Fprintf(cmd.OutOrStdout(), "stopped session=%s status=%s pid=%d alive=%v\n",
				meta.ID, meta.Status, meta.PID, alive)
			if meta.ExitCode != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "exit_code=%d\n", *meta.ExitCode)
			}
			return nil
		},
	}
	return cmd
}

func newTerminalReadCmd() *cobra.Command {
	var maxBytes int
	cmd := &cobra.Command{
		Use:   "read <session-id>",
		Short: "Read captured PTY output for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := ptyRoot()
			if err != nil {
				return err
			}
			id := strings.TrimSpace(args[0])
			meta, err := pty.LoadMeta(root, id)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("session %q not found", id)
				}
				return err
			}
			// Refresh liveness for display.
			if meta.PID > 0 && (meta.Status == pty.StatusRunning || meta.Status == pty.StatusStarting) {
				if !pty.ProcessAlive(meta.PID) {
					// Process died but meta stale — best-effort mark.
					// Worker should have updated; if not, show running? leave as-is.
				}
			}
			out, err := pty.ReadOutputFile(root, id, maxBytes)
			if err != nil {
				return err
			}
			code := "-"
			if meta.ExitCode != nil {
				code = fmt.Sprintf("%d", *meta.ExitCode)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "session=%s status=%s pid=%d exit=%s\n",
				meta.ID, meta.Status, meta.PID, code)
			fmt.Fprintf(cmd.OutOrStdout(), "--- output ---\n%s", out)
			if out != "" && !strings.HasSuffix(out, "\n") {
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&maxBytes, "max-bytes", 64<<10, "Max output bytes to print (tail)")
	return cmd
}

func newTerminalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List PTY sessions (id, command, pid, status)",
		Long: `Show known PTY sessions with id, command, pid, and status (VAL-TERM-012).

Fresh state prints "(no sessions)".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := ptyRoot()
			if err != nil {
				return err
			}
			list, err := pty.ListMeta(root)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no sessions)")
				return nil
			}
			// Header for machine-friendly fields.
			fmt.Fprintln(cmd.OutOrStdout(), "ID  STATUS  PID  COMMAND")
			for _, m := range list {
				// Refresh alive hint for running sessions.
				alive := ""
				if m.Status == pty.StatusRunning && m.PID > 0 {
					if pty.ProcessAlive(m.PID) {
						alive = " alive=yes"
					} else {
						alive = " alive=no"
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  status=%s  pid=%d  cmd=%q%s\n",
					m.ID, m.Status, m.PID, m.Command, alive)
			}
			return nil
		},
	}
}

// newTerminalWorkerCmd is an internal supervisor used by DetachStart.
// Hidden from help.
func newTerminalWorkerCmd() *cobra.Command {
	var root, id string
	cmd := &cobra.Command{
		Use:    "__worker",
		Short:  "Internal: run detached PTY worker",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				var err error
				root, err = ptyRoot()
				if err != nil {
					return err
				}
			}
			if id == "" {
				return fmt.Errorf("terminal __worker: --id required")
			}
			// Args after -- are the command.
			return pty.RunWorker(root, id, args)
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "PTY persist root")
	cmd.Flags().StringVar(&id, "id", "", "Session id")
	return cmd
}

func ptyRoot() (string, error) {
	// Ensure config home exists so default ~/.inferenesia/pty is under a real tree.
	if _, err := config.EnsureHome(); err != nil {
		// Non-fatal if override env is set for tests.
		_ = err
	}
	return pty.PersistRoot()
}
