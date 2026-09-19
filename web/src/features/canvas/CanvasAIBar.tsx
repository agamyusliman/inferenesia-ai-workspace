import { useCallback, useEffect, useRef, useState } from 'react'
import { canvasAI, type CanvasAIMode, type CanvasAIResult } from '../../lib/api'
import {
  elementsJSONForRequest,
  mergeCanvasAIResult,
  placeholderForMode,
  submitLabel,
} from './canvasAI'
import type { ExcalidrawDocumentData } from './excalidrawDoc'

export type CanvasAIBarProps = {
  path: string
  elements: readonly unknown[]
  document: ExcalidrawDocumentData
  onApplied: (serialized: string) => void
  testId?: string
}

type State =
  | { kind: 'idle' }
  | { kind: 'busy' }
  | { kind: 'error'; message: string }

export function CanvasAIBar({
  path,
  elements,
  document,
  onApplied,
  testId = 'canvas-ai-bar',
}: CanvasAIBarProps) {
  const [prompt, setPrompt] = useState('')
  const [state, setState] = useState<State>({ kind: 'idle' })
  const inputRef = useRef<HTMLInputElement | null>(null)
  const lastPathRef = useRef<string>(path)

  useEffect(() => {
    if (lastPathRef.current !== path) {
      lastPathRef.current = path
      setPrompt('')
      setState({ kind: 'idle' })
    }
  }, [path])

  const elementCount = Array.isArray(elements) ? elements.length : 0
  const mode: CanvasAIMode = elementCount > 0 ? 'edit' : 'generate'
  const placeholder = placeholderForMode(mode, elementCount)
  const label = submitLabel(elementCount)
  const busy = state.kind === 'busy'

  const submit = useCallback(async () => {
    const trimmed = prompt.trim()
    if (!trimmed || busy) return
    setState({ kind: 'busy' })
    const reqElements = mode === 'edit' ? elementsJSONForRequest(elements) : ''
    let result: CanvasAIResult
    try {
      result = await canvasAI({
        prompt: trimmed,
        mode,
        existing_elements_json: reqElements || undefined,
      })
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      setState({ kind: 'error', message: msg })
      return
    }
    if (!result.ok) {
      setState({
        kind: 'error',
        message: result.error || 'canvas AI: unknown error',
      })
      return
    }
    const merged = mergeCanvasAIResult(result, document)
    if ('error' in merged) {
      setState({ kind: 'error', message: merged.error })
      return
    }
    onApplied(merged.serialized)
    setPrompt('')
    setState({ kind: 'idle' })
  }, [prompt, busy, mode, elements, document, onApplied])

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        void submit()
      }
    },
    [submit],
  )

  return (
    <div
      data-testid={testId}
      className="flex shrink-0 flex-col gap-1 border-t border-shell-border bg-shell-panel px-2 py-1.5"
    >
      <div className="flex items-center gap-1.5">
        <input
          ref={inputRef}
          data-testid="canvas-ai-input"
          type="text"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={onKeyDown}
          disabled={busy}
          placeholder={placeholder}
          className="min-w-0 flex-1 rounded border border-shell-border bg-shell-bg px-2 py-1 text-[12px] text-shell-text placeholder:text-shell-muted focus:outline-none focus:ring-1 focus:ring-shell-border disabled:opacity-50"
        />
        <button
          data-testid="canvas-ai-submit"
          type="button"
          onClick={() => void submit()}
          disabled={busy || !prompt.trim()}
          className="rounded border border-shell-border bg-shell-bg px-3 py-1 text-[12px] font-medium text-shell-text hover:bg-shell-border disabled:cursor-not-allowed disabled:opacity-50"
        >
          {busy ? '…' : label}
        </button>
      </div>
      {state.kind === 'error' && (
        <div
          data-testid={`${testId}-error`}
          className="truncate text-[10px] text-red-400"
          title={state.message}
        >
          {state.message}
        </div>
      )}
    </div>
  )
}
