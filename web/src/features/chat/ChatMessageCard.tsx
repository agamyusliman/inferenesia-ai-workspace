import { memo, useCallback, useMemo, useState } from 'react'
import { assistantStatusLabel } from './chatStreamUx'
import { GeneratingLabel } from './GeneratingLabel'
import { ChatMarkdown } from './ChatMarkdown'
import { ChatMetaRow } from './ChatMetaRow'
import { shouldShowMetaRow } from './chatMetaBlocks'
import { GeneratedImageCards } from './GeneratedImageCard'
import { ImageLightbox } from './ImageLightbox'
import { MessageActionBar } from './MessageActionBar'
import { TaskBlock } from './TaskBlock'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolCallBlocks } from './ToolCallBlock'
import {
  resolveThinkingAndAnswer,
  type ChatMeta,
  type TaskBlock as TaskBlockModel,
  type ToolCallBlock as ToolCallBlockModel,
} from './chatMetaBlocks'
import {
  extractMarkdownImages,
  isAssistantPlainInterim,
  isAssistantRole,
  isUserRole,
  messageCardClass,
  shouldRenderMarkdown,
  shouldShowActionBar,
  stripMarkdownImages,
} from './messageChrome'
import { BubbleAttachmentChips } from './AttachmentChips'
import type { BubbleAttachmentChip } from './chatAttachments'
import type { WebPreviewSeed } from '../web-preview/webPreview'

const StableChatMarkdown = memo(function StableChatMarkdown({
  content,
  liveBlocksEnabled,
  onOpenWebPreview,
  onImageClick,
  streaming,
  stopped,
  role,
  sessionName,
  chatMessageId,
  workspaceId,
}: {
  content: string
  liveBlocksEnabled?: boolean
  onOpenWebPreview?: (seed: WebPreviewSeed) => void
  onImageClick: (src: string, alt: string) => void
  streaming?: boolean
  stopped?: boolean
  role: string
  sessionName?: string
  chatMessageId?: string
  workspaceId?: string
}) {
  return (
    <div
      data-testid={isAssistantRole(role) ? 'assistant-stream' : 'user-content'}
      data-streaming={streaming ? 'true' : 'false'}
      data-stopped={stopped ? 'true' : 'false'}
    >
      <ChatMarkdown
        content={content}
        onImageClick={onImageClick}
        liveBlocksEnabled={liveBlocksEnabled}
        onOpenWebPreview={onOpenWebPreview}
        sessionName={sessionName}
        chatMessageId={chatMessageId}
        workspaceId={workspaceId}
      />
    </div>
  )
})

export type ChatMessageCardModel = {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  mentions?: string[]
  /** Post-send paperclip chips on user bubble (VAL-CHAT-024). */
  attachments?: BubbleAttachmentChip[]
  streaming?: boolean
  stopped?: boolean
  /** Failed turn — show error styling in/near bubble (VAL-CHAT-028). */
  error?: boolean
  errorText?: string
  /** Auto-continue in progress (VAL-CHAT-026). */
  continuing?: boolean
  model?: string
  profile?: string
  host?: string
  /** Tokens / latency / stream status when available (VAL-CHAT-012). */
  meta?: ChatMeta
  /** Explicit reasoning text from stream (optional). */
  thinking?: string
  /** Distinct tool call UI blocks (VAL-CHAT-014). */
  tools?: ToolCallBlockModel[]
  /** Distinct subagent/task progress blocks (VAL-CHAT-015). */
  tasks?: TaskBlockModel[]
}

type Props = {
  message: ChatMessageCardModel
  /** Single-message delete (VAL-CHAT-018). */
  onDelete?: (messageId: string) => void
  /** Undo-from-here truncate (VAL-CHAT-017). */
  onUndoFromHere?: (messageId: string) => void
  liveBlocksEnabled?: boolean
  onOpenWebPreview?: (seed: import('../web-preview/webPreview').WebPreviewSeed) => void
  sessionName?: string
  workspaceId?: string
}

function ChatMessageCardInner({
  message: m,
  onDelete,
  onUndoFromHere,
  liveBlocksEnabled = false,
  onOpenWebPreview,
  sessionName,
  workspaceId,
}: Props) {
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [lightbox, setLightbox] = useState<{ src: string; alt: string } | null>(null)
  const onMarkdownImageClick = useCallback((src: string, alt: string) => {
    setLightbox({ src, alt })
  }, [])

  const hasToolsEarly = Boolean(m.tools && m.tools.length > 0)
  const hasTasksEarly = Boolean(m.tasks && m.tasks.length > 0)
  const images = useMemo(
    () => (isAssistantRole(m.role) ? extractMarkdownImages(m.content) : []),
    [m.role, m.content],
  )
  const plainInterim =
    isAssistantRole(m.role) &&
    isAssistantPlainInterim({
      streaming: m.streaming,
      stopped: m.stopped,
      error: m.error,
      hasTools: hasToolsEarly,
      hasTasks: hasTasksEarly,
      content: m.content,
    })
  const complete =
    !m.streaming &&
    !m.stopped &&
    !m.error &&
    Boolean(m.content?.length || m.tools?.length || m.tasks?.length || images.length)
  const showActions = shouldShowActionBar({
    streaming: m.streaming,
    complete,
    hovered,
    focused,
    plainInterim: plainInterim && images.length === 0,
  })

  const { thinking, answer } = isAssistantRole(m.role)
    ? resolveThinkingAndAnswer(m.content, m.thinking)
    : { thinking: '', answer: m.content || '' }

  const caption = images.length > 0 ? stripMarkdownImages(answer) : answer
  const useMarkdown =
    images.length === 0 &&
    shouldRenderMarkdown(m.role, caption) &&
    (Boolean(m.streaming) || !plainInterim)
  const imageGenTurn =
    m.meta?.mode === 'Image' ||
    images.length > 0 ||
    /^Generated image/i.test((m.content || '').trim())
  const status =
    isAssistantRole(m.role) && !plainInterim
      ? assistantStatusLabel(!!m.streaming, !!m.stopped, {
          continuing: !!m.continuing,
          error: !!m.error,
        })
      : isAssistantRole(m.role) && m.streaming
        ? assistantStatusLabel(true, false, { continuing: !!m.continuing })
        : ''

  // Footer: mode · profile:model · tokens · latency (OpenCode-style).
  const meta: ChatMeta | undefined = isAssistantRole(m.role)
    ? {
        ...m.meta,
        mode: m.meta?.mode,
        profile: m.meta?.profile || m.profile,
        model: m.meta?.model || m.model,
        streamStatus:
          m.meta?.streamStatus ||
          (m.error
            ? 'error'
            : m.streaming
              ? m.continuing
                ? 'continuing'
                : 'generating'
              : m.stopped
                ? 'stopped'
                : complete
                  ? 'done'
                  : undefined),
      }
    : undefined

  const hasTools = hasToolsEarly
  const hasTasks = hasTasksEarly
  const errorBody =
    m.error && m.errorText?.trim()
      ? m.errorText.trim()
      : m.error && answer.trim()
        ? answer.trim()
        : ''
  const answerIsOnlyError =
    Boolean(errorBody) && answer.trim() === errorBody
  const showAnswer =
    Boolean(caption && caption.trim().length > 0) && !answerIsOnlyError

  return (
    <>
      <article
        key={m.id}
        data-testid={`chat-msg-${m.role}`}
        data-role={m.role}
        data-message-id={m.id}
        data-complete={complete ? 'true' : 'false'}
        data-hovered={hovered ? 'true' : 'false'}
        data-error={m.error ? 'true' : undefined}
        data-continuing={m.continuing ? 'true' : undefined}
        data-plain={plainInterim ? 'true' : undefined}
        tabIndex={plainInterim ? -1 : 0}
        onMouseEnter={() => {
          if (!plainInterim) setHovered(true)
        }}
        onMouseLeave={() => setHovered(false)}
        onFocus={() => {
          if (!plainInterim) setFocused(true)
        }}
        onBlur={() => setFocused(false)}
        className={messageCardClass(m.role, {
          streaming: m.streaming,
          plainInterim,
        })}
      >
        {(isUserRole(m.role) ||
          (isAssistantRole(m.role) &&
            !plainInterim &&
            (status || m.streaming))) && (
          <div
            className={`flex min-w-0 max-w-full items-center justify-between gap-1 ${
              isUserRole(m.role) ? 'mb-1' : 'mb-0.5'
            }`}
          >
            <div className="flex min-w-0 flex-wrap items-center gap-1 text-[10px] font-semibold uppercase leading-4 text-shell-muted">
              {isUserRole(m.role) && (
                <span data-testid="chat-msg-role" className="text-shell-accent">
                  {m.role}
                </span>
              )}
              {isAssistantRole(m.role) && (
                <span data-testid="chat-msg-role" className="sr-only">
                  {m.role}
                </span>
              )}
              {status && (
                <span
                  data-testid={m.streaming ? 'chat-generating' : 'chat-stream-status'}
                  data-status={status}
                  className={`font-normal normal-case leading-4 ${
                    m.streaming
                      ? 'animate-pulse text-shell-accent'
                      : m.stopped
                        ? 'text-amber-300/90'
                        : ''
                  }`}
                >
                  {status}
                </span>
              )}
              {!m.streaming && isAssistantRole(m.role) && m.meta?.mode && (
                <span
                  data-testid="chat-msg-mode-badge"
                  className="font-normal normal-case leading-4 text-shell-muted/80"
                >
                  · {m.meta.mode}
                </span>
              )}
            </div>
            {isUserRole(m.role) && (
              <div
                className={`relative z-30 ml-auto flex h-4 shrink-0 items-center overflow-visible transition-opacity duration-150 ${
                  showActions ? 'opacity-100' : 'pointer-events-none opacity-0'
                }`}
              >
                <MessageActionBar
                  role={m.role}
                  content={m.content}
                  messageId={m.id}
                  visible={showActions}
                  streaming={m.streaming}
                  sessionName={sessionName}
                  onDelete={onDelete}
                  onUndoFromHere={onUndoFromHere}
                  onPreviewImage={(src, alt) => setLightbox({ src, alt })}
                />
              </div>
            )}
          </div>
        )}

        {m.mentions && m.mentions.length > 0 && (
          <div data-testid="chat-msg-mentions" className="mb-1 flex flex-wrap gap-1">
            {m.mentions.map((p) => (
              <span
                key={p}
                className="rounded bg-shell-panel px-1.5 py-0.5 font-mono text-[10px] text-shell-accent"
              >
                @{p}
              </span>
            ))}
          </div>
        )}

        {/* Post-send attachment chips on user bubble (VAL-CHAT-024). */}
        {isUserRole(m.role) && m.attachments && m.attachments.length > 0 && (
          <BubbleAttachmentChips chips={m.attachments} />
        )}

        {isAssistantRole(m.role) && thinking && (
          <ThinkingBlock
            content={thinking}
            streaming={!!m.streaming && !answer}
            defaultOpen={!!m.streaming && !answer}
          />
        )}

        {/* Subagent/task progress — distinct from primary prose (VAL-CHAT-015). */}
        {hasTasks && (
          <div data-testid="chat-task-blocks" className="mb-2 flex flex-col gap-1.5">
            {m.tasks!.map((t) => (
              <TaskBlock key={t.id} task={t} />
            ))}
          </div>
        )}

        {hasTools && (
          <ToolCallBlocks tools={m.tools!} streaming={!!m.streaming} />
        )}

        {showAnswer &&
          (useMarkdown ? (
            <StableChatMarkdown
              content={caption}
              role={m.role}
              streaming={m.streaming}
              stopped={m.stopped}
              liveBlocksEnabled={liveBlocksEnabled}
              onOpenWebPreview={onOpenWebPreview}
              onImageClick={onMarkdownImageClick}
              sessionName={sessionName}
              chatMessageId={m.id}
              workspaceId={workspaceId}
            />
          ) : (
            <p
              data-testid={isAssistantRole(m.role) ? 'assistant-stream' : 'user-content'}
              data-streaming={m.streaming ? 'true' : 'false'}
              data-stopped={m.stopped ? 'true' : 'false'}
              className={`m-0 max-w-full whitespace-pre-wrap break-words font-sans ${
                plainInterim && images.length === 0
                  ? 'text-[12px] leading-snug text-shell-text/85'
                  : 'text-shell-text'
              }`}
            >
              {caption}
            </p>
          ))}

        {images.length > 0 && (
          <GeneratedImageCards
            images={images}
            onClick={(src, alt) => setLightbox({ src, alt })}
            onOpenWebPreview={onOpenWebPreview}
            chatMessageId={m.id}
            workspaceId={workspaceId}
          />
        )}

        {m.streaming && (
          <div
            data-testid="chat-stream-indicator"
            className={`flex items-center gap-1.5 ${
              showAnswer || hasTools || hasTasks || thinking
                ? 'mt-2.5 pt-0.5'
                : 'mt-0.5'
            }`}
          >
            <GeneratingLabel
              continuing={!!m.continuing}
              imageGen={imageGenTurn || m.meta?.mode === 'Image'}
              variant="bubble"
            />
          </div>
        )}

        {isAssistantRole(m.role) && m.error && (
          <div
            data-testid="chat-msg-error-bubble"
            className="mt-1 text-[11px] leading-snug text-red-300/90"
            role="alert"
          >
            {errorBody || 'Chat turn failed'}
          </div>
        )}

        {isAssistantRole(m.role) &&
          (!plainInterim || images.length > 0) &&
          (shouldShowMetaRow(meta) || showActions) && (
          <div
            data-testid="chat-msg-footer"
            className="relative z-20 mt-1 flex min-w-0 items-center justify-between gap-2 overflow-visible pt-0.5"
          >
            <div className="min-w-0 flex-1 overflow-hidden">
              <ChatMetaRow meta={meta} />
            </div>
            <div
              className={`relative z-30 flex h-4 shrink-0 items-center overflow-visible transition-opacity duration-150 ${
                showActions ? 'opacity-100' : 'pointer-events-none opacity-0'
              }`}
            >
              <MessageActionBar
                role={m.role}
                content={m.content}
                messageId={m.id}
                visible={showActions}
                streaming={m.streaming}
                sessionName={sessionName}
                onDelete={onDelete}
                onUndoFromHere={onUndoFromHere}
                onPreviewImage={(src, alt) => setLightbox({ src, alt })}
              />
            </div>
          </div>
        )}
      </article>

      {lightbox && (
        <ImageLightbox
          src={lightbox.src}
          alt={lightbox.alt}
          sessionName={sessionName}
          onClose={() => setLightbox(null)}
        />
      )}
    </>
  )
}

export const ChatMessageCard = memo(ChatMessageCardInner)
