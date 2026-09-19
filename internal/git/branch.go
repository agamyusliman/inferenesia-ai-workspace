package git

import (
	"fmt"
	"strings"
)

// BranchInfo is one local or remote branch for the checkout picker.
type BranchInfo struct {
	Name     string `json:"name"`
	SHA      string `json:"sha"`
	ShortSHA string `json:"short_sha,omitempty"`
	Remote   bool   `json:"remote"`
	Current  bool   `json:"current"`
	Subject  string `json:"subject,omitempty"`
	Date     string `json:"date,omitempty"`
}

// BranchList is the checkout panel payload.
type BranchList struct {
	RepoID  string       `json:"repo_id,omitempty"`
	Current string       `json:"current"`
	Local   []BranchInfo `json:"local"`
	Remote  []BranchInfo `json:"remote"`
	Message string       `json:"message,omitempty"`
}

// Branches lists local + remote branches for UI checkout picker.
func (s *Service) Branches() (BranchList, error) {
	out := BranchList{Local: []BranchInfo{}, Remote: []BranchInfo{}}
	if !s.isRepo() {
		out.Message = "not a git repository"
		return out, fmt.Errorf("git: not a repository")
	}
	cur, _ := s.runCapture("rev-parse", "--abbrev-ref", "HEAD")
	out.Current = strings.TrimSpace(cur)

	local, err := s.listRefKind("refs/heads", false)
	if err != nil {
		out.Message = err.Error()
		return out, err
	}
	out.Local = local
	remote, err := s.listRefKind("refs/remotes", true)
	if err == nil {
		// Drop remote HEAD pointers
		for _, b := range remote {
			if strings.HasSuffix(b.Name, "/HEAD") || b.Name == "origin/HEAD" {
				continue
			}
			out.Remote = append(out.Remote, b)
		}
	}
	out.Message = fmt.Sprintf("%d local · %d remote", len(out.Local), len(out.Remote))
	return out, nil
}

func (s *Service) listRefKind(pattern string, remote bool) ([]BranchInfo, error) {
	text, err := s.runCapture(
		"for-each-ref",
		"--sort=-committerdate",
		"--format=%(refname:short)%00%(objectname:short)%00%(objectname)%00%(subject)%00%(committerdate:iso-strict)%00%(HEAD)",
		pattern,
	)
	if err != nil {
		return nil, err
	}
	var out []BranchInfo
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := strings.Split(line, "\x00")
		for len(p) < 6 {
			p = append(p, "")
		}
		name := p[0]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, BranchInfo{
			Name:     name,
			ShortSHA: p[1],
			SHA:      p[2],
			Subject:  p[3],
			Date:     p[4],
			Current:  p[5] == "*",
			Remote:   remote,
		})
	}
	return out, nil
}

// Checkout switches branch. createIfMissing runs checkout -b when create=true.
// For remote branches like origin/dev, checks out local tracking branch or creates it.
func (s *Service) Checkout(name string, create bool) (OpResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		err := fmt.Errorf("git: empty branch name")
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if strings.Contains(name, "..") || strings.ContainsAny(name, " \t\n") {
		err := fmt.Errorf("git: invalid branch name %q", name)
		return OpResult{OK: false, Message: err.Error()}, err
	}
	if !s.isRepo() {
		err := fmt.Errorf("git: not a repository")
		return OpResult{OK: false, Message: err.Error()}, err
	}

	var out string
	var err error
	if create {
		out, err = s.runCapture("checkout", "-b", name)
	} else if isRemoteBranchName(name) {
		// Try track first, then local name, then -B from remote tip.
		out, err = s.runCapture("checkout", "--track", name)
		if err != nil {
			local := remoteLocalName(name)
			out2, err2 := s.runCapture("checkout", local)
			if err2 != nil {
				out3, err3 := s.runCapture("checkout", "-B", local, name)
				out, err = out3, err3
			} else {
				out, err = out2, err2
			}
		}
	} else {
		out, err = s.runCapture("checkout", name)
	}
	if err != nil {
		return OpResult{
			OK:      false,
			Message: "checkout failed",
			Detail:  firstNonEmpty(strings.TrimSpace(out), err.Error()),
		}, err
	}
	st, _ := s.Status()
	return OpResult{
		OK:      true,
		Message: fmt.Sprintf("checked out %s", st.Branch),
		Detail:  truncate(out, 2000),
		Status:  &st,
	}, nil
}

func isRemoteBranchName(name string) bool {
	return strings.HasPrefix(name, "origin/") || strings.HasPrefix(name, "upstream/")
}

func remoteLocalName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}
