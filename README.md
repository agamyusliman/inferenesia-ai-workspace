# Inferenesia AI Workspace

A local-first AI workspace for your own files, notes, diagrams and documents — a desktop app plus a matching CLI, driven by any OpenAI-compatible model endpoint or your own provider keys.

You bring the workspace. You bring the model. Everything else runs on your machine.

---

## Why this exists

Most AI coding tools assume a single repository and a single chat thread. Real work is messier: you keep a folder of half-written specs, a spreadsheet of numbers, a diagram nobody remembers drawing, and several provider keys you would rather not paste into a hosted service.

Inferenesia treats **a workspace folder** as the unit of work. Inside it you get:

- a chat that can read and edit files through a single audited write path,
- a terminal that shares the same working directory,
- a Playground for the visual artefacts that do not belong in a code editor,
- and an undo history that restores *what your files looked like before the agent touched them*.

Nothing leaves your machine unless you point it at a model endpoint.

---

## What you get

### Chat with real file access

The agent reads and writes inside your workspace through one gateway that snapshots every mutation. That means **Undo** returns your files to their pre-turn bytes — it is not `git restore`, and it will not throw away work you had not committed. **Revert agent** and **Restore to HEAD** stay separate operations on purpose.

Plan Mode stops mutating tools before they run, so you can think out loud without the agent editing anything.

### A Playground that is not a toy

Eight kinds of canvas, each with its own editor:

| Kind | What it does |
|------|--------------|
| **Markdown doc** | Document with live outline, word and heading stats, source and preview |
| **Table** | Spreadsheet grid — see below |
| **Timeline** | Milestones with dates and status, list and schedule views |
| **Diagram** | Mermaid: flow, sequence, ERD; import `.mmd`, export source, SVG, PNG, JPG |
| **Excalidraw** | Freeform whiteboard; import and export `.excalidraw`, PNG, SVG |
| **Image Studio** | Generate and edit with reference images; import local files |
| **HTML page** | Live web preview the agent can edit |
| **Slides** | Presentation deck with measured export |

### The Table canvas is a spreadsheet

Not a data grid — a spreadsheet:

- **Formulas.** A cell beginning with `=` is live: arithmetic, comparisons, `SUM`, `AVERAGE`, `MIN`, `MAX`, `COUNT`, `IF`, `IFERROR`, `ROUND`, text functions, ranges like `A2:A9`, `%` and `&`. Formulas are evaluated locally by a hand-written parser, **never by `eval`**, so a sheet cannot execute JavaScript. Self-referencing cells report `#CYCLE!` instead of hanging.
- **Multiple sheets** per document, with rename, reorder and delete.
- **Formatting** that survives the round trip: bold, italic, alignment, fill and text colour, number formats.
- **Real structure:** drag rows and columns to reorder, hide and reveal, resize by dragging, merge and unmerge across a selection.
- **Real Excel export.** `.xlsx` files written as genuine OOXML with live formulas, cell styles, column widths, row heights, hidden rows and columns, and merge ranges.

Editing never strands your formatting: move, insert or delete a row and the styles, sizes, merges *and formula references* that pointed at it follow along.

### Slides that export without moving

Decks are authored on a fixed 1280×720 artboard. Export keeps that geometry:

- **Visual** (recommended) — one image per slide. Cannot reflow, so it always matches the preview.
- **Editable** — ornaments as a background plate, text as real PowerPoint objects you can retype.
- **PDF** — printed at the authored artboard, not squeezed onto A4.

### Everything else in the box

- **Terminal** built in, sharing the workspace directory.
- **Workspaces and sessions** — keep separate projects, resume old threads.
- **Command palette**, split editor, file explorer with context actions.
- **EN / ID interface**, light, dark and warm themes.
- **Memory as markdown.** Notes live in plain files you can read, diff and delete. No vector database is required to use the app.

---

## Requirements

| | |
|---|---|
| **Go** | 1.25 or newer |
| **Node.js** | 20 or newer (22 LTS tested) |
| **OS** | macOS, Linux, or Windows |

Optional, only if you want the native desktop window rather than the browser UI:

- **Wails v2** CLI — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- WebView2 (Windows), `webkit2gtk` (Linux)

A model endpoint is required to use chat: either an OpenAI-compatible base URL plus a key, or your own provider key. **The app ships with no credentials and calls nothing until you configure it.**

---

## Install and setup

A full walkthrough — build, first run, model configuration, BYOK, MCP and troubleshooting — is in **[docs/INSTALL.md](./docs/INSTALL.md)**.

The short version:

```bash
git clone https://github.com/agamyusliman/inferenesia-ai-workspace.git
cd inferenesia-ai-workspace

# 1. Build the browser UI and the Go binaries
(cd web && npm install && npm run build)
go build -o bin/inferenesia         ./cmd/inferenesia
go build -o bin/inferenesia-desktop ./cmd/desktop

# 2. Run the local hub (serves the UI on http://127.0.0.1:4110)
./bin/inferenesia-desktop --serve --port 4110
```

Then open <http://127.0.0.1:4110>, add a model endpoint in **Settings**, and point the workspace at a folder.

One-command development loop instead:

```bash
./scripts/dev.sh          # hub on :4110, Vite with hot reload on :4150
./scripts/dev.sh status   # what is running
./scripts/dev.sh stop
```

---

## Configuration

Settings live in **`~/.inferenesia`** (directory mode `0700`; `config.yaml` and secrets `0600`). Override the location with `INFERENESIA_HOME`.

Precedence is **environment > `config.yaml` > built-in defaults**.

### Model access

Two ways, and you can use both at once:

1. **An OpenAI-compatible endpoint** — set a base URL and API key; the app discovers models from `GET /v1/models`.
2. **BYOK** — store provider keys locally (`0600`) and route straight to that provider. Keys never leave your machine.

```bash
# .env, or export in your shell. .env is gitignored — never commit it.
INFERENESIA_BASE_URL=https://your-endpoint.example/v1
INFERENESIA_API_KEY=...
# INFERENESIA_MODEL=...        # optional; discovered when unset
```

See `.env.example` for every name the app reads.

### MCP servers

Add MCP servers from **Settings → MCP**. The panel shows which layer controls each server (project, environment, or app config) and refuses to let you silently override a read-only one.

### Token savers are off by default

Compression helpers stay **off** until you enable them. If a helper's tool is missing, requests pass through untouched rather than failing your chat.

---

## Safety

- **One write path.** Agent and user file mutations go through a single gateway that snapshots before changing anything.
- **Undo is not git.** Undo, Redo and Revert restore pre-agent bytes; they do not discard uncommitted work.
- **Plan Mode denies writes** before the handler runs.
- **Secrets are redacted** before reaching logs, and a pre-commit guard refuses to stage credentials.
- **Loopback only.** The hub binds `127.0.0.1` on ports `4100–4199`.
- **No telemetry.** The app contacts only the endpoints you configure.

---

## Development

```bash
go vet ./...
go test ./...
(cd web && npm run typecheck)
(cd web && npm run test:unit)
(cd web && npm run build)
```

[`AGENTS.md`](./AGENTS.md) documents the architecture, invariants and conventions in detail. It is written for both human contributors and AI agents working in this repository.

---

## Project layout

```
cmd/inferenesia/     CLI entry point
cmd/desktop/         Desktop app + local hub, embeds the built UI
internal/core/       Service, agent loop, HTTP/SSE hub
internal/provider/   Model routing and provider profiles
internal/writegate/  The single file-mutation path, with undo/redo
internal/tools/      Agent tools (filesystem, shell, git, browser, diagram, …)
internal/mcp/        MCP client and persistent host
web/                 React + Vite + TypeScript + Tailwind UI
docs/                Install guide and contributor notes
```

---

## Contributing

Issues and pull requests are welcome. Run the checks under **Development** before opening a PR, and keep each commit to one area.

---

## License

[MIT](./LICENSE) — see the license file for the full text.
