import type { SlashCommand } from './slashCommands'

type Props = {
  items: SlashCommand[]
  query: string
  activeIndex: number
  onSelect: (cmd: SlashCommand) => void
  onClose: () => void
  onHoverIndex?: (i: number) => void
}

export function SlashCommandPicker({
  items,
  query,
  activeIndex,
  onSelect,
  onClose,
  onHoverIndex,
}: Props) {
  return (
    <div
      data-testid="slash-picker"
      className="absolute bottom-full left-2 right-2 z-20 mb-1 max-h-56 overflow-auto rounded border border-shell-border bg-shell-panel shadow-lg"
      role="listbox"
      aria-label="Slash command picker"
    >
      <div className="flex items-center justify-between border-b border-shell-border px-2 py-1">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
          /commands {query ? `· ${query}` : ''}
        </span>
        <button
          type="button"
          data-testid="slash-picker-close"
          onClick={onClose}
          className="text-[10px] text-shell-muted hover:text-shell-text"
        >
          Esc
        </button>
      </div>
      {items.length === 0 && (
        <div data-testid="slash-picker-empty" className="px-2 py-2 text-[11px] text-shell-muted">
          No matching tools or commands
        </div>
      )}
      <ul className="py-0.5">
        {items.map((cmd, i) => {
          const active = i === activeIndex
          return (
            <li key={cmd.id}>
              <button
                type="button"
                data-testid="slash-item"
                data-slash-id={cmd.id}
                data-slash-kind={cmd.kind}
                role="option"
                aria-selected={active}
                onMouseEnter={() => onHoverIndex?.(i)}
                onClick={() => onSelect(cmd)}
                className={`flex w-full items-start gap-2 px-2 py-1.5 text-left text-[11px] ${
                  active
                    ? 'bg-shell-accent/15 text-shell-text'
                    : 'text-shell-text hover:bg-shell-active'
                }`}
              >
                <span className="w-12 shrink-0 pt-0.5 text-[9px] font-semibold uppercase tracking-wide text-shell-muted">
                  {cmd.kind}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="font-mono text-shell-accent">/{cmd.name}</span>
                  <span className="ml-1.5 text-shell-text/90">{cmd.label}</span>
                  <span className="mt-0.5 block truncate text-[10px] text-shell-muted">
                    {cmd.description}
                  </span>
                </span>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
