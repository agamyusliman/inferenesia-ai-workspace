---
title: Agent tools
category: agent
summary: Skills, MCP tools, parallel task explore, and Multi Brain notes the agent can read
keywords: agent skills mcp task explore multi brain memory tools parallel
order: 90
---

# Agent tools

Beyond plain chat, the agent can use **tools**: read and edit files, run shell commands, call **skills**, talk to **MCP** servers, explore in parallel **tasks**, and read project **notes** (Multi Brain). You mostly steer this with good prompts and Settings.

## What you see in chat

When the agent uses a tool, the reply shows **tool blocks** (and sometimes **task** blocks). Expand a block to see what ran. You do not need to memorize tool names; read the labels and results.

## Skills

**Skills** are packaged instructions the agent can load on demand (a focused playbook for a job).

Typical flow:

1. You ask for something that matches a skill (“use the release checklist skill”).
2. The agent loads the skill content.
3. It follows that guidance for the rest of the turn or task.

Tips:

- Project or user skills appear when configured for your install.
- Prefer “follow the X skill” over pasting a long checklist every time.
- If a skill is missing, the agent should continue without crashing; install or enable it in your environment if you need it.

## MCP tools

**MCP** (Model Context Protocol) connects external tool servers: issue trackers, docs hosts, internal APIs, and similar.

From your side:

1. Configure MCP servers in **Settings** / config (as supported by your build).
2. Ask the agent to use them in natural language (“list open bugs for this repo”).
3. Review tool output in chat before trusting side effects.

Safety:

- Only connect MCP servers you trust.
- Prefer read-only tools when exploring.
- Do not paste production secrets into MCP configs that get committed.

## Parallel task explore

For broad questions (“where is auth handled?”), the agent may split work into parallel **explore** tasks, then combine results.

What you should know:

| Idea | Meaning for you |
|------|------------------|
| Parallel explore | Several focused searches at once |
| Depth limit | Nested “task of a task” is limited so runs stay bounded |
| Synthesize | Prefer one combined answer after explores, not re-searching the same ground |

You can nudge: “Explore `src/auth` and `web/auth` in parallel, then summarize only.”

If task blocks look noisy, ask for a short synthesis: “Give me the three files that matter and why.”

## Multi Brain (project notes)

If the project has a **Multi Brain** folder (navigable markdown notes for agents), Inferenesia can let the agent read a **bounded** slice of that memory:

- A short master index first
- One or two matching topic lists
- Deeper notes only when the index points there

Think of it as **notes the agent can read**, not a second chat history and not a full dump of every file into the prompt.

### Good Multi Brain hygiene

| Do | Don’t |
|----|-------|
| Keep a short index of topics | Paste entire logs into the index |
| Point to detail files for decisions | Commit secrets into notes |
| Update after meaningful work | Expect the agent to invent history that was never written |

Multi Brain is usually **local** (often gitignored). Product truth for the team still belongs in normal `docs/` when everyone should see it.

## Built-in tool themes (user view)

| Theme | Examples of what the agent might do |
|-------|-------------------------------------|
| Files | Read, search, edit within the workspace |
| Shell | Run project commands |
| Git | Status, diff, stage, commit paths you allow in practice |
| Browser | Open pages, checks, screenshots when enabled |
| Diagrams | Create or update Mermaid / canvas diagrams |
| Memory | Read Multi Brain notes when present |

All of this stays inside the active workspace sandbox for file writes.

## Tips

- Name the tool class in the prompt when you care: “Use browser tools to verify the form.”
- For large codebases, ask for parallel explore + one summary.
- After MCP writes (tickets, comments), verify in the external system.
- Plan Mode still blocks mutating tools until you leave or approve.

## Common mistakes

| Mistake | Better |
|---------|--------|
| Expecting MCP without configuring it | Add servers in Settings / config first |
| Pasting secrets into skill files in git | Keep secrets in local env only |
| Re-asking the same search five times | Ask to synthesize existing task results |
