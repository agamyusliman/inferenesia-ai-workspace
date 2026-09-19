import {
  useCallback,
  useEffect,
  useRef,
  type PointerEvent as ReactPointerEvent,
} from 'react'

type Props = {
  testId: string
  value: number
  onChange: (next: number) => void
  /**
   * Which side grows when the pointer moves down.
   * - `bottom` (default): downward drag increases height of bottom panel
   * - `top`: downward drag decreases bottom panel (top grows)
   */
  growSide?: 'top' | 'bottom'
  minOpposite?: number
  'aria-label'?: string
}

/** Horizontal (row) splitter for top/bottom panel stacks. */
export function HorizontalSplitter({
  testId,
  value,
  onChange,
  growSide = 'bottom',
  minOpposite = 120,
  'aria-label': ariaLabel = 'Resize panel',
}: Props) {
  const valueRef = useRef(value)
  valueRef.current = value
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange
  const growSideRef = useRef(growSide)
  growSideRef.current = growSide
  const minOppositeRef = useRef(minOpposite)
  minOppositeRef.current = minOpposite
  const dragRef = useRef<{ startY: number; startValue: number } | null>(null)

  const onDocMove = useCallback((e: PointerEvent) => {
    if (!dragRef.current) return
    e.preventDefault()
    const delta = e.clientY - dragRef.current.startY
    const signed = growSideRef.current === 'top' ? -delta : delta
    const maxByViewport =
      typeof window !== 'undefined'
        ? Math.max(80, window.innerHeight - minOppositeRef.current - 8)
        : dragRef.current.startValue + signed
    const next = Math.min(
      maxByViewport,
      Math.max(80, dragRef.current.startValue + signed),
    )
    onChangeRef.current(next)
  }, [])

  const onDocUp = useCallback(() => {
    if (!dragRef.current) return
    dragRef.current = null
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    document.removeEventListener('pointermove', onDocMove)
    document.removeEventListener('pointerup', onDocUp)
    document.removeEventListener('pointercancel', onDocUp)
  }, [onDocMove])

  useEffect(() => {
    return () => {
      dragRef.current = null
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      document.removeEventListener('pointermove', onDocMove)
      document.removeEventListener('pointerup', onDocUp)
      document.removeEventListener('pointercancel', onDocUp)
    }
  }, [onDocMove, onDocUp])

  const onPointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      e.preventDefault()
      e.stopPropagation()
      dragRef.current = {
        startY: e.clientY,
        startValue: valueRef.current,
      }
      document.body.style.cursor = 'row-resize'
      document.body.style.userSelect = 'none'
      document.addEventListener('pointermove', onDocMove)
      document.addEventListener('pointerup', onDocUp)
      document.addEventListener('pointercancel', onDocUp)
    },
    [onDocMove, onDocUp],
  )

  return (
    <div
      role="separator"
      aria-orientation="horizontal"
      aria-label={ariaLabel}
      aria-valuenow={Math.round(value)}
      data-testid={testId}
      onPointerDown={onPointerDown}
      className="group relative z-10 h-1 w-full shrink-0 cursor-row-resize touch-none select-none bg-shell-border transition-colors hover:bg-shell-accent active:bg-shell-accent"
    >
      <div className="absolute inset-x-0 -top-1.5 -bottom-1.5" aria-hidden />
    </div>
  )
}
