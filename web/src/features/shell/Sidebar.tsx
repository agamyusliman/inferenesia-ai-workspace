import type { LucideIcon } from 'lucide-react'
import {
  BookOpen,
  BotMessageSquare,
  FolderOpen,
  LayoutPanelLeft,
  Settings,
  SquareTerminal,
} from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import type { MessageKey } from '../i18n/messages'
import { SHELL } from './shellTokens'

export type NavId =
  | 'workspaces'
  | 'sessions'
  | 'canvas'
  | 'files'
  | 'history'
  | 'git'
  | 'chat'
  | 'settings'
  | 'docs'
  | 'preview'
  | 'todos'
  | 'missions'
  | 'tasks'
  | 'browser'

type NavItem = {
  id: NavId | 'terminal'
  labelKey: MessageKey
  Icon: LucideIcon
  kind: 'nav' | 'terminal'
}

const NAV: NavItem[] = [
  { id: 'sessions', labelKey: 'nav.sessions', Icon: BotMessageSquare, kind: 'nav' },
  { id: 'workspaces', labelKey: 'nav.workspaces', Icon: FolderOpen, kind: 'nav' },
  { id: 'canvas', labelKey: 'nav.canvas', Icon: LayoutPanelLeft, kind: 'nav' },
  { id: 'terminal', labelKey: 'nav.terminal', Icon: SquareTerminal, kind: 'terminal' },
  { id: 'docs', labelKey: 'nav.docs', Icon: BookOpen, kind: 'nav' },
  { id: 'settings', labelKey: 'nav.settings', Icon: Settings, kind: 'nav' },
]

export const ACTIVITY_RAIL_WIDTH = SHELL.activityRail

type Props = {
  active: NavId
  onChange: (id: NavId) => void
  product: string
  terminalOpen?: boolean
  onToggleTerminal?: () => void
}

export function Sidebar({
  active,
  onChange,
  product,
  terminalOpen = false,
  onToggleTerminal,
}: Props) {
  const { t } = useLocale()
  const w = ACTIVITY_RAIL_WIDTH
  const railNav: NavId =
    active === 'files' ||
    active === 'git' ||
    active === 'chat' ||
    active === 'history'
      ? 'workspaces'
      : active === 'todos' ||
          active === 'missions' ||
          active === 'tasks' ||
          active === 'browser'
        ? 'sessions'
        : active

  const highlightTerminal = terminalOpen

  return (
    <aside
      data-testid="sidebar"
      data-sidebar-width={w}
      data-expanded="false"
      style={{ width: w, flex: 'none' }}
      className="relative z-20 flex shrink-0 flex-col border-r border-shell-border bg-shell-panel"
    >
      <div
        className="flex h-10 w-full shrink-0 items-center justify-center border-b border-shell-border"
        title={product}
        data-testid="sidebar-brand"
      >
        <span
          className="group/tip relative flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-shell-border bg-shell-active text-[11px] font-semibold tracking-tight text-shell-text"
          aria-label={product}
        >
          In
          <span
            role="tooltip"
            className="pointer-events-none absolute left-full top-1/2 z-[200] ml-2 -translate-y-1/2 whitespace-nowrap rounded border border-shell-border bg-shell-panel px-1.5 py-0.5 text-[10px] font-normal text-shell-text opacity-0 shadow-lg transition-opacity group-hover/tip:opacity-100"
          >
            {product}
          </span>
        </span>
      </div>
      <nav
        className="shell-activity-nav flex min-h-0 flex-1 flex-col items-center gap-1 py-2"
        aria-label={t('nav.primary')}
      >
        {NAV.map((item) => {
          if (item.kind === 'terminal') {
            if (!onToggleTerminal) return null
            const { Icon } = item
            const termLabel = t('nav.terminal')
            const termTitle = highlightTerminal
              ? t('nav.hideTerminal')
              : t('nav.showTerminal')
            return (
              <button
                key={item.id}
                type="button"
                data-testid="nav-terminal"
                data-active={highlightTerminal ? 'true' : 'false'}
                title={termTitle}
                aria-label={termLabel}
                aria-pressed={highlightTerminal}
                onClick={onToggleTerminal}
                className={`group/tip relative ${SHELL.activityBtnClass} ${
                  highlightTerminal
                    ? 'bg-shell-active text-shell-accent'
                    : 'text-shell-muted hover:bg-shell-hover hover:text-shell-text'
                }`}
              >
                <Icon size={SHELL.iconSm} strokeWidth={1.75} aria-hidden className="shrink-0" />
                <span
                  role="tooltip"
                  className="pointer-events-none absolute left-full top-1/2 z-[200] ml-2 -translate-y-1/2 whitespace-nowrap rounded border border-shell-border bg-shell-panel px-1.5 py-0.5 text-[10px] font-normal normal-case tracking-normal text-shell-text opacity-0 shadow-lg transition-opacity duration-100 group-hover/tip:opacity-100 group-focus-visible/tip:opacity-100"
                >
                  {termLabel}
                </span>
              </button>
            )
          }

          const isActive = item.id === railNav
          const { Icon } = item
          const label = t(item.labelKey)
          return (
            <button
              key={item.id}
              type="button"
              data-testid={`nav-${item.id}`}
              data-active={isActive ? 'true' : 'false'}
              title={label}
              aria-label={label}
              aria-current={isActive ? 'page' : undefined}
              onClick={() => onChange(item.id as NavId)}
              className={`group/tip relative ${SHELL.activityBtnClass} ${item.id === 'docs' ? 'mt-auto' : ''} ${
                isActive
                  ? 'bg-shell-active text-shell-accent'
                  : 'text-shell-muted hover:bg-shell-hover hover:text-shell-text'
              }`}
            >
              {isActive && (
                <span aria-hidden className="absolute -left-1.5 h-4 w-0.5 rounded-r bg-shell-accent" />
              )}
              <Icon size={SHELL.iconSm} strokeWidth={1.75} aria-hidden className="shrink-0" />
              <span
                role="tooltip"
                className="pointer-events-none absolute left-full top-1/2 z-[200] ml-2 -translate-y-1/2 whitespace-nowrap rounded border border-shell-border bg-shell-panel px-1.5 py-0.5 text-[10px] font-normal normal-case tracking-normal text-shell-text opacity-0 shadow-lg transition-opacity duration-100 group-hover/tip:opacity-100 group-focus-visible/tip:opacity-100"
              >
                {label}
              </span>
            </button>
          )
        })}
      </nav>
    </aside>
  )
}
