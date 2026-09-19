import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { LayoutPanelLeft, RotateCcw, X } from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import type { ChatDock, ExplorerDock } from './useLayoutConfig'
import { SHELL } from './shellTokens'

type Props = {
  explorerDock: ExplorerDock
  chatDock: ChatDock
  onExplorerDock: (d: ExplorerDock) => void
  onChatDock: (d: ChatDock) => void
  onReset: () => void
}

function optionClass(active: boolean) {
  return [
    'flex w-full items-center rounded px-2 py-1.5 text-left text-[12px] transition',
    active
      ? 'bg-shell-accent/15 font-medium text-shell-accent'
      : 'text-shell-text hover:bg-shell-border/30',
  ].join(' ')
}

export function LayoutControls({
  explorerDock,
  chatDock,
  onExplorerDock,
  onChatDock,
  onReset,
}: Props) {
  const { t } = useLocale()
  const [open, setOpen] = useState(false)
  const titleId = useId()
  const closeBtnRef = useRef<HTMLButtonElement | null>(null)

  const explorerOpts = useMemo(
    () =>
      [
        { value: 'left' as const, label: t('layout.left') },
        { value: 'right' as const, label: t('layout.right') },
      ] satisfies { value: ExplorerDock; label: string }[],
    [t],
  )

  const chatOpts = useMemo(
    () =>
      [
        { value: 'right' as const, label: t('layout.right') },
        { value: 'left' as const, label: t('layout.left') },
        { value: 'bottom' as const, label: t('layout.bottom') },
      ] satisfies { value: ChatDock; label: string }[],
    [t],
  )

  const close = useCallback(() => setOpen(false), [])

  useEffect(() => {
    if (!open) return
    closeBtnRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        close()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, close])

  return (
    <div data-testid="layout-controls" className="relative flex shrink-0 items-center">
      <button
        type="button"
        data-testid="layout-open"
        onClick={() => setOpen(true)}
        className="shell-transition flex h-5 items-center gap-1 rounded px-1.5 text-[10px] text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
        title={t('layout.settingsTitle')}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        <LayoutPanelLeft size={SHELL.iconXs} aria-hidden />
        <span className="hidden sm:inline">{t('layout.title')}</span>
      </button>

      {open && (
        <div
          data-testid="layout-dialog-backdrop"
          className="fixed inset-0 z-[120] flex items-center justify-center bg-black/50 p-4"
          role="presentation"
          onClick={close}
        >
          <div
            data-testid="layout-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
            className="w-full max-w-xs rounded-md border border-shell-border bg-shell-panel shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-shell-border px-3 py-2">
              <h2
                id={titleId}
                className="text-[12px] font-semibold uppercase tracking-wide text-shell-muted"
              >
                {t('layout.title')}
              </h2>
              <button
                ref={closeBtnRef}
                type="button"
                data-testid="layout-dialog-close"
                onClick={close}
                className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted transition hover:bg-shell-border/40 hover:text-shell-text"
                title={t('action.close')}
                aria-label={t('layout.closeAria')}
              >
                <X size={SHELL.iconXs} aria-hidden />
              </button>
            </div>

            <div className="space-y-3 px-3 py-3">
              <section data-testid="layout-explorer-section">
                <h3 className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                  {t('layout.explorer')}
                </h3>
                <div
                  data-testid="layout-explorer-dock"
                  data-value={explorerDock}
                  role="listbox"
                  aria-label={t('layout.explorerPos')}
                  className="flex flex-col gap-0.5"
                >
                  {explorerOpts.map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      role="option"
                      aria-selected={explorerDock === opt.value}
                      data-testid={`layout-explorer-${opt.value}`}
                      data-active={explorerDock === opt.value ? 'true' : 'false'}
                      className={optionClass(explorerDock === opt.value)}
                      onClick={() => onExplorerDock(opt.value)}
                    >
                      {opt.label}
                      {explorerDock === opt.value && (
                        <span className="ml-auto text-[10px] text-shell-accent">
                          {t('layout.current')}
                        </span>
                      )}
                    </button>
                  ))}
                </div>
              </section>

              <section data-testid="layout-chat-section">
                <h3 className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                  {t('layout.chat')}
                </h3>
                <div
                  data-testid="layout-chat-dock"
                  data-value={chatDock}
                  role="listbox"
                  aria-label={t('layout.chatPos')}
                  className="flex flex-col gap-0.5"
                >
                  {chatOpts.map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      role="option"
                      aria-selected={chatDock === opt.value}
                      data-testid={`layout-chat-${opt.value}`}
                      data-active={chatDock === opt.value ? 'true' : 'false'}
                      className={optionClass(chatDock === opt.value)}
                      onClick={() => onChatDock(opt.value)}
                    >
                      {opt.label}
                      {chatDock === opt.value && (
                        <span className="ml-auto text-[10px] text-shell-accent">
                          {t('layout.current')}
                        </span>
                      )}
                    </button>
                  ))}
                </div>
              </section>

              <div className="border-t border-shell-border pt-2">
                <button
                  type="button"
                  data-testid="layout-reset"
                  onClick={() => {
                    onReset()
                    close()
                  }}
                  className="inline-flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-[12px] text-shell-muted transition hover:bg-shell-border/30 hover:text-shell-text"
                  title={t('layout.resetTitle')}
                >
                  <RotateCcw size={SHELL.iconXs} aria-hidden />
                  {t('layout.reset')}
                </button>
              </div>
            </div>

            <div className="flex justify-end border-t border-shell-border px-3 py-2">
              <button
                type="button"
                data-testid="layout-dialog-done"
                onClick={close}
                className="rounded border border-shell-border bg-shell-bg px-3 py-1 text-[11px] text-shell-text transition hover:bg-shell-border/30"
              >
                {t('layout.done')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
