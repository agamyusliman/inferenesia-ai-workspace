import { useCallback, useEffect, useRef, useState } from 'react'
import { Download, LayoutPanelLeft } from 'lucide-react'
import {
  buildDiffPreview,
  CODE_PREVIEW_LINE_LIMIT,
  diffLineClass,
  downloadFilename,
  inferenesiaSnippetDownloadName,
  parseFenceMeta,
  shouldRenderAsDiff,
  truncateCodeLines,
} from './codeBlockUtils'
import { CodeEditor } from '../editor/CodeEditor'
import {
  findCanvasByHtml,
  LIBRARY_SCOPE_ID,
  loadCanvases,
  titleFromHtml,
} from '../web-preview/canvasStore'
import {
  buildPreviewSrcDoc,
  isPreviewableHtmlLanguage,
  looksLikeHtmlDocument,
  type WebPreviewSeed,
} from '../web-preview/webPreview'

type Props = {
  text: string
  languageClass?: string | null
  children?: React.ReactNode
  codeProps?: React.HTMLAttributes<HTMLElement>
  onOpenWebPreview?: (seed: WebPreviewSeed) => void
  sessionName?: string
  htmlBlockIndex?: number
  chatMessageId?: string
  workspaceId?: string
}

function canvasLinkId(
  chatMessageId: string | undefined,
  htmlBlockIndex: number | undefined,
): string | undefined {
  if (!chatMessageId) return undefined
  if (htmlBlockIndex && htmlBlockIndex > 1) {
    return `${chatMessageId}#html${htmlBlockIndex}`
  }
  return chatMessageId
}

function resolveCanvasTitle(opts: {
  workspaceId?: string
  linkId?: string
  html: string
  path?: string
  fallbackIndex?: number
}): string {
  const lib = findCanvasByHtml(LIBRARY_SCOPE_ID, opts.html)
  if (lib?.title?.trim()) return lib.title.trim()
  if (opts.workspaceId && opts.linkId) {
    const found = loadCanvases(opts.workspaceId).find(
      (c) => c.chatMessageId === opts.linkId,
    )
    if (found?.title?.trim()) return found.title.trim()
  }
  const fromHtml = titleFromHtml(opts.html, opts.path)
  if (fromHtml && !/^Canvas\s/i.test(fromHtml)) return fromHtml
  if (opts.path) {
    const base = opts.path.split(/[/\\]/).pop()
    if (base) return base
  }
  if (opts.fallbackIndex && opts.fallbackIndex > 1) {
    return `Canvas ${opts.fallbackIndex}`
  }
  return fromHtml || 'Playground'
}

const btnClass =
  'relative z-30 inline-flex shrink-0 items-center gap-1 rounded border border-shell-border/80 bg-shell-panel px-1.5 py-0.5 text-[10px] text-shell-muted transition pointer-events-auto hover:border-shell-accent/40 hover:text-shell-text'

export function ChatCodeBlock({
  text,
  languageClass,
  onOpenWebPreview,
  sessionName,
  htmlBlockIndex,
  chatMessageId,
  workspaceId,
}: Props) {
  const linkId = canvasLinkId(chatMessageId, htmlBlockIndex)
  const [expanded, setExpanded] = useState(false)
  const [htmlDraft, setHtmlDraft] = useState(text)
  const [htmlLive, setHtmlLive] = useState(text)
  const draftRef = useRef(text)
  const dirtyRef = useRef(false)
  const propTextRef = useRef(text)
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const lastSrcDocRef = useRef('')

  const langFromClass = (() => {
    if (!languageClass) return ''
    const m = /language-([\w+:./\\-]+)/i.exec(languageClass)
    return m?.[1] || ''
  })()
  const { language, path } = parseFenceMeta(langFromClass)
  const isDiff = shouldRenderAsDiff(language, text)
  const isHtml =
    isPreviewableHtmlLanguage(language) ||
    (language === '' && looksLikeHtmlDocument(text))
  const filename = isHtml
    ? inferenesiaSnippetDownloadName({
        sessionName,
        canvasIndex: htmlBlockIndex ?? 1,
        ext: 'html',
      })
    : downloadFilename(language, path, isDiff, {
        sessionName,
        blockIndex: htmlBlockIndex ?? 1,
      })

  useEffect(() => {
    if (text === propTextRef.current) return
    propTextRef.current = text
    if (dirtyRef.current) return
    draftRef.current = text
    setHtmlDraft(text)
    setHtmlLive(text)
  }, [text])

  useEffect(() => {
    if (!isHtml) return
    const next = buildPreviewSrcDoc(htmlLive)
    if (next === lastSrcDocRef.current) return
    lastSrcDocRef.current = next
    const el = iframeRef.current
    if (el) el.srcdoc = next
  }, [htmlLive, isHtml])

  const onIframeMount = useCallback(
    (node: HTMLIFrameElement | null) => {
      iframeRef.current = node
      if (!node || !isHtml) return
      const next = buildPreviewSrcDoc(htmlLive)
      if (next === lastSrcDocRef.current && node.srcdoc) return
      lastSrcDocRef.current = next
      node.srcdoc = next
    },
    [htmlLive, isHtml],
  )

  const applyLive = useCallback((next?: string) => {
    const body = next ?? draftRef.current
    dirtyRef.current = false
    setHtmlLive((prev) => (prev === body ? prev : body))
  }, [])

  const onHtmlChange = (v: string) => {
    dirtyRef.current = true
    draftRef.current = v
    setHtmlDraft(v)
  }

  const canvasTitle = isHtml
    ? resolveCanvasTitle({
        workspaceId,
        linkId,
        html: htmlDraft,
        path,
        fallbackIndex: htmlBlockIndex,
      })
    : ''

  const openCanvasPanel = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const html = draftRef.current
    applyLive(html)
    if (!onOpenWebPreview) return
    const title = resolveCanvasTitle({
      workspaceId,
      linkId,
      html,
      path,
      fallbackIndex: htmlBlockIndex,
    })
    const libMatch = findCanvasByHtml(LIBRARY_SCOPE_ID, html)
    const sessionMatch =
      workspaceId && linkId
        ? loadCanvases(workspaceId).find((c) => c.chatMessageId === linkId)
        : undefined
    if (libMatch) {
      onOpenWebPreview({
        mode: 'html',
        html,
        path,
        title: libMatch.title || title,
        prefer: 'edit',
        nonce: Date.now(),
        chatMessageId: linkId,
        sourceHtml: html,
        scopeId: LIBRARY_SCOPE_ID,
        canvasId: libMatch.id,
      })
      return
    }
    onOpenWebPreview({
      mode: 'html',
      html,
      path,
      title,
      prefer: sessionMatch ? 'edit' : 'new',
      nonce: Date.now(),
      chatMessageId: linkId,
      sourceHtml: html,
      scopeId: workspaceId || LIBRARY_SCOPE_ID,
      canvasId: sessionMatch?.id,
    })
  }

  const plainTrunc = truncateCodeLines(isHtml ? htmlDraft : text, CODE_PREVIEW_LINE_LIMIT)
  const diffPreview = isDiff
    ? buildDiffPreview(text, CODE_PREVIEW_LINE_LIMIT)
    : null
  const truncated = isDiff
    ? Boolean(diffPreview?.truncated)
    : plainTrunc.truncated
  const totalLines = isDiff
    ? diffPreview?.totalLines || 0
    : plainTrunc.totalLines
  const showFull = expanded || !truncated

  const onDownload = () => {
    const blob = new Blob([isHtml ? htmlDraft : text], {
      type: 'text/plain;charset=utf-8',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  const label = path || (isDiff ? 'diff' : language || 'code')
  const stats =
    isDiff && diffPreview
      ? `+${diffPreview.stats.additions} −${diffPreview.stats.deletions}`
      : null

  if (isHtml) {
    return (
      <div
        data-testid="chat-code-block"
        data-language={language || 'html'}
        data-html="true"
        className="chat-code-block my-1.5 block max-h-[min(40rem,72vh)] overflow-hidden rounded-md border border-shell-border bg-shell-panel"
        onMouseDown={(e) => e.stopPropagation()}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          data-testid="chat-html-toolbar"
          className="relative z-30 flex items-center justify-between gap-2 border-b border-shell-border bg-shell-panel px-2 py-1"
          onMouseDown={(e) => e.stopPropagation()}
        >
          <span className="min-w-0 flex items-center gap-1.5 truncate">
            <span className="shrink-0 font-mono text-[10px] uppercase tracking-wide text-shell-muted">
              html
            </span>
            <span
              data-testid="chat-html-canvas-title"
              className="min-w-0 truncate text-[11px] font-medium text-shell-text"
              title={canvasTitle}
            >
              {canvasTitle}
            </span>
          </span>
          <span className="flex shrink-0 flex-wrap items-center justify-end gap-1">
            <button
              type="button"
              data-testid="chat-code-download"
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                onDownload()
              }}
              className={btnClass}
              title={`Download ${filename}`}
            >
              <Download size={10} />
              Download
            </button>
            {onOpenWebPreview && (
              <button
                type="button"
                data-testid="chat-code-web-preview"
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => openCanvasPanel(e)}
                className={btnClass}
                title={`Open in playground · ${canvasTitle}`}
              >
                <LayoutPanelLeft size={10} />
                Open in playground
              </button>
            )}
          </span>
        </div>

        <div
          data-testid="chat-html-grid"
          className="grid h-[min(36rem,68vh)] min-h-72 grid-cols-1 divide-y divide-shell-border md:grid-cols-2 md:divide-x md:divide-y-0"
        >
          <div
            data-testid="chat-html-code-col"
            className="relative z-10 flex min-h-0 min-w-0 flex-col overflow-hidden"
            style={{ background: '#1e1e1e' }}
          >
            <CodeEditor
              path={
                path && /\.html?$/i.test(path)
                  ? path
                  : path
                    ? `${path.replace(/\/$/, '')}/preview.html`
                    : 'chat-inline-preview.html'
              }
              value={htmlDraft}
              onChange={onHtmlChange}
              onSave={() => applyLive()}
              testId="chat-html-editor"
              fontSize={12}
            />
          </div>
          <div
            data-testid="chat-html-preview-col"
            className="relative z-0 flex min-h-0 min-w-0 flex-col overflow-hidden bg-white"
          >
            <div className="min-h-0 flex-1 overflow-auto">
              <iframe
                ref={onIframeMount}
                data-testid="chat-html-inline-preview"
                title="HTML preview"
                sandbox="allow-scripts allow-forms allow-modals"
                className="block min-h-full w-full border-0 bg-white"
                style={{ minHeight: '100%', height: '100%' }}
              />
            </div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <span
      data-testid="chat-code-block"
      data-language={language || (isDiff ? 'diff' : 'text')}
      data-diff={isDiff ? 'true' : 'false'}
      data-html="false"
      data-truncated={truncated && !expanded ? 'true' : 'false'}
      className="chat-code-block my-1.5 block overflow-hidden rounded-md border border-shell-border"
    >
      <span className="flex items-center justify-between gap-2 border-b border-shell-border bg-shell-panel px-2 py-1">
        <span className="min-w-0 truncate font-mono text-[10px] uppercase tracking-wide text-shell-muted">
          {label}
          {stats ? (
            <span className="ml-1.5 normal-case tracking-normal text-shell-muted/90">
              {stats}
            </span>
          ) : null}
        </span>
        <span className="flex shrink-0 items-center gap-1">
          <button
            type="button"
            data-testid="chat-code-download"
            onClick={onDownload}
            className={btnClass}
            title={`Download ${filename}`}
          >
            <Download size={10} />
            Download
          </button>
        </span>
      </span>

      {isDiff && diffPreview ? (
        <pre
          data-testid="chat-code-diff"
          className="m-0 max-w-full overflow-x-auto rounded-b-md border-0 bg-[#0b1016] font-mono text-[11px] leading-snug"
        >
          {(showFull
            ? buildDiffPreview(text, Number.MAX_SAFE_INTEGER).lines
            : diffPreview.lines
          ).map((line, i) => (
            <div
              key={i}
              className={`whitespace-pre-wrap break-all px-2.5 py-0.5 ${diffLineClass(line.kind)}`}
            >
              {line.text || ' '}
            </div>
          ))}
        </pre>
      ) : (
        <div className="h-80 max-h-80 min-h-0">
          <CodeEditor
            path={
              path ||
              (language
                ? `snippet.${language.replace(/[^a-z0-9+.-]/gi, '') || 'txt'}`
                : 'snippet.txt')
            }
            value={showFull ? text : plainTrunc.preview}
            onChange={() => undefined}
            readOnly
            testId="chat-code-readonly"
            fontSize={12}
          />
        </div>
      )}

      {truncated && (
        <span className="flex items-center justify-between gap-2 border-t border-shell-border bg-shell-panel/80 px-2 py-1">
          <span className="text-[10px] text-shell-muted">
            Showing {Math.min(CODE_PREVIEW_LINE_LIMIT, totalLines)} of {totalLines}{' '}
            lines
          </span>
          <button
            type="button"
            data-testid="chat-code-view-more"
            onClick={() => setExpanded((v) => !v)}
            className="rounded border border-shell-border px-1.5 py-0.5 text-[10px] text-shell-accent hover:bg-shell-border/30"
          >
            {expanded ? 'View less' : 'View more'}
          </button>
        </span>
      )}
    </span>
  )
}
