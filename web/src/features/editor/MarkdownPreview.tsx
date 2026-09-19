import { useMemo, type ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { MermaidDiagram } from '../diagrams/MermaidDiagram'
import { isMermaidFence } from '../chat/messageChrome'
import { normalizeMermaidSource } from '../diagrams/mermaidRender'

type Props = {
  content: string
  path?: string
  testId?: string
  allowMermaidDocument?: boolean
}

function isMermaidSource(path?: string, content?: string): boolean {
  const base = (path || '').split(/[/\\]/).pop()?.toLowerCase() || ''
  if (base.endsWith('.mmd') || base.endsWith('.mermaid')) return true
  const t = (content || '').trim()
  return (
    /^```mermaid\b/i.test(t) ||
    isMermaidFence('', t) ||
    isMermaidFence('mermaid', t)
  )
}

function collectText(node: ReactNode): string {
  if (node == null || typeof node === 'boolean') return ''
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(collectText).join('')
  if (typeof node === 'object' && node !== null && 'props' in node) {
    const children = (node as { props?: { children?: ReactNode } }).props
      ?.children
    return collectText(children)
  }
  return ''
}

export function MarkdownPreview({
  content,
  path,
  testId = 'markdown-preview',
  allowMermaidDocument = true,
}: Props) {
  const mermaidOnly = allowMermaidDocument && isMermaidSource(path, content)

  const components = useMemo(
    () => ({
      pre: ({ children }: { children?: ReactNode }) => <>{children}</>,
      code: ({
        className,
        children,
        ...rest
      }: {
        className?: string
        children?: ReactNode
      }) => {
        const lang =
          /language-([\w+-]+)/i.exec(className || '')?.[1]?.toLowerCase() || ''
        const raw = collectText(children).replace(/\n$/, '')
        const isBlock =
          Boolean(className && /language-/.test(className)) ||
          raw.includes('\n')
        if (!isBlock) {
          return (
            <code
              {...rest}
              className="rounded border border-shell-border bg-shell-panel px-1 py-0.5 font-mono text-[0.9em]"
            >
              {children}
            </code>
          )
        }
        if (allowMermaidDocument && isMermaidFence(lang, raw)) {
          return (
            <MermaidDiagram
              source={raw}
              testId={`${testId}-mermaid-block`}
              showSourceFallback
            />
          )
        }
        return (
          <pre className="my-2 overflow-x-auto rounded border border-shell-border bg-shell-panel/80 p-2.5">
            <code className="font-mono text-[12px] text-shell-text">{raw}</code>
          </pre>
        )
      },
    }),
    [testId, allowMermaidDocument],
  )

  return (
    <div
      data-testid={testId}
      data-path={path}
      data-mode="preview"
      data-preview-kind={mermaidOnly ? 'mermaid' : 'markdown'}
      className="markdown-preview min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain px-4 py-3 text-[13px] leading-relaxed text-shell-text"
    >
      {!content.trim() ? (
        <p className="text-shell-muted">Empty document</p>
      ) : mermaidOnly ? (
        <MermaidDiagram
          source={normalizeMermaidSource(content)}
          testId={`${testId}-mermaid`}
          showSourceFallback
        />
      ) : (
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
          {content}
        </ReactMarkdown>
      )}
    </div>
  )
}
