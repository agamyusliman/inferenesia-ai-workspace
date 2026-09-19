import { useCallback, useMemo, useState } from 'react'
import { FolderOpen, Star } from 'lucide-react'
import type { Workspace } from '../../lib/api'
import { isSessionWorkspace, pickDirectory } from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'

type Props = {
  workspaces: Workspace[]
  busy?: boolean
  onOpenPath: (path: string) => void
  onSwitch: (id: string) => void
}

export function WorkspaceChatPlaceholder({
  workspaces,
  busy,
  onOpenPath,
  onSwitch,
}: Props) {
  const { t } = useLocale()
  const [picking, setPicking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pathInput, setPathInput] = useState('')

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

  const openPath = useCallback(
    (path: string) => {
      const p = path.trim()
      if (!p) return
      onOpenPath(p)
    },
    [onOpenPath],
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
      data-testid="workspace-chat-placeholder"
      className="flex h-full min-h-0 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader title={t('workspace.open')} dense />

      <div className="flex min-h-0 flex-1 flex-col overflow-auto">
        <div className="mx-auto my-auto flex w-full max-w-xl flex-col gap-5 px-5 py-8">
          <div className="space-y-1.5 text-center">
            <p className="text-[15px] font-semibold tracking-tight text-shell-text">
              {t('workspace.chatNeedsFolderTitle')}
            </p>
            <p className="mx-auto max-w-md text-[12px] leading-relaxed text-shell-muted">
              {t('workspace.chatNeedsFolder')}
            </p>
          </div>

          <button
            type="button"
            data-testid="workspace-chat-browse"
            disabled={busy || picking}
            onClick={() => void onBrowse()}
            className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-shell-accent px-3 py-2.5 text-[13px] font-medium text-white shadow-sm transition hover:brightness-110 disabled:opacity-40"
          >
            <FolderOpen size={SHELL.iconSm} aria-hidden />
            {picking ? t('workspace.browsing') : t('workspace.browseFolder')}
          </button>
          <div className="flex items-stretch gap-1.5">
            <input
              data-testid="workspace-chat-path-input"
              type="text"
              value={pathInput}
              onChange={(e) => setPathInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && pathInput.trim()) openPath(pathInput)
              }}
              placeholder="/path/to/project"
              className="min-w-0 flex-1 rounded border border-shell-border bg-shell-bg px-2 py-1.5 text-[12px] text-shell-text outline-none focus:border-shell-accent"
            />
            <button
              type="button"
              data-testid="workspace-chat-path-go"
              disabled={busy || !pathInput.trim()}
              onClick={() => openPath(pathInput)}
              className="shrink-0 rounded bg-shell-accent/90 px-3 py-1.5 text-[12px] font-medium text-white disabled:opacity-40"
            >
              {t('action.open')}
            </button>
          </div>

          {error && (
            <p
              data-testid="workspace-chat-error"
              className="text-center text-[11px] text-red-400"
            >
              {error}
            </p>
          )}

          <section className="min-h-0">
            <h3 className="mb-2 text-center text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
              {t('workspace.recent')}
            </h3>
            <ul
              data-testid="workspace-chat-recents"
              className="flex flex-col gap-1.5"
            >
              {recents.length === 0 && (
                <li className="rounded-lg border border-dashed border-shell-border bg-shell-panel px-3 py-6 text-center text-[12px] text-shell-muted">
                  {t('workspace.noRecent')}
                </li>
              )}
              {recents.map((ws) => (
                <li key={ws.id}>
                  <button
                    type="button"
                    data-testid={`workspace-chat-recent-${ws.id}`}
                    disabled={busy || ws.broken}
                    onClick={() => onSwitch(ws.id)}
                    className="flex w-full min-w-0 items-start gap-2.5 rounded-lg border border-shell-border bg-shell-panel px-3 py-2.5 text-left transition hover:border-shell-accent/35 hover:bg-shell-hover disabled:opacity-40"
                  >
                    <Star
                      size={SHELL.iconXs}
                      className="mt-0.5 shrink-0 text-shell-muted"
                      aria-hidden
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[12px] font-medium text-shell-text">
                        {ws.name}
                        {ws.broken ? ' (missing)' : ''}
                      </span>
                      <span
                        className="mt-0.5 block truncate font-mono text-[10px] text-shell-muted"
                        title={ws.root_path}
                      >
                        {ws.root_path}
                      </span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </section>
        </div>
      </div>
    </div>
  )
}
