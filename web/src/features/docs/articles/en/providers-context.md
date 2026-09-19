---
title: Providers & context
category: providers
summary: Connect Inferenesia API or BYOK, Token Savers on or off, keep chat context manageable
keywords: provider api key byok gateway model token saver context settings
order: 100
---

# Providers & context

Chat needs a **provider** (where models live) and a manageable **context** (what fits in each turn). Configure both in **Settings**, then pick models in chat.

## Two ways to connect models

| Path | What it is | Keys |
|------|------------|------|
| **Inferenesia API** | Hosted gateway for models | `INFERENESIA_API_KEY` (and base URL if you customize it) |
| **BYOK** | Bring your own key; traffic goes **direct** to the provider you configure | Your provider API keys, stored locally |

You can use one or both. BYOK does not send your provider key through the Inferenesia API gateway; it calls the provider base URL you set.

### Inferenesia API (gateway)

1. Open **Settings → Providers** (or equivalent).
2. Set the Inferenesia API base URL if needed (default is the public Inferenesia API endpoint for your install).
3. Paste your **API key**.
4. Save, then open the **model picker** in chat and choose a model.

Env vars (optional, for CLI and advanced setups):

```bash
export INFERENESIA_BASE_URL="https://inferenesia.cloud/v1"
export INFERENESIA_API_KEY="your-key"
# optional default model
export INFERENESIA_MODEL="..."
```

Config home defaults to `~/.inferenesia` (override with `INFERENESIA_HOME`).

### BYOK (your keys)

1. In **Settings**, add a provider profile (name, base URL, API key, models as required).
2. Save. Keys stay on your machine.
3. Pick that profile’s models in the chat model picker.

Never commit keys. Keep `.env` local. If a key leaks, rotate it at the provider.

## Model picker

After providers are configured:

1. Open chat.
2. Use the sticky **model picker**.
3. Select a model for the next turns.

If the list is empty, re-check keys, network, and that the profile is enabled.

## Token Savers

**Token Savers** are optional helpers that reduce how much text gets packed into prompts (shorter tool output, tighter phrasing aids, and similar). They default **off**.

| Behavior | Detail |
|----------|--------|
| Default | Off |
| Missing helper | Pass-through; chat should still work |
| When to enable | Long sessions, large repos, cost or limit pressure |
| When to leave off | You want maximum raw detail in tool results |

Toggle them in **Settings**. Turn one on, try a real task, and compare quality vs length. If answers feel under-informed, turn the saver off again.

## Keeping context manageable

The model only “sees” a limited window. You help by:

| Habit | Why it helps |
|-------|----------------|
| One goal per message | Less noise, clearer tools |
| `@file` on the real targets | Avoids stuffing unrelated files |
| Prefer Plan Mode for big designs | Less thrash writing |
| Start a new session for a new topic | Avoids ancient turns crowding the window |
| Don’t paste entire logs | Paste the failing slice |
| Commit or summarize long threads | Fresh start with a short brief |

If the agent seems to “forget” early instructions, restate the constraint in the latest message. Newest guidance wins in practice.

## Context vs Multi Brain

- **Chat history** — this session’s messages and tool results.
- **Multi Brain notes** — optional project notes the agent can read in a bounded way (see [Agent tools](agent-tools)).

Use Multi Brain for durable project facts. Use chat for the task in front of you.

## Tips

- Use a fast cheap model for refactors you can review quickly; a stronger model for hard design.
- Keep a dedicated BYOK profile per vendor so keys stay organized.
- After changing keys, send a tiny “ping” prompt before a big job.
- CLI users share the same provider config home as the desktop when `INFERENESIA_HOME` matches.

## Troubleshooting

| Symptom | Try |
|---------|-----|
| Auth errors | Re-paste API key; check base URL |
| Empty models | Profile disabled or network blocked |
| Truncated / shallow answers | Disable Token Savers; reduce unrelated attachments |
| Wrong project facts | Confirm workspace; add `@file` or Multi Brain note |
