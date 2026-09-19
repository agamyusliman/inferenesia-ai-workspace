import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  IMAGE_GEN_UNSUPPORTED_MESSAGE,
  MAX_IMAGE_GEN_REFS,
  capImageGenRefs,
  imageGenDegradeContent,
  isImageGenUnsupported,
  promptHasImageSlash,
  shouldRequestImageGen,
  stripImageSlashPrefix,
} from './chatImageGen'

describe('capImageGenRefs (VAL-CHAT-025)', () => {
  it('keeps first max refs', () => {
    const refs = [1, 2, 3, 4, 5, 6]
    assert.deepEqual(capImageGenRefs(refs), [1, 2, 3, 4])
    assert.equal(MAX_IMAGE_GEN_REFS, 4)
  })
  it('handles empty/null', () => {
    assert.deepEqual(capImageGenRefs(null), [])
    assert.deepEqual(capImageGenRefs(undefined), [])
    assert.deepEqual(capImageGenRefs([]), [])
  })
})

describe('shouldRequestImageGen', () => {
  it('requires toggle and non-empty prompt', () => {
    assert.equal(shouldRequestImageGen(true, 'draw a cat'), true)
    assert.equal(shouldRequestImageGen(false, 'draw a cat'), false)
    assert.equal(shouldRequestImageGen(true, '  '), false)
    assert.equal(shouldRequestImageGen(true, ''), false)
  })
  it('one-shot /image does not need toggle', () => {
    assert.equal(shouldRequestImageGen(false, 'logo for app', true), true)
    assert.equal(shouldRequestImageGen(false, '/image logo for app', true), true)
    assert.equal(shouldRequestImageGen(false, '/image', true), false)
    assert.equal(promptHasImageSlash('/image logo'), true)
    assert.equal(stripImageSlashPrefix('/image logo for app'), 'logo for app')
    assert.equal(stripImageSlashPrefix('normal text'), 'normal text')
  })
})

describe('isImageGenUnsupported / degrade', () => {
  it('detects clear unsupported messages', () => {
    assert.equal(isImageGenUnsupported('Image generation is not supported by provider'), true)
    assert.equal(isImageGenUnsupported('image_generation unsupported for this model'), true)
    assert.equal(isImageGenUnsupported('network timeout'), false)
  })
  it('returns friendly degrade content', () => {
    assert.equal(
      imageGenDegradeContent('Image generation is not supported'),
      IMAGE_GEN_UNSUPPORTED_MESSAGE,
    )
    assert.ok(imageGenDegradeContent('boom').includes('boom'))
  })
})
