package pty

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DetachStart spawns a detached worker process that owns a PTY session so the
// calling CLI can return immediately while the process keeps running
// (VAL-TERM-001 background/session mode).
//
// workerBinary is the path to this binary (or another that implements
// `terminal __worker`). The worker writes meta.json + out.log under root.
func DetachStart(root, workerBinary string, opts StartOpts) (SessionMeta, error) {
	args, display, err := resolveArgs(opts)
	if err != nil {
		return SessionMeta{}, err
	}
	wd := strings.TrimSpace(opts.WorkDir)
	if wd == "" {
		wd, _ = os.Getwd()
	}
	if wd != "" {
		if abs, e := filepath.Abs(wd); e == nil {
			wd = abs
		}
	}

	id := strings.TrimSpace(opts.ID)
	if id == "" {
		// Use a Manager solely for id generation.
		m := NewManager(wd)
		id = m.newID()
	}

	meta := SessionMeta{
		ID:       id,
		Command:  display,
		Args:     append([]string(nil), args...),
		WorkDir:  wd,
		Status:   StatusStarting,
		Started:  time.Now(),
		Detached: true,
	}
	if err := SaveMeta(root, meta); err != nil {
		return SessionMeta{}, err
	}
	// Ensure output log exists.
	if err := os.MkdirAll(SessionDir(root, id), 0o700); err != nil {
		return SessionMeta{}, err
	}
	f, err := os.OpenFile(OutputPath(root, id), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return SessionMeta{}, err
	}
	_ = f.Close()

	// Build worker args: inferenesia terminal __worker --root R --id ID -- -- argv...
	wargs := []string{
		"terminal", "__worker",
		"--root", root,
		"--id", id,
		"--",
	}
	wargs = append(wargs, args...)

	cmd := exec.Command(workerBinary, wargs...)
	cmd.Dir = wd
	cmd.Env = append(os.Environ(), PersistRootEnv+"="+root)
	// Fully detach: new session, no controlling terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Discard worker's own stdout/stderr (PTY output goes to out.log).
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		_ = UpdateMetaStatus(root, id, StatusError, nil, err.Error())
		return SessionMeta{}, fmt.Errorf("pty detach start: %w", err)
	}

	meta.WorkerPID = cmd.Process.Pid
	_ = SaveMeta(root, meta)

	// Wait briefly for worker to populate pid/status=running.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := LoadMeta(root, id)
		if err == nil && got.PID > 0 && (got.Status == StatusRunning || got.Status == StatusExited || got.Status == StatusStopped) {
			// Release the worker process so it is not reaped as our child
			// (allow it to continue independently).
			_ = cmd.Process.Release()
			return got, nil
		}
		// Worker may have failed quickly.
		if err == nil && got.Status == StatusError {
			_ = cmd.Process.Release()
			return got, fmt.Errorf("pty worker error: %s", got.Error)
		}
		time.Sleep(25 * time.Millisecond)
	}
	// Still starting — return best-effort meta with worker pid.
	got, _ := LoadMeta(root, id)
	if got.ID == "" {
		got = meta
	}
	_ = cmd.Process.Release()
	return got, nil
}

// DetachStop stops a detached session by signaling its process group / pid.
func DetachStop(root, id string, grace time.Duration) (SessionMeta, error) {
	if grace <= 0 {
		grace = 2 * time.Second
	}
	meta, err := LoadMeta(root, id)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionMeta{}, ErrNotFound
		}
		return SessionMeta{}, err
	}
	if meta.Status == StatusExited || meta.Status == StatusStopped {
		// Safe no-op (VAL-TERM-011).
		return meta, nil
	}

	// Prefer signaling the PTY child process group.
	if meta.PID > 0 {
		_ = syscall.Kill(-meta.PID, syscall.SIGTERM)
		_ = syscall.Kill(meta.PID, syscall.SIGTERM)
	}
	if meta.WorkerPID > 0 {
		_ = syscall.Kill(meta.WorkerPID, syscall.SIGTERM)
	}

	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		got, err := LoadMeta(root, id)
		if err == nil && (got.Status == StatusExited || got.Status == StatusStopped) {
			// Normalize to stopped if we requested stop.
			if got.Status == StatusExited {
				got.Status = StatusStopped
				_ = SaveMeta(root, got)
			}
			return got, nil
		}
		// Process gone?
		if meta.PID > 0 && syscall.Kill(meta.PID, 0) != nil {
			code := gotExitOr(got, 0)
			got.Status = StatusStopped
			got.ExitCode = &code
			got.Ended = time.Now()
			_ = SaveMeta(root, got)
			return got, nil
		}
		time.Sleep(50 * time.Millisecond)
		// refresh
		if g, e := LoadMeta(root, id); e == nil {
			got = g
		}
		_ = got
	}

	// Force kill.
	if meta.PID > 0 {
		_ = syscall.Kill(-meta.PID, syscall.SIGKILL)
		_ = syscall.Kill(meta.PID, syscall.SIGKILL)
	}
	if meta.WorkerPID > 0 {
		_ = syscall.Kill(meta.WorkerPID, syscall.SIGKILL)
	}
	time.Sleep(100 * time.Millisecond)

	got, _ := LoadMeta(root, id)
	if got.ID == "" {
		got = meta
	}
	code := 128 + int(syscall.SIGKILL)
	if got.ExitCode != nil {
		code = *got.ExitCode
	}
	got.Status = StatusStopped
	got.ExitCode = &code
	got.Ended = time.Now()
	_ = SaveMeta(root, got)
	return got, nil
}

func gotExitOr(m SessionMeta, def int) int {
	if m.ExitCode != nil {
		return *m.ExitCode
	}
	return def
}

// ProcessAlive reports whether pid responds to signal 0.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// ParsePID is a tiny helper for tests/CLI.
func ParsePID(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
