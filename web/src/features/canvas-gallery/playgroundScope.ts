// Workspace-bound mode was removed — the UI never wired real workspace ids to
// canvas panels; every non-Library canvas is inherently session-bound.
// Library canvases pass `undefined`.
import { GALLERY_GLOBAL_SCOPE } from './galleryStore'

export type PlaygroundMode = 'session' | 'library'

export type PlaygroundScope = {
  mode: PlaygroundMode
  sessionId: string | undefined
  playgroundId: string
  agentKind: string
}

/**
 * Resolves the 2 binding modes from gallery state.
 * - Session-bound: canvas entry has a real session ID (not 'global')
 * - Library: canvas entry is in library scope
 *
 * playgroundId is composed as `${kind}::${ownerScope}` where ownerScope is:
 * - session ID (session mode)
 * - `global` (library mode)
 *
 * `sessionOrWorkspaceHint` is the caller's ambient scope id (a `ws_sess_*`
 * session id or `undefined` for Library); it does not affect resolution and is
 * kept only so callers can keep passing their scope through unchanged.
 */
export function resolvePlaygroundScope(
  kind: string,
  canvasEntrySessionId: string | undefined,
  _sessionOrWorkspaceHint?: string | undefined,
): PlaygroundScope {
  // Session-bound: canvas entry has a real session ID
  if (canvasEntrySessionId && canvasEntrySessionId !== GALLERY_GLOBAL_SCOPE) {
    return {
      mode: 'session',
      sessionId: canvasEntrySessionId,
      playgroundId: `${kind}::${canvasEntrySessionId}`,
      agentKind: kind,
    }
  }

  // Library: standalone, no session
  return {
    mode: 'library',
    sessionId: undefined,
    playgroundId: `${kind}::global`,
    agentKind: kind,
  }
}
