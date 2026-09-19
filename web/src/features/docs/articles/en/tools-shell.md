---
title: Tools & shell
category: tools
summary: Terminal tabs and splits, snippets, browser tools overview, keyboard shortcuts
keywords: terminal shell tab split snippet browser keyboard shortcut tools
order: 70
---

# Tools & shell

Inferenesia pairs the editor and chat with an integrated **terminal** and optional **browser** tools the agent can drive. This guide is about using those surfaces day to day.

## Terminal basics

Open the **Terminal** panel from the layout chrome (bottom or docked region, depending on your layout).

| Feature | What you get |
|---------|----------------|
| Multi-tab | Several shells in one workspace |
| Split | Side-by-side terminals |
| Default cwd | Workspace root when a folder is active |
| Open from explorer | **Open in Integrated Terminal** on a folder |

Each tab is its own shell session. Closing a tab ends that session; other tabs keep running.

## Multi-tab and split

1. Open Terminal.
2. Use **New tab** for a second shell (tests in one, server in another).
3. Use **Split** when you want two panes visible at once (logs + commands).
4. Click a pane to focus it before typing.

Tips:

- Keep long-running servers in a dedicated tab so you do not kill them by accident.
- Name or place tabs consistently (left = app, right = tests) so muscle memory sticks.

## Snippets

Global **snippets** store short commands you run often:

- `npm run test:unit`
- `git status -sb`
- Project-specific build lines

Create or insert snippets from the terminal chrome when available. Snippets are for you; the agent still has its own shell tools when you ask in chat.

## Browser tools (overview)

The agent can use browser-related tools when your setup allows it (for example open a page, inspect, take a screenshot, or run checks). From your side:

1. Prefer asking in chat: “Open the local app and check the login form.”
2. Watch tool blocks in the reply for what ran.
3. Keep sensitive cookies out of prompts; results are sanitized where possible, but do not paste session tokens casually.

You do not need to operate a separate browser IDE for most tasks. When a human browser is easier, use your normal browser and paste a screenshot into chat as an attachment.

## Agent shell vs your terminal

| Surface | Who drives it | Good for |
|---------|---------------|----------|
| Integrated terminal | You | Servers, interactive CLIs, watching logs |
| Agent shell tools | Agent (from chat) | Scripted checks, short commands, install steps you approved in spirit |

If a command is destructive (`rm -rf`, mass migrate), prefer running it yourself in the terminal after reading the plan.

## Keyboard shortcuts (common)

Exact bindings can vary slightly by OS and build. These are the usual patterns:

| Action | Shortcut (typical) |
|--------|--------------------|
| Send chat message | **Enter** |
| New line in composer | **Shift+Enter** |
| Stop generation | **Stop** button or **Esc** (when chat focused) |
| Command palette | **Cmd/Ctrl+Shift+P** (when available) |
| Save file | **Cmd/Ctrl+S** |
| Focus terminal | Click terminal or layout shortcut if configured |
| Toggle chat | Header **Chat** control |

If a shortcut does nothing, click the target panel first so focus is correct.

## Tips

- Start the dev server in a terminal tab, then ask the agent to edit code while you watch logs.
- Prefer project scripts (`npm test`) over ad-hoc one-liners the agent invents, when scripts already exist.
- After the agent runs a command that fails, paste the error or let it read the terminal output in a follow-up.
- Keep secrets in env files loaded by your app, not hard-coded in snippet text you might share.

## Common mistakes

| Mistake | Better |
|---------|--------|
| Running prod deploys only via agent chat | Use your own terminal + review |
| One terminal for everything | Split tabs: server / test / misc |
| Ignoring failed tool commands in chat | Expand the tool block and fix the root error |
