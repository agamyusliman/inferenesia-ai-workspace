type Props = {
  /** Workspace-relative or absolute URL path used as display caption. */
  path: string
  /**
   * Optional data URL or hub URL for the image. When absent, shows a
   * placeholder with the path (hub may later stream binary via /api/fs/blob).
   */
  src?: string
  testId?: string
}

/**
 * Image preview pane for renderable image files (VAL-IDE-026).
 */
export function ImagePreview({ path, src, testId = 'image-preview' }: Props) {
  const base = path.split(/[/\\]/).pop() || path
  return (
    <div
      data-testid={testId}
      data-path={path}
      data-mode="preview"
      className="flex min-h-0 min-w-0 flex-1 flex-col items-center justify-center gap-3 overflow-auto bg-shell-bg/40 p-4"
    >
      {src ? (
        <img
          src={src}
          alt={base}
          className="max-h-full max-w-full object-contain"
          data-testid={`${testId}-img`}
        />
      ) : (
        <div className="rounded border border-dashed border-shell-border px-6 py-8 text-center text-xs text-shell-muted">
          <p className="font-medium text-shell-text">{base}</p>
          <p className="mt-1">Image preview</p>
          <p className="mt-2 break-all text-[10px] opacity-70">{path}</p>
        </div>
      )}
    </div>
  )
}
