/**
 * Pure helpers for chat reliability (VAL-CHATBUG-001..008):
 * sticky model/profile on next send, cancel unstick, generation token races.
 */

/** Snapshot of mid-session sticky provider/model for the next chat turn. */
export type StickySession = {
  profile?: string
  model?: string
  label?: string
}

/**
 * Merge ActiveSessionView (or partial) into a sticky snapshot used on streamChat.
 * Empty profile/model fields are omitted so core.Service can fall back to session.
 */
export function stickyFromActive(view: {
  profile?: string
  model?: string
  profile_name?: string
}): StickySession {
  const profile = (view.profile || '').trim()
  const model = (view.model || '').trim()
  const name = (view.profile_name || profile || '').trim()
  const label =
    name || model
      ? `${name || profile || 'provider'}${model ? ` · ${model}` : ''}`
      : ''
  return {
    ...(profile ? { profile } : {}),
    ...(model ? { model } : {}),
    ...(label ? { label } : {}),
  }
}

/**
 * Build streamChat profile/model fields from sticky session.
 * Used so the next send pins the picker selection even if a concurrent
 * finishing turn overwrote core session briefly (VAL-CHATBUG-001/002).
 */
export function stickyRequestFields(sticky: StickySession | null | undefined): {
  profile?: string
  model?: string
} {
  if (!sticky) return {}
  const out: { profile?: string; model?: string } = {}
  if (sticky.profile) out.profile = sticky.profile
  if (sticky.model) out.model = sticky.model
  return out
}

/**
 * Generation token: bumping invalidates in-flight send/auto-continue work so
 * abandoned promises cannot re-lock busy after Stop/Esc or workspace switch.
 */
export function nextGenerationToken(current: number): number {
  const n = (current || 0) + 1
  // wrap at safe integer range
  return n > 1_000_000_000 ? 1 : n
}

/** True when a run should abandon work (token no longer matches). */
export function isRunStale(myToken: number, liveToken: number): boolean {
  return myToken !== liveToken
}

/**
 * Composer busy policy after Stop/Esc (VAL-CHATBUG-003/004/005):
 * - clear busy immediately (do not wait for fetch settle)
 * - clear generating / continuing flags on the streaming assistant
 */
export function postCancelComposerState(): {
  busy: false
  stopContinue: true
} {
  return { busy: false, stopContinue: true }
}

/**
 * Whether Send may accept a new prompt (VAL-CHATBUG-003/004).
 * Busy only blocks; empty text handled by canSendWithAttachments elsewhere.
 */
export function canSendAfterCancel(busy: boolean): boolean {
  return !busy
}

/**
 * Mid-stream model picker policy (VAL-CHATBUG-002):
 * disable while streaming so the active turn cannot be mid-flight switched silently.
 * Deferred apply after stop is handled by re-enabling picker when busy=false.
 */
export function modelPickerDisabledWhileStreaming(busy: boolean): boolean {
  return !!busy
}

/**
 * After stop during auto-continue, no further ephemeral rounds should fire
 * (VAL-CHATBUG-007). Gate for the continue loop.
 */
export function shouldRunAutoContinue(opts: {
  stopContinue: boolean
  stopped: boolean
  error: boolean
  wantImageGen: boolean
  stale: boolean
}): boolean {
  if (opts.stale) return false
  if (opts.stopContinue) return false
  if (opts.stopped || opts.error) return false
  if (opts.wantImageGen) return false
  return true
}

export function workspaceSwitchChatReset(currentToken: number): {
  nextToken: number
  busy: false
  stopContinue: false
  streamingAssistantId: null
} {
  return {
    nextToken: nextGenerationToken(currentToken),
    busy: false,
    stopContinue: false,
    streamingAssistantId: null,
  }
}
