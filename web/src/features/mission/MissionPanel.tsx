import { useCallback, useEffect, useState } from 'react'
import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  Flag,
  Loader2,
  Plus,
  RefreshCw,
  ShieldCheck,
  X,
  XCircle,
} from 'lucide-react'
import {
  activateMission,
  attachMissionEvidence,
  blockMission,
  cancelMission,
  completeMission,
  createMission,
  deleteMission,
  listMissions,
  runMissionGates,
  type MissionView,
} from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'

type Props = {
  /** Bump on MissionStatus events (VAL-MISSION-006) — no manual refresh needed. */
  refreshKey?: number
  workspaceId?: string
  onClose?: () => void
}

function statusTone(status: string): string {
  switch (status) {
    case 'completed':
      return 'text-emerald-300 bg-emerald-950/50 border-emerald-800/60'
    case 'blocked':
      return 'text-rose-200 bg-rose-950/50 border-rose-800/60'
    case 'cancelled':
      return 'text-zinc-300 bg-zinc-900/60 border-zinc-700/60'
    case 'verifying':
      return 'text-amber-200 bg-amber-950/40 border-amber-800/50'
    case 'active':
      return 'text-sky-200 bg-sky-950/40 border-sky-800/50'
    default:
      return 'text-shell-muted bg-shell-panel border-shell-border'
  }
}

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return <CheckCircle2 size={SHELL.iconSm} className="text-emerald-400" aria-hidden />
    case 'blocked':
      return <XCircle size={SHELL.iconSm} className="text-rose-400" aria-hidden />
    case 'cancelled':
      return <XCircle size={SHELL.iconSm} className="text-zinc-400" aria-hidden />
    case 'verifying':
      return <Loader2 size={SHELL.iconSm} className="animate-spin text-amber-300" aria-hidden />
    case 'active':
      return <Flag size={SHELL.iconSm} className="text-sky-300" aria-hidden />
    default:
      return <Circle size={SHELL.iconSm} className="text-shell-muted" aria-hidden />
  }
}

export function MissionPanel({ refreshKey, workspaceId, onClose }: Props) {
  const { t } = useLocale()
  const [missions, setMissions] = useState<MissionView[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [banner, setBanner] = useState<string | null>(null)

  // Create form
  const [showCreate, setShowCreate] = useState(false)
  const [goal, setGoal] = useState('')
  const [budget, setBudget] = useState('')

  // Evidence form
  const [evSummary, setEvSummary] = useState('')
  const [evContent, setEvContent] = useState('')
  const [evCommand, setEvCommand] = useState('')

  // Block form
  const [blockReason, setBlockReason] = useState('')
  const [showBlock, setShowBlock] = useState(false)

  const refresh = useCallback(async () => {
    setBusy(true)
    try {
      const list = await listMissions()
      setMissions(list.missions || [])
      setError(null)
      // Keep selection if still present.
      setSelectedId((prev) => {
        if (prev && (list.missions || []).some((m) => m.id === prev)) return prev
        return (list.missions && list.missions[0]?.id) || null
      })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh, refreshKey])

  const selected = missions.find((m) => m.id === selectedId) || null

  const onCreate = async () => {
    const g = goal.trim()
    if (!g) {
      setBanner('Goal is required')
      return
    }
    setBusy(true)
    setBanner(null)
    try {
      const budgetTokens = budget.trim() ? Number(budget) : 0
      const res = await createMission({
        goal: g,
        budget_tokens: Number.isFinite(budgetTokens) ? budgetTokens : 0,
        workspace_id: workspaceId,
      })
      setGoal('')
      setBudget('')
      setShowCreate(false)
      setSelectedId(res.mission.id)
      await refresh()
      setBanner(`Created mission · ${res.mission.status}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onComplete = async () => {
    if (!selected) return
    setBusy(true)
    setBanner(null)
    try {
      const res = await completeMission(selected.id)
      if (!res.ok) {
        // VAL-MISSION-002: evidence required message shown; status stays non-complete.
        setBanner(res.message || res.error || 'evidence required')
        await refresh()
        return
      }
      setBanner('Mission completed')
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onAttachEvidence = async () => {
    if (!selected) return
    const summary = evSummary.trim()
    if (!summary) {
      setBanner('Evidence summary is required')
      return
    }
    setBusy(true)
    setBanner(null)
    try {
      await attachMissionEvidence({
        id: selected.id,
        kind: evCommand.trim() ? 'command' : 'note',
        summary,
        content: evContent,
        command: evCommand,
      })
      setEvSummary('')
      setEvContent('')
      setEvCommand('')
      setBanner('Evidence attached')
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onRunGates = async () => {
    if (!selected) return
    setBusy(true)
    setBanner(null)
    try {
      const res = await runMissionGates(selected.id)
      const failed = (res.mission.gates || []).filter((g) => !g.passed)
      if (failed.length) {
        setBanner(
          `Gate failed: ${failed.map((g) => g.name).join(', ')} — completion blocked`,
        )
      } else {
        setBanner('All verification gates passed')
      }
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onBlock = async () => {
    if (!selected) return
    const reason = blockReason.trim()
    if (!reason) {
      setBanner('Blocker reason is required')
      return
    }
    setBusy(true)
    try {
      await blockMission(selected.id, reason)
      setBlockReason('')
      setShowBlock(false)
      setBanner('Mission blocked')
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onActivate = async () => {
    if (!selected) return
    setBusy(true)
    try {
      await activateMission(selected.id)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onCancel = async () => {
    if (!selected) return
    setBusy(true)
    setBanner(null)
    try {
      await cancelMission(selected.id)
      setBanner('Mission cancelled')
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onDelete = async () => {
    if (!selected) return
    setBusy(true)
    setBanner(null)
    try {
      await deleteMission(selected.id)
      setSelectedId(null)
      setBanner('Mission deleted')
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section
      data-testid="mission-panel"
      data-count={missions.length}
      data-selected={selectedId || ''}
      aria-label={t('mission.title')}
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader
        testId="mission-panel-header"
        title={t('mission.title')}
        dense
        actions={
          <div className="flex shrink-0 items-center gap-0.5">
            <button
              type="button"
              data-testid="mission-create-toggle"
              title={t('action.new')}
              aria-label={t('action.new')}
              onClick={() => setShowCreate((v) => !v)}
              className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
            >
              <Plus size={SHELL.iconXs} />
            </button>
            <button
              type="button"
              data-testid="mission-refresh"
              title={t('action.refresh')}
              aria-label={t('action.refresh')}
              onClick={() => void refresh()}
              disabled={busy}
              className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
            >
              <RefreshCw size={SHELL.iconXs} className={busy ? 'animate-spin' : ''} />
            </button>
            {onClose && (
              <button
                type="button"
                data-testid="mission-close"
                title={t('panel.closeTitle')}
                aria-label={t('panel.closeTitle')}
                onClick={onClose}
                className="inline-flex h-6 w-6 items-center justify-center rounded text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
              >
                <X size={SHELL.iconXs} aria-hidden />
              </button>
            )}
          </div>
        }
      />

      {banner && (
        <div
          data-testid="mission-banner"
          role="status"
          className={`shrink-0 border-b px-2 py-1.5 text-[11px] ${
            banner.toLowerCase().includes('evidence required') ||
            banner.toLowerCase().includes('gate fail') ||
            banner.toLowerCase().includes('blocked')
              ? 'border-rose-900/50 bg-rose-950/40 text-rose-100'
              : 'border-shell-border bg-shell-panel text-shell-text'
          }`}
        >
          {banner.toLowerCase().includes('evidence required') && (
            <AlertTriangle size={12} className="mr-1 inline text-rose-300" aria-hidden />
          )}
          <span data-testid="mission-banner-text">{banner}</span>
        </div>
      )}

      {error && (
        <div
          data-testid="mission-error"
          className="shrink-0 border-b border-rose-900/50 bg-rose-950/40 px-2 py-1 text-[11px] text-rose-200"
        >
          {error}
        </div>
      )}

      {showCreate && (
        <div
          data-testid="mission-create-form"
          className="shrink-0 space-y-2 border-b border-shell-border bg-shell-panel/80 px-2 py-2"
        >
          <label className="block text-[10px] uppercase tracking-wide text-shell-muted">
            Goal
            <input
              data-testid="mission-goal-input"
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="Mission objective…"
              className="mt-0.5 w-full rounded border border-shell-border bg-shell-bg px-2 py-1 text-[12px] text-shell-text outline-none focus:border-shell-accent"
            />
          </label>
          <label className="block text-[10px] uppercase tracking-wide text-shell-muted">
            Budget tokens (optional)
            <input
              data-testid="mission-budget-input"
              value={budget}
              onChange={(e) => setBudget(e.target.value)}
              placeholder="e.g. 50000"
              inputMode="numeric"
              className="mt-0.5 w-full rounded border border-shell-border bg-shell-bg px-2 py-1 text-[12px] text-shell-text outline-none focus:border-shell-accent"
            />
          </label>
          <button
            type="button"
            data-testid="mission-create-submit"
            disabled={busy || !goal.trim()}
            onClick={() => void onCreate()}
            className="rounded bg-shell-accent/90 px-2.5 py-1 text-[11px] font-medium text-white hover:bg-shell-accent disabled:opacity-40"
          >
            Create mission
          </button>
        </div>
      )}

      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* List */}
        <div
          data-testid="mission-list"
          className="flex w-[40%] min-w-[140px] max-w-[220px] flex-col overflow-y-auto border-r border-shell-border"
        >
          {missions.length === 0 ? (
            <div
              data-testid="mission-empty"
              className="px-2 py-6 text-center text-[12px] text-shell-muted"
            >
              No missions yet. Create one with a goal and optional budget.
            </div>
          ) : (
            <ul className="flex flex-col gap-0.5 p-1">
              {missions.map((m) => (
                <li key={m.id}>
                  <button
                    type="button"
                    data-testid="mission-list-item"
                    data-mission-id={m.id}
                    data-status={m.status}
                    onClick={() => setSelectedId(m.id)}
                    className={`flex w-full items-start gap-1.5 rounded-md border px-1.5 py-1.5 text-left text-[11px] transition ${
                      selectedId === m.id
                        ? 'border-shell-accent/50 bg-shell-active'
                        : 'border-transparent hover:border-shell-border/80 hover:bg-shell-panel/60'
                    }`}
                  >
                    <StatusIcon status={m.status} />
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium text-shell-text">{m.goal}</div>
                      <div
                        data-testid="mission-list-status"
                        className={`mt-0.5 inline-block rounded border px-1 text-[9px] uppercase tracking-wide ${statusTone(m.status)}`}
                      >
                        {m.status}
                      </div>
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Detail */}
        <div
          data-testid="mission-detail"
          className="flex min-w-0 flex-1 flex-col overflow-y-auto px-2 py-2"
        >
          {!selected ? (
            <div className="py-8 text-center text-[12px] text-shell-muted">
              Select a mission
            </div>
          ) : (
            <>
              <div className="mb-2 flex flex-wrap items-center gap-2">
                <span
                  data-testid="mission-status"
                  data-status={selected.status}
                  className={`inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${statusTone(selected.status)}`}
                >
                  <StatusIcon status={selected.status} />
                  {selected.status}
                </span>
                {selected.budget_tokens ? (
                  <span
                    data-testid="mission-budget"
                    className="text-[10px] text-shell-muted"
                  >
                    budget {selected.budget_tokens.toLocaleString()} tokens
                  </span>
                ) : null}
              </div>

              <h3
                data-testid="mission-goal"
                className="mb-1 text-[13px] font-semibold leading-snug text-shell-text"
              >
                {selected.goal}
              </h3>
              <div className="mb-2 text-[10px] text-shell-muted" data-testid="mission-id">
                {selected.id}
              </div>

              {selected.status === 'blocked' && selected.blocker_reason && (
                <div
                  data-testid="mission-blocker"
                  className="mb-2 flex items-start gap-1.5 rounded border border-rose-800/60 bg-rose-950/40 px-2 py-1.5 text-[11px] text-rose-100"
                >
                  <AlertTriangle size={14} className="mt-0.5 shrink-0 text-rose-300" />
                  <div>
                    <div className="font-medium">Blocked</div>
                    <div data-testid="mission-blocker-reason">{selected.blocker_reason}</div>
                  </div>
                </div>
              )}

              {/* Gates */}
              <div className="mb-2" data-testid="mission-gates">
                <div className="mb-1 flex items-center justify-between">
                  <span className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                    Verification gates
                  </span>
                  <button
                    type="button"
                    data-testid="mission-run-gates"
                    disabled={busy || selected.status === 'completed'}
                    onClick={() => void onRunGates()}
                    className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-40"
                  >
                    <ShieldCheck size={12} />
                    Run gates
                  </button>
                </div>
                {(selected.gates || []).length === 0 ? (
                  <div className="text-[11px] text-shell-muted">No gates run yet</div>
                ) : (
                  <ul className="space-y-1">
                    {selected.gates.map((g) => (
                      <li
                        key={g.name}
                        data-testid="mission-gate-row"
                        data-gate={g.name}
                        data-passed={g.passed ? 'true' : 'false'}
                        className="flex items-start gap-1.5 rounded border border-shell-border/60 px-1.5 py-1 text-[11px]"
                      >
                        {g.passed ? (
                          <CheckCircle2 size={12} className="mt-0.5 text-emerald-400" />
                        ) : (
                          <XCircle size={12} className="mt-0.5 text-rose-400" />
                        )}
                        <div className="min-w-0">
                          <div className="font-medium text-shell-text">{g.name}</div>
                          <div className="truncate text-[10px] text-shell-muted">{g.command}</div>
                          <div className="text-[10px] text-shell-muted">
                            exit {g.exit_code}
                            {g.passed ? ' · pass' : ' · fail'}
                          </div>
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              {/* Evidence */}
              <div className="mb-2" data-testid="mission-evidence">
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                  Evidence ({(selected.evidence || []).length})
                </div>
                {(selected.evidence || []).length === 0 ? (
                  <div
                    data-testid="mission-evidence-empty"
                    className="text-[11px] text-shell-muted"
                  >
                    No verification evidence yet. Complete requires evidence.
                  </div>
                ) : (
                  <ul className="mb-2 max-h-28 space-y-1 overflow-y-auto">
                    {selected.evidence.map((e) => (
                      <li
                        key={e.id}
                        data-testid="mission-evidence-item"
                        className="rounded border border-shell-border/50 px-1.5 py-1 text-[11px]"
                      >
                        <div className="font-medium text-shell-text">{e.summary}</div>
                        {e.command && (
                          <div className="truncate text-[10px] text-shell-muted">{e.command}</div>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
                {selected.status !== 'completed' && (
                  <div
                    data-testid="mission-evidence-form"
                    className="space-y-1 rounded border border-shell-border/60 bg-shell-panel/40 p-1.5"
                  >
                    <input
                      data-testid="mission-evidence-summary"
                      value={evSummary}
                      onChange={(e) => setEvSummary(e.target.value)}
                      placeholder="Evidence summary (required)"
                      className="w-full rounded border border-shell-border bg-shell-bg px-1.5 py-1 text-[11px] text-shell-text outline-none focus:border-shell-accent"
                    />
                    <input
                      data-testid="mission-evidence-command"
                      value={evCommand}
                      onChange={(e) => setEvCommand(e.target.value)}
                      placeholder="Command (optional)"
                      className="w-full rounded border border-shell-border bg-shell-bg px-1.5 py-1 text-[11px] text-shell-text outline-none focus:border-shell-accent"
                    />
                    <textarea
                      data-testid="mission-evidence-content"
                      value={evContent}
                      onChange={(e) => setEvContent(e.target.value)}
                      placeholder="Output / notes (secrets redacted server-side)"
                      rows={2}
                      className="w-full resize-none rounded border border-shell-border bg-shell-bg px-1.5 py-1 text-[11px] text-shell-text outline-none focus:border-shell-accent"
                    />
                    <button
                      type="button"
                      data-testid="mission-attach-evidence"
                      disabled={busy || !evSummary.trim()}
                      onClick={() => void onAttachEvidence()}
                      className="rounded bg-shell-border/60 px-2 py-0.5 text-[10px] text-shell-text hover:bg-shell-hover disabled:opacity-40"
                    >
                      Attach evidence
                    </button>
                  </div>
                )}
              </div>

              <div
                data-testid="mission-actions"
                className="mt-auto flex flex-wrap gap-1.5 border-t border-shell-border pt-2"
              >
                {selected.status !== 'completed' &&
                  selected.status !== 'cancelled' && (
                    <>
                      {selected.status === 'pending' && (
                        <button
                          type="button"
                          data-testid="mission-activate"
                          disabled={busy}
                          onClick={() => void onActivate()}
                          className="rounded bg-sky-900/50 px-2 py-1 text-[11px] text-sky-100 hover:bg-sky-800/60 disabled:opacity-40"
                        >
                          Activate
                        </button>
                      )}
                      <button
                        type="button"
                        data-testid="mission-complete"
                        disabled={busy}
                        onClick={() => void onComplete()}
                        className="rounded bg-emerald-800/70 px-2 py-1 text-[11px] font-medium text-emerald-50 hover:bg-emerald-700/80 disabled:opacity-40"
                      >
                        Mark complete
                      </button>
                      <button
                        type="button"
                        data-testid="mission-block-toggle"
                        disabled={busy}
                        onClick={() => setShowBlock((v) => !v)}
                        className="rounded bg-rose-950/50 px-2 py-1 text-[11px] text-rose-100 hover:bg-rose-900/60 disabled:opacity-40"
                      >
                        Mark blocked
                      </button>
                      <button
                        type="button"
                        data-testid="mission-cancel"
                        disabled={busy}
                        onClick={() => void onCancel()}
                        className="rounded border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-border/30 hover:text-shell-text disabled:opacity-40"
                      >
                        Cancel
                      </button>
                    </>
                  )}
                <button
                  type="button"
                  data-testid="mission-delete"
                  disabled={busy}
                  onClick={() => void onDelete()}
                  className="rounded border border-rose-900/50 px-2 py-1 text-[11px] text-rose-200 hover:bg-rose-950/40 disabled:opacity-40"
                >
                  Delete
                </button>
              </div>

              {showBlock &&
                selected.status !== 'completed' &&
                selected.status !== 'cancelled' && (
                <div
                  data-testid="mission-block-form"
                  className="mt-2 space-y-1 rounded border border-rose-900/40 bg-rose-950/20 p-1.5"
                >
                  <input
                    data-testid="mission-block-reason"
                    value={blockReason}
                    onChange={(e) => setBlockReason(e.target.value)}
                    placeholder="Blocker reason…"
                    className="w-full rounded border border-shell-border bg-shell-bg px-1.5 py-1 text-[11px] text-shell-text outline-none focus:border-rose-500"
                  />
                  <button
                    type="button"
                    data-testid="mission-block-submit"
                    disabled={busy || !blockReason.trim()}
                    onClick={() => void onBlock()}
                    className="rounded bg-rose-800/80 px-2 py-0.5 text-[10px] text-white disabled:opacity-40"
                  >
                    Confirm blocked
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </section>
  )
}
