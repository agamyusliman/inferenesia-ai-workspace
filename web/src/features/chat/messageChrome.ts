/**
 * Pure helpers for polished chat message chrome (VAL-CHAT-007..011, 032).
 * Kept free of React so node:test can run without a DOM.
 */

export type MessageRole = 'user' | 'assistant' | 'system'

/** Roles that use the user card style (right-accent, user bubble). */
export function isUserRole(role: MessageRole | string): boolean {
  return role === 'user'
}

/** Roles that use the assistant card style (panel card, markdown). */
export function isAssistantRole(role: MessageRole | string): boolean {
  return role === 'assistant'
}

export function shouldShowActionBar(opts: {
  streaming?: boolean
  complete?: boolean
  hovered?: boolean
  focused?: boolean
  plainInterim?: boolean
}): boolean {
  if (opts.plainInterim) return false
  if (opts.streaming) return false
  if (opts.hovered || opts.focused) return true
  if (opts.complete) return true
  return false
}

export function isAssistantPlainInterim(opts: {
  streaming?: boolean
  stopped?: boolean
  error?: boolean
  hasTools?: boolean
  hasTasks?: boolean
  content?: string | null
}): boolean {
  if (opts.streaming || opts.stopped || opts.error) return true
  if (opts.hasTools || opts.hasTasks) return false
  const t = (opts.content || '').trim()
  if (!t) return true
  if (hasFencedCode(t)) return false
  if (extractMarkdownImages(t).length > 0) return false
  if (/data:image\//i.test(t)) return false
  if (!t.includes('\n\n') && t.length < 480) return true
  return false
}

export function messageCardClass(
  role: MessageRole | string,
  opts?: { streaming?: boolean; plainInterim?: boolean },
): string {
  if (isUserRole(role)) {
    return 'shell-user-bubble group relative min-w-0 max-w-full overflow-x-hidden rounded-lg px-2.5 py-2 text-xs leading-relaxed text-shell-text transition-colors duration-150'
  }
  if (role === 'system') {
    return 'group relative min-w-0 max-w-full overflow-x-hidden rounded-md border border-shell-border/80 bg-shell-panel/50 px-2 py-1.5 text-xs leading-relaxed text-shell-muted transition-colors duration-150'
  }
  if (opts?.plainInterim) {
    return 'relative min-w-0 max-w-full overflow-visible px-0.5 py-0.5 text-xs leading-snug text-shell-text/90'
  }
  const streamPulse = opts?.streaming ? ' opacity-95' : ''
  return `group relative min-w-0 max-w-full overflow-visible px-0.5 py-1 text-xs leading-relaxed text-shell-text transition-colors duration-150${streamPulse}`
}

export function extractMarkdownImages(content: string | undefined | null): { alt: string; src: string }[] {
  if (!content) return []
  const out: { alt: string; src: string }[] = []
  const re =
    /!\[([^\]]*)\]\((data:image\/[a-z0-9+.-]+;base64,[A-Za-z0-9+/=\s]+|https?:\/\/[^)\s]+|\/[^)\s]+)(?:\s+"[^"]*")?\)/gi
  let m: RegExpExecArray | null
  while ((m = re.exec(content)) !== null) {
    const src = (m[2] || '').replace(/\s+/g, '').trim()
    if (!src) continue
    out.push({ alt: (m[1] || '').trim(), src })
  }
  return out
}

export function stripMarkdownImages(content: string | undefined | null): string {
  if (!content) return ''
  return content
    .replace(
      /!\[([^\]]*)\]\((data:image\/[a-z0-9+.-]+;base64,[A-Za-z0-9+/=\s]+|https?:\/\/[^)\s]+|\/[^)\s]+)(?:\s+"[^"]*")?\)/gi,
      '',
    )
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

/** True when content contains a fenced code block. */
export function hasFencedCode(content: string | undefined | null): boolean {
  if (!content) return false
  return /```[\w+-]*/.test(content)
}

/**
 * Whether assistant content should render as structured markdown
 * (vs plain pre while empty/streaming with no tokens yet).
 */
export function shouldRenderMarkdown(role: MessageRole | string, content: string | undefined | null): boolean {
  if (!isAssistantRole(role) && !isUserRole(role)) return false
  return Boolean(content && content.trim().length > 0)
}

/** Label for a code fence language (empty → plain). */
export function codeFenceLanguage(className?: string | null): string {
  if (!className) return ''
  const m = /language-([\w+-]+)/i.exec(className)
  return m?.[1] || ''
}

/**
 * Whether a fenced code block should render as a Mermaid diagram preview
 * (VAL-DIAG-001). True when the fence language is `mermaid`, or when the
 * source text starts with a recognized Mermaid diagram-type keyword.
 */
export function isMermaidFence(lang: string | undefined | null, text: string | undefined | null): boolean {
  const l = (lang || '').trim().toLowerCase()
  if (l === 'mermaid' || l === 'mmd') return true
  const t = (text || '').trim()
  if (!t) return false
  return /^(graph|flowchart|sequenceDiagram|classDiagram|stateDiagram|stateDiagram-v2|erDiagram|gantt|pie|mindmap|journey|gitGraph|C4Context)\b/i.test(t)
}

/**
 * Copy text helper abstraction for tests (actual clipboard is browser-only).
 * Returns the string that would be written to the clipboard.
 */
export function textForCopy(content: string | undefined | null): string {
  return content || ''
}
