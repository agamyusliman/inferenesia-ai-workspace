import { useCallback, useEffect, useState } from 'react'
import { History, RefreshCw } from 'lucide-react'
import {
  getAgentChangeHistory,
  type AgentChangeEntry,
  type AgentChangeHistory,
} from '../../lib/api'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'

type Props = {
  /** Bump to reload after undo/file change. */
  refreshKey?: number
  /** Optional: open a changed path in the editor. */
  onOpenPath?: (path: string) => void
  onClose?: () => void
}

const EMPTY: AgentChangeHistory = {
  title: 'Agent changes',
  subtitle: 'WriteGateway file mutations (pre-agent dirty stack) — not git log',
  empty_message: 'no agent changes in history (fresh workspace)',
  entries: [],
  count: 0,
  source: 'writegateway',
}

function kindLabel(kind: string): string {
  switch (kind) {
    case 'user_edit':
      return 'User edit'
    case 'mixed':
      return 'Mixed'
    default:
      return 'Agent turn'
  }
}

function kindClass(kind: string): string {
  switch (kind) {
    case 'user_edit':
      return 'border-sky-800/60 bg-sky-950/40 text-sky-200'
    case 'mixed':
      return 'border-violet-800/60 bg-violet-950/40 text-violet-200'
    default:
      return 'border-amber-800/60 bg-amber-950/40 text-amber-200'
  }
}

function formatTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export function AgentChangesTimeline({ refreshKey, onOpenPath, onClose }: Props) {
  const [data, setData] = useState<AgentChangeHistory>(EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setBusy(true)
    try {
      const hist = await getAgentChangeHistory()
      setData(hist)
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

  return (
    <section
      data-testid="agent-changes-timeline"
      data-source={data.source || 'writegateway'}
      data-count={data.count}
      aria-label="Agent changes timeline"
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader
        testId="agent-changes-header"
        title={data.title || 'Agent changes'}
        dense
        actions={
          <div className="flex items-center gap-1">
            <History size={SHELL.iconXs} aria-hidden className="text-shell-muted" />
            <button
              type="button"
              data-testid="agent-changes-refresh"
              title="Refresh timeline"
              disabled={busy}
              onClick={() => void refresh()}
              className="inline-flex items-center gap-1 rounded border border-shell-border px-1.5 py-0.5 text-[10px] text-shell-muted enabled:hover:bg-shell-active disabled:opacity-40"
            >
              <RefreshCw size={SHELL.iconXs} aria-hidden className={busy ? 'animate-spin' : ''} />
              Refresh
            </button>
            {onClose && (
              <button
                type="button"
                data-testid="agent-changes-close"
                onClick={onClose}
                className="rounded border border-shell-border px-1.5 py-0.5 text-[10px] text-shell-muted hover:bg-shell-active"
              >
                Close
              </button>
            )}
          </div>
        }
      />

      <div
        data-testid="agent-changes-subtitle"
        className="shrink-0 border-b border-shell-border px-3 py-1.5 text-[11px] text-shell-muted"
      >
        <p className="leading-snug">{data.subtitle}</p>
        <p className="mt-0.5 text-[10px] text-shell-muted/80">
          Distinct from raw git history — use the Git panel for commits. Undo/Revert agent restores
          pre-agent dirty snapshots via WriteGateway.
        </p>
      </div>

      {error && (
        <div
          data-testid="agent-changes-error"
          className="border-b border-red-900/50 bg-red-950/30 px-3 py-1 text-[11px] text-red-300"
        >
          {error}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto px-2 py-2">
        {data.count === 0 || !Array.isArray(data.entries) || data.entries.length === 0 ? (
          <div
            data-testid="agent-changes-empty"
            className="rounded border border-dashed border-shell-border bg-shell-panel/50 px-3 py-6 text-center text-[12px] text-shell-muted"
          >
            {data.empty_message || 'no agent changes in history (fresh workspace)'}
          </div>
        ) : (
          <ol
            data-testid="agent-changes-list"
            className="flex flex-col gap-2"
            aria-label="Agent change entries newest first"
          >
            {(data.entries ?? []).map((entry) => (
              <TimelineRow
                key={entry.unit_id || `${entry.index}-${entry.turn_id}`}
                entry={entry}
                onOpenPath={onOpenPath}
              />
            ))}
          </ol>
        )}
      </div>
    </section>
  )
}

function TimelineRow({
  entry,
  onOpenPath,
}: {
  entry: AgentChangeEntry
  onOpenPath?: (path: string) => void
}) {
  return (
    <li
      data-testid="agent-change-entry"
      data-kind={entry.kind}
      data-index={entry.index}
      className="rounded border border-shell-border bg-shell-panel px-2.5 py-2"
    >
      <div className="flex flex-wrap items-center gap-1.5">
        <span
          data-testid="agent-change-kind"
          className={`inline-flex shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${kindClass(entry.kind)}`}
        >
          {kindLabel(entry.kind)}
        </span>
        <span className="text-[11px] font-medium text-shell-text">#{entry.index}</span>
        <span className="text-[10px] text-shell-muted" data-testid="agent-change-time">
          {formatTime(entry.created_at)}
        </span>
        {entry.turn_id && (
          <span
            className="ml-auto truncate text-[10px] font-mono text-shell-muted"
            title={entry.turn_id}
          >
            {entry.turn_id.length > 16 ? entry.turn_id.slice(0, 16) + '…' : entry.turn_id}
          </span>
        )}
      </div>

      <p
        data-testid="agent-change-summary"
        className="mt-1 text-[11px] text-shell-text/90"
      >
        {entry.summary || `${entry.file_count} file(s)`}
      </p>

      {entry.sources?.length > 0 && (
        <p className="mt-0.5 text-[10px] text-shell-muted">
          sources: {entry.sources.join(', ')}
        </p>
      )}

      {entry.paths?.length > 0 && (
        <ul
          data-testid="agent-change-paths"
          className="mt-1.5 flex flex-col gap-0.5 border-t border-shell-border/60 pt-1.5"
        >
          {entry.paths.map((p) => (
            <li key={p} className="min-w-0">
              {onOpenPath ? (
                <button
                  type="button"
                  data-testid="agent-change-path"
                  title={`Open ${p}`}
                  onClick={() => onOpenPath(p)}
                  className="block w-full truncate rounded px-1 py-0.5 text-left font-mono text-[11px] text-shell-accent hover:bg-shell-active"
                >
                  {p}
                </button>
              ) : (
                <span
                  data-testid="agent-change-path"
                  className="block truncate font-mono text-[11px] text-shell-text/80"
                >
                  {p}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}
