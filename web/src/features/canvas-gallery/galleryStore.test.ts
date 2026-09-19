import assert from 'node:assert/strict'
import { afterEach, describe, it } from 'node:test'
import {
  createGalleryDocCanvas,
  createGalleryExcalidrawCanvas,
  createGalleryImageCanvas,
  createGallerySlidesCanvas,
  deleteDocCanvas,
  formatGalleryTime,
  indexDiagramsFromChatMessages,
  listGalleryCanvases,
  loadDiagrams,
  loadDocCanvas,
  loadExcalidrawCanvas,
  loadImageCanvas,
  loadSlidesCanvas,
  parseGalleryId,
  purgeOrphanCanvasStorage,
  updateDiagramCanvas,
  updateDocCanvas,
  updateExcalidrawCanvas,
  updateImageCanvas,
  updateSlidesCanvas,
} from './galleryStore'

/** Byte ceiling for the fake quota; Infinity = unlimited. */
let quota = Infinity

const mem = new Map<string, string>()

function installLocalStorage() {
  const store: Storage = {
    get length() {
      return mem.size
    },
    clear() {
      mem.clear()
    },
    getItem(k: string) {
      return mem.has(k) ? mem.get(k)! : null
    },
    key(i: number) {
      return [...mem.keys()][i] ?? null
    },
    removeItem(k: string) {
      mem.delete(k)
    },
    setItem(k: string, v: string) {
      const value = String(v)
      let others = 0
      for (const [key, existing] of mem) {
        if (key !== k) others += existing.length
      }
      if (others + value.length > quota) {
        const err = new Error('QuotaExceededError')
        err.name = 'QuotaExceededError'
        throw err
      }
      mem.set(k, value)
    },
  }
  ;(globalThis as { window?: { localStorage: Storage } }).window = {
    localStorage: store,
  }
  ;(globalThis as { localStorage?: Storage }).localStorage = store
}

afterEach(() => {
  mem.clear()
})

afterEach(() => {
  quota = Infinity
})

describe('document playgrounds', () => {
  for (const kind of ['markdown', 'table', 'timeline'] as const) {
    it(`round-trips ${kind} content through storage`, () => {
      installLocalStorage()
      const created = createGalleryDocCanvas(kind, {
        scopeId: 'global',
        title: `My ${kind}`,
      })
      assert.equal(created.kind, kind)
      const parsed = parseGalleryId(created.id)
      assert.ok(parsed)
      assert.equal(parsed!.kind, kind)

      const body = `stored ${kind} payload`
      const saved = updateDocCanvas(kind, parsed!.sessionId, parsed!.localId, {
        content: body,
      })
      assert.equal(saved?.saveResult?.ok, true)
      assert.equal(
        loadDocCanvas(kind, parsed!.sessionId, parsed!.localId)!.content,
        body,
      )

      const listed = listGalleryCanvases([]).find((i) => i.id === created.id)
      assert.equal(listed?.source, body)

      assert.equal(deleteDocCanvas(kind, parsed!.sessionId, parsed!.localId), true)
      assert.equal(loadDocCanvas(kind, parsed!.sessionId, parsed!.localId), null)
    })
  }

  it('keeps session-scoped documents out of other scopes', () => {
    installLocalStorage()
    const doc = createGalleryDocCanvas('table', { scopeId: 'ws_sess_1' })
    assert.equal(
      listGalleryCanvases([]).some((i) => i.id === doc.id),
      false,
    )
    assert.equal(
      listGalleryCanvases([{ id: 'ws_sess_1', name: 'S1' }]).some(
        (i) => i.id === doc.id,
      ),
      true,
    )
  })

  it('purges documents of dead sessions only', () => {
    installLocalStorage()
    createGalleryDocCanvas('markdown', { scopeId: 'ws_sess_dead' })
    const live = createGalleryDocCanvas('markdown', { scopeId: 'ws_sess_live' })
    const parsed = parseGalleryId(live.id)!
    const result = purgeOrphanCanvasStorage(['ws_sess_live'])
    assert.ok(result.removedDocKeys >= 1)
    assert.ok(loadDocCanvas('markdown', parsed.sessionId, parsed.localId))
    assert.equal(loadDocCanvas('markdown', 'ws_sess_dead', 'whatever'), null)
  })
})

describe('storage never destroys content', () => {
  const dataUrl = `data:image/png;base64,${'A'.repeat(400)}`

  it('stores image data URLs verbatim instead of a placeholder', () => {
    installLocalStorage()
    const img = createGalleryImageCanvas({ scopeId: 'global' })
    const parsed = parseGalleryId(img.id)!
    updateImageCanvas(parsed.sessionId, parsed.localId, { imageUrl: dataUrl })
    assert.equal(
      loadImageCanvas(parsed.sessionId, parsed.localId)!.imageUrl,
      dataUrl,
    )
  })

  it('fails an over-quota image save without touching stored images', () => {
    installLocalStorage()
    const img = createGalleryImageCanvas({ scopeId: 'global' })
    const parsed = parseGalleryId(img.id)!
    updateImageCanvas(parsed.sessionId, parsed.localId, { imageUrl: dataUrl })
    const before = [...mem.entries()]

    quota = 200
    const result = updateImageCanvas(parsed.sessionId, parsed.localId, {
      imageUrl: `data:image/png;base64,${'B'.repeat(4000)}`,
    })
    assert.equal(result?.saveResult?.ok, false)
    assert.match(result!.saveResult!.error!, /storage is full/i)
    quota = Infinity

    assert.deepEqual([...mem.entries()], before)
    assert.equal(
      loadImageCanvas(parsed.sessionId, parsed.localId)!.imageUrl,
      dataUrl,
    )
  })

  it('keeps embedded deck images and fails an over-quota deck save', () => {
    installLocalStorage()
    const deck = createGallerySlidesCanvas({ scopeId: 'global' })
    const parsed = parseGalleryId(deck.id)!
    const withImage = `<html><img src="${dataUrl}" /></html>`
    assert.equal(
      updateSlidesCanvas(parsed.sessionId, parsed.localId, {
        content: withImage,
      })?.saveResult?.ok,
      true,
    )
    assert.equal(
      loadSlidesCanvas(parsed.sessionId, parsed.localId)!.content,
      withImage,
    )

    quota = 200
    const result = updateSlidesCanvas(parsed.sessionId, parsed.localId, {
      content: `<html><img src="data:image/png;base64,${'C'.repeat(4000)}" /></html>`,
    })
    assert.equal(result?.saveResult?.ok, false)
    quota = Infinity
    assert.equal(
      loadSlidesCanvas(parsed.sessionId, parsed.localId)!.content,
      withImage,
    )
  })
})

describe('parseGalleryId', () => {
  it('parses legacy html id', () => {
    assert.deepEqual(parseGalleryId('ws_sess_1::cv_abc'), {
      sessionId: 'ws_sess_1',
      kind: 'html',
      localId: 'cv_abc',
    })
  })

  it('parses kind-prefixed id', () => {
    assert.deepEqual(parseGalleryId('ws_sess_1::diagram::dg_1'), {
      sessionId: 'ws_sess_1',
      kind: 'diagram',
      localId: 'dg_1',
    })
  })

  it('rejects invalid', () => {
    assert.equal(parseGalleryId('nope'), null)
  })
})

describe('formatGalleryTime', () => {
  it('formats finite timestamps', () => {
    const s = formatGalleryTime(Date.UTC(2026, 0, 1, 12, 0, 0))
    assert.ok(s.length > 4)
  })
})

describe('indexDiagramsFromChatMessages', () => {
  it('does not re-add after source edit for same chatMessageId', () => {
    installLocalStorage()
    const sid = 'ws_sess_test'
    const original = 'flowchart TD\n  A[Start] --> B[End]'
    const edited = 'flowchart TD\n  A[Start] --> B[Done]'
    const added = indexDiagramsFromChatMessages(sid, [
      {
        id: 'msg_1',
        role: 'assistant',
        content: '```mermaid\n' + original + '\n```',
      },
    ])
    assert.equal(added, 1)
    const first = loadDiagrams(sid)
    assert.equal(first.length, 1)
    updateDiagramCanvas(sid, first[0].id, { source: edited })
    assert.equal(loadDiagrams(sid)[0].source, edited)

    const again = indexDiagramsFromChatMessages(sid, [
      {
        id: 'msg_1',
        role: 'assistant',
        content: '```mermaid\n' + original + '\n```',
      },
    ])
    assert.equal(again, 0)
    const after = loadDiagrams(sid)
    assert.equal(after.length, 1)
    assert.equal(after[0].id, first[0].id)
    assert.equal(after[0].source, edited)
  })
})

describe('excalidraw gallery', () => {
  it('creates and updates whiteboard content', () => {
    installLocalStorage()
    const created = createGalleryExcalidrawCanvas({
      scopeId: 'global',
      title: 'Board A',
    })
    assert.equal(created.kind, 'excalidraw')
    assert.equal(created.title, 'Board A')
    const parsed = parseGalleryId(created.id)
    assert.ok(parsed)
    assert.equal(parsed!.kind, 'excalidraw')
    const entry = loadExcalidrawCanvas(parsed!.sessionId, parsed!.localId)
    assert.ok(entry)
    assert.ok((entry!.content || '').includes('"type": "excalidraw"'))
    const next = updateExcalidrawCanvas(parsed!.sessionId, parsed!.localId, {
      content: '{"type":"excalidraw","version":2,"source":"inferenesia","elements":[{"id":"1"}],"appState":{},"files":{}}\n',
      title: 'Board B',
    })
    assert.ok(next)
    assert.equal(next!.title, 'Board B')
    assert.ok(next!.content.includes('"id":"1"'))
  })
})

describe('slides gallery', () => {
  it('creates and updates deck content', () => {
    installLocalStorage()
    const created = createGallerySlidesCanvas({
      scopeId: 'global',
      title: 'Deck A',
    })
    assert.equal(created.kind, 'slides')
    const parsed = parseGalleryId(created.id)
    assert.ok(parsed)
    assert.equal(parsed!.kind, 'slides')
    const entry = loadSlidesCanvas(parsed!.sessionId, parsed!.localId)
    assert.ok(entry)
    assert.ok((entry!.content || '').includes('inferenesia-deck'))
    assert.ok((entry!.content || '').includes('slide-blank'))
    const next = updateSlidesCanvas(parsed!.sessionId, parsed!.localId, {
      title: 'Deck B',
    })
    assert.ok(next)
    assert.equal(next!.title, 'Deck B')
  })
})

describe('purgeOrphanCanvasStorage', () => {
  it('does not wipe session keys when live session list is empty', () => {
    installLocalStorage()
    const sid = 'ws_sess_keep'
    indexDiagramsFromChatMessages(sid, [
      {
        id: 'msg_keep',
        role: 'assistant',
        content: '```mermaid\nflowchart TD\n  A --> B\n```',
      },
    ])
    assert.equal(loadDiagrams(sid).length, 1)
    const r = purgeOrphanCanvasStorage([])
    assert.equal(r.removedDiagramKeys, 0)
    assert.equal(loadDiagrams(sid).length, 1)
  })

  it('removes keys for deleted sessions only', () => {
    installLocalStorage()
    indexDiagramsFromChatMessages('ws_sess_dead', [
      {
        id: 'm1',
        role: 'assistant',
        content: '```mermaid\nflowchart TD\n  X --> Y\n```',
      },
    ])
    indexDiagramsFromChatMessages('ws_sess_live', [
      {
        id: 'm2',
        role: 'assistant',
        content: '```mermaid\nflowchart TD\n  P --> Q\n```',
      },
    ])
    const r = purgeOrphanCanvasStorage(['ws_sess_live'])
    assert.ok(r.removedDiagramKeys >= 1)
    assert.equal(loadDiagrams('ws_sess_dead').length, 0)
    assert.equal(loadDiagrams('ws_sess_live').length, 1)
  })
})
