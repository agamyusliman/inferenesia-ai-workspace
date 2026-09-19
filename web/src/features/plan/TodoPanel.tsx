import { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, Circle, Loader2, RefreshCw, X } from 'lucide-react'
import {
  getTodos,
  type TodoItem,
  type TodoListView,
} from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'

type Props = {
  /** Bump after PlanProposed / TodoUpdated / approve to reload. */
  refreshKey?: number
  onClose?: () => void
}

const EMPTY: TodoListView = {
  todos: [],
  count: 0,
}

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return (
        <CheckCircle2
          size={SHELL.iconSm}
          className="shrink-0 text-emerald-400"
          aria-hidden
        />
      )
    case 'in_progress':
      return (
        <Loader2
          size={SHELL.iconSm}
          className="shrink-0 animate-spin text-amber-300"
          aria-hidden
        />
      )
    case 'cancelled':
      return (
        <Circle
          size={SHELL.iconSm}
          className="shrink-0 text-shell-muted line-through opacity-50"
          aria-hidden
        />
      )
    default:
      return (
        <Circle
          size={SHELL.iconSm}
          className="shrink-0 text-shell-muted"
          aria-hidden
        />
      )
  }
}

function statusClass(status: string): string {
  switch (status) {
    case 'completed':
      return 'text-emerald-200/90 line-through decoration-emerald-700/80'
    case 'in_progress':
      return 'text-amber-100 font-medium'
    case 'cancelled':
      return 'text-shell-muted line-through opacity-60'
    default:
      return 'text-shell-text'
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'in_progress':
      return 'in progress'
    case 'completed':
      return 'completed'
    case 'cancelled':
      return 'cancelled'
    default:
      return 'pending'
  }
}

export function TodoPanel({ refreshKey, onClose }: Props) {
  const { t: tr } = useLocale()
  const [data, setData] = useState<TodoListView>(EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setBusy(true)
    try {
      const list = await getTodos()
      setData(list)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setData(EMPTY)
    } finally {
      setBusy(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh, refreshKey])

  const todos: TodoItem[] = data.todos || []
  const pending = todos.filter((t) => t.status === 'pending').length
  const active = todos.filter((t) => t.status === 'in_progress').length
  const done = todos.filter((t) => t.status === 'completed').length

  return (
    <section
      data-testid="todo-panel"
      data-count={data.count ?? todos.length}
      data-pending={pending}
      data-in-progress={active}
      data-completed={done}
      aria-label={tr('plan.title')}
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader
        testId="todo-panel-header"
        title={tr('plan.title')}
        dense
        actions={
          <div className="flex shrink-0 items-center gap-0.5">
            <button
              type="button"
              data-testid="todo-refresh"
              title={tr('action.refresh')}
              aria-label={tr('action.refresh')}
              onClick={() => void refresh()}
              disabled={busy}
              className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
            >
              <RefreshCw size={SHELL.iconXs} className={busy ? 'animate-spin' : ''} />
            </button>
            {onClose && (
              <button
                type="button"
                data-testid="todo-close"
                title={tr('panel.closeTitle')}
                aria-label={tr('panel.closeTitle')}
                onClick={onClose}
                className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
              >
                <X size={SHELL.iconXs} aria-hidden />
              </button>
            )}
          </div>
        }
      />

      {(pending + active + done > 0) && (
        <div
          data-testid="todo-summary"
          className="flex shrink-0 flex-wrap items-center gap-x-2 gap-y-0.5 border-b border-shell-border px-2 py-1 text-[10px] text-shell-muted"
        >
          <span data-testid="todo-summary-pending">{pending} pending</span>
          <span data-testid="todo-summary-active">{active} active</span>
          <span data-testid="todo-summary-done">{done} done</span>
        </div>
      )}

      {error && (
        <div
          data-testid="todo-error"
          className="shrink-0 border-b border-rose-900/50 bg-rose-950/40 px-2 py-1 text-[11px] text-rose-200"
        >
          {error}
        </div>
      )}

      <div className="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-1 py-1">
        {todos.length === 0 ? (
          <div
            data-testid="todo-empty"
            className="px-2 py-4 text-center text-[11px] leading-snug text-shell-muted"
          >
            No todos yet. Approve a plan to seed this checklist.
          </div>
        ) : (
          <ul data-testid="todo-list" className="flex flex-col gap-0.5">
            {todos.map((t) => (
              <li
                key={t.id}
                data-testid="todo-item"
                data-todo-id={t.id}
                data-status={t.status}
                className="flex min-w-0 items-start gap-1.5 rounded-md px-1.5 py-1 hover:bg-shell-panel/60"
              >
                <StatusIcon status={t.status} />
                <div className="min-w-0 flex-1 overflow-hidden">
                  <div
                    data-testid="todo-content"
                    className={`break-words text-[11px] leading-snug ${statusClass(t.status)}`}
                    title={t.content}
                  >
                    {t.content}
                  </div>
                  <div
                    data-testid="todo-status-label"
                    className="mt-0.5 text-[9px] uppercase tracking-wide text-shell-muted"
                  >
                    {statusLabel(t.status)}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}
