import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  DEVICE_PRESETS,
  chromeBezel,
  defaultOrientation,
  frameSize,
  getDevicePreset,
  outerFrameSize,
  scaleToFit,
  toggleOrientation,
} from './devicePresets'
import {
  applyCanvasSeed,
  findSupersededCanvasId,
  prepareCanvasSwitch,
  type CanvasEntry,
} from './canvasStore'
import {
  buildPreviewSrcDoc,
  iframeSandboxForMode,
  isFullHtmlDocument,
  isPreviewableHtmlLanguage,
  looksLikeHtmlDocument,
  normalizePreviewUrl,
  pickDefaultHtmlPath,
} from './webPreview'

describe('devicePresets', () => {
  it('includes mobile tablet laptop desktop', () => {
    const ids = DEVICE_PRESETS.map((d) => d.id)
    assert.deepEqual(ids, ['mobile', 'tablet', 'laptop', 'desktop'])
  })

  it('getDevicePreset falls back to mobile', () => {
    assert.equal(getDevicePreset('mobile').width, 390)
    assert.equal(getDevicePreset('desktop').width, 1440)
  })

  it('scaleToFit never exceeds 1 and shrinks when needed', () => {
    assert.equal(scaleToFit(100, 100, 500, 500, 0), 1)
    const s = scaleToFit(1000, 1000, 400, 400, 0)
    assert.ok(s < 1)
    assert.ok(s > 0)
    assert.equal(scaleToFit(0, 100, 400, 400), 1)
  })

  it('outerFrameSize includes bezel so scale accounts for phone chrome', () => {
    const bezel = chromeBezel('phone')
    assert.equal(bezel, 12)
    const outer = outerFrameSize({ width: 390, height: 844 }, 'phone')
    assert.equal(outer.width, 390 + bezel * 2)
    assert.equal(outer.height, 844 + bezel * 2)
    const stageW = 320
    const stageH = 600
    const sInner = scaleToFit(390, 844, stageW, stageH, 0)
    const sOuter = scaleToFit(outer.width, outer.height, stageW, stageH, 0)
    assert.ok(sOuter < sInner)
  })

  it('frameSize swaps axes for landscape/portrait', () => {
    const mobile = getDevicePreset('mobile')
    const port = frameSize(mobile, 'portrait')
    const land = frameSize(mobile, 'landscape')
    assert.equal(port.width, 390)
    assert.equal(port.height, 844)
    assert.equal(land.width, 844)
    assert.equal(land.height, 390)

    const desk = getDevicePreset('desktop')
    const dLand = frameSize(desk, 'landscape')
    const dPort = frameSize(desk, 'portrait')
    assert.equal(dLand.width, 1440)
    assert.equal(dLand.height, 900)
    assert.equal(dPort.width, 900)
    assert.equal(dPort.height, 1440)
  })

  it('toggleOrientation and defaultOrientation', () => {
    assert.equal(toggleOrientation('portrait'), 'landscape')
    assert.equal(toggleOrientation('landscape'), 'portrait')
    assert.equal(defaultOrientation('mobile'), 'portrait')
    assert.equal(defaultOrientation('desktop'), 'landscape')
  })
})

describe('normalizePreviewUrl', () => {
  it('returns null for empty or dangerous schemes', () => {
    assert.equal(normalizePreviewUrl(''), null)
    assert.equal(normalizePreviewUrl('   '), null)
    assert.equal(normalizePreviewUrl('javascript:alert(1)'), null)
    assert.equal(normalizePreviewUrl('data:text/html,hi'), null)
  })

  it('prefixes bare host:port with http', () => {
    assert.equal(normalizePreviewUrl('127.0.0.1:8080'), 'http://127.0.0.1:8080/')
    assert.equal(
      normalizePreviewUrl('localhost:4150/app'),
      'http://localhost:4150/app',
    )
  })

  it('accepts http(s) URLs', () => {
    assert.equal(normalizePreviewUrl('https://example.com/x'), 'https://example.com/x')
    assert.equal(normalizePreviewUrl('http://127.0.0.1:4110/'), 'http://127.0.0.1:4110/')
  })

  it('rejects non-http schemes', () => {
    assert.equal(normalizePreviewUrl('file:///tmp/x.html'), null)
    assert.equal(normalizePreviewUrl('ftp://example.com'), null)
  })
})

describe('html preview helpers', () => {
  it('detects full documents vs fragments', () => {
    assert.equal(isFullHtmlDocument('<!DOCTYPE html><html></html>'), true)
    assert.equal(isFullHtmlDocument('<html lang="en"><body></body></html>'), true)
    assert.equal(isFullHtmlDocument('<div>hi</div>'), false)
  })

  it('buildPreviewSrcDoc preserves full document', () => {
    const full = '<!DOCTYPE html><html><body><h1>Hi</h1></body></html>'
    assert.equal(buildPreviewSrcDoc(full), full)
  })

  it('buildPreviewSrcDoc wraps fragments', () => {
    const doc = buildPreviewSrcDoc('<p>hi</p>')
    assert.ok(doc.includes('<!DOCTYPE html>'))
    assert.ok(doc.includes('<p>hi</p>'))
  })

  it('pickDefaultHtmlPath prefers index.html', () => {
    assert.equal(pickDefaultHtmlPath(['readme.md', 'index.html']), 'index.html')
    assert.equal(pickDefaultHtmlPath(['page.html']), 'page.html')
    assert.equal(pickDefaultHtmlPath(['src']), null)
  })

  it('iframe sandbox differs for url vs html', () => {
    assert.ok(iframeSandboxForMode('url').includes('allow-same-origin'))
    assert.ok(!iframeSandboxForMode('html').includes('allow-same-origin'))
    assert.ok(!iframeSandboxForMode('file').includes('allow-same-origin'))
  })

  it('detects previewable html languages and documents', () => {
    assert.equal(isPreviewableHtmlLanguage('html'), true)
    assert.equal(isPreviewableHtmlLanguage('live-block'), true)
    assert.equal(isPreviewableHtmlLanguage('bash'), false)
    assert.equal(looksLikeHtmlDocument('<!DOCTYPE html><html></html>'), true)
    assert.equal(looksLikeHtmlDocument('python -m http.server'), false)
  })
})

describe('prepareCanvasSwitch', () => {
  const base = (partial: Partial<CanvasEntry> & Pick<CanvasEntry, 'id' | 'html'>): CanvasEntry => ({
    title: partial.title || partial.id,
    createdAt: partial.createdAt || 1,
    updatedAt: partial.updatedAt || 1,
    path: partial.path,
    id: partial.id,
    html: partial.html,
  })

  it('applyCanvasSeed updates incomplete stream instead of stacking history', () => {
    const partial =
      '<!DOCTYPE html><html><head><title>x</title></head><body><h1>Hi'
    const full = partial + '</h1><p>done</p></body></html>'
    const first = applyCanvasSeed([], {
      html: partial,
      title: 'html',
      prefer: 'new',
    })
    assert.equal(first.list.length, 1)
    const second = applyCanvasSeed(first.list, {
      html: full,
      title: 'html',
      prefer: 'new',
    })
    assert.equal(second.list.length, 1)
    assert.equal(second.activeId, first.activeId)
    assert.equal(second.list[0]?.html, full)
  })

  it('findSupersededCanvasId matches prefix continuations', () => {
    const list = [
      base({ id: 'a', html: '<html><body>abc' }),
      base({ id: 'b', html: '<div>other</div>' }),
    ]
    assert.equal(
      findSupersededCanvasId(list, '<html><body>abcdef</body></html>'),
      'a',
    )
  })

  it('saves draft on from canvas and returns target entry html', () => {
    const list = [
      base({ id: 'a', html: '<h1>A</h1>' }),
      base({ id: 'b', html: '<h1>B</h1>' }),
    ]
    const { list: next, entry, dirty } = prepareCanvasSwitch(list, {
      fromId: 'a',
      toId: 'b',
      draftHtml: '<h1>A-edited</h1>',
    })
    assert.equal(entry?.id, 'b')
    assert.equal(entry?.html, '<h1>B</h1>')
    assert.equal(next.find((c) => c.id === 'a')?.html, '<h1>A-edited</h1>')
    assert.equal(dirty, true)
  })

  it('does not bump updatedAt when draft unchanged', () => {
    const list = [
      base({ id: 'a', html: '<h1>A</h1>', updatedAt: 100 }),
      base({ id: 'b', html: '<h1>B</h1>', updatedAt: 200 }),
    ]
    const { list: next, dirty } = prepareCanvasSwitch(list, {
      fromId: 'a',
      toId: 'b',
      draftHtml: '<h1>A</h1>',
    })
    assert.equal(dirty, false)
    assert.equal(next.find((c) => c.id === 'a')?.updatedAt, 100)
  })

  it('returns null entry when target missing', () => {
    const list = [base({ id: 'a', html: '<h1>A</h1>' })]
    const { entry } = prepareCanvasSwitch(list, {
      fromId: 'a',
      toId: 'missing',
      draftHtml: '<h1>x</h1>',
    })
    assert.equal(entry, null)
  })
})
