import { Suspense, lazy, useCallback, useEffect, useRef } from 'react'
import type { OnMount } from '@monaco-editor/react'
import type { editor as MonacoEditor } from 'monaco-editor'
import { formatShortcut, isMacOS, isSaveKeyEvent } from '../../lib/platform'
import { useTheme } from '../theme/ThemeProvider'
import { monacoThemeFor } from '../theme/theme'
import { languageFromPath } from './languageFromPath'

const MonacoEditorReact = lazy(async () => {
  await import('./monacoSetup')
  const mod = await import('@monaco-editor/react')
  return { default: mod.default }
})

type Props = {
  path: string
  value: string
  onChange: (value: string) => void
  onFocus?: () => void
  onSave?: (value: string) => void
  viewState?: MonacoEditor.ICodeEditorViewState | null
  onViewStateChange?: (state: MonacoEditor.ICodeEditorViewState | null) => void
  testId: string
  focused?: boolean
  dirty?: boolean
  readOnly?: boolean
  fontSize?: number
}

function EditorLoading() {
  return (
    <div className="flex h-full items-center justify-center text-xs text-shell-muted">
      Loading editor…
    </div>
  )
}

/**
 * Monaco-based code editor with VS Code-like syntax highlighting by file extension.
 * Parent owns buffer state; model path is keyed by file path for multi-tab friendliness.
 * Cursor/scroll view state is restored when remounting or switching tabs (VAL-IDE-018).
 */
export function CodeEditor({
  path,
  value,
  onChange,
  onFocus,
  onSave,
  viewState,
  onViewStateChange,
  testId,
  focused,
  dirty,
  readOnly = false,
  fontSize = 13,
}: Props) {
  const { theme } = useTheme()
  const monacoTheme = monacoThemeFor(theme)
  const editorRef = useRef<MonacoEditor.IStandaloneCodeEditor | null>(null)
  const viewStateRef = useRef(viewState)
  const onViewStateChangeRef = useRef(onViewStateChange)
  const onSaveRef = useRef(onSave)
  viewStateRef.current = viewState
  onViewStateChangeRef.current = onViewStateChange
  onSaveRef.current = onSave
  const language = languageFromPath(path)

  const handleMount: OnMount = useCallback(
    (ed, monaco) => {
      editorRef.current = ed
      ed.onDidFocusEditorText(() => onFocus?.())
      ed.onDidFocusEditorWidget(() => onFocus?.())
      const vs = viewStateRef.current
      if (vs) {
        try {
          ed.restoreViewState(vs)
        } catch {
          void 0
        }
      }
      ed.onDidChangeCursorPosition(() => {
        onViewStateChangeRef.current?.(ed.saveViewState())
      })
      ed.onDidScrollChange(() => {
        onViewStateChangeRef.current?.(ed.saveViewState())
      })
      ed.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
        const value = ed.getModel()?.getValue() ?? ed.getValue()
        onSaveRef.current?.(value)
      })
      ed.onKeyDown((e) => {
        const dom = e.browserEvent
        if (
          dom &&
          isSaveKeyEvent({
            key: dom.key,
            code: dom.code,
            metaKey: dom.metaKey,
            ctrlKey: dom.ctrlKey,
            altKey: dom.altKey,
            shiftKey: dom.shiftKey,
          })
        ) {
          e.preventDefault()
          e.stopPropagation()
          const value = ed.getModel()?.getValue() ?? ed.getValue()
          onSaveRef.current?.(value)
        }
      })
    },
    [onFocus],
  )

  useEffect(() => {
    const onWindowKey = (e: KeyboardEvent) => {
      if (!onSaveRef.current) return
      const ed = editorRef.current
      if (!ed) return
      if (!ed.hasTextFocus() && !ed.hasWidgetFocus()) return
      if (!isSaveKeyEvent(e)) return
      e.preventDefault()
      e.stopPropagation()
      const value = ed.getModel()?.getValue() ?? ed.getValue()
      onSaveRef.current(value)
    }
    window.addEventListener('keydown', onWindowKey, true)
    return () => window.removeEventListener('keydown', onWindowKey, true)
  }, [])

  useEffect(() => {
    if (focused) {
      editorRef.current?.focus()
    }
  }, [focused, path])

  // Save view state when unmounting or path changes so tab switch keeps cursor.
  useEffect(() => {
    return () => {
      const ed = editorRef.current
      if (ed) {
        onViewStateChangeRef.current?.(ed.saveViewState())
      }
    }
  }, [path])

  return (
    <div
      data-testid={testId}
      data-path={path}
      data-language={language}
      data-focused={focused ? 'true' : 'false'}
      data-dirty={dirty ? 'true' : 'false'}
      data-buffer-length={value.length}
      data-save-shortcut={formatShortcut('S')}
      data-mod={isMacOS() ? 'cmd' : 'ctrl'}
      className="relative z-0 min-h-0 min-w-0 flex-1"
      style={{ minHeight: 0, height: '100%' }}
      title={onSave ? `Save (${formatShortcut('S')})` : undefined}
    >
      <Suspense fallback={<EditorLoading />}>
        <MonacoEditorReact
          path={path}
          language={language}
          value={value}
          theme={monacoTheme}
          onChange={(v) => onChange(v ?? '')}
          onMount={handleMount}
          loading={<EditorLoading />}
          options={{
            fontSize,
            fontFamily:
              'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
            lineHeight: Math.round(fontSize * 1.45),
            minimap: { enabled: false },
            scrollBeyondLastLine: false,
            wordWrap: 'on',
            automaticLayout: true,
            tabSize: 2,
            readOnly,
            renderLineHighlight: readOnly ? 'none' : 'line',
            padding: { top: 8, bottom: 8 },
            glyphMargin: false,
            folding: true,
            bracketPairColorization: { enabled: true },
            smoothScrolling: true,
            cursorBlinking: 'smooth',
            renderWhitespace: 'selection',
            overviewRulerLanes: 0,
            hideCursorInOverviewRuler: true,
            fixedOverflowWidgets: true,
            scrollbar: {
              verticalScrollbarSize: 10,
              horizontalScrollbarSize: 10,
              alwaysConsumeMouseWheel: false,
            },
          }}
          height="100%"
          width="100%"
        />
      </Suspense>
    </div>
  )
}
