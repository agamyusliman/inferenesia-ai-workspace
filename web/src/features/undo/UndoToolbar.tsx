import { useCallback, useEffect, useState } from 'react'
import { Redo2, Undo2 } from 'lucide-react'
import {
  getUndoStack,
  redoLast,
  restoreToHead,
  revertAgent,
  undoLast,
  type UndoStackState,
} from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { SHELL } from '../shell/shellTokens'

type Props = {
  selectedFile?: string
  refreshKey?: number
  onAfterAction?: () => void
}

const EMPTY: UndoStackState = {
  can_undo: false,
  can_redo: false,
  undo_depth: 0,
  redo_depth: 0,
  revert_agent_label: 'Revert agent',
  restore_to_head_label: 'Restore to HEAD',
  undo_label: 'Undo',
  redo_label: 'Redo',
}

export function UndoToolbar({
  selectedFile,
  refreshKey,
  onAfterAction,
}: Props) {
  const { t } = useLocale()
  const [stack, setStack] = useState<UndoStackState>(EMPTY)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState<string>('')

  const refresh = useCallback(async () => {
    try {
      const st = await getUndoStack()
      setStack(st)
    } catch {
      setStack(EMPTY)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh, refreshKey])

  const run = async (fn: () => Promise<{ message: string }>) => {
    setBusy(true)
    try {
      const res = await fn()
      setStatus(res.message)
      await refresh()
      onAfterAction?.()
    } catch (e) {
      setStatus(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onRestoreHead = async () => {
    if (!selectedFile) {
      setStatus(t('undo.selectFile'))
      return
    }
    setBusy(true)
    try {
      const res = await restoreToHead(selectedFile)
      setStatus(res.message)
      onAfterAction?.()
    } catch (e) {
      setStatus(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const btn =
    'inline-flex shrink-0 items-center gap-1 rounded border border-shell-border bg-shell-bg px-1.5 py-0.5 text-[11px] leading-none text-shell-text enabled:hover:bg-shell-active disabled:opacity-40'

  const undoLabel = stack.undo_label || t('action.undo')
  const redoLabel = stack.redo_label || t('action.redo')
  const revertLabel = stack.revert_agent_label || t('action.revertAgent')
  const restoreLabel = stack.restore_to_head_label || t('action.restoreHead')

  return (
    <div data-testid="undo-toolbar" className={SHELL.toolbarClass}>
      <span className="mr-0.5 hidden shrink-0 text-[10px] font-semibold uppercase tracking-wide text-shell-muted md:inline">
        {t('undo.agent')}
      </span>

      <button
        type="button"
        data-testid="btn-undo"
        data-label={undoLabel}
        disabled={busy || !stack.can_undo}
        onClick={() => void run(() => undoLast())}
        title={t('undo.undoTitle')}
        className={btn}
      >
        <Undo2 size={SHELL.iconXs} aria-hidden />
        <span className="hidden sm:inline">{undoLabel}</span>
      </button>

      <button
        type="button"
        data-testid="btn-redo"
        data-label={redoLabel}
        disabled={busy || !stack.can_redo}
        onClick={() => void run(() => redoLast())}
        title={t('undo.redoTitle')}
        className={btn}
      >
        <Redo2 size={SHELL.iconXs} aria-hidden />
        <span className="hidden sm:inline">{redoLabel}</span>
      </button>

      <span className="mx-0.5 h-3 w-px shrink-0 bg-shell-border" aria-hidden />

      <button
        type="button"
        data-testid="btn-revert-agent"
        data-label={revertLabel}
        disabled={busy || !stack.can_undo}
        onClick={() => void run(() => revertAgent())}
        title={t('undo.revertTitle')}
        className="inline-flex shrink-0 items-center rounded border border-amber-700/50 bg-amber-950/40 px-1.5 py-0.5 text-[11px] leading-none text-amber-200 enabled:hover:bg-amber-900/40 disabled:opacity-40"
      >
        <span className="truncate max-w-[7rem] sm:max-w-none">{revertLabel}</span>
      </button>

      <button
        type="button"
        data-testid="btn-restore-to-head"
        data-label={restoreLabel}
        disabled={busy || !selectedFile}
        onClick={() => void onRestoreHead()}
        title={t('undo.restoreTitle')}
        className="inline-flex shrink-0 items-center rounded border border-red-900/50 bg-red-950/30 px-1.5 py-0.5 text-[11px] leading-none text-red-200 enabled:hover:bg-red-900/30 disabled:opacity-40"
      >
        <span className="truncate max-w-[7rem] sm:max-w-none">{restoreLabel}</span>
      </button>

      <span
        data-testid="undo-toolbar-status"
        className="ml-auto min-w-0 max-w-[28%] truncate text-[10px] text-shell-muted sm:max-w-[40%]"
        title={status || stack.message || t('undo.hint')}
      >
        {status || stack.message || t('undo.hint')}
      </span>
    </div>
  )
}
