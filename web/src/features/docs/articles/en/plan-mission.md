---
title: Plan Mode & Mission
category: plan
summary: When to use Plan Mode, approve or reject plans, and Mission with evidence in plain words
keywords: plan mode mission approve reject evidence todos goal safe write
order: 40
---

# Plan Mode & Mission

Two related safety rails help you avoid “agent rewrote half the repo by accident.”

- **Plan Mode** — the agent may read and propose, but file writes are blocked until you leave Plan Mode or approve a plan path your build uses.
- **Mission** — a goal with checkpoints and evidence (tests, logs, checks) so execution is not “trust me, it’s done.”

They are not the same feature. Plan Mode is a write lock + planning stance. Mission is evidence-gated execution.

## Plan Mode: when to use it

Turn **Plan Mode** on when:

- You are unsure which files should change.
- The change touches auth, payments, migrations, or shared APIs.
- You want a step list before any edit.
- You are exploring a new codebase and want analysis without writes.

Leave Plan Mode off for small, well-scoped edits you already trust (“rename this label”, “fix this typo”).

## What happens in Plan Mode

| Behavior | In Plan Mode |
|----------|--------------|
| Read files, search, explain | Allowed |
| Propose a plan / todos | Allowed |
| Mutating tools (write, many shell edits, etc.) | Denied with a clear “plan mode: writes denied” style message |
| Your manual edits in the editor | Still yours; Plan Mode targets the **agent** |

If the agent tries to write while Plan Mode is on, the write does not land. Use that as a feature, not a bug.

## Approve or reject a plan

Depending on the UI chrome for your build:

1. Enable **Plan Mode** from chat / plan controls.
2. Ask for a plan: “Outline how you would add rate limiting to the login route. Do not edit yet.”
3. Read the plan and any todos.
4. **Approve** when you want execution to proceed (exit Plan Mode or follow the approve flow your UI shows).
5. **Reject** or revise: send feedback (“skip the database change, only middleware”) and ask for an updated plan.

Good plan prompts:

- State constraints: “No new dependencies.”
- State out of scope: “Do not touch the billing package.”
- Ask for file list: “List every path you would edit.”

## Mission: evidence in plain words

A **Mission** is a larger goal broken into steps where “done” means more than a confident chat message. Evidence can be:

- Test output that passed
- A command that exited cleanly
- A screenshot or log snippet the agent collected
- A checklist item you confirm in the UI

Think of Mission as: **goal → steps → proof → next step**, not free-form chatting until the model says finished.

### When Mission helps

| Good fit | Weak fit |
|----------|----------|
| Multi-step refactor with tests | One-line copy change |
| “Make CI green” | Pure design opinion |
| Cross-file feature with verification | Brainstorming only (use Session + Plan) |

### Working a mission

1. State the goal and what counts as success (“`npm test` passes”, “login returns 401 without cookie”).
2. Let the agent propose steps and evidence for each.
3. Run or allow the checks; do not skip proof for risky steps.
4. If evidence fails, fix or replan before marking the mission complete.

## Plan vs Mission vs normal chat

| Mode | Writes | Best for |
|------|--------|----------|
| Normal chat | Allowed (with undo safety) | Small tasks, known scope |
| **Plan Mode** | Blocked for agent until you leave / approve | Design first, high risk |
| **Mission** | Allowed under the mission flow | Multi-step work that needs proof |

You can plan first (Plan Mode), then run a Mission once the approach is clear.

## Tips

- Short plans beat 40-step novels. Prefer 3–7 concrete steps.
- Always name the success check in the first message of a mission.
- After approval, watch the first edits closely; **Revert agent** is still available.
- If the agent keeps trying to write in Plan Mode, your prompt may be pushing “just implement it.” Restate: “Stay in plan only.”

## Common mistakes

| Mistake | Better |
|---------|--------|
| Plan Mode on forever for tiny edits | Turn it off for small safe changes |
| Approving without reading file list | Skim paths before approve |
| Mission without a test or command | Define evidence up front |
| Treating Mission as Plan Mode | Plan = no writes; Mission = execute with proof |
