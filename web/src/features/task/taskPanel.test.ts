/**
 * Pure helpers for TaskPanel elapsed/status display (VAL-ORCH-010).
 * Panel is a subscriber only — no agent loop.
 */

export function formatElapsedMs(ms: number): string {
  if (!ms || ms < 0) return '0s'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return `${m}m ${rem}s`
}

export function parseTaskEventDetail(detail: string): {
  taskId: string
  category: string
  status: string
  label: string
} {
  // task_id|category|status|label
  const parts = (detail || '').split('|')
  return {
    taskId: parts[0] || '',
    category: parts[1] || '',
    status: parts[2] || '',
    label: parts.slice(3).join('|') || '',
  }
}

if (import.meta.vitest) {
  const { describe, it, expect } = import.meta.vitest

  describe('formatElapsedMs', () => {
    it('formats seconds', () => {
      expect(formatElapsedMs(0)).toBe('0s')
      expect(formatElapsedMs(1500)).toBe('1s')
      expect(formatElapsedMs(65_000)).toBe('1m 5s')
    })
  })

  describe('parseTaskEventDetail', () => {
    it('parses TaskSpawned/TaskDone detail pipe format', () => {
      const p = parseTaskEventDetail('task-abc|explore|running|find auth')
      expect(p.taskId).toBe('task-abc')
      expect(p.category).toBe('explore')
      expect(p.status).toBe('running')
      expect(p.label).toBe('find auth')
    })
  })
}
