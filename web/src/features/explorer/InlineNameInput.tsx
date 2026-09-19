import { useEffect, useRef, useState } from 'react'

type Props = {
  /** Starting name (for rename) or suggested default. */
  initialValue: string
  placeholder?: string
  onSubmit: (name: string) => void
  onCancel: () => void
  testId?: string
}

/**
 * Compact inline name editor for New File / New Folder / Rename.
 * Enter confirms, Escape cancels, blur confirms when non-empty.
 */
export function InlineNameInput({
  initialValue,
  placeholder,
  onSubmit,
  onCancel,
  testId = 'inline-name-input',
}: Props) {
  const [value, setValue] = useState(initialValue)
  const ref = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.focus()
    // Select basename without extension when possible for rename UX.
    const dot = initialValue.lastIndexOf('.')
    if (dot > 0) {
      el.setSelectionRange(0, dot)
    } else {
      el.select()
    }
  }, [initialValue])

  const commit = () => {
    const name = value.trim()
    if (!name) {
      onCancel()
      return
    }
    onSubmit(name)
  }

  return (
    <input
      ref={ref}
      data-testid={testId}
      type="text"
      value={value}
      placeholder={placeholder}
      onChange={(e) => setValue(e.target.value)}
      onKeyDown={(e) => {
        e.stopPropagation()
        if (e.key === 'Enter') {
          e.preventDefault()
          commit()
        } else if (e.key === 'Escape') {
          e.preventDefault()
          onCancel()
        }
      }}
      onBlur={() => {
        // Delay so a click on cancel elsewhere still works; non-empty commits.
        window.setTimeout(() => {
          if (document.activeElement === ref.current) return
          const name = value.trim()
          if (name) onSubmit(name)
          else onCancel()
        }, 0)
      }}
      onClick={(e) => e.stopPropagation()}
      className="w-full min-w-0 rounded border border-shell-accent bg-shell-bg px-1 py-0.5 text-[12px] text-shell-text outline-none"
    />
  )
}
