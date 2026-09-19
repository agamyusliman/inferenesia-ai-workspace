import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  blankSlideHtml,
  deleteSlideFromHtml,
  duplicateSlideInHtml,
  emptyHtmlDeck,
  ensureAiImagePlaceholderOnSlide,
  existingSlotLayoutClass,
  forcePlaceImageInSlide,
  insertBlankSlide,
  listAiImageSlots,
  listSlidesFromHtml,
  previewSrcDoc,
  reorderSlidesInHtml,
  resolveSlidesAgentReply,
  slidesDesignSystemPrompt,
} from './htmlDeck'

describe('htmlDeck', () => {
  it('seeds one blank slide', () => {
    const html = emptyHtmlDeck('Demo')
    const slides = listSlidesFromHtml(html)
    assert.equal(slides.length, 1)
    assert.ok(slides[0].outerHtml.includes('slide-blank'))
    assert.ok(html.includes('inferenesia-deck'))
    assert.ok(html.includes('1280'))
  })

  it('inserts blank after active', () => {
    const html = emptyHtmlDeck('Demo')
    const first = listSlidesFromHtml(html)[0]
    const { html: next, slideId } = insertBlankSlide(html, first.id)
    const slides = listSlidesFromHtml(next)
    assert.equal(slides.length, 2)
    assert.ok(slides.some((s) => s.id === slideId))
  })

  it('refuses deleting last slide', () => {
    const html = emptyHtmlDeck('Demo')
    const id = listSlidesFromHtml(html)[0].id
    assert.equal(deleteSlideFromHtml(html, id), null)
  })

  it('force-places image on blank slide', () => {
    const html = emptyHtmlDeck('Demo')
    const id = listSlidesFromHtml(html)[0].id
    const next = forcePlaceImageInSlide(
      html,
      id,
      { id: 'att-1', src: 'data:image/png;base64,xx', alt: 'x' },
      'right',
    )
    assert.ok(next)
    assert.ok(next!.includes('data-asset-id="att-1"'))
    assert.ok(next!.includes('data-image-slot="right"'))
  })

  it('preview srcdoc contains active slide', () => {
    const html = emptyHtmlDeck('Demo')
    const id = listSlidesFromHtml(html)[0].id
    const doc = previewSrcDoc(html, id)
    assert.ok(doc.includes(id))
    assert.ok(doc.includes(blankSlideHtml(id).slice(0, 40)) || doc.includes('slide-blank'))
  })

  it('reorders slides without duplicating count', () => {
    let html = emptyHtmlDeck('Demo')
    const a = listSlidesFromHtml(html)[0]
    const b = insertBlankSlide(html, a.id)
    html = b.html
    const c = insertBlankSlide(html, b.slideId)
    html = c.html
    const before = listSlidesFromHtml(html)
    assert.equal(before.length, 3)
    const ids = before.map((s) => s.id)
    const next = reorderSlidesInHtml(html, [...ids].reverse())
    assert.ok(next)
    const after = listSlidesFromHtml(next!)
    assert.equal(after.length, 3)
    assert.deepEqual(
      after.map((s) => s.id),
      [...ids].reverse(),
    )
    const next2 = reorderSlidesInHtml(next!, ids)
    assert.ok(next2)
    assert.equal(listSlidesFromHtml(next2!).length, 3)
  })

  it('deletes middle slide and count drops by one', () => {
    let html = emptyHtmlDeck('Demo')
    const a = listSlidesFromHtml(html)[0]
    const b = insertBlankSlide(html, a.id)
    html = b.html
    const c = insertBlankSlide(html, b.slideId)
    html = c.html
    const mid = listSlidesFromHtml(html)[1]
    const next = deleteSlideFromHtml(html, mid.id)
    assert.ok(next)
    const after = listSlidesFromHtml(next!)
    assert.equal(after.length, 2)
    assert.equal(after.some((s) => s.id === mid.id), false)
  })

  it('reorder returns null when ids mismatch (no partial splice)', () => {
    const html = emptyHtmlDeck('Demo')
    const id = listSlidesFromHtml(html)[0].id
    assert.equal(reorderSlidesInHtml(html, [id, 'missing']), null)
  })

  it('edit-active applies one section when model returns multi-slide', async () => {
    let html = emptyHtmlDeck('Demo')
    const a = listSlidesFromHtml(html)[0]
    const b = insertBlankSlide(html, a.id)
    html = b.html
    const active = listSlidesFromHtml(html)[0]
    const reply = [
      '```html',
      `<section class="slide" data-slide-id="wrong" data-layout="grid" data-title="A"><h1>One</h1></section>`,
      `<section class="slide" data-slide-id="${active.id}" data-layout="grid" data-title="Fixed"><h1>Fixed grid</h1></section>`,
      '```',
    ].join('\n')
    const resolved = resolveSlidesAgentReply(html, reply, {
      intent: 'edit-active',
      activeSlideId: active.id,
    })
    assert.equal(resolved.ok, true)
    if (resolved.ok) {
      assert.equal(listSlidesFromHtml(resolved.next).length, 2)
      assert.ok(resolved.next.includes('Fixed grid'))
      assert.equal(resolved.mode, 'slide')
    }
  })

  it('rejects a patch that changes an unselected slide', () => {
    let html = emptyHtmlDeck('Demo')
    const first = listSlidesFromHtml(html)[0]
    html = insertBlankSlide(html, first.id).html
    const [active, unselected] = listSlidesFromHtml(html)
    const reply = [
      '<<<<<<< SEARCH',
      active.outerHtml,
      '=======',
      active.outerHtml.replace('Blank slide', 'Edited slide'),
      '>>>>>>> REPLACE',
      '<<<<<<< SEARCH',
      unselected.outerHtml,
      '=======',
      unselected.outerHtml.replace('Blank slide', 'Leaked edit'),
      '>>>>>>> REPLACE',
    ].join('\n')
    const resolved = resolveSlidesAgentReply(html, reply, {
      intent: 'edit-active',
      activeSlideId: active.id,
    })
    assert.equal(resolved.ok, false)
    if (!resolved.ok) assert.match(resolved.error, /unselected slide/i)
  })

  it('duplicates a slide with unique slide and export ids', () => {
    const html = emptyHtmlDeck('Demo').replace(
      '<div class="hint">',
      '<div id="headline" data-ex="title" data-ex-id="headline">',
    )
    const source = listSlidesFromHtml(html)[0]
    const result = duplicateSlideInHtml(html, source.id)
    assert.ok(result)
    const slides = listSlidesFromHtml(result!.html)
    assert.equal(slides.length, 2)
    assert.notEqual(result!.slideId, source.id)
    assert.match(slides[1].outerHtml, new RegExp(`data-ex-id="${result!.slideId}-headline"`))
  })
})

describe('image slot geometry', () => {
  const deckWith = (slotDiv: string) =>
    `<!DOCTYPE html><html><head></head><body><div class="inferenesia-deck"><section class="slide" data-slide-id="s1" data-layout="two-column" data-title="Unit economics hold at scale"><h2>Unit economics hold at scale</h2><p>Gross margin improved for six consecutive quarters.</p>${slotDiv}</section></div></body></html>`

  it('reads authored geometry utilities and drops placeholder paint', () => {
    const html = deckWith(
      '<div data-image-slot="right" class="w-[520px] h-[300px] shrink-0 rounded-2xl border border-dashed bg-slate-900/30 text-slate-400 flex items-center justify-center">Image slot</div>',
    )
    const slide = listSlidesFromHtml(html)[0]
    const kept = existingSlotLayoutClass(slide.outerHtml, 'right')
    assert.ok(kept)
    assert.match(kept!, /w-\[520px\]/)
    assert.match(kept!, /h-\[300px\]/)
    assert.match(kept!, /shrink-0/)
    assert.equal(/border-dashed/.test(kept!), false)
    assert.equal(/bg-slate-900/.test(kept!), false)
  })

  it('returns null when the slot has no geometry classes', () => {
    const html = deckWith('<div data-image-slot="right" class="rounded-2xl border">x</div>')
    const slide = listSlidesFromHtml(html)[0]
    assert.equal(existingSlotLayoutClass(slide.outerHtml, 'right'), null)
  })

  it('keeps authored size through placeholder and final image', () => {
    const html = deckWith(
      '<div data-image-slot="right" class="w-[520px] h-[300px] shrink-0 rounded-2xl border border-dashed">Image slot</div>',
    )
    const pending = ensureAiImagePlaceholderOnSlide(html, 's1', 'a scene', 'right')
    assert.match(pending, /w-\[520px\]/)
    assert.match(pending, /h-\[300px\]/)
    assert.equal(/w-\[380px\]/.test(pending), false)

    const placed = forcePlaceImageInSlide(
      pending,
      's1',
      { id: 'a1', src: 'data:image/png;base64,xx', alt: '' },
      'right',
    )
    assert.ok(placed)
    assert.match(placed!, /w-\[520px\]/)
    assert.match(placed!, /h-\[300px\]/)
    assert.equal(
      /w-\[380px\]/.test(placed!),
      false,
      'the slot must not snap back to the default template size',
    )
  })

  it('still falls back to a default size when nothing is authored', () => {
    const html = emptyHtmlDeck('Demo')
    const id = listSlidesFromHtml(html)[0].id
    const pending = ensureAiImagePlaceholderOnSlide(html, id, 'a scene', 'right')
    assert.match(pending, /w-\[380px\]/)
    assert.match(pending, /data-image-slot="right"/)
  })

  it('keeps the slot discoverable after each lifecycle step', () => {
    const html = deckWith(
      '<div data-image-slot="right" class="w-[520px] h-[300px] shrink-0">Image slot</div>',
    )
    const pending = ensureAiImagePlaceholderOnSlide(html, 's1', '', 'right')
    const slots = listAiImageSlots(pending)
    assert.equal(slots.length, 1)
    assert.equal(slots[0].slot, 'right')
    assert.equal(slots[0].slideId, 's1')
  })
})

describe('ai image prompts', () => {
  it('derives the brief from slide content, not the title alone', () => {
    const html = `<!DOCTYPE html><html><head></head><body><div class="inferenesia-deck"><section class="slide" data-slide-id="s1" data-layout="two-column" data-title="Cold chain coverage"><h2>Cold chain coverage</h2><p>Refrigerated depots now reach eleven islands.</p><div data-image-slot="right" class="w-[380px] h-[260px]">Image slot</div></section></div></body></html>`
    const [slot] = listAiImageSlots(html)
    assert.ok(slot)
    assert.match(slot.prompt, /Cold chain coverage/)
    assert.match(slot.prompt, /Refrigerated depots/)
    assert.match(slot.prompt, /no text/i)
    assert.match(slot.prompt, /do not produce generic business stock imagery/i)
  })

  it('prefers an explicit authored data-ai-prompt', () => {
    const html = `<!DOCTYPE html><html><head></head><body><div class="inferenesia-deck"><section class="slide" data-slide-id="s1" data-title="T"><div data-image-slot="right" data-ai-prompt="a lone lighthouse at dusk" class="w-[380px] h-[260px]">Image slot</div></section></div></body></html>`
    const [slot] = listAiImageSlots(html)
    assert.equal(slot.prompt, 'a lone lighthouse at dusk')
  })
})

describe('slidesDesignSystemPrompt', () => {
  it('carries the export contract the exporter actually relies on', () => {
    const p = slidesDesignSystemPrompt()
    assert.match(p, /### Export contract/)
    for (const role of [
      'title',
      'subtitle',
      'body',
      'label',
      'bullet',
      'number',
      'list-item',
      'stat',
      'caption',
      'card',
      'shape',
      'image',
      'table-cell',
    ]) {
      assert.ok(p.includes(role), `design prompt must document the ${role} role`)
    }
    assert.match(p, /data-ex-valign/)
    assert.match(p, /data-ex-z/)
    assert.match(p, /Gradient, image, or blurred surfaces stay in the captured background/)
  })

  it('keeps the 1280x720 artboard contract', () => {
    assert.match(slidesDesignSystemPrompt(), /1280×720/)
  })
})
