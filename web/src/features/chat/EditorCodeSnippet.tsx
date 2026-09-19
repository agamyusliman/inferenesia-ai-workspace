type Props = {
  text: string
  language?: string
  maxHeightClass?: string
  testId?: string
  className?: string
}

const DEFAULT_MAX_H = 'max-h-96'

export function EditorCodeSnippet({
  text,
  language = 'html',
  maxHeightClass = DEFAULT_MAX_H,
  testId = 'editor-code-snippet',
  className = '',
}: Props) {
  const lines = (text || '').replace(/\r\n/g, '\n').split('\n')
  const gutterCh = Math.max(2, String(lines.length).length)

  return (
    <div
      data-testid={testId}
      data-language={language}
      className={
        'flex min-h-0 min-w-0 flex-col overflow-hidden font-mono text-[11px] ' +
        className
      }
      style={{
        background: '#1e1e1e',
        color: '#d4d4d4',
        lineHeight: 1.55,
      }}
    >
      <div
        className="flex shrink-0 items-center gap-2 border-b px-2 py-1"
        style={{ background: '#252526', borderColor: 'rgba(255,255,255,0.06)' }}
      >
        <span className="flex gap-1" aria-hidden>
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
          className="truncate text-[10px] uppercase tracking-wide"
          style={{ color: '#858585' }}
        >
          {language || 'text'}
        </span>
        <span
          className="ml-auto text-[10px] tabular-nums"
          style={{ color: '#6a6a6a' }}
        >
          {lines.length} lines
        </span>
      </div>
      <div className={'min-h-0 flex-1 overflow-auto ' + maxHeightClass}>
        <table className="w-full border-collapse">
          <tbody>
            {lines.map((line, i) => (
              <tr key={i}>
                <td
                  className="select-none whitespace-nowrap border-r px-2 py-0 text-right tabular-nums"
                  style={{
                    minWidth: `calc(${gutterCh}ch + 1rem)`,
                    color: '#858585',
                    borderColor: 'rgba(255,255,255,0.06)',
                    background: '#1e1e1e',
                  }}
                >
                  {i + 1}
                </td>
                <td
                  className="whitespace-pre-wrap break-all px-2.5 py-0"
                  style={{ color: '#d4d4d4' }}
                >
                  {line.length ? line : ' '}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
