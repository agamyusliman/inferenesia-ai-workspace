import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ArrowDownToLine,
  ArrowUpFromLine,
  Check,
  Download,
  FileDiff,
  GitBranch,
  GitCommitHorizontal,
  GitGraph,
  Minus,
  Plus,
  RefreshCw,
  Settings2,
  Workflow,
} from 'lucide-react'
import {
  getGitDiff,
  getGitStatus,
  gitCommit,
  gitFetch,
  gitPull,
  gitPush,
  gitStage,
  gitUnstage,
  type GitDiffResult,
  type GitFileStatus,
  type GitOpResult,
  type GitRepoRef,
  type GitStatus,
} from '../../lib/api'
import { ConfirmDialog } from '../explorer/ConfirmDialog'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'
import { VerticalSplitter } from '../shell/VerticalSplitter'
import {
  countDiffStats,
  parseDiffLines,
  partitionGitFiles,
  type DiffLine,
} from './gitDiffLines'
import { clampLeft, loadGitPanelLayout, saveGitPanelLayout } from './gitLayout'
import { GitActionsView } from './GitActionsView'
import { GitBranchesView } from './GitBranchesView'
import { GitGraphView } from './GitGraphView'
import { GitWorkflowView } from './GitWorkflowView'

type RightTab = 'diff' | 'graph' | 'branches' | 'workflow' | 'actions'

type Props = {
  refreshKey?: number
  workspaceId?: string
  onOpenPath?: (path: string) => void
  /** Deep-link from status bar (e.g. open Branches / Graph). */
  focusTab?: RightTab | null
  onFocusTabConsumed?: () => void
  /** Notify app shell footer of status (branch, dirty counts). */
  onStatusChange?: (st: GitStatus) => void
}

const EMPTY: GitStatus = {
  branch: '',
  is_repo: false,
  root: '',
  files: [],
  staged_count: 0,
  unstaged_count: 0,
  untracked_count: 0,
  porcelain: '',
  repos: [],
}

type RemoteBanner = {
  kind: 'success' | 'error' | 'info'
  text: string
  detail?: string
}

function statusColor(label: string): string {
  switch (label) {
    case 'A':
      return 'text-emerald-400'
    case 'D':
      return 'text-rose-400'
    case '?':
      return 'text-sky-400'
    case 'R':
    case 'C':
      return 'text-violet-400'
    case 'U':
      return 'text-amber-400'
    default:
      return 'text-amber-300'
  }
}

function lineClass(kind: DiffLine['kind']): string {
  switch (kind) {
    case 'add':
      return 'bg-emerald-950/50 text-emerald-200'
    case 'del':
      return 'bg-rose-950/50 text-rose-200'
    case 'hunk':
      return 'bg-sky-950/40 text-sky-300'
    case 'meta':
      return 'text-shell-muted'
    default:
      return 'text-shell-text/90'
  }
}

function fileBase(path: string) {
  const parts = path.split('/')
  return parts[parts.length - 1] || path
}

export function GitPanel({
  refreshKey,
  onOpenPath,
  focusTab,
  onFocusTabConsumed,
  onStatusChange,
}: Props) {
  const { t } = useLocale()
  const [status, setStatus] = useState<GitStatus>(EMPTY)
  const [repoID, setRepoID] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<{
    path: string
    staged: boolean
  } | null>(null)
  const [diff, setDiff] = useState<GitDiffResult | null>(null)
  const [diffBusy, setDiffBusy] = useState(false)
  const [commitMsg, setCommitMsg] = useState('')
  const [opBusy, setOpBusy] = useState(false)
  const [banner, setBanner] = useState<RemoteBanner | null>(null)
  const [rightTab, setRightTab] = useState<RightTab>('diff')
  const [leftWidth, setLeftWidth] = useState(() => loadGitPanelLayout().leftWidth)
  const [graphKey, setGraphKey] = useState(0)
  /** UI push confirm dialog (VAL-GIT-011); force push is gated separately. */
  const [pushConfirm, setPushConfirm] = useState(false)

  useEffect(() => {
    if (!focusTab) return
    setRightTab(focusTab)
    onFocusTabConsumed?.()
  }, [focusTab, onFocusTabConsumed])

  const setLeft = useCallback((w: number) => {
    const next = clampLeft(w)
    setLeftWidth(next)
    saveGitPanelLayout({ leftWidth: next })
  }, [])

  const refresh = useCallback(
    async (id?: string) => {
      setBusy(true)
      try {
        const st = await getGitStatus(id ?? repoID)
        setStatus(st)
        onStatusChange?.(st)
        if (st.repo_id && st.repo_id !== repoID) {
          setRepoID(st.repo_id)
        } else if (!repoID && st.repo_id) {
          setRepoID(st.repo_id)
        } else if (!repoID && st.repos && st.repos[0]) {
          setRepoID(st.repos[0].id)
        }
        setError(null)
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
        setStatus(EMPTY)
      } finally {
        setBusy(false)
      }
    },
    [repoID, onStatusChange],
  )

  useEffect(() => {
    void refresh(repoID)
  }, [refreshKey]) // eslint-disable-line react-hooks/exhaustive-deps -- external key

  useEffect(() => {
    void refresh(repoID)
  }, [repoID]) // eslint-disable-line react-hooks/exhaustive-deps

  const loadDiff = useCallback(
    async (path: string, staged: boolean) => {
      setRightTab('diff')
      setDiffBusy(true)
      setSelected({ path, staged })
      try {
        const d = await getGitDiff(path, staged, repoID)
        setDiff(d)
      } catch (e) {
        setDiff({
          path,
          staged,
          content: '',
          empty: true,
          message: e instanceof Error ? e.message : String(e),
        })
      } finally {
        setDiffBusy(false)
      }
    },
    [repoID],
  )

  const applyOpStatus = useCallback(
    (res: GitOpResult) => {
      if (res.status) {
        setStatus(res.status)
        if (res.status.repo_id) setRepoID(res.status.repo_id)
      } else {
        void refresh(repoID)
      }
      setGraphKey((k) => k + 1)
    },
    [refresh, repoID],
  )

  const onStage = async (path: string) => {
    setOpBusy(true)
    setBanner(null)
    try {
      const res = await gitStage({ path, repo_id: repoID })
      applyOpStatus(res)
      setBanner({
        kind: res.ok ? 'success' : 'error',
        text: res.message,
        detail: res.detail,
      })
      if (res.ok && selected?.path === path) void loadDiff(path, true)
    } catch (e) {
      setBanner({ kind: 'error', text: e instanceof Error ? e.message : String(e) })
    } finally {
      setOpBusy(false)
    }
  }

  const onUnstage = async (path: string) => {
    setOpBusy(true)
    setBanner(null)
    try {
      const res = await gitUnstage({ path, repo_id: repoID })
      applyOpStatus(res)
      setBanner({
        kind: res.ok ? 'success' : 'error',
        text: res.message,
        detail: res.detail,
      })
      if (res.ok && selected?.path === path) void loadDiff(path, false)
    } catch (e) {
      setBanner({ kind: 'error', text: e instanceof Error ? e.message : String(e) })
    } finally {
      setOpBusy(false)
    }
  }

  const onCommit = async () => {
    const msg = commitMsg.trim()
    if (!msg) {
      setBanner({ kind: 'error', text: 'Enter a commit message' })
      return
    }
    setOpBusy(true)
    setBanner(null)
    try {
      const res = await gitCommit({ message: msg, repo_id: repoID })
      applyOpStatus(res)
      if (res.ok) {
        setCommitMsg('')
        setBanner({
          kind: 'success',
          text: res.message,
          detail: res.hash ? `hash ${res.hash}` : res.detail,
        })
        setDiff(null)
        setSelected(null)
      } else {
        setBanner({ kind: 'error', text: res.message, detail: res.detail })
      }
    } catch (e) {
      setBanner({ kind: 'error', text: e instanceof Error ? e.message : String(e) })
    } finally {
      setOpBusy(false)
    }
  }

  const onFetch = async () => {
    setOpBusy(true)
    setBanner(null)
    try {
      const res = await gitFetch(repoID)
      applyOpStatus(res)
      setBanner({
        kind: res.ok ? 'success' : 'error',
        text: res.message,
        detail: res.detail,
      })
    } catch (e) {
      setBanner({ kind: 'error', text: e instanceof Error ? e.message : String(e) })
    } finally {
      setOpBusy(false)
    }
  }

  const onPull = async () => {
    setOpBusy(true)
    setBanner(null)
    try {
      const res = await gitPull(repoID)
      applyOpStatus(res)
      setBanner({
        kind: res.ok ? 'success' : 'error',
        text: res.message,
        detail: res.detail,
      })
    } catch (e) {
      setBanner({ kind: 'error', text: e instanceof Error ? e.message : String(e) })
    } finally {
      setOpBusy(false)
    }
  }

  const onPushConfirmed = async () => {
    setPushConfirm(false)
    setOpBusy(true)
    setBanner(null)
    try {
      // Non-force push only from this button (VAL-GIT-011). Force is never silent.
      const res = await gitPush({
        repo_id: repoID || undefined,
        confirmed: true,
        force: false,
      })
      applyOpStatus(res)
      setBanner({
        kind: res.ok ? 'success' : 'error',
        text: res.message,
        detail: res.detail,
      })
    } catch (e) {
      setBanner({ kind: 'error', text: e instanceof Error ? e.message : String(e) })
    } finally {
      setOpBusy(false)
    }
  }

  const { staged, unstaged } = useMemo(
    () => partitionGitFiles(status.files || []),
    [status.files],
  )
  const diffLines = useMemo(
    () => parseDiffLines(diff?.content || ''),
    [diff?.content],
  )
  const diffStats = useMemo(() => countDiffStats(diffLines), [diffLines])
  const repos: GitRepoRef[] = status.repos || []
  const multi = repos.length > 1

  const tabs: { id: RightTab; label: string; Icon: typeof FileDiff }[] = [
    { id: 'diff', label: t('git.tabDiff'), Icon: FileDiff },
    { id: 'graph', label: t('git.tabGraph'), Icon: GitGraph },
    { id: 'branches', label: t('git.tabBranches'), Icon: GitBranch },
    { id: 'actions', label: t('git.tabActions'), Icon: Workflow },
    { id: 'workflow', label: t('git.tabWorkflow'), Icon: Settings2 },
  ]

  return (
    <div
      data-testid="git-panel"
      className="flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      {pushConfirm && (
        <div data-testid="git-push-confirm">
          <ConfirmDialog
            title={t('git.pushConfirmTitle')}
            message={t('git.pushConfirmMessage', {
              branch: status.branch ? ` (${status.branch})` : '',
            })}
            confirmLabel={t('git.push')}
            cancelLabel={t('action.cancel')}
            danger={false}
            onConfirm={() => void onPushConfirmed()}
            onCancel={() => setPushConfirm(false)}
          />
        </div>
      )}
      <PanelHeader
        testId="git-panel-header"
        title={t('git.title')}
        subtitle={
          status.is_repo
            ? `${status.repo_name || status.repo_id || 'repo'} · ${status.branch || '—'}`
            : multi
              ? t('git.reposCount', { count: repos.length })
              : t('git.noRepo')
        }
        actions={
          <button
            type="button"
            data-testid="git-refresh"
            title={t('git.refreshTitle')}
            disabled={busy || opBusy}
            onClick={() => void refresh(repoID)}
            className="rounded p-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
          >
            <RefreshCw size={SHELL.iconSm} className={busy ? 'animate-spin' : ''} />
          </button>
        }
      />

      {/* Multi-repo source control picker */}
      {repos.length > 0 && (
        <div
          data-testid="git-repo-list"
          className="flex shrink-0 gap-1 overflow-x-auto border-b border-shell-border bg-shell-panel px-2 py-1"
        >
          {repos.map((r) => {
            const active = (repoID || status.repo_id) === r.id
            return (
              <button
                key={r.id}
                type="button"
                data-testid={`git-repo-${r.id}`}
                onClick={() => {
                  setRepoID(r.id)
                  setSelected(null)
                  setDiff(null)
                }}
                className={`inline-flex shrink-0 items-center gap-1 rounded px-2 py-0.5 text-[11px] ${
                  active
                    ? 'bg-shell-active text-shell-text'
                    : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
                }`}
                title={r.root}
              >
                <span className="font-medium">{r.name}</span>
                {r.branch && (
                  <span className="font-mono text-[10px] opacity-70">{r.branch}</span>
                )}
                {(r.dirty_count ?? 0) > 0 && (
                  <span className="rounded bg-amber-500/20 px-1 font-mono text-[9px] text-amber-300">
                    {r.dirty_count}
                  </span>
                )}
              </button>
            )
          })}
        </div>
      )}

      <div data-testid="git-toolbar" className={SHELL.toolbarClass}>
        <span
          data-testid="git-branch"
          className="flex min-w-0 items-center gap-1 truncate text-[12px] text-shell-text"
          title={status.branch}
        >
          <GitBranch size={SHELL.iconXs} className="shrink-0 text-shell-accent" />
          <span className="truncate font-medium">{status.branch || '—'}</span>
        </span>
        <button
          type="button"
          data-testid="git-open-branches"
          className="shrink-0 text-[10px] text-shell-accent underline"
          onClick={() => setRightTab('branches')}
        >
          {t('git.switch')}
        </button>
        <span
          data-testid="git-counts"
          className="shrink-0 text-[11px] text-shell-muted"
        >
          {t('git.counts', {
            staged: status.staged_count,
            unstaged: status.unstaged_count,
          })}
        </span>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <button
            type="button"
            data-testid="git-fetch"
            disabled={opBusy || !status.is_repo}
            onClick={() => void onFetch()}
            className="inline-flex items-center gap-1 rounded border border-shell-border px-2 py-0.5 text-[11px] text-shell-text hover:bg-shell-border/30 disabled:opacity-40"
          >
            <Download size={SHELL.iconXs} />
            {t('git.fetch')}
          </button>
          <button
            type="button"
            data-testid="git-pull"
            disabled={opBusy || !status.is_repo}
            onClick={() => void onPull()}
            className="inline-flex items-center gap-1 rounded border border-shell-border px-2 py-0.5 text-[11px] text-shell-text hover:bg-shell-border/30 disabled:opacity-40"
          >
            <ArrowDownToLine size={SHELL.iconXs} />
            {t('git.pull')}
          </button>
          <button
            type="button"
            data-testid="git-push"
            disabled={opBusy || !status.is_repo}
            onClick={() => setPushConfirm(true)}
            className="inline-flex items-center gap-1 rounded border border-shell-border px-2 py-0.5 text-[11px] text-shell-text hover:bg-shell-border/30 disabled:opacity-40"
            title={t('git.pushTitle')}
          >
            <ArrowUpFromLine size={SHELL.iconXs} />
            {t('git.push')}
          </button>
        </div>
      </div>

      {banner && (
        <div
          data-testid="git-remote-status"
          data-status={banner.kind}
          className={`shrink-0 border-b px-2 py-1.5 text-[11px] ${
            banner.kind === 'success'
              ? 'border-emerald-900/60 bg-emerald-950/40 text-emerald-200'
              : banner.kind === 'error'
                ? 'border-rose-900/60 bg-rose-950/40 text-rose-200'
                : 'border-shell-border bg-shell-panel text-shell-muted'
          }`}
        >
          <div className="font-medium" data-testid="git-remote-status-text">
            {banner.text}
          </div>
          {banner.detail && (
            <pre
              data-testid="git-remote-status-detail"
              className="mt-0.5 max-h-16 overflow-auto whitespace-pre-wrap font-mono text-[10px] opacity-80"
            >
              {banner.detail}
            </pre>
          )}
        </div>
      )}

      {error && (
        <div
          data-testid="git-error"
          className="shrink-0 border-b border-rose-900/50 bg-rose-950/30 px-2 py-1 text-[11px] text-rose-200"
        >
          {error}
        </div>
      )}

      {!status.is_repo && !error && repos.length === 0 && (
        <div
          data-testid="git-not-repo"
          className="flex flex-1 items-center justify-center px-6 text-center text-[12px] text-shell-muted"
        >
          {t('git.openWorkspaceRepo')}
        </div>
      )}

      {status.is_repo && (
        <div className="flex min-h-0 flex-1 overflow-hidden">
          {/* Left: changes + commit (resizable) */}
          <div
            data-testid="git-changes"
            style={{ width: leftWidth, flex: '0 0 auto' }}
            className="flex min-h-0 min-w-0 flex-col overflow-hidden border-r border-shell-border"
          >
            <section className="min-h-0 flex-1 overflow-y-auto">
              <SectionTitle
                title={t('git.stagedChanges')}
                count={staged.length}
                testId="git-staged-section"
              />
              {staged.length === 0 ? (
                <EmptyRow text={t('git.noStaged')} testId="git-staged-empty" />
              ) : (
                staged.map((f) => (
                  <FileRow
                    key={`s:${f.path}`}
                    file={f}
                    selected={selected?.path === f.path && selected.staged}
                    onSelect={() => void loadDiff(f.path, true)}
                    onPrimary={() => void onUnstage(f.path)}
                    primaryLabel={t('git.unstage')}
                    primaryTestId={`git-unstage-${f.path}`}
                    primaryIcon="minus"
                    disabled={opBusy}
                    onOpenPath={onOpenPath}
                  />
                ))
              )}
              <SectionTitle
                title={t('git.changes')}
                count={unstaged.length}
                testId="git-unstaged-section"
              />
              {unstaged.length === 0 ? (
                <EmptyRow text={t('git.noUnstaged')} testId="git-unstaged-empty" />
              ) : (
                unstaged.map((f) => (
                  <FileRow
                    key={`u:${f.path}`}
                    file={f}
                    selected={selected?.path === f.path && !selected.staged}
                    onSelect={() => void loadDiff(f.path, false)}
                    onPrimary={() => void onStage(f.path)}
                    primaryLabel={t('git.stage')}
                    primaryTestId={`git-stage-${f.path}`}
                    primaryIcon="plus"
                    disabled={opBusy}
                    onOpenPath={onOpenPath}
                  />
                ))
              )}
            </section>
            <div
              data-testid="git-commit-box"
              className="shrink-0 border-t border-shell-border bg-shell-panel p-2"
            >
              <textarea
                id="git-commit-message"
                data-testid="git-commit-message"
                value={commitMsg}
                onChange={(e) => setCommitMsg(e.target.value)}
                rows={2}
                placeholder={t('git.commitMessage')}
                className="w-full resize-none rounded border border-shell-border bg-shell-bg px-2 py-1.5 font-mono text-[12px] text-shell-text placeholder:text-shell-muted focus:border-shell-accent focus:outline-none"
              />
              <button
                type="button"
                data-testid="git-commit"
                disabled={
                  opBusy || status.staged_count === 0 || !commitMsg.trim()
                }
                onClick={() => void onCommit()}
                className="mt-1.5 inline-flex w-full items-center justify-center gap-1.5 rounded bg-shell-accent px-2 py-1.5 text-[12px] font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
              >
                <GitCommitHorizontal size={SHELL.iconSm} />
                {t('git.commit')}
                {status.staged_count > 0 ? ` (${status.staged_count})` : ''}
              </button>
            </div>
          </div>

          <VerticalSplitter
            testId="splitter-git"
            value={leftWidth}
            onChange={setLeft}
            growSide="left"
            minOpposite={280}
            aria-label={t('git.resizePanel')}
          />

          {/* Right: tabs Diff | Graph | Branches | Workflow */}
          <div
            data-testid="git-right-pane"
            className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
          >
            <div
              data-testid="git-right-tabs"
              className="flex h-10 shrink-0 items-center gap-0.5 border-b border-shell-border bg-shell-panel px-1"
            >
              {tabs.map((t) => {
                const { Icon } = t
                const active = rightTab === t.id
                return (
                  <button
                    key={t.id}
                    type="button"
                    data-testid={`git-tab-${t.id}`}
                    onClick={() => setRightTab(t.id)}
                    className={`inline-flex items-center gap-1 rounded px-2 py-1 text-[11px] ${
                      active
                        ? 'bg-shell-active text-shell-text'
                        : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
                    }`}
                  >
                    <Icon size={SHELL.iconXs} />
                    {t.label}
                  </button>
                )
              })}
            </div>

            <div className="min-h-0 flex-1 overflow-hidden">
              {rightTab === 'diff' && (
                <div
                  data-testid="git-diff-view"
                  className="flex h-full min-h-0 flex-col overflow-hidden"
                >
                  <div className="flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2">
                    <FileDiff size={SHELL.iconXs} className="text-shell-muted" />
                    <span
                      data-testid="git-diff-path"
                      className="min-w-0 flex-1 truncate text-[11px] text-shell-text"
                    >
                      {selected
                        ? `${selected.path}${selected.staged ? t('git.stagedSuffix') : ''}`
                        : t('git.selectFileDiff')}
                    </span>
                    {selected && !diff?.empty && (
                      <span
                        data-testid="git-diff-stats"
                        className="shrink-0 font-mono text-[10px]"
                      >
                        <span className="text-emerald-400">
                          +{diffStats.additions}
                        </span>{' '}
                        <span className="text-rose-400">
                          −{diffStats.deletions}
                        </span>
                      </span>
                    )}
                  </div>
                  <div className="min-h-0 flex-1 overflow-auto font-mono text-[11px] leading-5">
                    {diffBusy && (
                      <div className="p-3 text-shell-muted" data-testid="git-diff-loading">
                        {t('git.loadingDiff')}
                      </div>
                    )}
                    {!diffBusy && !selected && (
                      <div className="p-3 text-shell-muted" data-testid="git-diff-empty">
                        {t('git.noFileSelected')}
                      </div>
                    )}
                    {!diffBusy && selected && diff?.empty && (
                      <div
                        className="p-3 text-shell-muted"
                        data-testid="git-diff-no-changes"
                      >
                        {diff.message || t('git.noDifferences')}
                      </div>
                    )}
                    {!diffBusy && selected && diff && !diff.empty && (
                      <pre
                        data-testid="git-diff-content"
                        className="m-0 whitespace-pre px-0 py-1"
                      >
                        {diffLines.map((line, i) => (
                          <div
                            key={i}
                            data-diff-kind={line.kind}
                            className={`px-2 ${lineClass(line.kind)}`}
                          >
                            {line.text || ' '}
                          </div>
                        ))}
                      </pre>
                    )}
                  </div>
                </div>
              )}
              {rightTab === 'graph' && (
                <GitGraphView
                  repoID={repoID}
                  refreshKey={graphKey + (refreshKey || 0)}
                />
              )}
              {rightTab === 'branches' && (
                <GitBranchesView
                  repoID={repoID}
                  refreshKey={graphKey + (refreshKey || 0)}
                  onCheckedOut={(res) => {
                    applyOpStatus(res)
                    setBanner({
                      kind: res.ok ? 'success' : 'error',
                      text: res.message,
                      detail: res.detail,
                    })
                  }}
                />
              )}
              {rightTab === 'actions' && (
                <GitActionsView
                  repoID={repoID || status.repo_id || '.'}
                  refreshKey={graphKey + (refreshKey || 0)}
                />
              )}
              {rightTab === 'workflow' && (
                <GitWorkflowView
                  repoID={repoID || status.repo_id || '.'}
                  onSaved={(msg) =>
                    setBanner({ kind: 'success', text: msg })
                  }
                />
              )}
            </div>
          </div>
        </div>
      )}

      {status.message && status.is_repo && (
        <div
          data-testid="git-status-message"
          className="shrink-0 border-t border-shell-border px-2 py-1 text-[10px] text-shell-muted"
        >
          {status.message}
        </div>
      )}
    </div>
  )
}

function SectionTitle({
  title,
  count,
  testId,
}: {
  title: string
  count: number
  testId: string
}) {
  return (
    <div
      data-testid={testId}
      className="sticky top-0 z-[1] flex items-center gap-2 bg-shell-panel/95 px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted backdrop-blur"
    >
      <span>{title}</span>
      <span className="rounded bg-shell-border/50 px-1 font-mono text-[10px] normal-case">
        {count}
      </span>
    </div>
  )
}

function EmptyRow({ text, testId }: { text: string; testId: string }) {
  return (
    <div data-testid={testId} className="px-3 py-2 text-[11px] text-shell-muted">
      {text}
    </div>
  )
}

function FileRow({
  file,
  selected,
  onSelect,
  onPrimary,
  primaryLabel,
  primaryTestId,
  primaryIcon,
  disabled,
  onOpenPath,
}: {
  file: GitFileStatus
  selected: boolean
  onSelect: () => void
  onPrimary: () => void
  primaryLabel: string
  primaryTestId: string
  primaryIcon: 'plus' | 'minus'
  disabled?: boolean
  onOpenPath?: (path: string) => void
}) {
  return (
    <div
      data-testid={`git-file-${file.path}`}
      data-staged={file.staged ? 'true' : 'false'}
      data-unstaged={file.unstaged || file.untracked ? 'true' : 'false'}
      data-untracked={file.untracked ? 'true' : 'false'}
      data-selected={selected ? 'true' : 'false'}
      className={`group flex items-center gap-1 border-b border-shell-border/40 px-1 py-0.5 text-[12px] ${
        selected ? 'bg-shell-active/80' : 'hover:bg-shell-border/20'
      }`}
    >
      <button
        type="button"
        className="flex min-w-0 flex-1 items-center gap-1.5 px-1 py-1 text-left"
        onClick={onSelect}
        title={file.path}
      >
        <span
          data-testid={`git-file-label-${file.path}`}
          className={`w-3 shrink-0 font-mono text-[11px] font-semibold ${statusColor(file.status_label)}`}
        >
          {file.status_label}
        </span>
        <span className="min-w-0 truncate text-shell-text" title={file.path}>
          {fileBase(file.path)}
        </span>
        {file.path.includes('/') && (
          <span className="hidden min-w-0 truncate text-[10px] text-shell-muted sm:inline">
            {file.path}
          </span>
        )}
      </button>
      {onOpenPath && (
        <button
          type="button"
          title="Open file"
          className="hidden rounded p-1 text-shell-muted hover:bg-shell-border/40 group-hover:inline-flex"
          onClick={() => onOpenPath(file.path)}
        >
          <Check size={SHELL.iconXs} className="opacity-0" aria-hidden />
          <span className="sr-only">Open</span>
        </button>
      )}
      <button
        type="button"
        data-testid={primaryTestId}
        title={primaryLabel}
        disabled={disabled}
        onClick={onPrimary}
        className="inline-flex items-center gap-0.5 rounded border border-shell-border px-1.5 py-0.5 text-[10px] text-shell-text hover:bg-shell-border/40 disabled:opacity-40"
      >
        {primaryIcon === 'plus' ? (
          <Plus size={SHELL.iconXs} />
        ) : (
          <Minus size={SHELL.iconXs} />
        )}
        {primaryLabel}
      </button>
    </div>
  )
}
