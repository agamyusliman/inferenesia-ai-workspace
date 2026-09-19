package writegate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Default messages for empty stacks (VAL-UNDO-003, VAL-UNDO-006).
const (
	MsgNothingToUndo = "nothing to undo"
	MsgNothingToRedo = "nothing to redo"
)

// Soft-fail / external-edit messages (VAL-UNDO-013).
const (
	MsgRedoExternalEdit = "redo skipped: file changed outside undo stack (external edit)"
	MsgRedoPartialSkip  = "redo partially skipped: some files changed outside undo stack"
)

// ChangeEvent is emitted when a path is mutated by write, undo, or redo.
type ChangeEvent struct {
	// Op is "write", "undo", or "redo".
	Op string
	// Path is absolute.
	Path string
	// Rel is slash-normalized relative to the workspace root.
	Rel string
	// Content is the new on-disk content after the op (nil if file removed).
	Content []byte
	// Removed is true when the file no longer exists after the op.
	Removed bool
}

// StackUnit is one undo unit. Default granularity is one agent turn
// (BeginTurn…EndTurn may batch multiple WriteFile entries). Standalone
// writes outside a turn are one entry per unit.
type StackUnit struct {
	// ID is a stable unit id (for persistence / timeline).
	ID string `json:"id"`
	// TurnID groups entries from the same agent turn when set.
	TurnID string `json:"turn_id,omitempty"`
	// Entries are the file mutations in this unit (usually one).
	Entries []Entry `json:"entries"`
	// CreatedAt is when the unit was pushed.
	CreatedAt time.Time `json:"created_at"`
}

// UndoResult is the outcome of Undo or Redo.
type UndoResult struct {
	// Message is a human-readable summary (always set).
	Message string
	// Noop is true when the stack was empty (safe no-op).
	Noop bool
	// Unit is the applied stack unit (nil when Noop).
	Unit *StackUnit
	// Changes lists paths that were restored/reapplied.
	Changes []ChangeEvent
}

// OnChange is an optional listener for FileChanged-style events.
type OnChange func(ChangeEvent)

// --- Gateway extensions for undo/redo ---

// SetOnChange registers a callback invoked after write, undo, and redo.
func (g *Gateway) SetOnChange(fn OnChange) {
	if g == nil {
		return
	}
	g.onChange = fn
}

// BeginTurn starts batching subsequent WriteFile calls into one StackUnit
// (VAL-UNDO-004 multi-file turn; VAL-UNDO-005 turn-scoped LIFO). Nested
// BeginTurn ends the previous open turn first. An empty turnID is replaced
// with a generated id.
func (g *Gateway) BeginTurn(turnID string) {
	if g == nil {
		return
	}
	// Close any previously open turn so units stay well-formed.
	if g.openTurn != nil {
		g.EndTurn()
	}
	id := strings.TrimSpace(turnID)
	if id == "" {
		id = newUnitID()
	}
	g.openTurn = &openTurn{id: id}
}

// EndTurn commits the open turn as a single undo unit (if it has any writes).
// Safe no-op when no turn is open or the turn wrote nothing.
func (g *Gateway) EndTurn() {
	if g == nil || g.openTurn == nil {
		return
	}
	ot := g.openTurn
	g.openTurn = nil
	if len(ot.entries) == 0 {
		return
	}
	entries := make([]Entry, len(ot.entries))
	for i, e := range ot.entries {
		entries[i] = cloneEntry(e)
	}
	unit := StackUnit{
		ID:        newUnitID(),
		TurnID:    ot.id,
		Entries:   entries,
		CreatedAt: time.Now().UTC(),
	}
	g.undoStack = append(g.undoStack, unit)
	// Batch already invalidated redo on first write of the turn.
}

// InTurn reports whether a BeginTurn is active (writes are being batched).
func (g *Gateway) InTurn() bool {
	return g != nil && g.openTurn != nil
}

// WriteFile snapshots, writes content (source=fs), records an Entry, and updates
// the undo stack. Inside an open turn the entry is batched; otherwise it becomes
// its own unit. Clears redo (new write invalidates redo).
func (g *Gateway) WriteFile(path string, content []byte) (Entry, error) {
	return g.WriteFileWithSource(path, content, SourceFS)
}

// WriteFileWithSource is WriteFile with an explicit mutation source tag
// (SourceFS, SourceMultibrain, SourceDiagram). Empty source defaults to SourceFS.
func (g *Gateway) WriteFileWithSource(path string, content []byte, source string) (Entry, error) {
	return g.WriteFileTurn(path, content, "", source)
}

// DeleteFile snapshots then removes path, recording AfterAbsent so undo can restore
// Snapshot.Before (or keep absent if the file did not exist) (VAL-UNDO-010).
// Deleting a missing file is a recorded no-op-like unit with Before=nil, Existed=false,
// AfterAbsent=true (so undo of a later recreate can still remove cleanly when batched).
func (g *Gateway) DeleteFile(path string) (Entry, error) {
	return g.DeleteFileWithSource(path, SourceFS)
}

// DeleteFileWithSource is DeleteFile with an explicit mutation source tag.
func (g *Gateway) DeleteFileWithSource(path string, source string) (Entry, error) {
	if g == nil {
		return Entry{}, fmt.Errorf("writegate: gateway not configured")
	}
	source = normalizeSource(source)
	return g.Mutate(path, source, nil, func(abs string) error {
		err := os.Remove(abs)
		if err != nil && os.IsNotExist(err) {
			// Missing file: treat as successful delete-to-absent for stack purposes.
			return nil
		}
		return err
	})
}

// WriteMultibrain writes under .multibrain/ via WriteGateway (VAL-UNDO-007).
// Path must resolve under .multibrain (relative or absolute still sandboxed).
func (g *Gateway) WriteMultibrain(path string, content []byte) (Entry, error) {
	if g == nil {
		return Entry{}, fmt.Errorf("writegate: gateway not configured")
	}
	abs, rel, err := g.ResolveInWorkspace(path)
	if err != nil {
		return Entry{}, err
	}
	_ = abs
	if !isMultibrainRel(rel) {
		return Entry{}, fmt.Errorf("writegate: multibrain path required (got %q, want under .multibrain/)", rel)
	}
	return g.WriteFileWithSource(path, content, SourceMultibrain)
}

// WriteDiagram writes diagram sources under docs/diagrams/ via WriteGateway (VAL-UNDO-007).
// Path must resolve under docs/diagrams (or be a diagram-ish path under docs/).
func (g *Gateway) WriteDiagram(path string, content []byte) (Entry, error) {
	if g == nil {
		return Entry{}, fmt.Errorf("writegate: gateway not configured")
	}
	abs, rel, err := g.ResolveInWorkspace(path)
	if err != nil {
		return Entry{}, err
	}
	_ = abs
	if !isDiagramRel(rel) {
		return Entry{}, fmt.Errorf("writegate: diagram path required (got %q, want under docs/diagrams/)", rel)
	}
	return g.WriteFileWithSource(path, content, SourceDiagram)
}

// WriteResearch writes a research artifact under docs/research/ via WriteGateway
// (VAL-CROSS-005: skill + browser research path; every research write is
// undoable just like fs/multibrain/diagram mutations, VAL-UNDO-007).
// Path must resolve under docs/research and be a .md file (notes artifact).
func (g *Gateway) WriteResearch(path string, content []byte) (Entry, error) {
	if g == nil {
		return Entry{}, fmt.Errorf("writegate: gateway not configured")
	}
	abs, rel, err := g.ResolveInWorkspace(path)
	if err != nil {
		return Entry{}, err
	}
	_ = abs
	if !isResearchRel(rel) {
		return Entry{}, fmt.Errorf("writegate: research path required (got %q, want under docs/research/*.md)", rel)
	}
	return g.WriteFileWithSource(path, content, SourceResearch)
}

// RenameFile moves src → dst inside the workspace, recording both a delete
// of src and a write of dst on the undo stack so Undo restores the original
// file at src and removes dst. Both paths are sandbox-checked; either being
// outside root fails cleanly without touching disk.
//
// Use this for UI rename / clipboard cut so the mutation lands on the same
// undo stack as agent writes (VAL-UNDO-007 single-write-path invariant).
// If src is a directory, RenameFile falls back to `os.Rename` inside a Mutate
// call so subtree moves stay atomic; undo of a directory rename is not
// currently supported (returns the entry with Existed=false).
func (g *Gateway) RenameFile(src, dst string, source string) ([]Entry, error) {
	if g == nil {
		return nil, fmt.Errorf("writegate: gateway not configured")
	}
	source = normalizeSource(source)
	srcAbs, _, err := g.ResolveInWorkspace(src)
	if err != nil {
		return nil, err
	}
	dstAbs, _, err := g.ResolveInWorkspace(dst)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(srcAbs)
	if err != nil {
		return nil, fmt.Errorf("writegate: rename src: %w", err)
	}
	if info.IsDir() {
		// Directory move: use os.Rename inside Mutate on dst so a snapshot of dst
		// is captured (if any pre-existing content), then rename atomically.
		entry, err := g.Mutate(dst, source, nil, func(dstResolved string) error {
			return os.Rename(srcAbs, dstResolved)
		})
		if err != nil {
			return nil, err
		}
		return []Entry{entry}, nil
	}
	// File move: WriteFile(dst, srcBytes) → DeleteFile(src). Both entries land on
	// the current turn (or standalone) so Undo restores the original layout.
	bytes, err := os.ReadFile(srcAbs)
	if err != nil {
		return nil, fmt.Errorf("writegate: rename read src: %w", err)
	}
	writeEntry, err := g.WriteFileWithSource(dst, bytes, source)
	if err != nil {
		return nil, err
	}
	deleteEntry, err := g.DeleteFileWithSource(src, source)
	if err != nil {
		// Best-effort rollback: remove dst we just created so the tree is not
		// left with a duplicate. Undo can still recover from the write entry.
		_ = os.Remove(dstAbs)
		return []Entry{writeEntry}, fmt.Errorf("writegate: rename delete src: %w", err)
	}
	return []Entry{writeEntry, deleteEntry}, nil
}

// MakeDir creates a directory under the workspace, recording a lightweight
// entry so undo removes it if the directory is still empty at undo time.
// Existing directories are a no-op (no stack entry). Used for UI new-folder
// and paste-into-new-parent so directory creation obeys the single-write
// path invariant.
func (g *Gateway) MakeDir(path string, source string) (Entry, error) {
	if g == nil {
		return Entry{}, fmt.Errorf("writegate: gateway not configured")
	}
	source = normalizeSource(source)
	abs, _, err := g.ResolveInWorkspace(path)
	if err != nil {
		return Entry{}, err
	}
	if info, err := os.Stat(abs); err == nil {
		if info.IsDir() {
			return Entry{}, nil
		}
		return Entry{}, fmt.Errorf("writegate: mkdir: path exists and is not a directory")
	}
	return g.Mutate(path, source, nil, func(dstAbs string) error {
		return os.MkdirAll(dstAbs, 0o755)
	})
}

// MutateFunc applies the on-disk mutation for path abs after Prepare has
// captured the pre-mutation snapshot. Implementations may partially write
// then return an error (VAL-UNDO-008 mid-op failure path).
type MutateFunc func(abs string) error

// errInjectedWriteFail is reserved for tests; production mutators use real I/O errors.
// Defined here as a package-level sentinel that tests can reuse via exported wrapper.
var errInjectedWriteFail = fmt.Errorf("writegate: injected write failure")

// Mutate snapshots path first, then runs fn. If fn fails after changing the
// file, a recovery Entry is still pushed so Undo can restore Snapshot.Before
// (VAL-UNDO-008). If fn fails without changing disk, no stack entry is pushed.
func (g *Gateway) Mutate(path string, source string, intendedAfter []byte, fn MutateFunc) (Entry, error) {
	if g == nil {
		return Entry{}, fmt.Errorf("writegate: gateway not configured")
	}
	if fn == nil {
		return Entry{}, fmt.Errorf("writegate: mutate func is required")
	}
	source = normalizeSource(source)

	snap, err := g.Prepare(path)
	if err != nil {
		return Entry{}, err
	}

	// Capture pre-hash for "disk changed?" detection after failure.
	preState, preErr := os.ReadFile(snap.Path)
	preExisted := preErr == nil
	if preErr != nil && !os.IsNotExist(preErr) {
		// Unreadable target — still attempt mutate; recovery uses snap.Before.
		preExisted = snap.Existed
		preState = append([]byte(nil), snap.Before...)
	}

	dir := filepath.Dir(snap.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Entry{}, fmt.Errorf("writegate: mkdir %s: %w", dir, err)
	}

	mutErr := fn(snap.Path)

	// Read post-mutate disk for "changed?" and event content.
	postState, postErr := os.ReadFile(snap.Path)
	postExists := postErr == nil
	if postErr != nil && !os.IsNotExist(postErr) {
		// Treat unreadable as "changed" to force recovery entry when we had a snap.
		postExists = false
	}

	diskChanged := (preExisted != postExists) ||
		(postExists && !bytes.Equal(preState, postState)) ||
		(!preExisted && postExists)

	// AfterAbsent: DeleteFile (intendedAfter==nil) or successful remove leaves path gone.
	// WriteFile always passes a non-nil intendedAfter slice (possibly empty).
	intendedDelete := intendedAfter == nil
	afterAbsent := false
	after := append([]byte(nil), intendedAfter...)
	if mutErr == nil && postExists {
		after = append([]byte(nil), postState...)
		afterAbsent = false
	}
	if mutErr == nil && !postExists {
		after = nil
		afterAbsent = true
	}
	// Partial failure that deleted the file: mark absent for recovery.
	if mutErr != nil && !postExists && diskChanged {
		afterAbsent = true
		after = nil
	}

	entry := Entry{Snapshot: snap, After: after, AfterAbsent: afterAbsent, Source: source}

	// Clean failure, no disk change → no stack push.
	// Exception: DeleteFile on already-missing path still records (VAL-UNDO-010 batching).
	recordMissingDelete := mutErr == nil && intendedDelete && !preExisted && !postExists
	if mutErr != nil && !diskChanged {
		return entry, mutErr
	}
	if !diskChanged && !recordMissingDelete && mutErr != nil {
		return entry, mutErr
	}

	// Success (incl. identical rewrite), partial failure with disk change, or missing delete.
	g.recordEntry(entry, "")

	if g.onChange != nil {
		content := after
		removed := afterAbsent
		if postExists {
			content = append([]byte(nil), postState...)
			removed = false
		}
		g.onChange(ChangeEvent{
			Op:      "write",
			Path:    snap.Path,
			Rel:     snap.Rel,
			Content: content,
			Removed: removed,
		})
	}

	if mutErr != nil {
		return entry, fmt.Errorf("writegate: mutate %s failed after snapshot: %w", snap.Rel, mutErr)
	}
	return entry, nil
}

// WriteFileTurn is WriteFile with an optional turn id and source tag.
// When a turn is already open via BeginTurn, the entry joins that turn (turnID
// on the call is ignored for stacking). When no turn is open and turnID is
// non-empty, a single-entry unit is tagged with that turnID.
func (g *Gateway) WriteFileTurn(path string, content []byte, turnID string, source ...string) (Entry, error) {
	if g == nil {
		return Entry{}, fmt.Errorf("writegate: gateway not configured")
	}
	src := SourceFS
	if len(source) > 0 && strings.TrimSpace(source[0]) != "" {
		src = normalizeSource(source[0])
	}
	data := append([]byte(nil), content...)
	return g.Mutate(path, src, data, func(abs string) error {
		return writeBytes(abs, data)
	})
}

// recordEntry appends to flat history + undo stack (batched when in a turn).
func (g *Gateway) recordEntry(entry Entry, turnID string) {
	g.Entries = append(g.Entries, entry)

	// New mutation invalidates redo (docs/undo-revert.md).
	g.redoStack = nil

	if g.openTurn != nil {
		// Batch into open agent turn (VAL-UNDO-004).
		g.openTurn.entries = append(g.openTurn.entries, cloneEntry(entry))
		return
	}
	// Standalone unit (backward-compatible single-write undo).
	tid := strings.TrimSpace(turnID)
	unit := StackUnit{
		ID:        newUnitID(),
		TurnID:    tid,
		Entries:   []Entry{cloneEntry(entry)},
		CreatedAt: time.Now().UTC(),
	}
	g.undoStack = append(g.undoStack, unit)
}

func normalizeSource(source string) string {
	s := strings.ToLower(strings.TrimSpace(source))
	switch s {
	case SourceFS, SourceMultibrain, SourceDiagram, SourceUser, SourceResearch:
		return s
	case "":
		return SourceFS
	default:
		return s
	}
}

func isMultibrainRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	return rel == ".multibrain" || strings.HasPrefix(rel, ".multibrain/")
}

// isResearchRel reports whether rel targets a research artifact under docs/research/.
// Research artifacts are markdown notes (VAL-CROSS-005) and must live under
// docs/research so the sandbox keeps them workspace-scoped and undoable.
func isResearchRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "docs/research" || strings.HasPrefix(rel, "docs/research/") {
		return strings.ToLower(filepath.Ext(rel)) == ".md"
	}
	return false
}

func isDiagramRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "docs/diagrams" || strings.HasPrefix(rel, "docs/diagrams/") {
		return true
	}
	// Allow docs/*.mmd etc. for simple diagram saves under docs/.
	if strings.HasPrefix(rel, "docs/") {
		ext := strings.ToLower(filepath.Ext(rel))
		switch ext {
		case ".mmd", ".mermaid", ".dbml", ".puml", ".plantuml", ".excalidraw":
			return true
		}
	}
	return false
}

// Undo restores the last stack unit to pre-agent (pre-write) content.
// This restores the dirty-before snapshot, NOT git HEAD (VAL-UNDO-001/002).
// Empty stack is a safe no-op with MsgNothingToUndo (VAL-UNDO-006).
// Open turns are committed before undo so in-progress batch writes are stackable.
func (g *Gateway) Undo() UndoResult {
	if g == nil {
		return UndoResult{Message: MsgNothingToUndo, Noop: true}
	}
	if g.openTurn != nil {
		g.EndTurn()
	}
	if len(g.undoStack) == 0 {
		return UndoResult{Message: MsgNothingToUndo, Noop: true}
	}
	unit := g.undoStack[len(g.undoStack)-1]
	g.undoStack = g.undoStack[:len(g.undoStack)-1]

	var changes []ChangeEvent
	// Restore in reverse order within the unit so nested paths behave predictably.
	for i := len(unit.Entries) - 1; i >= 0; i-- {
		ev, err := g.restoreBefore(unit.Entries[i])
		if err != nil {
			// Soft-fail one path but still push redo for successful paths.
			// Report error in message when any fail.
			msg := fmt.Sprintf("undo partially failed: %v", err)
			if len(changes) == 0 {
				// Push unit back if nothing applied? Prefer leave stack consistent:
				// move unit to redo only after attempts; record failures in message.
				g.redoStack = append(g.redoStack, unit)
				return UndoResult{Message: msg, Noop: false, Unit: &unit, Changes: changes}
			}
			g.redoStack = append(g.redoStack, unit)
			return UndoResult{Message: msg, Noop: false, Unit: &unit, Changes: changes}
		}
		changes = append(changes, ev)
		if g.onChange != nil {
			g.onChange(ev)
		}
	}
	g.redoStack = append(g.redoStack, unit)

	paths := relList(unit.Entries)
	msg := fmt.Sprintf("undid %d file(s) to pre-agent state: %s", len(unit.Entries), strings.Join(paths, ", "))
	u := unit
	return UndoResult{Message: msg, Noop: false, Unit: &u, Changes: changes}
}

// Redo reapplies the last undone unit (agent "after" content).
// Empty redo stack is a safe no-op with MsgNothingToRedo (VAL-UNDO-003).
//
// VAL-UNDO-013: if the current disk content for a path no longer matches the
// expected pre-redo state (Snapshot.Before after undo, or absent when
// !Existed), redo soft-skips that path with a clear message rather than
// clobbering the external edit. Skipped paths drop from the unit when any
// path applies; if every path is external, the unit is discarded from redo
// (soft fail) and not moved to undo.
func (g *Gateway) Redo() UndoResult {
	if g == nil {
		return UndoResult{Message: MsgNothingToRedo, Noop: true}
	}
	if g.openTurn != nil {
		// Committing an open turn invalidates redo; flush for stack consistency.
		g.EndTurn()
	}
	if len(g.redoStack) == 0 {
		return UndoResult{Message: MsgNothingToRedo, Noop: true}
	}
	unit := g.redoStack[len(g.redoStack)-1]
	g.redoStack = g.redoStack[:len(g.redoStack)-1]

	var (
		changes   []ChangeEvent
		applied   []Entry
		skipped   []string
		hardFails []string
	)

	for _, entry := range unit.Entries {
		if mismatch, reason := externalRedoMismatch(entry); mismatch {
			rel := entry.Snapshot.Rel
			if rel == "" {
				rel = filepath.Base(entry.Snapshot.Path)
			}
			skipped = append(skipped, fmt.Sprintf("%s (%s)", rel, reason))
			continue
		}
		ev, err := g.reapplyAfter(entry)
		if err != nil {
			hardFails = append(hardFails, err.Error())
			continue
		}
		changes = append(changes, ev)
		applied = append(applied, entry)
		if g.onChange != nil {
			g.onChange(ev)
		}
	}

	// Full soft-skip: nothing applied, only external mismatches.
	if len(applied) == 0 && len(skipped) > 0 && len(hardFails) == 0 {
		// Drop unit from redo (invalidated); do not push to undo.
		msg := MsgRedoExternalEdit
		if len(skipped) > 1 {
			msg = fmt.Sprintf("%s: %s", MsgRedoExternalEdit, strings.Join(skipped, "; "))
		} else if len(skipped) == 1 {
			msg = fmt.Sprintf("%s: %s", MsgRedoExternalEdit, skipped[0])
		}
		u := unit
		return UndoResult{Message: msg, Noop: true, Unit: &u, Changes: nil}
	}

	if len(applied) == 0 && len(hardFails) > 0 {
		msg := fmt.Sprintf("redo partially failed: %s", strings.Join(hardFails, "; "))
		// Put unit back on redo so user can retry? Prefer leave off both stacks
		// after hard I/O failure mid-unit is rare; push to undo empty changes
		// would mislead. Return unit without stacking.
		u := unit
		return UndoResult{Message: msg, Noop: false, Unit: &u, Changes: changes}
	}

	// Partial apply: only applied entries move to undo so a future undo restores
	// what we actually redid (and skipped external paths stay as-is on disk).
	newUnit := unit
	if len(skipped) > 0 || len(applied) != len(unit.Entries) {
		newUnit.Entries = make([]Entry, len(applied))
		for i, e := range applied {
			newUnit.Entries[i] = cloneEntry(e)
		}
	}
	g.undoStack = append(g.undoStack, newUnit)

	paths := relList(newUnit.Entries)
	msg := fmt.Sprintf("redid %d file(s): %s", len(newUnit.Entries), strings.Join(paths, ", "))
	if len(skipped) > 0 {
		msg = fmt.Sprintf("%s; %s: %s", MsgRedoPartialSkip, msg, strings.Join(skipped, "; "))
	}
	if len(hardFails) > 0 {
		msg = fmt.Sprintf("%s; errors: %s", msg, strings.Join(hardFails, "; "))
	}
	u := newUnit
	return UndoResult{Message: msg, Noop: false, Unit: &u, Changes: changes}
}

// externalRedoMismatch reports whether current disk differs from the state undo
// left (Snapshot.Before / absent), so reapplying After would clobber an external edit.
func externalRedoMismatch(entry Entry) (mismatch bool, reason string) {
	abs := entry.Snapshot.Path
	cur, err := os.ReadFile(abs)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		// Unreadable: treat as external/unknown — soft-skip rather than clobber.
		return true, "unreadable after external change"
	}
	if !entry.Snapshot.Existed {
		// After undo the file should be absent.
		if exists {
			return true, "file reappeared or changed outside undo stack"
		}
		return false, ""
	}
	// After undo the file should equal Snapshot.Before.
	if !exists {
		return true, "file missing after external change"
	}
	if !bytes.Equal(cur, entry.Snapshot.Before) {
		return true, "content changed outside undo stack"
	}
	return false, ""
}

// UndoDepth returns the number of units available to undo.
func (g *Gateway) UndoDepth() int {
	if g == nil {
		return 0
	}
	return len(g.undoStack)
}

// RedoDepth returns the number of units available to redo.
func (g *Gateway) RedoDepth() int {
	if g == nil {
		return 0
	}
	return len(g.redoStack)
}

// UndoStack returns a copy of undo units (oldest first).
func (g *Gateway) UndoStack() []StackUnit {
	if g == nil || len(g.undoStack) == 0 {
		return nil
	}
	out := make([]StackUnit, len(g.undoStack))
	copy(out, g.undoStack)
	return out
}

// UnitByID returns a copy of the undo unit with the given id, or nil.
func (g *Gateway) UnitByID(id string) *StackUnit {
	if g == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	for i := range g.undoStack {
		if g.undoStack[i].ID == id {
			u := g.undoStack[i]
			return &u
		}
	}
	return nil
}

// RedoStack returns a copy of redo units (oldest first).
func (g *Gateway) RedoStack() []StackUnit {
	if g == nil || len(g.redoStack) == 0 {
		return nil
	}
	out := make([]StackUnit, len(g.redoStack))
	copy(out, g.redoStack)
	return out
}

// restoreBefore restores Snapshot.Before (pre-agent dirty), or removes the file
// if it did not exist before the agent write.
func (g *Gateway) restoreBefore(entry Entry) (ChangeEvent, error) {
	abs := entry.Snapshot.Path
	rel := entry.Snapshot.Rel
	if !entry.Snapshot.Existed {
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return ChangeEvent{}, fmt.Errorf("remove %s: %w", rel, err)
		}
		return ChangeEvent{Op: "undo", Path: abs, Rel: rel, Content: nil, Removed: true}, nil
	}
	if err := writeBytes(abs, entry.Snapshot.Before); err != nil {
		return ChangeEvent{}, err
	}
	return ChangeEvent{
		Op:      "undo",
		Path:    abs,
		Rel:     rel,
		Content: append([]byte(nil), entry.Snapshot.Before...),
		Removed: false,
	}, nil
}

// reapplyAfter writes Entry.After to disk (recreates file if needed), or removes
// the file when AfterAbsent (delete mutation) (VAL-UNDO-010).
func (g *Gateway) reapplyAfter(entry Entry) (ChangeEvent, error) {
	abs := entry.Snapshot.Path
	rel := entry.Snapshot.Rel
	if entry.AfterAbsent {
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return ChangeEvent{}, fmt.Errorf("remove %s: %w", rel, err)
		}
		return ChangeEvent{Op: "redo", Path: abs, Rel: rel, Content: nil, Removed: true}, nil
	}
	if err := writeBytes(abs, entry.After); err != nil {
		return ChangeEvent{}, err
	}
	return ChangeEvent{
		Op:      "redo",
		Path:    abs,
		Rel:     rel,
		Content: append([]byte(nil), entry.After...),
		Removed: false,
	}, nil
}

func writeBytes(abs string, content []byte) error {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp := abs + ".yura-ai-tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", abs, err)
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		if err2 := os.WriteFile(abs, content, 0o644); err2 != nil {
			return fmt.Errorf("write %s: %w", abs, err2)
		}
	}
	return nil
}

func cloneEntry(e Entry) Entry {
	return Entry{
		Snapshot: Snapshot{
			Path:    e.Snapshot.Path,
			Rel:     e.Snapshot.Rel,
			Before:  append([]byte(nil), e.Snapshot.Before...),
			Existed: e.Snapshot.Existed,
		},
		After:       append([]byte(nil), e.After...),
		AfterAbsent: e.AfterAbsent,
		Source:      e.Source,
	}
}

func relList(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		r := e.Snapshot.Rel
		if r == "" {
			r = filepath.Base(e.Snapshot.Path)
		}
		out = append(out, r)
	}
	return out
}

func newUnitID() string {
	// Time-based + random-ish bytes without importing crypto/rand noise for tests.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Unix())))
	return hex.EncodeToString(sum[:8])
}

// WorkspaceID returns a stable short id for a workspace root path.
func WorkspaceID(root string) string {
	root = filepath.Clean(root)
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:16])
}

// PersistPath returns the default stack file path under config home:
// {configHome}/workspaces/{workspaceId}/undo/stack.json
func PersistPath(configHome, workspaceRoot string) string {
	id := WorkspaceID(workspaceRoot)
	return filepath.Join(configHome, "workspaces", id, "undo", "stack.json")
}

// persistedStack is the on-disk JSON shape.
type persistedStack struct {
	Version   int         `json:"version"`
	Root      string      `json:"root"`
	Undo      []StackUnit `json:"undo"`
	Redo      []StackUnit `json:"redo"`
	// Entries is a flat copy for compatibility with older readers.
	Entries []Entry `json:"entries,omitempty"`
}

// SaveStack writes undo/redo stacks to path (mode 0600, parent dirs 0700).
// Any open turn is committed first so multi-file batches are persisted.
func (g *Gateway) SaveStack(path string) error {
	if g == nil {
		return fmt.Errorf("writegate: gateway not configured")
	}
	if g.openTurn != nil {
		g.EndTurn()
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("writegate: create undo dir: %w", err)
	}
	doc := persistedStack{
		Version: 1,
		Root:    g.Root,
		Undo:    g.undoStack,
		Redo:    g.redoStack,
		Entries: g.Entries,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("writegate: marshal stack: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writegate: write stack: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		if err2 := os.WriteFile(path, data, 0o600); err2 != nil {
			return fmt.Errorf("writegate: write stack: %w", err2)
		}
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

// LoadStack loads undo/redo stacks from path. Missing file is a no-op (empty stack).
// Paths in entries are re-rooted when the saved root differs but rel is valid.
func (g *Gateway) LoadStack(path string) error {
	if g == nil {
		return fmt.Errorf("writegate: gateway not configured")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("writegate: read stack: %w", err)
	}
	var doc persistedStack
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("writegate: parse stack: %w", err)
	}
	g.undoStack = rehydrateUnits(g.Root, doc.Undo)
	g.redoStack = rehydrateUnits(g.Root, doc.Redo)
	if len(doc.Entries) > 0 {
		g.Entries = rehydrateEntries(g.Root, doc.Entries)
	} else {
		// Rebuild flat Entries from undo stack for legacy callers.
		var flat []Entry
		for _, u := range g.undoStack {
			flat = append(flat, u.Entries...)
		}
		g.Entries = flat
	}
	return nil
}

func rehydrateUnits(root string, units []StackUnit) []StackUnit {
	out := make([]StackUnit, 0, len(units))
	for _, u := range units {
		u.Entries = rehydrateEntries(root, u.Entries)
		out = append(out, u)
	}
	return out
}

func rehydrateEntries(root string, entries []Entry) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		e = cloneEntry(e)
		if e.Snapshot.Rel != "" {
			e.Snapshot.Path = filepath.Join(root, filepath.FromSlash(e.Snapshot.Rel))
		}
		out = append(out, e)
	}
	return out
}

// ContentEqual reports byte-identical content (nil and empty are equal).
func ContentEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return bytes.Equal(a, b)
}
