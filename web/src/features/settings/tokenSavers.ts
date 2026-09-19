/**
 * Pure helpers for Settings → Token Saver section (VAL-SAVER-001/002/007).
 * Defaults off; missing install surfaces as "Not installed".
 */

import type { TokenSaverStatus, TokenSaversView } from '../../lib/api'

export const TOKEN_SAVER_IDS = ['rtk', 'headroom', 'caveman', 'ponytail'] as const

export type TokenSaverId = (typeof TOKEN_SAVER_IDS)[number]

export function isTokenSaverId(id: string): id is TokenSaverId {
  return (TOKEN_SAVER_IDS as readonly string[]).includes(id)
}

export type SaverStatusLabels = {
  ready: string
  notInstalled: string
}

const DEFAULT_SAVER_STATUS_LABELS: SaverStatusLabels = {
  ready: 'Ready',
  notInstalled: 'Not installed',
}

/** Human status badge: "Not installed" when binary/endpoint missing. */
export function saverStatusLabel(
  s: Pick<TokenSaverStatus, 'installed' | 'status'>,
  labels?: SaverStatusLabels,
): string {
  const L = labels ?? DEFAULT_SAVER_STATUS_LABELS
  if (s.installed || s.status === 'ready') return L.ready
  return L.notInstalled
}

export function isNotInstalled(s: Pick<TokenSaverStatus, 'installed' | 'status'>): boolean {
  return !s.installed || s.status === 'not_installed'
}

/**
 * Merge a toggled saver into an existing view for optimistic UI.
 * Enabling a not-installed saver stays non-crashing (status unchanged).
 */
export function applyLocalToggle(
  view: TokenSaversView | null | undefined,
  id: string,
  enabled: boolean,
): TokenSaversView | null {
  if (!view) return null
  return {
    ...view,
    savers: (view.savers || []).map((s) =>
      s.id === id ? { ...s, enabled } : s,
    ),
  }
}

/** True when every listed saver defaults to off. */
export function allSaversDefaultOff(view: TokenSaversView | null | undefined): boolean {
  if (!view?.savers?.length) return true
  return view.savers.every((s) => !s.enabled)
}

export function findSaver(
  view: TokenSaversView | null | undefined,
  id: string,
): TokenSaverStatus | undefined {
  return view?.savers?.find((s) => s.id === id)
}
