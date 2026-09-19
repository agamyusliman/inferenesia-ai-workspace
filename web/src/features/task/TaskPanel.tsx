/**
 * Background tasks panel — subscriber only (VAL-ORCH-010).
 * State is sourced from listTasks API + TaskSpawned/TaskDone events via refreshKey.
 * No second agent loop; no direct LLM calls.
 */
import { useCallback, useEffect, useState } from 'react'
import {
  CheckCircle2,
  Circle,
  Loader2,
  Play,
  RefreshCw,
  Square,
  X,
  XCircle,
} from 'lucide-react'
import {
  cancelTask,
  listTasks,
  resumeTask,
  type TaskView,
} from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'

type Props = {
  /** Bump on TaskSpawned/TaskDone stream events. */
  refreshKey?: number
  onClose?: () => void
}

function statusTone(status: string): string {
  switch (status) {
    case 'completed':
      return 'text-emerald-300 bg-emerald-950/50 border-emerald-800/60'
    case 'failed':
      return 'text-rose-200 bg-rose-950/50 border-rose-800/60'
    case 'cancelled':
      return 'text-zinc-300 bg-zinc-900/60 border-zinc-700/60'
    case 'running':
    case 'queued':
      return 'text-sky-200 bg-sky-950/40 border-sky-800/50'
    default:
      return 'text-shell-muted bg-shell-panel border-shell-border'
  }
}

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return <CheckCircle2 size={SHELL.iconSm} className="text-emerald-400" aria-hidden />
    case 'failed':
      return <XCircle size={SHELL.iconSm} className="text-rose-400" aria-hidden />
    case 'cancelled':
      return <Square size={SHELL.iconSm} className="text-zinc-400" aria-hidden />
    case 'running':
      return <Loader2 size={SHELL.iconSm} className="animate-spin text-sky-300" aria-hidden />
    default:
      return <Circle size={SHELL.iconSm} className="text-shell-muted" aria-hidden />
  }
}

function formatElapsed(ms: number): string {
  if (!ms || ms < 0) return '0s'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return `${m}m ${rem}s`
}

export function TaskPanel({ refreshKey, onClose }: Props) {
  const { t } = useLocale()
  const [tasks, setTasks] = useState<TaskView[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [banner, setBanner] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setBusy(true)
    try {
      const list = await listTasks()
      setTasks(list.tasks || [])
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh, refreshKey])

  // Tick elapsed for running tasks every second (display only).
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const hasRunning = tasks.some((t) => t.status === 'running' || t.status === 'queued')
    if (!hasRunning) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [tasks])

  const displayElapsed = (t: TaskView): number => {
    if (t.status === 'running' && t.started_at) {
      const start = Date.parse(t.started_at)
      if (!Number.isNaN(start)) return Math.max(0, now - start)
    }
    return t.elapsed_ms || 0
  }

  const onCancel = async (id: string) => {
    setBusy(true)
    setBanner(null)
    try {
      await cancelTask(id)
      setBanner(`Cancelled ${id}`)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onResume = async (id: string) => {
    setBusy(true)
    setBanner(null)
    try {
      await resumeTask(id)
      setBanner(`Resumed ${id}`)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      data-testid="task-panel"
      className="flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader
        title={t('task.title')}
        dense
        actions={
          <div className="flex shrink-0 items-center gap-0.5">
            <button
              type="button"
              data-testid="task-panel-refresh"
              className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
              title={t('action.refresh')}
              aria-label={t('action.refresh')}
              onClick={() => void refresh()}
              disabled={busy}
            >
              <RefreshCw size={SHELL.iconXs} className={busy ? 'animate-spin' : ''} />
            </button>
            {onClose && (
              <button
                type="button"
                data-testid="task-panel-close"
                className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
                title={t('panel.closeTitle')}
                aria-label={t('panel.closeTitle')}
                onClick={onClose}
              >
                <X size={SHELL.iconXs} aria-hidden />
              </button>
            )}
          </div>
        }
      />

      {banner && (
        <div
          data-testid="task-panel-banner"
          className="border-b border-shell-border bg-shell-panel px-3 py-1 text-[11px] text-shell-muted"
        >
          {banner}
        </div>
      )}
      {error && (
        <div
          data-testid="task-panel-error"
          className="border-b border-rose-900 bg-rose-950/40 px-3 py-1 text-[11px] text-rose-200"
        >
          {error}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto p-2" data-testid="task-panel-list">
        {tasks.length === 0 ? (
          <p className="px-2 py-6 text-center text-xs text-shell-muted">
            No background tasks yet. Parallel task() spawns appear here.
          </p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {tasks.map((t) => (
              <li
                key={t.task_id}
                data-testid="task-row"
                data-task-id={t.task_id}
                data-task-status={t.status}
                data-task-category={t.category}
                className="rounded border border-shell-border bg-shell-panel/60 px-2.5 py-2"
              >
                <div className="flex items-start gap-2">
                  <StatusIcon status={t.status} />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span
                        className="font-mono text-[11px] text-shell-text"
                        data-testid="task-id"
                      >
                        {t.task_id}
                      </span>
                      <span
                        className={`rounded border px-1.5 py-0.5 text-[10px] uppercase tracking-wide ${statusTone(t.status)}`}
                        data-testid="task-status"
                      >
                        {t.status}
                      </span>
                      <span
                        className="rounded bg-shell-bg px-1.5 py-0.5 text-[10px] text-shell-muted"
                        data-testid="task-category"
                      >
                        {t.category || 'explore'}
                      </span>
                      <span
                        className="ml-auto font-mono text-[10px] text-shell-muted"
                        data-testid="task-elapsed"
                      >
                        {formatElapsed(displayElapsed(t))}
                      </span>
                    </div>
                    {(t.description || t.prompt) && (
                      <p className="mt-1 truncate text-[11px] text-shell-muted">
                        {t.description || t.prompt}
                      </p>
                    )}
                    {t.summary && (
                      <p className="mt-0.5 line-clamp-2 text-[11px] text-shell-text/80">
                        {t.summary}
                      </p>
                    )}
                    <div className="mt-1.5 flex gap-1">
                      {(t.status === 'running' || t.status === 'queued') && (
                        <button
                          type="button"
                          data-testid="task-cancel"
                          className="inline-flex items-center gap-1 rounded border border-shell-border px-1.5 py-0.5 text-[10px] text-shell-muted hover:bg-shell-bg hover:text-shell-text"
                          onClick={() => void onCancel(t.task_id)}
                          disabled={busy}
                        >
                          <Square size={10} /> Cancel
                        </button>
                      )}
                      {(t.status === 'cancelled' || t.status === 'failed') && (
                        <button
                          type="button"
                          data-testid="task-resume"
                          className="inline-flex items-center gap-1 rounded border border-shell-border px-1.5 py-0.5 text-[10px] text-shell-muted hover:bg-shell-bg hover:text-shell-text"
                          onClick={() => void onResume(t.task_id)}
                          disabled={busy}
                        >
                          <Play size={10} /> Resume
                        </button>
                      )}
                    </div>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
