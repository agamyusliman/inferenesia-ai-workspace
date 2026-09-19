import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  CONTINUING_STATUS_LABEL,
  EPHEMERAL_CONTINUE_PROMPT,
  MAX_AUTO_CONTINUE_ROUNDS,
  canAutoContinue,
  ephemeralContinueRequest,
  looksLikeFakeToolOrShell,
  needsAutoContinue,
} from './chatAutoContinue'

describe('needsAutoContinue (VAL-CHAT-026)', () => {
  it('flags unfinished ellipsis and dangling connectors', () => {
    assert.equal(needsAutoContinue('Here is the plan…'), true)
    assert.equal(needsAutoContinue('I will implement the fix and'), true)
  })

  it('flags unclosed code fences', () => {
    assert.equal(needsAutoContinue('```ts\nconst x = 1'), true)
    assert.equal(needsAutoContinue('```ts\nconst x = 1\n```'), false)
  })

  it('flags fake tool / shell patterns', () => {
    assert.equal(
      needsAutoContinue('I will call the tool now:\n<tool_call name="run">'),
      true,
    )
    assert.equal(looksLikeFakeToolOrShell('<tool_call>shell</tool_call>'), true)
  })

  it('does not continue stopped or error answers', () => {
    assert.equal(needsAutoContinue('Hello\n\n_(stopped by user)_'), false)
    assert.equal(needsAutoContinue('(error) boom'), false)
  })

  it('does not continue complete prose', () => {
    assert.equal(
      needsAutoContinue('All tests passed. The feature is complete.'),
      false,
    )
  })
})

describe('canAutoContinue', () => {
  it('respects max rounds and stop/error flags', () => {
    assert.equal(canAutoContinue(0, 'more to come…'), true)
    assert.equal(canAutoContinue(MAX_AUTO_CONTINUE_ROUNDS, 'more to come…'), false)
    assert.equal(canAutoContinue(0, 'more to come…', { stopped: true }), false)
    assert.equal(canAutoContinue(0, 'more to come…', { error: true }), false)
  })
})

describe('ephemeralContinueRequest', () => {
  it('marks ephemeral continue payload (not a user history row)', () => {
    const req = ephemeralContinueRequest({ model: 'm1', profile: 'tempai' })
    assert.equal(req.ephemeral, true)
    assert.equal(req.continue, true)
    assert.equal(req.prompt, EPHEMERAL_CONTINUE_PROMPT)
    assert.equal(req.model, 'm1')
    assert.equal(req.profile, 'tempai')
    assert.ok(CONTINUING_STATUS_LABEL.includes('continuing'))
  })
})
