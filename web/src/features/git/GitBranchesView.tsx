import { useCallback, useEffect, useMemo, useState } from 'react'
import { GitBranch, Plus, RefreshCw } from 'lucide-react'
import {
  gitCheckout,
  listGitBranches,
  type GitBranchInfo,
  type GitBranchList,
  type GitOpResult,
} from '../../lib/api'
import { SHELL } from '../shell/shellTokens'

type Props = {
  repoID: string
  refreshKey?: number
  onCheckedOut?: (res: GitOpResult) => void
}

const EMPTY: GitBranchList = { current: '', local: [], remote: [] }

export function GitBranchesView({ repoID, refreshKey, onCheckedOut }: Props) {
  const [data, setData] = useState<GitBranchList>(EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [newName, setNewName] = useState('')
  const [opBusy, setOpBusy] = useState(false)

  const load = useCallback(async () => {
    setBusy(true)
    try {
      const list = await listGitBranches(repoID)
      setData(list)
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

  const filter = (list: GitBranchInfo[]) => {
    const q = query.trim().toLowerCase()
    if (!q) return list
    return list.filter(
      (b) =>
        b.name.toLowerCase().includes(q) ||
        (b.subject || '').toLowerCase().includes(q),
    )
  }

  const local = useMemo(() => filter(data.local || []), [data.local, query])
  const remote = useMemo(() => filter(data.remote || []), [data.remote, query])

  const doCheckout = async (name: string, create = false) => {
    setOpBusy(true)
    setError(null)
    try {
      const res = await gitCheckout({ repo_id: repoID, name, create })
      if (!res.ok) {
        setError(res.detail || res.message)
      } else {
        await load()
        onCheckedOut?.(res)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setOpBusy(false)
    }
  }

  return (
    <div
      data-testid="git-branches-view"
      className="flex h-full min-h-0 flex-col overflow-hidden"
    >
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2">
        <GitBranch size={SHELL.iconXs} className="text-shell-accent" />
        <span className="text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
          Branches
        </span>
        <span className="truncate font-mono text-[11px] text-shell-text">
          {data.current || '—'}
        </span>
        <button
          type="button"
          className="ml-auto rounded p-1 text-shell-muted hover:bg-shell-border/40"
          onClick={() => void load()}
          disabled={busy}
          title="Refresh"
        >
          <RefreshCw size={SHELL.iconXs} className={busy ? 'animate-spin' : ''} />
        </button>
      </div>

      <div className="shrink-0 space-y-1.5 border-b border-shell-border p-2">
        <input
          data-testid="git-branch-filter"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Select a branch or tag to checkout"
          className="w-full rounded border border-shell-border bg-shell-bg px-2 py-1 text-[12px] text-shell-text placeholder:text-shell-muted focus:border-shell-accent focus:outline-none"
        />
        <div className="flex gap-1">
          <input
            data-testid="git-branch-new-name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="Create new branch…"
            className="min-w-0 flex-1 rounded border border-shell-border bg-shell-bg px-2 py-1 font-mono text-[11px] text-shell-text placeholder:text-shell-muted focus:border-shell-accent focus:outline-none"
          />
          <button
            type="button"
            data-testid="git-branch-create"
            disabled={opBusy || !newName.trim()}
            onClick={() => void doCheckout(newName.trim(), true)}
            className="inline-flex items-center gap-1 rounded border border-shell-border px-2 py-1 text-[11px] text-shell-text hover:bg-shell-border/30 disabled:opacity-40"
          >
            <Plus size={SHELL.iconXs} />
            Create
          </button>
        </div>
      </div>

      {error && (
        <div
          data-testid="git-branch-error"
          className="shrink-0 border-b border-rose-900/40 bg-rose-950/30 px-2 py-1 text-[11px] text-rose-200"
        >
          {error}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        <Section label="branches" />
        {local.length === 0 ? (
          <div className="px-3 py-2 text-[11px] text-shell-muted">No local branches</div>
        ) : (
          local.map((b) => (
            <BranchRow
              key={`l:${b.name}`}
              branch={b}
              disabled={opBusy || b.current}
              onCheckout={() => void doCheckout(b.name)}
            />
          ))
        )}
        <Section label="remote branches" />
        {remote.length === 0 ? (
          <div className="px-3 py-2 text-[11px] text-shell-muted">No remote branches</div>
        ) : (
          remote.map((b) => (
            <BranchRow
              key={`r:${b.name}`}
              branch={b}
              disabled={opBusy}
              onCheckout={() => void doCheckout(b.name)}
            />
          ))
        )}
      </div>
    </div>
  )
}

function Section({ label }: { label: string }) {
  return (
    <div className="sticky top-0 z-[1] bg-shell-panel/95 px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted backdrop-blur">
      {label}
    </div>
  )
}

function BranchRow({
  branch,
  disabled,
  onCheckout,
}: {
  branch: GitBranchInfo
  disabled?: boolean
  onCheckout: () => void
}) {
  return (
    <button
      type="button"
      data-testid={`git-branch-${branch.name}`}
      data-current={branch.current ? 'true' : 'false'}
      disabled={disabled}
      onClick={onCheckout}
      className={`flex w-full flex-col gap-0.5 border-b border-shell-border/30 px-2 py-1.5 text-left text-[11px] ${
        branch.current
          ? 'bg-shell-active/50'
          : 'hover:bg-shell-border/20 disabled:opacity-50'
      }`}
      title={branch.current ? 'Current branch' : `Checkout ${branch.name}`}
    >
      <div className="flex items-center gap-1.5">
        <GitBranch
          size={12}
          className={branch.remote ? 'text-sky-400' : 'text-shell-accent'}
        />
        <span className="truncate font-medium text-shell-text">{branch.name}</span>
        {branch.current && (
          <span className="rounded bg-shell-accent/20 px-1 text-[9px] text-shell-accent">
            current
          </span>
        )}
        <span className="ml-auto shrink-0 font-mono text-[10px] text-shell-muted">
          {branch.short_sha}
        </span>
      </div>
      {branch.subject && (
        <div className="truncate pl-4 text-[10px] text-shell-muted">
          {branch.subject}
        </div>
      )}
    </button>
  )
}
