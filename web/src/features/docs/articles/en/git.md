---
title: Git
category: git
summary: Stage, commit, push, fetch, Actions, workflow rules, secrets, and how git differs from undo
keywords: git stage commit push pull fetch branch remote actions secrets undo
order: 50
---

# Git

The **Git** panel works on the active **workspace** folder. Use it for normal day-to-day version control: see status, stage, commit, fetch, pull, push, and peek at GitHub Actions when the repo is connected.

## Open Git

1. Open a folder **workspace** (Sessions have no Git strip).
2. Open the **Git** view from the activity rail or layout chrome.
3. Confirm the path matches the repo you expect (nested repos may show as chips).

## Everyday actions

| Action | What it does |
|--------|----------------|
| **Status / diff** | See changed files and line-level diff |
| **Stage** | Add selected files (or hunks, when offered) to the index |
| **Unstage** | Remove files from the index without deleting working tree edits |
| **Commit** | Create a commit with your message on the current branch |
| **Fetch** | Update remote refs without merging |
| **Pull** | Bring remote commits into your branch (per your remote setup) |
| **Push** | Publish local commits to the remote |

Typical loop:

1. Make or accept changes (you or the agent).
2. Review the diff.
3. Stage what belongs in this commit.
4. Write a clear message (why, not only what).
5. Commit.
6. Push when you want the remote updated.

## Branches and remotes

- Check the current **branch** name in the Git header before committing.
- Prefer a feature branch for agent-heavy experiments.
- **Fetch** before large merges so you know what moved on the remote.
- If push is rejected, pull/rebase per your team rules, then push again. Inferenesia does not invent a force-push for you.

## GitHub Actions

If the repository is on GitHub and credentials allow it, the **Actions** area can show recent workflow runs for the repo. Use it to:

- See whether CI failed after a push
- Open a failed run for logs (in the browser or linked UI)
- Avoid guessing “is main green?”

Not every private setup will list Actions; missing data usually means auth or remote visibility, not a broken Git status.

## Workflow rules file

Many teams keep agent or contributor rules in the repo (`AGENTS.md`, `CONTRIBUTING.md`, or similar). The agent can read those when the workspace is open. Put durable project rules there so every session sees them:

- How to run tests
- Branch naming
- “Do not touch generated/”
- Review expectations

That is separate from your personal Multi Brain notes under the project (see [Agent tools](agent-tools)).

## Secrets and safety

| Do | Don’t |
|----|-------|
| Keep `.env` and keys out of commits | Commit API keys, tokens, or private URLs with credentials |
| Rely on `.gitignore` for env and local config | Force-add ignored secret files |
| Rotate a key if it ever landed in git history | Paste production secrets into chat casually |

If the agent stages a suspicious file, **unstage** it and fix `.gitignore` before commit.

## Git vs Undo / Revert agent

| Need | Use |
|------|-----|
| Undo the last agent file writes, keep dirty WIP | **Undo** or **Revert agent** |
| Save a checkpoint you can push / PR | **Commit** |
| Throw away local tracked changes toward last commit | **Restore to HEAD** (destructive for dirty work) |
| Share history with the team | **Push** / pull request |

See [Undo / Revert agent](undo-revert) for the full comparison. Rule of thumb: fix bad agent turns with **Revert agent** first; use git when you care about history and remotes.

## Tips

- Commit before a large agent refactor so **Restore to HEAD** has a clean baseline.
- Prefer small commits after good agent turns over one giant “wip agent” blob.
- Read the diff even when the chat summary sounds perfect.
- CLI: `inferenesia` can drive related git-capable workflows from the terminal when you prefer not to use the panel.

## Common mistakes

| Mistake | Better |
|---------|--------|
| Commit without reviewing agent diffs | Open Git diff first |
| Using Restore to HEAD as undo | Use **Revert agent** |
| Pushing secrets | Unstage, gitignore, rotate keys |
| Working on the wrong branch | Check branch name before commit |
