package skills

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Loader discovers skills progressively and activates named skills for a session.
// Boot does not dump every skill body — only Meta via Discover (VAL-ORCH-004).
type Loader struct {
	mu   sync.RWMutex
	opts DiscoverOptions
	// loaded holds fully activated skills keyed by lower-case name.
	loaded map[string]*Skill
	// cache body loads (name → Skill) so re-load is cheap.
	cache map[string]*Skill
}

// NewLoader builds a session skill loader. IncludeBuiltin defaults true when
// the zero value of IncludeBuiltin is used with NewLoaderFromRoots helpers.
func NewLoader(opts DiscoverOptions) *Loader {
	return &Loader{
		opts:   opts,
		loaded: make(map[string]*Skill),
		cache:  make(map[string]*Skill),
	}
}

// NewDefaultLoader discovers under config home + workspace + builtins.
func NewDefaultLoader(home, workspace string, extra []string) *Loader {
	return NewLoader(DiscoverOptions{
		Home:           home,
		Workspace:      workspace,
		ExtraPaths:     extra,
		IncludeBuiltin: true,
	})
}

// List returns progressive metadata only (no body dump).
func (l *Loader) List() ([]Meta, error) {
	if l == nil {
		return nil, fmt.Errorf("skills: loader is nil")
	}
	l.mu.RLock()
	opts := l.opts
	l.mu.RUnlock()
	opts.IncludeBuiltin = true
	return Discover(opts)
}

// Loaded returns the skills currently activated in this session.
func (l *Loader) Loaded() []*Skill {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*Skill, 0, len(l.loaded))
	for _, sk := range l.loaded {
		cp := *sk
		out = append(out, &cp)
	}
	return out
}

// LoadedNames returns lower-case names of activated skills.
func (l *Loader) LoadedNames() []string {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, 0, len(l.loaded))
	for name := range l.loaded {
		out = append(out, name)
	}
	return out
}

// IsLoaded reports whether name is currently activated.
func (l *Loader) IsLoaded(name string) bool {
	if l == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.loaded[name]
	return ok
}

// Load fully loads a named skill (body + tool policy) and activates it for the
// session. Observable effects: system prompt section + allowed tool delta
// via SessionOverlay (VAL-ORCH-004).
func (l *Loader) Load(name string) (*Skill, error) {
	if l == nil {
		return nil, fmt.Errorf("skills: loader is nil")
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, fmt.Errorf("skills: name is required")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if sk, ok := l.loaded[name]; ok {
		cp := *sk
		return &cp, nil
	}
	if sk, ok := l.cache[name]; ok {
		cp := *sk
		l.loaded[name] = sk
		return &cp, nil
	}

	sk, err := l.loadUnlocked(name)
	if err != nil {
		return nil, err
	}
	l.cache[name] = sk
	l.loaded[name] = sk
	cp := *sk
	return &cp, nil
}

// Unload deactivates a skill for the session (body remains in cache).
func (l *Loader) Unload(name string) bool {
	if l == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.loaded[name]; !ok {
		return false
	}
	delete(l.loaded, name)
	return true
}

// ClearLoaded deactivates all skills (does not clear body cache).
func (l *Loader) ClearLoaded() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loaded = make(map[string]*Skill)
}

// SystemPromptOverlay concatenates prompt sections for all loaded skills.
func (l *Loader) SystemPromptOverlay() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.loaded) == 0 {
		return ""
	}
	// Stable order by name for tests.
	names := make([]string, 0, len(l.loaded))
	for n := range l.loaded {
		names = append(names, n)
	}
	// simple insertion sort
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	var b strings.Builder
	b.WriteString("# Loaded skills\n\n")
	for _, n := range names {
		sk := l.loaded[n]
		b.WriteString(sk.PromptSection())
		if !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// AllowedToolsUnion returns the union of AllowedTools from all loaded skills.
// Empty means no extra tools beyond the baseline registry.
func (l *Loader) AllowedToolsUnion() []string {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	seen := map[string]struct{}{}
	var out []string
	for _, sk := range l.loaded {
		for _, t := range sk.AllowedTools {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			// Expand simple globs later if needed; for now exact names + wildcards stored as-is.
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// HasExtraTool reports whether any loaded skill grants toolName (exact match,
// or a skill-level name for multi-brain tools).
func (l *Loader) HasExtraTool(toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	for _, t := range l.AllowedToolsUnion() {
		if t == toolName {
			return true
		}
		// Simple suffix wildcard: "browser_*"
		if strings.HasSuffix(t, "*") {
			prefix := strings.TrimSuffix(t, "*")
			if strings.HasPrefix(toolName, prefix) {
				return true
			}
		}
	}
	return false
}

func (l *Loader) loadUnlocked(name string) (*Skill, error) {
	// Builtin first (always available even without disk).
	if sk, ok := LoadBuiltin(name); ok {
		return sk, nil
	}

	opts := l.opts
	opts.IncludeBuiltin = true
	list, err := Discover(opts)
	if err != nil {
		return nil, err
	}
	var found *Meta
	for i := range list {
		if list[i].Name == name {
			found = &list[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("skills: unknown skill %q", name)
	}
	if found.Source == SourceBuiltin || found.Path == "" {
		if sk, ok := LoadBuiltin(name); ok {
			return sk, nil
		}
		return nil, fmt.Errorf("skills: builtin %q not loadable", name)
	}
	data, err := os.ReadFile(found.Path)
	if err != nil {
		return nil, fmt.Errorf("skills: read %s: %w", found.Path, err)
	}
	sk, err := ParseFullSkill(string(data))
	if err != nil {
		return nil, err
	}
	if sk.Name == "" {
		sk.Name = name
	}
	sk.Name = strings.ToLower(strings.TrimSpace(sk.Name))
	sk.Path = found.Path
	sk.Source = found.Source
	if sk.Description == "" {
		sk.Description = found.Description
	}
	return &sk, nil
}
