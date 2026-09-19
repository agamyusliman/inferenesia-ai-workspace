import { useCallback, useEffect, useState } from 'react'
import { getAutonomy, setAutonomy, type AutonomyView } from '../../lib/api'

const LEVELS = ['off', 'low', 'medium', 'high'] as const
type AutonomyLevel = (typeof LEVELS)[number]

export const AUTONOMY_CHANGED_EVENT = 'inferenesia:autonomy-changed'

const LEVEL_LABELS: Record<AutonomyLevel, string> = {
  off: 'Off',
  low: 'Low',
  medium: 'Med',
  high: 'High',
}


const LEVEL_TITLES: Record<AutonomyLevel, string> = {
  off: 'Read-only only. Applies on the next chat turn.',
  low: 'Read-only + file edits. Git/terminal/MCP/shell blocked. Applies next turn.',
  medium: 'Adds git, terminal, MCP. Shell + destructive git blocked. Applies next turn.',
  high: 'All tool risks allowed. Git may still show NeedsApproval. Applies next turn.',
}

function nextLevel(current: string): AutonomyLevel {
  const idx = LEVELS.indexOf(current as AutonomyLevel)
  if (idx < 0) return 'low'
  return LEVELS[(idx + 1) % LEVELS.length]
}

function emitAutonomyChanged(view: AutonomyView) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(
    new CustomEvent(AUTONOMY_CHANGED_EVENT, { detail: view }),
  )
}

export function AutonomyPicker() {
  const [view, setView] = useState<AutonomyView | null>(null)
  const [busy, setBusy] = useState(false)

  const reload = useCallback(async () => {
    try {
      const v = await getAutonomy()
      setView(v)
    } catch {
      /* non-critical chrome */
    }
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  useEffect(() => {
    const onChanged = (ev: Event) => {
      const detail = (ev as CustomEvent<AutonomyView>).detail
      if (detail?.level) setView(detail)
      else void reload()
    }
    window.addEventListener(AUTONOMY_CHANGED_EVENT, onChanged)
    const onFocus = () => void reload()
    window.addEventListener('focus', onFocus)
    return () => {
      window.removeEventListener(AUTONOMY_CHANGED_EVENT, onChanged)
      window.removeEventListener('focus', onFocus)
    }
  }, [reload])

  const onCycle = useCallback(async () => {
    if (!view || busy) return
    const next = nextLevel(view.level)
    setBusy(true)
    try {
      const v = await setAutonomy({ level: next })
      setView(v)
      emitAutonomyChanged(v)
    } catch {
      /* keep previous view */
    } finally {
      setBusy(false)
    }
  }, [view, busy])

  if (!view) return null

  const level = (LEVELS.includes(view.level as AutonomyLevel)
    ? view.level
    : 'low') as AutonomyLevel

  return (
    <button
      type="button"
      data-testid="chat-autonomy-picker"
      data-level={level}
      disabled={busy}
      onClick={() => void onCycle()}
      title={LEVEL_TITLES[level]}
      className="hidden rounded-lg border border-shell-border bg-shell-active px-2 py-1 text-[11px] font-medium text-shell-text transition hover:bg-shell-hover disabled:opacity-50 sm:inline"
    >
      Autonomy: {LEVEL_LABELS[level]}
    </button>
  )
}

export function notifyAutonomyChanged(view: AutonomyView) {
  emitAutonomyChanged(view)
}
