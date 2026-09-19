import { useEffect, useRef } from 'react'
import type { ContextMenuItem } from './contextMenuItems'

type Props = {
  x: number
  y: number
  items: ContextMenuItem[]
  onAction: (id: string) => void
  onClose: () => void
}

/**
 * Floating Pack A context menu. Closes on outside click / Escape.
 */
export function ExplorerContextMenu({ x, y, items, onAction, onClose }: Props) {
  const ref = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mousedown', onDown)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mousedown', onDown)
    }
  }, [onClose])

  // Clamp into viewport after first paint via inline max dimensions.
  const maxX = typeof window !== 'undefined' ? window.innerWidth - 8 : x
  const maxY = typeof window !== 'undefined' ? window.innerHeight - 8 : y
  const left = Math.min(x, Math.max(8, maxX - 180))
  const top = Math.min(y, Math.max(8, maxY - 40))

  const itemBtn =
    'flex w-full min-w-0 items-center gap-2 whitespace-nowrap px-2.5 py-0.5 text-left text-[11px] leading-tight hover:bg-shell-border/40 disabled:opacity-40'

  return (
    <div
      ref={ref}
      data-testid="explorer-context-menu"
      role="menu"
      className="fixed z-[100] w-max max-w-[14rem] min-w-[9.5rem] rounded-md border border-shell-border bg-shell-panel py-0.5 text-[11px] leading-tight text-shell-text shadow-lg shadow-black/40"
      style={{ left, top }}
    >
      {items.map((item, idx) => {
        if (item.id === 'separator') {
          return (
            <div
              key={`sep-${idx}`}
              role="separator"
              className="my-0.5 border-t border-shell-border"
            />
          )
        }
        if (item.children && item.children.length > 0) {
          return (
            <div
              key={item.id + item.label}
              className="group relative"
              data-testid={`ctx-sub-${item.id}`}
            >
              <button
                type="button"
                role="menuitem"
                title={item.label}
                className={`${itemBtn} justify-between`}
              >
                <span className="min-w-0 truncate">{item.label}</span>
                <span className="shrink-0 text-shell-muted">▸</span>
              </button>
              <div
                role="menu"
                data-testid="explorer-open-with-submenu"
                className="absolute left-full top-0 z-[101] hidden w-max max-w-[12rem] min-w-[7rem] rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg group-hover:block group-focus-within:block"
              >
                {item.children.map((child) => (
                  <button
                    key={child.id}
                    type="button"
                    role="menuitem"
                    title={child.label}
                    data-testid={`ctx-action-${child.id}`}
                    data-action={child.id}
                    className={itemBtn}
                    onClick={(e) => {
                      e.stopPropagation()
                      onAction(child.id)
                    }}
                  >
                    <span className="min-w-0 truncate">{child.label}</span>
                  </button>
                ))}
              </div>
            </div>
          )
        }
        return (
          <button
            key={item.id + item.label}
            type="button"
            role="menuitem"
            title={item.label}
            data-testid={`ctx-action-${item.id}`}
            data-action={item.id}
            disabled={item.disabled}
            className={`${itemBtn} ${
              item.danger ? 'text-red-400 hover:bg-red-950/40' : ''
            }`}
            onClick={(e) => {
              e.stopPropagation()
              if (!item.disabled) onAction(item.id)
            }}
          >
            <span className="min-w-0 truncate">{item.label}</span>
          </button>
        )
      })}
    </div>
  )
}
