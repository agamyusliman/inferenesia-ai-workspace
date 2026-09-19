import { useEffect, useState } from 'react'
import {
  ChevronDown,
  ChevronRight,
  Loader2,
  CheckCircle2,
  XCircle,
  Wrench,
} from 'lucide-react'
import {
  groupToolBlocks,
  toolStatusLabel,
  toolsActivityStatus,
  truncateResult,
  type ToolCallBlock as ToolCallBlockModel,
  type ToolCallGroup,
} from './chatMetaBlocks'

type Props = {
  tool: ToolCallBlockModel
}

export function ToolCallBlock({ tool }: Props) {
  return <ToolCallRow tool={tool} />
}

export function ToolCallBlocks({
  tools,
  streaming,
}: {
  tools: ToolCallBlockModel[]
  streaming?: boolean
}) {
  const groups = groupToolBlocks(tools)
  if (groups.length === 0) return null

  const total = tools.length
  const runN = tools.filter((t) => t.status === 'running').length
  const errN = tools.filter((t) => t.status === 'error').length
  const activity = toolsActivityStatus({ tools, streaming })
  const names = groups.map((g) =>
    g.tools.length > 1 ? `${g.name}×${g.tools.length}` : g.name,
  )
  const headline =
    activity.kind === 'running'
      ? `${total} tool${total === 1 ? '' : 's'} · ${runN} running`
      : activity.kind === 'active'
        ? `${total} tool${total === 1 ? '' : 's'}`
        : activity.kind === 'error'
          ? `${total} tool${total === 1 ? '' : 's'} · ${errN} error`
          : `${total} tool${total === 1 ? '' : 's'}`

  return (
    <div data-testid="chat-tool-blocks" className="mb-2">
      <ToolsActivityCard
        headline={headline}
        names={names.join(' · ')}
        activity={activity}
        groups={groups}
      />
    </div>
  )
}

function ToolsActivityCard({
  headline,
  names,
  activity,
  groups,
}: {
  headline: string
  names: string
  activity: ReturnType<typeof toolsActivityStatus>
  groups: ToolCallGroup[]
}) {
  const live = activity.kind === 'running' || activity.kind === 'active'
  const hasError = activity.kind === 'error'
  const [open, setOpen] = useState(live || hasError)

  useEffect(() => {
    if (live || hasError) setOpen(true)
  }, [live, hasError])

  return (
    <div
      data-testid="chat-tools-activity"
      data-activity-status={activity.kind}
      className={`min-w-0 max-w-full overflow-hidden rounded-md border text-[11px] ${
        hasError
          ? 'border-rose-800/40 bg-rose-950/15'
          : live
            ? 'border-sky-800/35 bg-sky-950/15'
            : 'border-shell-border/70 bg-shell-panel/40'
      }`}
    >
      <button
        type="button"
        data-testid="chat-tools-activity-header"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full min-w-0 items-center gap-1.5 px-2 py-1 text-left transition hover:bg-shell-border/15"
      >
        {open ? (
          <ChevronDown size={12} className="shrink-0 text-shell-muted" aria-hidden />
        ) : (
          <ChevronRight size={12} className="shrink-0 text-shell-muted" aria-hidden />
        )}
        {activity.kind === 'running' ? (
          <Loader2 size={12} className="shrink-0 animate-spin text-sky-300" aria-hidden />
        ) : activity.kind === 'active' ? (
          <Loader2 size={12} className="shrink-0 animate-spin text-sky-300/80" aria-hidden />
        ) : hasError ? (
          <XCircle size={12} className="shrink-0 text-rose-400" aria-hidden />
        ) : (
          <Wrench size={12} className="shrink-0 text-shell-muted" aria-hidden />
        )}
        <span className="min-w-0 flex-1 truncate text-shell-text">
          <span className="font-medium">{headline}</span>
          {names && (
            <span className="ml-1.5 font-mono text-[10px] text-shell-muted">{names}</span>
          )}
        </span>
        <span
          data-testid="chat-tools-activity-status"
          className={`shrink-0 text-[9px] font-semibold uppercase tracking-wide ${
            hasError
              ? 'text-rose-300/90'
              : live
                ? 'text-sky-300/90'
                : 'text-emerald-300/80'
          }`}
        >
          {activity.label}
        </span>
      </button>
      {open && (
        <div
          data-testid="chat-tools-activity-body"
          className="space-y-0.5 border-t border-shell-border/50 px-1.5 py-1"
        >
          {groups.map((g) =>
            g.tools.length === 1 ? (
              <ToolCallRow key={g.key} tool={g.tools[0]} compact />
            ) : (
              <ToolCallGroupBlock key={g.key} group={g} />
            ),
          )}
        </div>
      )}
    </div>
  )
}

function ToolCallGroupBlock({ group }: { group: ToolCallGroup }) {
  const live = group.status === 'running'
  const [open, setOpen] = useState(live)
  const n = group.tools.length
  const status = toolStatusLabel(group.status)

  useEffect(() => {
    if (live) setOpen(true)
  }, [live])

  return (
    <div
      data-testid="chat-tool-group"
      data-tool-name={group.name}
      data-tool-count={n}
      data-tool-status={group.status}
      className="min-w-0 max-w-full overflow-hidden rounded border border-shell-border/50 bg-shell-bg/40"
    >
      <button
        type="button"
        data-testid="chat-tool-group-header"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full min-w-0 items-center gap-1 px-1.5 py-0.5 text-left text-[11px] transition hover:bg-shell-border/15"
      >
        {open ? (
          <ChevronDown size={11} className="shrink-0 text-shell-muted" aria-hidden />
        ) : (
          <ChevronRight size={11} className="shrink-0 text-shell-muted" aria-hidden />
        )}
        <StatusIcon status={group.status} />
        <span className="min-w-0 flex-1 truncate font-mono text-shell-text">
          {group.name}
          <span className="ml-1 text-[10px] text-shell-muted">×{n}</span>
        </span>
        <span
          className={`shrink-0 text-[9px] font-semibold uppercase tracking-wide ${
            group.status === 'error'
              ? 'text-rose-300/90'
              : group.status === 'running'
                ? 'text-sky-300/90'
                : 'text-emerald-300/80'
          }`}
        >
          {status}
        </span>
      </button>
      {open && (
        <div data-testid="chat-tool-group-body" className="space-y-0.5 px-1 pb-1">
          {group.tools.map((t, i) => (
            <ToolCallRow key={t.id} tool={t} index={i + 1} nested compact />
          ))}
        </div>
      )}
    </div>
  )
}

function ToolCallRow({
  tool,
  index,
  nested,
  compact,
}: {
  tool: ToolCallBlockModel
  index?: number
  nested?: boolean
  compact?: boolean
}) {
  const running = tool.status === 'running'
  const err = tool.status === 'error'
  const [open, setOpen] = useState(tool.status === 'error')
  const status = toolStatusLabel(tool.status)
  const result = truncateResult(tool.result, 320)
  const hasBody = Boolean(tool.detail || result)
  const label = nested
    ? detailPreview(tool) || tool.name
    : tool.detail
      ? `${tool.name}  ${shortDetail(tool.detail)}`
      : tool.name

  useEffect(() => {
    if (err) setOpen(true)
  }, [err])

  return (
    <div
      data-testid="chat-tool-block"
      data-tool-name={tool.name}
      data-tool-status={tool.status}
      data-nested={nested ? 'true' : undefined}
      className={`min-w-0 max-w-full overflow-hidden rounded ${
        compact
          ? 'bg-transparent'
          : err
            ? 'border border-rose-800/40 bg-rose-950/15'
            : 'border border-shell-border/50 bg-shell-bg/30'
      }`}
    >
      <button
        type="button"
        data-testid="chat-tool-header"
        disabled={!hasBody}
        onClick={() => hasBody && setOpen((v) => !v)}
        className="flex w-full min-w-0 items-center gap-1 px-1.5 py-0.5 text-left text-[11px] transition hover:bg-shell-border/15 disabled:cursor-default"
      >
        {hasBody ? (
          open ? (
            <ChevronDown size={11} className="shrink-0 text-shell-muted" aria-hidden />
          ) : (
            <ChevronRight size={11} className="shrink-0 text-shell-muted" aria-hidden />
          )
        ) : (
          <Wrench size={11} className="shrink-0 text-shell-muted" aria-hidden />
        )}
        <StatusIcon status={tool.status} />
        {typeof index === 'number' && (
          <span className="shrink-0 font-mono text-[9px] text-shell-muted">#{index}</span>
        )}
        <span
          data-testid="chat-tool-name"
          className="min-w-0 flex-1 truncate font-mono text-shell-text"
          title={tool.detail || tool.name}
        >
          {label}
        </span>
        <span
          data-testid="chat-tool-status"
          className={`shrink-0 text-[9px] font-semibold uppercase tracking-wide ${
            err
              ? 'text-rose-300/90'
              : running
                ? 'text-sky-300/90'
                : 'text-emerald-300/80'
          }`}
        >
          {status}
        </span>
      </button>
      {open && hasBody && (
        <div
          data-testid="chat-tool-body"
          className="space-y-0.5 border-t border-shell-border/40 px-2 py-1 font-mono text-[10px] text-shell-muted"
        >
          {tool.detail && (
            <div data-testid="chat-tool-detail" className="break-all">
              {tool.detail}
            </div>
          )}
          {result && (
            <pre
              data-testid="chat-tool-result"
              className="max-h-32 overflow-auto whitespace-pre-wrap break-words rounded bg-shell-bg/70 p-1.5 text-shell-text/90"
            >
              {result}
            </pre>
          )}
        </div>
      )}
    </div>
  )
}

function shortDetail(detail: string): string {
  const d = detail.trim()
  if (d.length <= 42) return d
  return `${d.slice(0, 42)}…`
}

function detailPreview(tool: ToolCallBlockModel): string {
  const d = (tool.detail || '').trim()
  if (!d) return ''
  return d.length > 48 ? `${d.slice(0, 48)}…` : d
}

function StatusIcon({ status }: { status: string }) {
  if (status === 'running') {
    return <Loader2 size={11} className="shrink-0 animate-spin text-sky-300" aria-hidden />
  }
  if (status === 'error') {
    return <XCircle size={11} className="shrink-0 text-rose-400" aria-hidden />
  }
  return <CheckCircle2 size={11} className="shrink-0 text-emerald-400" aria-hidden />
}
