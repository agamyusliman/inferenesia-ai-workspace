import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  addProvider,
  completeOnboarding,
  getSettings,
  getTokenSavers,
  getLiveBlocks,
  getMCPStatus,
  mcpServerAction,
  getAutonomy,
  setAutonomy,
  setActiveProvider,
  setTokenSaver,
  setLiveBlocks,
  testProvider,
  type AddProviderRequest,
  type AutonomyView,
  type LiveBlocksView,
  type MCPStatusView,
  type MCPServerView,
  type OnboardingState,
  type ProfileView,
  type SettingsView,
  type TestProviderResult,
  type TokenSaverStatus,
  type TokenSaversView,
} from '../../lib/api'
import { useLocale, type LocaleContextValue } from '../i18n/LocaleProvider'
import {
  LOCALE_IDS,
  localeLabel,
  type LocaleId,
} from '../i18n/locale'
import { useTheme } from '../theme/ThemeProvider'
import {
  THEME_IDS,
  themeLabel,
  type ThemeId,
} from '../theme/theme'
import { Plus, RefreshCw } from 'lucide-react'
import { PanelHeader } from '../shell/PanelHeader'
import { notifyAutonomyChanged } from '../chat/AutonomyPicker'
import { displayBaseURL } from './maskBaseUrl'
import { isNotInstalled, saverStatusLabel } from './tokenSavers'

type Props = {
  onClose?: () => void
  onOpenTerminalWithCommand?: (command: string, title: string) => void
}

/** Gateway profile id as registered by the backend router (legacy alias: tempai). */
const GATEWAY_PROFILE = 'inferenesia'

/** Onboarding path ids returned by the backend onboarding state. */
type PathTab = typeof GATEWAY_PROFILE | 'byok' | null

/** Stable settings category ids (local UI state only — never hits the API). */
type SettingsCategory = 'appearance' | 'providers' | 'agent' | 'workspace'

const SECTION_TITLE = 'text-[13px] font-semibold text-shell-text'
const SECTION_DESC = 'text-[11px] leading-relaxed text-shell-muted'
const FIELD_LABEL = 'block text-[11px] text-shell-muted'
const FIELD_INPUT =
  'mt-1 w-full rounded border border-shell-border bg-shell-panel px-2 py-1.5 text-[12px] text-shell-text'
const ROW = 'rounded-md border border-shell-border bg-shell-bg px-2.5 py-2'
const BTN_SECONDARY =
  'shell-transition rounded border border-shell-border px-2.5 py-1 text-[11px] text-shell-text hover:bg-shell-hover disabled:opacity-40'
const BTN_PRIMARY =
  'shell-primary-button rounded px-2.5 py-1 text-[11px] font-medium disabled:opacity-40'

/** RTK / Headroom accept an optional setup value (command / compress base URL). */
function saverSetup(
  id: string,
): { field: 'command' | 'base_url'; placeholder: string } | null {
  if (id === 'rtk') return { field: 'command', placeholder: 'rtk (default)' }
  if (id === 'headroom') {
    return { field: 'base_url', placeholder: 'https://headroom.local/v1' }
  }
  return null
}

/** Autonomy levels in ascending capability order (matches the Go tool gate). */
const AUTONOMY_LEVELS = ['off', 'low', 'medium', 'high'] as const

/**
 * Active-level styling. Alpha tints (not fixed 950/200 pairs) so the same class
 * stays legible on dark, warm and light surfaces; the level stays readable
 * because the text keeps the theme's own foreground colour.
 */
const AUTONOMY_ACTIVE_CLASS: Record<(typeof AUTONOMY_LEVELS)[number], string> = {
  off: 'border-red-500/60 bg-red-500/10 text-shell-text',
  low: 'border-amber-500/60 bg-amber-500/10 text-shell-text',
  medium: 'border-blue-500/60 bg-blue-500/10 text-shell-text',
  high: 'border-emerald-500/60 bg-emerald-500/10 text-shell-text',
}

/** Inline feedback for one settings action. `kind` drives styling and a11y role. */
type Notice = { kind: 'success' | 'error'; text: string } | null

/** Normalize a thrown value into a displayable message. */
function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/**
 * Tell the rest of the shell that MCP state changed, so the chat footer badge
 * refreshes without waiting for a window focus event.
 */
function notifyMCPChanged() {
  window.dispatchEvent(new CustomEvent('inferenesia-mcp-changed'))
}

export function SettingsPanel({ onOpenTerminalWithCommand }: Props) {
  const { theme, setTheme } = useTheme()
  const { locale, setLocale, t } = useLocale()
  const [view, setView] = useState<SettingsView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [selectedProfile, setSelectedProfile] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  /**
   * Profile the model field was last seeded for. Lets a reload distinguish
   * "same profile, user typed a model" (keep it) from "profile changed"
   * (re-seed), without adding the model to the reload dependency list.
   */
  const selectedProfileRef = useRef('')
  /**
   * Monotonic id of the newest settings load. Guards every state write so a
   * slow early response cannot overwrite a newer one (rapid profile clicks).
   */
  const reloadReq = useRef(0)
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null)
  const [testing, setTesting] = useState(false)
  const [showAdd, setShowAdd] = useState(false)
  const [addForm, setAddForm] = useState({
    name: '',
    base_url: '',
    api_key: '',
    default_model: '',
  })
  // Wire format preset for the add-provider form (P9-prov):
  //   openai   → type openai_compatible + api_format openai (base_url required)
  //   anthropic→ type anthropic + api_format anthropic (base_url optional)
  //   litellm  → type openai_compatible + api_format anthropic (Claude-compatible host)
  type AddWirePreset = 'openai' | 'anthropic' | 'litellm'
  const [addWire, setAddWire] = useState<AddWirePreset>('openai')
  const [addBusy, setAddBusy] = useState(false)
  const [addError, setAddError] = useState<string | null>(null)
  const [pathTab, setPathTab] = useState<PathTab>(null)
  /** Inline result of the last action. Failures must never render as success. */
  const [notice, setNotice] = useState<Notice>(null)
  const [applyBusy, setApplyBusy] = useState(false)
  const [tokenSavers, setTokenSaversState] = useState<TokenSaversView | null>(null)
  const [saverBusy, setSaverBusy] = useState<string | null>(null)
  const [liveBlocks, setLiveBlocksState] = useState<LiveBlocksView | null>(null)
  const [liveBlocksBusy, setLiveBlocksBusy] = useState(false)
  const [mcpStatus, setMcpStatus] = useState<MCPStatusView | null>(null)
  const [mcpBusy, setMcpBusy] = useState<string | null>(null)
  const [autonomy, setAutonomyState] = useState<AutonomyView | null>(null)
  const [autonomyBusy, setAutonomyBusy] = useState(false)
  const [category, setCategory] = useState<SettingsCategory>('providers')
  /** Unsaved setup values per saver id; falls back to the persisted value. */
  const [saverDrafts, setSaverDrafts] = useState<Record<string, string>>({})

  const reload = useCallback(async (profile = '') => {
    const req = ++reloadReq.current
    setLoading(true)
    setError(null)
    try {
      const v = await getSettings(profile)
      // A newer reload started while this one was in flight: its result is the
      // truth, so this stale response must not write any state.
      if (req !== reloadReq.current) return null
      setView(v)
      if (v.token_savers) {
        setTokenSaversState(v.token_savers)
      } else {
        try {
          const ts = await getTokenSavers()
          if (req === reloadReq.current) setTokenSaversState(ts)
        } catch {
          // Token savers optional — never block settings open.
        }
      }
      if (v.live_blocks) {
        setLiveBlocksState(v.live_blocks)
      } else {
        try {
          const lb = await getLiveBlocks()
          if (req === reloadReq.current) setLiveBlocksState(lb)
        } catch {
          // Live Blocks optional — never block settings open.
        }
      }
      if (v.mcp) {
        setMcpStatus(v.mcp)
      } else {
        try {
          const ms = await getMCPStatus()
          if (req === reloadReq.current) setMcpStatus(ms)
        } catch {
          // MCP section optional — never block settings open.
        }
      }
      if (v.autonomy) {
        setAutonomyState(v.autonomy)
      } else {
        try {
          const av = await getAutonomy()
          if (req === reloadReq.current) setAutonomyState(av)
        } catch {
          // Autonomy section optional — never block settings open.
        }
      }
      // Prefer the id the backend resolved: the requested one may be a legacy
      // alias that matches no profile row.
      const sel =
        v.selected_profile ||
        profile ||
        v.default_profile ||
        v.profiles[0]?.id ||
        ''
      setSelectedProfile(sel)
      // Seed the model picker only when the user has not typed one for this
      // profile — a background refresh must not discard an in-progress entry.
      const prof = v.profiles.find((p) => p.id === sel)
      const seed = prof?.current_model || prof?.default_model || v.models?.[0]?.id || ''
      // Capture the comparison NOW. React may run the updater below after this
      // function returns, by which time the ref would already equal `sel` and a
      // real profile switch would look like "same profile" — keeping the model
      // of the profile the user just left.
      const profileChanged = sel !== selectedProfileRef.current
      selectedProfileRef.current = sel
      setSelectedModel((current) => {
        if (!current) return seed
        if (profileChanged) return seed
        return current
      })
      return v
    } catch (e) {
      if (req !== reloadReq.current) return null
      setError(errorText(e))
      return null
    } finally {
      if (req === reloadReq.current) setLoading(false)
    }
  }, [])

  /** Toggle one Token Saver independently (VAL-SAVER-001/002/007). Missing install is OK. */
  const onToggleSaver = useCallback(
    async (id: string, enabled: boolean) => {
      // Saver writes are serialized: setTokenSaver returns the FULL saver list,
      // so two in-flight writes would race and the slower response would
      // resurrect the other row's pre-change state.
      if (saverBusy !== null) return
      setSaverBusy(id)
      setNotice(null)
      try {
        const next = await setTokenSaver({ id, enabled })
        setTokenSaversState(next)
        const row = next.savers.find((s) => s.id === id)
        // Report the state the backend persisted, not the state requested.
        if (row && row.enabled && isNotInstalled(row)) {
          setNotice({
            kind: 'success',
            text: t('settings.saverEnabledMissing', { name: row.name }),
          })
        } else if (row) {
          setNotice({
            kind: 'success',
            text: t('settings.saverSaved', {
              name: row.name,
              state: row.enabled ? t('settings.on') : t('settings.off'),
              home: next.config_home || '~/.inferenesia',
            }),
          })
        }
      } catch (e) {
        // Never crash settings UI when a toggle fails, and never imply success:
        // the row keeps showing the last known backend truth.
        setNotice({
          kind: 'error',
          text: `${t('settings.saverToggleFailed')}: ${errorText(e)}`,
        })
      } finally {
        setSaverBusy(null)
      }
    },
    [t],
  )

  /**
   * Save the optional setup value for one saver (RTK command / Headroom base
   * URL) without flipping its enabled state. Blank clears back to the default.
   */
  const onSaveSaverSetup = useCallback(
    async (row: TokenSaverStatus, field: 'command' | 'base_url', value: string) => {
      if (saverBusy !== null) return
      setSaverBusy(row.id)
      setNotice(null)
      try {
        const trimmed = value.trim()
        // `enabled` is deliberately omitted: this action edits setup only, and
        // sending a stale toggle value could disable the saver as a side effect.
        const next = await setTokenSaver(
          field === 'command'
            ? { id: row.id, command: trimmed }
            : { id: row.id, base_url: trimmed },
        )
        setTokenSaversState(next)
        // Only drop the draft once the value is known to be persisted, so a
        // failed save keeps the user's text in the field.
        setSaverDrafts((d) => {
          if (!(row.id in d)) return d
          const rest = { ...d }
          delete rest[row.id]
          return rest
        })
        const saved = next.savers.find((s) => s.id === row.id)
        setNotice({
          kind: 'success',
          text: t('settings.saverSetupSaved', {
            name: saved?.name || row.name,
            home: next.config_home || '~/.inferenesia',
          }),
        })
      } catch (e) {
        setNotice({
          kind: 'error',
          text: `${t('settings.saverSetupFailed')}: ${errorText(e)}`,
        })
      } finally {
        setSaverBusy(null)
      }
    },
    [t],
  )

  /** Toggle Live Blocks opt-in (P9-live). Default off; sandboxed iframe previews. */
  const onToggleLiveBlocks = useCallback(
    async (enabled: boolean) => {
      setLiveBlocksBusy(true)
      setNotice(null)
      try {
        const next = await setLiveBlocks({ enabled })
        setLiveBlocksState(next)
        setNotice({
          kind: 'success',
          text: t('settings.liveBlocksSaved', {
            state: next.enabled ? t('settings.on') : t('settings.off'),
          }),
        })
      } catch (e) {
        setNotice({
          kind: 'error',
          text: `${t('settings.liveBlocksFailed')}: ${errorText(e)}`,
        })
      } finally {
        setLiveBlocksBusy(false)
      }
    },
    [t],
  )

  const onToggleMCP = useCallback(
    async (name: string, enabled: boolean) => {
      setMcpBusy(name)
      setNotice(null)
      try {
        const next = await mcpServerAction({ action: 'toggle', name, enabled })
        setMcpStatus(next)
        notifyMCPChanged()
        // Read the persisted row back: a server owned by an env override or
        // project config can refuse the change without failing the request.
        const row = next.servers.find((srv) => srv.name === name)
        setNotice({
          kind: 'success',
          text: t('settings.mcpToggled', {
            name,
            state: (row ? row.enabled : enabled)
              ? t('settings.enabled')
              : t('settings.disabled'),
          }),
        })
      } catch (e) {
        setNotice({
          kind: 'error',
          text: `${t('settings.mcpToggleFailed')}: ${errorText(e)}`,
        })
      } finally {
        setMcpBusy(null)
      }
    },
    [t],
  )

  const onReloadMCP = useCallback(async () => {
    setMcpBusy('__reload__')
    setNotice(null)
    try {
      const next = await mcpServerAction({ action: 'reload' })
      setMcpStatus(next)
      notifyMCPChanged()
      setNotice({
        kind: 'success',
        // Health is only meaningful against enabled servers; disabled ones are
        // intentionally not running.
        text: t('settings.mcpReloaded', {
          healthy: next.healthy_count,
          enabled: next.enabled_count,
          tools: next.total_tools,
        }),
      })
    } catch (e) {
      setNotice({
        kind: 'error',
        text: `${t('settings.mcpReloadFailed')}: ${errorText(e)}`,
      })
    } finally {
      setMcpBusy(null)
    }
  }, [t])

  const onSetAutonomy = useCallback(
    async (level: string) => {
      setAutonomyBusy(true)
      setNotice(null)
      try {
        const next = await setAutonomy({ level })
        setAutonomyState(next)
        notifyAutonomyChanged(next)
        // next.level is what the backend persisted; a rejected level throws.
        setNotice({
          kind: 'success',
          text: t('settings.autonomySaved', { level: next.level }),
        })
      } catch (e) {
        setNotice({
          kind: 'error',
          text: `${t('settings.autonomyFailed')}: ${errorText(e)}`,
        })
      } finally {
        setAutonomyBusy(false)
      }
    },
    [t],
  )

  useEffect(() => {
    void reload('')
  }, [reload])

  const onSelectProfile = useCallback(
    async (id: string) => {
      setSelectedProfile(id)
      setTestResult(null)
      setNotice(null)
      await reload(id)
    },
    [reload],
  )

  const onTest = useCallback(async (id: string) => {
    setTesting(true)
    setTestResult(null)
    setNotice(null)
    try {
      const res = await testProvider(id)
      setTestResult(res)
    } catch (e) {
      setTestResult({
        ok: false,
        profile: id,
        error: errorText(e),
      })
    } finally {
      setTesting(false)
    }
  }, [])

  const onAdd = useCallback(async () => {
    setAddBusy(true)
    setAddError(null)
    setNotice(null)
    try {
      // Resolve type + api_format from the wire-format preset (P9-prov).
      let type: 'openai_compatible' | 'anthropic'
      let apiFormat: 'openai' | 'anthropic'
      switch (addWire) {
        case 'anthropic':
          type = 'anthropic'
          apiFormat = 'anthropic'
          break
        case 'litellm':
          // Claude-compatible host that speaks the Anthropic Messages wire
          // format but is configured as an openai_compatible profile.
          type = 'openai_compatible'
          apiFormat = 'anthropic'
          break
        default:
          type = 'openai_compatible'
          apiFormat = 'openai'
          break
      }
      const baseTrim = addForm.base_url.trim()
      // Only the pure Anthropic preset may omit base URL (defaults to
      // api.anthropic.com). LiteLLM/Claude-compatible hosts still need one.
      if (addWire !== 'anthropic' && baseTrim === '') {
        setAddError(t('settings.baseUrlRequired'))
        return
      }
      const req: AddProviderRequest = {
        name: addForm.name.trim() || (type === 'anthropic' ? 'Anthropic' : 'BYOK'),
        type,
        api_format: apiFormat,
        base_url: baseTrim,
        api_key: addForm.api_key.trim(),
        default_model: addForm.default_model.trim() || undefined,
        onboarding_path: 'byok',
      }
      const v = await addProvider(req)
      setView(v)
      setShowAdd(false)
      setAddForm({ name: '', base_url: '', api_key: '', default_model: '' })
      setAddWire('openai')
      // The backend returns the freshly added profile as selected_profile.
      const newId = v.selected_profile || v.default_profile
      if (newId) {
        setSelectedProfile(newId)
        await reload(newId)
      }
      setNotice({ kind: 'success', text: t('settings.byokSaved') })
      setPathTab(null)
    } catch (e) {
      setAddError(errorText(e))
    } finally {
      setAddBusy(false)
    }
  }, [addForm, addWire, reload, t])

  const markGatewayPath = useCallback(async () => {
    setNotice(null)
    setApplyBusy(true)
    try {
      await completeOnboarding(GATEWAY_PROFILE)
      // The backend canonicalizes legacy aliases, so trust the id it reports
      // back rather than assuming the one that was requested.
      const session = await setActiveProvider({ profile: GATEWAY_PROFILE })
      const active = session.profile || GATEWAY_PROFILE
      setSelectedProfile(active)
      await reload(active)
      setNotice({ kind: 'success', text: t('settings.gatewaySelected') })
      setPathTab(null)
    } catch (e) {
      // Selecting the gateway failed: leave the previous selection visible and
      // say so instead of switching the UI as if it had worked.
      setNotice({
        kind: 'error',
        text: `${t('settings.gatewaySelectFailed')}: ${errorText(e)}`,
      })
    } finally {
      setApplyBusy(false)
    }
  }, [reload, t])

  /** Apply selected profile/model to the live chat session (VAL-PROV-005/011). */
  const onApplySession = useCallback(
    async (asDefault: boolean) => {
      if (!selectedProfile) return
      const applied = selectedProfile
      setApplyBusy(true)
      setNotice(null)
      try {
        const session = await setActiveProvider({
          profile: selectedProfile,
          model: selectedModel || undefined,
          set_default: asDefault,
        })
        // The user may have selected another profile while this was in flight;
        // re-syncing then would silently move their selection back.
        if (selectedProfileRef.current !== applied) return
        // Re-sync to the ids the backend actually persisted.
        if (session.profile) {
          setSelectedProfile(session.profile)
          selectedProfileRef.current = session.profile
        }
        if (session.model) setSelectedModel(session.model)
        const route = `${session.profile_name || session.profile}${session.model ? ` / ${session.model}` : ''}`
        setNotice({
          kind: 'success',
          text: asDefault
            ? t('settings.defaultProviderSet', { route })
            : t('settings.sessionProviderSet', { route }),
        })
      } catch (e) {
        setNotice({
          kind: 'error',
          text: `${t('settings.applyProviderFailed')}: ${errorText(e)}`,
        })
      } finally {
        setApplyBusy(false)
      }
    },
    [selectedModel, selectedProfile, t],
  )

  const profiles: ProfileView[] = view?.profiles || []
  const models = view?.models || []
  const onboarding: OnboardingState | undefined = view?.onboarding
  const showOnboardingBanner = onboarding && !onboarding.done

  const selectedMeta = useMemo(
    () => profiles.find((p) => p.id === selectedProfile) || null,
    [profiles, selectedProfile],
  )

  const navItems: { id: SettingsCategory; label: string }[] = [
    { id: 'appearance', label: t('settings.appearance') },
    { id: 'providers', label: t('settings.providers') },
    { id: 'agent', label: t('settings.agent') },
    { id: 'workspace', label: t('settings.workspacePrefs') },
  ]

  // Sections rendered as real controls above; only unknown ids stay as info rows.
  const extraSections = (view?.sections || []).filter(
    (s) =>
      ![
        'onboarding',
        'profiles',
        'models',
        'token-savers',
        'live-blocks',
        'mcp',
        'autonomy',
        'workspace',
      ].includes(s.id),
  )

  return (
    <div
      data-testid="settings-panel"
      className="flex h-full min-h-0 w-full min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader
        testId="settings-panel-header"
        title={t('settings.title')}
        subtitle={view?.config_home || undefined}
      />

      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden lg:flex-row">
        <nav
          data-testid="settings-nav"
          aria-label={t('settings.title')}
          className="flex shrink-0 flex-wrap gap-1 border-b border-shell-border bg-shell-panel p-2 lg:w-[180px] lg:flex-col lg:flex-nowrap lg:overflow-y-auto lg:border-b-0 lg:border-r"
        >
          {navItems.map((item) => {
            const active = category === item.id
            return (
              <button
                key={item.id}
                type="button"
                data-testid={`settings-nav-${item.id}`}
                data-active={active ? 'true' : 'false'}
                aria-pressed={active}
                aria-controls={`settings-category-${item.id}`}
                onClick={() => setCategory(item.id)}
                className="shell-list-item rounded-md px-2.5 py-1.5 text-left text-[12px] font-medium text-shell-text lg:w-full"
              >
                {item.label}
              </button>
            )
          })}
        </nav>

        <div className="min-h-0 min-w-0 flex-1 overflow-auto">
          <div className="w-full max-w-[920px] space-y-3 p-3 md:p-4">
            {loading && !view && (
              <p className="text-xs text-shell-muted">{t('settings.loading')}</p>
            )}
            {error && (
              <p
                data-testid="settings-error"
                role="alert"
                className="rounded border border-red-500/60 bg-red-500/10 px-2.5 py-1.5 text-[11px] text-shell-text"
              >
                {error}
              </p>
            )}
            {notice && (
              <p
                data-testid="settings-banner"
                data-kind={notice.kind}
                role={notice.kind === 'error' ? 'alert' : 'status'}
                aria-live={notice.kind === 'error' ? 'assertive' : 'polite'}
                className={`rounded border px-2.5 py-1.5 text-[11px] text-shell-text ${
                  notice.kind === 'error'
                    ? 'border-red-500/60 bg-red-500/10'
                    : 'border-emerald-500/60 bg-emerald-500/10'
                }`}
              >
                {notice.text}
              </p>
            )}

            {/* ---------------------------------------------------- appearance */}
            <div
              id="settings-category-appearance"
              hidden={category !== 'appearance'}
              className="space-y-3"
            >
              <section
                data-testid="settings-appearance"
                className="rounded-lg border border-shell-border bg-shell-panel p-3"
              >
                <h3 className={SECTION_TITLE}>{t('settings.appearance')}</h3>
                <p className={`mt-0.5 ${SECTION_DESC}`}>
                  {t('settings.appearanceHelp')}{' '}
                  <span className="font-mono">inferenesia-theme</span>
                  {' · '}
                  <span className="font-mono">inferenesia-locale</span>
                </p>
                <div className="mt-2.5 flex flex-wrap items-start gap-4">
                  <div>
                    <div className="mb-1 text-[11px] text-shell-muted">
                      {t('settings.theme')}
                    </div>
                    <div
                      role="group"
                      aria-label={t('settings.theme')}
                      className="inline-flex rounded-md border border-shell-border p-0.5"
                      data-testid="theme-select"
                    >
                      {THEME_IDS.map((id: ThemeId) => {
                        const active = theme === id
                        return (
                          <button
                            key={id}
                            type="button"
                            data-testid={`theme-select-${id}`}
                            data-active={active ? 'true' : 'false'}
                            aria-pressed={active}
                            onClick={() => setTheme(id)}
                            className={`rounded px-3 py-1.5 text-[11px] font-medium ${
                              active
                                ? 'shell-primary-button'
                                : 'shell-transition text-shell-muted hover:bg-shell-hover hover:text-shell-text'
                            }`}
                          >
                            {themeLabel(id)}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                  <div>
                    <div className="mb-1 text-[11px] text-shell-muted">
                      {t('settings.language')}
                    </div>
                    <div
                      role="group"
                      aria-label={t('settings.language')}
                      className="inline-flex rounded-md border border-shell-border p-0.5"
                      data-testid="locale-select"
                    >
                      {LOCALE_IDS.map((id: LocaleId) => {
                        const active = locale === id
                        return (
                          <button
                            key={id}
                            type="button"
                            data-testid={`locale-select-${id}`}
                            data-active={active ? 'true' : 'false'}
                            aria-pressed={active}
                            onClick={() => setLocale(id)}
                            className={`rounded px-3 py-1.5 text-[11px] font-medium ${
                              active
                                ? 'shell-primary-button'
                                : 'shell-transition text-shell-muted hover:bg-shell-hover hover:text-shell-text'
                            }`}
                          >
                            {localeLabel(id)}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                </div>
              </section>
            </div>

            {/* ----------------------------------------------------- providers */}
            <div
              id="settings-category-providers"
              hidden={category !== 'providers'}
              className="space-y-3"
            >
              {view && (
                <>
                  <section
                    data-testid="settings-onboarding"
                    className="rounded-lg border border-shell-border bg-shell-panel p-3"
                  >
                    <h3 className={SECTION_TITLE}>{t('settings.connectModel')}</h3>
                    <p className={`mt-0.5 ${SECTION_DESC}`}>
                      {t('settings.providersHelp')}
                    </p>
                    {showOnboardingBanner && (
                      <p
                        data-testid="onboarding-needed"
                        className="mt-2 rounded border border-amber-500/50 bg-amber-500/10 px-2 py-1 text-[11px] text-shell-text"
                      >
                        {t('settings.firstRun')}
                      </p>
                    )}
                    <div className="mt-2 grid gap-2 sm:grid-cols-2">
                      {(onboarding?.paths || []).map((p) => (
                        <button
                          key={p.id}
                          type="button"
                          data-testid={`onboarding-path-${p.id}`}
                          data-active={pathTab === p.id ? 'true' : 'false'}
                          aria-pressed={pathTab === p.id}
                          onClick={() => setPathTab(p.id as PathTab)}
                          className="shell-list-item rounded-md border-shell-border px-2.5 py-2 text-left text-[11px] text-shell-text"
                        >
                          <div className="text-[12px] font-semibold">{p.title}</div>
                          <div className="mt-0.5 text-shell-muted">{p.description}</div>
                          {!p.requires_other && (
                            <div className="mt-1 text-[10px] text-shell-muted">
                              {t('settings.noOtherPath')}
                            </div>
                          )}
                        </button>
                      ))}
                    </div>

                    {pathTab === GATEWAY_PROFILE && (
                      <div
                        data-testid="onboarding-tempai-panel"
                        className={`mt-2.5 space-y-2 ${ROW}`}
                      >
                        <p className="text-[11px] text-shell-text">
                          Inferenesia API gateway uses{' '}
                          <span className="font-mono">INFERENESIA_BASE_URL</span> +{' '}
                          <span className="font-mono">INFERENESIA_API_KEY</span> (or legacy{' '}
                          <span className="font-mono">TEMP_AI_*</span>) from your environment or
                          local config. Default base:{' '}
                          <span className="font-mono">https://inferenesia.cloud/v1</span>
                        </p>
                        <p className="text-[11px] text-shell-muted">
                          Status:{' '}
                          {onboarding?.has_tempai_key ? (
                            <span className="text-emerald-300">gateway key detected</span>
                          ) : (
                            <span className="text-amber-300">
                              no key yet — set INFERENESIA_API_KEY (or TEMP_AI_API_KEY) in .env,
                              then Test the gateway profile
                            </span>
                          )}
                        </p>
                        <div className="flex flex-wrap gap-2">
                          <button
                            type="button"
                            data-testid="onboarding-use-tempai"
                            onClick={() => void markGatewayPath()}
                            className={BTN_PRIMARY}
                          >
                            {t('settings.useGateway')}
                          </button>
                          <button
                            type="button"
                            data-testid="onboarding-test-tempai"
                            disabled={testing}
                            onClick={() => {
                              setSelectedProfile(GATEWAY_PROFILE)
                              void onTest(GATEWAY_PROFILE)
                            }}
                            className={BTN_SECONDARY}
                          >
                            {testing ? t('settings.testing') : t('settings.testTempai')}
                          </button>
                        </div>
                      </div>
                    )}

                    {pathTab === 'byok' && (
                      <div
                        data-testid="onboarding-byok-panel"
                        className={`mt-2.5 space-y-2 ${ROW}`}
                      >
                        <p className="text-[11px] text-shell-text">
                          Bring your own OpenAI-compatible base URL + key. Keys persist only under{' '}
                          <span className="font-mono">~/.inferenesia/config.yaml</span> (mode 0600)
                          and are never POSTed to the Inferenesia API gateway host.
                        </p>
                        <button
                          type="button"
                          data-testid="onboarding-open-byok-form"
                          onClick={() => {
                            setShowAdd(true)
                            setPathTab('byok')
                          }}
                          className={BTN_PRIMARY}
                        >
                          {t('settings.addByokProfile')}
                        </button>
                      </div>
                    )}
                  </section>

                  <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(320px,1fr))]">
                    <section
                      data-testid="settings-profiles"
                      className="min-w-0 rounded-lg border border-shell-border bg-shell-panel p-3"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <h3 className={SECTION_TITLE}>{t('settings.providers')}</h3>
                        <button
                          type="button"
                          data-testid="add-provider-btn"
                          aria-expanded={showAdd}
                          onClick={() => setShowAdd((v) => !v)}
                          className={`${BTN_SECONDARY} inline-flex items-center gap-1`}
                        >
                          {!showAdd && <Plus size={12} aria-hidden />}
                          {showAdd ? t('action.cancel') : t('settings.addByok')}
                        </button>
                      </div>

                      {showAdd && (
                        <form
                          data-testid="add-provider-form"
                          className={`mt-2 space-y-2 ${ROW}`}
                          onSubmit={(e) => {
                            e.preventDefault()
                            void onAdd()
                          }}
                        >
                          <p className="text-[12px] font-semibold text-shell-text">
                            {t('settings.newByokProfile')}
                          </p>
                          <label className={FIELD_LABEL}>
                            {t('settings.wireFormat')}
                            <select
                              data-testid="add-provider-wire-format"
                              className={FIELD_INPUT}
                              value={addWire}
                              onChange={(e) => setAddWire(e.target.value as AddWirePreset)}
                            >
                              <option value="openai">
                                OpenAI-compatible (/v1/chat/completions)
                              </option>
                              <option value="anthropic">Anthropic (Messages API)</option>
                              <option value="litellm">
                                OpenAI-compatible host, Anthropic wire (LiteLLM Claude)
                              </option>
                            </select>
                          </label>
                          <label className={FIELD_LABEL}>
                            {t('settings.name')}
                            <input
                              data-testid="add-provider-name"
                              className={FIELD_INPUT}
                              value={addForm.name}
                              onChange={(e) =>
                                setAddForm((f) => ({ ...f, name: e.target.value }))
                              }
                              placeholder="OpenAI / Anthropic / Local"
                              autoComplete="off"
                            />
                          </label>
                          <label className={FIELD_LABEL}>
                            {t('settings.baseUrl')}
                            {addWire === 'anthropic' && (
                              <span className="ml-1">
                                (optional — defaults to https://api.anthropic.com)
                              </span>
                            )}
                            <input
                              data-testid="add-provider-base-url"
                              className={`${FIELD_INPUT} font-mono`}
                              value={addForm.base_url}
                              onChange={(e) =>
                                setAddForm((f) => ({ ...f, base_url: e.target.value }))
                              }
                              placeholder={
                                addWire === 'anthropic'
                                  ? 'https://api.anthropic.com'
                                  : 'https://api.openai.com/v1'
                              }
                              required={addWire !== 'anthropic'}
                              autoComplete="off"
                            />
                          </label>
                          <label className={FIELD_LABEL}>
                            {t('settings.apiKey')}
                            <input
                              data-testid="add-provider-api-key"
                              type="password"
                              className={`${FIELD_INPUT} font-mono`}
                              value={addForm.api_key}
                              onChange={(e) =>
                                setAddForm((f) => ({ ...f, api_key: e.target.value }))
                              }
                              placeholder="sk-…"
                              required
                              autoComplete="off"
                            />
                          </label>
                          <label className={FIELD_LABEL}>
                            {t('settings.defaultModel')}
                            <input
                              data-testid="add-provider-model"
                              className={`${FIELD_INPUT} font-mono`}
                              value={addForm.default_model}
                              onChange={(e) =>
                                setAddForm((f) => ({ ...f, default_model: e.target.value }))
                              }
                              placeholder="(auto from GET /v1/models)"
                              autoComplete="off"
                            />
                          </label>
                          {addError && (
                            <p
                              data-testid="add-provider-error"
                              className="text-[11px] text-red-300"
                            >
                              {addError}
                            </p>
                          )}
                          <button
                            type="submit"
                            data-testid="add-provider-submit"
                            disabled={
                              addBusy ||
                              !addForm.api_key.trim() ||
                              (addWire !== 'anthropic' && !addForm.base_url.trim())
                            }
                            className={BTN_PRIMARY}
                          >
                            {addBusy ? t('settings.saving') : t('settings.saveProfile')}
                          </button>
                        </form>
                      )}

                      <ul className="mt-2 space-y-1" data-testid="profiles-list">
                        {profiles.map((p) => {
                          const active = p.id === selectedProfile
                          return (
                            <li
                              key={p.id}
                              data-testid={`profile-${p.id}`}
                              data-profile-id={p.id}
                              data-active={active ? 'true' : 'false'}
                              className="shell-list-item rounded-md border-shell-border px-2.5 py-1.5 text-[11px]"
                            >
                              <div className="flex items-start justify-between gap-2">
                                <button
                                  type="button"
                                  className="min-w-0 flex-1 text-left"
                                  onClick={() => void onSelectProfile(p.id)}
                                  data-testid={`profile-select-${p.id}`}
                                  aria-pressed={active}
                                >
                                  <div className="flex flex-wrap items-center gap-1">
                                    <span className="font-medium text-shell-text">
                                      {p.name}
                                      {p.is_default ? ` ${t('settings.default')}` : ''}
                                    </span>
                                    {p.is_gateway && (
                                      <span className="rounded bg-shell-active px-1 text-[10px] text-shell-text">
                                        {t('settings.gatewayBadge')}
                                      </span>
                                    )}
                                    <span className="text-shell-muted">{p.type}</span>
                                    {p.api_format && p.api_format !== p.type && (
                                      <span className="rounded bg-shell-panel px-1 text-[10px] text-shell-muted">
                                        wire: {p.api_format}
                                      </span>
                                    )}
                                  </div>
                                  <div className="mt-0.5 truncate font-mono text-shell-muted">
                                    {displayBaseURL(p.base_url_masked, p.base_host)}
                                  </div>
                                  <div className="mt-0.5 text-shell-muted">
                                    model: {p.current_model || p.default_model || '—'} · key:{' '}
                                    {p.has_key ? t('settings.keySet') : t('settings.keyMissing')}
                                  </div>
                                </button>
                                <button
                                  type="button"
                                  data-testid={`profile-test-${p.id}`}
                                  disabled={testing}
                                  onClick={() => void onTest(p.id)}
                                  className={`${BTN_SECONDARY} shrink-0`}
                                >
                                  {t('settings.testProvider')}
                                </button>
                              </div>
                            </li>
                          )
                        })}
                        {profiles.length === 0 && (
                          <li className="text-[11px] text-shell-muted">
                            {t('settings.noProfiles')}
                          </li>
                        )}
                      </ul>
                    </section>

                    <div className="flex min-w-0 flex-col gap-3">
                      <section
                        data-testid="settings-models"
                        className="rounded-lg border border-shell-border bg-shell-panel p-3"
                      >
                        <h3 className={SECTION_TITLE}>{t('settings.models')}</h3>
                        <p className={`mt-0.5 ${SECTION_DESC}`}>
                          {t('settings.modelsHelp')}{' '}
                          <span className="font-mono text-shell-text">
                            {selectedMeta?.name || selectedProfile || '—'}
                          </span>
                        </p>
                        {view.models_error && (
                          <p
                            data-testid="models-error"
                            className="mt-1 text-[11px] text-amber-300"
                          >
                            {view.models_error}
                          </p>
                        )}
                        <label className={`mt-2 ${FIELD_LABEL}`}>
                          {t('settings.model')}
                          <select
                            data-testid="model-picker"
                            className={`${FIELD_INPUT} font-mono`}
                            value={selectedModel}
                            onChange={(e) => setSelectedModel(e.target.value)}
                          >
                            {models.length === 0 && !selectedModel && (
                              <option value="">
                                {view.models_error
                                  ? t('settings.noModelsError')
                                  : t('settings.noModels')}
                              </option>
                            )}
                            {selectedModel &&
                              !models.some((m) => m.id === selectedModel) && (
                                <option value={selectedModel} data-testid="model-id">
                                  {selectedModel}
                                </option>
                              )}
                            {models.map((m) => (
                              <option key={m.id} value={m.id} data-testid="model-id">
                                {m.id}
                                {m.owned_by ? ` · ${m.owned_by}` : ''}
                              </option>
                            ))}
                          </select>
                        </label>
                        <label className={`mt-2 ${FIELD_LABEL}`}>
                          {t('settings.modelManual')}
                          <input
                            data-testid="model-manual-input"
                            className={`${FIELD_INPUT} font-mono`}
                            value={selectedModel}
                            onChange={(e) => setSelectedModel(e.target.value)}
                            placeholder="gpt-4o-mini / claude-sonnet-4"
                            autoComplete="off"
                            spellCheck={false}
                          />
                        </label>
                        {models.length > 0 && (
                          <p className="mt-1 text-[10px] text-shell-muted">
                            {t('settings.modelsCount', { count: models.length })}
                          </p>
                        )}
                        <div className="mt-2 flex flex-wrap gap-2">
                          <button
                            type="button"
                            data-testid="apply-session-provider"
                            disabled={applyBusy || !selectedProfile}
                            onClick={() => void onApplySession(false)}
                            className={BTN_PRIMARY}
                          >
                            {applyBusy ? t('settings.applying') : t('settings.useForSession')}
                          </button>
                          <button
                            type="button"
                            data-testid="apply-default-provider"
                            disabled={applyBusy || !selectedProfile}
                            onClick={() => void onApplySession(true)}
                            className={BTN_SECONDARY}
                          >
                            {t('settings.setDefault')}
                          </button>
                        </div>
                        <p className="mt-1.5 text-[10px] leading-relaxed text-shell-muted">
                          {t('settings.sessionSwitchHelp')}
                        </p>
                      </section>

                      {testResult && (
                        <section
                          data-testid="provider-test-result"
                          className={`rounded-lg border px-3 py-2.5 text-[11px] text-shell-text ${
                            testResult.ok
                              ? 'border-emerald-500/60 bg-emerald-500/10'
                              : 'border-red-500/60 bg-red-500/10'
                          }`}
                        >
                          <div className="font-semibold">
                            {t('settings.testProvider')} {testResult.profile}:{' '}
                            {testResult.ok
                              ? t('settings.testSuccess')
                              : t('settings.testError')}
                          </div>
                          {testResult.host && (
                            <div className="mt-0.5 font-mono text-[10px] opacity-80">
                              host: {testResult.host}
                            </div>
                          )}
                          {testResult.model && (
                            <div className="mt-0.5 font-mono text-[10px] opacity-80">
                              model: {testResult.model}
                            </div>
                          )}
                          {testResult.ok && testResult.preview && (
                            <div data-testid="test-preview" className="mt-1">
                              preview: {testResult.preview}
                            </div>
                          )}
                          {!testResult.ok && testResult.error && (
                            <div data-testid="test-error" className="mt-1">
                              {testResult.error}
                            </div>
                          )}
                          {typeof testResult.latency_ms === 'number' && (
                            <div className="mt-0.5 text-[10px] opacity-70">
                              {testResult.latency_ms} ms
                            </div>
                          )}
                        </section>
                      )}
                    </div>
                  </div>
                </>
              )}
            </div>

            {/* --------------------------------------------------------- agent */}
            <div
              id="settings-category-agent"
              hidden={category !== 'agent'}
              className="space-y-3"
            >
              <section
                data-testid="settings-token-savers"
                className="rounded-lg border border-shell-border bg-shell-panel p-3"
              >
                <h3 className={SECTION_TITLE}>{t('settings.tokenSavers')}</h3>
                <p className={`mt-0.5 ${SECTION_DESC}`}>{t('settings.tokenSaversHelp')}</p>
                <p className="mt-1 text-[10px] leading-relaxed text-shell-muted">
                  {locale === 'id'
                    ? t('settings.tokenSaversNote')
                    : tokenSavers?.note || t('settings.tokenSaversNote')}
                </p>
                <ul className="mt-2 space-y-1.5" data-testid="token-savers-list">
                  {(tokenSavers?.savers || []).map((s) => {
                    const missing = isNotInstalled(s)
                    const busy = saverBusy === s.id
                    const setup = saverSetup(s.id)
                    const stored =
                      setup?.field === 'command' ? s.command || '' : s.base_url || ''
                    const draft = saverDrafts[s.id] ?? stored
                    // Masked values never round-trip; compare trimmed text so a
                    // no-op save stays disabled but any real edit is saveable.
                    const dirty = draft.trim() !== stored.trim()
                    const descKey =
                      s.id === 'rtk'
                        ? 'settings.saverRtkDesc'
                        : s.id === 'headroom'
                          ? 'settings.saverHeadroomDesc'
                          : s.id === 'caveman'
                            ? 'settings.saverCavemanDesc'
                            : s.id === 'ponytail'
                              ? 'settings.saverPonytailDesc'
                              : null
                    const description = descKey ? t(descKey) : s.description
                    return (
                      <li
                        key={s.id}
                        data-testid={`token-saver-${s.id}`}
                        data-saver-id={s.id}
                        data-enabled={s.enabled ? 'true' : 'false'}
                        data-status={s.status}
                        className={ROW}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-1.5">
                              <span className="text-[12px] font-semibold text-shell-text">
                                {s.name}
                              </span>
                              <span
                                data-testid={`token-saver-status-${s.id}`}
                                className={`rounded px-1.5 py-0.5 text-[10px] text-shell-text ${
                                  missing
                                    ? 'bg-amber-500/15'
                                    : 'bg-emerald-500/15'
                                }`}
                              >
                                {saverStatusLabel(s, {
                                  ready: t('settings.ready'),
                                  notInstalled: t('settings.notInstalled'),
                                })}
                              </span>
                            </div>
                            <p className="mt-0.5 text-[11px] leading-relaxed text-shell-muted">
                              {description}
                            </p>
                            {s.detail && missing && (
                              <p className="mt-0.5 font-mono text-[10px] text-shell-muted">
                                {s.detail}
                              </p>
                            )}
                          </div>
                          <label
                            className="flex shrink-0 cursor-pointer items-center gap-1.5 pt-0.5"
                            title={missing ? t('settings.notInstalledTitle') : undefined}
                          >
                            <span className="text-[10px] uppercase tracking-wide text-shell-muted">
                              {s.enabled ? t('settings.on') : t('settings.off')}
                            </span>
                            <button
                              type="button"
                              role="switch"
                              aria-checked={s.enabled}
                              data-testid={`token-saver-toggle-${s.id}`}
                              disabled={busy}
                              onClick={() => void onToggleSaver(s.id, !s.enabled)}
                              className={`shell-transition relative h-5 w-9 rounded-full border ${
                                s.enabled
                                  ? 'border-shell-accent bg-shell-accent'
                                  : 'border-shell-border bg-shell-panel'
                              } ${busy ? 'opacity-50' : ''}`}
                            >
                              <span
                                className={`absolute top-0.5 h-3.5 w-3.5 rounded-full bg-white shadow transition ${
                                  s.enabled ? 'left-[1.125rem]' : 'left-0.5'
                                }`}
                              />
                            </button>
                          </label>
                        </div>
                        {setup && (
                          <div className="mt-2 flex flex-wrap items-end gap-2">
                            <label
                              htmlFor={`token-saver-setup-${s.id}`}
                              className={`${FIELD_LABEL} min-w-[200px] flex-1`}
                            >
                              {setup.field === 'command'
                                ? t('settings.saverCommand')
                                : t('settings.saverBaseUrl')}
                              <input
                                id={`token-saver-setup-${s.id}`}
                                data-testid={`token-saver-setup-${s.id}`}
                                data-dirty={dirty ? 'true' : 'false'}
                                className={`${FIELD_INPUT} font-mono`}
                                value={draft}
                                placeholder={setup.placeholder}
                                onChange={(e) =>
                                  setSaverDrafts((d) => ({ ...d, [s.id]: e.target.value }))
                                }
                                onKeyDown={(e) => {
                                  if (e.key !== 'Enter' || busy || !dirty) return
                                  e.preventDefault()
                                  void onSaveSaverSetup(s, setup.field, draft)
                                }}
                                autoComplete="off"
                                spellCheck={false}
                              />
                            </label>
                            <button
                              type="button"
                              data-testid={`token-saver-setup-save-${s.id}`}
                              disabled={busy || !dirty}
                              onClick={() => void onSaveSaverSetup(s, setup.field, draft)}
                              className={BTN_SECONDARY}
                            >
                              {busy ? t('settings.saving') : t('settings.saveSaverSetup')}
                            </button>
                            {dirty && (
                              <button
                                type="button"
                                data-testid={`token-saver-setup-reset-${s.id}`}
                                disabled={busy}
                                onClick={() =>
                                  setSaverDrafts((d) => {
                                    const rest = { ...d }
                                    delete rest[s.id]
                                    return rest
                                  })
                                }
                                className={BTN_SECONDARY}
                              >
                                {t('action.cancel')}
                              </button>
                            )}
                          </div>
                        )}
                      </li>
                    )
                  })}
                  {!tokenSavers?.savers?.length && !loading && (
                    <li className="text-[11px] text-shell-muted">
                      {t('settings.loadingSavers')}
                    </li>
                  )}
                </ul>
              </section>

              <section
                data-testid="settings-live-blocks"
                className="rounded-lg border border-shell-border bg-shell-panel p-3"
              >
                <h3 className={SECTION_TITLE}>{t('settings.liveBlocks')}</h3>
                <p className={`mt-0.5 ${SECTION_DESC}`}>{t('settings.liveBlocksHelp')}</p>
                <p className="mt-1 text-[10px] leading-relaxed text-shell-muted">
                  {t('settings.liveBlocksHistoryNote')}
                </p>
                {liveBlocks?.note && (
                  <p className="mt-1 text-[10px] text-shell-muted">{liveBlocks.note}</p>
                )}
                <div
                  data-testid="live-blocks-toggle"
                  data-enabled={liveBlocks?.enabled ? 'true' : 'false'}
                  className={`mt-2 flex items-center justify-between gap-3 ${ROW}`}
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="text-[12px] font-semibold text-shell-text">
                        Live Blocks
                      </span>
                      <span
                        className={`rounded px-1.5 py-0.5 text-[10px] ${
                          liveBlocks?.enabled
                            ? 'bg-emerald-500/15 text-shell-text'
                            : 'bg-shell-panel text-shell-muted'
                        }`}
                      >
                        {liveBlocks?.enabled ? t('settings.on') : t('settings.off')}
                      </span>
                    </div>
                    <p className="mt-0.5 text-[11px] leading-relaxed text-shell-muted">
                      {t('settings.liveBlocksPreviewNote')}
                    </p>
                  </div>
                  <label
                    className="flex shrink-0 cursor-pointer items-center gap-1.5 pt-0.5"
                    title={t('settings.liveBlocksToggleTitle')}
                  >
                    <span className="text-[10px] uppercase tracking-wide text-shell-muted">
                      {liveBlocks?.enabled ? t('settings.on') : t('settings.off')}
                    </span>
                    <button
                      type="button"
                      role="switch"
                      aria-checked={!!liveBlocks?.enabled}
                      data-testid="live-blocks-toggle-switch"
                      disabled={liveBlocksBusy}
                      onClick={() => void onToggleLiveBlocks(!liveBlocks?.enabled)}
                      className={`shell-transition relative h-5 w-9 rounded-full border ${
                        liveBlocks?.enabled
                          ? 'border-shell-accent bg-shell-accent'
                          : 'border-shell-border bg-shell-panel'
                      } ${liveBlocksBusy ? 'opacity-50' : ''}`}
                    >
                      <span
                        className={`absolute top-0.5 h-3.5 w-3.5 rounded-full bg-white shadow transition ${
                          liveBlocks?.enabled ? 'left-[1.125rem]' : 'left-0.5'
                        }`}
                      />
                    </button>
                  </label>
                </div>
              </section>

              <section
                data-testid="settings-mcp"
                className="rounded-lg border border-shell-border bg-shell-panel p-3"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <h3 className={SECTION_TITLE}>{t('settings.mcpServers')}</h3>
                    <p className={`mt-0.5 ${SECTION_DESC}`}>{t('settings.mcpHelp')}</p>
                  </div>
                  <button
                    type="button"
                    data-testid="mcp-reload"
                    onClick={() => void onReloadMCP()}
                    disabled={mcpBusy === '__reload__'}
                    className={`${BTN_SECONDARY} inline-flex shrink-0 items-center gap-1`}
                  >
                    <RefreshCw size={12} aria-hidden />
                    {mcpBusy === '__reload__' ? t('settings.mcpReloading') : t('action.refresh')}
                  </button>
                </div>

                {mcpStatus && mcpStatus.total_servers > 0 ? (
                  <div className="mt-2 space-y-1.5">
                    <div className="flex flex-wrap gap-2 text-[10px] text-shell-muted">
                      {/* Health counts only enabled servers: a server the user
                          switched off is not a failure. */}
                      <span
                        data-testid="mcp-health-summary"
                        className="rounded bg-shell-bg px-1.5 py-0.5"
                      >
                        {t('settings.mcpHealthy', {
                          healthy: mcpStatus.healthy_count,
                          enabled: mcpStatus.enabled_count,
                        })}
                      </span>
                      {mcpStatus.enabled_count < mcpStatus.total_servers && (
                        <span className="rounded bg-shell-bg px-1.5 py-0.5">
                          {t('settings.mcpDisabledCount', {
                            count: mcpStatus.total_servers - mcpStatus.enabled_count,
                          })}
                        </span>
                      )}
                      <span className="rounded bg-shell-bg px-1.5 py-0.5">
                        {t('settings.mcpToolCount', { count: mcpStatus.total_tools })}
                      </span>
                    </div>
                    {mcpStatus.servers.map((srv) => (
                      <MCPServerRow
                        key={srv.name}
                        srv={srv}
                        busy={mcpBusy === srv.name}
                        t={t}
                        onToggle={(enabled) => void onToggleMCP(srv.name, enabled)}
                      />
                    ))}
                  </div>
                ) : (
                  <div className={`mt-2 text-[11px] text-shell-muted ${ROW}`}>
                    {t('settings.mcpEmpty')}{' '}
                    <code className="font-mono text-shell-text">
                      {view?.config_home || '~/.inferenesia'}/config.yaml
                    </code>{' '}
                    <code className="font-mono text-shell-text">mcp:</code>
                    <div className="mt-2 rounded bg-shell-panel px-2 py-1.5 font-mono text-[10px] text-shell-muted">
                      mcp:
                      <br />
                      &nbsp;&nbsp;my-server:
                      <br />
                      &nbsp;&nbsp;&nbsp;&nbsp;type: stdio
                      <br />
                      &nbsp;&nbsp;&nbsp;&nbsp;command: /path/to/server
                    </div>
                    <div className="mt-2 rounded border border-indigo-800/30 bg-indigo-950/20 px-2.5 py-2">
                      <div className="flex items-center justify-between gap-2">
                        <div className="min-w-0">
                          <span className="text-[11px] font-semibold text-indigo-200">
                            CocoIndex
                          </span>
                          <p className="mt-0.5 text-[10px] text-indigo-200/70">
                            {t('settings.mcpCocoindexHelp')}
                          </p>
                        </div>
                        <button
                          type="button"
                          data-testid="mcp-install-cocoindex"
                          disabled={!onOpenTerminalWithCommand}
                          title={
                            onOpenTerminalWithCommand
                              ? undefined
                              : t('settings.mcpInstallUnavailable')
                          }
                          onClick={() =>
                            onOpenTerminalWithCommand?.(
                              "pipx install 'cocoindex-code[full]'",
                              'CocoIndex Install',
                            )
                          }
                          className="shell-transition shrink-0 rounded-md border border-indigo-700/50 bg-indigo-800/30 px-2.5 py-1 text-[10px] font-medium text-indigo-100 hover:bg-indigo-700/40 disabled:opacity-40"
                        >
                          {t('settings.mcpInstall')}
                        </button>
                      </div>
                    </div>
                  </div>
                )}
              </section>

              <section
                data-testid="settings-autonomy"
                className="rounded-lg border border-shell-border bg-shell-panel p-3"
              >
                <h3 className={SECTION_TITLE}>{t('settings.autonomy')}</h3>
                <p className={`mt-0.5 ${SECTION_DESC}`}>{t('settings.autonomyHelp')}</p>
                <div
                  role="group"
                  aria-label={t('settings.autonomy')}
                  className="mt-2 flex flex-col gap-1.5"
                >
                  {AUTONOMY_LEVELS.map((lvl) => {
                    const active = autonomy?.level === lvl
                    return (
                      <button
                        key={lvl}
                        type="button"
                        data-testid={`autonomy-${lvl}`}
                        data-active={active ? 'true' : 'false'}
                        aria-pressed={active}
                        disabled={autonomyBusy}
                        onClick={() => void onSetAutonomy(lvl)}
                        className={`shell-transition flex items-baseline gap-2 rounded-md border px-2.5 py-1.5 text-left disabled:opacity-50 ${
                          active
                            ? AUTONOMY_ACTIVE_CLASS[lvl]
                            : 'border-shell-border bg-shell-bg text-shell-text hover:bg-shell-hover'
                        }`}
                      >
                        <span className="w-16 shrink-0 text-[12px] font-semibold">
                          {t(`settings.autonomyLevel.${lvl}` as const)}
                        </span>
                        <span className="min-w-0 text-[11px] opacity-80">
                          {t(`settings.autonomyLevelDesc.${lvl}` as const)}
                        </span>
                      </button>
                    )
                  })}
                </div>
                {autonomy ? (
                  <p
                    data-testid="autonomy-description"
                    className="mt-2 text-[10px] leading-relaxed text-shell-muted"
                  >
                    {autonomy.description}
                  </p>
                ) : (
                  !loading && (
                    <p className="mt-2 text-[10px] text-shell-muted">
                      {t('settings.autonomyUnavailable')}
                    </p>
                  )
                )}
              </section>
            </div>

            {/* ----------------------------------------------------- workspace */}
            <div
              id="settings-category-workspace"
              hidden={category !== 'workspace'}
              className="space-y-3"
            >
              {view && (
                <section
                  data-testid="settings-workspace-prefs"
                  className="rounded-lg border border-shell-border bg-shell-panel p-3"
                >
                  <h3 className={SECTION_TITLE}>{t('settings.workspacePrefs')}</h3>
                  <div className={`mt-2 space-y-1 text-[11px] text-shell-text ${ROW}`}>
                    <div>
                      {t('settings.active')}:{' '}
                      <span className="font-medium">
                        {view.workspace.active_name || t('settings.none')}
                      </span>
                    </div>
                    {view.workspace.root_path && (
                      <div
                        className="truncate font-mono text-shell-muted"
                        title={view.workspace.root_path}
                      >
                        {view.workspace.root_path}
                      </div>
                    )}
                    <div className="pt-1 leading-relaxed text-shell-muted">
                      {locale === 'id'
                        ? t('settings.workspaceUndoNote')
                        : view.workspace.undo_note || t('settings.workspaceUndoNote')}
                    </div>
                    <div className="flex flex-wrap gap-1 pt-1">
                      <span
                        data-testid="settings-label-revert-agent"
                        className="rounded bg-amber-950/50 px-1.5 py-0.5 text-amber-200"
                      >
                        {view.labels.revert_agent}
                      </span>
                      <span
                        data-testid="settings-label-restore-head"
                        className="rounded bg-red-950/40 px-1.5 py-0.5 text-red-200"
                      >
                        {view.labels.restore_to_head}
                      </span>
                    </div>
                  </div>
                </section>
              )}

              {extraSections.length > 0 && (
                <section data-testid="settings-sections" className="space-y-1.5">
                  <h3 className={SECTION_TITLE}>{t('settings.more')}</h3>
                  {extraSections.map((s) => (
                    <div
                      key={s.id}
                      data-testid={`settings-section-${s.id}`}
                      className={ROW}
                    >
                      <div className="text-[12px] font-semibold text-shell-text">
                        {s.title}
                      </div>
                      <p className="mt-0.5 text-[11px] leading-relaxed text-shell-muted">
                        {s.description}
                      </p>
                    </div>
                  ))}
                </section>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function MCPServerRow({
  srv,
  busy,
  t,
  onToggle,
}: {
  srv: MCPServerView
  busy: boolean
  t: LocaleContextValue['t']
  onToggle: (enabled: boolean) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const hasTools = !!srv.tool_names?.length
  // Env-supplied servers win over anything Settings can persist, so the toggle
  // is presented as read-only rather than as an action that always fails.
  const readOnly = !!srv.managed
  // A disabled server is intentionally not running; only an enabled one can be
  // "error". This keeps an opt-out from looking like a broken integration.
  const health = !srv.enabled ? 'off' : srv.ok ? 'running' : 'error'
  return (
    <div
      data-testid={`mcp-server-${srv.name}`}
      data-enabled={srv.enabled ? 'true' : 'false'}
      data-managed={readOnly ? 'true' : 'false'}
      data-layer={srv.layer || 'home'}
      className="rounded border border-shell-border bg-shell-bg px-3 py-2"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <button
              type="button"
              aria-expanded={hasTools ? expanded : undefined}
              disabled={!hasTools}
              onClick={() => setExpanded((v) => !v)}
              className="text-xs font-semibold text-shell-text hover:text-shell-accent disabled:cursor-default disabled:hover:text-shell-text"
            >
              {srv.name}
            </button>
            {srv.builtin && (
              <span className="rounded bg-indigo-950/40 px-1.5 py-0.5 text-[9px] font-medium text-indigo-200">
                {t('settings.mcpBuiltin')}
              </span>
            )}
            <span
              data-testid={`mcp-health-${srv.name}`}
              className={`rounded px-1.5 py-0.5 text-[9px] ${
                health === 'running'
                  ? 'bg-emerald-950/40 text-emerald-200'
                  : health === 'error'
                    ? 'bg-red-950/40 text-red-200'
                    : 'bg-shell-panel text-shell-muted'
              }`}
            >
              {health === 'running'
                ? t('settings.mcpRunning')
                : health === 'error'
                  ? t('settings.mcpError')
                  : t('settings.disabled')}
            </span>
            {readOnly && (
              <span
                data-testid={`mcp-managed-${srv.name}`}
                title={t('settings.mcpManagedTitle')}
                className="rounded bg-amber-950/40 px-1.5 py-0.5 text-[9px] text-amber-200"
              >
                {t('settings.mcpManaged')}
              </span>
            )}
            {srv.layer === 'project' && (
              <span className="rounded bg-shell-panel px-1.5 py-0.5 text-[9px] text-shell-muted">
                {t('settings.mcpLayerProject')}
              </span>
            )}
            <span className="rounded bg-shell-panel px-1.5 py-0.5 text-[9px] text-shell-muted">
              {srv.type}
            </span>
            {srv.tool_count > 0 && (
              <span className="rounded bg-shell-panel px-1.5 py-0.5 text-[9px] text-shell-muted">
                {t('settings.mcpToolCount', { count: srv.tool_count })}
              </span>
            )}
          </div>
          {srv.command && (
            <p
              className="mt-0.5 truncate font-mono text-[10px] text-shell-muted"
              title={srv.command}
            >
              {srv.command}
            </p>
          )}
          {srv.base_url && (
            <p
              className="mt-0.5 truncate font-mono text-[10px] text-shell-muted"
              title={srv.base_url}
            >
              {srv.base_url}
            </p>
          )}
          {srv.error && srv.enabled && (
            <p className="mt-0.5 text-[10px] text-red-300">{srv.error}</p>
          )}
          {expanded && hasTools && (
            <div className="mt-1.5 flex flex-wrap gap-1">
              {srv.tool_names?.map((tn) => (
                <span
                  key={tn}
                  className="rounded bg-shell-panel px-1.5 py-0.5 font-mono text-[9px] text-shell-muted"
                >
                  {tn}
                </span>
              ))}
            </div>
          )}
        </div>
        <label
          className={`flex shrink-0 items-center gap-1.5 pt-0.5 ${
            readOnly ? '' : 'cursor-pointer'
          }`}
          title={readOnly ? t('settings.mcpManagedTitle') : undefined}
        >
          <span className="text-[9px] uppercase tracking-wide text-shell-muted">
            {srv.enabled ? t('settings.on') : t('settings.off')}
          </span>
          <button
            type="button"
            role="switch"
            aria-checked={srv.enabled}
            aria-label={t('settings.mcpToggleLabel', { name: srv.name })}
            aria-disabled={readOnly || undefined}
            data-testid={`mcp-toggle-${srv.name}`}
            disabled={busy || readOnly}
            onClick={() => onToggle(!srv.enabled)}
            className={`shell-transition relative h-4 w-7 rounded-full border ${
              srv.enabled
                ? 'border-shell-accent bg-shell-accent'
                : 'border-shell-border bg-shell-panel'
            } ${busy || readOnly ? 'opacity-50' : ''} ${
              readOnly ? 'cursor-not-allowed' : ''
            }`}
          >
            <span
              className={`absolute top-0.5 h-2.5 w-2.5 rounded-full bg-white shadow transition ${
                srv.enabled ? 'left-[0.875rem]' : 'left-0.5'
              }`}
            />
          </button>
        </label>
      </div>
    </div>
  )
}
