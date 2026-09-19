import { useEffect, useRef, useState, type ReactNode } from 'react'
import { AlertTriangle, Save, X } from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'

/** Shared toolbar button style for the document playground headers. */
export const DOC_PANEL_TOOL_BUTTON =
  'inline-flex h-6 min-w-[1.5rem] items-center justify-center gap-1 rounded-md border border-shell-border bg-shell-bg px-1.5 text-[10px] text-shell-muted transition hover:bg-shell-hover hover:text-shell-text disabled:opacity-40 data-[active=true]:bg-shell-active data-[active=true]:text-shell-accent'

/**
 * Below this detail width a code+preview split leaves neither pane usable, so
 * the document panels stack instead. Matches the Playground HTML/Mermaid rule.
 */
export const DOC_PANEL_NARROW_PX = 720

export function useDocPanelNarrow(breakpoint = DOC_PANEL_NARROW_PX) {
  const ref = useRef<HTMLDivElement | null>(null)
  const [narrow, setNarrow] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const apply = (w: number) => {
      if (w <= 0) return
      setNarrow((prev) => (prev === w < breakpoint ? prev : w < breakpoint))
    }
    if (typeof ResizeObserver === 'undefined') {
      apply(el.clientWidth)
      return
    }
    const ro = new ResizeObserver((entries) => {
      const cr = entries[0]?.contentRect
      if (cr) apply(cr.width)
    })
    ro.observe(el)
    apply(el.clientWidth)
    return () => ro.disconnect()
  }, [breakpoint])
  return { ref, narrow }
}

export function downloadDocFile(
  content: string,
  filename: string,
  mime: string,
): void {
  const safe = filename.replace(/[/\\:*?"<>|]+/g, '_')
  const blob = new Blob([content], { type: `${mime};charset=utf-8` })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = safe
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 2000)
}

export type PickedTextFile = {
  name: string
  text: string
  error?: string
}

const MAX_IMPORT_BYTES = 4 * 1024 * 1024

/**
 * Opens the OS file dialog and reads one text file. Resolves `null` when the
 * user cancels, and an `error` payload for oversized or unreadable files.
 */
export function pickTextFile(accept: string): Promise<PickedTextFile | null> {
  // Executor form: the project targets ES2022, where Promise.withResolvers
  // is not available.
  let resolve!: (value: PickedTextFile | null) => void
  const promise = new Promise<PickedTextFile | null>((r) => {
    resolve = r
  })
  if (typeof document === 'undefined') {
    resolve(null)
    return promise
  }
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = accept
  input.style.display = 'none'
  let settled = false
  const finish = (value: PickedTextFile | null) => {
    if (settled) return
    settled = true
    input.remove()
    resolve(value)
  }
  input.addEventListener('change', () => {
    const file = input.files?.[0]
    if (!file) {
      finish(null)
      return
    }
    if (file.size > MAX_IMPORT_BYTES) {
      finish({
        name: file.name,
        text: '',
        error: `${file.name} is too large (max 4 MB)`,
      })
      return
    }
    const reader = new FileReader()
    reader.onerror = () =>
      finish({ name: file.name, text: '', error: `Could not read ${file.name}` })
    reader.onload = () =>
      finish({ name: file.name, text: String(reader.result || '') })
    reader.readAsText(file)
  })
  // Cancelling the dialog fires no change event; regaining window focus is the
  // only cross-browser signal. The delay lets a real `change` win the race.
  window.addEventListener(
    'focus',
    () => window.setTimeout(() => finish(null), 500),
    { once: true },
  )
  document.body.appendChild(input)
  input.click()
  return promise
}

type HeaderProps = {
  title: string
  scopeLabel: string
  dirty: boolean
  saveFlash: boolean
  statusText?: string
  /** Narrow detail pane: hide the stats line to keep tools reachable. */
  compact?: boolean
  error?: string | null
  onSave: () => void
  onDismissError?: () => void
  tools?: ReactNode
  testId: string
}

/** Title + scope + dirty/save affordance shared by the 3 document panels. */
export function DocPanelHeader({
  title,
  scopeLabel,
  dirty,
  saveFlash,
  statusText,
  compact,
  error,
  onSave,
  onDismissError,
  tools,
  testId,
}: HeaderProps) {
  const { t } = useLocale()
  return (
    <div className="shrink-0 border-b border-shell-border bg-shell-panel">
      <div
        data-testid={`${testId}-header`}
        className="flex min-h-10 flex-wrap items-center gap-1.5 px-2 py-1"
      >
        <span
          className="min-w-0 flex-1 truncate text-[11px] font-semibold text-shell-text"
          title={`${title} · ${scopeLabel}`}
        >
          {title}
          <span className="font-normal text-shell-muted">
            {' · '}
            {scopeLabel}
          </span>
        </span>
        {statusText && !compact ? (
          <span
            data-testid={`${testId}-stats`}
            className="shrink-0 text-[10px] tabular-nums text-shell-muted"
          >
            {statusText}
          </span>
        ) : null}
        <div className="ml-auto flex shrink-0 flex-wrap items-center justify-end gap-1.5">
          {tools}
          <button
            type="button"
            data-testid={`${testId}-save`}
            data-dirty={dirty ? 'true' : 'false'}
            disabled={!dirty}
            onClick={onSave}
            title={t('editor.saveTitle')}
            aria-label={t('action.save')}
            className={`inline-flex h-6 items-center gap-1 rounded-md border px-2 text-[10px] transition disabled:cursor-default ${
              dirty
                ? 'border-shell-accent text-shell-accent hover:bg-shell-hover'
                : 'border-shell-border text-shell-muted'
            }`}
          >
            <Save size={11} strokeWidth={2} aria-hidden />
            {dirty
              ? t('editor.unsaved')
              : saveFlash
                ? t('canvasGallery.saved')
                : t('action.save')}
          </button>
        </div>
      </div>
      {error ? (
        <div
          data-testid={`${testId}-error`}
          role="alert"
          className="flex items-start gap-1.5 border-t border-shell-border bg-shell-bg px-2 py-1.5 text-[10px] text-red-400"
        >
          <AlertTriangle size={12} strokeWidth={2} className="mt-px shrink-0" aria-hidden />
          <span className="min-w-0 flex-1 break-words">{error}</span>
          {onDismissError ? (
            <button
              type="button"
              data-testid={`${testId}-error-dismiss`}
              onClick={onDismissError}
              title={t('action.close')}
              aria-label={t('action.close')}
              className="shrink-0 rounded p-0.5 text-shell-muted transition hover:text-shell-text"
            >
              <X size={11} strokeWidth={2} aria-hidden />
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
