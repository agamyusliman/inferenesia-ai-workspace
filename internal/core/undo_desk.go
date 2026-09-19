package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// UndoStackState is the desktop undo toolbar snapshot (pre-agent dirty, not HEAD).
type UndoStackState struct {
	// CanUndo is true when Revert agent / Undo can restore pre-agent dirty content.
	CanUndo bool `json:"can_undo"`
	// CanRedo is true when redo can re-apply the last undone agent mutation.
	CanRedo bool `json:"can_redo"`
	// UndoDepth is the number of agent turn units available to undo.
	UndoDepth int `json:"undo_depth"`
	// RedoDepth is the number of units available to redo.
	RedoDepth int `json:"redo_depth"`
	// Labels distinguish UI actions (VAL-DESK-010).
	// RevertAgentLabel is the primary undo of agent writes (pre-agent dirty).
	RevertAgentLabel string `json:"revert_agent_label"`
	// RestoreToHeadLabel is the separate git restore action (never default undo).
	RestoreToHeadLabel string `json:"restore_to_head_label"`
	// UndoLabel is the toolbar Undo button (same stack as Revert agent for last unit).
	UndoLabel string `json:"undo_label"`
	// RedoLabel is the toolbar Redo button.
	RedoLabel string `json:"redo_label"`
	// Message is a short status for the toolbar strip.
	Message string `json:"message,omitempty"`
}

// Default desktop labels (VAL-DESK-010): never conflate agent undo with git HEAD.
const (
	LabelRevertAgent   = "Revert agent"
	LabelRestoreToHead = "Restore to HEAD"
	LabelUndo          = "Undo"
	LabelRedo          = "Redo"
)

// GetUndoStack returns toolbar state for the active workspace WriteGateway stack.
func (s *Service) GetUndoStack() (UndoStackState, error) {
	st := baseUndoLabels()
	root, err := s.activeRoot()
	if err != nil {
		st.Message = err.Error()
		return st, nil // empty stack, no hard error for toolbar probe
	}
	g, err := s.openGateway(root)
	if err != nil {
		return st, err
	}
	st.UndoDepth = g.UndoDepth()
	st.RedoDepth = g.RedoDepth()
	st.CanUndo = st.UndoDepth > 0
	st.CanRedo = st.RedoDepth > 0
	if st.CanUndo {
		st.Message = fmt.Sprintf("%d agent change(s) to Revert agent", st.UndoDepth)
	} else {
		st.Message = "nothing to undo (pre-agent dirty stack empty)"
	}
	return st, nil
}

func baseUndoLabels() UndoStackState {
	return UndoStackState{
		RevertAgentLabel:   LabelRevertAgent,
		RestoreToHeadLabel: LabelRestoreToHead,
		UndoLabel:          LabelUndo,
		RedoLabel:          LabelRedo,
	}
}

// UndoActionResult is the outcome of Undo / Redo / Revert agent for the desktop.
type UndoActionResult struct {
	Message  string          `json:"message"`
	Noop     bool            `json:"noop"`
	Paths    []string        `json:"paths,omitempty"`
	Stack    UndoStackState  `json:"stack"`
	// ContentByPath is optional sample of restored content (for tests; capped).
	ContentByPath map[string]string `json:"content_by_path,omitempty"`
}

// UndoLast restores the last agent WriteGateway unit to pre-agent dirty content
// (byte-identical), NOT git HEAD (VAL-DESK-005 / VAL-UNDO-001).
func (s *Service) UndoLast() (UndoActionResult, error) {
	return s.runStackOp(func(g *writegate.Gateway) writegate.UndoResult {
		return g.Undo()
	})
}

// RedoLast re-applies the last undone agent mutation (VAL-DESK-005).
func (s *Service) RedoLast() (UndoActionResult, error) {
	return s.runStackOp(func(g *writegate.Gateway) writegate.UndoResult {
		return g.Redo()
	})
}

// RevertAgent is the explicit "Revert agent" action: same as UndoLast of the
// last agent turn (pre-agent dirty). Named distinctly from RestoreToHEAD.
func (s *Service) RevertAgent() (UndoActionResult, error) {
	return s.runStackOp(func(g *writegate.Gateway) writegate.UndoResult {
		return g.Revert()
	})
}

func (s *Service) runStackOp(op func(*writegate.Gateway) writegate.UndoResult) (UndoActionResult, error) {
	root, err := s.activeRoot()
	if err != nil {
		return UndoActionResult{Message: err.Error(), Noop: true, Stack: baseUndoLabels()}, err
	}
	g, err := s.openGateway(root)
	if err != nil {
		return UndoActionResult{Message: err.Error(), Noop: true, Stack: baseUndoLabels()}, err
	}
	res := op(g)
	if err := s.saveGateway(root, g); err != nil {
		return UndoActionResult{Message: err.Error(), Stack: baseUndoLabels()}, err
	}
	stack, _ := s.GetUndoStack()
	out := UndoActionResult{
		Message: res.Message,
		Noop:    res.Noop,
		Stack:   stack,
	}
	for _, ch := range res.Changes {
		label := ch.Rel
		if label == "" {
			label = ch.Path
		}
		out.Paths = append(out.Paths, label)
	}
	s.mu.Lock()
	s.statusMsg = res.Message
	s.mu.Unlock()
	return out, nil
}

// RestoreToHEADResult is the outcome of an explicit git restore (not agent undo).
type RestoreToHEADResult struct {
	// Message explains what happened (or why it failed).
	Message string `json:"message"`
	// Label is always LabelRestoreToHead for UI evidence.
	Label string `json:"label"`
	// Path is the restored workspace-relative path.
	Path string `json:"path,omitempty"`
	// Warning clarifies this is NOT Revert agent / pre-agent dirty undo.
	Warning string `json:"warning"`
}

// RestoreToHEAD explicitly restores a path via `git restore` / `git checkout --`
// to the index/HEAD. This is intentionally separate from Undo/Revert agent
// (VAL-DESK-010). path is workspace-relative.
func (s *Service) RestoreToHEAD(rel string) (RestoreToHEADResult, error) {
	out := RestoreToHEADResult{
		Label:   LabelRestoreToHead,
		Warning: "Restore to HEAD uses git and is NOT the same as Revert agent (pre-agent dirty).",
	}
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" || strings.Contains(rel, "..") || filepath.IsAbs(rel) {
		out.Message = "invalid path for Restore to HEAD"
		return out, fmt.Errorf("core: invalid path for Restore to HEAD")
	}
	root, err := s.activeRoot()
	if err != nil {
		out.Message = err.Error()
		return out, err
	}
	// Ensure path is under workspace.
	abs := filepath.Join(root, filepath.FromSlash(rel))
	absClean := filepath.Clean(abs)
	if absClean != root && !strings.HasPrefix(absClean, root+string(os.PathSeparator)) {
		out.Message = "path escapes workspace"
		return out, fmt.Errorf("core: path escapes workspace")
	}
	// Prefer git restore (modern); fall back to checkout --.
	cmd := exec.Command("git", "-C", root, "restore", "--", rel)
	if b, err := cmd.CombinedOutput(); err != nil {
		cmd2 := exec.Command("git", "-C", root, "checkout", "HEAD", "--", rel)
		if b2, err2 := cmd2.CombinedOutput(); err2 != nil {
			msg := strings.TrimSpace(string(b) + " " + string(b2))
			out.Message = fmt.Sprintf("%s failed: %s", LabelRestoreToHead, msg)
			return out, fmt.Errorf("core: restore to HEAD: %w", err2)
		}
	}
	out.Path = rel
	out.Message = fmt.Sprintf("%s: restored %s from git index/HEAD (not pre-agent dirty)", LabelRestoreToHead, rel)
	s.mu.Lock()
	s.statusMsg = out.Message
	s.mu.Unlock()
	return out, nil
}
