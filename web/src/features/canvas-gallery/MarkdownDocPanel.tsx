import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Code2,
  Download,
  Eye,
  ListTree,
  Upload,
} from 'lucide-react'
import { CodeEditor } from '../editor/CodeEditor'
import { MarkdownPreview } from '../editor/MarkdownPreview'
import { useLocale } from '../i18n/LocaleProvider'
import { DocAgentPanel } from './DocAgentPanel'
import { markdownOutline, markdownStats } from './markdownDoc'
import {
  DOC_PANEL_TOOL_BUTTON,
  DocPanelHeader,
  downloadDocFile,
  pickTextFile,
  useDocPanelNarrow,
} from './docPanelChrome'

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

type ViewMode = 'source' | 'preview'

export function MarkdownDocPanel({
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
  const [view, setView] = useState<ViewMode>('preview')
  const [outlineOpen, setOutlineOpen] = useState(true)
  const [importError, setImportError] = useState<string | null>(null)
  const { ref: panelRef, narrow } = useDocPanelNarrow()
  const contentRef = useRef(content)
  contentRef.current = content

  const outline = useMemo(() => markdownOutline(content), [content])
  const stats = useMemo(() => markdownStats(content), [content])

  useEffect(() => {
    setImportError(null)
  }, [canvasKey])

  const onImport = useCallback(async () => {
    setImportError(null)
    const picked = await pickTextFile('.md,.markdown,.txt,text/markdown,text/plain')
    if (!picked) return
    if (picked.error) {
      setImportError(picked.error)
      return
    }
    onSave(picked.text)
  }, [onSave])

  /**
   * Rendered headings appear in the same order as the source outline, so the
   * outline index addresses the matching preview node directly.
   */
  const jumpToHeading = useCallback((index: number) => {
    setView('preview')
    window.requestAnimationFrame(() => {
      const root = document.querySelector('[data-testid="markdown-doc-preview"]')
      const nodes = root?.querySelectorAll('h1, h2, h3, h4, h5, h6')
      nodes?.[index]?.scrollIntoView({ block: 'start', behavior: 'smooth' })
    })
  }, [])

  const showSource = view === 'source'

  return (
    <div
      ref={panelRef}
      data-testid="markdown-doc-panel"
      data-narrow={narrow ? 'true' : 'false'}
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
    >
      <DocPanelHeader
        compact={narrow}
        title={title}
        scopeLabel={scopeLabel}
        dirty={dirty}
        saveFlash={saveFlash}
        onSave={() => onSave(contentRef.current)}
        statusText={t('markdownDoc.stats', {
          words: stats.words,
          headings: stats.headings,
          minutes: stats.readingMinutes,
        })}
        error={importError || error || null}
        onDismissError={() => {
          setImportError(null)
          onDismissError?.()
        }}
        testId="markdown-doc"
        tools={
          <>
            <button
              type="button"
              data-testid="markdown-doc-outline-toggle"
              data-active={outlineOpen ? 'true' : 'false'}
              onClick={() => setOutlineOpen((v) => !v)}
              title={t('markdownDoc.outline')}
              aria-label={t('markdownDoc.outline')}
              aria-pressed={outlineOpen}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <ListTree size={12} strokeWidth={2} aria-hidden />
            </button>
            <div
              className="inline-flex h-6 overflow-hidden rounded-md border border-shell-border bg-shell-bg"
              role="group"
              aria-label={t('markdownDoc.viewMode')}
            >
              {(
                [
                  ['preview', t('editor.preview'), Eye],
                  ['source', t('editor.source'), Code2],
                ] as Array<[ViewMode, string, typeof Eye]>
              ).map(([id, label, Icon], i) => {
                const active = view === id
                return (
                  <button
                    key={id}
                    type="button"
                    data-testid={`markdown-doc-view-${id}`}
                    data-active={active ? 'true' : 'false'}
                    aria-pressed={active}
                    title={label}
                    onClick={() => setView(id)}
                    className={`inline-flex h-6 items-center gap-1 px-1.5 text-[10px] transition ${
                      active
                        ? 'bg-shell-active text-shell-accent'
                        : 'text-shell-muted hover:text-shell-text'
                    } ${i < 1 ? 'border-r border-shell-border' : ''}`}
                  >
                    <Icon size={11} strokeWidth={2} aria-hidden />
                    {narrow ? null : <span>{label}</span>}
                  </button>
                )
              })}
            </div>
            <button
              type="button"
              data-testid="markdown-doc-import"
              onClick={() => void onImport()}
              title={t('docPanel.import')}
              aria-label={t('docPanel.import')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Upload size={12} strokeWidth={2} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="markdown-doc-export"
              onClick={() =>
                downloadDocFile(
                  contentRef.current,
                  `${title || 'document'}.md`,
                  'text/markdown',
                )
              }
              title={t('docPanel.exportMarkdown')}
              aria-label={t('docPanel.exportMarkdown')}
              className={DOC_PANEL_TOOL_BUTTON}
            >
              <Download size={12} strokeWidth={2} aria-hidden />
            </button>
          </>
        }
      />

      <div
        data-testid="markdown-doc-body"
        className="flex min-h-0 min-w-0 flex-1 overflow-hidden"
      >
        {outlineOpen && outline.length > 0 && !narrow ? (
          <nav
            data-testid="markdown-doc-outline"
            aria-label={t('markdownDoc.outline')}
            className="explorer-scroll w-44 shrink-0 overflow-auto border-r border-shell-border bg-shell-panel px-1.5 py-2"
          >
            <ul className="space-y-0.5">
              {outline.map((h, index) => (
                <li key={h.id}>
                  <button
                    type="button"
                    data-testid={`markdown-doc-outline-${h.id}`}
                    onClick={() => jumpToHeading(index)}
                    title={h.text}
                    className="block w-full truncate rounded px-1.5 py-1 text-left text-[11px] text-shell-muted transition hover:bg-shell-hover hover:text-shell-text"
                    style={{ paddingLeft: `${0.375 + (h.level - 1) * 0.4}rem` }}
                  >
                    {h.text}
                  </button>
                </li>
              ))}
            </ul>
          </nav>
        ) : null}

        <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
          {showSource ? (
            <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-[#1e1e1e]">
              <CodeEditor
                path={`gallery://${canvasKey}.md`}
                value={content}
                dirty={dirty}
                onChange={onContentChange}
                onSave={onSave}
                testId="markdown-doc-editor"
                fontSize={12}
              />
            </div>
          ) : (
            <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg">
              <MarkdownPreview
                content={content}
                path={`gallery://${canvasKey}.md`}
                testId="markdown-doc-preview"
                allowMermaidDocument={false}
              />
            </div>
          )}
        </div>
      </div>

      <DocAgentPanel
        kind="markdown"
        contextId={contextId}
        canvasKey={canvasKey}
        canvasEntrySessionId={canvasEntrySessionId}
        currentContent={content}
        onApply={onApplyFromAgent}
        placeholder={t('markdownDoc.agentPlaceholder')}
      />
    </div>
  )
}
