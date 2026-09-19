import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react'
import { useLocale } from '../i18n/LocaleProvider'
import {
  filterCommands,
  getPaletteCommands,
  type PaletteCommand,
} from './commands'

type Props = {
  open: boolean
  onClose: () => void
  onRun: (id: string) => void
  commands?: PaletteCommand[]
  mode?: 'commands' | 'files'
}

export function CommandPalette({
  open,
  onClose,
  onRun,
  commands,
  mode = 'commands',
}: Props) {
  const { t } = useLocale()
  const [query, setQuery] = useState('')
  const [index, setIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const listRef = useRef<HTMLDivElement | null>(null)

  const resolvedCommands = useMemo(
    () => commands ?? getPaletteCommands(t),
    [commands, t],
  )

  const filtered = useMemo(
    () => filterCommands(query, resolvedCommands),
    [query, resolvedCommands],
  )

  // Reset state each open.
  useEffect(() => {
    if (open) {
      setQuery('')
      setIndex(0)
      // Focus after paint.
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  // Keep selection in range when filter shrinks.
  useEffect(() => {
    if (index >= filtered.length) {
      setIndex(filtered.length > 0 ? filtered.length - 1 : 0)
    }
  }, [filtered.length, index])

  // Scroll active item into view.
  useEffect(() => {
    if (!listRef.current) return
    const el = listRef.current.querySelector<HTMLElement>(
      `[data-palette-index="${index}"]`,
    )
    el?.scrollIntoView({ block: 'nearest' })
  }, [index, filtered])

  if (!open) return null

  const runAt = (i: number) => {
    const cmd = filtered[i]
    if (!cmd) return
    onRun(cmd.id)
    onClose()
  }

  const onKeyDown = (e: ReactKeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      onClose()
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setIndex((i) => (filtered.length ? (i + 1) % filtered.length : 0))
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      setIndex((i) =>
        filtered.length ? (i - 1 + filtered.length) % filtered.length : 0,
      )
      return
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      runAt(index)
    }
  }

  return (
    <div
      data-testid="command-palette-overlay"
      className="fixed inset-0 z-[200] flex items-start justify-center bg-black/50 pt-[12vh]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        data-testid="command-palette"
        data-mode={mode}
        role="dialog"
        aria-modal="true"
        aria-label={
          mode === 'files' ? t('palette.quickOpen') : t('palette.title')
        }
        className="w-full max-w-lg overflow-hidden rounded-lg border border-shell-border bg-shell-panel shadow-2xl shadow-black/50"
        onKeyDown={onKeyDown}
      >
        <div className="border-b border-shell-border px-3 py-2">
          <input
            ref={inputRef}
            data-testid="command-palette-input"
            type="text"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setIndex(0)
            }}
            placeholder={
              mode === 'files'
                ? t('palette.placeholderFiles')
                : t('palette.placeholder')
            }
            className="w-full bg-transparent text-sm text-shell-text outline-none placeholder:text-shell-muted"
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div
          ref={listRef}
          data-testid="command-palette-list"
          role="listbox"
          className="max-h-72 overflow-y-auto py-1"
        >
          {filtered.length === 0 ? (
            <div
              data-testid="command-palette-empty"
              className="px-3 py-4 text-center text-xs text-shell-muted"
            >
              {t('palette.empty')}
            </div>
          ) : (
            filtered.map((cmd, i) => (
              <button
                key={cmd.id}
                type="button"
                role="option"
                aria-selected={i === index}
                data-testid={`command-palette-item-${cmd.id}`}
                data-command-id={cmd.id}
                data-palette-index={i}
                data-active={i === index ? 'true' : 'false'}
                className={`flex w-full items-center justify-between gap-3 px-3 py-1.5 text-left text-[13px] ${
                  i === index
                    ? 'bg-shell-active text-shell-text'
                    : 'text-shell-text hover:bg-shell-border/30'
                }`}
                onMouseEnter={() => setIndex(i)}
                onClick={() => runAt(i)}
              >
                <span className="min-w-0 truncate">{cmd.label}</span>
                {cmd.shortcut && (
                  <span className="shrink-0 text-[11px] text-shell-muted">
                    {cmd.shortcut}
                  </span>
                )}
              </button>
            ))
          )}
        </div>
        <div className="border-t border-shell-border px-3 py-1.5 text-[10px] text-shell-muted">
          {t('palette.footer')}
        </div>
      </div>
    </div>
  )
}
