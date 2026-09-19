import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FolderOpen, Star } from 'lucide-react'
import type { Workspace } from '../../lib/api'
import { isSessionWorkspace, pickDirectory } from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { SHELL } from '../shell/shellTokens'

type Props = {
  workspaces: Workspace[]
  busy?: boolean
  onOpenPath: (path: string) => void
  onSwitch: (id: string) => void
  onClose: () => void
}

export function OpenWorkspaceModal({
  workspaces,
  busy,
  onOpenPath,
  onSwitch,
  onClose,
}: Props) {
  const { t } = useLocale()
  const [picking, setPicking] = useState(false)
  const [pathInput, setPathInput] = useState('')
  const [error, setError] = useState<string | null>(null)
  const dialogRef = useRef<HTMLDivElement>(null)
  const browseRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const opener = document.activeElement
    browseRef.current?.focus()
    return () => {
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus()
    }
  }, [])

  const recents = useMemo(
    () =>
      workspaces
        .filter((w) => !isSessionWorkspace(w))
        .slice()
        .sort((a, b) => {
          const ta = a.last_opened_at ? Date.parse(a.last_opened_at) : 0
          const tb = b.last_opened_at ? Date.parse(b.last_opened_at) : 0
          return tb - ta
        }),
    [workspaces],
  )

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const openPath = useCallback(
    (path: string) => {
      const p = path.trim()
      if (!p) return
      onOpenPath(p)
      onClose()
    },
    [onClose, onOpenPath],
  )

  const onBrowse = useCallback(async () => {
    if (busy || picking) return
    setPicking(true)
    setError(null)
    try {
      const path = (await pickDirectory()).trim()
      if (!path) return
      openPath(path)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setPicking(false)
    }
  }, [busy, openPath, picking])

  return (
    <div
      data-testid="open-workspace-modal-backdrop"
      className="fixed inset-0 z-[110] flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onClick={onClose}
    >
      <div
        ref={dialogRef}
        tabIndex={-1}
        data-testid="open-workspace-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="open-workspace-title"
        className="shell-scroll flex max-h-[calc(100dvh-2rem)] w-full max-w-md flex-col overflow-y-auto rounded-lg border border-shell-border bg-shell-panel shadow-xl"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key !== 'Tab') return
          const controls = dialogRef.current?.querySelectorAll<HTMLElement>(
            'button:not(:disabled), input:not(:disabled), [tabindex="0"]',
          )
          if (!controls?.length) {
            e.preventDefault()
            dialogRef.current?.focus()
            return
          }
          const first = controls[0]
          const last = controls[controls.length - 1]
          if (e.shiftKey && (document.activeElement === first || document.activeElement === dialogRef.current)) {
            e.preventDefault()
            last.focus()
          } else if (!e.shiftKey && (document.activeElement === last || document.activeElement === dialogRef.current)) {
            e.preventDefault()
            first.focus()
          }
        }}
      >
        <div className="flex items-center justify-between border-b border-shell-border px-3 py-2">
          <h2
            id="open-workspace-title"
            className="text-sm font-medium text-shell-text"
          >
            {t('workspace.open')}
          </h2>
          <button
            type="button"
            data-testid="open-workspace-close"
            aria-label={t('action.close')}
            className="rounded px-2 py-1 text-xs text-shell-muted hover:bg-shell-hover hover:text-shell-text"
            onClick={onClose}
          >
            Esc
          </button>
        </div>

        <div className="px-3 pt-3">
          <label htmlFor="open-workspace-path" className="mb-1.5 block text-xs font-medium text-shell-text">
            {t('workspace.folderPath')}
          </label>
          <div className="flex gap-2">
          <input
            data-testid="open-workspace-path-input"
            id="open-workspace-path"
            disabled={busy || picking}
            type="text"
            value={pathInput}
            onChange={(e) => setPathInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') openPath(pathInput)
            }}
            placeholder="/path/to/project"
            className="min-w-0 flex-1 rounded-md border border-shell-border bg-shell-bg px-2.5 py-2 text-xs text-shell-text outline-none focus:border-shell-accent"
          />
          <button
            type="button"
            data-testid="open-workspace-path-go"
            disabled={busy || picking || !pathInput.trim()}
            onClick={() => openPath(pathInput)}
            className="shrink-0 rounded-md border border-shell-border px-3 py-2 text-xs font-medium text-shell-text hover:bg-shell-hover disabled:opacity-40"
          >
            {t('action.open')}
          </button>
          </div>
        </div>

        <div className="px-3 pt-3">
          <button
            type="button"
            ref={browseRef}
            data-testid="open-workspace-browse"
            disabled={busy || picking}
            onClick={() => void onBrowse()}
            className="shell-primary-button flex w-full items-center justify-center gap-2 rounded-md px-3 py-2.5 text-xs font-semibold disabled:opacity-40"
          >
            <FolderOpen size={SHELL.iconSm} aria-hidden />
            {picking ? t('workspace.browsing') : t('workspace.browseFolder')}
          </button>
        </div>

        <div className="mt-4 border-t border-shell-border">
          <p className="px-3 py-2 text-xs font-medium text-shell-muted">
            {t('workspace.recent')}
          </p>
          <ul
            data-testid="open-workspace-recents"
            className="shell-scroll max-h-52 overflow-auto px-1 pb-2"
          >
            {recents.length === 0 && (
              <li className="px-2 py-3 text-center text-[11px] text-shell-muted">
                {t('workspace.noRecent')}
              </li>
            )}
            {recents.map((ws) => (
              <li key={ws.id}>
                <button
                  type="button"
                  data-testid={`open-workspace-recent-${ws.id}`}
                  disabled={busy || ws.broken}
                  onClick={() => {
                    onSwitch(ws.id)
                    onClose()
                  }}
                  className="mb-0.5 flex w-full min-w-0 items-start gap-1.5 rounded-md px-2 py-1.5 text-left text-xs text-shell-muted transition hover:bg-shell-border/30 hover:text-shell-text disabled:opacity-40"
                >
                  <Star
                    size={SHELL.iconXs}
                    className="mt-0.5 shrink-0 opacity-50"
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-shell-text">
                      {ws.name}
                      {ws.broken ? ' (missing)' : ''}
                    </span>
                    <span
                      className="block truncate font-mono text-[11px] text-shell-muted"
                      title={ws.root_path}
                    >
                      {ws.root_path}
                    </span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>

        {error && (
          <p
            data-testid="open-workspace-error"
            role="alert"
            className="border-t border-shell-border px-3 py-2 text-xs text-shell-text"
          >
            {error}
          </p>
        )}
      </div>
    </div>
  )
}
