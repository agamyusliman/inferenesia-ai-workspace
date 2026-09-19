import { useMemo, type ReactNode } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { MermaidDiagram } from '../diagrams/MermaidDiagram'
import type { WebPreviewSeed } from '../web-preview/webPreview'
import { codeFenceLanguage, isMermaidFence } from './messageChrome'
import { ChatCodeBlock } from './ChatCodeBlock'
import { isLiveBlockLanguage } from './liveBlocks'

type Props = {
  content: string
  onImageClick?: (src: string, alt: string) => void
  testId?: string
  className?: string
  liveBlocksEnabled?: boolean
  onOpenWebPreview?: (seed: WebPreviewSeed) => void
  sessionName?: string
  chatMessageId?: string
  workspaceId?: string
}

function collectText(node: ReactNode): string {
  if (node == null || typeof node === 'boolean') return ''
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(collectText).join('')
  if (typeof node === 'object' && node !== null && 'props' in node) {
    const children = (node as { props?: { children?: ReactNode } }).props?.children
    return collectText(children)
  }
  return ''
}

export function ChatMarkdown({
  content,
  onImageClick,
  testId = 'chat-markdown',
  className,
  liveBlocksEnabled = false,
  onOpenWebPreview,
  sessionName,
  chatMessageId,
  workspaceId,
}: Props) {
  const htmlBlockSeq = useMemo(() => ({ n: 0 }), [content])
  htmlBlockSeq.n = 0

  const components = useMemo(
    () => ({
      a: ({ href, children, ...rest }: any) => (
        <a
          {...rest}
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="text-shell-accent underline decoration-shell-accent/40 underline-offset-2 transition hover:decoration-shell-accent"
        >
          {children}
        </a>
      ),
      img: ({ src, alt, ...rest }: any) => {
        const s = typeof src === 'string' ? src.replace(/\s+/g, '') : ''
        const a = typeof alt === 'string' ? alt : ''
        if (!s) return null
        return (
          <button
            type="button"
            data-testid="chat-md-image"
            data-src={s.slice(0, 48)}
            className="group/img my-1.5 flex w-full max-w-sm flex-col overflow-hidden rounded-lg border border-shell-border bg-shell-panel/50 p-0 text-left transition hover:border-shell-accent/50"
            onClick={() => onImageClick?.(s, a)}
            title="Click to preview"
          >
            <span className="flex max-h-72 min-h-[8rem] items-center justify-center bg-[#0b1016] p-2">
              <img
                {...rest}
                src={s}
                alt={a || 'Chat image'}
                className="max-h-64 max-w-full object-contain transition group-hover/img:opacity-95"
                loading="lazy"
                decoding="async"
              />
            </span>
          </button>
        )
      },
      pre: ({ children }: any) => <>{children}</>,
      code: ({ className: cn, children, ...rest }: any) => {
        const lang = codeFenceLanguage(cn)
        const raw = collectText(children)
        const isBlock =
          Boolean(cn && /language-/.test(cn)) || raw.includes('\n')
        if (!isBlock) {
          return (
            <code
              {...rest}
              className="rounded border border-shell-border bg-shell-panel px-1 py-0.5 font-mono text-[0.9em] text-shell-text"
            >
              {children}
            </code>
          )
        }
        const text = raw.replace(/\n$/, '')
        const langLower = (lang || '').toLowerCase()
        const looksHtml =
          langLower === 'html' ||
          langLower === 'htm' ||
          langLower.startsWith('html:') ||
          isLiveBlockLanguage(lang) ||
          /<!doctype|<html[\s>]/i.test(text.slice(0, 200))
        let htmlBlockIndex: number | undefined
        if (looksHtml) {
          htmlBlockSeq.n += 1
          htmlBlockIndex = htmlBlockSeq.n
        }
        if (liveBlocksEnabled && isLiveBlockLanguage(lang)) {
          return (
            <ChatCodeBlock
              text={text}
              languageClass={cn || 'language-html'}
              codeProps={rest}
              onOpenWebPreview={onOpenWebPreview}
              sessionName={sessionName}
              htmlBlockIndex={htmlBlockIndex}
              chatMessageId={chatMessageId}
              workspaceId={workspaceId}
            >
              {children}
            </ChatCodeBlock>
          )
        }
        if (isMermaidFence(lang, text)) {
          return (
            <MermaidDiagram
              source={text}
              testId="chat-mermaid-preview"
              showSourceFallback
              onOpenInCanvas={
                onOpenWebPreview
                  ? () =>
                      onOpenWebPreview({
                        mode: 'diagram',
                        diagramSource: text,
                        title: 'Mermaid diagram',
                        prefer: 'edit',
                        nonce: Date.now(),
                        chatMessageId,
                        scopeId: workspaceId,
                      })
                  : undefined
              }
              openInCanvasLabel="Open in playground"
            />
          )
        }
        return (
          <ChatCodeBlock
            text={text}
            languageClass={cn}
            codeProps={rest}
            onOpenWebPreview={onOpenWebPreview}
            sessionName={sessionName}
            htmlBlockIndex={htmlBlockIndex}
            chatMessageId={chatMessageId}
            workspaceId={workspaceId}
          >
            {children}
          </ChatCodeBlock>
        )
      },
    }),
    [
      liveBlocksEnabled,
      onImageClick,
      onOpenWebPreview,
      sessionName,
      chatMessageId,
      workspaceId,
      htmlBlockSeq,
    ],
  )

  return (
    <div
      data-testid={testId}
      className={`chat-markdown markdown-preview min-w-0 text-[12px] leading-relaxed text-shell-text ${className || ''}`}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {content}
      </ReactMarkdown>
    </div>
  )
}
