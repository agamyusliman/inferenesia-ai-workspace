/**
 * Auto-continue heuristics + ephemeral continue prompts (VAL-CHAT-026).
 * Continue prompts must NOT be stored as user history rows — core receives
 * Ephemeral=true so the durable workspace thread only keeps real turns.
 */

/** Default max auto-continue rounds per user send (stoppable). */
export const MAX_AUTO_CONTINUE_ROUNDS = 3

/** Ephemeral prompt text sent to core when continuing (not shown as a user bubble). */
export const EPHEMERAL_CONTINUE_PROMPT =
  'Continue from where you left off. Complete the unfinished answer. Do not fake shell or tool calls as plain text — either invoke real tools or finish the reply in prose.'

/** Visible UI label while auto-continuing (VAL-CHAT-026). */
export const CONTINUING_STATUS_LABEL = '…continuing…'

/**
 * Detect incomplete assistant answers that warrant an auto-continue.
 * Patterns: truncated mid-sentence, open code fences, unfinished lists,
 * fake tool / shell patterns, explicit "continue" invitations.
 */
export function needsAutoContinue(content: string | null | undefined): boolean {
  const text = (content || '').trim()
  if (!text) return false

  // Already stopped / error / fully stopped-by-user — do not continue.
  if (text.includes('_(stopped by user)_')) return false
  if (/^\(error\)/i.test(text) || text.startsWith('(image gen)')) return false

  // Unclosed markdown code fence.
  const fenceCount = (text.match(/```/g) || []).length
  if (fenceCount % 2 === 1) return true

  // Fake tool / shell call patterns (model printed tool XML/markdown instead of calling tools).
  if (looksLikeFakeToolOrShell(text)) return true

  // Explicit unfinished markers.
  if (
    /\b(to be continued|continue\s*…|continue\.\.\.|i'll continue|let me continue)\b/i.test(
      text,
    )
  ) {
    return true
  }
  if (/\b(i'll|i will)\s+(now\s+)?(run|call|execute|use)\s+(the\s+)?tool/i.test(text)) {
    return true
  }

  // Ends mid-thought: trailing ellipsis or incomplete sentence without terminal punctuation.
  if (/(…|\.\.\.)\s*$/.test(text)) return true

  // Ends with dangling connectors.
  if (/\b(and|or|but|with|to|for|of|the|a|an|because|so|then|also)\s*$/i.test(text)) {
    return true
  }

  // Very short assistant blip that looks cut off (no tools yet).
  if (text.length < 40 && !/[.!?…:)]\s*$/.test(text) && !text.includes('\n')) {
    // Single-word or partial word answers are often truncated streams.
    if (!/\b(ok|yes|no|done|ready)\b/i.test(text)) return true
  }

  return false
}

/** Detect fake tool / shell dumps in assistant prose. */
export function looksLikeFakeToolOrShell(text: string): boolean {
  const t = text || ''
  if (/<\s*tool_call\b/i.test(t)) return true
  if (/<\/\s*tool_call\s*>/i.test(t)) return true
  if (/```(?:tool|shell|bash|sh|zsh)\b/i.test(t) && /(?:run_terminal|execute_command|shell\()/i.test(t)) {
    return true
  }
  // Model narrates a tool without invoking real tools.
  if (
    /(?:invoke|calling|call)\s+(?:tool|function)\s*[`:]?\s*\w+/i.test(t) &&
    /(?:wait|hold on|let me|i will)/i.test(t)
  ) {
    return true
  }
  // Fake XML-style function call blocks.
  if (/<\s*function\b/i.test(t) && /<\/\s*function\s*>/i.test(t)) return true
  if (/run_terminal_cmd|execute_bash|run_shell_command/i.test(t) && /```/.test(t)) {
    return true
  }
  return false
}

/**
 * Whether another continue round is allowed.
 */
export function canAutoContinue(
  alreadyContinued: number,
  content: string | null | undefined,
  opts?: { maxRounds?: number; stopped?: boolean; error?: boolean },
): boolean {
  if (opts?.stopped || opts?.error) return false
  const max = opts?.maxRounds ?? MAX_AUTO_CONTINUE_ROUNDS
  if (alreadyContinued >= max) return false
  return needsAutoContinue(content)
}

/**
 * Build the streamChat payload fields for an ephemeral continue turn.
 * Client must set ephemeral: true so core does not insert a user history row
 * with this prompt (VAL-CHAT-026).
 */
export function ephemeralContinueRequest(base?: {
  model?: string
  profile?: string
}): {
  prompt: string
  ephemeral: true
  continue: true
  model?: string
  profile?: string
} {
  return {
    prompt: EPHEMERAL_CONTINUE_PROMPT,
    ephemeral: true,
    continue: true,
    ...(base?.model ? { model: base.model } : {}),
    ...(base?.profile ? { profile: base.profile } : {}),
  }
}
