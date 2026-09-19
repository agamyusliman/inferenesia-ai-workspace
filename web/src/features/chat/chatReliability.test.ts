import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  canSendAfterCancel,
  isRunStale,
  modelPickerDisabledWhileStreaming,
  nextGenerationToken,
  postCancelComposerState,
  shouldRunAutoContinue,
  stickyFromActive,
  stickyRequestFields,
  workspaceSwitchChatReset,
} from './chatReliability'

describe('sticky model/profile (VAL-CHATBUG-001/002)', () => {
  it('builds sticky snapshot and request fields from active session', () => {
    const s = stickyFromActive({
      profile: 'tempai',
      model: 'grok-4',
      profile_name: 'temp-ai',
    })
    assert.equal(s.profile, 'tempai')
    assert.equal(s.model, 'grok-4')
    assert.ok(s.label?.includes('grok-4'))
    assert.deepEqual(stickyRequestFields(s), {
      profile: 'tempai',
      model: 'grok-4',
    })
  })

  it('omits empty fields so core can fall back to session sticky', () => {
    assert.deepEqual(stickyRequestFields({}), {})
    assert.deepEqual(stickyRequestFields(null), {})
    assert.deepEqual(stickyRequestFields({ profile: 'byok-a' }), {
      profile: 'byok-a',
    })
  })

  it('disables model picker while streaming', () => {
    assert.equal(modelPickerDisabledWhileStreaming(true), true)
    assert.equal(modelPickerDisabledWhileStreaming(false), false)
  })
})

describe('cancel unstick generation token (VAL-CHATBUG-003..005)', () => {
  it('bumps token and marks prior run stale', () => {
    const t1 = nextGenerationToken(0)
    const t2 = nextGenerationToken(t1)
    assert.equal(isRunStale(t1, t2), true)
    assert.equal(isRunStale(t2, t2), false)
  })

  it('clears busy immediately on cancel policy', () => {
    const s = postCancelComposerState()
    assert.equal(s.busy, false)
    assert.equal(s.stopContinue, true)
    assert.equal(canSendAfterCancel(s.busy), true)
    assert.equal(canSendAfterCancel(true), false)
  })

  it('stops auto-continue chain on stop / stale / error / image-gen', () => {
    assert.equal(
      shouldRunAutoContinue({
        stopContinue: true,
        stopped: false,
        error: false,
        wantImageGen: false,
        stale: false,
      }),
      false,
    )
    assert.equal(
      shouldRunAutoContinue({
        stopContinue: false,
        stopped: true,
        error: false,
        wantImageGen: false,
        stale: false,
      }),
      false,
    )
    assert.equal(
      shouldRunAutoContinue({
        stopContinue: false,
        stopped: false,
        error: false,
        wantImageGen: false,
        stale: true,
      }),
      false,
    )
    assert.equal(
      shouldRunAutoContinue({
        stopContinue: false,
        stopped: false,
        error: false,
        wantImageGen: false,
        stale: false,
      }),
      true,
    )
  })
})

describe('workspace switch after cancel (VAL-CHATBUG-008)', () => {
  it('clears busy lock without requesting backend stop', () => {
    const r = workspaceSwitchChatReset(3)
    assert.equal(r.busy, false)
    assert.equal(r.stopContinue, false)
    assert.equal(r.streamingAssistantId, null)
    assert.equal(isRunStale(3, r.nextToken), true)
  })
})
