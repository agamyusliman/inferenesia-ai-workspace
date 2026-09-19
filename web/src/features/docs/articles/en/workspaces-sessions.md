---
title: Workspaces & sessions
category: workspace
summary: Open and switch project folders vs chat-only Sessions, and what changes in the UI
keywords: workspace session open switch remove explorer git chat terminal folder
order: 10
---

# Workspaces & sessions

Inferenesia treats a **workspace** (your project folder) and a **session** (a chat-only thread) as two different things. Mixing them up is the usual reason a chat “disappeared.”

## Quick map

| Surface | What it is | You get |
|---------|------------|---------|
| **Workspaces** | A real folder on disk | Explorer, editor, Git panel, terminal, agent file tools, chat for that project |
| **Sessions** | A chat thread with no project root | Chat only (no tree, no Git panel) |

Clicking a project is not the same as switching a chat tab. Each item keeps its own chat history.

## Open a workspace

1. Open the activity rail → **Workspaces**.
2. Click **Open workspace…**
3. Pick a recent folder, choose **Browse folder…**, or enter a folder path.
4. Inferenesia registers that folder, makes it active, and loads the explorer plus chat for it.

The dialog focuses **Browse folder…** when opened. Tab and Shift+Tab move through its controls; Esc closes it and returns focus to the opening button.

Opening the same path twice reuses the existing entry. If the folder was moved or deleted, use **Relink…** to point at the new location.

### From the CLI

```bash
inferenesia open /path/to/project
```

That registers the same kind of workspace the desktop uses.

## Switch workspace

| Action | What happens |
|--------|----------------|
| Click another workspace in the list | Active root changes; explorer, Git, and terminal follow; chat history switches to that workspace |
| Switch while a reply is still streaming | The in-progress stream stops; the finished reply stays on the workspace where it started |

You can keep several projects registered and jump between them without closing Inferenesia.

## Remove a workspace

Removing only unregisters it from Inferenesia. Your files on disk stay put. Chat history for that workspace id is no longer shown in the rail (it is not a git delete).

## What changes when a folder is active

| Area | Behavior |
|------|----------|
| **Explorer** | Tree rooted at the project folder (common noise like `node_modules` stays hidden) |
| **Editor** | Tabs open files under that root; Save writes into the project |
| **Git** | Status, stage, commit, fetch, pull, push in the **Git** panel (not the shell footer) |
| **Chat** | History and agent tools are scoped to this workspace |
| **Terminal** | Default working directory is the workspace root |
| **Undo / Revert agent** | Stack is **per workspace**, not shared across projects |
| **Status bar** | Product + context label (`workspace` name, `session: …`, or `playground: …`) + plan/theme — **no** git branch strip |

The agent cannot write outside the registered root. That keeps edits inside the project you opened.

## Sessions (chat-only)

Use **Sessions** when you want a thread that is not tied to a repo: brainstorming, API design notes, general Q&A.

1. Activity rail → **Sessions**.
2. Create or pick a session.
3. Chat as usual. There is no explorer or Git for that item.

You can still mention ideas and paste snippets. For real file edits, open a **workspace** instead.

## Tips

- Name workspaces by project, not by task. Use chat titles or sessions for temporary threads.
- Prefer one workspace per git root. Nested repos show as chips in Git when needed.
- Config and the workspace list live under `~/.inferenesia` (or `INFERENESIA_HOME` if you set it). That is separate from your project folder.
- If chat looks empty after a switch, check whether you are on **Workspaces** or **Sessions**. History does not merge across the two rails.

## Common mistakes

| Mistake | Better approach |
|---------|-----------------|
| Expecting Git on a Session | Open a folder workspace |
| Looking for last night’s chat on the wrong project | Switch to the workspace you used then |
| Deleting a workspace to “clean disk” | Remove only unregisters; delete files in your OS file manager if you truly want them gone |
