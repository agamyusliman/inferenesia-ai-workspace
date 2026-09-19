/**
 * Pure helpers for chat streaming UX (VAL-CHAT-001..006).
 * Kept free of React so node:test can run without a DOM.
 */

/** Visible marker when the user stops mid-stream (Stop or Esc). */
export const STOPPED_BY_USER_MARKER = '_(stopped by user)_'

/** Stream status labels for assistant chrome and composer. */
export const STREAM_STATUS = {
  generating: 'Generating…',
  streaming: 'streaming',
  stopped: 'stopped',
  done: 'done',
  idle: 'idle',
  /** Auto-continue follow-up in progress (VAL-CHAT-026). */
  continuing: '…continuing…',
  error: 'error',
} as const

export type StreamStatus = (typeof STREAM_STATUS)[keyof typeof STREAM_STATUS]

/**
 * Append the stopped-by-user marker without duplicating it.
 * Empty partial content → marker alone.
 */
export function applyStoppedByUser(content: string | undefined | null): string {
  const base = (content || '').trimEnd()
  if (!base) return STOPPED_BY_USER_MARKER
  if (base.includes(STOPPED_BY_USER_MARKER)) return base
  return `${base}\n\n${STOPPED_BY_USER_MARKER}`
}

/** Role line suffix while streaming vs finished. */
export function assistantStatusLabel(
  streaming: boolean,
  stopped?: boolean,
  opts?: { continuing?: boolean; error?: boolean },
): string {
  if (stopped) return STREAM_STATUS.stopped
  if (opts?.error) return STREAM_STATUS.error
  if (opts?.continuing && streaming) return STREAM_STATUS.continuing
  if (streaming) return STREAM_STATUS.generating
  return ''
}

export type ComposerKeyAction = 'send' | 'newline' | 'stop' | 'none'

/**
 * Composer keyboard contract (VAL-CHAT-004 / VAL-CHAT-006):
 * - Enter without Shift → send (if busy: interrupt then send — handled by send())
 * - Shift+Enter → newline (browser default; no prevent)
 * - Escape while streaming → stop only (no new message)
 */
export function composerKeyAction(
  key: string,
  opts: { shiftKey?: boolean; busy?: boolean; pickerOpen?: boolean },
): ComposerKeyAction {
  if (key === 'Escape') {
    if (opts.pickerOpen) return 'none' // picker closes first
    if (opts.busy) return 'stop'
    return 'none'
  }
  // Enter always requests send; ChatPanel.send() interrupts if busy.
  if (key === 'Enter' && !opts.shiftKey) {
    return 'send'
  }
  if (key === 'Enter' && opts.shiftKey) {
    return 'newline'
  }
  return 'none'
}

/** True when Enter should submit (not Shift+Enter). */
export function shouldSendOnEnter(key: string, shiftKey: boolean): boolean {
  return key === 'Enter' && !shiftKey
}
