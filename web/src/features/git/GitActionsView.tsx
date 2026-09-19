import { useCallback, useEffect, useState } from 'react'
import { ExternalLink, RefreshCw, Workflow } from 'lucide-react'
import {
  getGitActions,
  openGitActionsRun,
  type GitActionsResult,
  type GitWorkflowRun,
} from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import type { MessageKey } from '../i18n/messages'
import { SHELL } from '../shell/shellTokens'

type Props = {
  repoID: string
  /** Bump to force reload without changing repo. */
  refreshKey?: number
}

const EMPTY: GitActionsResult = {
  ok: false,
  availability: 'empty',
  message: '',
  runs: [],
}

function badgeClass(badge: string): string {
  switch (badge) {
    case 'success':
      return 'bg-emerald-500/20 text-emerald-300 border-emerald-700/50'
    case 'failure':
      return 'bg-rose-500/20 text-rose-300 border-rose-700/50'
    case 'in_progress':
      return 'bg-sky-500/20 text-sky-300 border-sky-700/50'
    case 'queued':
      return 'bg-amber-500/20 text-amber-200 border-amber-700/50'
    case 'cancelled':
      return 'bg-shell-border/60 text-shell-muted border-shell-border'
    case 'skipped':
      return 'bg-violet-500/15 text-violet-300 border-violet-700/40'
    default:
      return 'bg-shell-border/40 text-shell-muted border-shell-border'
  }
}

function badgeLabelKey(badge: string): MessageKey | null {
  switch (badge) {
    case 'in_progress':
      return 'git.badgeInProgress'
    case 'queued':
      return 'git.badgeQueued'
    case 'success':
      return 'git.badgeSuccess'
    case 'failure':
      return 'git.badgeFailure'
    case 'cancelled':
      return 'git.badgeCancelled'
    case 'skipped':
      return 'git.badgeSkipped'
    default:
      return null
  }
}

export function GitActionsView({ repoID, refreshKey }: Props) {
  const { t } = useLocale()
  const [data, setData] = useState<GitActionsResult>(EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      const res = await getGitActions(repoID, 25)
      setData(res)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setData(EMPTY)
    } finally {
      setBusy(false)
    }
  }, [repoID])

  useEffect(() => {
    void load()
  }, [load, refreshKey])

  const onOpen = async (run: GitWorkflowRun) => {
    if (!run.html_url) return
    try {
      // Prefer OS open via core; also allow anchor fallback.
      await openGitActionsRun(run.html_url)
    } catch {
      // Fallback: window.open for browser/dev hub
      window.open(run.html_url, '_blank', 'noopener,noreferrer')
    }
  }

  const badgeLabel = (badge: string, status: string, conclusion?: string): string => {
    const key = badgeLabelKey(badge)
    if (key) return t(key)
    return conclusion || status || badge || t('git.badgeUnknown')
  }

  // Show guidance only for blocked/missing states — not when we have a successful list.
  const showGuidance =
    !data.ok ||
    (data.availability !== 'ok' &&
      data.availability !== 'empty' &&
      data.runs.length === 0)
  const guidance = showGuidance
    ? guidanceFor(data.availability, data.message, t)
    : null

  return (
    <div
      data-testid="git-actions-view"
      className="flex h-full min-h-0 flex-col overflow-hidden"
    >
      <div
        data-testid="git-actions-toolbar"
        className="flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2"
      >
        <Workflow size={SHELL.iconXs} className="text-shell-muted" />
        <span className="min-w-0 flex-1 truncate text-[11px] text-shell-text">
          {data.owner && data.repo
            ? `${data.owner}/${data.repo}`
            : t('git.actionsTitle')}
          {data.fetched_at ? (
            <span className="ml-2 text-shell-muted" data-testid="git-actions-fetched-at">
              · {formatFetched(data.fetched_at)}
            </span>
          ) : null}
        </span>
        <button
          type="button"
          data-testid="git-actions-refresh"
          title={t('git.actionsRefresh')}
          disabled={busy}
          onClick={() => void load()}
          className="rounded p-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
        >
          <RefreshCw size={SHELL.iconSm} className={busy ? 'animate-spin' : ''} />
        </button>
      </div>

      {error && (
        <div
          data-testid="git-actions-error"
          className="shrink-0 border-b border-rose-900/50 bg-rose-950/30 px-2 py-1.5 text-[11px] text-rose-200"
        >
          {error}
        </div>
      )}

      {busy && data.runs.length === 0 && !guidance && (
        <div className="p-3 text-[11px] text-shell-muted" data-testid="git-actions-loading">
          {t('git.actionsLoading')}
        </div>
      )}

      {guidance && (
        <div
          data-testid="git-actions-empty"
          data-availability={data.availability}
          className="flex flex-1 flex-col items-center justify-center gap-2 px-6 text-center"
        >
          <p className="max-w-md text-[12px] text-shell-muted">{guidance}</p>
          {data.detail && (
            <pre
              data-testid="git-actions-detail"
              className="max-h-20 max-w-full overflow-auto whitespace-pre-wrap font-mono text-[10px] text-shell-muted/80"
            >
              {data.detail}
            </pre>
          )}
          {(data.availability === 'no_auth' ||
            data.availability === 'auth_failed') && (
            <p className="max-w-md text-[11px] text-shell-muted/90">
              {t('git.actionsAuthHint')}
            </p>
          )}
        </div>
      )}

      {!guidance && data.runs.length > 0 && (
        <ul
          data-testid="git-actions-list"
          className="min-h-0 flex-1 list-none overflow-y-auto p-0 m-0"
        >
          {data.runs.map((run) => (
            <li
              key={run.id}
              data-testid={`git-actions-run-${run.id}`}
              data-badge={run.badge}
              className="flex items-start gap-2 border-b border-shell-border/60 px-2 py-1.5 hover:bg-shell-border/20"
            >
              <span
                data-testid={`git-actions-badge-${run.id}`}
                className={`mt-0.5 shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium capitalize ${badgeClass(run.badge)}`}
              >
                {badgeLabel(run.badge, run.status, run.conclusion)}
              </span>
              <div className="min-w-0 flex-1">
                <div
                  data-testid={`git-actions-name-${run.id}`}
                  className="truncate text-[12px] text-shell-text"
                  title={run.display_title || run.name}
                >
                  {run.name}
                  {run.display_title && run.display_title !== run.name ? (
                    <span className="text-shell-muted"> · {run.display_title}</span>
                  ) : null}
                </div>
                <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-shell-muted">
                  {run.branch && (
                    <span data-testid={`git-actions-branch-${run.id}`} className="font-mono">
                      {run.branch}
                    </span>
                  )}
                  {run.event && <span>{run.event}</span>}
                  {run.relative_time && (
                    <span data-testid={`git-actions-time-${run.id}`}>
                      {run.relative_time}
                    </span>
                  )}
                </div>
              </div>
              {run.html_url && (
                <a
                  href={run.html_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  data-testid={`git-actions-open-${run.id}`}
                  title={t('git.actionsOpen')}
                  onClick={(e) => {
                    // Prefer OS open via core when available (desktop); keep href for accessibility.
                    e.preventDefault()
                    void onOpen(run)
                  }}
                  className="mt-0.5 shrink-0 rounded p-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-accent"
                >
                  <ExternalLink size={SHELL.iconXs} />
                </a>
              )}
            </li>
          ))}
        </ul>
      )}

      {!guidance && !busy && data.runs.length === 0 && data.ok && (
        <div
          data-testid="git-actions-empty"
          data-availability="empty"
          className="flex flex-1 items-center justify-center px-6 text-center text-[12px] text-shell-muted"
        >
          {data.message || t('git.actionsEmpty')}
        </div>
      )}
    </div>
  )
}

function guidanceFor(
  availability: string,
  message: string,
  t: (key: MessageKey, vars?: Record<string, string | number>) => string,
): string | null {
  switch (availability) {
    case 'ok':
      return null
    case 'empty':
      // list empty is handled separately when ok
      return message || t('git.actionsEmpty')
    case 'not_repo':
      return message || t('git.actionsNotRepo')
    case 'no_remote':
      return message || t('git.actionsNoRemote')
    case 'not_github':
      return message || t('git.actionsNotGithub')
    case 'no_auth':
    case 'auth_failed':
    case 'fetch_error':
      return message || t('git.actionsLoadFailed')
    default:
      return message || null
  }
}

function formatFetched(iso: string): string {
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso
    return d.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return iso
  }
}
