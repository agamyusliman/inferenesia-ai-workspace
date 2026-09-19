import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  isNearBottom,
  shouldAutoScroll,
  scrollTopToBottom,
  updateScrollLock,
} from './chatSmartScroll'

describe('smart scroll (VAL-CHAT-029)', () => {
  it('is near bottom within threshold', () => {
    assert.equal(
      isNearBottom({ scrollTop: 920, scrollHeight: 1000, clientHeight: 100 }),
      true,
    )
    assert.equal(
      isNearBottom({ scrollTop: 100, scrollHeight: 1000, clientHeight: 100 }),
      false,
    )
  })

  it('locks when scrolled up; unlocks at bottom', () => {
    assert.equal(
      updateScrollLock({ scrollTop: 10, scrollHeight: 1000, clientHeight: 200 }, false),
      true,
    )
    assert.equal(
      updateScrollLock({ scrollTop: 800, scrollHeight: 1000, clientHeight: 200 }, true),
      false,
    )
  })

  it('auto-scroll only when unlocked', () => {
    assert.equal(shouldAutoScroll(false), true)
    assert.equal(shouldAutoScroll(true), false)
  })

  it('computes scrollTop to bottom', () => {
    assert.equal(scrollTopToBottom({ scrollHeight: 500, clientHeight: 120 }), 380)
  })
})
