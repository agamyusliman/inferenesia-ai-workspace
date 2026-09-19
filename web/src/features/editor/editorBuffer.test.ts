import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  fromDisk,
  isDirty,
  markSaved,
  withContent,
} from './editorBuffer'

describe('editorBuffer dirty helpers (VAL-IDE-016/017)', () => {
  it('fresh from disk is not dirty', () => {
    const buf = fromDisk('hello\n')
    assert.equal(isDirty(buf), false)
    assert.equal(buf.content, 'hello\n')
  })

  it('typing makes dirty; save clears dirty', () => {
    let buf = fromDisk('hello\n')
    buf = withContent(buf, 'hello world\n')
    assert.equal(isDirty(buf), true)
    buf = markSaved(buf)
    assert.equal(isDirty(buf), false)
    assert.equal(buf.lastSavedContent, 'hello world\n')
  })

  it('reverting content to last saved clears dirty without save', () => {
    let buf = fromDisk('a')
    buf = withContent(buf, 'b')
    assert.equal(isDirty(buf), true)
    buf = withContent(buf, 'a')
    assert.equal(isDirty(buf), false)
  })
})
