import { useCallback, useEffect, useRef, useState } from 'react'
import { Search, X } from 'lucide-react'
import { findInFolder, type FindHit } from '../../lib/api'

type Props = {
  workspaceId: string
  /** Workspace-relative folder path (empty = workspace root). */
  folder: string
  folderLabel?: string
  onOpenResult: (path: string) => void
  onClose: () => void
}

/**
 * Scoped search UI for "Find in Folder" (VAL-IDE-008).
 * Results are limited to paths under the selected folder by the core API.
 */
export function FindInFolderPanel({
  workspaceId,
  folder,
  folderLabel,
  onOpenResult,
  onClose,
}: Props) {
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<FindHit[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const reqId = useRef(0)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const runSearch = useCallback(
    async (q: string) => {
      const id = ++reqId.current
      const trimmed = q.trim()
      if (!trimmed) {
        setHits([])
        setLoading(false)
        setError(null)
        return
      }
      setLoading(true)
      setError(null)
      try {
        const res = await findInFolder(workspaceId, folder, trimmed, 200)
        if (id !== reqId.current) return
        // Defensive filter: never show paths outside folder (negative test for UI).
        const scoped = res.filter((h) => underFolder(h.path, folder))
        setHits(scoped)
      } catch (e) {
        if (id !== reqId.current) return
        setError(e instanceof Error ? e.message : String(e))
        setHits([])
      } finally {
        if (id === reqId.current) setLoading(false)
      }
    },
    [folder, workspaceId],
  )

  // Debounce query
  useEffect(() => {
    const t = setTimeout(() => {
      void runSearch(query)
    }, 250)
    return () => clearTimeout(t)
  }, [query, runSearch])

  const scopeLabel = folderLabel || folder || 'workspace root'

  return (
    <div
      data-testid="find-in-folder-panel"
      data-folder={folder}
      className="flex max-h-[45%] min-h-[8rem] shrink-0 flex-col border-t border-shell-border bg-shell-panel"
    >
      <div className="flex shrink-0 items-center gap-1 border-b border-shell-border px-2 py-1">
        <Search size={12} className="shrink-0 text-shell-muted" aria-hidden />
        <span
          className="min-w-0 flex-1 truncate text-[10px] font-medium uppercase tracking-wide text-shell-muted"
          title={folder || '.'}
        >
          Find in {scopeLabel}
        </span>
        <button
          type="button"
          data-testid="find-in-folder-close"
          aria-label="Close find in folder"
          className="rounded p-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
          onClick={onClose}
        >
          <X size={12} aria-hidden />
        </button>
      </div>
      <div className="shrink-0 px-2 py-1">
        <input
          ref={inputRef}
          data-testid="find-in-folder-input"
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search path or content…"
          className="w-full rounded border border-shell-border bg-shell-bg px-1.5 py-1 text-[11px] text-shell-text outline-none placeholder:text-shell-muted/70 focus:border-shell-accent"
        />
      </div>
      <div
        data-testid="find-in-folder-results"
        className="min-h-0 flex-1 overflow-auto px-1 pb-1"
      >
        {loading && (
          <p className="px-1.5 py-1 text-[11px] text-shell-muted">Searching…</p>
        )}
        {error && (
          <p className="px-1.5 py-1 text-[11px] text-red-400" data-testid="find-in-folder-error">
            {error}
          </p>
        )}
        {!loading && !error && query.trim() && hits.length === 0 && (
          <p className="px-1.5 py-1 text-[11px] text-shell-muted">No results</p>
        )}
        {!loading &&
          hits.map((h, i) => (
            <button
              key={`${h.path}:${h.line ?? 0}:${h.kind}:${i}`}
              type="button"
              data-testid={`find-hit-${h.path}`}
              data-path={h.path}
              data-kind={h.kind}
              title={h.text || h.path}
              onClick={() => onOpenResult(h.path)}
              className="flex w-full flex-col gap-0.5 rounded px-1.5 py-1 text-left text-[11px] text-shell-muted hover:bg-shell-border/30 hover:text-shell-text"
            >
              <span className="truncate font-medium text-shell-text/90">
                {h.path}
                {h.line ? `:${h.line}` : ''}
              </span>
              {h.text && (
                <span className="truncate text-[10px] text-shell-muted/90">{h.text}</span>
              )}
            </button>
          ))}
      </div>
    </div>
  )
}

function underFolder(rel: string, folder: string): boolean {
  const r = rel.replace(/\\/g, '/').replace(/^\.\//, '')
  const f = folder.replace(/\\/g, '/').replace(/\/+$/, '')
  if (!f) return r !== ''
  return r === f || r.startsWith(f + '/')
}
