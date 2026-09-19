# Install and setup

Everything needed to get a working workspace, from a clean machine to your first chat.

If you only want the shortest path, the [README](../README.md) has a five-line version. This document explains what each step does and what to do when something goes wrong.

- [1. What you need](#1-what-you-need)
- [2. Get the code](#2-get-the-code)
- [3. Build](#3-build)
- [4. First run](#4-first-run)
- [5. Choose a workspace](#5-choose-a-workspace)
- [6. Connect a model](#6-connect-a-model)
- [7. Your first chat](#7-your-first-chat)
- [8. Optional: native desktop window](#8-optional-native-desktop-window)
- [9. Optional: MCP servers](#9-optional-mcp-servers)
- [10. Where your data lives](#10-where-your-data-lives)
- [11. Development setup](#11-development-setup)
- [12. Troubleshooting](#12-troubleshooting)

---

## 1. What you need

### Required

| Tool | Version | Check |
|------|---------|-------|
| **Go** | 1.25+ | `go version` |
| **Node.js** | 20+ (22 LTS tested) | `node --version` |
| **npm** | ships with Node | `npm --version` |
| **Git** | any recent | `git --version` |

### Optional

Only needed for the native desktop window instead of the browser UI:

- **Wails v2 CLI**

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```

  Make sure `$(go env GOPATH)/bin` is on your `PATH`.

- **Platform WebView**

  | OS | Requirement |
  |----|-------------|
| macOS | none — uses the system WebKit |
| Windows | [WebView2 runtime](https://developer.microsoft.com/microsoft-edge/webview2/) (preinstalled on Windows 11) |
| Linux | `webkit2gtk-4.1` and `libgtk-3-dev` — e.g. `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev` |

You do **not** need Wails to use the app. The hub serves the same interface at a local URL.

---

## 2. Get the code

```bash
git clone https://github.com/agamyusliman/inferenesia-ai-workspace.git
cd inferenesia-ai-workspace
```

---

## 3. Build

Two builds, in this order. The Go binary embeds the built UI, so the frontend has to exist first.

### 3a. Build the interface

```bash
cd web
npm install
npm run build
cd ..
```

`npm run build` does four things: generates the in-app documentation, type-checks, bundles with Vite, and copies the result into `cmd/desktop/frontend/dist` where Go embeds it.

> **Skipping this step is the most common mistake.** Without it the desktop binary compiles but shows a blank window, because there is no interface to embed.

### 3b. Build the binaries

```bash
go build -o bin/inferenesia         ./cmd/inferenesia
go build -o bin/inferenesia-desktop ./cmd/desktop
```

| Binary | What it is |
|--------|-----------|
| `bin/inferenesia` | CLI — scripting, one-shot prompts, config inspection |
| `bin/inferenesia-desktop` | The app. Opens a native window, or serves the UI with `--serve` |

Confirm both built:

```bash
./bin/inferenesia --help
./bin/inferenesia-desktop --help
```

---

## 4. First run

The fastest way to see it working is the hub — no WebView needed:

```bash
./bin/inferenesia-desktop --serve --port 4110
```

Open **<http://127.0.0.1:4110>**.

Health check, useful for scripts and for confirming it is actually up:

```bash
curl -sf http://127.0.0.1:4110/api/health
# {"ok":true,"product":"Inferenesia",...}
```

The hub binds **loopback only** and expects a port in `4100–4199`. Use `--port` if `4110` is taken.

To stop it, `Ctrl-C` in the terminal running it.

---

## 5. Choose a workspace

A **workspace** is a folder on disk that the agent may read and write. Nothing outside it is touched.

1. Open the app.
2. Create a workspace and point it at a folder — an existing project, or an empty directory you just made for notes.
3. The file explorer, editor, terminal and chat all operate inside that folder.

Use **separate workspaces** for unrelated projects. Sessions belong to a workspace, so this keeps threads from mixing.

---

## 6. Connect a model

The app ships with **no credentials** and contacts nothing until you configure it. There are two routes, and you can use both.

### Route A — an OpenAI-compatible endpoint

Any service that speaks the OpenAI API works. In **Settings → Providers**, add a profile with:

- **Base URL** — the endpoint root, ending in `/v1`
- **API key**
- optionally **Model** — leave it empty and the app lists models from `GET /v1/models`

Or set it in the environment, which takes precedence over `config.yaml`:

```bash
export INFERENESIA_BASE_URL=https://your-endpoint.example/v1
export INFERENESIA_API_KEY=your-key-here
```

### Route B — BYOK, straight to a provider

Store a provider's own key and route directly to it. Keys are written to `~/.inferenesia` with mode `0600` and are never sent anywhere except that provider.

Add one from **Settings → Providers → Add**, or declare it in `~/.inferenesia/config.yaml` and keep the key itself in an environment variable:

```yaml
profiles:
  my-provider:
    type: openai_compatible
    base_url: https://api.example.com/v1
    api_key_env: MY_PROVIDER_API_KEY   # read from the environment, not stored
```

Then:

```bash
export MY_PROVIDER_API_KEY=...
```

Referencing a key by **variable name** rather than pasting it keeps the secret out of your config file.

### Using `.env` instead

```bash
cp .env.example .env
# edit .env
```

`.env` is loaded from the repository root and **never overrides a variable already set in your shell**. It is gitignored. Do not commit it.

### Checking it worked

Open the model picker in chat. Configured profiles appear there, and the endpoint's models are listed if you left the model blank. If a profile shows a key-required state, its credential is missing — chat will say so rather than failing silently.

---

## 7. Your first chat

With a workspace selected and a model configured:

1. Open a session.
2. Ask something that touches a file, e.g. *"summarise what is in this folder"*.
3. Watch the file list — the agent reads through the same gateway that mediates writes.

Then try something that writes, and undo it:

- Ask for an edit.
- Use **Undo**. Your file returns to its exact pre-turn bytes.

That is the core difference from a plain chat window: **Undo restores what your files looked like before the agent ran**, not what git last committed. Uncommitted work you had before the turn survives.

**Plan Mode** is worth trying early: with it on, mutating tools are denied before they execute, so you can discuss a change without any file being touched.

---

## 8. Optional: native desktop window

Once the Wails CLI is installed:

```bash
# Live development — hot reload, opens the app window
wails dev

# Build a distributable application
wails build
```

`wails build` produces a platform bundle under `build/bin/`. It embeds the interface, so build `web/` first or the window will be blank.

---

## 9. Optional: MCP servers

MCP lets the agent use external tools over a standard protocol.

Open **Settings → MCP** and add a server. The panel shows, for each one:

- which **layer** controls it — project, environment, or app config,
- whether it is **managed** elsewhere,

and refuses to let you silently override a server defined by a level you do not control. That is deliberate: a change that appears to save but is overridden by the environment is worse than an error.

Servers defined by the environment cannot be edited from the interface.

---

## 10. Where your data lives

| Path | Contents |
|------|----------|
| `~/.inferenesia/` | Settings and credentials — directory `0700` |
| `~/.inferenesia/config.yaml` | Profiles and preferences — mode `0600` |
| `~/.inferenesia/` (secrets) | Provider keys and tokens — mode `0600` |
| your workspace | Your files, plus its session database |

Override the settings location:

```bash
export INFERENESIA_HOME=/some/other/path
```

Useful for keeping separate profiles, and for testing without touching your real configuration.

To start over, move the directory aside rather than deleting it:

```bash
mv ~/.inferenesia ~/.inferenesia.backup
```

If you use full-disk backups, note that provider keys live in that directory in plaintext with owner-only permissions.

---

## 11. Development setup

```bash
./scripts/dev.sh          # hub on :4110 + Vite hot reload on :4150
./scripts/dev.sh hub      # hub only, serving the built UI
./scripts/dev.sh status   # which ports are up and healthy
./scripts/dev.sh stop
./scripts/dev.sh restart
```

Open **<http://127.0.0.1:4150>** when using `dev.sh` — that is Vite, with hot reload, proxying API calls to the hub.

### Checks before committing

```bash
go vet ./...
go test ./...
(cd web && npm run typecheck)
(cd web && npm run test:unit)
(cd web && npm run build)
```

`docs/` holds the install guide (this file). Architecture and contributor notes are in [`AGENTS.md`](../AGENTS.md).

### Ports

Ports `4100–4199` are reserved for this project so it never collides with your other local services. `4110` is the hub, `4150` Vite, `4151` the preview server.

### Isolating a test instance

```bash
INFERENESIA_HOME=/tmp/inferenesia-scratch \
  ./bin/inferenesia-desktop --serve --port 4120
```

Your real settings are untouched.

---

## 12. Troubleshooting

### Blank window, or the UI does not load

The frontend was not built, or was built after the Go binary.

```bash
(cd web && npm install && npm run build)
go build -o bin/inferenesia-desktop ./cmd/desktop
```

The Go binary embeds `cmd/desktop/frontend/dist`, which `npm run build` produces.

### `port already in use`

```bash
./bin/inferenesia-desktop --serve --port 4120
```

Ports must stay within `4100–4199`.

### Chat says no model, or the profile is key-required

The profile exists but has no usable credential. Check, in this order:

1. Is the API key set — in Settings, or in the environment variable the profile names?
2. Is `INFERENESIA_BASE_URL` reachable from this machine?

```bash
curl -sf "$INFERENESIA_BASE_URL/models" -H "Authorization: Bearer $INFERENESIA_API_KEY"
```

A shell variable set in one terminal is not visible to an app launched from another.

### `go build` fails on the embedded frontend

Same root cause as the blank window: build `web/` first.

### Wails not found

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
export PATH="$(go env GOPATH)/bin:$PATH"
```

Wails is only needed for `wails dev` and `wails build`. The hub works without it.

### Linux: WebKit errors when building

```bash
# Debian / Ubuntu
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
```

### Node version errors

```bash
node --version   # must be 20 or newer
```

If several Node versions are installed, confirm which one `npm` uses — a version manager shim on `PATH` is the usual culprit.

### Windows: build tools missing

Wails needs a C compiler for the native window:

```powershell
# winget
winget install -e --id GnuWin32.Make
# or use MSYS2 / the mingw toolchain
```

Again, the hub path needs none of this.

### Agent cannot see a file

Files outside the selected workspace are not visible by design. Point the workspace at a parent folder, or move the file inside.

### A write did not happen

Check whether **Plan Mode** is on — it denies mutating tools before they run.

---

## Getting help

Open an issue at <https://github.com/agamyusliman/inferenesia-ai-workspace/issues> and include:

- your OS and `go version` / `node --version`,
- the exact command you ran,
- what happened, and what you expected instead.

**Never paste API keys, tokens, or the contents of `~/.inferenesia` into an issue.**
