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
   * Which side grows when the pointer moves right.
   * - `left` (default): rightward drag increases `value` (left panel grows)
   * - `right`: leftward drag increases `value` (right panel grows)
   */
  growSide?: 'left' | 'right'
  /** Minimum remaining space for the opposite side of the split. */
  minOpposite?: number
  'aria-label'?: string
}

/**
 * Draggable vertical splitter. Document-level pointer listeners so drag continues
 * when the cursor leaves the thin handle (and works with agent-browser mouse).
 */
export function VerticalSplitter({
  testId,
  value,
  onChange,
  growSide = 'left',
  minOpposite = 280,
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
  const dragRef = useRef<{ startX: number; startValue: number } | null>(null)

  const onDocMove = useCallback((e: PointerEvent) => {
    if (!dragRef.current) return
    e.preventDefault()
    const delta = e.clientX - dragRef.current.startX
    // left grow: +delta widens left panel; right grow: -delta widens right panel
    const signed =
      growSideRef.current === 'right' ? -delta : delta
    const maxByViewport =
      typeof window !== 'undefined'
        ? Math.max(0, window.innerWidth - minOppositeRef.current - 8)
        : dragRef.current.startValue + signed
    const next = Math.min(maxByViewport, dragRef.current.startValue + signed)
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
        startX: e.clientX,
        startValue: valueRef.current,
      }
      document.body.style.cursor = 'col-resize'
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
      aria-orientation="vertical"
      aria-label={ariaLabel}
      aria-valuenow={Math.round(value)}
      data-testid={testId}
      onPointerDown={onPointerDown}
      className="group relative z-10 w-1 shrink-0 cursor-col-resize touch-none select-none bg-shell-border transition-colors hover:bg-shell-accent active:bg-shell-accent"
    >
      <div className="absolute inset-y-0 -left-1.5 -right-1.5" aria-hidden />
    </div>
  )
}
