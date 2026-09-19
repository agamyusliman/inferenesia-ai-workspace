import { useEffect, useRef, useState } from 'react'
import { Check, Copy, Save } from 'lucide-react'
import { formatShortcut } from '../../lib/platform'

type Props = {
  value: string
  onChange: (value: string) => void
  onSave?: () => void
  testId?: string
  readOnly?: boolean
  className?: string
  label?: string
  showToolbar?: boolean
}

const BG = '#1e1e1e'
const FG = '#d4d4d4'
const GUTTER_FG = '#858585'
const GUTTER_BG = '#1e1e1e'
const HEADER_BG = '#252526'
const BORDER = 'rgba(255,255,255,0.08)'
const CARET = '#aeafad'
const SEL_BG = 'rgba(38,79,120,0.55)'

export function HtmlCodeEditor({
  value,
  onChange,
  onSave,
  testId = 'html-code-editor',
  readOnly = false,
  className = '',
  label = 'HTML',
  showToolbar = true,
}: Props) {
  const taRef = useRef<HTMLTextAreaElement | null>(null)
  const gutterRef = useRef<HTMLDivElement | null>(null)
  const [copied, setCopied] = useState(false)

  const lines = Math.max(1, (value || '').split('\n').length)
  const gutter = Array.from({ length: lines }, (_, i) => i + 1).join('\n')

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1200)
    } catch {
      void 0
    }
  }

  const toolBtnStyle = {
    color: FG,
    border: `1px solid ${BORDER}`,
    background: 'rgba(255,255,255,0.04)',
  } as const

  useEffect(() => {
    const ta = taRef.current
    const g = gutterRef.current
    if (!ta || !g) return
    const sync = () => {
      g.scrollTop = ta.scrollTop
    }
    ta.addEventListener('scroll', sync)
    return () => ta.removeEventListener('scroll', sync)
  }, [])

  return (
    <div
      data-testid={testId}
      className={'flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden font-mono text-[12px] ' + className}
      style={{ background: BG, color: FG, lineHeight: 1.55 }}
    >
      {showToolbar && (
        <div
          data-testid={`${testId}-toolbar`}
          className="flex shrink-0 items-center gap-2 px-2 py-1"
          style={{
            background: HEADER_BG,
            borderBottom: `1px solid ${BORDER}`,
            color: GUTTER_FG,
          }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <span className="flex items-center gap-1" aria-hidden>
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{ background: '#ff5f57' }}
            />
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{ background: '#febc2e' }}
            />
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{ background: '#28c840' }}
            />
          </span>
          <span
            className="truncate text-[10px] font-medium uppercase tracking-wide"
            style={{ color: GUTTER_FG }}
          >
            {label}
          </span>
          <span className="ml-auto flex items-center gap-1.5">
            <span
              className="mr-1 tabular-nums text-[10px]"
              style={{ color: GUTTER_FG }}
            >
              {lines} lines
            </span>
            {!readOnly && onSave && (
              <button
                type="button"
                data-testid={`${testId}-save`}
                title={`Save preview (${formatShortcut('S')})`}
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  onSave()
                }}
                className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] transition"
                style={toolBtnStyle}
              >
                <Save size={10} />
                Save
              </button>
            )}
            <button
              type="button"
              data-testid={`${testId}-copy`}
              title="Copy"
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                void onCopy()
              }}
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] transition"
              style={toolBtnStyle}
            >
              {copied ? <Check size={10} style={{ color: '#4ade80' }} /> : <Copy size={10} />}
              {copied ? 'Copied' : 'Copy'}
            </button>
          </span>
        </div>
      )}

      <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
        <div
          ref={gutterRef}
          aria-hidden
          className="select-none overflow-hidden px-2 py-2 text-right tabular-nums"
          style={{
            minWidth: '2.75rem',
            background: GUTTER_BG,
            color: GUTTER_FG,
            borderRight: `1px solid ${BORDER}`,
          }}
        >
          <pre className="m-0 whitespace-pre" style={{ color: GUTTER_FG, margin: 0 }}>
            {gutter}
          </pre>
        </div>
        <textarea
          ref={taRef}
          value={value}
          readOnly={readOnly}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
              e.preventDefault()
              onSave?.()
            }
            e.stopPropagation()
          }}
          onMouseDown={(e) => e.stopPropagation()}
          className="min-h-0 min-w-0 flex-1 resize-none border-0 p-2 outline-none"
          style={{
            tabSize: 2,
            background: BG,
            color: FG,
            caretColor: CARET,
            WebkitTextFillColor: FG,
            lineHeight: 1.55,
            opacity: 1,
          }}
        />
      </div>
      <style>{`
        [data-testid="${testId}"] textarea {
          color: ${FG} !important;
          -webkit-text-fill-color: ${FG} !important;
          background: ${BG} !important;
          caret-color: ${CARET} !important;
        }
        [data-testid="${testId}"] textarea::selection {
          background: ${SEL_BG};
          color: ${FG};
        }
      `}</style>
    </div>
  )
}
