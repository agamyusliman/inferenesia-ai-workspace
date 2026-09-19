import {
  formatMetaParts,
  shouldShowMetaRow,
  type ChatMeta,
} from './chatMetaBlocks'

type Props = {
  meta?: ChatMeta | null
  className?: string
}

export function ChatMetaRow({ meta, className }: Props) {
  if (!shouldShowMetaRow(meta)) return null
  const parts = formatMetaParts(meta)
  return (
    <div
      data-testid="chat-msg-meta"
      data-tokens={meta?.tokens != null ? String(meta.tokens) : undefined}
      data-latency-ms={meta?.latencyMs != null ? String(meta.latencyMs) : undefined}
      data-stream-status={meta?.streamStatus || undefined}
      data-mode={meta?.mode || undefined}
      data-model={meta?.model || undefined}
      data-profile={meta?.profile || undefined}
      className={`flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[10px] text-shell-muted ${className || ''}`}
    >
      {parts.map((p, i) => (
        <span key={`${p}-${i}`} className="inline-flex min-w-0 items-center gap-1.5">
          {i > 0 && (
            <span className="text-shell-muted/50" aria-hidden>
              ·
            </span>
          )}
          <span
            data-testid={metaPartTestId(p, i, meta)}
            className={
              i === 0 && meta?.mode
                ? 'font-medium text-shell-text/90'
                : i <= 1 && (meta?.model || meta?.profile)
                  ? 'truncate font-mono text-shell-muted'
                  : undefined
            }
            title={p}
          >
            {p}
          </span>
        </span>
      ))}
    </div>
  )
}

function metaPartTestId(part: string, index: number, meta?: ChatMeta | null): string {
  if (index === 0 && meta?.mode && part === meta.mode) return 'chat-meta-mode'
  if (part.includes('token')) return 'chat-meta-tokens'
  if (part.includes('ms') || part.endsWith(' s')) return 'chat-meta-latency'
  if (meta?.model && part.includes(meta.model)) return 'chat-meta-model'
  if (meta?.profile && part.includes(meta.profile)) return 'chat-meta-profile'
  return 'chat-meta-status'
}
