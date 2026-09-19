---
name: inferenesia-product-ui
description: Default frontend skill for Inferenesia App product shell — Playground, chat, workspaces, sessions, settings, and shell chrome. Wraps design-taste-frontend + redesign-existing-projects with Inferenesia constraints (existing shell tokens, density, no landing-page takeover).
---

# Inferenesia product UI (default frontend)

Use this skill for **product UI** work inside Inferenesia App:

| Surface | Examples |
|---------|----------|
| **Playground** | Gallery list, HTML device frames, Mermaid panes, agent bar, empty create grid |
| **Chat** | Composer, bubbles, code/HTML blocks, model picker, meta rows |
| **Workspaces / Sessions** | Lists, open-folder empty state, Browse, recents, session title |
| **Shell** | Sidebar rail, MainPane, splitters, status bar, command palette, settings |

Also load (when available):

1. **`design-taste-frontend`** — anti-slop direction, dials, pre-flight (from Leonxlnx/taste-skill)
2. **`redesign-existing-projects`** — audit → fix in place (preferred over greenfield restyle)
3. **`frontend-ui-ux` / `impeccable-style`** — OCS visual bar when present

Do **not** treat this skill as a license to redesign the whole product unless the user asks.

---

## 0. Design read (always, one line)

State before code:

> Reading this as: **Inferenesia product shell** (Playground | Chat | Workspaces | Settings) for **developers**, language = **dense dark IDE / Linear-adjacent**, foundation = **existing `shell-*` tokens + Tailwind**, motion = **restrained**.

If the user wants a **marketing landing** or **portfolio** (not the app shell), drop this skill and use `design-taste-frontend` alone with landing dials.

---

## 1. Dials for product shell (defaults)

| Dial | Product shell | Landing / portfolio |
|------|---------------|---------------------|
| `DESIGN_VARIANCE` | **4–5** | 7–9 |
| `MOTION_INTENSITY` | **2–4** | 6–8 |
| `VISUAL_DENSITY` | **6–8** | 3–5 |

Inferenesia is a **coding workspace**: information density and scanability beat artsy asymmetry. Do not force split-screen heroes or bento marketing layouts into the rail/chat/playground.

---

## 2. Hard product constraints (non-negotiable)

### Design system = already exists

- Tokens: CSS variables `--shell-*`, Tailwind `shell.bg`, `shell.panel`, `shell.border`, `shell.text`, `shell.muted`, `shell.accent`, `shell.hover`
- Theme: warm / dark / light via `data-theme` — preserve all three
- Icons: **`lucide-react` is already the product family** — keep it (taste-skill’s Lucide ban does **not** apply here)
- Type: shell UI type scale already set — do not swap the whole app to Geist/Inter display stacks without an explicit brand pass
- Radius / spacing: match existing panels (`rounded-md` / `rounded-lg`, tight `gap-1`–`gap-2` chrome)

### Stack truth (do not invent)

- React + Vite + Tailwind (not Next.js RSC)
- Monaco for code; Mermaid for diagrams; device frames via `devicePresets.ts`
- No new heavy motion library (GSAP / Motion) unless the user asks and you justify bundle cost
- Prefer CSS transitions / existing patterns over cinematic scroll hijacks

### Accessibility & density

- WCAG AA contrast on buttons, form fields, badges (shell-muted on shell-bg is a known risk — fix when touching)
- Keyboard: Esc closes overlays; focus traps in modals
- `prefers-reduced-motion`: no mandatory continuous animation

### Scope discipline

| In scope | Out of scope unless asked |
|----------|---------------------------|
| Local component polish | Full rebrand / new color system |
| Spacing, hierarchy, empty states | Replacing Lucide with Phosphor |
| Playground mockup / chat block UX | Marketing landing pages |
| Consistent hover/focus/disabled | GSAP page transitions |

---

## 3. Surface-specific rules

### Playground

- List + detail: one clear hierarchy (title → scope badge → updatedAt)
- HTML device frames: content **must stay inside** mockup (`outerFrameSize` + bezel padding + `overflow-hidden`)
- Empty create grid: vertical center, roadmap kinds “coming soon” readable but not louder than ready kinds
- Agent bar: ephemeral, dense; Generating left of Send — do not turn into a chat composer clone

### Chat

- HTML blocks: readable code + preview height; actions (Download / Open in playground) scannable
- Prefer product language **Playground** (not Canvas) in user-visible strings
- Avoid decorative gradients behind messages; stick to shell panel/border

### Workspaces / Sessions

- Open folder: **Browse folder…** primary CTA, full-width; no drop-zone theater
- Recents: solid panel bg + hover; path mono muted
- Session vs folder modes stay visually distinct without loud color coding

### Shell chrome

- Rail icons: tooltips via i18n; active state = accent, not a new brand color
- Splitters/resizers: hit targets ≥ existing; no ornamental grips

---

## 4. Anti-slop still applies (adapted)

Still ban:

- Purple/blue “AI gradient” CTAs
- Three equal generic marketing cards in product chrome
- Inter+slate hero paste into settings/chat
- Emoji as UI decoration
- Placeholder “lorem” / Acme copy in empty states

Prefer:

- One accent (`shell-accent`) locked for the surface
- Clear empty states with one primary action
- Full interactive states: hover, active, disabled, loading, error
- Consistent corner radius within a panel

---

## 5. Workflow

1. **Design read** (one line) + dials for product shell  
2. If touching existing UI → follow **`redesign-existing-projects`** (scan → diagnose → fix)  
3. Match neighboring components (read 1–2 siblings before inventing styles)  
4. Keep i18n keys in `messages.ts` for new user-visible strings (en + id)  
5. Verify: typecheck/tests if you touched logic; visual hard-refresh for layout  

---

## 6. When NOT to use this skill

- Backend/Go, hub API, writegate, providers  
- Docs-only / Multi Brain notes  
- Pure logic refactors with no markup/CSS change  
- Greenfield **marketing** sites → use `design-taste-frontend` alone  

---

## 7. Install / companion skills

Project-installed (via `npx skills add Leonxlnx/taste-skill`):

- `.agents/skills/design-taste-frontend/`
- `.agents/skills/redesign-existing-projects/`
- `.agents/skills/full-output-enforcement/`

Update:

```bash
npx skills add https://github.com/Leonxlnx/taste-skill \
  --skill design-taste-frontend \
  --skill redesign-existing-projects \
  --skill full-output-enforcement \
  -a opencode -a claude-code -a codex -a cursor -y
```

Lockfile: `skills-lock.json` at repo root.
