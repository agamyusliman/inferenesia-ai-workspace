package writegate

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Human-readable messages for revert / history (VAL-MEM-007/008/012).
const (
	MsgNothingToRevert     = "nothing to revert"
	MsgFileNotChanged      = "file was not changed by the agent (nothing to revert)"
	MsgEmptyHistory        = "no agent changes in history (fresh workspace)"
	MsgRevertedLastTurn    = "reverted last agent turn to pre-agent state"
	MsgRevertedFile        = "reverted file to pre-agent state"
)

// HistoryEntry is one timeline row for agent changes available for undo/revert
// (VAL-MEM-012). Newest turn is first when produced by History().
type HistoryEntry struct {
	// Index is 1-based from the newest undo unit (1 = last turn / top of undo stack).
	Index int
	// UnitID is the stack unit id.
	UnitID string
	// TurnID is the agent turn id when set.
	TurnID string
	// CreatedAt is when the unit was pushed.
	CreatedAt time.Time
	// Paths are relative workspace paths changed in this unit.
	Paths []string
	// Sources are distinct mutation sources in the unit (fs, multibrain, diagram).
	Sources []string
	// FileCount is len(Paths).
	FileCount int
}

// Revert is an alias for Undo of the last turn unit: restores all files in the
// top StackUnit to pre-agent dirty snapshots (including multibrain writes).
// Memory roll-back is automatic when multibrain files were written in that turn
// through WriteGateway (VAL-MEM-007, VAL-CROSS-013).
//
// Empty stack → safe no-op with MsgNothingToRevert.
func (g *Gateway) Revert() UndoResult {
	if g == nil {
		return UndoResult{Message: MsgNothingToRevert, Noop: true}
	}
	if g.openTurn != nil {
		g.EndTurn()
	}
	if len(g.undoStack) == 0 {
		return UndoResult{Message: MsgNothingToRevert, Noop: true}
	}
	res := g.Undo()
	if res.Noop {
		// Preserve empty-stack wording as "nothing to revert" when applicable.
		if res.Message == MsgNothingToUndo {
			res.Message = MsgNothingToRevert
		}
		return res
	}
	// Prefer turn/revert wording over generic undo for user-facing CLI.
	paths := relList(res.Unit.Entries)
	res.Message = fmt.Sprintf("%s (%d file(s)): %s",
		MsgRevertedLastTurn, len(res.Unit.Entries), strings.Join(paths, ", "))
	return res
}

// RevertFile restores a single path to its pre-agent dirty content from the
// most recent undo unit that touched that path (LIFO). Other files in the same
// turn and other turns are left unchanged. Multibrain memory for those other
// paths is not rolled back (VAL-MEM-008).
//
// If the file was never changed by the agent (no matching stack entry), returns
// a safe no-op with MsgFileNotChanged.
//
// Within a multi-file turn, only the matching path is restored; the unit is
// rewritten (remaining entries stay on the undo stack) so a subsequent full
// Revert/Undo still restores the sibling files.
func (g *Gateway) RevertFile(path string) UndoResult {
	if g == nil {
		return UndoResult{Message: MsgFileNotChanged, Noop: true}
	}
	if g.openTurn != nil {
		g.EndTurn()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return UndoResult{Message: MsgFileNotChanged, Noop: true}
	}

	// Resolve to slash-relative path for comparison when possible.
	var wantRel string
	var wantAbs string
	if abs, rel, err := g.ResolveInWorkspace(path); err == nil {
		wantAbs = abs
		wantRel = rel
	} else {
		// Fall back to path string matching on Rel / Base.
		wantRel = filepath.ToSlash(path)
		wantAbs = path
	}

	// Search undo stack from newest to oldest for the first unit containing path.
	unitIdx := -1
	entryIdx := -1
	for ui := len(g.undoStack) - 1; ui >= 0; ui-- {
		u := g.undoStack[ui]
		for ei, e := range u.Entries {
			if pathMatchesEntry(e, wantRel, wantAbs) {
				unitIdx = ui
				entryIdx = ei
				break
			}
		}
		if unitIdx >= 0 {
			break
		}
	}
	if unitIdx < 0 || entryIdx < 0 {
		return UndoResult{Message: MsgFileNotChanged, Noop: true}
	}

	unit := g.undoStack[unitIdx]
	entry := unit.Entries[entryIdx]

	ev, err := g.restoreBefore(entry)
	if err != nil {
		return UndoResult{
			Message: fmt.Sprintf("revert file failed: %v", err),
			Noop:    false,
		}
	}
	if g.onChange != nil {
		g.onChange(ev)
	}

	// Remove the reverted entry from the unit; drop the unit if empty.
	remaining := make([]Entry, 0, len(unit.Entries)-1)
	for i, e := range unit.Entries {
		if i == entryIdx {
			continue
		}
		remaining = append(remaining, cloneEntry(e))
	}
	if len(remaining) == 0 {
		// Drop the unit entirely.
		g.undoStack = append(g.undoStack[:unitIdx], g.undoStack[unitIdx+1:]...)
	} else {
		unit.Entries = remaining
		g.undoStack[unitIdx] = unit
	}

	// Record a single-file unit on the redo stack so redo can reapply only this path.
	redoUnit := StackUnit{
		ID:        newUnitID(),
		TurnID:    unit.TurnID,
		Entries:   []Entry{cloneEntry(entry)},
		CreatedAt: time.Now().UTC(),
	}
	g.redoStack = append(g.redoStack, redoUnit)

	rel := entry.Snapshot.Rel
	if rel == "" {
		rel = filepath.Base(entry.Snapshot.Path)
	}
	msg := fmt.Sprintf("%s: %s", MsgRevertedFile, rel)
	u := redoUnit
	return UndoResult{
		Message: msg,
		Noop:    false,
		Unit:    &u,
		Changes: []ChangeEvent{ev},
	}
}

// History returns agent-change timeline entries, newest first (VAL-MEM-012).
// Includes any open turn committed first so in-progress batches appear.
func (g *Gateway) History() []HistoryEntry {
	if g == nil {
		return nil
	}
	if g.openTurn != nil {
		g.EndTurn()
	}
	n := len(g.undoStack)
	if n == 0 {
		return nil
	}
	out := make([]HistoryEntry, 0, n)
	// Newest first: reverse walk.
	for i := n - 1; i >= 0; i-- {
		u := g.undoStack[i]
		paths := relList(u.Entries)
		srcs := uniqueSources(u.Entries)
		out = append(out, HistoryEntry{
			Index:     n - i, // 1 = newest
			UnitID:    u.ID,
			TurnID:    u.TurnID,
			CreatedAt: u.CreatedAt,
			Paths:     paths,
			Sources:   srcs,
			FileCount: len(paths),
		})
	}
	return out
}

// FormatHistory renders a human-readable timeline for CLI (VAL-MEM-012).
// Empty history returns MsgEmptyHistory plus a trailing newline.
func FormatHistory(entries []HistoryEntry) string {
	if len(entries) == 0 {
		return MsgEmptyHistory + "\n"
	}
	var b strings.Builder
	b.WriteString("Agent change timeline (newest first; use revert / undo):\n")
	for _, e := range entries {
		turn := e.TurnID
		if turn == "" {
			turn = e.UnitID
			if len(turn) > 8 {
				turn = turn[:8]
			}
		}
		ts := e.CreatedAt.Local().Format("2006-01-02 15:04:05")
		if e.CreatedAt.IsZero() {
			ts = "(unknown time)"
		}
		src := strings.Join(e.Sources, ",")
		if src == "" {
			src = SourceFS
		}
		b.WriteString(fmt.Sprintf("  #%d  %s  turn=%s  files=%d  sources=%s\n",
			e.Index, ts, turn, e.FileCount, src))
		for _, p := range e.Paths {
			b.WriteString(fmt.Sprintf("       - %s\n", p))
		}
	}
	return b.String()
}

func pathMatchesEntry(e Entry, wantRel, wantAbs string) bool {
	rel := filepath.ToSlash(e.Snapshot.Rel)
	abs := e.Snapshot.Path
	wantRel = filepath.ToSlash(strings.TrimPrefix(wantRel, "./"))
	// Exact rel match.
	if wantRel != "" && rel != "" && rel == wantRel {
		return true
	}
	// Exact abs match.
	if wantAbs != "" && abs != "" && filepath.Clean(abs) == filepath.Clean(wantAbs) {
		return true
	}
	// Basename-only match when user passes "f.txt".
	base := filepath.Base(wantRel)
	if base != "" && base != "." && base != string(filepath.Separator) {
		if filepath.Base(rel) == base || filepath.Base(abs) == base {
			// Prefer unique match; for simplicity accept basename match.
			return true
		}
	}
	// Also try wantAbs as a relative-looking string.
	if wantAbs != "" && rel != "" && filepath.ToSlash(wantAbs) == rel {
		return true
	}
	return false
}

func uniqueSources(entries []Entry) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, e := range entries {
		s := normalizeSource(e.Source)
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
