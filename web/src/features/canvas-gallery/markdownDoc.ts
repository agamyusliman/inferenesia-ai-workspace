// Markdown playground helpers: outline extraction + word/read stats for the
// document panel. Rendering reuses the shared MarkdownPreview component.

export type MarkdownHeading = {
  id: string
  level: number
  text: string
  /** 0-based line index in the source, used to scroll the editor. */
  line: number
}

export type MarkdownStats = {
  words: number
  characters: number
  headings: number
  readingMinutes: number
}

export const BLANK_MARKDOWN_DOC = `# New document

Write in Markdown. Use the outline on the left to jump between sections, and the
instruction bar below to ask for a rewrite, summary, or new section.

## Section

- Point one
- Point two
`

const FENCE_LINE = /^\s{0,3}(?:`{3,}|~{3,})/

/** Skips fenced code blocks so \`# comment\` lines never become headings. */
export function markdownOutline(source: string): MarkdownHeading[] {
  const lines = (source || '').split('\n')
  const out: MarkdownHeading[] = []
  let fence: string | null = null
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i]
    if (fence) {
      if (line.trimStart().startsWith(fence)) fence = null
      continue
    }
    if (FENCE_LINE.test(line)) {
      fence = line.trim().startsWith('~') ? '~~~' : '```'
      continue
    }
    const m = /^(#{1,6})\s+(.*\S)\s*$/.exec(line)
    if (!m) continue
    const text = m[2].replace(/#+\s*$/, '').trim()
    if (!text) continue
    out.push({
      id: `h${i}`,
      level: m[1].length,
      text,
      line: i,
    })
  }
  return out
}

export function markdownStats(source: string): MarkdownStats {
  const text = source || ''
  const words = text.trim() ? text.trim().split(/\s+/).length : 0
  return {
    words,
    characters: text.length,
    headings: markdownOutline(text).length,
    readingMinutes: words === 0 ? 0 : Math.max(1, Math.round(words / 220)),
  }
}

/** First H1/H2, else first non-empty line — used to auto-name new documents. */
export function titleFromMarkdown(source: string): string {
  const heading = markdownOutline(source).find((h) => h.level <= 2)
  if (heading?.text) return heading.text.slice(0, 80)
  const first = (source || '')
    .split('\n')
    .map((l) => l.trim())
    .find((l) => l && !l.startsWith('#'))
  return (first || '').slice(0, 80)
}

/**
 * Pulls a document out of a model reply: a fenced ```markdown block when
 * present, otherwise the raw text once conversational wrappers are gone.
 */
export function extractMarkdownFromReply(text: string): string | null {
  const raw = (text || '').trim()
  if (!raw) return null
  const fenced = /```(?:markdown|md)\s*\n([\s\S]*?)```/i.exec(raw)
  if (fenced?.[1]?.trim()) return fenced[1].trim()
  const open = /```(?:markdown|md)\s*\n([\s\S]*)$/i.exec(raw)
  if (open?.[1] && open[1].trim().length > 20) return open[1].trim()
  // A bare reply is acceptable only when it reads like a document, not chatter.
  if (/^#{1,6}\s/m.test(raw) || /^[-*]\s/m.test(raw) || raw.length > 200) {
    return raw
  }
  return null
}
