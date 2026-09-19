---
title: Undo / Revert agent
category: safety
summary: Undo vs Revert agent vs Restore to HEAD on dirty files, with a simple workflow
keywords: undo redo revert agent restore head safety timeline dirty files
order: 30
---

# Undo / Revert agent

When the agent edits files, Inferenesia keeps a safety net so you can get your uncommitted work back. The labels matter: **Undo**, **Revert agent**, and **Restore to HEAD** do different jobs.

## Why this exists

A common failure:

1. You edit `auth.ts` (not committed yet).
2. The agent rewrites `auth.ts` and touches another file.
3. You run a git restore thinking it “undoes the agent.”
4. Your own uncommitted edit is gone too.

Inferenesia’s agent undo path restores the **bytes that were on disk before the agent wrote**, including your dirty edits. That is not the same as going back to the last commit.

## Three actions

| Action | Restores to | Keeps your pre-agent dirty edits? | Uses git? |
|--------|-------------|-----------------------------------|-----------|
| **Undo** | Previous step on the undo stack | Yes (that is the point) | No |
| **Redo** | Re-applies what you just undid | Until you edit outside the stack | No |
| **Revert agent** | Pre-agent content for files in the last agent turn | Yes | No |
| **Restore to HEAD** | Working tree toward the last commit | **No** (can wipe dirty work) | Yes |

**Revert agent** and **Restore to HEAD** stay separate in the UI on purpose.

## Toolbar

```text
[Undo] [Redo] [Revert agent] [Restore to HEAD] [Agent changes]
```

- **Undo / Redo** — step through recent agent (and related) file writes for this workspace.
- **Revert agent** — throw away what the last agent turn did, restore pre-turn dirty files.
- **Restore to HEAD** — git-style reset of tracked files toward HEAD. Use only when you mean it.
- **Agent changes** — timeline of what the agent touched so you can inspect before deciding.

The undo stack is **per workspace**. Switching projects does not mix stacks.

## When to use which

| Situation | Prefer |
|-----------|--------|
| Agent just made a bad multi-file turn | **Revert agent** |
| You only want the previous write step | **Undo** (maybe more than once) |
| You undid too far | **Redo** |
| You want the last commit, discard local dirty | **Restore to HEAD** (confirm first) |
| You want history in git, not just local undo | **Commit** first, then experiment |

## Suggested workflow

1. Keep important WIP either committed on a branch or clearly named.
2. Ask the agent for a change.
3. Review **Agent changes** or the diff in Git / editor.
4. If the turn is wrong: **Revert agent** (or Undo step by step).
5. If the turn is good: keep working, or stage and commit when ready.
6. Only use **Restore to HEAD** when you intentionally want the committed baseline.

## Undo is not git

| Topic | Undo / Revert agent | Git |
|-------|---------------------|-----|
| Baseline | Snapshot before the agent write | Last commit (HEAD) / index |
| Scope | Files the agent (or gateway) wrote | Tracked tree you choose |
| Survives “discard all local” | Yes, until you overwrite the stack | No |
| Good for | Day-to-day agent mistakes | Sharing history, remotes, PR |

You can use both: undo to fix a bad turn, then commit when the tree looks right.

## Tips

- Prefer **Revert agent** right after a bad turn, before you hand-edit the same files a lot.
- After many manual edits, the undo stack may not match your mental model. Check the file contents, not only the button name.
- **Restore to HEAD** is irreversible for uncommitted work. Treat it like a sharp tool.
- CLI users can run undo against a workspace path the same way the desktop stack works (`inferenesia undo` when available for that workspace).

## Common mistakes

| Mistake | Fix |
|---------|-----|
| Using Restore to HEAD to “undo the agent” | Use **Revert agent** or **Undo** |
| Assuming Undo reverts a commit | It does not; use git for commits |
| Expecting one global undo across all projects | Stack is per workspace |
