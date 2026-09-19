# Documentation

Public documentation for **Inferenesia AI Workspace**.

## Contents

| Document | What it covers |
|----------|----------------|
| [INSTALL.md](./INSTALL.md) | Full install and setup guide — build, first run, workspaces, model configuration, BYOK, MCP, troubleshooting |
| [../README.md](../README.md) | Project overview, features, quick start |
| [../AGENTS.md](../AGENTS.md) | Architecture, invariants and conventions — for contributors and AI agents working in this repo |

## What this project is

A local-first AI workspace: a desktop app and CLI that operate on **a folder**, rather than a single repository or a single chat thread.

- **Bring your own model.** Any OpenAI-compatible endpoint, or your own provider keys (BYOK). The app ships with no credentials and contacts nothing until you configure it.
- **One write path.** Every file mutation goes through a gateway that snapshots first, so undo restores pre-agent bytes rather than a git revision.
- **Nothing is locked in.** Notes are plain markdown, settings are a readable config file, and no vector database is required to use the app.

## Notes on this folder

This `docs/` directory deliberately tracks only what helps someone who has just cloned the repository — this index and the install guide.

Maintainer design notes, roadmap drafts and brand material are kept outside version control. They are working documents rather than documentation, and they reference private infrastructure.

## Getting started

Start with [INSTALL.md](./INSTALL.md). For the shortest path, the [README](../README.md) has a five-line version.
