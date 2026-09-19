import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  getActiveSession,
  getSettings,
  setActiveProvider,
  type ActiveSessionView,
  type ProfileView,
} from '../../lib/api'

export type ModelPickerLocalValue = {
  profile?: string
  model?: string
  label?: string
}

type Props = {
  /** When busy streaming, disable switches so the active turn is not mid-flight. */
  disabled?: boolean
  /** Notifies parent of current label (profile · model). */
  onActiveChange?: (label: string, view: ActiveSessionView) => void
  /**
   * Local-only picker: does NOT call setActiveProvider.
   * Selection is reported via onLocalChange (for multi-slot wizard models).
   */
  localOnly?: boolean
  /** Controlled local value when localOnly. */
  value?: ModelPickerLocalValue | null
  /** Called when user picks profile+model in localOnly mode. */
  onLocalChange?: (next: ModelPickerLocalValue) => void
  /** Short prefix on the trigger button, e.g. "Teks" / "Vision". */
  slotLabel?: string
  /** data-testid suffix for multi-picker UIs. */
  testId?: string
}

/**
 * Composer model switcher: a compact select-style button that opens a modern
 * modal list (not a native <select> dropdown) for provider + model.
 * Mid-session switch via core.Service SetActiveProvider (VAL-PROV-005/011).
 * Placement: under chat input, next to Send (not the chat header).
 * With localOnly=true, selection stays local (no global session switch).
 */
export function ModelPicker({
  disabled,
  onActiveChange,
  localOnly = false,
  value = null,
  onLocalChange,
  slotLabel,
  testId,
}: Props) {
  const [active, setActive] = useState<ActiveSessionView | null>(null)
  const [profiles, setProfiles] = useState<ProfileView[]>([])
  const [models, setModels] = useState<{ id: string; owned_by?: string }[]>([])
  /**
   * Profile id the current `models` list was discovered for. Without it a
   * seed taken from `models[0]` can belong to a previously viewed provider.
   */
  const [modelsFor, setModelsFor] = useState('')
  const [modelsError, setModelsError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const [pendingProfile, setPendingProfile] = useState<string | null>(null)
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const titleId = useId()
  const seededLocal = useRef(false)
  /**
   * Monotonic id of the newest model-discovery request. A slower earlier
   * response must not overwrite the list the user is currently looking at.
   */
  const modelsReq = useRef(0)

  const applyView = useCallback(
    (v: ActiveSessionView, modelList?: { id: string; owned_by?: string }[], mErr?: string) => {
      setActive(v)
      if (v.profiles && v.profiles.length > 0) {
        setProfiles(v.profiles)
      }
      if (modelList) {
        setModels(modelList)
        setModelsFor(v.profile || '')
      } else if (v.models) {
        setModels(v.models)
        setModelsFor(v.profile || '')
      }
      setModelsError(mErr || v.models_error || null)
      const label = `${v.profile_name || v.profile}${v.model ? ` · ${v.model}` : ''}`
      if (!localOnly) {
        onActiveChange?.(label, v)
      } else if (!seededLocal.current && !value?.profile) {
        seededLocal.current = true
        onLocalChange?.({
          profile: v.profile,
          model: v.model,
          label,
        })
      }
    },
    [localOnly, onActiveChange, onLocalChange, value?.profile],
  )

  const reload = useCallback(async () => {
    const req = ++modelsReq.current
    try {
      const sess = await getActiveSession()
      const settings = await getSettings(sess.profile || '')
      if (req !== modelsReq.current) return
      applyView(
        {
          ...sess,
          profiles: sess.profiles?.length ? sess.profiles : settings.profiles,
        },
        settings.models || [],
        settings.models_error,
      )
    } catch (e) {
      if (req !== modelsReq.current) return
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [applyView])

  useEffect(() => {
    void reload()
  }, [reload])

  // Portaled dialogs keep focus inside the picker and restore the trigger.
  useEffect(() => {
    if (!open) return
    const opener = document.activeElement
    dialogRef.current?.querySelector<HTMLElement>('button:not(:disabled)')?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        setOpen(false)
      }
      if (e.key === 'Tab') {
        const controls = dialogRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), [tabindex="0"]')
        if (!controls?.length) return
        const first = controls[0]
        const last = controls[controls.length - 1]
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus()
    }
  }, [open])

  const openModal = useCallback(() => {
    if (disabled || busy) return
    const profileId = (localOnly ? value?.profile : active?.profile) || ''
    setPendingProfile(profileId || null)
    setOpen(true)
    // Refresh models for current profile when opening.
    void (async () => {
      const req = ++modelsReq.current
      try {
        const settings = await getSettings(profileId)
        if (req !== modelsReq.current) return
        setModels(settings.models || [])
        setModelsFor(profileId)
        setModelsError(settings.models_error || null)
        if (settings.profiles?.length) setProfiles(settings.profiles)
      } catch {
        /* keep cached */
      }
    })()
  }, [active?.profile, busy, disabled, localOnly, value?.profile])

  const selectProfile = useCallback(
    async (profileId: string) => {
      if (!profileId || busy || disabled) return
      setPendingProfile(profileId)
      // Drop the previous provider's list immediately so "Use provider" can
      // never commit a model id that belongs to the profile left behind.
      setModels([])
      setModelsFor('')
      setBusy(true)
      setError(null)
      const req = ++modelsReq.current
      try {
        const settings = await getSettings(profileId)
        if (req !== modelsReq.current) return
        setModels(settings.models || [])
        setModelsFor(profileId)
        setModelsError(settings.models_error || null)
        if (settings.profiles?.length) setProfiles(settings.profiles)
      } catch (e) {
        if (req !== modelsReq.current) return
        setError(e instanceof Error ? e.message : String(e))
      } finally {
        if (req === modelsReq.current) setBusy(false)
      }
    },
    [busy, disabled],
  )

  const commitProfileModel = useCallback(
    async (profileId: string, modelId: string) => {
      if (!profileId || busy || disabled) return
      setBusy(true)
      setError(null)
      try {
        if (localOnly) {
          const req = ++modelsReq.current
          const settings = await getSettings(profileId)
          if (req === modelsReq.current) {
            if (settings.profiles?.length) setProfiles(settings.profiles)
            setModels(settings.models || [])
            setModelsFor(profileId)
            setModelsError(settings.models_error || null)
          }
          const pname =
            settings.profiles?.find((p) => p.id === profileId)?.name ||
            profiles.find((p) => p.id === profileId)?.name ||
            profileId
          const label = `${pname}${modelId ? ` · ${modelId}` : ''}`
          onLocalChange?.({
            profile: profileId,
            model: modelId || undefined,
            label,
          })
          setOpen(false)
          return
        }
        const next = await setActiveProvider({
          profile: profileId,
          model: modelId || '',
        })
        const req = ++modelsReq.current
        const settings = await getSettings(profileId)
        if (req !== modelsReq.current) return
        applyView(next, settings.models || [], settings.models_error)
        setOpen(false)
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      } finally {
        setBusy(false)
      }
    },
    [applyView, busy, disabled, localOnly, onLocalChange, profiles],
  )

  const selectModel = useCallback(
    async (modelId: string) => {
      const profileId = pendingProfile || active?.profile
      if (!profileId || !modelId) return
      await commitProfileModel(profileId, modelId)
    },
    [active?.profile, commitProfileModel, pendingProfile],
  )

  /**
   * "Use provider" without an explicit model pick. Order matters: the profile's
   * own current/default model wins, then a discovered model that is known to
   * belong to THIS profile, then the active model only when the profile is
   * unchanged. Falling back to '' lets the backend apply the profile default
   * (SetActiveProvider: empty model → pc.DefaultModel()) instead of importing
   * the previous provider's model id.
   */
  const useProfileOnly = useCallback(async () => {
    const profileId = pendingProfile || active?.profile
    if (!profileId) return
    const meta = profiles.find((p) => p.id === profileId)
    const discovered = modelsFor === profileId ? models[0]?.id : undefined
    const seed =
      meta?.current_model ||
      meta?.default_model ||
      discovered ||
      (profileId === active?.profile ? active?.model : '') ||
      ''
    await commitProfileModel(profileId, seed)
  }, [
    active?.model,
    active?.profile,
    commitProfileModel,
    models,
    modelsFor,
    pendingProfile,
    profiles,
  ])

  const effectiveProfile = localOnly
    ? value?.profile || active?.profile || ''
    : active?.profile || ''
  // A local slot that has no model yet must not display the globally active
  // model of a different profile.
  const effectiveModel = localOnly
    ? value?.model || (!value?.profile || value.profile === active?.profile ? active?.model || '' : '')
    : active?.model || ''
  const profileName =
    profiles.find((p) => p.id === effectiveProfile)?.name ||
    (localOnly ? value?.label?.split(' · ')[0] : null) ||
    active?.profile_name ||
    active?.profile ||
    'Provider'
  const modelLabel = effectiveModel || 'auto'
  const triggerLabel = slotLabel
    ? `${slotLabel}: ${modelLabel}`
    : `${profileName}: ${modelLabel}`
  const viewingProfile = pendingProfile || effectiveProfile || ''
  const viewingMeta = profiles.find((p) => p.id === viewingProfile)
  const blocked =
    modelsError &&
    (modelsError.toLowerCase().includes('blocked') ||
      modelsError.toLowerCase().includes('key required') ||
      modelsError.toLowerCase().includes('missing api key'))

  return (
    <div
      data-testid={testId || 'model-picker-bar'}
      className="relative flex min-w-0 items-center gap-1.5"
    >
      <button
        type="button"
        id="chat-model-picker-trigger"
        data-testid={
          testId ? `${testId}-trigger` : 'chat-model-picker-trigger'
        }
        disabled={disabled || busy}
        onClick={openModal}
        className="inline-flex min-w-0 max-w-[min(16rem,100%)] items-center gap-1.5 rounded-md border border-shell-border bg-shell-bg px-2.5 py-1 text-left text-[11px] text-shell-text transition hover:border-shell-accent hover:bg-shell-panel disabled:opacity-50"
        title={
          localOnly
            ? value?.label || slotLabel || 'Select provider / model'
            : active?.host
              ? `host: ${active.host}`
              : 'Select provider / model'
        }
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        <span
          className="h-1.5 w-1.5 shrink-0 rounded-full bg-shell-accent"
          aria-hidden
        />
        <span className="min-w-0 truncate font-medium">{triggerLabel}</span>
        <svg
          className="h-3 w-3 shrink-0 text-shell-muted"
          viewBox="0 0 12 12"
          fill="none"
          aria-hidden
        >
          <path d="M3 4.5L6 7.5L9 4.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
        </svg>
      </button>

      {busy && (
        <span data-testid="model-picker-busy" className="text-[10px] text-shell-muted">
          switching…
        </span>
      )}
      {error && !open && (
        <span
          data-testid="model-picker-error"
          className="max-w-[8rem] truncate text-[10px] text-red-300"
          title={error}
        >
          {error}
        </span>
      )}

      {open && createPortal(
        <div
          data-testid="model-picker-modal-overlay"
          className="fixed inset-0 z-50 flex items-end justify-center bg-black/55 p-3 sm:items-center"
          onClick={() => setOpen(false)}
          role="presentation"
        >
          <div
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
            data-testid="model-picker-modal"
            className="flex max-h-[min(80vh,32rem)] w-full max-w-md flex-col overflow-hidden rounded-xl border border-shell-border bg-shell-panel shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-shell-border px-4 py-3">
              <div>
                <h3 id={titleId} className="text-sm font-semibold text-shell-text">
                  {slotLabel
                    ? `Model · ${slotLabel}`
                    : 'Provider & model'}
                </h3>
                <p className="mt-0.5 text-[11px] text-shell-muted">
                  {localOnly
                    ? 'Pilihan lokal untuk slot ini (tidak mengubah session global).'
                    : 'Switch mid-session without restart. BYOK keys stay local.'}
                </p>
              </div>
              <button
                type="button"
                data-testid="model-picker-close"
                onClick={() => setOpen(false)}
                className="rounded-md px-2 py-1 text-sm text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
                aria-label="Close"
              >
                ✕
              </button>
            </div>

            <div className="min-h-0 flex-1 overflow-auto px-2 py-2">
              <p className="px-2 pb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                Providers
              </p>
              <ul data-testid="model-picker-profile-list" className="space-y-0.5">
                {profiles.length === 0 && (
                  <li className="px-2 py-3 text-xs text-shell-muted">No profiles configured</li>
                )}
                {profiles.map((p) => {
                  const selected = p.id === viewingProfile
                  const isActive = p.id === active?.profile
                  return (
                    <li key={p.id}>
                      <button
                        type="button"
                        data-testid={`chat-profile-opt-${p.id}`}
                        onClick={() => void selectProfile(p.id)}
                        className={`flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left transition ${
                          selected
                            ? 'bg-shell-accent/15 ring-1 ring-shell-accent/40'
                            : 'hover:bg-shell-border/25'
                        }`}
                      >
                        <span
                          className={`mt-1 h-2 w-2 shrink-0 rounded-full ${
                            p.has_key ? 'bg-emerald-400' : 'bg-amber-400'
                          }`}
                          title={p.has_key ? 'Key configured' : 'Key required'}
                        />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-[12px] font-medium text-shell-text">
                            {p.name}
                            {p.is_gateway ? ' · gateway' : ''}
                            {isActive ? ' · active' : ''}
                          </span>
                          <span className="block truncate font-mono text-[10px] text-shell-muted">
                            {p.type}
                            {p.base_host ? ` · ${p.base_host}` : ''}
                            {!p.has_key ? ' · key required' : ''}
                          </span>
                        </span>
                      </button>
                    </li>
                  )
                })}
              </ul>

              <p className="mt-3 px-2 pb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                Models
                {viewingMeta ? ` · ${viewingMeta.name}` : ''}
              </p>
              {blocked && (
                <div
                  data-testid="model-picker-blocked"
                  className="mx-2 mb-2 rounded-lg border border-amber-700/50 bg-amber-950/40 px-2.5 py-2 text-[11px] text-amber-100"
                >
                  {modelsError}
                </div>
              )}
              {!blocked && modelsError && (
                <p className="mx-2 mb-2 text-[11px] text-red-300" title={modelsError}>
                  {modelsError}
                </p>
              )}
              <ul data-testid="model-picker-model-list" className="space-y-0.5">
                {models.length === 0 && !blocked && (
                  <li className="px-2 py-3 text-xs text-shell-muted">
                    {modelsError ? 'Models unavailable' : 'No models discovered — use profile default'}
                  </li>
                )}
                {models.map((m) => {
                  const selected =
                    m.id === effectiveModel && viewingProfile === effectiveProfile
                  return (
                    <li key={m.id}>
                      <button
                        type="button"
                        data-testid="chat-model-opt"
                        onClick={() => void selectModel(m.id)}
                        disabled={busy || disabled}
                        className={`flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left font-mono text-[11px] transition ${
                          selected
                            ? 'bg-shell-accent/20 text-shell-text ring-1 ring-shell-accent/40'
                            : 'text-shell-text hover:bg-shell-border/25'
                        } disabled:opacity-50`}
                      >
                        <span className="min-w-0 flex-1 truncate">{m.id}</span>
                        {selected && (
                          <span className="text-[10px] font-sans text-shell-accent">selected</span>
                        )}
                      </button>
                    </li>
                  )
                })}
              </ul>
            </div>

            <div className="flex items-center justify-between gap-2 border-t border-shell-border px-3 py-2.5">
              {error && (
                <span data-testid="model-picker-error" className="min-w-0 truncate text-[10px] text-red-300">
                  {error}
                </span>
              )}
              <div className="ml-auto flex gap-1.5">
                <button
                  type="button"
                  onClick={() => setOpen(false)}
                  className="rounded-lg border border-shell-border px-3 py-1.5 text-[11px] text-shell-muted hover:bg-shell-border/30"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  data-testid="model-picker-apply"
                  disabled={busy || disabled || !viewingProfile}
                  onClick={() => void useProfileOnly()}
                  className="shell-primary-button rounded-md px-3 py-1.5 text-[11px] font-medium disabled:opacity-40"
                >
                  Use provider
                </button>
              </div>
            </div>
          </div>
        </div>,
        document.body,
      )}

      {/* Hidden legacy test hooks so existing selectors still find labels */}
      <span id="chat-profile-picker" data-testid="chat-profile-picker" className="sr-only">
        {active?.profile || ''}
      </span>
      <span id="chat-model-picker" data-testid="chat-model-picker" className="sr-only">
        {active?.model || ''}
      </span>
    </div>
  )
}
