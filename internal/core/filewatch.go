package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileChangeEvent is the payload streamed to desktop subscribers when a
// workspace file is mutated (by CLI, OS, or another process). It mirrors the
// ChatEventFileChanged shape so the renderer treats explorer refreshes
// uniformly (VAL-CROSS-003).
type FileChangeEvent struct {
	// Op is "write" | "create" | "remove" | "rename" (best-effort).
	Op string `json:"op"`
	// Path is the workspace-relative slash path (for explorer refresh / select).
	Path string `json:"path"`
	// Absolute is the absolute path (for diagnostics; not used by renderer fs ops).
	Absolute string `json:"absolute,omitempty"`
	// Removed is true when the file no longer exists after the event.
	Removed bool `json:"removed,omitempty"`
	// IsDir is true when the mutated path is a directory.
	IsDir bool `json:"is_dir,omitempty"`
	// Source is "external" for fsnotify-detected writes (CLI / OS / editor)
	// and "internal" for WriteGateway-echoed desktop mutations.
	Source string `json:"source"`
}

// fileWatcher watches the active workspace root with fsnotify and broadcasts
// FileChangeEvent values to all subscribers. This is what makes a CLI agent
// write visible in the desktop explorer within ~2s via a push event, not
// polling (VAL-CROSS-003).
type fileWatcher struct {
	mu   sync.Mutex
	w    *fsnotify.Watcher
	root string // absolute, symlink-evaluated workspace root

	// subscribers is the set of active SSE listeners. Each subscriber gets its
	// own buffered channel so a slow client never blocks the watcher loop.
	// The map key is the receive-only view the caller holds; Go channels are
	// comparable by underlying pointer so a recv-only channel still works as
	// a key (same allocation identity).
	subscribers map[<-chan FileChangeEvent]chan FileChangeEvent

	// stop closes the watcher goroutine.
	stop chan struct{}
	// done is closed when the watcher loop has exited.
	done chan struct{}

	// started tracks whether the goroutine is running.
	started bool

	// debounce deduplicates rapid Write/Create events on the same path (editors
	// frequently emit several events per save).
	debounce map[string]*time.Timer

	// suppressUntil lets the desktop suppress echoes of its own WriteGateway
	// mutations (which already flow through the chat SSE as FileChanged). A CLI
	// write is external and must NOT be suppressed.
	suppress map[string]time.Time
}

// newFileWatcher constructs an idle watcher (no root yet). Use setRoot to
// (re)attach to the active workspace.
func newFileWatcher() *fileWatcher {
	return &fileWatcher{
		subscribers: make(map[<-chan FileChangeEvent]chan FileChangeEvent),
		debounce:    make(map[string]*time.Timer),
		suppress:    make(map[string]time.Time),
	}
}

// ensureStarted lazily opens the fsnotify watcher + goroutine on first use.
// Cheap no-op when already running. Returns an error only if fsnotify fails
// to initialise (e.g. on unsupported platforms); the rest of the app still
// functions without file watching — explorer just won't auto-refresh.
func (fw *fileWatcher) ensureStarted() error {
	fw.mu.Lock()
	if fw.started && fw.w != nil {
		fw.mu.Unlock()
		return nil
	}
	fw.mu.Unlock()
	return fw.recreateWatcher()
}

// setRoot attaches the watcher to a new workspace root (recursively). Pass an
// empty path to detach from any root (e.g. when the last workspace closes).
// Safe to call repeatedly. Detach recreates the fsnotify Watcher (O(1)) instead
// of walking the previous tree to Remove each path.
func (fw *fileWatcher) setRoot(root string) error {
	if root == "" {
		fw.closeRoot()
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("filewatch: abs root: %w", err)
	}
	if eval, err := filepath.EvalSymlinks(abs); err == nil {
		abs = eval
	}
	abs = filepath.Clean(abs)
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		fw.closeRoot()
		return nil
	}

	fw.mu.Lock()
	if fw.root == abs && fw.started && fw.w != nil {
		fw.mu.Unlock()
		return nil
	}
	fw.mu.Unlock()

	if err := fw.recreateWatcher(); err != nil {
		return err
	}

	fw.mu.Lock()
	fw.root = abs
	w := fw.w
	fw.mu.Unlock()
	return addWatchTree(w, abs)
}

// recreateWatcher stops the current loop and opens a fresh fsnotify Watcher so
// the previous watch set is discarded without walking it.
func (fw *fileWatcher) recreateWatcher() error {
	fw.mu.Lock()
	var done <-chan struct{}
	if fw.started && fw.stop != nil {
		select {
		case <-fw.stop:
		default:
			close(fw.stop)
		}
		done = fw.done
	}
	old := fw.w
	fw.w = nil
	fw.started = false
	fw.root = ""
	fw.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("filewatch: fsnotify: %w", err)
	}
	fw.mu.Lock()
	fw.w = w
	fw.stop = make(chan struct{})
	fw.done = make(chan struct{})
	fw.started = true
	fw.debounce = make(map[string]*time.Timer)
	go fw.loop(w, fw.stop, fw.done)
	fw.mu.Unlock()
	return nil
}

// maxWatchDirs caps recursive fsnotify watches so a huge monorepo cannot
// exhaust process FDs (EMFILE → SQLite/chat/history and HTTP accept fail).
const maxWatchDirs = 1500

// maxWatchDepth limits how deep we recurse from the workspace root.
const maxWatchDepth = 6

func addWatchTree(w *fsnotify.Watcher, abs string) error {
	if w == nil || abs == "" {
		return nil
	}
	count := 0
	return filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if path != abs && skipWatchDir(base) {
			return filepath.SkipDir
		}
		if path != abs {
			rel, relErr := filepath.Rel(abs, path)
			if relErr != nil {
				return filepath.SkipDir
			}
			depth := 1
			if rel != "." {
				depth = strings.Count(rel, string(os.PathSeparator)) + 1
			}
			if depth > maxWatchDepth {
				return filepath.SkipDir
			}
		}
		if count >= maxWatchDirs {
			return filepath.SkipAll
		}
		if err := w.Add(path); err == nil {
			count++
		}
		return nil
	})
}

func skipWatchDir(base string) bool {
	switch base {
	case ".git", "node_modules", ".DS_Store", "dist", "build", ".next", ".turbo",
		"vendor", "coverage", ".cache", "tmp", "temp", "__pycache__", ".venv", "venv",
		"target", "out", ".idea", ".vscode", "Pods", "DerivedData", ".gradle",
		"bower_components", ".pnpm-store", ".yarn", "site-packages":
		return true
	default:
		return false
	}
}

// closeRoot drops the current root watch set (used by setRoot("")).
func (fw *fileWatcher) closeRoot() {
	fw.mu.Lock()
	var done <-chan struct{}
	if fw.started && fw.stop != nil {
		select {
		case <-fw.stop:
		default:
			close(fw.stop)
		}
		done = fw.done
	}
	old := fw.w
	fw.w = nil
	fw.root = ""
	fw.started = false
	fw.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

// suppressPath marks a path as "our own WriteGateway mutation" for a short
// window so the fsnotify echo does not double-fire. Called by desktop fs ops
// after they invoke WriteGateway. CLI writes are never suppressed.
func (fw *fileWatcher) suppressPath(abs string) {
	if abs == "" {
		return
	}
	fw.mu.Lock()
	fw.suppress[abs] = time.Now().Add(400 * time.Millisecond)
	fw.mu.Unlock()
}

// subscribe returns a buffered channel of FileChangeEvent values. The caller
// MUST call unsubscribeByRecv when the SSE client disconnects to avoid leaks.
// Returns a receive-only channel so callers cannot accidentally write to it.
func (fw *fileWatcher) subscribe() <-chan FileChangeEvent {
	ch := make(chan FileChangeEvent, 32)
	fw.mu.Lock()
	fw.subscribers[ch] = ch
	fw.mu.Unlock()
	return ch
}

// unsubscribeByRecv removes a previously-subscribed channel using its
// receive-only view. Safe to call with a channel that was already removed.
func (fw *fileWatcher) unsubscribeByRecv(ch <-chan FileChangeEvent) {
	fw.mu.Lock()
	if bidir, ok := fw.subscribers[ch]; ok {
		delete(fw.subscribers, ch)
		close(bidir)
	}
	fw.mu.Unlock()
}

// close stops the watcher goroutine and closes the fsnotify Watcher. Called
// on Service shutdown / process exit.
func (fw *fileWatcher) close() {
	fw.mu.Lock()
	if !fw.started {
		fw.mu.Unlock()
		return
	}
	stop := fw.stop
	done := fw.done
	w := fw.w
	fw.started = false
	fw.root = ""
	fw.w = nil
	for key, bidir := range fw.subscribers {
		close(bidir)
		delete(fw.subscribers, key)
	}
	fw.mu.Unlock()
	if stop != nil {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
	if w != nil {
		_ = w.Close()
	}
	if done != nil {
		<-done
	}
}

// loop is the fsnotify event loop. It converts fsnotify events into
// FileChangeEvent values, debounces, and broadcasts to subscribers.
// Local w/stop/done avoid races when setRoot recreates the watcher.
func (fw *fileWatcher) loop(w *fsnotify.Watcher, stop <-chan struct{}, done chan struct{}) {
	defer close(done)
	if w == nil {
		return
	}
	for {
		select {
		case <-stop:
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			fw.handleEvent(ev)
		case <-w.Errors:
		}
	}
}

// handleEvent converts one fsnotify event into a broadcast FileChangeEvent,
// applying debounce + suppress logic.
func (fw *fileWatcher) handleEvent(ev fsnotify.Event) {
	fw.mu.Lock()
	root := fw.root
	if root == "" {
		fw.mu.Unlock()
		return
	}
	// Suppress our own WriteGateway echoes for a short window.
	if until, ok := fw.suppress[ev.Name]; ok {
		if time.Now().Before(until) {
			fw.mu.Unlock()
			return
		}
		delete(fw.suppress, ev.Name)
	}
	// Compute relative path. Events outside the root are ignored.
	rel, err := filepath.Rel(root, ev.Name)
	fw.mu.Unlock()
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "..") {
		return
	}

	op := classifyOp(ev.Op)
	if op == "" {
		return // Ignore Chmod (not a content change visible to explorer).
	}

	// Debounce per path: editors (and WriteGateway rename+write) emit several
	// events per save. Coalesce into a single FileChanged within 120ms.
	key := ev.Name
	fw.mu.Lock()
	if t, ok := fw.debounce[key]; ok {
		t.Stop()
	}
	removed := ev.Op&fsnotify.Remove != 0 || ev.Op&fsnotify.Rename != 0
	isDir := false
	if st, err := os.Stat(ev.Name); err == nil {
		isDir = st.IsDir()
	} else if removed {
		// Best-effort: assume file for removed paths.
		isDir = false
	}
	abs := ev.Name
	opFinal := op
	relFinal := rel
	removedFinal := removed
	isDirFinal := isDir
	fw.debounce[key] = time.AfterFunc(120*time.Millisecond, func() {
		fw.mu.Lock()
		delete(fw.debounce, key)
		subs := make([]chan FileChangeEvent, 0, len(fw.subscribers))
		for _, bidir := range fw.subscribers {
			subs = append(subs, bidir)
		}
		fw.mu.Unlock()
		evt := FileChangeEvent{
			Op:       opFinal,
			Path:     relFinal,
			Absolute: abs,
			Removed:  removedFinal,
			IsDir:    isDirFinal,
			Source:   "external",
		}
		for _, ch := range subs {
			select {
			case ch <- evt:
			default:
				// Subscriber buffer full — drop event for that client rather
				// than blocking the watcher. Client will re-sync on next event.
			}
		}
	})
	fw.mu.Unlock()
}

// classifyOp maps fsnotify bitmask to a short string. Returns "" for Chmod.
func classifyOp(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create != 0:
		return "create"
	case op&fsnotify.Remove != 0:
		return "remove"
	case op&fsnotify.Rename != 0:
		return "rename"
	case op&fsnotify.Write != 0:
		return "write"
	default:
		return ""
	}
}
