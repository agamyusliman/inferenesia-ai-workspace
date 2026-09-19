import { useMemo, useState } from 'react'
import { AlignCenter, AlignLeft, AlignRight, Bold, Grid2x2X, Italic, PaintBucket, Palette, Split } from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import type { TableCellStyle } from './tableDoc'

const SWATCHES = [
  '1F4E79', '2E75B6', '548235', 'BF8F00', 'C00000', '7030A0',
  'D9E2F3', 'E2EFDA', 'FFF2CC', 'FCE4D6', 'FFE599', 'F2F2F2',
  'FFFFFF', 'D0CECE', '808080', '1F2937',
]

const NUM_FORMATS: Array<{ id: string; label: string; value: string }> = [
  { id: 'general', label: 'General', value: '' },
  { id: 'thousands', label: '1,234', value: '#,##0' },
  { id: 'decimal', label: '1,234.00', value: '#,##0.00' },
  { id: 'percent', label: '12%', value: '0%' },
  { id: 'percent2', label: '12.34%', value: '0.00%' },
  { id: 'currency', label: 'Rp 1,234', value: '"Rp"#,##0' },
]

type Props = {
  style: TableCellStyle | undefined
  selectionSize: number
  onPatch: (patch: TableCellStyle | null) => void
  onMerge: () => void
  onUnmerge: () => void
  canMerge: boolean
  canUnmerge: boolean
}

export function TableFormatToolbar({
  style,
  selectionSize,
  onPatch,
  onMerge,
  onUnmerge,
  canMerge,
  canUnmerge,
}: Props) {
  const { t } = useLocale()
  const [showColors, setShowColors] = useState(false)
  const active = useMemo(() => style || {}, [style])

  const btn = (on: boolean) =>
    `inline-flex h-6 w-6 items-center justify-center rounded transition ${
      on ? 'bg-shell-active text-shell-accent' : 'text-shell-muted hover:bg-shell-hover hover:text-shell-text'
    }`

  const disabled = selectionSize === 0

  return (
    <div
      data-testid="table-doc-format-bar"
      className="flex shrink-0 flex-wrap items-center gap-0.5 border-b border-shell-border bg-shell-panel px-1 py-1"
    >
      <button
        type="button"
        data-testid="table-doc-bold"
        data-active={active.bold ? 'true' : 'false'}
        disabled={disabled}
        onClick={() => onPatch({ bold: !active.bold })}
        title={t('tableDoc.bold')}
        aria-label={t('tableDoc.bold')}
        aria-pressed={Boolean(active.bold)}
        className={btn(Boolean(active.bold))}
      >
        <Bold size={12} strokeWidth={2.5} aria-hidden />
      </button>
      <button
        type="button"
        data-testid="table-doc-italic"
        data-active={active.italic ? 'true' : 'false'}
        disabled={disabled}
        onClick={() => onPatch({ italic: !active.italic })}
        title={t('tableDoc.italic')}
        aria-label={t('tableDoc.italic')}
        aria-pressed={Boolean(active.italic)}
        className={btn(Boolean(active.italic))}
      >
        <Italic size={12} strokeWidth={2.5} aria-hidden />
      </button>

      <span className="mx-0.5 h-4 w-px bg-shell-border" aria-hidden />

      {(
        [
          ['left', AlignLeft],
          ['center', AlignCenter],
          ['right', AlignRight],
        ] as const
      ).map(([id, Icon]) => (
        <button
          key={id}
          type="button"
          data-testid={`table-doc-align-${id}`}
          data-active={active.align === id ? 'true' : 'false'}
          disabled={disabled}
          onClick={() => onPatch({ align: id })}
          title={t(`tableDoc.align.${id}`)}
          aria-label={t(`tableDoc.align.${id}`)}
          aria-pressed={active.align === id}
          className={btn(active.align === id)}
        >
          <Icon size={12} strokeWidth={2} aria-hidden />
        </button>
      ))}

      <span className="mx-0.5 h-4 w-px bg-shell-border" aria-hidden />

      <div className="relative">
        <button
          type="button"
          data-testid="table-doc-fill-toggle"
          data-active={showColors ? 'true' : 'false'}
          disabled={disabled}
          onClick={() => setShowColors((v) => !v)}
          title={t('tableDoc.fill')}
          aria-label={t('tableDoc.fill')}
          aria-expanded={showColors}
          className={btn(showColors)}
        >
          <PaintBucket size={12} strokeWidth={2} aria-hidden />
        </button>
        {showColors ? (
          <div
            data-testid="table-doc-fill-palette"
            className="absolute left-0 top-7 z-30 w-[164px] rounded-md border border-shell-border bg-shell-panel p-1.5 shadow-lg"
          >
            <div className="mb-1 flex items-center justify-between gap-1">
              <span className="text-[9px] font-semibold uppercase tracking-wide text-shell-muted">
                {t('tableDoc.fill')}
              </span>
              <button
                type="button"
                data-testid="table-doc-fill-none"
                onClick={() => {
                  onPatch({ fill: undefined })
                  setShowColors(false)
                }}
                className="rounded px-1 text-[9px] text-shell-muted hover:text-shell-text"
              >
                {t('tableDoc.clear')}
              </button>
            </div>
            <div className="grid grid-cols-8 gap-1">
              {SWATCHES.map((hex) => (
                <button
                  key={`fill-${hex}`}
                  type="button"
                  data-testid={`table-doc-fill-${hex}`}
                  onClick={() => {
                    onPatch({ fill: hex })
                    setShowColors(false)
                  }}
                  title={`#${hex}`}
                  aria-label={`#${hex}`}
                  style={{ backgroundColor: `#${hex}` }}
                  className="h-4 w-4 rounded-sm border border-shell-border transition hover:scale-110"
                />
              ))}
            </div>

            <div className="mb-1 mt-2 flex items-center gap-1">
              <Palette size={10} strokeWidth={2} className="text-shell-muted" aria-hidden />
              <span className="text-[9px] font-semibold uppercase tracking-wide text-shell-muted">
                {t('tableDoc.textColor')}
              </span>
            </div>
            <div className="grid grid-cols-8 gap-1">
              {SWATCHES.map((hex) => (
                <button
                  key={`color-${hex}`}
                  type="button"
                  data-testid={`table-doc-color-${hex}`}
                  onClick={() => {
                    onPatch({ color: hex })
                    setShowColors(false)
                  }}
                  title={`#${hex}`}
                  aria-label={`#${hex}`}
                  style={{ backgroundColor: `#${hex}` }}
                  className="h-4 w-4 rounded-sm border border-shell-border transition hover:scale-110"
                />
              ))}
            </div>
          </div>
        ) : null}
      </div>

      <select
        data-testid="table-doc-numfmt"
        disabled={disabled}
        value={active.numFmt || ''}
        onChange={(e) => onPatch({ numFmt: e.target.value || undefined })}
        title={t('tableDoc.numberFormat')}
        aria-label={t('tableDoc.numberFormat')}
        className="h-6 rounded-md border border-shell-border bg-shell-bg px-1 text-[10px] text-shell-muted outline-none hover:text-shell-text disabled:opacity-40"
      >
        {NUM_FORMATS.map((f) => (
          <option key={f.id} value={f.value}>
            {f.label}
          </option>
        ))}
      </select>

      <span className="mx-0.5 h-4 w-px bg-shell-border" aria-hidden />

      <button
        type="button"
        data-testid="table-doc-merge"
        disabled={!canMerge}
        onClick={onMerge}
        title={t('tableDoc.merge')}
        aria-label={t('tableDoc.merge')}
        className={btn(false)}
      >
        <Grid2x2X size={12} strokeWidth={2} aria-hidden />
      </button>
      <button
        type="button"
        data-testid="table-doc-unmerge"
        disabled={!canUnmerge}
        onClick={onUnmerge}
        title={t('tableDoc.unmerge')}
        aria-label={t('tableDoc.unmerge')}
        className={btn(false)}
      >
        <Split size={12} strokeWidth={2} aria-hidden />
      </button>

      <span
        data-testid="table-doc-selection-info"
        className="ml-auto shrink-0 px-1 text-[10px] text-shell-muted"
      >
        {selectionSize === 0
          ? t('tableDoc.noSelection')
          : t('tableDoc.selectionCount', { count: selectionSize })}
      </span>
    </div>
  )
}

export function dragClasses({
  dragging,
  dropTarget,
}: {
  dragging: boolean
  dropTarget: boolean
}) {
  if (dragging) return 'opacity-40'
  if (dropTarget) return 'ring-1 ring-inset ring-shell-accent'
  return ''
}
