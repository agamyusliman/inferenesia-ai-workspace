import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { FileCode2, RefreshCw, Save } from 'lucide-react'
import {
  ensureGitWorkflow,
  getGitWorkflow,
  saveGitWorkflow,
  type GitWorkflow,
} from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import { SHELL } from '../shell/shellTokens'

type Props = {
  repoID: string
  onSaved?: (msg: string) => void
}

const empty: GitWorkflow = {
  version: 1,
  defaults: {},
  repos: {},
  exists: false,
}

export function GitWorkflowView({ repoID, onSaved }: Props) {
  const { t } = useLocale()
  const [wf, setWf] = useState<GitWorkflow>(empty)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [rules, setRules] = useState('')
  const [notes, setNotes] = useState('')
  const [defaultBranch, setDefaultBranch] = useState('main')
  const [commitStyle, setCommitStyle] = useState('conventional')
  const [deployEnabled, setDeployEnabled] = useState(false)
  const [deployWhen, setDeployWhen] = useState('')
  const [deployCmd, setDeployCmd] = useState('')
  const [deploySteps, setDeploySteps] = useState('')
  const [neverCommit, setNeverCommit] = useState('.env, .env.*, *.pem')
  const [scopeRepo, setScopeRepo] = useState(false)

  const applyForm = useCallback((w: GitWorkflow, forRepo: string, useRepo: boolean) => {
    const base = w.defaults || {}
    const over =
      useRepo && forRepo && w.repos && w.repos[forRepo] ? w.repos[forRepo] : null
    const r = over || base
    setRules(r.rules || base.rules || '')
    setNotes(r.agent_notes || base.agent_notes || '')
    setDefaultBranch(r.default_branch || base.default_branch || 'main')
    setCommitStyle(r.commit_style || base.commit_style || 'conventional')
    setDeployEnabled(!!r.deploy?.enabled)
    setDeployWhen(r.deploy?.when || '')
    setDeployCmd(r.deploy?.command || '')
    setDeploySteps((r.deploy?.steps || []).join('\n'))
    setNeverCommit((r.never_commit || base.never_commit || []).join(', '))
  }, [])

  const load = useCallback(async () => {
    setBusy(true)
    try {
      const w = await getGitWorkflow()
      setWf(w)
      applyForm(w, repoID, scopeRepo)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }, [applyForm, repoID, scopeRepo])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    applyForm(wf, repoID, scopeRepo)
  }, [repoID, scopeRepo, wf, applyForm])

  const onInit = async () => {
    setBusy(true)
    try {
      const w = await ensureGitWorkflow()
      setWf(w)
      applyForm(w, repoID, scopeRepo)
      onSaved?.(t('workflow.created'))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onSave = async () => {
    setBusy(true)
    setError(null)
    try {
      const next: GitWorkflow = {
        ...wf,
        version: wf.version || 1,
        defaults: { ...(wf.defaults || {}) },
        repos: { ...(wf.repos || {}) },
      }
      const patch = {
        default_branch: defaultBranch.trim() || 'main',
        commit_style: commitStyle.trim() || 'conventional',
        never_commit: neverCommit
          .split(/[,\n]/)
          .map((s) => s.trim())
          .filter(Boolean),
        rules: rules,
        agent_notes: notes,
        deploy: {
          enabled: deployEnabled,
          when: deployWhen.trim() || undefined,
          command: deployCmd.trim() || undefined,
          steps: deploySteps
            .split('\n')
            .map((s) => s.trim())
            .filter(Boolean),
        },
        push: {
          require_confirm: true,
          allow_force: false,
          remote: 'origin',
          protected_branches: ['main', 'master', 'release', 'production'],
        },
      }
      if (scopeRepo && repoID && repoID !== '.') {
        next.repos = next.repos || {}
        next.repos[repoID] = { ...(next.repos[repoID] || {}), ...patch }
      } else {
        next.defaults = { ...next.defaults, ...patch }
      }
      const saved = await saveGitWorkflow(next)
      setWf(saved)
      onSaved?.(t('workflow.saved'))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      data-testid="git-workflow-view"
      className="flex h-full min-h-0 flex-col overflow-hidden"
    >
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2">
        <FileCode2 size={SHELL.iconXs} className="text-shell-accent" />
        <span className="text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
          {t('workflow.title')}
        </span>
        <span className="truncate text-[10px] text-shell-muted">
          {wf.exists
            ? wf.source_path || '.inferenesia/git-workflow.yaml'
            : t('workflow.defaultsNotSaved')}
        </span>
        <button
          type="button"
          className="ml-auto rounded p-1 text-shell-muted hover:bg-shell-border/40"
          onClick={() => void load()}
          disabled={busy}
        >
          <RefreshCw size={SHELL.iconXs} className={busy ? 'animate-spin' : ''} />
        </button>
      </div>

      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-2 text-[12px]">
        <p className="text-[11px] text-shell-muted">
          {t('workflow.help', { path: '.inferenesia/git-workflow.yaml' })}
        </p>

        <label className="flex items-center gap-2 text-[11px] text-shell-text">
          <input
            type="checkbox"
            checked={scopeRepo}
            onChange={(e) => setScopeRepo(e.target.checked)}
            data-testid="git-workflow-scope-repo"
          />
          {t('workflow.editRepoOnly')}
          {repoID ? (
            <span className="font-mono text-shell-accent">({repoID})</span>
          ) : null}
        </label>

        <Field label={t('workflow.defaultBranch')}>
          <input
            data-testid="git-workflow-default-branch"
            value={defaultBranch}
            onChange={(e) => setDefaultBranch(e.target.value)}
            className={inputClass}
          />
        </Field>
        <Field label={t('workflow.commitStyle')}>
          <input
            data-testid="git-workflow-commit-style"
            value={commitStyle}
            onChange={(e) => setCommitStyle(e.target.value)}
            className={inputClass}
            placeholder={t('workflow.placeholderStyle')}
          />
        </Field>
        <Field label={t('workflow.neverCommit')}>
          <input
            data-testid="git-workflow-never-commit"
            value={neverCommit}
            onChange={(e) => setNeverCommit(e.target.value)}
            className={inputClass}
          />
        </Field>
        <Field label={t('workflow.rules')}>
          <textarea
            data-testid="git-workflow-rules"
            value={rules}
            onChange={(e) => setRules(e.target.value)}
            rows={6}
            className={textareaClass}
          />
        </Field>
        <Field label={t('workflow.agentNotes')}>
          <textarea
            data-testid="git-workflow-notes"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            rows={3}
            className={textareaClass}
          />
        </Field>

        <label className="flex items-center gap-2 text-[11px] text-shell-text">
          <input
            type="checkbox"
            checked={deployEnabled}
            onChange={(e) => setDeployEnabled(e.target.checked)}
            data-testid="git-workflow-deploy-enabled"
          />
          {t('workflow.deployEnabled')}
        </label>
        {deployEnabled && (
          <>
            <Field label={t('workflow.deployWhen')}>
              <input
                value={deployWhen}
                onChange={(e) => setDeployWhen(e.target.value)}
                className={inputClass}
                placeholder={t('workflow.placeholderWhen')}
              />
            </Field>
            <Field label={t('workflow.deployCommand')}>
              <input
                value={deployCmd}
                onChange={(e) => setDeployCmd(e.target.value)}
                className={inputClass}
                placeholder={t('workflow.placeholderCmd')}
              />
            </Field>
            <Field label={t('workflow.deploySteps')}>
              <textarea
                value={deploySteps}
                onChange={(e) => setDeploySteps(e.target.value)}
                rows={3}
                className={textareaClass}
              />
            </Field>
          </>
        )}

        {error && (
          <div className="rounded border border-rose-900/50 bg-rose-950/30 px-2 py-1 text-[11px] text-rose-200">
            {error}
          </div>
        )}
      </div>

      <div className="flex shrink-0 gap-2 border-t border-shell-border p-2">
        {!wf.exists && (
          <button
            type="button"
            data-testid="git-workflow-init"
            disabled={busy}
            onClick={() => void onInit()}
            className="rounded border border-shell-border px-2 py-1.5 text-[11px] text-shell-text hover:bg-shell-border/30 disabled:opacity-40"
          >
            {t('workflow.createFile')}
          </button>
        )}
        <button
          type="button"
          data-testid="git-workflow-save"
          disabled={busy}
          onClick={() => void onSave()}
          className="inline-flex flex-1 items-center justify-center gap-1 rounded bg-shell-accent px-2 py-1.5 text-[12px] font-medium text-white hover:opacity-90 disabled:opacity-40"
        >
          <Save size={SHELL.iconSm} />
          {t('workflow.save')}
        </button>
      </div>
    </div>
  )
}

const inputClass =
  'w-full rounded border border-shell-border bg-shell-bg px-2 py-1 font-mono text-[11px] text-shell-text focus:border-shell-accent focus:outline-none'
const textareaClass =
  'w-full resize-y rounded border border-shell-border bg-shell-bg px-2 py-1 font-mono text-[11px] text-shell-text focus:border-shell-accent focus:outline-none'

function Field({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <label className="block space-y-0.5">
      <span className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
        {label}
      </span>
      {children}
    </label>
  )
}
