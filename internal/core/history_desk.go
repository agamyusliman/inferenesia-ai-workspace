package core

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// AgentChangeEntry is one row in the desktop agent-changes timeline (VAL-DESK-012).
// This is WriteGateway undo/turn history — not git log / raw git history.
type AgentChangeEntry struct {
	// Index is 1-based from newest undo unit (1 = last turn / top of stack).
	Index int `json:"index"`
	// UnitID is the stack unit id.
	UnitID string `json:"unit_id"`
	// TurnID is the agent turn id when set.
	TurnID string `json:"turn_id,omitempty"`
	// CreatedAt is when the unit was pushed (RFC3339 in JSON).
	CreatedAt time.Time `json:"created_at"`
	// Paths are workspace-relative paths changed in this unit.
	Paths []string `json:"paths"`
	// Sources are distinct mutation sources (fs, multibrain, diagram, user).
	Sources []string `json:"sources"`
	// FileCount is len(Paths).
	FileCount int `json:"file_count"`
	// Kind is a short UI label: agent_turn | user_edit | mixed.
	Kind string `json:"kind"`
	// Summary is a one-line human description for the timeline row.
	Summary string `json:"summary"`
}

// AgentChangeHistory is the desktop timeline payload (VAL-DESK-012 / VAL-MEM-012).
// Distinct from git history: entries come only from WriteGateway undo units.
type AgentChangeHistory struct {
	// Title is always the product label for this panel (not "Git history").
	Title string `json:"title"`
	// Subtitle clarifies this is agent WriteGateway history, not git log.
	Subtitle string `json:"subtitle"`
	// EmptyMessage is shown when there are no agent/user WriteGateway units.
	EmptyMessage string `json:"empty_message"`
	// Entries are newest-first.
	Entries []AgentChangeEntry `json:"entries"`
	// Count is len(Entries).
	Count int `json:"count"`
	// Source labels the data origin for UI probes (writegateway, not git).
	Source string `json:"source"`
}

const (
	// AgentChangesTitle is the desktop panel title (VAL-DESK-012).
	AgentChangesTitle = "Agent changes"
	// AgentChangesSubtitle distinguishes this timeline from raw git history.
	AgentChangesSubtitle = "WriteGateway file mutations (pre-agent dirty stack) — not git log"
	// AgentChangesSource tags the API origin for validators/UI probes.
	AgentChangesSource = "writegateway"
)

// GetAgentChangeHistory returns the newest-first timeline of agent (and user-edit)
// file mutations from the active workspace WriteGateway stack. This is intentionally
// separate from git history (VAL-DESK-012).
func (s *Service) GetAgentChangeHistory() (AgentChangeHistory, error) {
	out := AgentChangeHistory{
		Title:        AgentChangesTitle,
		Subtitle:     AgentChangesSubtitle,
		EmptyMessage: writegate.MsgEmptyHistory,
		Entries:      []AgentChangeEntry{},
		Count:        0,
		Source:       AgentChangesSource,
	}
	if s == nil {
		return out, nil
	}
	root, err := s.activeRoot()
	if err != nil {
		// No workspace → empty timeline (not a hard error for the panel).
		out.EmptyMessage = "open a workspace to see agent changes"
		return out, nil
	}
	g, err := s.openGateway(root)
	if err != nil {
		return out, err
	}
	raw := g.History()
	if len(raw) == 0 {
		return out, nil
	}
	entries := make([]AgentChangeEntry, 0, len(raw))
	for _, e := range raw {
		kind := classifyChangeKind(e.Sources)
		entries = append(entries, AgentChangeEntry{
			Index:     e.Index,
			UnitID:    e.UnitID,
			TurnID:    e.TurnID,
			CreatedAt: e.CreatedAt,
			Paths:     e.Paths,
			Sources:   e.Sources,
			FileCount: e.FileCount,
			Kind:      kind,
			Summary:   summarizeChange(e, kind),
		})
	}
	out.Entries = entries
	out.Count = len(entries)
	return out, nil
}

func classifyChangeKind(sources []string) string {
	hasUser := false
	hasAgent := false
	for _, s := range sources {
		switch s {
		case writegate.SourceUser:
			hasUser = true
		case writegate.SourceFS, writegate.SourceMultibrain, writegate.SourceDiagram, "":
			hasAgent = true
		default:
			// Unknown tags still count as non-user agent-side mutations.
			if s != writegate.SourceUser {
				hasAgent = true
			}
		}
	}
	switch {
	case hasUser && hasAgent:
		return "mixed"
	case hasUser:
		return "user_edit"
	default:
		return "agent_turn"
	}
}

func summarizeChange(e writegate.HistoryEntry, kind string) string {
	n := e.FileCount
	if n == 0 {
		n = len(e.Paths)
	}
	turn := e.TurnID
	if turn == "" {
		turn = e.UnitID
		if len(turn) > 8 {
			turn = turn[:8]
		}
	}
	switch kind {
	case "user_edit":
		return formatChangeSummary("user edit", n, turn)
	case "mixed":
		return formatChangeSummary("mixed (agent + user)", n, turn)
	default:
		return formatChangeSummary("agent turn", n, turn)
	}
}

func formatChangeSummary(label string, n int, turn string) string {
	if turn != "" {
		return label + " · " + strconv.Itoa(n) + " file(s) · " + turn
	}
	return label + " · " + strconv.Itoa(n) + " file(s)"
}

type FileTimelineEntry struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Kind      string    `json:"kind,omitempty"`
	Summary   string    `json:"summary"`
	Author    string    `json:"author,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	SHA       string    `json:"sha,omitempty"`
	ShortSHA  string    `json:"short_sha,omitempty"`
	UnitID    string    `json:"unit_id,omitempty"`
	Paths     []string  `json:"paths,omitempty"`
}

type FileTimeline struct {
	Path     string              `json:"path"`
	Entries  []FileTimelineEntry `json:"entries"`
	Count    int                 `json:"count"`
	LocalN   int                 `json:"local_count"`
	GitN     int                 `json:"git_count"`
	Message  string              `json:"message,omitempty"`
	EmptyMsg string              `json:"empty_message,omitempty"`
}

func (s *Service) GetFileTimeline(path string, limit int) (FileTimeline, error) {
	if limit <= 0 {
		limit = 50
	}
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	out := FileTimeline{
		Path:     path,
		Entries:  []FileTimelineEntry{},
		EmptyMsg: "select a file to see local and git history",
	}
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		out.Message = "invalid path"
		return out, nil
	}
	if s == nil {
		return out, nil
	}

	if hist, err := s.GetAgentChangeHistory(); err == nil {
		for _, e := range hist.Entries {
			if !pathsContain(e.Paths, path) {
				continue
			}
			out.Entries = append(out.Entries, FileTimelineEntry{
				ID:        "local:" + e.UnitID,
				Source:    "local",
				Kind:      e.Kind,
				Summary:   e.Summary,
				CreatedAt: e.CreatedAt,
				UnitID:    e.UnitID,
				Paths:     e.Paths,
			})
			out.LocalN++
		}
	}

	gs, selected, _, err := s.gitServiceFor("")
	if err == nil && gs != nil {
		rel := path
		if selected.Root != "" {
			wsRoot, werr := s.activeRoot()
			if werr == nil {
				absFile := filepath.Join(wsRoot, filepath.FromSlash(path))
				if r, rerr := filepath.Rel(selected.Root, absFile); rerr == nil {
					rel = filepath.ToSlash(r)
				}
			}
		}
		if fl, ferr := gs.FileLog(rel, limit); ferr == nil {
			for _, c := range fl.Commits {
				t := parseGitDate(c.AuthorDateISO)
				out.Entries = append(out.Entries, FileTimelineEntry{
					ID:        "git:" + c.SHA,
					Source:    "git",
					Kind:      "commit",
					Summary:   c.Subject,
					Author:    c.AuthorName,
					CreatedAt: t,
					SHA:       c.SHA,
					ShortSHA:  c.Short,
				})
				out.GitN++
			}
			if fl.Message != "" && out.GitN == 0 {
				out.Message = fl.Message
			}
		}
	}

	sort.SliceStable(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i].CreatedAt, out.Entries[j].CreatedAt
		if a.IsZero() && b.IsZero() {
			return i < j
		}
		if a.IsZero() {
			return false
		}
		if b.IsZero() {
			return true
		}
		return a.After(b)
	})
	if len(out.Entries) > limit {
		out.Entries = out.Entries[:limit]
	}
	out.Count = len(out.Entries)
	if out.Count == 0 {
		out.EmptyMsg = "no local or git history for this file"
	}
	return out, nil
}

type FileTimelineChange struct {
	Path      string `json:"path"`
	Source    string `json:"source"`
	Title     string `json:"title"`
	Original  string `json:"original"`
	Modified  string `json:"modified"`
	LabelOrig string `json:"label_original"`
	LabelMod  string `json:"label_modified"`
	Message   string `json:"message,omitempty"`
	UnitID    string `json:"unit_id,omitempty"`
	SHA       string `json:"sha,omitempty"`
}

func (s *Service) GetFileTimelineChange(path, source, unitID, sha string) (FileTimelineChange, error) {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	out := FileTimelineChange{Path: path, Source: source}
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		out.Message = "invalid path"
		return out, fmt.Errorf("core: invalid path")
	}
	if s == nil {
		return out, fmt.Errorf("core: service not ready")
	}
	source = strings.TrimSpace(strings.ToLower(source))
	switch source {
	case "local":
		return s.localTimelineChange(path, unitID)
	case "git":
		return s.gitTimelineChange(path, sha)
	default:
		out.Message = "unknown source"
		return out, fmt.Errorf("core: unknown timeline source")
	}
}

func (s *Service) localTimelineChange(path, unitID string) (FileTimelineChange, error) {
	out := FileTimelineChange{
		Path:      path,
		Source:    "local",
		UnitID:    unitID,
		LabelOrig: "before",
		LabelMod:  "after",
		Title:     "local change",
	}
	root, err := s.activeRoot()
	if err != nil {
		out.Message = err.Error()
		return out, nil
	}
	g, err := s.openGateway(root)
	if err != nil {
		out.Message = err.Error()
		return out, nil
	}
	u := g.UnitByID(unitID)
	if u == nil {
		for _, cand := range g.UndoStack() {
			if cand.ID == unitID || strings.HasPrefix(cand.ID, unitID) || strings.HasPrefix(unitID, cand.ID) {
				cp := cand
				u = &cp
				break
			}
		}
	}
	want := strings.TrimPrefix(path, "./")
	fillFromEntry := func(e writegate.Entry, unit *writegate.StackUnit) FileTimelineChange {
		if e.Snapshot.Before != nil {
			out.Original = string(e.Snapshot.Before)
		}
		if e.AfterAbsent {
			out.Modified = ""
			out.LabelMod = "deleted"
		} else if e.After != nil {
			out.Modified = string(e.After)
		}
		if !e.Snapshot.Existed {
			out.LabelOrig = "did not exist"
		}
		id := unit.ID
		if len(id) > 8 {
			id = id[:8]
		}
		out.Title = "local · " + id
		out.UnitID = unit.ID
		return out
	}
	matchRel := func(rel string) bool {
		rel = strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "./")
		return rel == want || strings.HasSuffix(rel, "/"+want) || strings.HasSuffix(want, "/"+rel)
	}
	if u != nil {
		for _, e := range u.Entries {
			if matchRel(e.Snapshot.Rel) {
				return fillFromEntry(e, u), nil
			}
		}
	}
	stack := g.UndoStack()
	for i := len(stack) - 1; i >= 0; i-- {
		unit := stack[i]
		for _, e := range unit.Entries {
			if matchRel(e.Snapshot.Rel) {
				cp := unit
				return fillFromEntry(e, &cp), nil
			}
		}
	}
	if cur, rerr := s.ReadFile("", path); rerr == nil {
		out.Modified = cur
		out.Original = ""
		out.LabelOrig = "unknown previous"
		out.Message = "local snapshot not found; showing current file"
		return out, nil
	}
	out.Message = "local change not found"
	return out, nil
}

func (s *Service) gitTimelineChange(path, sha string) (FileTimelineChange, error) {
	out := FileTimelineChange{
		Path:      path,
		Source:    "git",
		SHA:       sha,
		LabelOrig: "parent",
		LabelMod:  "commit",
		Title:     "git " + shortSHA(sha),
	}
	gs, selected, _, err := s.gitServiceFor("")
	if err != nil {
		out.Message = err.Error()
		return out, nil
	}
	rel := path
	if selected.Root != "" {
		wsRoot, werr := s.activeRoot()
		if werr == nil {
			absFile := filepath.Join(wsRoot, filepath.FromSlash(path))
			if r, rerr := filepath.Rel(selected.Root, absFile); rerr == nil {
				rel = filepath.ToSlash(r)
			}
		}
	}
	mod, merr := gs.FileAtRevision(rel, sha)
	if merr == nil && mod != nil {
		out.Modified = string(mod)
	}
	parent := sha + "^"
	orig, oerr := gs.FileAtRevision(rel, parent)
	if oerr == nil && orig != nil {
		out.Original = string(orig)
	} else {
		out.LabelOrig = "did not exist"
	}
	out.LabelMod = shortSHA(sha)
	if out.Original == "" && out.Modified == "" {
		out.Message = "file not found in that commit"
	}
	return out, nil
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func pathsContain(paths []string, want string) bool {
	want = strings.TrimPrefix(strings.ReplaceAll(want, "\\", "/"), "./")
	for _, p := range paths {
		p = strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "./")
		if p == want || strings.HasSuffix(p, "/"+want) || strings.HasSuffix(want, "/"+p) {
			return true
		}
	}
	return false
}

func parseGitDate(iso string) time.Time {
	if iso == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05-07:00", iso); err == nil {
		return t
	}
	return time.Time{}
}
