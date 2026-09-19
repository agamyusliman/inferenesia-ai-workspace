package git

import (
	"fmt"
	"strconv"
	"strings"
)

// GraphCommit is one node for the Git Graph tab (branch topology viewer).
type GraphCommit struct {
	// SHA full hex.
	SHA string `json:"sha"`
	// Short is abbreviated sha (7–12).
	Short string `json:"short"`
	// Subject is the first line of the commit message.
	Subject string `json:"subject"`
	// Body is optional remainder of message.
	Body string `json:"body,omitempty"`
	// AuthorName / AuthorEmail / AuthorDate.
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email,omitempty"`
	// AuthorDateISO is committer/author date RFC3339-ish from %aI.
	AuthorDateISO string `json:"author_date"`
	// Parents is space-separated parent shas (0 = root, 1 = normal, 2+ = merge).
	Parents []string `json:"parents"`
	// Refs is decorated ref names (branch/tag) when available.
	Refs []string `json:"refs,omitempty"`
	// Lane is a simple visual column index for the custom graph (0-based).
	Lane int `json:"lane"`
}

// GraphResult is commit history + branch heads for the Graph tab.
type GraphResult struct {
	// RepoID selected repo.
	RepoID string `json:"repo_id"`
	// Branch is the current HEAD branch label.
	Branch string `json:"branch"`
	// Commits newest-first.
	Commits []GraphCommit `json:"commits"`
	// Heads maps short ref name → sha (local + optional remotes truncated).
	Heads []GraphHead `json:"heads"`
	// Truncated when hit limit.
	Truncated bool   `json:"truncated"`
	Message   string `json:"message,omitempty"`
}

// GraphHead is a branch tip for labeling.
type GraphHead struct {
	Name   string `json:"name"`
	SHA    string `json:"sha"`
	Remote bool   `json:"remote,omitempty"`
	// Current is true if this is the checked-out branch.
	Current bool `json:"current,omitempty"`
}

// FileHistoryEntry is one commit that touched a path (file timeline).
type FileHistoryEntry struct {
	SHA           string `json:"sha"`
	Short         string `json:"short"`
	Subject       string `json:"subject"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email,omitempty"`
	AuthorDateISO string `json:"author_date"`
}

// FileHistory is git log for a single path (newest-first).
type FileHistory struct {
	Path      string             `json:"path"`
	Commits   []FileHistoryEntry `json:"commits"`
	Truncated bool               `json:"truncated,omitempty"`
	Message   string             `json:"message,omitempty"`
}

// FileLog returns commits that modified path (relative to this repo root).
func (s *Service) FileLog(path string, limit int) (FileHistory, error) {
	if limit <= 0 {
		limit = 40
	}
	if limit > 100 {
		limit = 100
	}
	out := FileHistory{Path: path, Commits: []FileHistoryEntry{}}
	if !s.isRepo() {
		out.Message = "not a git repository"
		return out, nil
	}
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		out.Message = "invalid path"
		return out, fmt.Errorf("git: invalid path")
	}
	const sep = "\x1f"
	fmtStr := "%H" + sep + "%h" + sep + "%s" + sep + "%an" + sep + "%ae" + sep + "%aI"
	text, err := s.runCapture(
		"log",
		fmt.Sprintf("-%d", limit+1),
		"--pretty=format:"+fmtStr,
		"--",
		path,
	)
	if err != nil {
		out.Message = err.Error()
		return out, nil
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return out, nil
	}
	if len(lines) > limit {
		out.Truncated = true
		lines = lines[:limit]
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, sep)
		for len(parts) < 6 {
			parts = append(parts, "")
		}
		out.Commits = append(out.Commits, FileHistoryEntry{
			SHA:           parts[0],
			Short:         parts[1],
			Subject:       parts[2],
			AuthorName:    parts[3],
			AuthorEmail:   parts[4],
			AuthorDateISO: parts[5],
		})
	}
	return out, nil
}

// FileAtRevision returns file bytes at a commit (git show rev:path). Empty if missing.
func (s *Service) FileAtRevision(path, rev string) ([]byte, error) {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	rev = strings.TrimSpace(rev)
	if path == "" || rev == "" || strings.Contains(path, "..") {
		return nil, fmt.Errorf("git: invalid path or revision")
	}
	if !s.isRepo() {
		return nil, fmt.Errorf("git: not a repository")
	}
	// rev:path form; escape nothing — path already relative.
	spec := rev + ":" + path
	text, err := s.runCapture("show", spec)
	if err != nil {
		// New file in that commit or missing path.
		return nil, nil
	}
	return []byte(text), nil
}

// FileCommitDiff returns unified diff for path introduced by commit sha.
func (s *Service) FileCommitDiff(path, sha string) (string, error) {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	sha = strings.TrimSpace(sha)
	if path == "" || sha == "" {
		return "", fmt.Errorf("git: invalid path or sha")
	}
	if !s.isRepo() {
		return "", fmt.Errorf("git: not a repository")
	}
	// Prefer parent..sha; root commits use --root.
	text, err := s.runCapture("show", "--format=", "--unified=3", sha, "--", path)
	if err != nil {
		return "", err
	}
	return text, nil
}

// Graph returns a limited commit graph for the UI tab.
// Uses git log --date-order with parents + decorations.
func (s *Service) Graph(limit int) (GraphResult, error) {
	if limit <= 0 {
		limit = 80
	}
	if limit > 400 {
		limit = 400
	}
	out := GraphResult{Commits: []GraphCommit{}, Heads: []GraphHead{}}
	if !s.isRepo() {
		out.Message = "not a git repository"
		return out, fmt.Errorf("git: not a repository")
	}
	st, _ := s.Status()
	out.Branch = st.Branch

	// Format: sha %x1f subject %x1f body %x1f author %x1f email %x1f date %x1f parents %x1f deco
	// Use RS unit separators unlikely in messages.
	const sep = "\x1f"
	fmtStr := "%H" + sep + "%h" + sep + "%s" + sep + "%b" + sep + "%an" + sep + "%ae" + sep + "%aI" + sep + "%P" + sep + "%D"
	text, err := s.runCapture(
		"log",
		"--date-order",
		"--all",
		fmt.Sprintf("-%d", limit+1),
		"--pretty=format:"+fmtStr,
	)
	if err != nil {
		out.Message = err.Error()
		return out, err
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > limit {
		out.Truncated = true
		lines = lines[:limit]
	}

	// Parent index for lane assignment: sha → first-seen lane of child side
	laneBySHA := map[string]int{}
	nextLane := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, sep)
		// pad
		for len(parts) < 9 {
			parts = append(parts, "")
		}
		c := GraphCommit{
			SHA:           parts[0],
			Short:         parts[1],
			Subject:       parts[2],
			Body:          strings.TrimSpace(parts[3]),
			AuthorName:    parts[4],
			AuthorEmail:   parts[5],
			AuthorDateISO: parts[6],
		}
		if parts[7] != "" {
			c.Parents = strings.Fields(parts[7])
		}
		if parts[8] != "" {
			// decorations: HEAD -> main, origin/main, tag: v1
			raw := parts[8]
			for _, chunk := range strings.Split(raw, ", ") {
				chunk = strings.TrimSpace(chunk)
				if chunk == "" {
					continue
				}
				chunk = strings.TrimPrefix(chunk, "HEAD -> ")
				chunk = strings.TrimPrefix(chunk, "tag: ")
				c.Refs = append(c.Refs, chunk)
			}
		}
		// Lane: reuse parent's lane if known; else allocate.
		if lane, ok := laneBySHA[c.SHA]; ok {
			c.Lane = lane
		} else if len(c.Parents) > 0 {
			if pl, ok := laneBySHA[c.Parents[0]]; ok {
				c.Lane = pl
			} else {
				c.Lane = nextLane
				nextLane++
			}
		} else {
			c.Lane = nextLane
			nextLane++
		}
		laneBySHA[c.SHA] = c.Lane
		// Children of merge parents may fork to new lanes later via parent[1+].
		for i := 1; i < len(c.Parents); i++ {
			if _, ok := laneBySHA[c.Parents[i]]; !ok {
				laneBySHA[c.Parents[i]] = nextLane
				nextLane++
			}
		}
		out.Commits = append(out.Commits, c)
	}

	// Branch heads
	headsOut, err := s.runCapture("for-each-ref", "--format=%(refname:short)%00%(objectname)%00%(HEAD)", "refs/heads", "refs/remotes")
	if err == nil {
		for _, line := range strings.Split(headsOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			p := strings.Split(line, "\x00")
			if len(p) < 2 {
				continue
			}
			name := p[0]
			sha := p[1]
			current := len(p) >= 3 && p[2] == "*"
			remote := strings.HasPrefix(name, "origin/") || strings.Contains(name, "/")
			// Skip redundant remotes beyond a soft cap of 40 heads for UI.
			out.Heads = append(out.Heads, GraphHead{
				Name:    name,
				SHA:     sha,
				Remote:  remote,
				Current: current,
			})
			if len(out.Heads) >= 40 {
				break
			}
		}
	}
	out.Message = fmt.Sprintf("%d commit(s)", len(out.Commits))
	if out.Truncated {
		out.Message += " (truncated)"
	}
	return out, nil
}

// ParseGraphLimit clamps query string limit.
func ParseGraphLimit(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 80
	}
	if n > 400 {
		return 400
	}
	return n
}
