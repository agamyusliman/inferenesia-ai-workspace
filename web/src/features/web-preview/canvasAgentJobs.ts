export type CanvasAgentJob = {
  canvasKey: string
  busy: boolean
  status: string | null
  error: string | null
}

type Listener = () => void

const jobs = new Map<string, CanvasAgentJob>()
const idleByKey = new Map<string, CanvasAgentJob>()
const listeners = new Set<Listener>()

function emit() {
  for (const l of listeners) l()
}

function idleJob(canvasKey: string): CanvasAgentJob {
  let idle = idleByKey.get(canvasKey)
  if (!idle) {
    idle = {
      canvasKey,
      busy: false,
      status: null,
      error: null,
    }
    idleByKey.set(canvasKey, idle)
  }
  return idle
}

export function subscribeCanvasAgentJobs(listener: Listener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function getCanvasAgentJob(canvasKey: string): CanvasAgentJob {
  return jobs.get(canvasKey) || idleJob(canvasKey)
}

export function setCanvasAgentJob(
  canvasKey: string,
  patch: Partial<Omit<CanvasAgentJob, 'canvasKey'>>,
): void {
  if (!canvasKey) return
  const prev = getCanvasAgentJob(canvasKey)
  const next: CanvasAgentJob = {
    canvasKey,
    busy: patch.busy ?? prev.busy,
    status: patch.status !== undefined ? patch.status : prev.status,
    error: patch.error !== undefined ? patch.error : prev.error,
  }
  if (!next.busy && !next.status && !next.error) {
    jobs.delete(canvasKey)
  } else {
    jobs.set(canvasKey, next)
  }
  emit()
}

export function clearCanvasAgentJob(canvasKey: string): void {
  if (!jobs.has(canvasKey)) return
  jobs.delete(canvasKey)
  emit()
}
