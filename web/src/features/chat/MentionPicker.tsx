import type { FileMention } from '../../lib/api'

type Props = {
  items: FileMention[]
  loading: boolean
  query: string
  onSelect: (file: FileMention) => void
  onClose: () => void
}

export function MentionPicker({ items, loading, query, onSelect, onClose }: Props) {
  return (
    <div
      data-testid="mention-picker"
      className="absolute bottom-full left-2 right-2 z-20 mb-1 max-h-48 overflow-auto rounded border border-shell-border bg-shell-panel shadow-lg"
      role="listbox"
      aria-label="File mention picker"
    >
      <div className="flex items-center justify-between border-b border-shell-border px-2 py-1">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
          @file {query ? `· ${query}` : ''}
        </span>
        <button
          type="button"
          data-testid="mention-picker-close"
          onClick={onClose}
          className="text-[10px] text-shell-muted hover:text-shell-text"
        >
          Esc
        </button>
      </div>
      {loading && (
        <div className="px-2 py-2 text-[11px] text-shell-muted">Loading workspace files…</div>
      )}
      {!loading && items.length === 0 && (
        <div data-testid="mention-picker-empty" className="px-2 py-2 text-[11px] text-shell-muted">
          No files match
        </div>
      )}
      <ul className="py-0.5">
        {items.map((f) => (
          <li key={f.path}>
            <button
              type="button"
              data-testid="mention-item"
              data-path={f.path}
              role="option"
              onClick={() => onSelect(f)}
              className="flex w-full items-center gap-2 px-2 py-1 text-left text-[11px] text-shell-text hover:bg-shell-active"
            >
              <span className="shrink-0 font-mono text-shell-accent">@{f.name}</span>
              <span className="truncate text-shell-muted">{f.path}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
