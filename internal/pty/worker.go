package pty

import (
	"fmt"
	"os"
	"time"
)

// RunWorker is the in-process entry for the detached supervisor.
// It starts a PTY session, streams output to out.log, and updates meta.json
// until the process exits. Intended to be invoked as:
//
//	inferenesia terminal __worker --root R --id ID -- argv...
func RunWorker(root, id string, args []string) error {
	if root == "" {
		return fmt.Errorf("pty worker: root required")
	}
	if id == "" {
		return fmt.Errorf("pty worker: id required")
	}
	if len(args) == 0 {
		return fmt.Errorf("pty worker: command required")
	}

	meta, err := LoadMeta(root, id)
	if err != nil {
		// Create minimal meta if missing (should already exist from DetachStart).
		meta = SessionMeta{
			ID:       id,
			Command:  joinArgs(args),
			Args:     append([]string(nil), args...),
			Status:   StatusStarting,
			Started:  time.Now(),
			Detached: true,
		}
	}
	meta.WorkerPID = os.Getpid()
	_ = SaveMeta(root, meta)

	// Bounded out.log so a spammy process cannot fill the disk (VAL-TERM-006).
	logPath := OutputPath(root, id)
	logf, err := newBoundedLogWriter(logPath, DefaultMaxLogBytes)
	if err != nil {
		_ = UpdateMetaStatus(root, id, StatusError, nil, err.Error())
		return err
	}
	defer logf.Close()

	// Input FIFO for terminal_write / terminal write (VAL-TERM-008).
	if _, err := EnsureInputFIFO(root, id); err != nil {
		_ = UpdateMetaStatus(root, id, StatusError, nil, err.Error())
		return err
	}

	mgr := NewManager(meta.WorkDir)
	mgr.IDPrefix = "w"
	// Workers are detached: do not register for CLI-exit cleanup of parent.
	s, err := mgr.Start(StartOpts{
		ID:      id,
		Command: args,
		WorkDir: meta.WorkDir,
		OnOutput: func(b []byte) {
			_, _ = logf.Write(b)
			_ = logf.Sync()
		},
	})
	if err != nil {
		_ = UpdateMetaStatus(root, id, StatusError, nil, err.Error())
		return err
	}

	// Pump stdin from FIFO into the PTY until session ends.
	go pumpInputFIFO(root, id, s)

	// Publish running pid immediately.
	meta.PID = s.PID()
	meta.Status = StatusRunning
	meta.WorkerPID = os.Getpid()
	meta.Command = s.Command
	meta.Args = append([]string(nil), args...)
	_ = SaveMeta(root, meta)

	// Wait for process exit.
	s.Wait()

	code, ok := s.ExitCode()
	if !ok {
		code = -1
	}
	info := s.Info()
	status := info.Status
	if status == StatusRunning {
		status = StatusExited
	}
	// Prefer exited unless Stop set stopped — worker is the authority on natural exit.
	if status != StatusStopped {
		status = StatusExited
	}
	meta.Status = status
	meta.ExitCode = &code
	meta.Ended = time.Now()
	meta.PID = s.PID()
	if info.Error != "" {
		meta.Error = info.Error
	}
	_ = SaveMeta(root, meta)
	return nil
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := args[0]
	for _, a := range args[1:] {
		out += " " + a
	}
	return out
}
