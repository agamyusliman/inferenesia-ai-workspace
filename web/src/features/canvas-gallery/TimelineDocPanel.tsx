import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Braces,
  Download,
  GanttChartSquare,
  List,
  Plus,
  Trash2,
  Upload,
} from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import type { MessageKey } from '../i18n/messages'
import { DocAgentPanel } from './DocAgentPanel'
import {
  DOC_PANEL_TOOL_BUTTON,
  DocPanelHeader,
  downloadDocFile,
  pickTextFile,
  useDocPanelNarrow,
} from './docPanelChrome'
import {
  newMilestoneId,
  parseTimelineDoc,
  serializeTimelineDoc,
  shiftIso,
  sortTimelineMilestones,
  timelineBounds,
  timelineSpan,
  timelineToCsv,
  timelineToMarkdown,
  timelineToMermaidGantt,
  todayIso,
  TIMELINE_STATUSES,
  type TimelineDoc,
  type TimelineMilestone,
  type TimelineStatus,
} from './timelineDoc'

type Props = {
  contextId?: string
  canvasKey: string
  canvasEntrySessionId?: string
  title: string
  scopeLabel: string
  content: string
  dirty: boolean
  saveFlash: boolean
  error?: string | null
  onContentChange: (next: string) => void
  onSave: (next: string) => void
  onApplyFromAgent: (next: string, opts: { canvasId: string }) => void
  onDismissError?: () => void
}

const STATUS_LABEL_KEY: Record<TimelineStatus, MessageKey> = {
  planned: 'timelineDoc.statusPlanned',
  in_progress: 'timelineDoc.statusInProgress',
  done: 'timelineDoc.statusDone',
  blocked: 'timelineDoc.statusBlocked',
}

const STATUS_BAR_CLASS: Record<TimelineStatus, string> = {
  planned: 'bg-shell-border',
  in_progress: 'bg-shell-accent',
  done: 'bg-emerald-500/80',
  blocked: 'bg-red-500/80',
}

const STATUS_DOT_CLASS: Record<TimelineStatus, string> = {
  planned: 'border-shell-border bg-shell-panel',
  in_progress: 'border-shell-accent bg-shell-accent',
  done: 'border-emerald-500 bg-emerald-500',
  blocked: 'border-red-500 bg-red-500',
}

export function TimelineDocPanel({
  contextId,
  canvasKey,
  canvasEntrySessionId,
  title,
  scopeLabel,
  content,
  dirty,
  saveFlash,
  error,
  onContentChange,
  onSave,
  onApplyFromAgent,
  onDismissError,
}: Props) {
  const { t } = useLocale()
  const [view, setView] = useState<'timeline' | 'list'>('timeline')
  const [localError, setLocalError] = useState<string | null>(null)
  const contentRef = useRef(content)
  contentRef.current = content
  const { ref: panelRef, narrow } = useDocPanelNarrow(560)

  const parsed = useMemo(() => parseTimelineDoc(content), [content])
  const doc = parsed.doc
  const ordered = useMemo(
    () => sortTimelineMilestones(doc.milestones),
    [doc.milestones],
  )
  const bounds = useMemo(() => timelineBounds(doc), [doc])

  useEffect(() => {
    setLocalError(null)
  }, [canvasKey])

  const commit = useCallback(
    (next: TimelineDoc) => {
      onContentChange(serializeTimelineDoc(next))
    },
    [onContentChange],
  )

  const patchMilestone = useCallback(
    (id: string, patch: Partial<TimelineMilestone>) => {
      commit({
        ...doc,
        milestones: doc.milestones.map((m) =>
          m.id === id ? { ...m, ...patch } : m,
        ),
      })
    },
    [commit, doc],
  )

  const addMilestone = useCallback(() => {
    const last = ordered[ordered.length - 1]
    const start = last?.end || last?.start || todayIso()
    commit({
      ...doc,
      milestones: [
        ...doc.milestones,
        {
          id: newMilestoneId(),
          title: t('timelineDoc.newMilestone'),
          start: shiftIso(start, 7) || todayIso(),
          end: '',
          status: 'planned',
          owner: '',
          notes: '',
        },
      ],
    })
  }, [commit, doc, ordered, t])

  const onImport = useCallback(async () => {
    setLocalError(null)
    const picked = await pickTextFile(
      '.csv,.txt,.json,text/csv,application/json,text/plain',
    )
    if (!picked) return
    if (picked.error) {
      setLocalError(picked.error)
      return
    }
    // parseTimelineDoc accepts CSV and JSON and reports the first bad date,
    // status or ordering instead of quietly rewriting the file's data.
    const imported = parseTimelineDoc(picked.text)
    if (imported.error) {
      setLocalError(`${picked.name}: ${imported.error}`)
      return
    }
    if (imported.doc.milestones.length === 0) {
      setLocalError(t('timelineDoc.importEmpty', { name: picked.name }))
      return
    }
    onSave(serializeTimelineDoc({ ...imported.doc, title: doc.title }))
  }, [doc.title, onSave, t])

  return (
    <div
      ref={panelRef}
      data-testid="timeline-doc-panel"
      data-narrow={narrow ? 'true' : 'false'}
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
    >
      <DocPanelHeader
        compact={narrow}
        title={title}
        scopeLabel={scopeLabel}
        dirty={dirty}
        saveFlash={saveFlash}
        statusText={t('timelineDoc.stats', {
          count: doc.milestones.length,
          days: bounds?.days ?? 0,
        })}
        error={localError || parsed.error || error || null}
        onDismissError={() => {
          setLocalError(null)
          onDismissError?.()
        }}
        onSave={() => onSave(contentRef.current)}
        testId="timeline-doc"
        tools={
          <>
            <div
              className="inline-flex h-6 overflow-hidden rounded-md border border-shell-border bg-shell-bg"
              role="group"
              aria-label={t('timelineDoc.viewMode')}
            >
              {(
                [
                  ['timeline', t('timelineDoc.viewTimeline'), GanttChartSquare],
                  ['list', t('timelineDoc.viewList'), List],
                ] as Array<['timeline' | 'list', string, typeof List]>
              ).map(([id, label, Icon], i) => {
                const active = view === id
                return (
                  <button
                    key={id}
                    type="button"
                    data-testid={`timeline-doc-view-${id}`}
                    data-active={active ? 'true' : 'false'}
                    aria-pressed={active}
                    title={label}
                    onClick={() => setView(id)}
                    className={`inline-flex h-6 items-center gap-1 px-1.5 text-[10px] transition ${
                      active
                        ? 'bg-shell-active text-shell-accent'
                        : 'text-shell-muted hover:text-shell-text'
                    } ${i === 0 ? 'border-r border-shell-border' : ''}`}
                  >
                    <Icon size={11} strokeWidth={2} aria-hidden />
                    {narrow ? null : <span>{label}</span>}
                  </button>
                )
              })}
            </div>
            <button
              type="button"
              data-testid="timeline-doc-add"
              onClick={addMilestone}
              title={t('timelineDoc.addMilestone')}
              aria-label={t('timelineDoc.addMilestone')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Plus size={12} strokeWidth={2} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="timeline-doc-import"
              onClick={() => void onImport()}
              title={t('timelineDoc.importCsv')}
              aria-label={t('timelineDoc.importCsv')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Upload size={12} strokeWidth={2} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="timeline-doc-export-csv"
              onClick={() =>
                downloadDocFile(
                  timelineToCsv(doc),
                  `${title || 'timeline'}.csv`,
                  'text/csv',
                )
              }
              title={t('timelineDoc.exportCsv')}
              aria-label={t('timelineDoc.exportCsv')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Download size={12} strokeWidth={2} aria-hidden />
              {narrow ? null : <span>CSV</span>}
            </button>
            <button
              type="button"
              data-testid="timeline-doc-export-markdown"
              onClick={() =>
                downloadDocFile(
                  timelineToMarkdown(doc),
                  `${title || 'timeline'}.md`,
                  'text/markdown',
                )
              }
              title={t('timelineDoc.exportMarkdown')}
              aria-label={t('timelineDoc.exportMarkdown')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Download size={12} strokeWidth={2} aria-hidden />
              {narrow ? null : <span>MD</span>}
            </button>
            <button
              type="button"
              data-testid="timeline-doc-export-mermaid"
              onClick={() =>
                downloadDocFile(
                  timelineToMermaidGantt(doc),
                  `${title || 'timeline'}.mmd`,
                  'text/plain',
                )
              }
              title={t('timelineDoc.exportMermaid')}
              aria-label={t('timelineDoc.exportMermaid')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <GanttChartSquare size={12} strokeWidth={2} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="timeline-doc-export-json"
              onClick={() =>
                downloadDocFile(
                  serializeTimelineDoc(doc),
                  `${title || 'timeline'}.json`,
                  'application/json',
                )
              }
              title={t('timelineDoc.exportJson')}
              aria-label={t('timelineDoc.exportJson')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Braces size={12} strokeWidth={2} aria-hidden />
            </button>
          </>
        }
      />

      <div className="shell-scroll min-h-0 min-w-0 flex-1 overflow-auto bg-shell-bg">
        {ordered.length === 0 ? (
          <div
            data-testid="timeline-doc-empty"
            className="flex h-full flex-col items-center justify-center gap-2 px-6 text-center"
          >
            <p className="text-[12px] font-medium text-shell-text">
              {t('timelineDoc.emptyTitle')}
            </p>
            <p className="max-w-sm text-[11px] leading-relaxed text-shell-muted">
              {t('timelineDoc.emptyBody')}
            </p>
            <button
              type="button"
              data-testid="timeline-doc-empty-add"
              onClick={addMilestone}
              className="shell-primary-button mt-1 rounded-md px-3 py-1 text-[11px] font-medium"
            >
              {t('timelineDoc.addMilestone')}
            </button>
          </div>
        ) : (
          <div className="min-w-0 px-2 py-2">
            {view === 'timeline' && bounds ? (
              <div
                data-testid="timeline-doc-track"
                data-span-days={bounds.days}
                className="mb-3 rounded-lg border border-shell-border bg-shell-panel p-2"
              >
                <div className="mb-1.5 flex items-center justify-between text-[9px] tabular-nums text-shell-muted">
                  <span>{bounds.min}</span>
                  <span>{t('timelineDoc.spanDays', { days: bounds.days })}</span>
                  <span>{bounds.max}</span>
                </div>
                <ul className="space-y-1">
                  {ordered.map((m) => {
                    const span = timelineSpan(m, bounds)
                    return (
                      <li
                        key={m.id}
                        data-testid={`timeline-doc-bar-${m.id}`}
                        data-status={m.status}
                        className="flex min-w-0 items-center gap-2"
                      >
                        <span
                          className="w-28 shrink-0 truncate text-[10px] text-shell-text"
                          title={m.title}
                        >
                          {m.title}
                        </span>
                        <span className="relative h-3 min-w-0 flex-1 overflow-hidden rounded bg-shell-bg">
                          {span ? (
                            <span
                              className={`absolute top-0 block h-3 rounded ${STATUS_BAR_CLASS[m.status]}`}
                              style={{
                                left: `${span.offset * 100}%`,
                                width: `${span.width * 100}%`,
                              }}
                              title={`${m.start}${m.end ? ` → ${m.end}` : ''}`}
                            />
                          ) : (
                            <span className="absolute inset-y-0 left-1 text-[9px] leading-3 text-shell-muted">
                              {t('timelineDoc.undated')}
                            </span>
                          )}
                        </span>
                        <span className="w-20 shrink-0 text-right text-[9px] tabular-nums text-shell-muted">
                          {m.start || '—'}
                        </span>
                      </li>
                    )
                  })}
                </ul>
              </div>
            ) : null}

            <ul data-testid="timeline-doc-list" className="space-y-1.5">
              {ordered.map((m) => (
                <li
                  key={m.id}
                  data-testid={`timeline-doc-item-${m.id}`}
                  data-status={m.status}
                  className="rounded-lg border border-shell-border bg-shell-panel p-2"
                >
                  <div className="flex min-w-0 items-start gap-2">
                    <span
                      className={`mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full border ${STATUS_DOT_CLASS[m.status]}`}
                      aria-hidden
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 items-center gap-1.5">
                        <input
                          data-testid={`timeline-doc-title-${m.id}`}
                          value={m.title}
                          onChange={(e) =>
                            patchMilestone(m.id, { title: e.target.value })
                          }
                          aria-label={t('timelineDoc.milestoneTitle')}
                          className="h-7 min-w-0 flex-1 rounded border border-transparent bg-transparent px-1 text-[12px] font-medium text-shell-text outline-none transition hover:border-shell-border focus:border-shell-accent focus:bg-shell-bg"
                        />
                        <button
                          type="button"
                          data-testid={`timeline-doc-remove-${m.id}`}
                          onClick={() =>
                            commit({
                              ...doc,
                              milestones: doc.milestones.filter(
                                (x) => x.id !== m.id,
                              ),
                            })
                          }
                          title={t('timelineDoc.removeMilestone')}
                          aria-label={t('timelineDoc.removeMilestone')}
                          className="shrink-0 rounded p-1 text-shell-muted transition hover:bg-shell-hover hover:text-red-400"
                        >
                          <Trash2 size={11} strokeWidth={2} aria-hidden />
                        </button>
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5">
                        <label className="flex items-center gap-1 text-[9px] uppercase tracking-wide text-shell-muted">
                          {t('timelineDoc.start')}
                          <input
                            type="date"
                            data-testid={`timeline-doc-start-${m.id}`}
                            value={m.start}
                            onChange={(e) =>
                              patchMilestone(m.id, { start: e.target.value })
                            }
                            className="h-6 rounded border border-shell-border bg-shell-bg px-1 text-[10px] text-shell-text outline-none focus:border-shell-accent"
                          />
                        </label>
                        <label className="flex items-center gap-1 text-[9px] uppercase tracking-wide text-shell-muted">
                          {t('timelineDoc.end')}
                          <input
                            type="date"
                            data-testid={`timeline-doc-end-${m.id}`}
                            value={m.end}
                            min={m.start || undefined}
                            onChange={(e) =>
                              patchMilestone(m.id, { end: e.target.value })
                            }
                            className="h-6 rounded border border-shell-border bg-shell-bg px-1 text-[10px] text-shell-text outline-none focus:border-shell-accent"
                          />
                        </label>
                        <label className="flex items-center gap-1 text-[9px] uppercase tracking-wide text-shell-muted">
                          {t('timelineDoc.status')}
                          <select
                            data-testid={`timeline-doc-status-${m.id}`}
                            value={m.status}
                            onChange={(e) =>
                              patchMilestone(m.id, {
                                status: e.target.value as TimelineStatus,
                              })
                            }
                            className="h-6 rounded border border-shell-border bg-shell-bg px-1 text-[10px] text-shell-text outline-none focus:border-shell-accent"
                          >
                            {TIMELINE_STATUSES.map((s) => (
                              <option key={s} value={s}>
                                {t(STATUS_LABEL_KEY[s])}
                              </option>
                            ))}
                          </select>
                        </label>
                        <input
                          data-testid={`timeline-doc-owner-${m.id}`}
                          value={m.owner}
                          onChange={(e) =>
                            patchMilestone(m.id, { owner: e.target.value })
                          }
                          placeholder={t('timelineDoc.owner')}
                          aria-label={t('timelineDoc.owner')}
                          className="h-6 w-28 min-w-0 rounded border border-shell-border bg-shell-bg px-1.5 text-[10px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
                        />
                      </div>
                      <input
                        data-testid={`timeline-doc-notes-${m.id}`}
                        value={m.notes}
                        onChange={(e) =>
                          patchMilestone(m.id, { notes: e.target.value })
                        }
                        placeholder={t('timelineDoc.notes')}
                        aria-label={t('timelineDoc.notes')}
                        className="mt-1 h-6 w-full rounded border border-transparent bg-transparent px-1 text-[10px] text-shell-muted outline-none transition hover:border-shell-border focus:border-shell-accent focus:bg-shell-bg focus:text-shell-text"
                      />
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      <DocAgentPanel
        kind="timeline"
        contextId={contextId}
        canvasKey={canvasKey}
        canvasEntrySessionId={canvasEntrySessionId}
        currentContent={content}
        onApply={onApplyFromAgent}
        placeholder={t('timelineDoc.agentPlaceholder')}
      />
    </div>
  )
}
