import { Palette } from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import { useTheme } from '../theme/ThemeProvider'
import { themeLabel } from '../theme/theme'
import { LayoutControls } from './LayoutControls'
import type { ChatDock, ExplorerDock } from './useLayoutConfig'
import { SHELL } from './shellTokens'

type Props = {
  product: string
  message?: string
  workspaceName?: string
  planModeActive?: boolean
  explorerDock?: ExplorerDock
  chatDock?: ChatDock
  onExplorerDock?: (d: ExplorerDock) => void
  onChatDock?: (d: ChatDock) => void
  onResetLayout?: () => void
}

export function StatusBar({
  product,
  message,
  workspaceName,
  planModeActive,
  explorerDock,
  chatDock,
  onExplorerDock,
  onChatDock,
  onResetLayout,
}: Props) {
  const { theme, cycleTheme } = useTheme()
  const { t } = useLocale()
  const themeName = themeLabel(theme)
  const showLayout =
    !!explorerDock &&
    !!chatDock &&
    !!onExplorerDock &&
    !!onChatDock &&
    !!onResetLayout

  return (
    <footer data-testid="status-bar" className={SHELL.statusBarClass}>
      <div className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
        <span
          data-testid="status-product"
          className="shrink-0 font-medium text-shell-text"
        >
          {product}
        </span>
        {workspaceName ? (
          <span data-testid="status-workspace" className="min-w-0 max-w-[220px] truncate">
            {workspaceName.startsWith('session:') ||
            workspaceName.startsWith('playground:') ? (
              <span className="text-shell-text">{workspaceName}</span>
            ) : (
              <>
                {t('status.workspace')}: <span className="text-shell-text">{workspaceName}</span>
              </>
            )}
          </span>
        ) : (
          <span className="shrink-0">{t('status.noWorkspace')}</span>
        )}
        {planModeActive && (
          <span
            data-testid="status-plan-mode"
            className="shrink-0 rounded bg-amber-500/20 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-300"
            title={t('status.planModeTitle')}
          >
            {t('status.planMode')}
          </span>
        )}
      </div>
      <div className="flex min-w-0 max-w-[55%] shrink-0 items-center justify-end gap-1.5">
        {showLayout && (
          <LayoutControls
            explorerDock={explorerDock}
            chatDock={chatDock}
            onExplorerDock={onExplorerDock}
            onChatDock={onChatDock}
            onReset={onResetLayout}
          />
        )}
        <button
          type="button"
          data-testid="status-theme-cycle"
          data-theme={theme}
          title={t('status.themeCycle', { name: themeName })}
          aria-label={t('status.themeAria', { name: themeName })}
          onClick={cycleTheme}
          className="shell-transition flex h-5 items-center gap-1 rounded px-1.5 text-[10px] text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
        >
          <Palette size={SHELL.iconXs} aria-hidden />
          <span className="hidden sm:inline">{themeName}</span>
        </button>
        <div
          data-testid="status-message"
          className="min-w-0 truncate text-right"
        >
          {message || t('status.ready')}
        </div>
      </div>
    </footer>
  )
}
