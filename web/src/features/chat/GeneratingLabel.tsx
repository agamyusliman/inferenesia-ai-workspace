type Props = {
  continuing?: boolean
  imageGen?: boolean
  variant?: 'bubble' | 'composer'
  className?: string
  hideSpinner?: boolean
}

export function GeneratingLabel({
  continuing = false,
  imageGen = false,
  variant = 'bubble',
  className = '',
  hideSpinner = false,
}: Props) {
  const label = imageGen
    ? 'Generating image'
    : continuing
      ? 'Continuing'
      : 'Generating'
  const testId =
    variant === 'composer'
      ? imageGen
        ? 'chat-composer-generating-image'
        : 'chat-composer-generating'
      : imageGen
        ? 'chat-generating-image-label'
        : continuing
          ? 'chat-continuing-label'
          : 'chat-generating-label'

  return (
    <span
      data-testid={testId}
      data-continuing={continuing ? 'true' : 'false'}
      data-image-gen={imageGen ? 'true' : undefined}
      className={`inline-flex items-center gap-1.5 text-[10px] text-shell-accent ${className}`}
      aria-live="polite"
      aria-label={
        imageGen
          ? 'Generating image'
          : continuing
            ? 'Continuing response'
            : 'Generating response'
      }
    >
      {!hideSpinner ? (
        <span className="chat-gen-spinner" aria-hidden />
      ) : null}
      <span className="chat-gen-shimmer font-medium tracking-wide">{label}</span>
      <span className="chat-gen-dots" aria-hidden>
        <span className="chat-gen-dot" />
        <span className="chat-gen-dot" />
        <span className="chat-gen-dot" />
      </span>
    </span>
  )
}
