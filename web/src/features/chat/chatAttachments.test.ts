import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  MAX_VISION_IMAGES,
  attachmentIconLabel,
  buildAttachmentPayload,
  canSendWithAttachments,
  classifyAttachment,
  composePromptWithAttachments,
  formatBytes,
  pdfAnalysisPolicy,
  removeAttachmentById,
  selectVisionImages,
  truncateInline,
  type ChatAttachment,
} from './chatAttachments'

function att(partial: Partial<ChatAttachment> & Pick<ChatAttachment, 'id' | 'name' | 'kind'>): ChatAttachment {
  return {
    size: 100,
    mime: '',
    ...partial,
  }
}

describe('classifyAttachment (VAL-CHAT-021..024)', () => {
  it('classifies images by mime and extension', () => {
    assert.equal(classifyAttachment({ name: 'a.png', type: 'image/png' }), 'image')
    assert.equal(classifyAttachment({ name: 'photo.JPG' }), 'image')
  })
  it('classifies PDF', () => {
    assert.equal(classifyAttachment({ name: 'doc.pdf', type: 'application/pdf' }), 'pdf')
    assert.equal(classifyAttachment({ name: 'x.PDF' }), 'pdf')
  })
  it('classifies code and text', () => {
    assert.equal(classifyAttachment({ name: 'main.go' }), 'code')
    assert.equal(classifyAttachment({ name: 'notes.txt' }), 'text')
    assert.equal(classifyAttachment({ name: 'readme.md' }), 'code')
  })
  it('classifies unknown binary', () => {
    assert.equal(classifyAttachment({ name: 'blob.bin', type: 'application/octet-stream' }), 'binary')
  })
})

describe('formatBytes / icons', () => {
  it('formats sizes', () => {
    assert.equal(formatBytes(500), '500 B')
    assert.match(formatBytes(2048), /KB/)
  })
  it('icons by kind', () => {
    assert.equal(attachmentIconLabel('image'), 'IMG')
    assert.equal(attachmentIconLabel('pdf'), 'PDF')
  })
})

describe('selectVisionImages (VAL-CHAT-022)', () => {
  it('caps at MAX_VISION_IMAGES', () => {
    const list = Array.from({ length: 6 }, (_, i) =>
      att({
        id: `i${i}`,
        name: `i${i}.png`,
        kind: 'image',
        dataUrl: `data:image/png;base64,AAA${i}`,
      }),
    )
    const sel = selectVisionImages(list)
    assert.equal(sel.length, MAX_VISION_IMAGES)
    assert.equal(MAX_VISION_IMAGES, 4)
  })
  it('skips images without data URL or with errors', () => {
    const list = [
      att({ id: '1', name: 'a.png', kind: 'image', dataUrl: 'data:image/png;base64,AA' }),
      att({ id: '2', name: 'b.png', kind: 'image', error: 'too big' }),
      att({ id: '3', name: 'c.txt', kind: 'text', textContent: 'hi' }),
    ]
    assert.equal(selectVisionImages(list).length, 1)
  })
})

describe('pdfAnalysisPolicy (VAL-CHAT-023)', () => {
  it('states analysis-only and forbids local shell claims', () => {
    const p = pdfAnalysisPolicy('invoice.pdf')
    assert.match(p, /analysis only/i)
    assert.match(p, /pdf\.js/i)
    assert.match(p, /invoice\.pdf/)
    assert.match(p, /Do not claim to open local shell/i)
    assert.doesNotMatch(p, /run this PDF in shell/i)
  })
})

describe('buildAttachmentPayload', () => {
  it('builds vision parts and text inline + bubble chips', () => {
    const payload = buildAttachmentPayload([
      att({
        id: 'img1',
        name: 'shot.png',
        kind: 'image',
        mime: 'image/png',
        size: 1200,
        dataUrl: 'data:image/png;base64,QUFB',
        previewUrl: 'data:image/png;base64,QUFB',
      }),
      att({
        id: 'pdf1',
        name: 'spec.pdf',
        kind: 'pdf',
        size: 4000,
        textContent: 'Hello PDF body',
      }),
      att({
        id: 'code1',
        name: 'util.ts',
        kind: 'code',
        size: 50,
        textContent: 'export const x = 1',
      }),
    ])
    assert.equal(payload.images.length, 1)
    assert.equal(payload.images[0].data_url, 'data:image/png;base64,QUFB')
    assert.match(payload.inlineText, /Hello PDF body/)
    assert.match(payload.inlineText, /analysis only/i)
    assert.match(payload.inlineText, /export const x = 1/)
    assert.match(payload.inlineText, /Attached image: shot\.png/)
    assert.equal(payload.bubbleChips.length, 3)
    assert.equal(payload.bubbleChips[0].kind, 'image')
  })

  it('does not claim shell for PDF path', () => {
    const payload = buildAttachmentPayload([
      att({ id: 'p', name: 'a.pdf', kind: 'pdf', textContent: 'body' }),
    ])
    assert.doesNotMatch(payload.inlineText, /local shell is ready/i)
    assert.match(payload.inlineText, /no local shell is available/i)
  })
})

describe('composePromptWithAttachments', () => {
  it('joins user text and inline bodies', () => {
    const payload = buildAttachmentPayload([
      att({ id: 't', name: 'a.txt', kind: 'text', textContent: 'file body' }),
    ])
    const out = composePromptWithAttachments('Please review', payload)
    assert.match(out, /^Please review/)
    assert.match(out, /file body/)
  })
  it('works with attachment-only send', () => {
    const payload = buildAttachmentPayload([
      att({ id: 't', name: 'a.txt', kind: 'text', textContent: 'only' }),
    ])
    assert.match(composePromptWithAttachments('', payload), /only/)
  })
})

describe('canSendWithAttachments / remove', () => {
  it('blocks while loading', () => {
    assert.equal(
      canSendWithAttachments('', [att({ id: '1', name: 'a', kind: 'image', loading: true })], []),
      false,
    )
  })
  it('allows when images ready without text', () => {
    assert.equal(
      canSendWithAttachments(
        '',
        [att({ id: '1', name: 'a.png', kind: 'image', dataUrl: 'data:image/png;base64,AA' })],
        [],
      ),
      true,
    )
  })
  it('removes by id', () => {
    const list = [
      att({ id: 'a', name: 'a', kind: 'text' }),
      att({ id: 'b', name: 'b', kind: 'text' }),
    ]
    assert.deepEqual(
      removeAttachmentById(list, 'a').map((x) => x.id),
      ['b'],
    )
  })
})

describe('truncateInline', () => {
  it('truncates long text', () => {
    const long = 'x'.repeat(100)
    const out = truncateInline(long, 50)
    assert.equal(out.length > 50, true)
    assert.match(out, /truncated/)
  })
})
