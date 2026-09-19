import { ChevronDown, ChevronUp, Code, Eye, X } from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import type { EditorColumn } from '../shell/SplitEditorArea'
import {
  isRenderablePreviewPath,
  supportsExcalidrawCanvas,
  supportsMarkdownPreview,
  type EditorViewMode,
} from './editorMode'

type Props = {
  tabs: EditorColumn[]
  activeId: string | null
  onActivate: (id: string) => void
  onClose: (id: string) => void
  onSave?: () => void
  viewMode?: EditorViewMode
  onViewModeChange?: (mode: EditorViewMode) => void
  onPrevChange?: () => void
  onNextChange?: () => void
  canPrevChange?: boolean
  canNextChange?: boolean
}

function baseName(path: string) {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

function tabDirty(tab: EditorColumn): boolean {
  if (tab.lastSavedContent === undefined) return false
  return tab.content !== tab.lastSavedContent
}

/**
 * Multi-tab strip: file tabs on the left, active-tab MD Edit/Preview icons on the right.
 * Dirty tabs show a ● indicator (VAL-IDE-016).
 */
export function EditorTabs({
  tabs,
  activeId,
  onActivate,
  onClose,
  onSave,
  viewMode = 'edit',
  onViewModeChange,
  onPrevChange,
  onNextChange,
  canPrevChange = false,
  canNextChange = false,
}: Props) {
  const { t } = useLocale()
  if (tabs.length === 0) return null

  const activeTab = tabs.find((tab) => tab.id === activeId) || tabs[0]
  const isDiffActive = activeTab?.kind === 'diff'
  const activeDirty =
    !!activeTab &&
    activeTab.kind !== 'diff' &&
    activeTab.lastSavedContent !== undefined &&
    activeTab.content !== activeTab.lastSavedContent
  const showMdToolbar =
    !!activeTab &&
    !isDiffActive &&
    (supportsMarkdownPreview(activeTab.path) || isRenderablePreviewPath(activeTab.path)) &&
    !!onViewModeChange
  const isCanvasTab = !!activeTab && supportsExcalidrawCanvas(activeTab.path)
  const showDiffNav = isDiffActive && (!!onPrevChange || !!onNextChange)
  const effective: EditorViewMode =
    showMdToolbar && viewMode === 'preview' ? 'preview' : 'edit'
  const sourceTitle = isCanvasTab ? t('action.source') : t('editor.edit')
  const canvasTitle = isCanvasTab ? t('action.canvas') : t('editor.preview')
  const toolbarLabel = isCanvasTab
    ? 'Excalidraw view mode'
    : showDiffNav
      ? 'Timeline change navigation'
      : 'Markdown view mode'

  return (
    <div
      data-testid="editor-tabs"
      className="flex h-10 shrink-0 items-stretch justify-between gap-1 border-b border-shell-border bg-shell-panel/80"
      role="tablist"
      aria-label={t('editor.openEditors')}
    >
      <div className="flex min-w-0 flex-1 items-stretch overflow-x-auto">
        {tabs.map((tab) => {
          const active = tab.id === activeId
          const dirty = tabDirty(tab)
          const isDiff = tab.kind === 'diff'
          const label = isDiff
            ? `${baseName(tab.diff?.path || tab.path)} ↔`
            : baseName(tab.path)
          return (
            <div
              key={tab.id}
              role="tab"
              aria-selected={active}
              data-testid={`editor-tab-${tab.path}`}
              data-active={active ? 'true' : 'false'}
              data-dirty={dirty ? 'true' : 'false'}
              data-kind={isDiff ? 'diff' : 'file'}
              data-path={tab.path}
              className={`group flex max-w-[11rem] min-w-[5rem] shrink-0 items-center gap-0.5 border-r border-shell-border px-2 text-[11px] ${
                active
                  ? 'bg-shell-bg text-shell-text'
                  : 'bg-transparent text-shell-muted hover:bg-shell-border/20 hover:text-shell-text'
              }`}
            >
              {dirty && (
                <span
                  data-testid={`editor-tab-dirty-${tab.path}`}
                  className="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-shell-accent"
                  title={t('editor.unsaved')}
                  aria-label={t('editor.unsaved')}
                />
              )}
              <button
                type="button"
                className="min-w-0 flex-1 truncate text-left"
                title={dirty ? `${tab.path} (unsaved)` : tab.path}
                onClick={() => onActivate(tab.id)}
              >
                {label}
              </button>
              <button
                type="button"
                data-testid={`editor-tab-close-${tab.path}`}
                aria-label={`${t('action.close')} ${baseName(tab.path)}`}
                title={t('action.close')}
                onClick={(e) => {
                  e.stopPropagation()
                  onClose(tab.id)
                }}
                className="inline-flex shrink-0 rounded p-0.5 opacity-60 hover:bg-shell-border/40 hover:opacity-100 group-hover:opacity-100"
              >
                <X size={12} aria-hidden />
              </button>
            </div>
          )
        })}
      </div>

      <div
        className="flex shrink-0 items-center gap-0.5 border-l border-shell-border px-1.5"
        role="toolbar"
        aria-label={toolbarLabel}
      >
        {activeTab && !isDiffActive && onSave && (
          <button
            type="button"
            data-testid="editor-save-button"
            data-dirty={activeDirty ? 'true' : 'false'}
            title={t('editor.saveTitle')}
            aria-label={t('action.save')}
            disabled={!activeDirty}
            onClick={() => onSave()}
            className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] ${
              activeDirty
                ? 'text-shell-accent hover:bg-shell-border/40'
                : 'text-shell-muted/50'
            } disabled:cursor-default`}
          >
            {activeDirty && (
              <span
                data-testid="header-dirty-indicator"
                className="inline-block h-1.5 w-1.5 rounded-full bg-shell-accent"
                title={t('editor.unsaved')}
                aria-hidden
              />
            )}
            {t('action.save')}
          </button>
        )}
        {showDiffNav && (
          <>
            <button
              type="button"
              data-testid="editor-diff-prev"
              title={t('editor.olderChange')}
              aria-label={t('editor.olderChange')}
              disabled={!canPrevChange}
              onClick={() => onPrevChange?.()}
              className="inline-flex items-center justify-center rounded p-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-30"
            >
              <ChevronDown size={13} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="editor-diff-next"
              title={t('editor.newerChange')}
              aria-label={t('editor.newerChange')}
              disabled={!canNextChange}
              onClick={() => onNextChange?.()}
              className="inline-flex items-center justify-center rounded p-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-30"
            >
              <ChevronUp size={13} aria-hidden />
            </button>
          </>
        )}
        {showMdToolbar && (
          <>
            <button
              type="button"
              data-testid="editor-mode-edit"
              data-active={effective === 'edit' ? 'true' : 'false'}
              title={sourceTitle}
              aria-label={sourceTitle}
              aria-pressed={effective === 'edit'}
              onClick={() => onViewModeChange?.('edit')}
              className={`inline-flex items-center justify-center rounded p-1 ${
                effective === 'edit'
                  ? 'bg-shell-active text-shell-text'
                  : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
              }`}
            >
              <Code size={13} aria-hidden />
            </button>
            <button
              type="button"
              data-testid="editor-mode-preview"
              data-active={effective === 'preview' ? 'true' : 'false'}
              title={canvasTitle}
              aria-label={canvasTitle}
              aria-pressed={effective === 'preview'}
              onClick={() => onViewModeChange?.('preview')}
              className={`inline-flex items-center justify-center rounded p-1 ${
                effective === 'preview'
                  ? 'bg-shell-active text-shell-text'
                  : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
              }`}
            >
              <Eye size={13} aria-hidden />
            </button>
          </>
        )}
      </div>
    </div>
  )
}
