type Props = {
  /** Basename or path shown in the dialog title/message. */
  fileName: string
  onSave: () => void
  onDontSave: () => void
  onCancel: () => void
}

/**
 * Close-tab confirmation when the buffer has unsaved edits (VAL-IDE-019).
 * Save / Don't Save / Cancel.
 */
export function CloseDirtyDialog({ fileName, onSave, onDontSave, onCancel }: Props) {
  return (
    <div
      data-testid="close-dirty-dialog-backdrop"
      className="fixed inset-0 z-[120] flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onClick={onCancel}
    >
      <div
        data-testid="close-dirty-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="close-dirty-title"
        className="w-full max-w-sm rounded-md border border-shell-border bg-shell-panel p-4 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2
          id="close-dirty-title"
          className="text-sm font-medium text-shell-text"
        >
          Unsaved changes
        </h2>
        <p className="mt-2 text-xs text-shell-muted">
          Do you want to save changes to{' '}
          <span className="font-medium text-shell-text">{fileName}</span> before closing?
        </p>
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          <button
            type="button"
            data-testid="close-dirty-cancel"
            className="rounded border border-shell-border px-3 py-1 text-xs text-shell-text hover:bg-shell-border/30"
            onClick={onCancel}
          >
            Cancel
          </button>
          <button
            type="button"
            data-testid="close-dirty-dont-save"
            className="rounded border border-shell-border px-3 py-1 text-xs text-shell-text hover:bg-shell-border/30"
            onClick={onDontSave}
          >
            Don&apos;t Save
          </button>
          <button
            type="button"
            data-testid="close-dirty-save"
            className="rounded bg-shell-accent px-3 py-1 text-xs text-white hover:brightness-110"
            onClick={onSave}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  )
}
