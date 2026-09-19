import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import {
  getGitGraph,
  type GitGraphCommit,
  type GitGraphResult,
} from '../../lib/api'
import { SHELL } from '../shell/shellTokens'

type Props = {
  repoID: string
  refreshKey?: number
}

const EMPTY: GitGraphResult = {
  branch: '',
  commits: [],
  heads: [],
}

const LANE_COLORS = [
  '#60a5fa',
  '#34d399',
  '#f472b6',
  '#fbbf24',
  '#a78bfa',
  '#2dd4bf',
  '#fb7185',
  '#94a3b8',
]

function formatDate(iso?: string): string {
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

export function GitGraphView({ repoID, refreshKey }: Props) {
  const [data, setData] = useState<GitGraphResult>(EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<string | null>(null)

  const load = useCallback(async () => {
    setBusy(true)
    try {
      const g = await getGitGraph(repoID, 100)
      setData(g)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setData(EMPTY)
    } finally {
      setBusy(false)
    }
  }, [repoID])

  useEffect(() => {
    void load()
  }, [load, refreshKey])

  const maxLane = useMemo(() => {
    let m = 0
    for (const c of data.commits || []) {
      if (c.lane > m) m = c.lane
    }
    return Math.min(m, 12)
  }, [data.commits])

  const graphWidth = 12 + (maxLane + 1) * 14

  return (
    <div
      data-testid="git-graph-view"
      className="flex h-full min-h-0 flex-col overflow-hidden"
    >
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2">
        <span className="text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
          Graph
        </span>
        <span className="truncate text-[11px] text-shell-text">
          {data.branch || '—'} · {data.commits?.length || 0} commits
          {data.truncated ? ' (truncated)' : ''}
        </span>
        <button
          type="button"
          data-testid="git-graph-refresh"
          className="ml-auto rounded p-1 text-shell-muted hover:bg-shell-border/40"
          onClick={() => void load()}
          disabled={busy}
          title="Refresh graph"
        >
          <RefreshCw size={SHELL.iconXs} className={busy ? 'animate-spin' : ''} />
        </button>
      </div>
      {error && (
        <div className="border-b border-rose-900/40 bg-rose-950/30 px-2 py-1 text-[11px] text-rose-200">
          {error}
        </div>
      )}
      <div className="min-h-0 flex-1 overflow-auto">
        {(data.commits || []).length === 0 && !busy ? (
          <div className="p-4 text-[12px] text-shell-muted">No commits yet.</div>
        ) : (
          <ul className="m-0 list-none p-0" data-testid="git-graph-list">
            {(data.commits || []).map((c) => (
              <GraphRow
                key={c.sha}
                commit={c}
                maxLane={maxLane}
                graphWidth={graphWidth}
                selected={selected === c.sha}
                onSelect={() => setSelected(c.sha)}
              />
            ))}
          </ul>
        )}
      </div>
      {selected && (
        <CommitDetail
          commit={(data.commits || []).find((c) => c.sha === selected) || null}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  )
}

function GraphRow({
  commit,
  maxLane,
  graphWidth,
  selected,
  onSelect,
}: {
  commit: GitGraphCommit
  maxLane: number
  graphWidth: number
  selected: boolean
  onSelect: () => void
}) {
  const lane = Math.min(commit.lane, maxLane)
  const color = LANE_COLORS[lane % LANE_COLORS.length]
  return (
    <li>
      <button
        type="button"
        data-testid={`git-graph-commit-${commit.short}`}
        onClick={onSelect}
        className={`flex w-full items-stretch gap-0 border-b border-shell-border/30 text-left text-[11px] ${
          selected ? 'bg-shell-active/70' : 'hover:bg-shell-border/20'
        }`}
      >
        <svg
          width={graphWidth}
          height={36}
          className="shrink-0"
          aria-hidden
        >
          {/* vertical rails */}
          {Array.from({ length: maxLane + 1 }).map((_, i) => (
            <line
              key={i}
              x1={10 + i * 14}
              y1={0}
              x2={10 + i * 14}
              y2={36}
              stroke={LANE_COLORS[i % LANE_COLORS.length]}
              strokeOpacity={0.25}
              strokeWidth={1.5}
            />
          ))}
          <circle
            cx={10 + lane * 14}
            cy={18}
            r={4}
            fill={color}
            stroke="#0f172a"
            strokeWidth={1}
          />
        </svg>
        <div className="flex min-w-0 flex-1 flex-col justify-center gap-0.5 px-2 py-1">
          <div className="flex min-w-0 items-center gap-1.5">
            <span className="truncate font-medium text-shell-text">
              {commit.subject}
            </span>
            {(commit.refs || []).slice(0, 3).map((r) => (
              <span
                key={r}
                className="shrink-0 rounded border border-shell-accent/40 bg-shell-accent/10 px-1 font-mono text-[9px] text-shell-accent"
              >
                {r}
              </span>
            ))}
          </div>
          <div className="flex min-w-0 gap-2 font-mono text-[10px] text-shell-muted">
            <span className="text-shell-text/80">{commit.short}</span>
            <span className="truncate">{commit.author_name}</span>
            <span className="shrink-0">{formatDate(commit.author_date)}</span>
          </div>
        </div>
      </button>
    </li>
  )
}

function CommitDetail({
  commit,
  onClose,
}: {
  commit: GitGraphCommit | null
  onClose: () => void
}) {
  if (!commit) return null
  return (
    <div
      data-testid="git-graph-detail"
      className="max-h-36 shrink-0 overflow-auto border-t border-shell-border bg-shell-panel px-3 py-2 text-[11px]"
    >
      <div className="mb-1 flex items-center justify-between gap-2">
        <span className="font-mono text-shell-accent">{commit.short}</span>
        <button
          type="button"
          className="text-shell-muted hover:text-shell-text"
          onClick={onClose}
        >
          Close
        </button>
      </div>
      <div className="font-medium text-shell-text">{commit.subject}</div>
      <div className="mt-0.5 text-shell-muted">
        {commit.author_name}
        {commit.author_email ? ` <${commit.author_email}>` : ''} ·{' '}
        {formatDate(commit.author_date)}
      </div>
      {commit.body && (
        <pre className="mt-1 whitespace-pre-wrap font-mono text-[10px] text-shell-text/80">
          {commit.body}
        </pre>
      )}
    </div>
  )
}
