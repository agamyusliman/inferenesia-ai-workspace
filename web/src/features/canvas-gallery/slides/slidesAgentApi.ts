export type SlideEditInstruction = {
  text: string
  slideId?: string
  intent?: 'edit-active' | 'fill-blank' | 'new-after' | 'rebuild-deck'
}

type Dispatcher = (instruction: SlideEditInstruction) => void

const dispatchers = new Map<string, Dispatcher>()

export function registerSlideDispatcher(
  canvasKey: string,
  dispatch: Dispatcher,
): void {
  dispatchers.set(canvasKey, dispatch)
}

export function clearSlideDispatcher(canvasKey: string): void {
  dispatchers.delete(canvasKey)
}

export function sendSlideInstruction(
  canvasKey: string,
  instruction: SlideEditInstruction,
): boolean {
  const dispatch = dispatchers.get(canvasKey)
  if (!dispatch) return false
  dispatch(instruction)
  return true
}

export function isSlideDispatcherReady(canvasKey: string): boolean {
  return dispatchers.has(canvasKey)
}
