import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { buildPrintHtml } from './slidesExport'
import { SLIDE_H, SLIDE_W } from './htmlDeck'

describe('buildPrintHtml', () => {
  const deck = `<!DOCTYPE html><html><head><title>d</title></head><body><section class="slide slide-pad"><h1>Hi</h1></section></body></html>`

  it('prints at the authored 16:9 artboard, not A4 landscape', () => {
    const out = buildPrintHtml(deck)
    assert.match(out, new RegExp(`@page \\{ size: ${SLIDE_W}px ${SLIDE_H}px;`))
    assert.equal(/A4 landscape/.test(out), false)
    assert.equal(/297mm/.test(out), false)
    assert.equal(/210mm/.test(out), false)
  })

  it('keeps the page aspect ratio equal to the slide aspect ratio', () => {
    const out = buildPrintHtml(deck)
    const page = /@page \{ size: (\d+)px (\d+)px;/.exec(out)
    assert.ok(page)
    const ratio = Number(page![1]) / Number(page![2])
    assert.ok(
      Math.abs(ratio - SLIDE_W / SLIDE_H) < 1e-9,
      `page ratio ${ratio} must equal artboard ratio ${SLIDE_W / SLIDE_H}`,
    )
  })

  it('does not strip the slide padding or force display:block', () => {
    const out = buildPrintHtml(deck)
    const slideRule = /\.slide \{[\s\S]*?\}/.exec(out)
    assert.ok(slideRule)
    assert.equal(
      /padding: 0 !important/.test(slideRule![0]),
      false,
      'print CSS must not drop authored slide padding',
    )
    assert.equal(
      /display: block !important/.test(slideRule![0]),
      false,
      'forcing display:block breaks flex slide layouts',
    )
  })

  it('forces exact colour so backgrounds survive the print pipeline', () => {
    assert.match(buildPrintHtml(deck), /print-color-adjust: exact/)
  })

  it('injects into head when present and still applies without a head', () => {
    assert.match(buildPrintHtml(deck), /<style>[\s\S]*<\/style><\/head>/)
    const bare = '<section class="slide"></section>'
    const out = buildPrintHtml(bare)
    assert.ok(out.startsWith('<style>'))
    assert.ok(out.endsWith(bare))
  })
})
