import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  assistantMarkdownFilename,
  deleteMessageById,
  imageDownloadFilename,
  messageActionFlags,
  toPersistableMessages,
  truncateFromMessageId,
  indexOfMessageId,
} from './messageActions'

const sample = [
  { id: 'm-0', role: 'user' as const, content: 'u0' },
  { id: 'm-1', role: 'assistant' as const, content: 'a0' },
  { id: 'm-2', role: 'user' as const, content: 'u1' },
  { id: 'm-3', role: 'assistant' as const, content: 'a1' },
]

describe('truncateFromMessageId (VAL-CHAT-017)', () => {
  it('removes message and all after it', () => {
    const out = truncateFromMessageId(sample, 'm-2')
    assert.deepEqual(
      out.map((m) => m.id),
      ['m-0', 'm-1'],
    )
  })
  it('truncating first leaves empty list', () => {
    assert.equal(truncateFromMessageId(sample, 'm-0').length, 0)
  })
  it('missing id returns copy of original', () => {
    const out = truncateFromMessageId(sample, 'nope')
    assert.equal(out.length, sample.length)
    assert.notEqual(out, sample)
  })
})

describe('deleteMessageById (VAL-CHAT-018)', () => {
  it('removes a single user message', () => {
    const out = deleteMessageById(sample, 'm-2')
    assert.deepEqual(
      out.map((m) => m.id),
      ['m-0', 'm-1', 'm-3'],
    )
  })
  it('removes a single assistant message', () => {
    const out = deleteMessageById(sample, 'm-1')
    assert.deepEqual(
      out.map((m) => m.id),
      ['m-0', 'm-2', 'm-3'],
    )
  })
  it('missing id returns copy', () => {
    assert.equal(deleteMessageById(sample, 'x').length, 4)
  })
})

describe('messageActionFlags (VAL-CHAT-016..020)', () => {
  it('user complete: copy, undo, delete; no md download', () => {
    const f = messageActionFlags({ role: 'user' })
    assert.equal(f.copy, true)
    assert.equal(f.undoFromHere, true)
    assert.equal(f.delete, true)
    assert.equal(f.downloadMd, false)
    assert.equal(f.confirmUndoFromHere, true)
    assert.equal(f.confirmDelete, false)
  })
  it('assistant complete: copy + md download + undo + delete', () => {
    const f = messageActionFlags({ role: 'assistant' })
    assert.equal(f.copy, true)
    assert.equal(f.downloadMd, true)
    assert.equal(f.undoFromHere, true)
    assert.equal(f.delete, true)
  })
  it('assistant with images: preview + download image', () => {
    const f = messageActionFlags({ role: 'assistant', hasImages: true })
    assert.equal(f.previewImage, true)
    assert.equal(f.downloadImage, true)
  })
  it('streaming hides mutating / clipboard actions', () => {
    const f = messageActionFlags({ role: 'assistant', streaming: true, hasImages: true })
    assert.equal(f.copy, false)
    assert.equal(f.delete, false)
    assert.equal(f.undoFromHere, false)
    assert.equal(f.previewImage, false)
  })
})

describe('filenames & persist helpers', () => {
  it('assistant markdown filename uses session + index', () => {
    assert.equal(
      assistantMarkdownFilename('m-3', 'Landing page CV', 2),
      'Inferenesia - Landing page CV - 2.md',
    )
    assert.ok(assistantMarkdownFilename('m-3').startsWith('Inferenesia - '))
  })
  it('image filename from data URL / alt', () => {
    assert.equal(
      imageDownloadFilename('data:image/png;base64,abc', '', 0, 'Portfolio'),
      'Inferenesia - Portfolio - image-1.png',
    )
    assert.equal(
      imageDownloadFilename('x', 'diagram', 1, 'Portfolio'),
      'Inferenesia - Portfolio - diagram.png',
    )
  })
  it('toPersistableMessages drops system', () => {
    const out = toPersistableMessages([
      { id: '1', role: 'system', content: 's' },
      { id: '2', role: 'user', content: 'hi' },
    ])
    assert.equal(out.length, 1)
    assert.equal(out[0].role, 'user')
  })
  it('indexOfMessageId finds index', () => {
    assert.equal(indexOfMessageId(sample, 'm-3'), 3)
  })
})
