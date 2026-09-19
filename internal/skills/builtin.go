package skills

import "strings"

// Builtin skill names.
const (
	NameMultiBrain = "multi-brain"
	NameGoCore     = "go-core-worker"
)

// multiBrainBody is the Multi Brain protocol skill (VAL-ORCH-005).
// Read order: session.md → 1–2 buckets → pointed contexts only.
// Write: append one newest-first entry with required timestamp + pointer.
const multiBrainBody = `# Multi Brain protocol

Shared agent memory lives under .multibrain/. Treat it as a navigable index, not a dump.

## Read order (mandatory)

1. Read .multibrain/session.md first (master index only).
2. Open **1–2** bucket files under .multibrain/indexes/*.md that match the task.
3. Open .multibrain/context/*.md **only** when a bucket entry points to them.
4. **Never** load the entire .multibrain/ tree into the prompt.

Use tools:
- multibrain_read — bounded read (session first, at most 2 buckets, pointed contexts only).
- multibrain_append — after meaningful work: one newest-first entry + optional context.

## Write rules (after meaningful work)

- Append **one** entry to the best-matching bucket under .multibrain/indexes/<bucket>.md.
- Write a context file when there is a decision, blocker, important file list, or verification evidence.
- Refresh Last updated (and short scope if needed) on .multibrain/session.md for that bucket; add a bucket line if the area is new.
- Soft cap ~**25** entries per bucket: compact older runs to a context pointer.

## Entry format (indexes)

- Newest-first (newest entry at the top).
- One line per entry: timestamp — agent name: short summary -> .multibrain/context/<file>.md
- Timestamp format: YYYY-MM-DD HH:mm WIB
- Example: 2026-07-17 16:05 WIB — Droid: expand AGENTS Definition of Done -> .multibrain/context/2026-07-17-1605-droid-agents-done-push-rules.md

## Tools

When this skill is loaded, multibrain_read and multibrain_append are available.
All writes go through WriteGateway (undoable). Secrets are scrubbed before write.
`

// goCoreBody is a sample skill that appends a distinctive system prompt
// and grants skill_status so skill load is observable (VAL-ORCH-004).
const goCoreBody = `# Go core worker

Use this skill when implementing Go packages under internal/* and CLI under cmd/inferenesia.

## Procedure

1. Prefer TDD: failing tests for behavioral contracts first.
2. Keep core.Service transport-agnostic; all file mutations through WriteGateway.
3. Run scoped go test ./internal/<pkg>/... then go test ./... and go vet ./....
4. Update product docs for user-facing changes; Multi Brain after meaningful work.
5. Local commit when done; do not push mid-mission unless policy says yes.

## Tool notes

- Prefer read_file / grep / list_dir for exploration.
- Use write_file for source edits (WriteGateway).
- Use shell for go test / go vet / go build.
`

// BuiltinMetas returns discovery-only entries for built-in skills (no full body dump).
// Includes optional 9Router pack as metadata only (VAL-KIT-001).
func BuiltinMetas() []Meta {
	out := []Meta{
		{
			Name:          NameMultiBrain,
			Description:   "Multi Brain protocol: session.md → 1–2 buckets → pointed contexts; newest-first append with timestamp format",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
		{
			Name:          NameGoCore,
			Description:   "Go core worker: TDD, WriteGateway, go test/vet, Multi Brain Definition of Done",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
	}
	out = append(out, ResearchMetas()...)
	out = append(out, NineRouterPackMetas()...)
	return out
}

// LoadBuiltin returns a fully loaded builtin skill by name.
func LoadBuiltin(name string) (*Skill, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case NameMultiBrain:
		return &Skill{
			Meta: Meta{
				Name:          NameMultiBrain,
				Description:   "Multi Brain protocol: session.md → 1–2 buckets → pointed contexts; newest-first append with timestamp format",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			Body: multiBrainBody,
			// Grant multi-brain protocol tools while skill is active (toolset delta).
			AllowedTools: []string{"multibrain_read", "multibrain_append"},
		}, true
	case NameGoCore:
		return &Skill{
			Meta: Meta{
				Name:          NameGoCore,
				Description:   "Go core worker: TDD, WriteGateway, go test/vet, Multi Brain Definition of Done",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			Body: goCoreBody,
			// Observable toolset: skill_status appears only after load.
			AllowedTools: []string{"skill_status"},
		}, true
	default:
		if sk, ok := loadResearchBuiltin(name); ok {
			return sk, true
		}
		if sk, ok := loadNineRouterBuiltin(name); ok {
			return sk, true
		}
		return nil, false
	}
}

// IsBuiltin reports whether name is a builtin skill id.
func IsBuiltin(name string) bool {
	_, ok := LoadBuiltin(name)
	return ok
}
