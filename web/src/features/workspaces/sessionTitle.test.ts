import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  deriveSessionTitle,
  isPlaceholderSessionName,
} from './sessionTitle'

describe('deriveSessionTitle', () => {
  it('returns empty for blank', () => {
    assert.equal(deriveSessionTitle(''), '')
    assert.equal(deriveSessionTitle('   '), '')
  })

  it('takes first sentence and trims', () => {
    assert.equal(
      deriveSessionTitle('Improve CV landing page with Tailwind. Add canvas.'),
      'Improve CV landing page with Tailwind',
    )
  })

  it('truncates long prompts with ellipsis', () => {
    const long =
      'please build a very long multi word portfolio landing page with many sections and animations'
    const t = deriveSessionTitle(long, 40)
    assert.ok(t.endsWith('…'))
    assert.ok(t.length <= 41)
  })

  it('strips slash image prefix', () => {
    assert.equal(deriveSessionTitle('/image a cat in space'), 'a cat in space')
  })
})

describe('isPlaceholderSessionName', () => {
  it('detects defaults', () => {
    assert.equal(isPlaceholderSessionName('New chat'), true)
    assert.equal(isPlaceholderSessionName('Session 15:04'), true)
    assert.equal(isPlaceholderSessionName('Portfolio landing'), false)
  })
})
