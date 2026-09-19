/**
 * Shared shell metrics so panel headers, toolbars, and chrome stay aligned.
 * Top chrome is h-10 (40px) — closer to the 48px activity rail / nav hit targets.
 * Window resize is handled via overflow/min-size clamps in layout config.
 *
 * Panel headers must stay a single flex row (items-center). Do not use flex-col
 * under a fixed height — that clips Source Control / git status under the title.
 */
export const SHELL = {
  panelHeaderClass:
    'flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2',
  panelHeaderTitleClass:
    'truncate text-[11px] font-semibold uppercase tracking-wide text-shell-muted',
  panelSublineClass: 'truncate text-[10px] text-shell-text/80',
  mainHeaderClass:
    'flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2',
  toolbarClass:
    'flex h-10 shrink-0 items-center gap-1 overflow-x-auto border-b border-shell-border bg-shell-panel px-2',
  statusBarClass:
    'flex h-7 shrink-0 items-center justify-between gap-2 overflow-hidden border-t border-shell-border bg-shell-panel px-2 text-[11px] text-shell-muted',
  /** Fixed activity rail (icons only). */
  activityRail: 48,
  activityBtnClass:
    'shell-transition flex h-9 w-9 items-center justify-center rounded-md',
  iconSm: 16,
  iconXs: 12,
} as const
