import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  STOPPED_BY_USER_MARKER,
  STREAM_STATUS,
  applyStoppedByUser,
  assistantStatusLabel,
  composerKeyAction,
  shouldSendOnEnter,
} from './chatStreamUx'

describe('applyStoppedByUser', () => {
  it('uses marker alone when content empty', () => {
    assert.equal(applyStoppedByUser(''), STOPPED_BY_USER_MARKER)
    assert.equal(applyStoppedByUser(null), STOPPED_BY_USER_MARKER)
    assert.equal(applyStoppedByUser(undefined), STOPPED_BY_USER_MARKER)
  })

  it('appends marker after partial tokens without duplicating', () => {
    const once = applyStoppedByUser('Hello')
    assert.equal(once, `Hello\n\n${STOPPED_BY_USER_MARKER}`)
    assert.equal(applyStoppedByUser(once), once)
  })
})

describe('assistantStatusLabel', () => {
  it('shows Generating… while streaming', () => {
    assert.equal(assistantStatusLabel(true), STREAM_STATUS.generating)
  })
  it('shows stopped after user cancel', () => {
    assert.equal(assistantStatusLabel(false, true), STREAM_STATUS.stopped)
  })
  it('clears status when idle/done', () => {
    assert.equal(assistantStatusLabel(false), '')
  })
})

describe('composerKeyAction (VAL-CHAT-004/006)', () => {
  it('Enter without Shift sends', () => {
    assert.equal(composerKeyAction('Enter', { shiftKey: false }), 'send')
    assert.equal(shouldSendOnEnter('Enter', false), true)
  })
  it('Enter while busy still requests send (interrupt & send in UI)', () => {
    assert.equal(composerKeyAction('Enter', { shiftKey: false, busy: true }), 'send')
  })
  it('Shift+Enter is newline only', () => {
    assert.equal(composerKeyAction('Enter', { shiftKey: true }), 'newline')
    assert.equal(shouldSendOnEnter('Enter', true), false)
  })
  it('Esc stops when busy and picker closed', () => {
    assert.equal(composerKeyAction('Escape', { busy: true, pickerOpen: false }), 'stop')
  })
  it('Esc does not stop when picker open (picker closes first)', () => {
    assert.equal(composerKeyAction('Escape', { busy: true, pickerOpen: true }), 'none')
  })
  it('Esc is no-op when idle', () => {
    assert.equal(composerKeyAction('Escape', { busy: false }), 'none')
  })
})
