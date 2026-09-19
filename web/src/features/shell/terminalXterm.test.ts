import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  buildXtermOptions,
  isInterruptSequence,
  usesInteractiveSurfaceInput,
} from './terminalXterm.ts'

describe('terminalXterm (VAL-IDE-031/032)', () => {
  it('uses interactive surface input (not chat-style command box)', () => {
    assert.equal(usesInteractiveSurfaceInput(), true)
  })

  it('builds xterm options with convertEol and cursor enabled', () => {
    const opts = buildXtermOptions()
    assert.equal(opts.convertEol, true)
    assert.equal(opts.cursorBlink, true)
    assert.equal(opts.disableStdin, false)
    assert.ok(opts.fontSize >= 10)
    assert.equal(opts.theme.background, '#0d1117')
  })

  it('detects Ctrl+C interrupt sequence for PTY SIGINT', () => {
    assert.equal(isInterruptSequence('\u0003'), true)
    assert.equal(isInterruptSequence('c'), false)
    assert.equal(isInterruptSequence('\n'), false)
  })
})
