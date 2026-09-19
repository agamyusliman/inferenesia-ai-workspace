package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// uniqueDestName returns a non-colliding basename under parentAbs.
// First try the preferred name; if taken, use "name copy.ext", then
// "name copy 2.ext", "name copy 3.ext", … (VAL-IDE-013).
func uniqueDestName(parentAbs, preferred string) (string, error) {
	preferred = sanitizeSegment(preferred)
	if preferred == "" {
		return "", fmt.Errorf("core: empty destination name")
	}
	try := preferred
	for n := 0; ; n++ {
		if n == 1 {
			try = copyStyleName(preferred, 0)
		} else if n > 1 {
			try = copyStyleName(preferred, n)
		}
		candidate := filepath.Join(parentAbs, try)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return try, nil
		} else if err != nil {
			return "", fmt.Errorf("core: unique name stat: %w", err)
		}
		// collision → next
		if n > 10_000 {
			return "", fmt.Errorf("core: could not find unique name for %s", preferred)
		}
	}
}

// copyStyleName builds "base copy.ext" (n==0) or "base copy N.ext" (n>=2).
// preferred already includes extension.
func copyStyleName(preferred string, n int) string {
	ext := filepath.Ext(preferred)
	base := strings.TrimSuffix(preferred, ext)
	if n <= 0 {
		return base + " copy" + ext
	}
	return fmt.Sprintf("%s copy %d%s", base, n, ext)
}

// CopyPath duplicates srcRel into destParentRel (folder or workspace root).
// Routes file content through WriteGateway with SourceUser so undo stays
// consistent for UI clipboard paste (VAL-IDE-013/025). Auto-renames on
// clobber (e.g. "file copy.ext").
func (s *Service) CopyPath(workspaceID, srcRel, destParentRel string) (FSResult, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return FSResult{}, err
	}
	srcAbs, srcClean, err := resolveUnderRoot(root, srcRel)
	if err != nil {
		return FSResult{}, err
	}
	if srcClean == "" {
		return FSResult{}, fmt.Errorf("core: cannot copy workspace root")
	}
	st, err := os.Stat(srcAbs)
	if err != nil {
		return FSResult{}, fmt.Errorf("core: copy source: %w", err)
	}

	destParentRel = strings.TrimSpace(destParentRel)
	destParentRel = strings.TrimPrefix(destParentRel, "./")
	destParentRel = strings.ReplaceAll(destParentRel, "\\", "/")
	if destParentRel == "." {
		destParentRel = ""
	}
	var destParentAbs, destParentClean string
	if destParentRel == "" {
		destParentAbs = root
		destParentClean = ""
	} else {
		destParentAbs, destParentClean, err = resolveUnderRoot(root, destParentRel)
		if err != nil {
			return FSResult{}, err
		}
		pst, err := os.Stat(destParentAbs)
		if err != nil {
			return FSResult{}, fmt.Errorf("core: paste target: %w", err)
		}
		if !pst.IsDir() {
			return FSResult{}, fmt.Errorf("core: paste target is not a folder")
		}
	}

	// Refuse pasting a folder into itself or a descendant.
	if st.IsDir() {
		if destParentClean == srcClean || strings.HasPrefix(destParentClean, srcClean+"/") {
			return FSResult{}, fmt.Errorf("core: cannot paste folder into itself")
		}
	}

	base := filepath.Base(srcAbs)
	newName, err := uniqueDestName(destParentAbs, base)
	if err != nil {
		return FSResult{}, err
	}
	var destRel string
	if destParentClean == "" {
		destRel = newName
	} else {
		destRel = destParentClean + "/" + newName
	}
	destAbs := filepath.Join(root, filepath.FromSlash(destRel))

	g, err := s.openGateway(root)
	if err != nil {
		return FSResult{}, err
	}
	// Close any open agent turn so UI paste is a standalone user unit.
	if g.InTurn() {
		g.EndTurn()
	}
	g.BeginTurn("ui-copy")
	defer g.EndTurn()

	if st.IsDir() {
		if err := copyDirViaGateway(g, srcAbs, destAbs, root); err != nil {
			return FSResult{}, err
		}
	} else {
		data, err := os.ReadFile(srcAbs)
		if err != nil {
			return FSResult{}, fmt.Errorf("core: read source: %w", err)
		}
		if _, err := g.WriteFileWithSource(destRel, data, writegate.SourceUser); err != nil {
			return FSResult{}, err
		}
	}
	if err := s.saveGateway(root, g); err != nil {
		return FSResult{}, err
	}
	s.suppressFile(destAbs)
	s.setStatus(fmt.Sprintf("Copied to %s", destRel))
	return FSResult{
		Path:     destRel,
		Absolute: destAbs,
		IsDir:    st.IsDir(),
		Message:  "copied (user edit)",
	}, nil
}

// MovePath moves srcRel into destParentRel. Creates a duplicate-with-delete
// batch through WriteGateway for files (SourceUser). Directories use rename
// when possible, else copy+delete. Auto-renames on clobber (VAL-IDE-013).
func (s *Service) MovePath(workspaceID, srcRel, destParentRel string) (FSResult, error) {
	root, err := s.rootFor(workspaceID)
	if err != nil {
		return FSResult{}, err
	}
	srcAbs, srcClean, err := resolveUnderRoot(root, srcRel)
	if err != nil {
		return FSResult{}, err
	}
	if srcClean == "" {
		return FSResult{}, fmt.Errorf("core: cannot move workspace root")
	}
	st, err := os.Stat(srcAbs)
	if err != nil {
		return FSResult{}, fmt.Errorf("core: move source: %w", err)
	}

	destParentRel = strings.TrimSpace(destParentRel)
	destParentRel = strings.TrimPrefix(destParentRel, "./")
	destParentRel = strings.ReplaceAll(destParentRel, "\\", "/")
	if destParentRel == "." {
		destParentRel = ""
	}
	var destParentAbs, destParentClean string
	if destParentRel == "" {
		destParentAbs = root
		destParentClean = ""
	} else {
		destParentAbs, destParentClean, err = resolveUnderRoot(root, destParentRel)
		if err != nil {
			return FSResult{}, err
		}
		pst, err := os.Stat(destParentAbs)
		if err != nil {
			return FSResult{}, fmt.Errorf("core: paste target: %w", err)
		}
		if !pst.IsDir() {
			return FSResult{}, fmt.Errorf("core: paste target is not a folder")
		}
	}

	srcParent := filepath.ToSlash(filepath.Dir(srcClean))
	if srcParent == "." {
		srcParent = ""
	}
	// Same-folder cut+paste is a no-op (item already at destination).
	if srcParent == destParentClean {
		return FSResult{
			Path:     srcClean,
			Absolute: srcAbs,
			IsDir:    st.IsDir(),
			Message:  "unchanged",
		}, nil
	}

	if st.IsDir() {
		if destParentClean == srcClean || strings.HasPrefix(destParentClean, srcClean+"/") {
			return FSResult{}, fmt.Errorf("core: cannot move folder into itself")
		}
	}

	base := filepath.Base(srcAbs)
	// Prefer original name; auto-rename only when a different item collides.
	newName, err := uniqueDestName(destParentAbs, base)
	if err != nil {
		return FSResult{}, err
	}
	var destRel string
	if destParentClean == "" {
		destRel = newName
	} else {
		destRel = destParentClean + "/" + newName
	}
	destAbs := filepath.Join(root, filepath.FromSlash(destRel))

	g, err := s.openGateway(root)
	if err != nil {
		return FSResult{}, err
	}
	if g.InTurn() {
		g.EndTurn()
	}
	g.BeginTurn("ui-move")
	defer g.EndTurn()

	if st.IsDir() {
		// Directory move: RenameFile picks os.Rename atomically when possible;
		// fall back to copy+delete through the gateway on cross-device rename.
		if _, err := g.RenameFile(srcClean, destRel, writegate.SourceUser); err != nil {
			if strings.Contains(err.Error(), "invalid cross-device link") || strings.Contains(err.Error(), "EXDEV") {
				if cerr := copyDirViaGateway(g, srcAbs, destAbs, root); cerr != nil {
					return FSResult{}, cerr
				}
				if rerr := os.RemoveAll(srcAbs); rerr != nil {
					return FSResult{}, fmt.Errorf("core: remove source after move: %w", rerr)
				}
			} else {
				return FSResult{}, err
			}
		}
	} else {
		data, err := os.ReadFile(srcAbs)
		if err != nil {
			return FSResult{}, fmt.Errorf("core: read source: %w", err)
		}
		if _, err := g.WriteFileWithSource(destRel, data, writegate.SourceUser); err != nil {
			return FSResult{}, err
		}
		if _, err := g.DeleteFileWithSource(srcClean, writegate.SourceUser); err != nil {
			return FSResult{}, err
		}
	}
	if err := s.saveGateway(root, g); err != nil {
		return FSResult{}, err
	}
	s.suppressFile(srcAbs)
	s.suppressFile(destAbs)
	s.mu.Lock()
	if s.selectedFile == srcClean {
		s.selectedFile = destRel
	} else if strings.HasPrefix(s.selectedFile, srcClean+"/") {
		s.selectedFile = destRel + strings.TrimPrefix(s.selectedFile, srcClean)
	}
	s.statusMsg = fmt.Sprintf("Moved to %s", destRel)
	s.mu.Unlock()
	return FSResult{
		Path:     destRel,
		Absolute: destAbs,
		IsDir:    st.IsDir(),
		Message:  "moved (user edit)",
	}, nil
}

// copyDirViaGateway recursively copies a directory tree, writing each file
// through WriteGateway (SourceUser). parent dirs are created with mkdir.
func copyDirViaGateway(g *writegate.Gateway, srcAbs, destAbs, root string) error {
	destRel, err := filepath.Rel(root, destAbs)
	if err != nil {
		return fmt.Errorf("core: rel dest: %w", err)
	}
	destRel = filepath.ToSlash(destRel)
	if _, err := g.MakeDir(destRel, writegate.SourceUser); err != nil {
		return fmt.Errorf("core: mkdir dest: %w", err)
	}
	return filepath.WalkDir(srcAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(destAbs, rel)
		if d.IsDir() {
			relWS, err := filepath.Rel(root, target)
			if err != nil {
				return err
			}
			if _, err := g.MakeDir(filepath.ToSlash(relWS), writegate.SourceUser); err != nil {
				return err
			}
			return nil
		}
		// Stream small files into WriteGateway.
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return err
		}
		// dest relative to workspace root
		relWS, err := filepath.Rel(root, target)
		if err != nil {
			return err
		}
		relWS = filepath.ToSlash(relWS)
		if _, err := g.WriteFileWithSource(relWS, data, writegate.SourceUser); err != nil {
			return err
		}
		return nil
	})
}
