/**
 * Browser status panel — subscriber only (VAL-BRW-008).
 * Renders BrowserStatus lifecycle from core.Service; no browser automation in TS.
 */
import { useCallback, useEffect, useState } from 'react'
import {
  AlertTriangle,
  Circle,
  Globe,
  Loader2,
  Power,
  RefreshCw,
  X,
  XCircle,
} from 'lucide-react'
import {
  browserClose,
  getBrowserStatus,
  type BrowserEngineAvail,
  type BrowserStatusListView,
  type BrowserStatusView,
} from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'

type Props = {
  /** Bump on BrowserStatus stream events. */
  refreshKey?: number
  onClose?: () => void
}

function stateTone(state: string): string {
  switch (state) {
    case 'ready':
    case 'navigate_end':
      return 'text-emerald-300 bg-emerald-950/50 border-emerald-800/60'
    case 'error':
    case 'unavailable':
      return 'text-rose-200 bg-rose-950/50 border-rose-800/60'
    case 'launching':
    case 'navigating':
    case 'selecting':
    case 'waiting_human':
      return 'text-sky-200 bg-sky-950/40 border-sky-800/50'
    case 'stopped':
    case 'idle':
      return 'text-zinc-300 bg-zinc-900/60 border-zinc-700/60'
    default:
      return 'text-shell-muted bg-shell-panel border-shell-border'
  }
}

function StateIcon({ state }: { state: string }) {
  switch (state) {
    case 'ready':
    case 'navigate_end':
      return <Globe size={SHELL.iconSm} className="text-emerald-400" aria-hidden />
    case 'error':
    case 'unavailable':
      return <XCircle size={SHELL.iconSm} className="text-rose-400" aria-hidden />
    case 'launching':
    case 'navigating':
    case 'selecting':
    case 'waiting_human':
      return <Loader2 size={SHELL.iconSm} className="animate-spin text-sky-300" aria-hidden />
    case 'stopped':
      return <Power size={SHELL.iconSm} className="text-zinc-400" aria-hidden />
    default:
      return <Circle size={SHELL.iconSm} className="text-shell-muted" aria-hidden />
  }
}

function EngineRow({ eng }: { eng: BrowserEngineAvail }) {
  return (
    <div
      data-testid={`browser-engine-${eng.engine}`}
      className="min-w-0 rounded border border-shell-border/70 bg-shell-panel/60 px-2 py-1.5 text-[11px]"
    >
      <div className="flex min-w-0 items-center justify-between gap-2">
        <span className="min-w-0 truncate font-medium text-shell-text">{eng.engine}</span>
        <span className={`shrink-0 ${eng.ok ? 'text-emerald-300' : 'text-amber-200'}`}>
          {eng.ok ? 'ready' : 'unavailable'}
        </span>
      </div>
      {eng.detail && (
        <div className="mt-0.5 break-all text-[10px] leading-snug text-shell-muted" title={eng.detail}>
          {eng.detail}
        </div>
      )}
      {!eng.ok && eng.install && (
        <div className="mt-1 flex min-w-0 items-start gap-1 text-amber-100/90">
          <AlertTriangle size={12} className="mt-0.5 shrink-0" aria-hidden />
          <span className="min-w-0 break-words leading-snug">{eng.install}</span>
        </div>
      )}
    </div>
  )
}

function LifecycleRow({ row }: { row: BrowserStatusView }) {
  return (
    <li
      data-testid="browser-status-event"
      data-state={row.state}
      data-engine={row.engine || ''}
      className={`min-w-0 rounded border px-2 py-1.5 text-[11px] ${stateTone(row.state)}`}
    >
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <StateIcon state={row.state} />
        <span className="font-semibold">{row.state || 'idle'}</span>
        {row.engine && (
          <span className="max-w-full truncate text-[10px] opacity-80">engine={row.engine}</span>
        )}
        {row.fallback && (
          <span className="rounded bg-amber-950/60 px-1 text-[10px] text-amber-200">
            fallback
          </span>
        )}
        <span className="ml-auto shrink-0 text-[10px] opacity-70">{row.at || ''}</span>
      </div>
      {(row.url || row.detail || row.reason || row.cdp_url) && (
        <div className="mt-0.5 min-w-0 space-y-0.5 pl-0 text-[10px] opacity-90 sm:pl-5">
          {row.url && (
            <div className="break-all" title={row.url}>
              url={row.url}
            </div>
          )}
          {row.cdp_url && (
            <div className="break-all" title={row.cdp_url}>
              cdp={row.cdp_url}
            </div>
          )}
          {row.cdp_port ? <div>cdp_port={row.cdp_port}</div> : null}
          {row.detail && (
            <div className="break-words" title={row.detail}>
              detail={row.detail}
            </div>
          )}
          {row.reason && (
            <div className="break-words" title={row.reason}>
              reason={row.reason}
            </div>
          )}
        </div>
      )}
    </li>
  )
}

export function BrowserPanel({ refreshKey, onClose }: Props) {
  const { t } = useLocale()
  const [data, setData] = useState<BrowserStatusListView | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setBusy(true)
    try {
      const st = await getBrowserStatus()
      setData(st)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh, refreshKey])

  const onCloseEngine = async () => {
    setBusy(true)
    try {
      await browserClose()
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const current = data?.current
  const history = [...(data?.history || [])].reverse() // newest first for UI

  return (
    <div
      data-testid="browser-panel"
      className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-shell-bg text-shell-text"
    >
      <PanelHeader
        title={t('browser.title')}
        dense
        testId="browser-panel-header"
        actions={
          <div className="flex shrink-0 items-center gap-0.5">
            <button
              type="button"
              data-testid="browser-refresh"
              className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
              onClick={() => void refresh()}
              title={t('action.refresh')}
              aria-label={t('action.refresh')}
              disabled={busy}
            >
              <RefreshCw size={SHELL.iconXs} className={busy ? 'animate-spin' : ''} />
            </button>
            <button
              type="button"
              data-testid="browser-close-engine"
              className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
              onClick={() => void onCloseEngine()}
              title={t('action.stop')}
              aria-label={t('action.stop')}
              disabled={busy}
            >
              <Power size={SHELL.iconXs} />
            </button>
            {onClose && (
              <button
                type="button"
                data-testid="browser-panel-close"
                className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
                title={t('panel.closeTitle')}
                aria-label={t('panel.closeTitle')}
                onClick={onClose}
              >
                <X size={SHELL.iconXs} aria-hidden />
              </button>
            )}
          </div>
        }
      />

      {error && (
        <div
          data-testid="browser-panel-error"
          className="mx-1.5 mt-1.5 break-words rounded border border-rose-800/60 bg-rose-950/40 px-2 py-1 text-[11px] text-rose-100"
        >
          {error}
        </div>
      )}

      <div className="min-h-0 min-w-0 flex-1 space-y-3 overflow-y-auto overflow-x-hidden p-1.5">
        <section data-testid="browser-current" className="min-w-0">
          <h3 className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
            Current
          </h3>
          <div
            className={`min-w-0 rounded border px-2 py-1.5 text-[11px] ${stateTone(current?.state || 'idle')}`}
          >
            <div className="flex min-w-0 flex-wrap items-center gap-1.5">
              <StateIcon state={current?.state || 'idle'} />
              <span className="font-semibold" data-testid="browser-current-state">
                {current?.state || 'idle'}
              </span>
              {current?.engine && (
                <span className="min-w-0 truncate" data-testid="browser-current-engine">
                  {current.engine}
                </span>
              )}
              {current?.fallback && <span className="text-amber-200">fallback</span>}
            </div>
            {current?.cdp_port ? (
              <div className="mt-1 break-all text-[10px]" data-testid="browser-cdp-port">
                cdp_port={current.cdp_port}
                {current.cdp_url ? ` · ${current.cdp_url}` : ''}
              </div>
            ) : null}
            {(current?.wait_min_ms || current?.wait_max_ms) && (
              <div className="mt-0.5 text-[10px] opacity-80" data-testid="browser-human-wait">
                human_wait={current.wait_min_ms}–{current.wait_max_ms}ms
              </div>
            )}
            {current?.reason && (
              <div className="mt-1 break-words text-[11px] text-rose-100/90">{current.reason}</div>
            )}
          </div>
        </section>

        <section data-testid="browser-engines" className="min-w-0">
          <h3 className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
            Engines
          </h3>
          <div className="grid min-w-0 gap-1.5">
            {(data?.engines || []).map((e) => (
              <EngineRow key={e.engine} eng={e} />
            ))}
          </div>
        </section>

        <section data-testid="browser-lifecycle" className="min-w-0">
          <h3 className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
            Lifecycle
          </h3>
          {history.length === 0 ? (
            <p className="break-words text-[11px] leading-snug text-shell-muted">
              No BrowserStatus events yet. Agent tools or CLI browser set-engine emit
              select → launch → navigate-start → navigate-end → stop.
            </p>
          ) : (
            <ul className="min-w-0 space-y-1">
              {history.map((row, i) => (
                <LifecycleRow key={`${row.at}-${row.state}-${i}`} row={row} />
              ))}
            </ul>
          )}
        </section>

        <p className="break-words text-[10px] leading-snug text-shell-muted">
          Status subscriber only. Engines run as Go sidecars; cookies never enter chat
          context.
        </p>
      </div>
    </div>
  )
}
