package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverOptions configures progressive skill discovery (metadata only).
type DiscoverOptions struct {
	// Home is the config home (~/.inferenesia). When empty, personal skills are skipped.
	Home string
	// Workspace is the project root. When empty, project/claude/agents dirs are skipped.
	Workspace string
	// ExtraPaths are additional roots that contain skill-name/SKILL.md trees
	// (from config.skills.paths).
	ExtraPaths []string
	// IncludeBuiltin, when true (default if zero-value treated carefully by callers),
	// injects builtin skill metas without loading full bodies.
	IncludeBuiltin bool
}

// Discover walks known skill roots and returns metadata only (progressive
// disclosure). It does not read skill bodies or dump every file's content
// beyond frontmatter name/description (VAL-ORCH-004).
//
// Priority (later override earlier by name, first-wins for listing order —
// personal overrides project for same name by replacing in map; list is
// stable: builtins first, then personal, project, compat, extra):
//
//  1. builtin
//  2. ~/.inferenesia/skills/**/SKILL.md
//  3. <workspace>/.inferenesia/skills/**
//  4. <workspace>/.claude/skills/**
//  5. <workspace>/.agents/skills/**
//  6. ExtraPaths
func Discover(opts DiscoverOptions) ([]Meta, error) {
	// name → Meta; later discovery roots override earlier (disk wins over builtin).
	byName := map[string]Meta{}
	order := []string{} // first-seen order for stable listing of distinct names

	remember := func(m Meta) {
		name := strings.ToLower(strings.TrimSpace(m.Name))
		if name == "" {
			return
		}
		m.Name = name
		if _, exists := byName[name]; !exists {
			order = append(order, name)
		}
		byName[name] = m
	}

	if opts.IncludeBuiltin {
		for _, m := range BuiltinMetas() {
			remember(m)
		}
	}

	// Personal: ~/.inferenesia/skills
	if home := strings.TrimSpace(opts.Home); home != "" {
		if err := walkSkillRoot(filepath.Join(home, "skills"), SourcePersonal, remember); err != nil {
			return nil, err
		}
	}

	ws := strings.TrimSpace(opts.Workspace)
	if ws != "" {
		roots := []struct {
			dir string
			src Source
		}{
			{filepath.Join(ws, ".inferenesia", "skills"), SourceProject},
			{filepath.Join(ws, ".yura-ai", "skills"), SourceProject},
			{filepath.Join(ws, ".claude", "skills"), SourceClaude},
			{filepath.Join(ws, ".agents", "skills"), SourceAgents},
		}
		for _, r := range roots {
			if err := walkSkillRoot(r.dir, r.src, remember); err != nil {
				return nil, err
			}
		}
	}

	for _, extra := range opts.ExtraPaths {
		extra = strings.TrimSpace(extra)
		if extra == "" {
			continue
		}
		if err := walkSkillRoot(extra, SourceExtra, remember); err != nil {
			return nil, err
		}
	}

	out := make([]Meta, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	// Stable secondary sort by name within same source for readability.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			return out[i].Name < out[j].Name
		}
		return sourceRank(out[i].Source) < sourceRank(out[j].Source)
	})
	return out, nil
}

func sourceRank(s Source) int {
	switch s {
	case SourceBuiltin:
		return 0
	case SourcePersonal:
		return 1
	case SourceProject:
		return 2
	case SourceClaude:
		return 3
	case SourceAgents:
		return 4
	case SourceExtra:
		return 5
	default:
		return 9
	}
}

// walkSkillRoot finds **/SKILL.md (or skill.md) under root and parses frontmatter only.
func walkSkillRoot(root string, src Source, remember func(Meta)) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skills: stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable nodes; discovery must not crash list.
			return nil
		}
		if d.IsDir() {
			// Skip hidden dirs except the root itself; allow normal skill dirs.
			base := d.Name()
			if base == ".git" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.EqualFold(name, "SKILL.md") {
			return nil
		}
		meta, perr := ParseFrontmatterFile(path)
		if perr != nil {
			// Malformed skill: skip
			return nil
		}
		if meta.Name == "" {
			// Fall back to parent directory name.
			meta.Name = filepath.Base(filepath.Dir(path))
		}
		meta.Name = strings.ToLower(strings.TrimSpace(meta.Name))
		meta.Path = path
		meta.Source = src
		// Default user-invocable true when not specified in frontmatter.
		// ParseFrontmatter sets UserInvocable from YAML; zero value means true
		// only when frontmatter omitted the key — handled in ParseFrontmatter.
		remember(meta)
		return nil
	})
}

// ResolveMeta finds one skill's metadata by name without loading the body.
func ResolveMeta(opts DiscoverOptions, name string) (Meta, bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return Meta{}, false, fmt.Errorf("skills: name is required")
	}
	opts.IncludeBuiltin = true
	list, err := Discover(opts)
	if err != nil {
		return Meta{}, false, err
	}
	for _, m := range list {
		if m.Name == name {
			return m, true, nil
		}
	}
	return Meta{}, false, nil
}
