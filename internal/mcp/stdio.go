package mcp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// stdioConn holds a spawned stdio MCP server process + session.
type stdioConn struct {
	name   string
	cfg    ServerConfig
	cmd    *exec.Cmd
	sess   *session
	stdin  io.WriteCloser
	stdout io.ReadCloser
	tools  []ToolSchema
	mu     sync.Mutex
	closed bool
}

func startStdio(ctx context.Context, cfg ServerConfig, info ClientInfo) (*stdioConn, error) {
	argv := cfg.CommandArgv()
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	// Long-lived process: do NOT bind to Register's context (would kill the
	// server when the short registration timeout ends). Close() terminates it.
	cmd := exec.Command(argv[0], argv[1:]...)
	if len(cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), cfg.Env...)
	}
	// Keep stderr available for diagnostics (not logged with secrets).
	var stderr strings.Builder
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		// VAL-MCP-003: clear unavailable shape with server name + failure mode.
		return nil, fmt.Errorf("%s", FormatUnavailable(cfg.Name, "start failed: "+err.Error()))
	}

	c := &stdioConn{
		name:   cfg.Name,
		cfg:    cfg,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		sess:   newSession(stdout, stdin),
	}

	// Handshake with timeout so a hung server does not block the agent forever.
	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tools, err := c.sess.Initialize(initCtx, info)
	if err != nil {
		_ = c.Close()
		reason := err.Error()
		if s := strings.TrimSpace(stderr.String()); s != "" {
			// Cap stderr snippet; never include auth headers (stdio has none by default).
			if len(s) > 200 {
				s = s[:200] + "…"
			}
			reason = reason + "; stderr: " + s
		}
		return nil, fmt.Errorf("%s", FormatUnavailable(cfg.Name, "crashed/handshake: "+reason))
	}
	c.tools = tools
	return c, nil
}

func (c *stdioConn) Tools() []ToolSchema {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ToolSchema(nil), c.tools...)
}

func (c *stdioConn) Call(ctx context.Context, tool string, args any) (string, bool, error) {
	c.mu.Lock()
	closed := c.closed
	sess := c.sess
	c.mu.Unlock()
	if closed || sess == nil {
		return "", true, fmt.Errorf("%s", FormatUnavailable(c.name, "process stopped"))
	}
	text, isErr, err := sess.CallTool(ctx, tool, args)
	if err != nil {
		// Process may have died.
		if c.cmd != nil && c.cmd.ProcessState != nil && c.cmd.ProcessState.Exited() {
			return "", true, fmt.Errorf("%s", FormatUnavailable(c.name, "crashed: "+err.Error()))
		}
		return "", true, fmt.Errorf("%s", FormatUnavailable(c.name, err.Error()))
	}
	return text, isErr, nil
}

func (c *stdioConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	sess := c.sess
	c.mu.Unlock()

	if sess != nil {
		sess.Close()
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		// Graceful then hard kill.
		_ = c.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = c.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
		}
	}
	if c.stdout != nil {
		_ = c.stdout.Close()
	}
	return nil
}

// Pid returns the OS pid when the process is running (0 if unknown).
func (c *stdioConn) Pid() int {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}
