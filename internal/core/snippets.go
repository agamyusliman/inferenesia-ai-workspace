package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
)

const (
	// snippetsDirName is under config home (~/.inferenesia/snippets; legacy ~/.yura-ai).
	snippetsDirName = "snippets"
	// snippetsFileName stores the global terminal snippet library as JSON.
	snippetsFileName = "terminal.json"
	// maxSnippetBody caps stored body length (characters).
	maxSnippetBody = 16 * 1024
	// maxSnippetName caps the display name length.
	maxSnippetName = 120
	// maxSnippets caps library size to keep the UI/list cheap.
	maxSnippets = 200
)

// TerminalSnippet is one global terminal snippet (VAL-IDE-036).
// Stored under {configHome}/snippets/terminal.json — not in the workspace.
type TerminalSnippet struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// TerminalSnippetList is the API list payload.
type TerminalSnippetList struct {
	// Path is the absolute library file path under config home.
	Path     string            `json:"path"`
	Snippets []TerminalSnippet `json:"snippets"`
}

// TerminalSnippetSaveRequest creates or updates a snippet.
// When ID is empty a new snippet is created; Name and Body are required for create.
type TerminalSnippetSaveRequest struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Body string `json:"body"`
}

// TerminalSnippetDeleteRequest removes one snippet by id.
type TerminalSnippetDeleteRequest struct {
	ID string `json:"id"`
}

// snippetFile stores on-disk shape (includes a schema version).
type snippetFile struct {
	Version  int               `json:"version"`
	Snippets []TerminalSnippet `json:"snippets"`
}

// defaultTerminalSnippets are shipped on first load. Must contain NO secrets
// (keys, tokens, passwords, private paths with credentials).
func defaultTerminalSnippets() []TerminalSnippet {
	now := time.Now().UTC().Format(time.RFC3339)
	return []TerminalSnippet{
		{
			ID:        "default-git-status",
			Name:      "Git status",
			Body:      "git status\n",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "default-ls",
			Name:      "List files",
			Body:      "ls -la\n",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "default-pwd",
			Name:      "Print working directory",
			Body:      "pwd\n",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

func (s *Service) snippetsPath() string {
	return filepath.Join(s.ConfigHome(), snippetsDirName, snippetsFileName)
}

// ListTerminalSnippets returns the global snippet library under config home.
// Creates the file with safe defaults (no secrets) on first access.
func (s *Service) ListTerminalSnippets() (TerminalSnippetList, error) {
	path := s.snippetsPath()
	items, err := s.loadSnippets(path)
	if err != nil {
		return TerminalSnippetList{}, err
	}
	return TerminalSnippetList{Path: path, Snippets: items}, nil
}

// SaveTerminalSnippet creates or updates a global snippet (VAL-IDE-036).
func (s *Service) SaveTerminalSnippet(req TerminalSnippetSaveRequest) (TerminalSnippet, error) {
	name := strings.TrimSpace(req.Name)
	body := req.Body // preserve intentional trailing newline for terminal insert
	if name == "" {
		return TerminalSnippet{}, fmt.Errorf("snippet: name is required")
	}
	if utf8.RuneCountInString(name) > maxSnippetName {
		return TerminalSnippet{}, fmt.Errorf("snippet: name too long (max %d)", maxSnippetName)
	}
	if body == "" {
		return TerminalSnippet{}, fmt.Errorf("snippet: body is required")
	}
	if utf8.RuneCountInString(body) > maxSnippetBody {
		return TerminalSnippet{}, fmt.Errorf("snippet: body too long (max %d)", maxSnippetBody)
	}
	// Never allow secret-like payload into the global library by default path.
	// Users may still store tooling text; this is a soft reject for obvious keys.
	if secrethyg.ContainsSecretLike(body) || secrethyg.ContainsSecretLike(name) {
		return TerminalSnippet{}, fmt.Errorf("snippet: body or name looks secret-like; refuse to store credentials in the global library")
	}

	path := s.snippetsPath()
	items, err := s.loadSnippets(path)
	if err != nil {
		return TerminalSnippet{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := strings.TrimSpace(req.ID)
	if id != "" {
		for i := range items {
			if items[i].ID == id {
				items[i].Name = name
				items[i].Body = body
				items[i].UpdatedAt = now
				if items[i].CreatedAt == "" {
					items[i].CreatedAt = now
				}
				if err := s.writeSnippets(path, items); err != nil {
					return TerminalSnippet{}, err
				}
				return items[i], nil
			}
		}
		return TerminalSnippet{}, fmt.Errorf("snippet: id %q not found", id)
	}

	if len(items) >= maxSnippets {
		return TerminalSnippet{}, fmt.Errorf("snippet: library full (max %d)", maxSnippets)
	}
	// New id (stable, collision-resistant enough for local library).
	id = fmt.Sprintf("snip_%d", time.Now().UnixNano())
	sn := TerminalSnippet{
		ID:        id,
		Name:      name,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	items = append(items, sn)
	if err := s.writeSnippets(path, items); err != nil {
		return TerminalSnippet{}, err
	}
	return sn, nil
}

// DeleteTerminalSnippet removes a snippet by id.
func (s *Service) DeleteTerminalSnippet(id string) (TerminalSnippetList, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return TerminalSnippetList{}, fmt.Errorf("snippet: id is required")
	}
	path := s.snippetsPath()
	items, err := s.loadSnippets(path)
	if err != nil {
		return TerminalSnippetList{}, err
	}
	out := make([]TerminalSnippet, 0, len(items))
	found := false
	for _, sn := range items {
		if sn.ID == id {
			found = true
			continue
		}
		out = append(out, sn)
	}
	if !found {
		return TerminalSnippetList{}, fmt.Errorf("snippet: id %q not found", id)
	}
	if err := s.writeSnippets(path, out); err != nil {
		return TerminalSnippetList{}, err
	}
	return TerminalSnippetList{Path: path, Snippets: out}, nil
}

func (s *Service) loadSnippets(path string) ([]TerminalSnippet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			defaults := defaultTerminalSnippets()
			if werr := s.writeSnippets(path, defaults); werr != nil {
				return nil, werr
			}
			return defaults, nil
		}
		return nil, fmt.Errorf("snippet: read %s: %w", path, err)
	}
	var f snippetFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("snippet: parse %s: %w", path, err)
	}
	if f.Snippets == nil {
		f.Snippets = []TerminalSnippet{}
	}
	// Hard filter: drop anything secret-like from older files so defaults stay safe.
	clean := make([]TerminalSnippet, 0, len(f.Snippets))
	for _, sn := range f.Snippets {
		if secrethyg.ContainsSecretLike(sn.Body) || secrethyg.ContainsSecretLike(sn.Name) {
			continue
		}
		clean = append(clean, sn)
	}
	return clean, nil
}

func (s *Service) writeSnippets(path string, items []TerminalSnippet) error {
	if err := os.MkdirAll(filepath.Dir(path), config.DirPerm); err != nil {
		return fmt.Errorf("snippet: mkdir: %w", err)
	}
	payload, err := json.MarshalIndent(snippetFile{Version: 1, Snippets: items}, "", "  ")
	if err != nil {
		return fmt.Errorf("snippet: marshal: %w", err)
	}
	payload = append(payload, '\n')
	// Snippets are not secret material by policy; still use owner-only home norms.
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("snippet: write: %w", err)
	}
	// Ensure parent dir remains 0700 (config home semantics).
	_ = os.Chmod(filepath.Dir(path), config.DirPerm)
	return nil
}
