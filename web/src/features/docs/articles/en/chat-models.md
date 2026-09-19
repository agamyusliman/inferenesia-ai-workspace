---
title: Chat & models
category: chat
summary: Send messages, stop runs, @file mentions, attachments, model picker, Live Blocks
keywords: chat model stop esc mention attachment live blocks stream prompt picker
order: 20
---

# Chat & models

Chat is the main way you work with the agent: describe a change, attach context, pick a model, and watch the reply stream in.

## Layout

| Part | Role |
|------|------|
| Message list | Your prompts and the agent’s replies, plus tool and task blocks |
| Composer | Text, attachments, @ mentions, Send |
| Model picker | Choose the model for this conversation without leaving chat |
| Plan / todos chrome | Plan Mode badge, proposal dialogs, todo sidebar when active |
| Generating indicator | Shows while a reply is still open |

Show or hide chat with the **Chat** toggle in the header. Pair it with the active **workspace** or **session** so history matches what you expect.

## Send a message

1. Focus the composer.
2. Type your prompt. **Enter** sends; **Shift+Enter** inserts a new line.
3. Optionally attach files or add `@file` mentions (below).
4. Press Send and watch the reply stream.

An empty chat offers starter prompts for the current context: project exploration/review in a workspace, planning/debugging in a session. A starter only fills an empty composer and focuses it; edit the draft and press Send when ready. It does not send a request or replace an existing draft.

While the agent is working you may see tool calls (read file, run command, edit, and so on) inline with the answer.

## Stop a run

| Control | Effect |
|---------|--------|
| **Stop** | Ends the active stream / turn from the UI |
| **Esc** | Cancels streaming when the chat panel allows it; also closes many overlays |

The composer is read-only during a stream so **Esc** is not eaten by typing. Switching workspace mid-stream also cancels the client stream; the turn stays associated with the workspace where it started.

## @file mentions

Type `@` in the composer and pick a path from the project. Mentions pin that file into the turn so the agent is more likely to read the right context without you pasting the whole file.

Tips:

- Prefer one or two focused files over dumping the whole tree.
- Mention the file you want changed, not every related module.
- Large generated folders are usually noise; skip them unless the task needs them.

## Attachments

You can attach images, PDFs, and text-like files from the composer. Use attachments when:

- The bug is visual (screenshot).
- You have a short log or spec that is not already in the repo.
- You want the model to see a document that is not under the workspace root.

Keep attachments small and relevant. Huge binaries waste context and slow the turn.

## Pick a model

Use the sticky **model picker** above or beside the composer:

1. Open the picker.
2. Choose a model from your connected providers (Inferenesia API and/or BYOK profiles in **Settings**).
3. Send the next message on that model.

Model choice applies to new turns. If a model is missing, check **Settings → Providers** for keys and base URL.

## Live Blocks

**Live Blocks** can render safe HTML previews of certain assistant content in a sandboxed iframe. Default is **off**.

| Setting | Where |
|---------|--------|
| Live Blocks on/off | **Settings** (workspace or app prefs, depending on build) |

Turn it on only when you want rich previews. Leave it off if you prefer plain markdown only.

## Web Preview (full panel)

For landing pages and local servers, open **Preview** on the activity rail — a **full main panel** (not a chat split):

| Mode | Use |
|------|-----|
| **URL** | Live server (`127.0.0.1:8080`, Vite, etc.) |
| **File** | Load HTML from the active session or workspace |
| **HTML** | Paste/edit HTML and Apply for instant iframe refresh |

Device frames: **Mobile**, **Tablet**, **Laptop**, **Desktop**. Switch devices anytime; the frame scales to fit the stage.

This is separate from Excalidraw **canvas** (diagrams) and from in-chat Live Blocks.

## Reading the reply

A typical assistant turn can include:

- Streamed text (the main answer)
- Tool blocks (what the agent ran or edited)
- Task blocks (parallel explore subtasks when used)
- Short thinking or meta notes when the model provides them

You do not need to open every tool block. Expand the ones that matter when something looks wrong.

## Tips

- One clear goal per message beats a long laundry list.
- Name the file or feature: “In `src/auth.ts`, fix the null check on `user`.”
- Use **Stop** early if the agent goes off track, then rephrase.
- Switch model for hard reasoning vs fast cheap edits when your providers allow it.
- Plan Mode (see [Plan Mode & Mission](plan-mission)) is better than hoping the agent “just looks” before writing.

## Troubleshooting

| Symptom | Try this |
|---------|----------|
| Empty or stuck “Generating” | Stop, check network and API key in Settings, resend |
| Model list empty | Configure Inferenesia API or BYOK under **Settings → Providers** |
| Agent ignores a file | Add `@file` or open the file and mention the path in text |
| Wrong project context | Confirm the active **workspace** in the Workspaces rail |
