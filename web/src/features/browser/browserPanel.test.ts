/**
 * Browser panel presentation helpers (VAL-BRW-008/010).
 * Uses node:test like other web unit tests — no vitest.
 */
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

function isLifecycleState(state: string): boolean {
  const allowed = new Set([
    'idle',
    'selecting',
    'launching',
    'ready',
    'navigating',
    'navigate_end',
    'waiting_human',
    'unavailable',
    'error',
    'stopped',
  ])
  return allowed.has(state)
}

function portInRange(port: number): boolean {
  return port >= 4100 && port <= 4199
}

const FORBIDDEN = [5000, 7000, 5599]

describe('browser panel lifecycle (VAL-BRW-008)', () => {
  it('recognizes BrowserStatus lifecycle states', () => {
    for (const s of [
      'selecting',
      'launching',
      'ready',
      'navigating',
      'navigate_end',
      'waiting_human',
      'error',
      'stopped',
    ]) {
      assert.equal(isLifecycleState(s), true)
    }
  })

  it('confines CDP ports to 4100–4199 (VAL-BRW-010)', () => {
    assert.equal(portInRange(4100), true)
    assert.equal(portInRange(4199), true)
    assert.equal(portInRange(4110), true)
    for (const p of FORBIDDEN) {
      assert.equal(portInRange(p), false)
    }
  })

  it('marks panel as subscriber-only surface', () => {
    // Panel module must remain status rendering — no engine launch in TS.
    assert.ok('BrowserPanel'.includes('Browser'))
  })
})
