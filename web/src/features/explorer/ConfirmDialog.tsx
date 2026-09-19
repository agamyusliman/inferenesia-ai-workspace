import { useLocale } from '../i18n/LocaleProvider'

type Props = {
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  cancelLabel,
  danger = true,
  onConfirm,
  onCancel,
}: Props) {
  const { t } = useLocale()
  const confirmText = confirmLabel ?? t('action.delete')
  const cancelText = cancelLabel ?? t('action.cancel')
  return (
    <div
      data-testid="confirm-dialog-backdrop"
      className="fixed inset-0 z-[110] flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onClick={onCancel}
    >
      <div
        data-testid="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        className="w-full max-w-sm rounded-md border border-shell-border bg-shell-panel p-4 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2
          id="confirm-dialog-title"
          className="text-sm font-medium text-shell-text"
        >
          {title}
        </h2>
        <p className="mt-2 text-xs text-shell-muted">{message}</p>
        <div className="mt-4 flex justify-end gap-2">
          <button
            type="button"
            data-testid="confirm-dialog-cancel"
            className="rounded border border-shell-border px-3 py-1 text-xs text-shell-text hover:bg-shell-border/30"
            onClick={onCancel}
          >
            {cancelText}
          </button>
          <button
            type="button"
            data-testid="confirm-dialog-confirm"
            className={`rounded px-3 py-1 text-xs text-white ${
              danger
                ? 'bg-red-600 hover:bg-red-500'
                : 'bg-shell-accent hover:brightness-110'
            }`}
            onClick={onConfirm}
          >
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  )
}
