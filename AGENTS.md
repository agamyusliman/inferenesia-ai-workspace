# Agent instructions — Inferenesia App

## 1. Multi Brain (MANDATORY, **local only**)

Shared agent memory lives under **`.multibrain/`** on disk. It is **gitignored** — never commit or push Multi Brain files (local paths, session notes, agent handoffs).

Treat it as a navigable index, not a dump.

### Read order

1. Read `.multibrain/session.md` first (master index only).
2. Open **1–2** matching buckets under `.multibrain/indexes/*.md`.
3. Open `.multibrain/context/*.md` **only** when a bucket points to detail you need.
4. Do **not** load the whole `.multibrain/` tree into the prompt.

### Write rules (after meaningful work)

- Append one entry to the best bucket in `.multibrain/indexes/<bucket>.md`.
- Write a context file when there is a decision, blocker, important file list, or verification evidence.
- Refresh `Last updated` (and short scope if needed) in `.multibrain/session.md` for that bucket; add a new bucket if the area is new.
- Soft cap ~**25 entries** per bucket: summarize old rows into context/memory, replace the old run with one summary + pointer.
- **Do not** `git add` `.multibrain/` — Multi Brain is laptop-local agent memory.
- **Product design docs** live under **`docs/`** (also **gitignored** for public GitHub). Keep them on disk; do not stage for public push. Public-facing copy for GitHub visitors is root **`README.md` only**.

### Entry format (indexes)

- Newest-first.
- One line: `timestamp — agent name: short summary -> .multibrain/context/<file>.md`
- Example: `2026-07-19 15:32 WIB — OpenCode: refresh AGENTS.md for shipped monorepo commands -> .multibrain/context/2026-07-19-1532-opencode-agents-md-refresh.md`

---

## 2. Product map (do not re-discover)

| Fact | Detail |
|------|--------|
| Ecosystem | **Two products only:** **Inferenesia App** (this repo: desktop + CLI) + **Inferenesia Cloud** (account, prepaid/QRIS, catalog, OpenAI-compatible API). API is a **Cloud surface**, not a third product. Spec: `docs/brand/ecosystem-naming.md` (local). |
| Product (this repo) | **Inferenesia App**. Provider/gateway label may still appear as legacy `temp-ai` / alias `tempai` → profile `inferenesia` — never market as product name (`internal/brand`). |
| Module | `github.com/agamyusliman/inferenesia-app` · Go **1.25+** |
| Binaries | `cmd/inferenesia` → CLI; `cmd/desktop` → Wails + hub HTTP |
| Shared core | CLI and desktop both use **`core.NewService` / `core.Run`**. There is **no** LLM client in React/`web/`. |
| Config home | `~/.inferenesia` (legacy `~/.yura-ai`; dir **0700**); override with **`INFERENESIA_HOME`** (legacy `YURA_AI_HOME`). `config.yaml` / secrets **0600**. Hierarchy: **env > config.yaml > defaults**. |
| Models | **Inferenesia Cloud** path (`INFERENESIA_BASE_URL` / `INFERENESIA_API_KEY`, default base **`https://inferenesia.cloud/v1`**; legacy `https://ai.temp.web.id/v1` still readable via `TEMP_AI_*` dual-read) **and/or BYOK** (keys local; direct to provider). |
| Store | SQLite via **`modernc.org/sqlite`** (pure Go — prefer **no CGO**). |
| Ports | Hub, Vite, browser CDP, MCP fixtures: **4100–4199 only**. Defaults: hub **`4110`**, Vite **`4150`**, preview **`4151`**. |
| Memory | Multi Brain markdown default. **Vector RAG is not wired in-host**; MCP `type: rag` is rejected. |
| Docs | **`docs/` is local-only** (gitignored for public GitHub). Design/roadmap/brand deep-dives stay on disk. **Public GitHub:** root `README.md` only (end-user). Agents still read local `docs/` when present. Locked choices: `docs/decisions.md` / brand files — **do not invent** product decisions. |
| External ref (outside repo) | Maintainer's local gateway notes, outside this tree. Read-only context; **never copy secrets** into this repo. |

### Layout agents edit

```
cmd/inferenesia/       # thin CLI entry → internal/cli (cobra)
cmd/desktop/         # Wails + --serve; embeds frontend/dist
internal/core/       # Service, agent loop, hub HTTP/SSE, desk APIs
internal/provider/   # ModelRouter (openai_compatible + BYOK profiles)
internal/writegate/  # ALL workspace file mutations + undo/redo/revert
internal/tools/      # agent tools (fs, shell, git, browser, task, diagram, …)
internal/{plan,mission,orchestrator,mcp,browser,pty,skills,memory,store,git,config,brand,secrethyg,workspace,cli}/
web/                 # React/Vite/TS/Tailwind SPA (features/*)
docs/                # LOCAL ONLY (gitignored) — design/brand/roadmap; not for public GitHub
README.md            # PUBLIC end-user readme (no internal design dump)
```

### Hard invariants (easy to break)

1. **Single write path:** agent/user FS mutations go through **WriteGateway** (`internal/writegate`). Do not bypass with raw `os.WriteFile` into a workspace when an agent/tool path exists.
2. **Undo ≠ git HEAD:** Undo/Redo/Revert restore **pre-agent dirty** bytes (snapshot before the agent turn), **not** `git restore` to HEAD. UI labels must keep **Revert agent** vs **Restore to HEAD** distinct.
3. **Plan Mode:** while active, mutating tools are denied with `plan mode: writes denied` before handlers run. Plan ≠ Mission (mission = evidence-gated execution).
4. **Orchestrator:** `task` depth default **1**; nested `task` denied. Prefer `task(synthesize=true)` after parallel explores — do not re-search.
5. **Frontend embed:** `web` build copies to **`cmd/desktop/frontend/dist`** (`web/scripts/copy-dist.mjs`). Desktop `//go:embed all:frontend/dist`. Missing dist → blank window / serve without SPA. Prefer building web before desktop binary when UI changes.
6. **Brand strings:** user-facing products are **Inferenesia App** + **Inferenesia Cloud** only; reserve legacy **temp-ai** for provider/gateway alias only (`tempai` → `inferenesia`). Legacy env `TEMP_AI_*` / `YURA_AI_*` must stay readable via dual-read.
7. **Wails bindings:** `web/wailsjs/` is generated runtime glue — do not hand-edit as product logic.

---

## 3. Commands (executable truth)

No Makefile / CI workflows in-repo. Use these:

```bash
# Env (never commit .env)
cp .env.example .env   # INFERENESIA_BASE_URL, INFERENESIA_API_KEY (or legacy TEMP_AI_*); optional INFERENESIA_MODEL, INFERENESIA_HOME (legacy YURA_AI_HOME)

# Go binaries
go build -o bin/inferenesia ./cmd/inferenesia
go build -o bin/inferenesia-desktop ./cmd/desktop

# Frontend (typecheck + vite + copy → cmd/desktop/frontend/dist)
(cd web && npm install && npm run build)

# Verify (typical order for a feature unit)
go vet ./...
go test ./...
go build -o /dev/null ./cmd/inferenesia ./cmd/desktop
(cd web && npm run typecheck)
(cd web && npm run test:unit)   # node --test over an explicit file list in package.json — not a glob
(cd web && npm run build)

# Focused Go package
go test ./internal/writegate/...
go test ./internal/core/ -count=1

# Hub for manual QA / agent-browser (isolate config home)
INFERENESIA_HOME=/tmp/inferenesia-smoke \
  ./bin/inferenesia-desktop --serve --port 4110
# → http://127.0.0.1:4110  · health: curl -sf http://127.0.0.1:4110/api/health

# Vite HMR (proxies /api → 4110; port locked)
(cd web && npm run dev)   # http://127.0.0.1:4150
```

Optional env names (no secrets here): `INFERENESIA_*` (legacy `TEMP_AI_*`, `YURA_AI_*`), `INFERENESIA_HOME` (legacy `YURA_AI_HOME`), `INFERENESIA_BYOK_*` (legacy `YURA_AI_BYOK_*`), `INFERENESIA_MCP` (legacy `YURA_AI_MCP`), `INFERENESIA_PTY_DIR` (legacy `YURA_AI_PTY_DIR`). 9Router and Anthropic are **not** auto-registered from env — declare them as YAML profiles with `api_key_env` (see [docs/providers.md](./docs/providers.md)).

Token Savers (RTK / Headroom / Caveman / Ponytail) default **off**; missing tools must **pass through**, not crash chat.

---

## 3b. Frontend skills (Playground / chat / workspaces) — DEFAULT

Product UI work on **Playground**, **chat**, **workspaces/sessions**, and **shell chrome** must load the project skills below (not greenfield marketing aesthetics alone).

| Skill | Role |
|-------|------|
| **`inferenesia-product-ui`** | **Default adapter** — dials for dense IDE shell, keep `shell-*` tokens + Lucide, surface rules |
| **`design-taste-frontend`** | Anti-slop direction from [Leonxlnx/taste-skill](https://github.com/Leonxlnx/taste-skill) (v2) |
| **`redesign-existing-projects`** | Audit → fix in place (preferred for existing panels) |
| **`full-output-enforcement`** | No truncated / placeholder UI code |
| OCS `frontend-ui-ux` / `impeccable-style` | When available in OpenCode global skills |

### Paths (project-local)

```
.agents/skills/inferenesia-product-ui/SKILL.md
.agents/skills/design-taste-frontend/SKILL.md
.agents/skills/redesign-existing-projects/SKILL.md
.agents/skills/full-output-enforcement/SKILL.md
skills-lock.json
```

### Install / update

```bash
# From repo root (inferenesia-app)
npx skills add https://github.com/Leonxlnx/taste-skill \
  --skill design-taste-frontend \
  --skill redesign-existing-projects \
  --skill full-output-enforcement \
  -a opencode -a claude-code -a codex -a cursor -y
```

Keep **`inferenesia-product-ui`** (hand-authored adapter) when re-installing taste-skill.

### Delegation

When spawning visual work (`category="visual-engineering"` or Playground/chat/workspace UI):

```
load_skills=["inferenesia-product-ui", "design-taste-frontend", "redesign-existing-projects", "frontend-ui-ux"]
```

**Do not** apply landing-page dials (high variance / cinematic motion) to the product shell unless the user asks for a marketing page.

---

## 3c. Diagrams & office document skills

Project skills for diagrams / design docs / Word / PDF (load **on demand**, not for every UI task):

| Skill | Source | Use when |
|-------|--------|----------|
| **`excalidraw-diagram-generator`** | [github/awesome-copilot](https://github.com/github/awesome-copilot) | Create/edit `.excalidraw` flowcharts, architecture, mind maps, ERD |
| **`design-doc-mermaid`** | [spillwavesolutions/design-doc-mermaid](https://github.com/spillwavesolutions/design-doc-mermaid) | Mermaid sequence/activity/architecture from text or code; design-doc templates |
| **`docx`** | [anthropics/skills](https://github.com/anthropics/skills) | Create/read/edit Word `.docx` / `.dotx` |
| **`pdf`** | [anthropics/skills](https://github.com/anthropics/skills) | Read/merge/split/create/OCR PDF |

Paths: `.agents/skills/<name>/` · lockfile: `skills-lock.json`.

```bash
# From repo root — reinstall / update
npx skills add https://github.com/github/awesome-copilot --skill excalidraw-diagram-generator -a opencode -a claude-code -a codex -a cursor -y
npx skills add https://github.com/spillwavesolutions/design-doc-mermaid --skill design-doc-mermaid -a opencode -a claude-code -a codex -a cursor -y
npx skills add https://github.com/anthropics/skills --skill docx -a opencode -a claude-code -a codex -a cursor -y
npx skills add https://github.com/anthropics/skills --skill pdf -a opencode -a claude-code -a codex -a cursor -y
```

**Delegation examples:**

```
# Excalidraw / whiteboard files
load_skills=["excalidraw-diagram-generator"]

# Mermaid design docs / architecture from code
load_skills=["design-doc-mermaid"]

# Word / PDF deliverables (office track)
load_skills=["docx"]   # or ["pdf"]
```

**Notes:**

- Prefer **Playground Mermaid** + app tools for in-app diagram UX; use these skills for **file generation** and heavy design-doc workflows.
- `docx` / `pdf` are Anthropic document skills (proprietary license in their LICENSE.txt) — use only for document tasks, not product shell UI.
- Security scanners may flag some diagram skills as higher risk (full agent permissions); review scripts under `.agents/skills/*/scripts/` before untrusted runs.

---

## 4. Definition of Done — every feature (MANDATORY)

A feature is **not done** until **all** of:

1. **Implementation + verification evidence** — run available tests / typecheck / vet / build; record pass or intentional gap.
2. **Docs updated** for user-facing or architectural change — edit the right file under `docs/`; new area → link from `docs/README.md`. Touch `docs/decisions.md` only for truly locked decisions.
3. **Multi Brain updated (local only)** — index entry + context when decisions/blockers/files/verification matter; refresh `session.md` timestamps. **Never stage `.multibrain/`.**
4. **Local git commit** with a clear message; include product docs under `docs/` when they document that feature (not Multi Brain files).
5. **Push** only under **Push Policy** below (not every commit).

Mental checklist: `[ ] code verified  [ ] docs updated  [ ] multibrain local updated  [ ] committed  [ ] push only if policy says yes`

---

## 5. Docs update rules

- Prefer small accurate edits over sprawling new docs.
- After code lands, keep **local** `docs/` aligned with real behavior (stack, surfaces, undo, providers) when you change product behavior.
- **Public GitHub** must not grow internal design dumps: update root **`README.md`** only for end-user-facing facts (install, features, safety). Do not re-introduce a public `docs/` tree unless the user explicitly opens it.
- Never put secrets in docs or README (keys, tokens, `.env` contents, credential strings).
- Cross-link local design notes from `docs/README.md` when working on this machine.

---

## 6. Push Policy (MANDATORY)

### PUSH when (any true)

- User explicitly asks to push / publish / update remote.
- Feature milestone finished **and** full Definition of Done (code + docs + local commit; Multi Brain stays local).
- End of a planned phase/milestone in `docs/roadmap.md` that leaves remote coherent.
- Hotfix that must exist on remote after verification.
- Hand-off to a human/machine that **only** has the GitHub remote (not this disk).
- Multi-session work that would otherwise live only on this laptop.

### DO NOT PUSH when

- WIP, red tests, or broken build.
- Exploratory notes without verified outcome.
- Working tree contains secrets / `.env` / keys / credential-like files (stop and fix first).
- No explicit push criterion and user did not request push this session — default = **local commit only**, report readiness.
- Remote would expose local-only paths or secrets.

### Before every push

1. `git status` clean or only intended files staged.
2. Review commits about to push — no secrets in history of those commits.
3. `.gitignore` covers env/key patterns.
4. Prefer `git push -u origin main` only if tracking unset; **never** force-push `main` unless user explicitly orders force.
5. Never `--force` / `--force-with-lease` without explicit user request.
6. On remote reject: report the error; do not rewrite history silently.

| | |
|---|---|
| **Commit** | After a meaningful local unit; part of feature Done |
| **Push** | Only when policy says yes; many local commits may stack first |

---

## 7. Git hygiene & safety

- Never commit credentials, API keys, tokens, or real `.env` files.
- Respect root `.gitignore` (env, keys, `bin/`, `dist/`, `node_modules/`, `.yura-ai/`, `.multibrain/`, **`docs/`**, `.playwright-mcp/`, `.run/`, local DBs). Do not `git add -f` ignored secrets, agent memory, or private design docs.
- Note: broad `*secret*` ignore patterns re-include intentional **source** paths (`internal/secrethyg/`, `internal/git/secrets.go`, some docs) — **not** `.multibrain/`.
- Commit subject: short, **why**-focused. Optional co-author trailer if this environment already uses the droid pattern:
  `Co-authored-by: factory-droid[bot] <138933559+factory-droid[bot]@users.noreply.github.com>`
- Do not change `git config` unless the user asks.
- Do not put secrets in `docs/`, `.multibrain/`, commit messages, or PR/issue text.
- If a credential-looking file appears in the tree: stop, unstage, ensure ignored, report — do not push.
- `config.LoadDotEnvFromCWD()` loads repo `.env` without overriding non-empty existing env vars — still never commit that file.
