import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import { Code, Eye, X } from 'lucide-react'
import { ExcalidrawCanvas } from '../canvas/ExcalidrawCanvas'
import { CodeEditor } from '../editor/CodeEditor'
import { DiffCodeEditor } from '../editor/DiffCodeEditor'
import { ImagePreview } from '../editor/ImagePreview'
import { MarkdownPreview } from '../editor/MarkdownPreview'
import {
  isRenderablePreviewPath,
  supportsExcalidrawCanvas,
  supportsMarkdownPreview,
  type EditorViewMode,
} from '../editor/editorMode'

function isImagePath(path: string): boolean {
  const base = (path.split(/[/\\]/).pop() || path).toLowerCase()
  return /\.(png|jpe?g|gif|webp|svg|bmp|ico)$/i.test(base)
}

export type DiffTabMeta = {
  original: string
  modified: string
  labelOriginal: string
  labelModified: string
  title: string
  entryId: string
  source: string
  path: string
}

export type EditorColumn = {
  id: string
  path: string
  content: string
  /**
   * Content last loaded from disk or successfully saved.
   * Dirty when content !== lastSavedContent (VAL-IDE-016/017).
   * Optional for secondary split; primary tabs always set this.
   */
  lastSavedContent?: string
  /** Preferred view mode when opened via Open With / Open Preview. */
  preferredMode?: EditorViewMode
  /** Optional Monaco selection restore across tab switches (VAL-IDE-018). */
  viewState?: unknown
  kind?: 'file' | 'diff'
  diff?: DiffTabMeta
}

type Props = {
  primary: EditorColumn | null
  secondary: EditorColumn | null
  onPrimaryChange: (content: string) => void
  onSecondaryChange: (content: string) => void
  onCloseSecondary: () => void
  onFocusColumn?: (which: 'primary' | 'secondary') => void
  /** Persist Monaco view state per primary tab (VAL-IDE-018). */
  onPrimaryViewState?: (state: unknown) => void
  onSecondaryViewState?: (state: unknown) => void
  /** Primary column view mode (controlled by EditorTabs when provided). */
  primaryMode?: EditorViewMode
  onPrimaryModeChange?: (mode: EditorViewMode) => void
}

function baseName(path: string) {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

/**
 * Split editor columns: independently focusable and editable (VAL-IDE-028).
 * Primary chrome (filename / MD modes) lives in EditorTabs — no duplicate header.
 * Secondary keeps a compact strip: name + MD icons + close.
 */
export function SplitEditorArea({
  primary,
  secondary,
  onPrimaryChange,
  onSecondaryChange,
  onCloseSecondary,
  onFocusColumn,
  onPrimaryViewState,
  onSecondaryViewState,
  primaryMode: primaryModeProp,
  onPrimaryModeChange: _onPrimaryModeChange,
}: Props) {
  const [leftWidth, setLeftWidth] = useState(0)
  const [focused, setFocused] = useState<'primary' | 'secondary'>('primary')
  const [primaryModeLocal, setPrimaryModeLocal] = useState<EditorViewMode>('edit')
  const [secondaryMode, setSecondaryMode] = useState<EditorViewMode>('edit')
  const containerRef = useRef<HTMLDivElement | null>(null)
  const dragRef = useRef<{ startX: number; startWidth: number } | null>(null)

  const primaryMode = primaryModeProp ?? primaryModeLocal

  useEffect(() => {
    if (!secondary) setFocused('primary')
  }, [secondary])

  useEffect(() => {
    // Local fallback only when parent is not controlling primaryMode.
    if (primaryModeProp === undefined) setPrimaryModeLocal('edit')
  }, [primary?.path, primaryModeProp])

  useEffect(() => {
    if (secondary && supportsExcalidrawCanvas(secondary.path)) {
      setSecondaryMode(secondary.preferredMode || 'preview')
    } else {
      setSecondaryMode(secondary?.preferredMode || 'edit')
    }
  }, [secondary?.path, secondary?.preferredMode])

  useEffect(() => {
    if (secondary && leftWidth <= 0 && containerRef.current) {
      const w = containerRef.current.getBoundingClientRect().width
      if (w > 0) setLeftWidth(Math.floor(w / 2))
    }
  }, [secondary, leftWidth])

  const focusPrimary = useCallback(() => {
    setFocused('primary')
    onFocusColumn?.('primary')
  }, [onFocusColumn])

  const focusSecondary = useCallback(() => {
    setFocused('secondary')
    onFocusColumn?.('secondary')
  }, [onFocusColumn])

  const onSplitDocMove = useCallback((e: PointerEvent) => {
    if (!dragRef.current || !containerRef.current) return
    e.preventDefault()
    const total = containerRef.current.getBoundingClientRect().width
    if (total <= 0) return
    const delta = e.clientX - dragRef.current.startX
    const next = dragRef.current.startWidth + delta
    const min = Math.max(120, total * 0.2)
    const max = Math.min(total - 120, total * 0.8)
    setLeftWidth(Math.min(max, Math.max(min, next)))
  }, [])

  const onSplitDocUp = useCallback(() => {
    if (!dragRef.current) return
    dragRef.current = null
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    document.removeEventListener('pointermove', onSplitDocMove)
    document.removeEventListener('pointerup', onSplitDocUp)
    document.removeEventListener('pointercancel', onSplitDocUp)
  }, [onSplitDocMove])

  useEffect(() => {
    return () => {
      dragRef.current = null
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      document.removeEventListener('pointermove', onSplitDocMove)
      document.removeEventListener('pointerup', onSplitDocUp)
      document.removeEventListener('pointercancel', onSplitDocUp)
    }
  }, [onSplitDocMove, onSplitDocUp])

  const onSplitPointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      e.preventDefault()
      e.stopPropagation()
      const current =
        leftWidth > 0
          ? leftWidth
          : containerRef.current
            ? Math.floor(containerRef.current.getBoundingClientRect().width / 2)
            : 320
      dragRef.current = { startX: e.clientX, startWidth: current }
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
      document.addEventListener('pointermove', onSplitDocMove)
      document.addEventListener('pointerup', onSplitDocUp)
      document.addEventListener('pointercancel', onSplitDocUp)
    },
    [leftWidth, onSplitDocMove, onSplitDocUp],
  )

  if (!primary && !secondary) {
    return null
  }

  const renderBody = (
    col: EditorColumn,
    mode: EditorViewMode,
    onChange: (v: string) => void,
    which: 'primary' | 'secondary',
    onFocusCol: () => void,
    onViewState?: (state: unknown) => void,
  ) => {
    if (col.kind === 'diff' && col.diff) {
      return (
        <DiffCodeEditor
          testId={`editor-column-diff-${which}`}
          path={col.diff.path}
          original={col.diff.original}
          modified={col.diff.modified}
          labelOriginal={col.diff.labelOriginal}
          labelModified={col.diff.labelModified}
          title={col.diff.title}
          onFocus={onFocusCol}
          focused={focused === which}
        />
      )
    }
    const allowPreview =
      supportsMarkdownPreview(col.path) || isRenderablePreviewPath(col.path)
    const effective: EditorViewMode =
      allowPreview && mode === 'preview' ? 'preview' : 'edit'
    const dirty =
      col.lastSavedContent !== undefined && col.content !== col.lastSavedContent
    if (effective === 'preview') {
      if (supportsExcalidrawCanvas(col.path)) {
        return (
          <ExcalidrawCanvas
            path={col.path}
            content={col.content}
            onChange={onChange}
            testId={`excalidraw-canvas-${which}`}
          />
        )
      }
      if (isImagePath(col.path)) {
        return (
          <ImagePreview
            path={col.path}
            testId={`image-preview-${which}`}
          />
        )
      }
      return (
        <MarkdownPreview
          content={col.content}
          path={col.path}
          testId={`markdown-preview-${which}`}
        />
      )
    }
    return (
      <CodeEditor
        testId={`editor-column-${which}`}
        path={col.path}
        value={col.content}
        onChange={onChange}
        onFocus={onFocusCol}
        focused={focused === which}
        dirty={dirty}
        viewState={col.viewState as never}
        onViewStateChange={onViewState}
      />
    )
  }

  if (primary && !secondary) {
    return (
      <div data-testid="editor-area" className="flex h-full min-h-0 flex-col">
        {renderBody(
          primary,
          primaryMode,
          onPrimaryChange,
          'primary',
          focusPrimary,
          onPrimaryViewState,
        )}
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      data-testid="editor-area"
      className="flex h-full min-h-0 flex-row"
    >
      <div
        data-testid="editor-pane-primary"
        className="flex min-h-0 min-w-0 flex-col border-r border-shell-border"
        style={{
          width: leftWidth > 0 ? leftWidth : '50%',
          flex: 'none',
        }}
      >
        {renderBody(
          primary!,
          primaryMode,
          onPrimaryChange,
          'primary',
          focusPrimary,
          onPrimaryViewState,
        )}
      </div>

      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize editor columns"
        data-testid="splitter-editor-columns"
        onPointerDown={onSplitPointerDown}
        className="relative z-10 w-1 shrink-0 cursor-col-resize touch-none bg-shell-border hover:bg-shell-accent"
      >
        <div className="absolute inset-y-0 -left-1.5 -right-1.5" />
      </div>

      <div
        data-testid="editor-pane-secondary"
        className="flex min-h-0 min-w-0 flex-1 flex-col"
      >
        <SecondaryStrip
          path={secondary!.path}
          focused={focused === 'secondary'}
          mode={secondaryMode}
          onModeChange={setSecondaryMode}
          onClose={onCloseSecondary}
        />
        {renderBody(
          secondary!,
          secondaryMode,
          onSecondaryChange,
          'secondary',
          focusSecondary,
          onSecondaryViewState,
        )}
      </div>
    </div>
  )
}

/** Compact chrome for the open-to-side column only (tabs cover the primary). */
function SecondaryStrip({
  path,
  focused,
  mode,
  onModeChange,
  onClose,
}: {
  path: string
  focused: boolean
  mode: EditorViewMode
  onModeChange: (m: EditorViewMode) => void
  onClose: () => void
}) {
  const allowPreview =
    supportsMarkdownPreview(path) || isRenderablePreviewPath(path)
  const isCanvas = supportsExcalidrawCanvas(path)
  const effective: EditorViewMode =
    allowPreview && mode === 'preview' ? 'preview' : 'edit'
  const sourceTitle = isCanvas ? 'Source' : 'Edit'
  const canvasTitle = isCanvas ? 'Playground' : 'Preview'

  return (
    <div
      data-testid="editor-secondary-strip"
      className={`flex h-10 shrink-0 items-center gap-0.5 border-b border-shell-border px-1.5 text-[11px] ${
        focused ? 'bg-shell-bg text-shell-text' : 'bg-shell-panel/80 text-shell-muted'
      }`}
    >
      <span className="min-w-0 flex-1 truncate px-0.5" title={path}>
        {baseName(path)}
      </span>
      {allowPreview && (
        <span
          data-testid="editor-md-toolbar-secondary"
          className="flex shrink-0 items-center"
          role="toolbar"
          aria-label={isCanvas ? 'Excalidraw view mode' : 'Markdown view mode'}
        >
          <button
            type="button"
            data-testid="editor-mode-edit-secondary"
            data-active={effective === 'edit' ? 'true' : 'false'}
            title={sourceTitle}
            aria-label={sourceTitle}
            aria-pressed={effective === 'edit'}
            onClick={() => onModeChange('edit')}
            className={`inline-flex rounded p-0.5 ${
              effective === 'edit'
                ? 'bg-shell-active text-shell-text'
                : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
            }`}
          >
            <Code size={12} aria-hidden />
          </button>
          <button
            type="button"
            data-testid="editor-mode-preview-secondary"
            data-active={effective === 'preview' ? 'true' : 'false'}
            title={canvasTitle}
            aria-label={canvasTitle}
            aria-pressed={effective === 'preview'}
            onClick={() => onModeChange('preview')}
            className={`inline-flex rounded p-0.5 ${
              effective === 'preview'
                ? 'bg-shell-active text-shell-text'
                : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
            }`}
          >
            <Eye size={12} aria-hidden />
          </button>
        </span>
      )}
      <button
        type="button"
        data-testid="editor-close-secondary"
        aria-label="Close secondary editor"
        onClick={onClose}
        className="inline-flex items-center rounded p-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
      >
        <X size={12} aria-hidden />
      </button>
    </div>
  )
}
