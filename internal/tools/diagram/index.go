package diagram

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IndexRelPath is the relative path (from workspace root) of the diagram index.
const IndexRelPath = "docs/diagrams/README.md"

// IndexEntry describes one diagram in the index.
type IndexEntry struct {
	Slug   string // file stem, e.g. "auth-flow"
	Path   string // workspace-relative path, e.g. "docs/diagrams/auth-flow.mmd"
	Format Format
	Title  string
}

// IndexUpdater writes/updates docs/diagrams/README.md so saved diagrams are
// discoverable (VAL-DIAG-008). It operates on the workspace root.
type IndexUpdater struct {
	root string
}

// NewIndexUpdater creates an updater for the given workspace root.
func NewIndexUpdater(workspaceRoot string) *IndexUpdater {
	return &IndexUpdater{root: workspaceRoot}
}

// IndexAbsPath returns the absolute path of the diagram index file.
func (u *IndexUpdater) IndexAbsPath() string {
	if u == nil {
		return ""
	}
	return filepath.Join(u.root, filepath.FromSlash(IndexRelPath))
}

// Update writes the index file to include the given entry, creating the file
// when first invoked. It scans docs/diagrams/ for existing diagram sources so
// the index reflects the directory. It is idempotent: running twice with the
// same entry yields the same file.
//
// The index is written using brand-correct heading "Inferenesia — Diagrams".
// Update writes the index file to include the given entry via the provided
// gateway backend (routed through WriteGateway so the diagram index is
// undoable and honors the single-write-path invariant, VAL-UNDO-007).
// The `gw` argument may be nil in tests: it will fall back to a direct disk
// write. Production callers pass the diagram Tools backend.
func (u *IndexUpdater) Update(entry IndexEntry, gw GatewayBackend) error {
	if u == nil {
		return fmt.Errorf("diagram: nil index updater")
	}
	entries := u.scan()
	// Ensure the new entry is present (dedupe by path).
	if entry.Path != "" {
		dup := false
		for _, e := range entries {
			if e.Path == entry.Path {
				dup = true
				break
			}
		}
		if !dup {
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Slug < entries[j].Slug
	})
	return u.write(entries, gw)
}

func (u *IndexUpdater) scan() []IndexEntry {
	dir := filepath.Join(u.root, "docs", "diagrams")
	var out []IndexEntry
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == "README.md" {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == "" {
			return nil
		}
		format := FormatFromExt(ext)
		// Skip rendered artifacts (png/svg/pdf) — they are not sources.
		if ext == ".png" || ext == ".svg" || ext == ".pdf" {
			return nil
		}
		slug := strings.TrimSuffix(name, ext)
		rel, _ := filepath.Rel(u.root, path)
		rel = filepath.ToSlash(rel)
		title := titleFromSlug(slug, format)
		out = append(out, IndexEntry{Slug: slug, Path: rel, Format: format, Title: title})
		return nil
	})
	return out
}

func (u *IndexUpdater) write(entries []IndexEntry, gw GatewayBackend) error {
	var b strings.Builder
	fmt.Fprintln(&b, "# Inferenesia — Diagrams")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Index of saved diagram sources under `docs/diagrams/`. Sources are authored as text (Mermaid first; DBML/PlantUML optional). Rendered PNG/SVG artifacts are git-friendly companions, not the source of truth.")
	fmt.Fprintln(&b)
	if len(entries) == 0 {
		fmt.Fprintln(&b, "_(no diagrams yet)_")
	} else {
		fmt.Fprintln(&b, "| Slug | Format | Title | Source |")
		fmt.Fprintln(&b, "|------|--------|-------|--------|")
		for _, e := range entries {
			fmt.Fprintf(&b, "| %s | %s | %s | [`%s`](./%s) |\n",
				e.Slug, e.Format, e.Title, e.Path, relFromIndex(e.Path))
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Adding diagrams")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Use the `create_diagram` agent tool (saves through WriteGateway, undoable) or add a `.mmd`/`.dbml`/`.puml` file here directly. Each new diagram updates this index in the same logical work unit.")
	fmt.Fprintln(&b)
	if gw == nil {
		return os.WriteFile(u.IndexAbsPath(), []byte(b.String()), 0o644)
	}
	if _, err := gw.MakeDir("docs/diagrams", "diagram"); err != nil {
		return fmt.Errorf("diagram: mkdir index dir via gateway: %w", err)
	}
	if _, err := gw.WriteFileWithSource(IndexRelPath, []byte(b.String()), "diagram"); err != nil {
		return fmt.Errorf("diagram: write index via gateway: %w", err)
	}
	return nil
}

// relFromIndex returns a path relative to docs/diagrams/ for the markdown link.
func relFromIndex(workspaceRel string) string {
	prefix := "docs/diagrams/"
	if strings.HasPrefix(workspaceRel, prefix) {
		return strings.TrimPrefix(workspaceRel, prefix)
	}
	return workspaceRel
}

func titleFromSlug(slug string, format Format) string {
	// Humanize: replace - and _ with spaces, title-case each word.
	human := strings.ReplaceAll(slug, "-", " ")
	human = strings.ReplaceAll(human, "_", " ")
	human = strings.TrimSpace(human)
	if human == "" {
		human = string(format)
	}
	words := strings.Fields(human)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
