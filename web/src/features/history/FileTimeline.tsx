import { useCallback, useEffect, useState } from 'react'
import {
  ChevronDown,
  ChevronUp,
  FileText,
  GitCommitHorizontal,
  History,
  RefreshCw,
} from 'lucide-react'
import {
  getFileTimeline,
  getFileTimelineChange,
  type FileTimeline as FileTimelineData,
  type FileTimelineChange,
  type FileTimelineEntry,
} from '../../lib/api'
import { SHELL } from '../shell/shellTokens'

export type TimelineDiffRequest = {
  path: string
  entry: FileTimelineEntry
  change: FileTimelineChange
  entries: FileTimelineEntry[]
  index: number
}

type Props = {
  path?: string
  activeEntryId?: string | null
  refreshKey?: number
  onOpenPath?: (path: string) => void
  onOpenDiff?: (req: TimelineDiffRequest) => void
}

const EMPTY: FileTimelineData = {
  path: '',
  entries: [],
  count: 0,
  local_count: 0,
  git_count: 0,
  empty_message: 'select a file to see local and git history',
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
  })
}

function baseName(path: string): string {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

function sourceBadge(source: string): string {
  if (source === 'git') return 'git'
  if (source === 'local') return 'local'
  return source
}

export function FileTimeline({
  path,
  activeEntryId = null,
  refreshKey,
  onOpenPath,
  onOpenDiff,
}: Props) {
  const [data, setData] = useState<FileTimelineData>(EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<'all' | 'local' | 'git'>('all')
  const [activeId, setActiveId] = useState<string | null>(null)
  const [loadingId, setLoadingId] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!path) {
      setData(EMPTY)
      setError(null)
      return
    }
    setBusy(true)
    try {
      const tl = await getFileTimeline(path)
      setData(tl)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setData({ ...EMPTY, path })
    } finally {
      setBusy(false)
    }
  }, [path])

  useEffect(() => {
    void refresh()
  }, [refresh, refreshKey])

  useEffect(() => {
    if (activeEntryId) setActiveId(activeEntryId)
  }, [activeEntryId])

  const entries = (data.entries || []).filter((e) => {
    if (filter === 'all') return true
    return e.source === filter
  })

  const openChange = useCallback(
    async (entry: FileTimelineEntry, index: number) => {
      if (!path || !onOpenDiff) return
      setLoadingId(entry.id)
      setActiveId(entry.id)
      setError(null)
      try {
        const change = await getFileTimelineChange({
          path,
          source: entry.source,
          unit_id: entry.unit_id,
          sha: entry.sha,
        })
        onOpenDiff({ path, entry, change, entries, index })
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      } finally {
        setLoadingId(null)
      }
    },
    [path, onOpenDiff, entries],
  )

  const activeIndex = entries.findIndex((e) => e.id === activeId)

  const goRelative = async (delta: number) => {
    if (!entries.length) return
    let idx = activeIndex
    if (idx < 0) idx = 0
    else idx = idx + delta
    if (idx < 0 || idx >= entries.length) return
    await openChange(entries[idx], idx)
  }

  const title = path ? baseName(path) : 'Timeline'

  return (
    <section
      data-testid="file-timeline"
      data-path={path || ''}
      data-count={data.count}
      className="flex h-full min-h-0 flex-col overflow-hidden bg-shell-panel"
    >
      <div className="flex h-7 shrink-0 items-center gap-1 border-b border-shell-border px-1.5">
        <History size={SHELL.iconXs} className="shrink-0 text-shell-muted" aria-hidden />
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-shell-text">
          Timeline
          {path ? (
            <span className="ml-1 font-normal text-shell-muted">{title}</span>
          ) : null}
        </span>
        <div className="flex shrink-0 items-center gap-0.5">
          <button
            type="button"
            data-testid="timeline-prev"
            disabled={!path || entries.length === 0 || activeIndex <= 0}
            onClick={() => void goRelative(1)}
            className="rounded p-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-30"
            title="Older change"
          >
            <ChevronDown size={12} />
          </button>
          <button
            type="button"
            data-testid="timeline-next"
            disabled={
              !path ||
              entries.length === 0 ||
              activeIndex < 0 ||
              activeIndex >= entries.length - 1
            }
            onClick={() => void goRelative(-1)}
            className="rounded p-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-30"
            title="Newer change"
          >
            <ChevronUp size={12} />
          </button>
          <button
            type="button"
            data-testid="timeline-open-file"
            disabled={!path}
            onClick={() => path && onOpenPath?.(path)}
            className="rounded p-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-30"
            title="Open file"
          >
            <FileText size={12} />
          </button>
          {(['all', 'local', 'git'] as const).map((f) => (
            <button
              key={f}
              type="button"
              data-testid={`timeline-filter-${f}`}
              onClick={() => setFilter(f)}
              className={`rounded px-1 py-0.5 text-[9px] uppercase tracking-wide ${
                filter === f
                  ? 'bg-shell-active text-shell-text'
                  : 'text-shell-muted hover:bg-shell-border/40'
              }`}
            >
              {f}
            </button>
          ))}
          <button
            type="button"
            data-testid="timeline-refresh"
            disabled={busy || !path}
            onClick={() => void refresh()}
            className="rounded p-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
            title="Refresh"
          >
            <RefreshCw size={12} className={busy ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>

      {error && (
        <div className="border-b border-red-900/50 px-2 py-0.5 text-[10px] text-red-300">
          {error}
        </div>
      )}

      <ul
        data-testid="file-timeline-list"
        className="min-h-0 flex-1 overflow-y-auto px-1 py-1"
      >
        {!path && (
          <li className="px-2 py-3 text-center text-[11px] text-shell-muted">
            Select a file in the explorer to see local agent edits and git commits.
          </li>
        )}
        {path && entries.length === 0 && (
          <li className="px-2 py-3 text-center text-[11px] text-shell-muted">
            {data.empty_message || data.message || 'No history for this file'}
          </li>
        )}
        {entries.map((entry, index) => (
          <TimelineRow
            key={entry.id}
            entry={entry}
            active={activeId === entry.id}
            loading={loadingId === entry.id}
            onOpen={() => void openChange(entry, index)}
            onOpenFile={() => path && onOpenPath?.(path)}
          />
        ))}
      </ul>

      {path && data.count > 0 && (
        <div className="shrink-0 border-t border-shell-border px-2 py-0.5 text-[9px] text-shell-muted">
          local {data.local_count} · git {data.git_count}
          {activeIndex >= 0 ? ` · ${activeIndex + 1}/${entries.length}` : ''}
        </div>
      )}
    </section>
  )
}

function TimelineRow({
  entry,
  active,
  loading,
  onOpen,
  onOpenFile,
}: {
  entry: FileTimelineEntry
  active: boolean
  loading: boolean
  onOpen: () => void
  onOpenFile: () => void
}) {
  const isGit = entry.source === 'git'
  return (
    <li className="mb-0.5">
      <div
        data-testid={`timeline-entry-${entry.id}`}
        data-source={entry.source}
        data-active={active ? 'true' : 'false'}
        className={`flex w-full min-w-0 items-start gap-1 rounded-md px-1 py-1 text-[11px] transition ${
          active
            ? 'bg-shell-active text-shell-text'
            : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
        }`}
      >
        <button
          type="button"
          className="flex min-w-0 flex-1 items-start gap-1.5 text-left"
          onClick={onOpen}
          title="Open diff"
        >
          <span
            className={`mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full text-[8px] font-bold ${
              isGit
                ? 'bg-sky-950/60 text-sky-300'
                : 'bg-amber-950/60 text-amber-300'
            }`}
          >
            {isGit ? <GitCommitHorizontal size={10} aria-hidden /> : 'L'}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium text-shell-text">
              {loading ? 'Loading…' : entry.summary}
            </span>
            <span className="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[9px] opacity-80">
              <span className="uppercase tracking-wide">
                {sourceBadge(entry.source)}
              </span>
              {entry.author ? <span>· {entry.author}</span> : null}
              {entry.created_at ? (
                <span>· {formatTime(entry.created_at)}</span>
              ) : null}
              {entry.short_sha ? (
                <span className="font-mono text-shell-accent/90">
                  {entry.short_sha}
                </span>
              ) : null}
            </span>
          </span>
        </button>
        <button
          type="button"
          data-testid={`timeline-open-file-${entry.id}`}
          title="Open file"
          onClick={(e) => {
            e.stopPropagation()
            onOpenFile()
          }}
          className="mt-0.5 shrink-0 rounded p-0.5 opacity-60 hover:bg-shell-border/40 hover:opacity-100"
        >
          <FileText size={11} />
        </button>
      </div>
    </li>
  )
}
