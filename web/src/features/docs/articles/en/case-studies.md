---
title: Case studies
category: cases
summary: Two practical walkthroughs: first safe edit, and diagram or canvas with the agent
keywords: case study walkthrough safe edit diagram canvas tutorial example
order: 110
---

# Case studies

Two short walkthroughs you can copy. Both assume the desktop app, a folder **workspace**, and a working model in **Settings**.

---

## Case 1: First safe edit

**Goal:** Change a real file with the agent, review it, and keep an easy escape hatch.

### Setup

1. Open **Workspaces** → open your project folder.
2. Confirm Git status is clean or you know what is already dirty.
3. Optional: create a branch for the experiment.
4. Open **Chat** and pick a model you trust for small edits.

### Steps

1. **Mention the file**  
   In the composer, type `@` and select the file you want changed (example: `README.md` or a small UI string file).

2. **Ask for a tiny, testable change**  
   Example prompt:

   > In `@README.md`, add a short “Requirements” bullet that we need Node 20+. Do not edit any other files.

3. **Watch the turn**  
   Expand tool blocks if you want to see the edit. Wait until Generating finishes.

4. **Review**  
   - Open the file in the editor.  
   - Or open **Git** and read the diff.  
   - Or open **Agent changes** on the safety toolbar.

5. **Decide**

   | Outcome | Action |
   |---------|--------|
   | Looks good | Keep it; stage and commit when ready |
   | Mostly good | Hand-edit the rest yourself |
   | Wrong | **Revert agent** (or **Undo**) before more edits pile on |

6. **Commit (optional but healthy)**  

   > Message example: “docs: note Node 20 requirement”

### Why this is “safe”

- Scope was one file and one clear change.
- You reviewed before commit.
- **Revert agent** restores pre-agent dirty content, not only git HEAD.

### Variations

- Turn on **Plan Mode** first and ask for a plan; approve only when the file list is one path.
- Use **Stop** if the agent starts touching unrelated files, then rephrase with a harder scope.

---

## Case 2: Diagram / canvas

**Goal:** Produce a shareable diagram with Mermaid or the Excalidraw canvas.

### Path A — Mermaid in a doc

1. Open or create `docs/overview.md` in the workspace.
2. Prompt:

   > Add a Mermaid flowchart to `docs/overview.md` that shows: User → Inferenesia desktop → Agent → Project files. Keep it to five nodes.

3. Open the markdown **Preview** and check the diagram.
4. Fix labels in the editor or ask: “Rename node Agent to Chat agent.”
5. Save, then commit if the team should see it.

### Path B — Excalidraw canvas

1. Ask:

   > Create `docs/diagrams/request-flow.excalidraw` with boxes for Client, API, and Database, and arrows left to right.

2. Open the new `.excalidraw` file.
3. Use **Canvas** to nudge layout and labels.
4. Optional AI edit:

   > On the open canvas, add a Cache box between API and Database.

5. **Save** when the dirty indicator appears.
6. Review in **Git**, commit with a clear name.

### Review checklist

| Check | OK? |
|-------|-----|
| Filename makes sense for the repo | |
| No secrets in labels | |
| Diagram matches the real system enough to discuss | |
| File saved and, if needed, committed | |

### If AI overdraws

- Delete extra shapes on the canvas manually.
- Or **Revert agent** if the whole file was better before the turn.
- Ask for a smaller edit: “Only add one arrow, do not reshuffle everything.”

---

## Habits that transfer

1. **Small scope** in the first prompt.  
2. **Review** in editor, Git, or Agent changes.  
3. **Revert agent** early when wrong; **commit** when right.  
4. Prefer **Plan Mode** when you cannot list the files yourself yet.  
5. Keep diagrams next to docs the team already reads.
