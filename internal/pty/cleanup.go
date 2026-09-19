package pty

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// processExitCleanup coordinates StopAll for in-process (non-detached) Managers
// when the CLI receives SIGINT/SIGTERM or when CleanupNow is called (VAL-TERM-009).
// Detached sessions intentionally survive CLI exit and are not registered here.

var (
	cleanupMu      sync.Mutex
	cleanupMgrs    []*Manager
	cleanupOnce    sync.Once
	cleanupGrace   = 2 * time.Second
	signalsHooked  bool
	defaultManager *Manager // optional process-wide default for tools/CLI
)

// RegisterCleanup adds m to the set stopped on CLI exit signals.
// Safe to call multiple times with the same manager (deduped by pointer).
func RegisterCleanup(m *Manager) {
	if m == nil {
		return
	}
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	for _, existing := range cleanupMgrs {
		if existing == m {
			return
		}
	}
	cleanupMgrs = append(cleanupMgrs, m)
	ensureSignalHookLocked()
}

// UnregisterCleanup removes m from the cleanup set.
func UnregisterCleanup(m *Manager) {
	if m == nil {
		return
	}
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	out := cleanupMgrs[:0]
	for _, existing := range cleanupMgrs {
		if existing != m {
			out = append(out, existing)
		}
	}
	cleanupMgrs = out
}

// SetDefaultManager sets the process-wide Manager used by agent tools
// when no explicit manager is injected (in-process / tests).
func SetDefaultManager(m *Manager) {
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	defaultManager = m
	if m != nil {
		for _, existing := range cleanupMgrs {
			if existing == m {
				return
			}
		}
		cleanupMgrs = append(cleanupMgrs, m)
		ensureSignalHookLocked()
	}
}

// DefaultManager returns the process-wide Manager (may be nil).
func DefaultManager() *Manager {
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	return defaultManager
}

// CleanupNow stops all registered in-process managers. Detached sessions are
// unaffected (they live in separate worker processes).
func CleanupNow() {
	cleanupMu.Lock()
	mgrs := append([]*Manager(nil), cleanupMgrs...)
	grace := cleanupGrace
	cleanupMu.Unlock()
	for _, m := range mgrs {
		m.StopAll(grace)
	}
}

func ensureSignalHookLocked() {
	if signalsHooked {
		return
	}
	signalsHooked = true
	cleanupOnce.Do(func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			CleanupNow()
			// Re-raise default behavior for second signal: after cleanup, exit.
			signal.Stop(ch)
			// Best-effort: if a second interrupt arrives, process may hang on
			// Wait; callers should exit after CleanupNow via their own handlers.
		}()
	})
}
