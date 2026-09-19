package skills

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterYAML is the subset of SKILL.md YAML frontmatter we understand.
type frontmatterYAML struct {
	Name                   string           `yaml:"name"`
	Description            string           `yaml:"description"`
	DisableModelInvocation *bool            `yaml:"disable-model-invocation"`
	UserInvocable          *bool            `yaml:"user-invocable"`
	AllowedTools           flexibleStr      `yaml:"allowed-tools"`
	MCPServers             []MCPServerSpec  `yaml:"mcp-servers"`
}

// flexibleStr accepts YAML string or list of strings for allowed-tools.
type flexibleStr []string

func (f *flexibleStr) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		s := strings.TrimSpace(value.Value)
		if s == "" {
			*f = nil
			return nil
		}
		// Comma-separated or single token.
		parts := strings.Split(s, ",")
		var out []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		*f = out
		return nil
	case yaml.SequenceNode:
		var out []string
		for _, n := range value.Content {
			s := strings.TrimSpace(n.Value)
			if s != "" {
				out = append(out, s)
			}
		}
		*f = out
		return nil
	default:
		return fmt.Errorf("allowed-tools: expected string or list")
	}
}

// SplitFrontmatter splits SKILL.md into YAML frontmatter + body.
// Supports --- ... --- delimiters. If no frontmatter, raw is all body.
func SplitFrontmatter(raw string) (yamlBlock, body string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	trim := strings.TrimLeft(raw, "\n")
	if !strings.HasPrefix(trim, "---") {
		return "", raw
	}
	// Find end of opening ---
	rest := strings.TrimPrefix(trim, "---")
	rest = strings.TrimPrefix(rest, "\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		// No closing fence: treat all as body
		return "", raw
	}
	yamlBlock = rest[:idx]
	body = rest[idx+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	return yamlBlock, body
}

// ParseFrontmatterFile reads only enough of the file to extract metadata
// (full file is small SKILL.md; we still treat body as unloaded for callers
// that only use Meta — Discover discards body).
func ParseFrontmatterFile(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	meta, _, err := ParseSkillMarkdown(string(data))
	return meta, err
}

// ParseSkillMarkdown parses a full SKILL.md into Meta + body.
func ParseSkillMarkdown(raw string) (Meta, string, error) {
	yamlBlock, body := SplitFrontmatter(raw)
	meta := Meta{
		UserInvocable: true, // default
	}
	if strings.TrimSpace(yamlBlock) != "" {
		var fm frontmatterYAML
		if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
			return Meta{}, "", fmt.Errorf("skills: frontmatter: %w", err)
		}
		meta.Name = strings.TrimSpace(fm.Name)
		meta.Description = strings.TrimSpace(fm.Description)
		if fm.DisableModelInvocation != nil {
			meta.DisableModelInvocation = *fm.DisableModelInvocation
		}
		if fm.UserInvocable != nil {
			meta.UserInvocable = *fm.UserInvocable
		}
	}
	return meta, body, nil
}

// ParseFullSkill loads Meta + Skill fields including AllowedTools from raw text.
func ParseFullSkill(raw string) (Skill, error) {
	yamlBlock, body := SplitFrontmatter(raw)
	sk := Skill{
		Meta: Meta{
			UserInvocable: true,
		},
		Body:           body,
		FullText:       raw,
		RawFrontmatter: yamlBlock,
	}
	if strings.TrimSpace(yamlBlock) == "" {
		return sk, nil
	}
	var fm frontmatterYAML
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return Skill{}, fmt.Errorf("skills: frontmatter: %w", err)
	}
	sk.Name = strings.TrimSpace(fm.Name)
	sk.Description = strings.TrimSpace(fm.Description)
	if fm.DisableModelInvocation != nil {
		sk.DisableModelInvocation = *fm.DisableModelInvocation
	}
	if fm.UserInvocable != nil {
		sk.UserInvocable = *fm.UserInvocable
	}
	sk.AllowedTools = []string(fm.AllowedTools)
	// MCPServers: normalize empty/whitespace and rewrite server names to
	// "<skill-name>:<server>" so two skills cannot collide on the MCP host
	// (VAL-CROSS-012). The skill loader validates names before registration.
	for i := range fm.MCPServers {
		spec := fm.MCPServers[i]
		spec.Name = strings.TrimSpace(spec.Name)
		spec.Type = strings.TrimSpace(spec.Type)
		spec.Command = strings.TrimSpace(spec.Command)
		spec.BaseURL = strings.TrimSpace(spec.BaseURL)
		if spec.Name == "" {
			continue // skip malformed entries defensively
		}
		sk.MCPServers = append(sk.MCPServers, spec)
	}
	return sk, nil
}
