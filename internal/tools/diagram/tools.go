package diagram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// Tool names exposed to the model (OpenAI-compatible function names).
const (
	NameCreateDiagram = "create_diagram"
	NameUpdateDiagram = "update_diagram"
	NameExportDiagram = "export_diagram"
	// NameConvertDiagram is the Mermaid <-> Excalidraw bridge (VAL-DIAG-007).
	// It converts a Mermaid flowchart to an Excalidraw document preserving
	// node/edge count, and saves the result under docs/diagrams/ via
	// WriteGateway (undoable). The visual canvas (D5) is a separate surface;
	// this tool produces the `.excalidraw` JSON source the editor round-trips.
	NameConvertDiagram = "convert_diagram"
)

// DiagramsRelDir is the workspace-relative directory for saved diagrams.
const DiagramsRelDir = "docs/diagrams"

// GatewayBackend is the subset of *writegate.Gateway the diagram tools need.
// All file mutations route through WriteGateway (VAL-UNDO-007 / VAL-DIAG-002/003).
type GatewayBackend interface {
	RootPath() string
	WriteDiagram(path string, content []byte) (writegate.Entry, error)
	WriteFileWithSource(path string, content []byte, source string) (writegate.Entry, error)
	MakeDir(path string, source string) (writegate.Entry, error)
	ResolveInWorkspace(path string) (abs, rel string, err error)
	BeginTurn(turnID string)
	EndTurn()
	InTurn() bool
}

// Tools is the diagram tool backend wired into tools.Registry.
// It owns the WriteGateway, index updater, and exporter.
type Tools struct {
	gw     GatewayBackend
	index  *IndexUpdater
	export *Exporter
}

// NewTools builds diagram tools bound to a WriteGateway.
func NewTools(gw *writegate.Gateway) *Tools {
	var backend GatewayBackend
	root := ""
	if gw != nil {
		backend = gw
		root = gw.RootPath()
	}
	return &Tools{
		gw:     backend,
		index:  NewIndexUpdater(root),
		export: NewExporter(),
	}
}

// ToolDef is one OpenAI-compatible tool schema entry (mirrors tools.Definition
// without import cycle).
type ToolDef struct {
	Type        string          `json:"type"` // always "function"
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Definitions returns the OpenAI-compatible tool schemas for diagram tools.
func (t *Tools) Definitions() []ToolDef {
	if t == nil || t.gw == nil {
		return nil
	}
	return []ToolDef{
		{
			Type:        "function",
			Name:        NameCreateDiagram,
			Description: "Create a new diagram source file under docs/diagrams/ via WriteGateway (undoable). Validates Mermaid (incl. erDiagram), DBML, or PlantUML syntax. Saved diagrams are linked from docs/diagrams/README.md in the same work unit. Returns the saved file path.",
			Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": { "type": "string", "description": "Diagram slug used as filename stem, e.g. 'auth-flow' (lowercase, kebab-case)" },
    "format": { "type": "string", "enum": ["mermaid","dbml","plantuml"], "description": "Diagram source format (default mermaid)" },
    "source": { "type": "string", "description": "Full diagram source text" },
    "title": { "type": "string", "description": "Optional human-readable title for the index entry" }
  },
  "required": ["slug", "source"]
}`),
		},
		{
			Type:        "function",
			Name:        NameUpdateDiagram,
			Description: "Update an existing diagram source file under docs/diagrams/ via WriteGateway (undoable). Undo/redo traverse diagram edits like any file mutation. Accepts slug or path.",
			Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": { "type": "string", "description": "Diagram slug (filename stem) of the existing diagram to update" },
    "path": { "type": "string", "description": "Alternatively, an explicit path under docs/diagrams/" },
    "source": { "type": "string", "description": "Full new diagram source text" },
    "format": { "type": "string", "enum": ["mermaid","dbml","plantuml"], "description": "Source format (inferred from path when omitted)" }
  },
  "required": ["source"]
}`),
		},
		{
			Type:        "function",
			Name:        NameExportDiagram,
			Description: "Export (render) a saved diagram to PNG/SVG (optionally PDF). Uses mmdc for Mermaid and plantuml for PlantUML when available; SVG placeholder for Mermaid when mmdc absent. Export path is workspace-sandboxed under docs/diagrams/.",
			Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": { "type": "string", "description": "Diagram slug (filename stem) of the saved diagram to export" },
    "path": { "type": "string", "description": "Alternatively, an explicit source path under docs/diagrams/" },
    "format": { "type": "string", "enum": ["svg","png","pdf"], "description": "Export target format (default svg)" }
  }
}`),
		},
		{
			Type:        "function",
			Name:        NameConvertDiagram,
			Description: "Convert a Mermaid flowchart to an Excalidraw document (.excalidraw JSON) preserving node/edge count (VAL-DIAG-007 bridge). The result is saved under docs/diagrams/<slug>.excalidraw via WriteGateway (undoable). Supports flowchart/graph with TD/LR/RL/BT direction and the common node/edge syntaxes. The visual Excalidraw canvas (D5) is a separate surface; this tool produces the source the text editor round-trips.",
			Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": { "type": "string", "description": "Slug for the output .excalidraw file (lowercase kebab-case)" },
    "source": { "type": "string", "description": "Mermaid flowchart source to convert (e.g. 'flowchart TD\\n  A --> B')" },
    "title": { "type": "string", "description": "Optional human-readable title for the index entry" }
  },
  "required": ["slug", "source"]
}`),
		},
	}
}

// Call is one model-requested tool invocation (mirrors tools.Call).
type Call struct {
	ID        string
	Name      string
	Arguments string
}

// Result is returned to the model (mirrors tools.Result).
type Result struct {
	ID      string
	Name    string
	Content string
	IsError bool
}

// Execute runs one diagram tool call.
func (t *Tools) Execute(ctx context.Context, call Call) Result {
	if t == nil || t.gw == nil {
		return Result{ID: call.ID, Name: call.Name, Content: "diagram: WriteGateway not configured", IsError: true}
	}
	switch call.Name {
	case NameCreateDiagram:
		return t.execCreate(call)
	case NameUpdateDiagram:
		return t.execUpdate(call)
	case NameExportDiagram:
		return t.execExport(call)
	case NameConvertDiagram:
		return t.execConvert(call)
	default:
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("unknown diagram tool %q", call.Name), IsError: true}
	}
}

type createArgs struct {
	Slug   string `json:"slug"`
	Format string `json:"format"`
	Source string `json:"source"`
	Title  string `json:"title"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (t *Tools) execCreate(call Call) Result {
	t.gw.BeginTurn("create_diagram")
	defer t.gw.EndTurn()
	var args createArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("create_diagram: invalid arguments JSON: %v", err), IsError: true}
	}
	slug := strings.TrimSpace(args.Slug)
	if slug == "" {
		return Result{ID: call.ID, Name: call.Name, Content: "create_diagram: slug is required", IsError: true}
	}
	if !slugRe.MatchString(slug) {
		return Result{ID: call.ID, Name: call.Name, Content: "create_diagram: slug must be lowercase kebab-case (a-z0-9 and -)", IsError: true}
	}
	format := normalizeFormat(args.Format)
	// Parse before writing (VAL-DIAG-005: unknown syntax returns a parse error).
	res, err := Parse(format, args.Source)
	if err != nil {
		return Result{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
	}
	rel := diagramRelPath(slug, format)
	// Write through WriteGateway (VAL-DIAG-002 / VAL-UNDO-007).
	entry, wErr := t.gw.WriteDiagram(rel, []byte(args.Source))
	if wErr != nil {
		return sandboxOrError(call, "create_diagram", wErr)
	}
	// Update index in the same logical work unit (VAL-DIAG-008).
	title := strings.TrimSpace(args.Title)
	if title == "" {
		title = titleFromSlug(slug, format)
	}
	idxEntry := IndexEntry{Slug: slug, Path: rel, Format: format, Title: title}
	if uErr := t.index.Update(idxEntry, t.gw); uErr != nil {
		// Index failure is non-fatal: the diagram was saved through WriteGateway.
		return Result{
			ID:   call.ID,
			Name: call.Name,
			Content: fmt.Sprintf(
				"created diagram %s (%s, type=%s) at %s via WriteGateway (warning: index update failed: %v)",
				slug, format, res.DiagramType, entry.Snapshot.Rel, uErr,
			),
		}
	}
	return Result{
		ID:   call.ID,
		Name: call.Name,
		Content: fmt.Sprintf(
			"created diagram %s (%s, type=%s) at %s via WriteGateway (undoable); index updated",
			slug, format, res.DiagramType, entry.Snapshot.Rel,
		),
	}
}

type updateArgs struct {
	Slug   string `json:"slug"`
	Path   string `json:"path"`
	Source string `json:"source"`
	Format string `json:"format"`
}

func (t *Tools) execUpdate(call Call) Result {
	t.gw.BeginTurn("update_diagram")
	defer t.gw.EndTurn()
	var args updateArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("update_diagram: invalid arguments JSON: %v", err), IsError: true}
	}
	rel, err := t.resolveExistingDiagramPath(args.Slug, args.Path, args.Format)
	if err != nil {
		return Result{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
	}
	// Format precedence: explicit arg > path extension > mermaid default.
	// normalizeFormat("") returns FormatMermaid, so we must check the raw arg
	// string to know whether the caller explicitly chose a format.
	format := FormatMermaid
	if strings.TrimSpace(args.Format) != "" {
		format = normalizeFormat(args.Format)
	} else if ext := filepath.Ext(rel); ext != "" {
		format = FormatFromExt(ext)
	}
	// Parse before writing (VAL-DIAG-005).
	if _, pErr := Parse(format, args.Source); pErr != nil {
		return Result{ID: call.ID, Name: call.Name, Content: pErr.Error(), IsError: true}
	}
	entry, wErr := t.gw.WriteDiagram(rel, []byte(args.Source))
	if wErr != nil {
		return sandboxOrError(call, "update_diagram", wErr)
	}
	// Keep index in sync (entry may already exist; idempotent).
	slug := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	_ = t.index.Update(IndexEntry{Slug: slug, Path: rel, Format: format, Title: titleFromSlug(slug, format)}, t.gw)
	return Result{
		ID:   call.ID,
		Name: call.Name,
		Content: fmt.Sprintf(
			"updated diagram at %s via WriteGateway (undoable; undo/redo traverses edits)",
			entry.Snapshot.Rel,
		),
	}
}

type exportArgs struct {
	Slug   string `json:"slug"`
	Path   string `json:"path"`
	Format string `json:"format"`
}

func (t *Tools) execExport(call Call) Result {
	t.gw.BeginTurn("export_diagram")
	defer t.gw.EndTurn()
	var args exportArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("export_diagram: invalid arguments JSON: %v", err), IsError: true}
	}
	rel, err := t.resolveExistingDiagramPath(args.Slug, args.Path, "")
	if err != nil {
		return Result{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
	}
	format := ExportSVG
	switch strings.ToLower(strings.TrimSpace(args.Format)) {
	case "png":
		format = ExportPNG
	case "pdf":
		format = ExportPDF
	case "", "svg":
		format = ExportSVG
	default:
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("export_diagram: unsupported export format %q (use svg, png, or pdf)", args.Format), IsError: true}
	}
	// Source and out paths must be workspace-sandboxed (VAL-DIAG-004).
	srcAbs, _, srcErr := t.gw.ResolveInWorkspace(rel)
	if srcErr != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("export_diagram: %v", srcErr), IsError: true}
	}
	outRel := exportRelPath(rel, format)
	outAbs, _, outErr := t.gw.ResolveInWorkspace(outRel)
	if outErr != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("export_diagram: %v", outErr), IsError: true}
	}
	res, eErr := t.export.Export(srcAbs, outAbs, format, t.gw)
	if eErr != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("export_diagram: %v", eErr), IsError: true}
	}
	return Result{
		ID:   call.ID,
		Name: call.Name,
		Content: fmt.Sprintf(
			"exported %s to %s (%d bytes, renderer=%s, available=%v)",
			rel, outRel, res.Bytes, res.Renderer, res.Available,
		),
	}
}

type convertArgs struct {
	Slug   string `json:"slug"`
	Source string `json:"source"`
	Title  string `json:"title"`
}

// execConvert implements convert_diagram (VAL-DIAG-007 Mermaid -> Excalidraw
// bridge). It parses the Mermaid flowchart, converts to an Excalidraw document
// preserving node/edge count, serializes the JSON, and writes it to
// docs/diagrams/<slug>.excalidraw via WriteGateway (undoable). The visual
// canvas (D5) is a separate surface; the editor round-trips the JSON source.
func (t *Tools) execConvert(call Call) Result {
	t.gw.BeginTurn("convert_diagram")
	defer t.gw.EndTurn()
	var args convertArgs
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("convert_diagram: invalid arguments JSON: %v", err), IsError: true}
	}
	slug := strings.TrimSpace(args.Slug)
	if slug == "" {
		return Result{ID: call.ID, Name: call.Name, Content: "convert_diagram: slug is required", IsError: true}
	}
	if !slugRe.MatchString(slug) {
		return Result{ID: call.ID, Name: call.Name, Content: "convert_diagram: slug must be lowercase kebab-case (a-z0-9 and -)", IsError: true}
	}
	doc, gErr := ConvertMermaidToExcalidraw(args.Source)
	if gErr != nil {
		return Result{ID: call.ID, Name: call.Name, Content: gErr.Error(), IsError: true}
	}
	out, sErr := SerializeExcalidraw(doc)
	if sErr != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("convert_diagram: serialize: %v", sErr), IsError: true}
	}
	// Validate the produced JSON round-trips through ParseExcalidraw before
	// writing (round-trip guard, VAL-DIAG-007).
	if _, pErr := ParseExcalidraw(out); pErr != nil {
		return Result{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("convert_diagram: round-trip validation failed: %v", pErr), IsError: true}
	}
	summary := SummarizeExcalidraw(doc)
	rel := diagramRelPath(slug, FormatExcalidraw)
	entry, wErr := t.gw.WriteDiagram(rel, []byte(out))
	if wErr != nil {
		return sandboxOrError(call, "convert_diagram", wErr)
	}
	// Update the index in the same work unit (VAL-DIAG-008).
	title := strings.TrimSpace(args.Title)
	if title == "" {
		title = titleFromSlug(slug, FormatExcalidraw)
	}
	_ = t.index.Update(IndexEntry{Slug: slug, Path: rel, Format: FormatExcalidraw, Title: title}, t.gw)
	return Result{
		ID:   call.ID,
		Name: call.Name,
		Content: fmt.Sprintf(
			"converted mermaid -> excalidraw %s at %s via WriteGateway (undoable); nodes=%d edges=%d (preserved); index updated",
			slug, entry.Snapshot.Rel, summary.Nodes, summary.Edges,
		),
	}
}

// resolveExistingDiagramPath resolves a slug or explicit path to an existing
// diagram source under docs/diagrams/. When format is empty, the on-disk
// extension is used to infer the format.
func (t *Tools) resolveExistingDiagramPath(slug, path, format string) (string, error) {
	root := t.gw.RootPath()
	if p := strings.TrimSpace(path); p != "" {
		abs, rel, err := t.gw.ResolveInWorkspace(p)
		if err != nil {
			return "", fmt.Errorf("update_diagram: %v", err)
		}
		if _, sErr := os.Stat(abs); sErr != nil {
			return "", fmt.Errorf("update_diagram: diagram not found at %s", rel)
		}
		return rel, nil
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", fmt.Errorf("update_diagram: slug or path is required")
	}
	// Search for an existing file matching slug + any diagram extension.
	candidates := candidateExts(format)
	for _, ext := range candidates {
		rel := diagramRelPath(slug, FormatFromExt(ext))
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(abs); err == nil {
			return rel, nil
		}
	}
	return "", fmt.Errorf("update_diagram: no existing diagram found for slug %q under %s/ (looked for extensions: %s)", slug, DiagramsRelDir, strings.Join(candidates, ", "))
}

func candidateExts(format string) []string {
	// An empty/unspecified format searches all known diagram extensions so a
	// slug-only update finds an existing `.excalidraw` file too.
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "mermaid", "mmd":
		// When the caller passes an explicit mermaid format, search only .mmd.
		// When empty, search all formats (we don't know which one was saved).
		if strings.TrimSpace(format) == "" {
			return []string{"mmd", "dbml", "puml", "excalidraw"}
		}
		return []string{"mmd"}
	case "dbml":
		return []string{"dbml"}
	case "plantuml", "puml":
		return []string{"puml"}
	case "excalidraw":
		return []string{"excalidraw"}
	default:
		return []string{"mmd", "dbml", "puml", "excalidraw"}
	}
}

func normalizeFormat(s string) Format {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "mermaid", "mmd":
		return FormatMermaid
	case "dbml":
		return FormatDBML
	case "plantuml", "puml":
		return FormatPlantUML
	case "excalidraw":
		return FormatExcalidraw
	default:
		return FormatMermaid
	}
}

// diagramRelPath returns the workspace-relative path for a new diagram.
func diagramRelPath(slug string, format Format) string {
	return fmt.Sprintf("%s/%s.%s", DiagramsRelDir, slug, ExtForFormat(format))
}

// exportRelPath returns the workspace-relative path for an exported artifact,
// placed next to the source with a .<format> extension.
func exportRelPath(sourceRel string, format ExportFormat) string {
	ext := filepath.Ext(sourceRel)
	out := strings.TrimSuffix(sourceRel, ext) + "." + string(format)
	return out
}

// sandboxOrError converts a WriteGateway error into a clear Result.
func sandboxOrError(call Call, tool string, err error) Result {
	if writegate.IsSandboxDenied(err) {
		return Result{
			ID:      call.ID,
			Name:    call.Name,
			Content: fmt.Sprintf("%s: sandbox denied — diagram path must be under %s/ (%v)", tool, DiagramsRelDir, err),
			IsError: true,
		}
	}
	return Result{
		ID:      call.ID,
		Name:    call.Name,
		Content: fmt.Sprintf("%s failed: %v", tool, err),
		IsError: true,
	}
}
