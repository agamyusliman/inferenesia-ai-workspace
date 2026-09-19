import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

/**
 * Pure helpers for chat header model label (VAL-PROV-005/011).
 * Kept free of React so node:test can run without a DOM.
 */
export function formatActiveLabel(profile?: string, model?: string): string {
  const p = (profile || '').trim()
  const m = (model || '').trim()
  if (!p && !m) return ''
  if (!m) return p
  if (!p) return m
  return `${p} · ${m}`
}

export function formatMessageRoute(
  profile?: string,
  model?: string,
): string {
  const p = (profile || '').trim() || 'provider'
  const m = (model || '').trim()
  return m ? `· ${p} / ${m}` : `· ${p}`
}

describe('formatActiveLabel', () => {
  it('joins profile and model for header', () => {
    assert.equal(formatActiveLabel('tempai', 'grok-4'), 'tempai · grok-4')
  })
  it('handles model-only and empty', () => {
    assert.equal(formatActiveLabel('', 'm1'), 'm1')
    assert.equal(formatActiveLabel('tempai', ''), 'tempai')
    assert.equal(formatActiveLabel(), '')
  })
})

describe('formatMessageRoute', () => {
  it('shows profile and model on assistant message', () => {
    assert.equal(formatMessageRoute('byok-a', 'model-a1'), '· byok-a / model-a1')
  })
  it('falls back to provider when profile missing', () => {
    assert.equal(formatMessageRoute('', 'm'), '· provider / m')
  })
})
