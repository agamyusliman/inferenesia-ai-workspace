import { Suspense, lazy, useCallback, useEffect, useRef } from 'react'
import type { DiffOnMount } from '@monaco-editor/react'
import type { editor as MonacoEditor } from 'monaco-editor'
import { useTheme } from '../theme/ThemeProvider'
import { monacoThemeFor } from '../theme/theme'
import { languageFromPath } from './languageFromPath'

const MonacoDiffReact = lazy(async () => {
  await import('./monacoSetup')
  const mod = await import('@monaco-editor/react')
  return { default: mod.DiffEditor }
})

type Props = {
  path: string
  original: string
  modified: string
  labelOriginal?: string
  labelModified?: string
  title?: string
  onFocus?: () => void
  focused?: boolean
  testId: string
}

function EditorLoading() {
  return (
    <div className="flex h-full items-center justify-center text-xs text-shell-muted">
      Loading diff…
    </div>
  )
}

export function DiffCodeEditor({
  path,
  original,
  modified,
  labelOriginal = 'before',
  labelModified = 'after',
  title,
  onFocus,
  focused,
  testId,
}: Props) {
  const { theme } = useTheme()
  const monacoTheme = monacoThemeFor(theme)
  const editorRef = useRef<MonacoEditor.IStandaloneDiffEditor | null>(null)
  const language = languageFromPath(path)

  const handleMount: DiffOnMount = useCallback(
    (ed) => {
      editorRef.current = ed
      const mod = ed.getModifiedEditor()
      mod.onDidFocusEditorText(() => onFocus?.())
      mod.onDidFocusEditorWidget(() => onFocus?.())
      const orig = ed.getOriginalEditor()
      orig.onDidFocusEditorText(() => onFocus?.())
    },
    [onFocus],
  )

  useEffect(() => {
    if (focused) {
      editorRef.current?.getModifiedEditor().focus()
    }
  }, [focused, path])

  return (
    <div
      data-testid={testId}
      data-path={path}
      data-kind="diff"
      className="flex min-h-0 min-w-0 flex-1 flex-col"
    >
      <div className="flex h-7 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2 text-[11px] text-shell-muted">
        <span className="min-w-0 flex-1 truncate font-medium text-shell-text">
          {title || path}
        </span>
        <span className="shrink-0 font-mono text-[10px]">
          <span className="text-red-300/90">{labelOriginal}</span>
          <span className="mx-1 opacity-50">↔</span>
          <span className="text-emerald-300/90">{labelModified}</span>
        </span>
      </div>
      <div className="min-h-0 min-w-0 flex-1">
        <Suspense fallback={<EditorLoading />}>
          <MonacoDiffReact
            original={original}
            modified={modified}
            language={language}
            theme={monacoTheme}
            onMount={handleMount}
            loading={<EditorLoading />}
            options={{
              readOnly: true,
              renderSideBySide: true,
              fontSize: 13,
              fontFamily:
                'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
              lineHeight: 19,
              minimap: { enabled: false },
              scrollBeyondLastLine: false,
              automaticLayout: true,
              renderIndicators: true,
              originalEditable: false,
              ignoreTrimWhitespace: false,
              scrollbar: {
                verticalScrollbarSize: 10,
                horizontalScrollbarSize: 10,
              },
            }}
            height="100%"
            width="100%"
          />
        </Suspense>
      </div>
    </div>
  )
}
