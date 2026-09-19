package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/agamyusliman/inferenesia-app/internal/workspace"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// FSResult is the outcome of a UI file mutation routed through core.Service.
type FSResult struct {
	// Path is the workspace-relative slash path of the created/renamed/deleted item.
	Path string `json:"path"`
	// Absolute is the absolute filesystem path (for Reveal / copy-path).
	Absolute string `json:"absolute,omitempty"`
	// IsDir is true when the target is a directory.
	IsDir bool `json:"is_dir,omitempty"`
	// Message is a short status line for the UI.
	Message string `json:"message,omitempty"`
}

// PathInfo describes a resolved workspace-relative path.
type PathInfo struct {
	// Relative is slash-normalized workspace-relative path.
	Relative string `json:"relative"`
	// Absolute is the absolute path on disk.
	Absolute string `json:"absolute"`
	// Exists is true when the path exists.
	Exists bool `json:"exists"`
	// IsDir is true when the path is a directory.
	IsDir bool `json:"is_dir"`
}

// SaveFile writes buffer content for a workspace-relative path (desktop editor save).
// Routes through WriteGateway with SourceUser so the mutation is tracked as a user edit
// (not an agent turn), still snapshot/undo-aware and FileChanged-capable (VAL-IDE-017/025).
func (s *Service) SaveFile(workspaceID, rel, content string) (FSResult, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return FSResult{}, err
	}
	abs, cleanRel, err := resolveUnderRoot(root, rel)
	if err != nil {
		return FSResult{}, err
	}
	if cleanRel == "" {
		return FSResult{}, fmt.Errorf("core: cannot overwrite workspace root")
	}
	// Refuse directory targets.
	if st, err := os.Stat(abs); err == nil && st.IsDir() {
		return FSResult{}, fmt.Errorf("core: cannot save directory: %s", cleanRel)
	} else if err != nil && !os.IsNotExist(err) {
		return FSResult{}, fmt.Errorf("core: stat: %w", err)
	}
	g, err := s.openGateway(root)
	if err != nil {
		return FSResult{}, err
	}
	// Manual editor save is a standalone user unit (not an open agent turn batch).
	if g.InTurn() {
		g.EndTurn()
	}
	if _, err := g.WriteFileWithSource(cleanRel, []byte(content), writegate.SourceUser); err != nil {
		return FSResult{}, err
	}
	if err := s.saveGateway(root, g); err != nil {
		return FSResult{}, err
	}
	// Suppress the fsnotify echo of our own WriteGateway write so the explorer
	// does not double-refresh (the chat SSE already emitted FileChanged).
	// CLI writes are external and never suppressed (VAL-CROSS-003).
	s.suppressFile(abs)
	s.mu.Lock()
	s.selectedFile = cleanRel
	s.statusMsg = fmt.Sprintf("Saved %s", cleanRel)
	s.mu.Unlock()
	return FSResult{
		Path:     cleanRel,
		Absolute: abs,
		Message:  "saved (user edit)",
	}, nil
}

// CreateFile creates an empty file under parentRel (or workspace root when parent is empty).
// name is a single path segment. Colliding names are rejected (no silent overwrite).
// Mutation routes through WriteGateway (user UI new file).
func (s *Service) CreateFile(workspaceID, parentRel, name string) (FSResult, error) {
	rel, abs, err := s.childPath(workspaceID, parentRel, name)
	if err != nil {
		return FSResult{}, err
	}
	if st, err := os.Stat(abs); err == nil {
		return FSResult{}, fmt.Errorf("core: already exists: %s", st.Name())
	} else if !os.IsNotExist(err) {
		return FSResult{}, fmt.Errorf("core: stat: %w", err)
	}
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return FSResult{}, err
	}
	g, err := s.openGateway(root)
	if err != nil {
		return FSResult{}, err
	}
	// UI new-file is a user edit (VAL-IDE-025), not an agent turn.
	if g.InTurn() {
		g.EndTurn()
	}
	if _, err := g.WriteFileWithSource(rel, []byte{}, writegate.SourceUser); err != nil {
		return FSResult{}, err
	}
	if err := s.saveGateway(root, g); err != nil {
		return FSResult{}, err
	}
	s.suppressFile(abs)
	s.setStatus(fmt.Sprintf("Created file %s", rel))
	return FSResult{Path: rel, Absolute: abs, Message: "created file (user edit)"}, nil
}

// CreateFolder creates an empty directory under parentRel.
// Colliding names are rejected.
func (s *Service) CreateFolder(workspaceID, parentRel, name string) (FSResult, error) {
	rel, abs, err := s.childPath(workspaceID, parentRel, name)
	if err != nil {
		return FSResult{}, err
	}
	if _, err := os.Stat(abs); err == nil {
		return FSResult{}, fmt.Errorf("core: already exists: %s", name)
	} else if !os.IsNotExist(err) {
		return FSResult{}, fmt.Errorf("core: stat: %w", err)
	}
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return FSResult{}, err
	}
	g, err := s.openGateway(root)
	if err != nil {
		return FSResult{}, err
	}
	if g.InTurn() {
		g.EndTurn()
	}
	if _, err := g.MakeDir(rel, writegate.SourceUser); err != nil {
		return FSResult{}, fmt.Errorf("core: mkdir: %w", err)
	}
	if err := s.saveGateway(root, g); err != nil {
		return FSResult{}, err
	}
	s.suppressFile(abs)
	s.setStatus(fmt.Sprintf("Created folder %s", rel))
	return FSResult{Path: rel, Absolute: abs, IsDir: true, Message: "created folder"}, nil
}

// RenamePath renames a file or folder within the same parent directory.
// newName is a single segment. Colliding names are rejected (no silent overwrite).
func (s *Service) RenamePath(workspaceID, rel, newName string) (FSResult, error) {
	newName = sanitizeSegment(newName)
	if newName == "" {
		return FSResult{}, fmt.Errorf("core: empty name")
	}
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return FSResult{}, err
	}
	oldAbs, oldRel, err := resolveUnderRoot(root, rel)
	if err != nil {
		return FSResult{}, err
	}
	if oldRel == "" {
		return FSResult{}, fmt.Errorf("core: cannot rename workspace root")
	}
	st, err := os.Stat(oldAbs)
	if err != nil {
		return FSResult{}, fmt.Errorf("core: stat: %w", err)
	}
	parentRel := filepath.ToSlash(filepath.Dir(oldRel))
	if parentRel == "." {
		parentRel = ""
	}
	newRel, newAbs, err := s.childPath(workspaceID, parentRel, newName)
	if err != nil {
		return FSResult{}, err
	}
	if newAbs == oldAbs {
		return FSResult{Path: oldRel, Absolute: oldAbs, IsDir: st.IsDir(), Message: "unchanged"}, nil
	}
	if _, err := os.Stat(newAbs); err == nil {
		return FSResult{}, fmt.Errorf("core: already exists: %s", newName)
	} else if !os.IsNotExist(err) {
		return FSResult{}, fmt.Errorf("core: stat: %w", err)
	}
	g, err := s.openGateway(root)
	if err != nil {
		return FSResult{}, err
	}
	if g.InTurn() {
		g.EndTurn()
	}
	if _, err := g.RenameFile(oldRel, newRel, writegate.SourceUser); err != nil {
		return FSResult{}, fmt.Errorf("core: rename: %w", err)
	}
	if err := s.saveGateway(root, g); err != nil {
		return FSResult{}, err
	}
	// Suppress fsnotify echoes for both old (rename/remove) and new (create) paths.
	s.suppressFile(oldAbs)
	s.suppressFile(newAbs)
	s.mu.Lock()
	if s.selectedFile == oldRel {
		s.selectedFile = newRel
	}
	s.statusMsg = fmt.Sprintf("Renamed to %s", newRel)
	s.mu.Unlock()
	return FSResult{Path: newRel, Absolute: newAbs, IsDir: st.IsDir(), Message: "renamed"}, nil
}

// DeletePath removes a file or directory (recursive for directories).
// Files go through WriteGateway; directories use RemoveAll after sandbox check.
func (s *Service) DeletePath(workspaceID, rel string) (FSResult, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return FSResult{}, err
	}
	abs, cleanRel, err := resolveUnderRoot(root, rel)
	if err != nil {
		return FSResult{}, err
	}
	if cleanRel == "" {
		return FSResult{}, fmt.Errorf("core: cannot delete workspace root")
	}
	st, err := os.Stat(abs)
	if err != nil {
		return FSResult{}, fmt.Errorf("core: stat: %w", err)
	}
	if st.IsDir() {
		if err := os.RemoveAll(abs); err != nil {
			return FSResult{}, fmt.Errorf("core: delete folder: %w", err)
		}
	} else {
		g, err := s.openGateway(root)
		if err != nil {
			return FSResult{}, err
		}
		// UI delete is a user edit unit (VAL-IDE-025).
		if g.InTurn() {
			g.EndTurn()
		}
		if _, err := g.DeleteFileWithSource(cleanRel, writegate.SourceUser); err != nil {
			return FSResult{}, err
		}
		if err := s.saveGateway(root, g); err != nil {
			return FSResult{}, err
		}
	}
	s.suppressFile(abs)
	s.mu.Lock()
	if s.selectedFile == cleanRel || strings.HasPrefix(s.selectedFile, cleanRel+"/") {
		s.selectedFile = ""
	}
	s.statusMsg = fmt.Sprintf("Deleted %s", cleanRel)
	s.mu.Unlock()
	return FSResult{Path: cleanRel, Absolute: abs, IsDir: st.IsDir(), Message: "deleted"}, nil
}

// ResolvePath returns absolute + relative path info for copy-path / reveal.
func (s *Service) ResolvePath(workspaceID, rel string) (PathInfo, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return PathInfo{}, err
	}
	// Empty rel → workspace root itself.
	if strings.TrimSpace(rel) == "" || rel == "." {
		st, err := os.Stat(root)
		exists := err == nil
		isDir := exists && st.IsDir()
		return PathInfo{Relative: "", Absolute: root, Exists: exists, IsDir: isDir}, nil
	}
	abs, cleanRel, err := resolveUnderRoot(root, rel)
	if err != nil {
		return PathInfo{}, err
	}
	st, err := os.Stat(abs)
	info := PathInfo{Relative: cleanRel, Absolute: abs, Exists: err == nil}
	if err == nil {
		info.IsDir = st.IsDir()
	}
	return info, nil
}

// RevealInOS opens the OS file manager focused on the item (macOS: Finder).
// Does not navigate the app away from the desktop shell.
func (s *Service) RevealInOS(workspaceID, rel string) (FSResult, error) {
	info, err := s.ResolvePath(workspaceID, rel)
	if err != nil {
		return FSResult{}, err
	}
	if !info.Exists {
		return FSResult{}, fmt.Errorf("core: path does not exist: %s", info.Absolute)
	}
	if err := revealInOS(info.Absolute); err != nil {
		return FSResult{}, err
	}
	s.setStatus(fmt.Sprintf("Revealed %s", info.Absolute))
	return FSResult{
		Path:     info.Relative,
		Absolute: info.Absolute,
		IsDir:    info.IsDir,
		Message:  "revealed in file manager",
	}, nil
}

// RemoveWorkspace removes a workspace from the registry.
// Folder workspaces: registry only (project files on disk are never deleted).
// Session workspaces: also purge chat history, tasks, and the session scratch
// directory under <configHome>/sessions/<id>.
func (s *Service) RemoveWorkspace(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("core: empty workspace id")
	}
	if s == nil || s.registry == nil {
		return fmt.Errorf("core: service not ready")
	}

	var (
		wasSession bool
		rootPath   string
	)
	if w, err := s.registry.Get(id); err == nil && w != nil {
		wasSession = workspace.IsSession(*w)
		rootPath = w.RootPath
	}

	if err := s.registry.Remove(id); err != nil {
		return err
	}

	if wasSession {
		s.purgeSessionArtifacts(id, rootPath)
		s.mu.Lock()
		clearedChat := s.chatWorkspaceID == id
		if clearedChat {
			s.chatWorkspaceID = ""
			s.chat = ChatSession{}
		}
		s.statusMsg = "Session deleted"
		s.mu.Unlock()
		if clearedChat {
			s.BindTodosToWorkspace(s.registry.ActiveID())
		}
	} else {
		s.mu.Lock()
		s.statusMsg = "Removed folder from workspace"
		s.mu.Unlock()
	}

	s.mu.Lock()
	if s.registry.ActiveID() == "" {
		s.selectedFile = ""
	}
	s.mu.Unlock()
	return nil
}

// purgeSessionArtifacts best-effort deletes durable chat, tasks, and session
// scratch files. Safe to call only for session-kind workspaces.
func (s *Service) purgeSessionArtifacts(id, rootPath string) {
	if st, err := s.ensureStore(); err == nil && st != nil {
		_ = st.DeleteWorkspaceThread(id)
	}
	if rootPath == "" {
		return
	}
	slash := filepath.ToSlash(rootPath)
	if !strings.Contains(slash, "/sessions/") && !strings.Contains(slash, "/session-scratch") {
		return
	}
	_ = os.RemoveAll(rootPath)
}

// AddFolderToWorkspace registers an absolute folder path (or workspace-relative
// subfolder of the active workspace) as an additional workspace root without
// removing existing registry entries.
func (s *Service) AddFolderToWorkspace(workspaceID, relOrAbs string) (workspace.Workspace, error) {
	path := strings.TrimSpace(relOrAbs)
	if path == "" {
		return workspace.Workspace{}, fmt.Errorf("core: empty path")
	}
	// Absolute → open directly; relative → join under active/specified workspace root.
	if !filepath.IsAbs(path) {
		root, err := s.rootFor(workspaceID)
		if err != nil {
			return workspace.Workspace{}, err
		}
		abs, _, err := resolveUnderRoot(root, path)
		if err != nil {
			return workspace.Workspace{}, err
		}
		path = abs
	}
	st, err := os.Stat(path)
	if err != nil {
		return workspace.Workspace{}, fmt.Errorf("core: stat: %w", err)
	}
	if !st.IsDir() {
		return workspace.Workspace{}, fmt.Errorf("core: not a directory: %s", path)
	}
	return s.OpenWorkspace(path)
}

func (s *Service) setStatus(msg string) {
	s.mu.Lock()
	s.statusMsg = msg
	s.mu.Unlock()
}

func (s *Service) childPath(workspaceID, parentRel, name string) (rel, abs string, err error) {
	name = sanitizeSegment(name)
	if name == "" {
		return "", "", fmt.Errorf("core: empty name")
	}
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return "", "", err
	}
	parentRel = strings.TrimSpace(parentRel)
	parentRel = strings.TrimPrefix(parentRel, "./")
	parentRel = strings.ReplaceAll(parentRel, "\\", "/")
	if parentRel == "." {
		parentRel = ""
	}
	if parentRel != "" {
		// Ensure parent exists and is a dir under root.
		parentAbs, cleanParent, err := resolveUnderRoot(root, parentRel)
		if err != nil {
			return "", "", err
		}
		st, err := os.Stat(parentAbs)
		if err != nil {
			return "", "", fmt.Errorf("core: parent: %w", err)
		}
		if !st.IsDir() {
			return "", "", fmt.Errorf("core: parent is not a directory")
		}
		rel = cleanParent + "/" + name
	} else {
		rel = name
	}
	abs = filepath.Join(root, filepath.FromSlash(rel))
	absClean := filepath.Clean(abs)
	rootClean := filepath.Clean(root)
	if absClean != rootClean && !strings.HasPrefix(absClean, rootClean+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("core: path escapes root")
	}
	return rel, absClean, nil
}

func resolveUnderRoot(root, rel string) (abs, cleanRel string, err error) {
	root = filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.ReplaceAll(rel, "\\", "/")
	if rel == "." {
		rel = ""
	}
	if rel == "" {
		return root, "", nil
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", "", fmt.Errorf("core: absolute path denied")
	}
	if strings.Contains(rel, "..") {
		clean := filepath.Clean(rel)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return "", "", fmt.Errorf("core: path escapes root")
		}
		rel = filepath.ToSlash(clean)
		if rel == "." {
			rel = ""
		}
	}
	abs = filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("core: path escapes root")
	}
	return abs, rel, nil
}

func sanitizeSegment(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "/\\")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if strings.ContainsAny(name, `/\`) {
		return ""
	}
	// Reject control characters.
	for _, r := range name {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return name
}

func revealInOS(abs string) error {
	abs = filepath.Clean(abs)
	switch runtime.GOOS {
	case "darwin":
		// open -R reveals the item selected in Finder.
		cmd := exec.Command("open", "-R", abs)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("core: reveal in Finder: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "windows":
		cmd := exec.Command("explorer", "/select,", abs)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("core: reveal in Explorer: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		// Linux: open parent directory in the default file manager.
		dir := abs
		if st, err := os.Stat(abs); err == nil && !st.IsDir() {
			dir = filepath.Dir(abs)
		}
		cmd := exec.Command("xdg-open", dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("core: reveal in file manager: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}

