import { useState } from 'react'
import {
  Bot,
  CheckCircle2,
  Circle,
  Loader2,
  Square,
  XCircle,
  ChevronDown,
  ChevronRight,
} from 'lucide-react'
import { taskStatusLabel, type TaskBlock as TaskBlockModel } from './chatMetaBlocks'

type Props = {
  task: TaskBlockModel
}

/**
 * Distinct subagent/task progress block separate from primary chat prose (VAL-CHAT-015).
 */
export function TaskBlock({ task }: Props) {
  const [open, setOpen] = useState(false)
  const status = taskStatusLabel(task.status)
  const running = status === 'running' || status === 'queued'
  const failed = status === 'failed'
  const hasBody = Boolean(task.detail || task.label)

  return (
    <div
      data-testid="chat-task-block"
      data-task-id={task.id}
      data-task-status={status}
      data-task-category={task.category || undefined}
      className={`overflow-hidden rounded-md border text-[11px] ${
        failed
          ? 'border-rose-800/50 bg-rose-950/20'
          : running
            ? 'border-amber-800/40 bg-amber-950/15'
            : status === 'completed'
              ? 'border-emerald-800/40 bg-emerald-950/15'
              : 'border-shell-border bg-shell-panel/60'
      }`}
    >
      <button
        type="button"
        data-testid="chat-task-header"
        disabled={!hasBody}
        onClick={() => hasBody && setOpen((v) => !v)}
        className="flex w-full items-center gap-1.5 px-2 py-1.5 text-left transition hover:bg-shell-border/20 disabled:cursor-default"
      >
        {hasBody ? (
          open ? (
            <ChevronDown size={12} className="shrink-0 text-shell-muted" aria-hidden />
          ) : (
            <ChevronRight size={12} className="shrink-0 text-shell-muted" aria-hidden />
          )
        ) : (
          <Bot size={12} className="shrink-0 text-shell-muted" aria-hidden />
        )}
        <TaskStatusIcon status={status} />
        <span className="text-[9px] font-semibold uppercase tracking-wide text-shell-muted">
          Subagent
        </span>
        {task.category && (
          <span
            data-testid="chat-task-category"
            className="rounded bg-shell-bg px-1 py-0.5 font-mono text-[9px] text-shell-accent"
          >
            {task.category}
          </span>
        )}
        <span data-testid="chat-task-id" className="min-w-0 truncate font-mono text-shell-text">
          {task.id}
        </span>
        <span
          data-testid="chat-task-status"
          className={`ml-auto shrink-0 rounded px-1 py-0.5 text-[9px] font-semibold uppercase tracking-wide ${
            failed
              ? 'bg-rose-900/40 text-rose-200'
              : running
                ? 'bg-amber-900/40 text-amber-100'
                : status === 'completed'
                  ? 'bg-emerald-900/30 text-emerald-200'
                  : 'bg-shell-border/40 text-shell-muted'
          }`}
        >
          {status || 'task'}
        </span>
      </button>
      {open && hasBody && (
        <div
          data-testid="chat-task-body"
          className="space-y-0.5 border-t border-shell-border/50 px-2.5 py-1.5 text-[10px] text-shell-muted"
        >
          {task.label && (
            <div data-testid="chat-task-label" className="text-shell-text/90">
              {task.label}
            </div>
          )}
          {task.detail && (
            <div data-testid="chat-task-detail" className="font-mono break-all">
              {task.detail}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function TaskStatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return <CheckCircle2 size={12} className="shrink-0 text-emerald-400" aria-hidden />
    case 'failed':
      return <XCircle size={12} className="shrink-0 text-rose-400" aria-hidden />
    case 'cancelled':
      return <Square size={12} className="shrink-0 text-zinc-400" aria-hidden />
    case 'running':
    case 'queued':
      return <Loader2 size={12} className="shrink-0 animate-spin text-amber-300" aria-hidden />
    default:
      return <Circle size={12} className="shrink-0 text-shell-muted" aria-hidden />
  }
}
