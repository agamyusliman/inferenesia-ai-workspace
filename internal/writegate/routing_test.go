package writegate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-UNDO-007: every source-kind mutation (fs, multibrain, diagram) is on the
// undo stack as one entry/unit and undo restores prior content.
func TestAllMutationSourcesRouteThroughGateway(t *testing.T) {
	root := t.TempDir()

	// Pre-seed files for each mutation source.
	fsPath := filepath.Join(root, "app.txt")
	mbPath := filepath.Join(root, ".multibrain", "indexes", "agents.md")
	dgPath := filepath.Join(root, "docs", "diagrams", "flow.mmd")
	if err := os.MkdirAll(filepath.Dir(mbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fsPath, []byte("FS-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mbPath, []byte("MB-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dgPath, []byte("DG-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	// One standalone mutation per source (no BeginTurn → one unit each).
	if _, err := g.WriteFileWithSource("app.txt", []byte("FS-AFTER"), SourceFS); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteMultibrain(".multibrain/indexes/agents.md", []byte("MB-AFTER")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.WriteDiagram("docs/diagrams/flow.mmd", []byte("DG-AFTER")); err != nil {
		t.Fatal(err)
	}

	if g.UndoDepth() != 3 {
		t.Fatalf("want 3 undo units (one per mutation source), depth=%d stack=%+v", g.UndoDepth(), summarizeStack(g))
	}

	// Stack must record the source kinds (newest last = diagram, multibrain, fs).
	units := g.UndoStack()
	if len(units) != 3 {
		t.Fatalf("units=%d", len(units))
	}
	wantSources := []string{SourceFS, SourceMultibrain, SourceDiagram}
	for i, want := range wantSources {
		if len(units[i].Entries) != 1 {
			t.Fatalf("unit %d entries=%d", i, len(units[i].Entries))
		}
		got := units[i].Entries[0].Source
		if got != want {
			t.Fatalf("unit %d source=%q want %q", i, got, want)
		}
	}

	// Undo LIFO: diagram, multibrain, fs → prior content for each.
	u1 := g.Undo()
	if u1.Noop || string(mustRead(t, dgPath)) != "DG-BEFORE" {
		t.Fatalf("undo diagram: noop=%v content=%q msg=%s", u1.Noop, mustRead(t, dgPath), u1.Message)
	}
	u2 := g.Undo()
	if u2.Noop || string(mustRead(t, mbPath)) != "MB-BEFORE" {
		t.Fatalf("undo multibrain: noop=%v content=%q msg=%s", u2.Noop, mustRead(t, mbPath), u2.Message)
	}
	u3 := g.Undo()
	if u3.Noop || string(mustRead(t, fsPath)) != "FS-BEFORE" {
		t.Fatalf("undo fs: noop=%v content=%q msg=%s", u3.Noop, mustRead(t, fsPath), u3.Message)
	}
	if string(mustRead(t, dgPath)) != "DG-BEFORE" || string(mustRead(t, mbPath)) != "MB-BEFORE" {
		t.Fatal("prior undos must leave earlier restores intact")
	}
}

// VAL-UNDO-008: Prepare always captures before-mutate; mid-op write failure
// still leaves a recoverable snapshot when the disk was touched / intended path.
func TestSnapshotBeforeMutateSurvivesFailedWrite(t *testing.T) {
	root := t.TempDir()
	fPath := filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	pre := mustRead(t, fPath)

	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	// Inject a mutator that corrupts the file then fails (partial write simulation).
	entry, err := g.Mutate("f.txt", SourceFS, []byte("INTENDED-AFTER"), func(abs string) error {
		// Mid-operation corruption: truncate / write garbage, then fail.
		if werr := os.WriteFile(abs, []byte("PARTIAL-CORRUPT"), 0o644); werr != nil {
			return werr
		}
		return errInjectedWriteFail
	})
	if err == nil {
		t.Fatal("expected mutate failure")
	}
	// Snapshot must have been captured before mutate.
	if !bytes.Equal(entry.Snapshot.Before, []byte("DIRTY-BEFORE")) {
		t.Fatalf("before snapshot = %q, want DIRTY-BEFORE", entry.Snapshot.Before)
	}
	if !entry.Snapshot.Existed {
		t.Fatal("snapshot.Existed must be true")
	}
	// File is partially corrupt on disk.
	if string(mustRead(t, fPath)) != "PARTIAL-CORRUPT" {
		t.Fatalf("expected partial corruption on disk, got %q", mustRead(t, fPath))
	}
	// Undo stack still has recovery unit.
	if g.UndoDepth() != 1 {
		t.Fatalf("failed mid-op write must still leave undo entry, depth=%d", g.UndoDepth())
	}
	res := g.Undo()
	if res.Noop {
		t.Fatalf("undo must restore prior content after failed write: %s", res.Message)
	}
	got := mustRead(t, fPath)
	if !bytes.Equal(got, pre) {
		t.Fatalf("after undo = %q, want DIRTY-BEFORE (no partial-write corruption left)", got)
	}
}

// Clean write failures (disk never changed) do not invent phantom undo units.
func TestCleanWriteFailureDoesNotPolluteStack(t *testing.T) {
	root := t.TempDir()
	// Create a file then make the path a directory so the write target conflicts.
	// Better: use a read-only file that we can't overwrite after Prepare.
	// On Unix, chmod 0444 + not owning root often still allows owner write;
	// instead mutate via Mutate that fails without touching disk.
	fPath := filepath.Join(root, "f.txt")
	if err := os.WriteFile(fPath, []byte("DIRTY-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Mutate("f.txt", SourceFS, []byte("AFTER"), func(abs string) error {
		// Fail without modifying disk.
		return errInjectedWriteFail
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	if g.UndoDepth() != 0 {
		t.Fatalf("clean fail (no disk change) should not push undo, depth=%d", g.UndoDepth())
	}
	if string(mustRead(t, fPath)) != "DIRTY-BEFORE" {
		t.Fatalf("content must be unchanged: %q", mustRead(t, fPath))
	}
}

// VAL-UNDO-009 (package-level): FileChanged-style OnChange fires for write, undo, redo with path.
func TestFileChangedEventsOnWriteUndoRedo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	var events []ChangeEvent
	g.SetOnChange(func(ev ChangeEvent) { events = append(events, ev) })

	if _, err := g.WriteFileWithSource("x.txt", []byte("AFTER"), SourceFS); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Op != "write" || events[0].Rel != "x.txt" {
		t.Fatalf("write event = %+v", events)
	}

	u := g.Undo()
	if u.Noop {
		t.Fatal(u.Message)
	}
	if len(events) != 2 || events[1].Op != "undo" || events[1].Rel != "x.txt" {
		t.Fatalf("undo event = %+v", events)
	}

	r := g.Redo()
	if r.Noop {
		t.Fatal(r.Message)
	}
	if len(events) != 3 || events[2].Op != "redo" || events[2].Rel != "x.txt" {
		t.Fatalf("redo event = %+v", events)
	}
}

// Multibrain and diagram helpers reject paths outside their domains when using the dedicated API.
func TestWriteMultibrainRequiresMultibrainPath(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.WriteMultibrain("not-multibrain.txt", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "multibrain") {
		t.Fatalf("want multibrain path error, got %v", err)
	}
	if g.UndoDepth() != 0 {
		t.Fatal("denied write must not push stack")
	}
}

func TestWriteDiagramRequiresDiagramsPath(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.WriteDiagram("random.md", []byte("x"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "diagram") {
		t.Fatalf("want diagram path error, got %v", err)
	}
}

// VAL-CROSS-005 / VAL-UNDO-007: WriteResearch routes through WriteGateway and
// is undoable. Path must be under docs/research/*.md.
func TestWriteResearchRoutesThroughGatewayAndUndoable(t *testing.T) {
	root := t.TempDir()
	rsPath := filepath.Join(root, "docs", "research", "example.md")
	if err := os.MkdirAll(filepath.Dir(rsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rsPath, []byte("RS-BEFORE"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	// WriteResearch snapshots then mutates.
	entry, err := g.WriteResearch("docs/research/example.md", []byte("RS-AFTER"))
	if err != nil {
		t.Fatalf("WriteResearch: %v", err)
	}
	if entry.Source != SourceResearch {
		t.Fatalf("source=%q want %q", entry.Source, SourceResearch)
	}
	if g.UndoDepth() != 1 {
		t.Fatalf("depth=%d want 1", g.UndoDepth())
	}
	// On-disk content updated.
	if string(mustRead(t, rsPath)) != "RS-AFTER" {
		t.Fatalf("content=%q want RS-AFTER", mustRead(t, rsPath))
	}
	// Undo restores pre-research dirty content (not HEAD).
	u := g.Undo()
	if u.Noop {
		t.Fatalf("undo noop: %s", u.Message)
	}
	if string(mustRead(t, rsPath)) != "RS-BEFORE" {
		t.Fatalf("after undo content=%q want RS-BEFORE", mustRead(t, rsPath))
	}
	// Redo reapplies.
	r := g.Redo()
	if r.Noop {
		t.Fatalf("redo noop: %s", r.Message)
	}
	if string(mustRead(t, rsPath)) != "RS-AFTER" {
		t.Fatalf("after redo content=%q want RS-AFTER", mustRead(t, rsPath))
	}
}

// WriteResearch rejects paths outside docs/research and non-markdown extensions.
func TestWriteResearchRequiresResearchPath(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"random.md",
		"docs/research.txt",
		"docs/research/notes.txt",
		"docs/random.md",
	} {
		_, err := g.WriteResearch(bad, []byte("x"))
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "research") {
			t.Fatalf("want research path error for %q, got %v", bad, err)
		}
	}
	// Path that escapes the workspace entirely is rejected by the sandbox
	// (sandbox denied) before the research-path check fires.
	_, err = g.WriteResearch("../escape.md", []byte("x"))
	if err == nil {
		t.Fatal("expected sandbox/path error for ../escape.md")
	}
	if g.UndoDepth() != 0 {
		t.Fatal("denied write must not push stack")
	}
}

func summarizeStack(g *Gateway) string {
	var b strings.Builder
	for i, u := range g.UndoStack() {
		if i > 0 {
			b.WriteString(" | ")
		}
		for j, e := range u.Entries {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString(e.Source)
			b.WriteString(":")
			b.WriteString(e.Snapshot.Rel)
		}
	}
	return b.String()
}
