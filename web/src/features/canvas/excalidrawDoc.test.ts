import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  emptyExcalidrawDocument,
  isExcalidrawPath,
  parseExcalidrawContent,
  serializeExcalidraw,
  stripVolatileAppState,
  EXCALIDRAW_SOURCE,
  EXCALIDRAW_TYPE,
  EXCALIDRAW_VERSION,
} from './excalidrawDoc'

describe('isExcalidrawPath', () => {
  it('detects .excalidraw suffix (case-insensitive)', () => {
    assert.equal(isExcalidrawPath('docs/diagrams/flow.excalidraw'), true)
    assert.equal(isExcalidrawPath('Flow.EXCALIDRAW'), true)
    assert.equal(isExcalidrawPath('a/b/c.excalidraw'), true)
    assert.equal(isExcalidrawPath('flow.mmd'), false)
    assert.equal(isExcalidrawPath('flow.json'), false)
    assert.equal(isExcalidrawPath('excalidraw'), false)
  })
})

describe('emptyExcalidrawDocument', () => {
  it('matches Go SerializeExcalidraw empty shape', () => {
    const doc = emptyExcalidrawDocument()
    assert.equal(doc.type, EXCALIDRAW_TYPE)
    assert.equal(doc.version, EXCALIDRAW_VERSION)
    assert.equal(doc.source, EXCALIDRAW_SOURCE)
    assert.deepEqual(doc.elements, [])
    assert.deepEqual(doc.appState, {})
    assert.deepEqual(doc.files, {})
  })
})

describe('parseExcalidrawContent', () => {
  it('empty string → empty doc ok', () => {
    const r = parseExcalidrawContent('')
    assert.equal(r.ok, true)
    if (r.ok) {
      assert.deepEqual(r.data.elements, [])
      assert.equal(r.data.type, 'excalidraw')
    }
  })

  it('whitespace only → empty doc ok', () => {
    const r = parseExcalidrawContent('  \n\t  ')
    assert.equal(r.ok, true)
  })

  it('garbage JSON → soft fail + empty doc (never throws)', () => {
    const r = parseExcalidrawContent('{not json')
    assert.equal(r.ok, false)
    if (!r.ok) {
      assert.match(r.error, /invalid JSON/i)
      assert.deepEqual(r.data.elements, [])
    }
  })

  it('missing elements → soft fail', () => {
    const r = parseExcalidrawContent('{"type":"excalidraw"}')
    assert.equal(r.ok, false)
    if (!r.ok) {
      assert.match(r.error, /elements/i)
    }
  })

  it('valid convert_diagram-style document loads elements', () => {
    const raw = JSON.stringify({
      type: 'excalidraw',
      version: 2,
      source: 'inferenesia',
      elements: [
        { type: 'rectangle', id: 'n1', x: 0, y: 0, width: 100, height: 40 },
        {
          type: 'arrow',
          id: 'e1',
          x: 0,
          y: 0,
          points: [
            [0, 0],
            [50, 0],
          ],
        },
      ],
      appState: { viewBackgroundColor: '#ffffff' },
      files: {},
    })
    const r = parseExcalidrawContent(raw)
    assert.equal(r.ok, true)
    if (r.ok) {
      assert.equal(r.data.elements.length, 2)
      assert.equal(r.data.source, EXCALIDRAW_SOURCE)
      assert.equal(r.data.version, 2)
    }
  })

  it('elements-only export (no type) still ok', () => {
    const r = parseExcalidrawContent('{"elements":[]}')
    assert.equal(r.ok, true)
    if (r.ok) {
      assert.equal(r.data.type, 'excalidraw')
      assert.equal(r.data.source, EXCALIDRAW_SOURCE)
    }
  })
})

describe('serializeExcalidraw', () => {
  it('stable 2-space JSON with trailing newline', () => {
    const s = serializeExcalidraw(emptyExcalidrawDocument())
    assert.ok(s.endsWith('\n'))
    const parsed = JSON.parse(s)
    assert.equal(parsed.type, 'excalidraw')
    assert.equal(parsed.version, 2)
    assert.equal(parsed.source, EXCALIDRAW_SOURCE)
    assert.deepEqual(parsed.elements, [])
    assert.ok(s.includes('\n  "type"'))
  })

  it('strips volatile appState keys', () => {
    const s = serializeExcalidraw({
      elements: [],
      appState: {
        viewBackgroundColor: '#1a1a1a',
        selectedElementIds: { a: true },
        collaborators: new Map() as unknown as never,
        theme: 'dark',
      },
      files: {},
    })
    const parsed = JSON.parse(s) as {
      appState: Record<string, unknown>
    }
    assert.equal(parsed.appState.viewBackgroundColor, '#1a1a1a')
    assert.equal(parsed.appState.theme, 'dark')
    assert.equal(parsed.appState.selectedElementIds, undefined)
    assert.equal(parsed.appState.collaborators, undefined)
  })
})

describe('stripVolatileAppState', () => {
  it('returns empty object for null/undefined', () => {
    assert.deepEqual(stripVolatileAppState(null), {})
    assert.deepEqual(stripVolatileAppState(undefined), {})
  })
})
