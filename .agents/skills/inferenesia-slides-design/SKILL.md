---
name: inferenesia-slides-design
description: Design system rules for Inferenesia Playground Slides (HTML 1280×720 decks). NOT for product shell / web app UI. Use when generating or editing slide HTML, agent prompts for slides, deck CSS, or reviewing slide layout quality.
---

# Inferenesia slides design system

**This is presentation design, not product UI.**

| Surface | Skill |
|---------|--------|
| App shell, Playground chrome, chat, settings | `inferenesia-product-ui` |
| **Slide artboard content (what the audience sees)** | **`inferenesia-slides-design` (this skill)** |

Do **not** apply dense IDE shell rules, `shell-*` tokens, or 12px UI type to slide content.

---

## 0. Canvas truth

- Fixed artboard: **1280 × 720** (16:9)
- Each slide: `<section class="slide" data-slide-id data-layout data-title>`
- Overflow: **hidden** — nothing may clip outside the artboard
- Tailwind utilities OK inside slide; base rules live in deck CSS

---

## 1. Safe area + spacing scale (non-negotiable)

Outer inset alone is not enough — **inner gaps** between components must breathe.

### Outer safe area (DYNAMIC)

| Rule | Value |
|------|--------|
| Minimum outer inset | **≥ 20px** (`p-5`) — never flush to edge |
| Typical | **24–40px** (`p-6` / `p-8` / `p-10`) by density |
| Sparse hero only | up to **48–56px** when content is light |
| Dense multi-card | prefer **20–28px** so content fits without forced wrap |
| Bleed / full-bleed | BG/edge only; text still ≥ **20px** from edges |

**Do not** force `p-14` / 56px on every slide.

### Inner spacing scale (use these, not web-app density)

| Token | px | Use for |
|-------|-----|---------|
| `xs` | 12–16 | Kicker → title only |
| `sm` | 16–20 | Title → subtitle; inside-card title → body |
| `md` | **20–28** | Default block gap; **card grid gap** |
| `lg` | **28–36** | Header block → cards / media / CTA |
| `xl` | 32–40 | Two-column column gap |

| Rule | Value |
|------|--------|
| Slide root stack | `flex flex-col gap-*` scaled to density |
| Card grid gap | **≥ 16–24px** (`gap-4` / `gap-6`) — never `gap-1`/`gap-2` |
| Split columns | **≥ 24–36px**; **tops aligned** |
| Card inner padding | **16–24px** (`p-4` / `p-5` / `p-6`) — not always `p-7` |

**Forbidden:** content flush to artboard edge; cards without padding; text past 720/1280 boundary; zero-margin heading piles.

**If tight:** scale title/type or cut copy — keep min **20px** outer inset.

---

## 2. Typography (presentation scale)

Slides are viewed from a distance / small stage preview. Prefer **large + bold**.

| Role | Size | Weight | Notes |
|------|------|--------|-------|
| Eyebrow / kicker | 12–14px | 700 | one line |
| Title (hero) | **36–48px** | **700–800** | long titles → smaller before wrap |
| Title (content) | **28–36px** | **700** | prefer 1–2 lines; `text-wrap:balance` |
| Body | **16–20px** | 500–600 | |
| Card title | **18–22px** | **700** | |
| Card body | **14–16px** | 500 | |
| KPI number | **36–52px** | **800** | |

**Forbidden:** `font-light` / `font-thin`; forcing `text-5xl`/`text-6xl` when it wraps badly — **scale down first**.

Line length: titles max ~2 lines; body max ~3 short lines per block.

---

## 3. Alignment & composition

Pick **one** primary alignment per slide and stick to it:

| Pattern | When |
|---------|------|
| **Center stack** | Title / CTA / quote |
| **Left stack + right media** | Two-column / split |
| **Left stack only** | Bullets / report |
| **Grid (2×2 / 3-col)** | Equal cards — same height, same padding |

Rules:

- Vertical rhythm scaled to density (not 8–12px web-app gaps)
- Prefer `flex flex-col gap-*` / `grid gap-*` over collapsing margins
- **Split / two-col:** both columns **top-aligned**. Long title → smaller type or shared header **above** the columns so left list and right feature card tops stay level
- Do not mix left-aligned titles with randomly centered cards on the same slide
- Numbered badges stay inside padded cards

### Card grid by item count (never force skinny N-col)

| Count | Prefer | Avoid |
|------:|--------|--------|
| 2 | 2-col | — |
| 3 | 3-col or 1+2 | — |
| 4 | **2×2** | 4 skinny columns |
| **5** | **3 top + 2 bottom** (center bottom) or **2+3** or compact horizontal rows | **Single 5-col row of tall empty cards** |
| 6 | 3×2 or 2×3 | 6-col |
| 7–8 | 4+3 / 4+4 / 3+3+2 | one long row |
| 9+ | 3×3 multi-row | |

- Card height follows **content** (auto). Do not stretch short copy into half-empty white towers.
- Row of cards may share equal height within that row only; never invent blank interior to fill the artboard.
- Dense sequential lists (e.g. 5 rukun): compact horizontal cards (number + title + 1 line) often beat vertical mini-posters.

---

## 4. Components on slides

### Cards

```
rounded-2xl + padding ≥ 24px + border OR soft shadow
title bold large + body medium 16–18px
never empty of padding; never overflow parent grid cell
```

### Split / media

- Text column: min 40% width; image `object-cover` with rounded container
- Text column keeps safe padding; do not pin text to the color edge without inset

### CTA

- Button: large hit (`px-8 py-3.5`), `font-semibold`, rounded-full or rounded-xl
- Secondary action clearly secondary (outline / ghost)

### Backgrounds

- Solid or soft gradient OK
- Decorative blobs must not reduce text contrast below readable levels
- Image background: darken overlay if text sits on top

### Images (smart — not default)

Images are **optional**, not a template habit.

| Do | Don't |
|----|--------|
| Use image when content is visual (product, photo, chart) or user attached one | Put an image on every slide of a deck |
| Keep feature/problem/benefit points as **text cards** | Replace every bullet with an image tile |
| 0–2 image-forward slides in a typical 5-slide deck | Invent stock photos for abstract points |
| Pure HTML/CSS UI when no attachment / no photo ask | Force placeholder image cards “for polish” |

### Emoji / decor (measure need — neither ban nor force)

Decorative style ≠ emoji on every bullet.

| Prefer | Avoid |
|--------|--------|
| Number badges (1–5) for sequential **content** lists | 5 emoji headers on a 5-card grid by default |
| Soft BG shapes / blobs / large low-opacity emoji as **atmosphere** | Treating emoji as required bullet chrome |
| 0–1 small icon on a **highlight** card when it adds meaning | Forcing emoji on every list item “because decorative” |
| Plain text cards when the list is already clear | Sticker spam / competing neon accents |

**Judgment:** if numbers or plain text are enough, skip per-item emoji. If one mood cue or feature callout helps, use it. BG ornaments often beat emoji-as-bullet.

### No deck slide index in content (hard)

Filmstrip order is **sortable** (drag-and-drop). Hardcoded deck position will go stale.

| Avoid on the slide artboard | OK |
|-----------------------------|-----|
| Big badge “1 / 2 / 3” next to the **slide title** (position in deck) | Semantic content numbers (Rukun 1–5, process steps) |
| “Slide 3”, “03”, footer “2 of 5” | Kicker/topic labels without deck index |
| Numbering that means “this is the Nth slide” | data-slide-id only (not shown as UI chrome on the canvas) |

---

## 5. Color & contrast

- Prefer high contrast: dark text on light OR light text on dark/brand panels
- Accent sparingly (one accent for kicker + CTA)
- Cards on tinted backgrounds need enough surface contrast (white/near-white or deep panel)

---

## 6. Layout recipes (prefer these)

| `data-layout` | Structure |
|---------------|-----------|
| `title` | Center: kicker + big title + short subtitle |
| `title-content` | Top kicker/title/lede + content block below with padding |
| `bullets` | Title left + large bullets (not tiny lists) |
| `two-column` / `split` | 50/50 or 45/55; both columns padded |
| `tiled` / `cards` | Grid by count: 2×2, 3-col, **3+2 for five**, 3×2 for six — never forced 5-col empty towers |
| `kpi` | 3–4 big numbers with labels |
| `bleed` | Full color/image panel + inset content card |
| `qa` / `cta` | Center question + primary/secondary actions |

---

## 7. Agent output checklist (must pass)

Before emitting HTML, verify:

1. [ ] Section is exactly 1280×720 class `slide`
2. [ ] Outer content inset ≥ **20px** (typical 24–40; not forced 56)
3. [ ] Titles fit (scale before awkward wrap); split columns top-aligned
4. [ ] Title bold presentation-scale; body readable (~16–20px)
5. [ ] Grid matches item count (5 → 3+2 / 2+3, not 5-col); cards not empty towers
6. [ ] No deck slide index badge/footer (“Slide 2”, “2 of 5”) — only semantic content numbers if needed
7. [ ] Decor measured: no forced emoji-per-bullet; BG accents OK
8. [ ] One alignment system; no overflow; Tailwind only (absolute for BG/bleed OK)

---

## 8. What this skill is NOT

- Not for Excalidraw freeform whiteboard
- Not for product shell density / Lucide chrome
- Not for marketing landing pages outside the 16:9 deck
- Not an excuse to ship tiny web-app UI onto a projector

---

## 9. Wiring in code

- CSS guardrails: `web/src/features/canvas-gallery/slides/htmlDeck.ts` → `deckBaseStyles()`
- Agent hard rules: `SlidesAgentPanel` system prompt must include this skill’s safe-area + type scale
- Product chrome around slides still uses `inferenesia-product-ui`

When delegating slide visual work:

```
load_skills=["inferenesia-slides-design"]
```

Do **not** load only `inferenesia-product-ui` for slide **content** design.
