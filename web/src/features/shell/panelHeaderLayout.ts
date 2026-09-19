/**
 * Pure helpers for panel chrome header layout.
 * Keeps git-panel-header (and other panels) on one visual toolbar row:
 * never pair fixed header height with flex-col (clips subtitle / status under title).
 */

import { SHELL } from './shellTokens'

export type PanelHeaderLayoutMode = 'single-row'

/**
 * Root class for panel headers. Always a single-line flex toolbar (items-center).
 * Subtitle/status, when present, is truncated inline on the same row — not a second
 * flex-col row forced into fixed height.
 */
export function panelHeaderRootClass(_opts?: {
  dense?: boolean
  hasSubtitle?: boolean
}): string {
  return SHELL.panelHeaderClass
}

/**
 * Guard used by tests and docs: multi-row chrome under fixed header height is forbidden.
 */
export function isClippedMultiRowHeaderClass(className: string): boolean {
  const tokens = className.split(/\s+/).filter(Boolean)
  const hasFixedHeader =
    tokens.includes('h-8') || tokens.includes('h-10') || tokens.includes('h-9')
  const isCol = tokens.includes('flex-col')
  return hasFixedHeader && isCol
}
