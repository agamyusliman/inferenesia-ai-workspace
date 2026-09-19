package diagram

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrPlantUMLUnavailable is returned when the plantuml binary/jar is missing
// (VAL-DIAG-006). Mermaid/ER paths must never be blocked by this error.
var ErrPlantUMLUnavailable = errors.New("plantuml unavailable: no plantuml binary/jar found on PATH or PLANTUML_JAR (install plantuml or set PLANTUML_JAR)")

// PlantUMLResolver locates the PlantUML renderer. It checks:
//  1. PLANTUML_JAR env (jar path; requires java)
//  2. plantuml binary on PATH
//
// Returns a PlantUMLRenderer when available, otherwise ErrPlantUMLUnavailable.
type PlantUMLResolver struct {
	jarPath string
	javaBin string
	binPath string
}

// ResolvePlantUML attempts to locate a PlantUML renderer.
func ResolvePlantUML() (*PlantUMLResolver, error) {
	// 1) PLANTUML_JAR + java
	if jar := strings.TrimSpace(os.Getenv("PLANTUML_JAR")); jar != "" {
		if _, err := os.Stat(jar); err == nil {
			if java, err := exec.LookPath("java"); err == nil {
				return &PlantUMLResolver{jarPath: jar, javaBin: java}, nil
			}
		}
	}
	// 2) plantuml binary
	if bin, err := exec.LookPath("plantuml"); err == nil {
		return &PlantUMLResolver{binPath: bin}, nil
	}
	return nil, ErrPlantUMLUnavailable
}

// Available reports whether a PlantUML renderer is available.
func (r *PlantUMLResolver) Available() bool {
	return r != nil && (r.binPath != "" || (r.jarPath != "" && r.javaBin != ""))
}

// Command returns the exec.Cmd base (without output args) for rendering, or
// an error when unavailable.
func (r *PlantUMLResolver) Command(sourceFile string) (*exec.Cmd, error) {
	if !r.Available() {
		return nil, ErrPlantUMLUnavailable
	}
	if r.binPath != "" {
		return exec.Command(r.binPath, sourceFile), nil
	}
	// jar path: java -jar plantuml.jar <file>
	return exec.Command(r.javaBin, "-jar", r.jarPath, sourceFile), nil
}

// ErrMermaidRendererUnavailable is returned when no mermaid renderer (mmdc) is
// installed. Export still produces an SVG placeholder so the tool does not
// hard-fail on the Mermaid path, but PNG requires mmdc.
var ErrMermaidRendererUnavailable = errors.New("mermaid renderer unavailable: install @mermaid-js/mermaid-cli (mmdc) for PNG export")

// MermaidRenderer locates the mermaid-cli (mmdc) binary.
type MermaidRenderer struct {
	bin string
}

// ResolveMermaid returns a MermaidRenderer when mmdc is on PATH.
func ResolveMermaid() (*MermaidRenderer, error) {
	bin, err := exec.LookPath("mmdc")
	if err != nil {
		return nil, ErrMermaidRendererUnavailable
	}
	return &MermaidRenderer{bin: bin}, nil
}

// Available reports whether mmdc is installed.
func (m *MermaidRenderer) Available() bool {
	return m != nil && m.bin != ""
}

// ExportFormat is a rendered export target (VAL-DIAG-004).
type ExportFormat string

const (
	ExportSVG ExportFormat = "svg"
	ExportPNG ExportFormat = "png"
	ExportPDF ExportFormat = "pdf"
)

// Exporter renders a saved diagram source file to a target format.
// It uses the appropriate renderer for the source format:
//   - Mermaid: mmdc when available (SVG/PNG/PDF); SVG placeholder when mmdc missing
//   - PlantUML: plantuml binary/jar when available; ErrPlantUMLUnavailable otherwise
//   - DBML: not rendered (returns ErrDBMLRenderUnsupported)
//
// outPath is the absolute, workspace-sandboxed target path. The caller is
// responsible for sandbox checks; Exporter writes only to outPath.
type Exporter struct {
	mermaid *MermaidRenderer
	plant   *PlantUMLResolver
}

// NewExporter builds an Exporter with discovered renderers. Missing renderers
// are tolerated (Export returns a clear error per format, VAL-DIAG-006).
func NewExporter() *Exporter {
	m, _ := ResolveMermaid()
	p, _ := ResolvePlantUML()
	return &Exporter{mermaid: m, plant: p}
}

// ExportResult is the outcome of an Export.
type ExportResult struct {
	OutPath   string
	Format    ExportFormat
	Bytes     int
	Renderer  string // "mmdc" | "plantuml" | "svg-placeholder"
	Available bool   // false when a renderer was unavailable (placeholder used)
}

// Export renders sourceFile (absolute) to outPath (absolute) in the given
// format, inferring the source format from sourceFile's extension. When
// `gw` is non-nil, the rendered artifact is installed at outPath via
// WriteGateway so exports land on the undo stack (VAL-UNDO-007). Passing
// `gw = nil` keeps the legacy direct-write path for tests / CLI dry runs.
func (e *Exporter) Export(sourceFile, outPath string, format ExportFormat, gw GatewayBackend) (ExportResult, error) {
	if e == nil {
		return ExportResult{}, fmt.Errorf("diagram: nil exporter")
	}
	if _, err := os.Stat(sourceFile); err != nil {
		return ExportResult{}, fmt.Errorf("diagram: source file not found: %w", err)
	}
	srcFormat := FormatFromExt(strings.ToLower(filepath.Ext(sourceFile)))
	switch srcFormat {
	case FormatMermaid:
		return e.exportMermaid(sourceFile, outPath, format, gw)
	case FormatPlantUML:
		return e.exportPlantUML(sourceFile, outPath, format, gw)
	case FormatDBML:
		return ExportResult{}, fmt.Errorf("diagram: DBML rendering is not supported by the built-in exporter (use dbml-renderer externally)")
	default:
		return ExportResult{}, fmt.Errorf("diagram: cannot export format %q", srcFormat)
	}
}

func (e *Exporter) exportMermaid(sourceFile, outPath string, format ExportFormat, gw GatewayBackend) (ExportResult, error) {
	if e.mermaid == nil || !e.mermaid.Available() {
		// SVG placeholder: never hard-fail Mermaid path when mmdc missing (VAL-DIAG-006).
		if format == ExportSVG {
			data := svgPlaceholder(sourceFile)
			if err := installArtifact(gw, outPath, []byte(data)); err != nil {
				return ExportResult{}, fmt.Errorf("diagram: write svg placeholder: %w", err)
			}
			return ExportResult{
				OutPath:   outPath,
				Format:    ExportSVG,
				Bytes:     len(data),
				Renderer:  "svg-placeholder",
				Available: false,
			}, nil
		}
		return ExportResult{}, ErrMermaidRendererUnavailable
	}
	// mmdc writes directly to disk; capture into a temp file and install via
	// gateway so the export lands on the undo stack (VAL-UNDO-007).
	tmp := outPath + ".mmdcout"
	cmd := exec.Command(e.mermaid.bin, "-i", sourceFile, "-o", tmp)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return ExportResult{}, fmt.Errorf("diagram: mmdc render failed: %w: %s", err, stderr.String())
	}
	data, err := os.ReadFile(tmp)
	_ = os.Remove(tmp)
	if err != nil {
		return ExportResult{}, fmt.Errorf("diagram: read rendered output: %w", err)
	}
	if err := installArtifact(gw, outPath, data); err != nil {
		return ExportResult{}, fmt.Errorf("diagram: install rendered output: %w", err)
	}
	return ExportResult{
		OutPath:   outPath,
		Format:    format,
		Bytes:     len(data),
		Renderer:  "mmdc",
		Available: true,
	}, nil
}

// installArtifact writes bytes to outPath via WriteGateway when gw is non-nil,
// else falls back to a direct os.WriteFile (test / CLI dry-run path).
func installArtifact(gw GatewayBackend, outPath string, data []byte) error {
	if gw == nil {
		return os.WriteFile(outPath, data, 0o644)
	}
	_, rel, err := gw.ResolveInWorkspace(outPath)
	if err != nil {
		return err
	}
	if _, err := gw.WriteFileWithSource(rel, data, "diagram"); err != nil {
		return err
	}
	return nil
}

func (e *Exporter) exportPlantUML(sourceFile, outPath string, format ExportFormat, gw GatewayBackend) (ExportResult, error) {
	if e.plant == nil || !e.plant.Available() {
		return ExportResult{}, ErrPlantUMLUnavailable
	}
	cmd, err := e.plant.Command(sourceFile)
	if err != nil {
		return ExportResult{}, err
	}
	flag := "-tsvg"
	switch format {
	case ExportPNG:
		flag = "-tpng"
	case ExportPDF:
		flag = "-tpdf"
	}
	outDir := filepath.Dir(outPath)
	cmd.Args = append(cmd.Args, flag, "-o", outDir, sourceFile)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ExportResult{}, fmt.Errorf("diagram: plantuml render failed: %w: %s", err, stderr.String())
	}
	// plantuml writes <basename>.<ext> in outDir. Read + install via gateway.
	base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	genExt := string(format)
	generated := filepath.Join(outDir, base+"."+genExt)
	data, err := os.ReadFile(generated)
	if err != nil {
		return ExportResult{}, fmt.Errorf("diagram: read plantuml output: %w", err)
	}
	if generated != outPath {
		_ = os.Remove(generated)
	}
	if err := installArtifact(gw, outPath, data); err != nil {
		return ExportResult{}, fmt.Errorf("diagram: install plantuml output: %w", err)
	}
	return ExportResult{
		OutPath:   outPath,
		Format:    format,
		Bytes:     len(data),
		Renderer:  "plantuml",
		Available: true,
	}, nil
}

// svgPlaceholder returns a minimal SVG that references the source file so the
// export never hard-fails when mmdc is absent (Mermaid path resilience, VAL-DIAG-006).
func svgPlaceholder(sourceFile string) string {
	name := filepath.Base(sourceFile)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!-- Inferenesia diagram export placeholder. Install mmdc (@mermaid-js/mermaid-cli) to render %s to a real image. -->
<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120">
  <rect width="640" height="120" fill="#f5f5f5" stroke="#ccc"/>
  <text x="20" y="50" font-family="sans-serif" font-size="16" fill="#333">Mermaid diagram: %s</text>
  <text x="20" y="80" font-family="sans-serif" font-size="12" fill="#666">SVG placeholder (mmdc not installed). Source: %s</text>
</svg>
`, name, name, name)
}
