import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  extractElementsFromModelText,
  extractFirstJSONBlock,
  isCanvasElementType,
  mergeCanvasAIResult,
  elementsJSONForRequest,
  placeholderForMode,
  stripFences,
  submitLabel,
  type CanvasElement,
} from './canvasAI'
import {
  emptyExcalidrawDocument,
  serializeExcalidraw,
} from './excalidrawDoc'
import type { CanvasAIResult } from '../../lib/api'

describe('stripFences', () => {
  it('strips ```json fence', () => {
    const raw = '```json\n[{"a":1}]\n```'
    assert.equal(stripFences(raw), '[{"a":1}]')
  })

  it('strips bare ``` fence', () => {
    const raw = '```\n[{"a":1}]\n```'
    assert.equal(stripFences(raw), '[{"a":1}]')
  })

  it('strips ```excalidraw fence', () => {
    const raw = '```excalidraw\n[{"a":1}]\n```'
    assert.equal(stripFences(raw), '[{"a":1}]')
  })

  it('passes through non-fenced json', () => {
    assert.equal(stripFences('[1,2,3]'), '[1,2,3]')
  })

  it('strips fence with surrounding whitespace', () => {
    const raw = '  ```json\n[1]\n```  '
    assert.equal(stripFences(raw), '[1]')
  })
})

describe('extractFirstJSONBlock', () => {
  it('extracts array from prose', () => {
    const s = 'Here:\n[{"x":1}]\nDone!'
    assert.equal(extractFirstJSONBlock(s), '[{"x":1}]')
  })

  it('extracts object from prose', () => {
    const s = 'Result: {"elements":[]} end'
    assert.equal(extractFirstJSONBlock(s), '{"elements":[]}')
  })

  it('returns empty when no json', () => {
    assert.equal(extractFirstJSONBlock('no json here'), '')
  })

  it('handles nested braces in strings', () => {
    const s = 'x {"a":"}","b":2} y'
    assert.equal(extractFirstJSONBlock(s), '{"a":"}","b":2}')
  })

  it('handles escaped quotes in strings', () => {
    const s = 'x {"a":"he said \\"hi\\""} y'
    assert.equal(extractFirstJSONBlock(s), '{"a":"he said \\"hi\\""}')
  })
})

describe('extractElementsFromModelText', () => {
  it('parses bare array', () => {
    const r = extractElementsFromModelText(
      '[{"type":"rectangle","id":"a","x":0}]',
    )
    assert.equal(r.ok, true)
    if (r.ok) {
      assert.equal(r.elements.length, 1)
      assert.equal(r.elements[0].id, 'a')
    }
  })

  it('parses fenced array', () => {
    const r = extractElementsFromModelText(
      '```json\n[{"type":"text","id":"b","text":"hi"}]\n```',
    )
    assert.equal(r.ok, true)
    if (r.ok) assert.equal(r.elements[0].id, 'b')
  })

  it('parses object with elements key', () => {
    const r = extractElementsFromModelText(
      '{"elements":[{"type":"line","id":"c","points":[[0,0]]}]}',
    )
    assert.equal(r.ok, true)
    if (r.ok) assert.equal(r.elements[0].id, 'c')
  })

  it('wraps single element object', () => {
    const r = extractElementsFromModelText('{"type":"rectangle","id":"d"}')
    assert.equal(r.ok, true)
    if (r.ok) assert.equal(r.elements.length, 1)
  })

  it('mints missing ids', () => {
    const r = extractElementsFromModelText('[{"type":"rectangle","x":0}]')
    assert.equal(r.ok, true)
    if (r.ok) {
      assert.ok(String(r.elements[0].id).startsWith('el'))
      assert.ok(r.elements[0].id.length >= 10)
    }
  })

  it('preserves existing ids', () => {
    const r = extractElementsFromModelText(
      '[{"type":"rectangle","id":"keep","x":0}]',
    )
    assert.equal(r.ok, true)
    if (r.ok) assert.equal(r.elements[0].id, 'keep')
  })

  it('rejects empty array', () => {
    const r = extractElementsFromModelText('[]')
    assert.equal(r.ok, false)
    if (!r.ok) assert.match(r.error, /empty/i)
  })

  it('rejects missing type', () => {
    const r = extractElementsFromModelText('[{"id":"x"}]')
    assert.equal(r.ok, false)
    if (!r.ok) assert.match(r.error, /type/i)
  })

  it('rejects garbage', () => {
    const r = extractElementsFromModelText('not json at all')
    assert.equal(r.ok, false)
    if (!r.ok) assert.match(r.error, /not valid JSON/i)
  })

  it('rejects empty input', () => {
    const r = extractElementsFromModelText('   ')
    assert.equal(r.ok, false)
    if (!r.ok) assert.match(r.error, /empty/i)
  })

  it('extracts json surrounded by prose', () => {
    const r = extractElementsFromModelText(
      'Here is your diagram:\n[{"type":"rectangle","id":"e","x":0,"y":0,"width":1,"height":1}]\nHope it helps!',
    )
    assert.equal(r.ok, true)
    if (r.ok) assert.equal(r.elements[0].id, 'e')
  })
})

describe('isCanvasElementType', () => {
  it('accepts known types', () => {
    assert.equal(isCanvasElementType('rectangle'), true)
    assert.equal(isCanvasElementType('ellipse'), true)
    assert.equal(isCanvasElementType('arrow'), true)
    assert.equal(isCanvasElementType('text'), true)
    assert.equal(isCanvasElementType('freedraw'), true)
  })

  it('rejects unknown types', () => {
    assert.equal(isCanvasElementType('widget'), false)
    assert.equal(isCanvasElementType(''), false)
    assert.equal(isCanvasElementType(null), false)
    assert.equal(isCanvasElementType(42), false)
  })
})

describe('elementsJSONForRequest', () => {
  it('serializes clean objects', () => {
    const els: CanvasElement[] = [
      { id: 'a', type: 'rectangle', x: 0 },
      { id: 'b', type: 'text', text: 'hi' },
    ]
    const s = elementsJSONForRequest(els)
    const parsed = JSON.parse(s)
    assert.equal(parsed.length, 2)
    assert.equal(parsed[0].id, 'a')
  })

  it('filters non-objects', () => {
    const els = [
      { id: 'a', type: 'rectangle' },
      null,
      'string',
      42,
      { id: 'b', type: 'text' },
    ]
    const s = elementsJSONForRequest(els)
    const parsed = JSON.parse(s)
    assert.equal(parsed.length, 2)
  })
})

describe('mergeCanvasAIResult', () => {
  it('merges into a fresh document', () => {
    const result: CanvasAIResult = {
      ok: true,
      elements_json:
        '[{"type":"rectangle","id":"r1","x":0,"y":0,"width":10,"height":10}]',
      mode: 'generate',
    }
    const base = emptyExcalidrawDocument()
    const merged = mergeCanvasAIResult(result, base)
    assert.ok('document' in merged)
    if ('document' in merged) {
      assert.equal(merged.document.type, 'excalidraw')
      assert.equal(merged.document.source, EXCALIDRAW_SOURCE)
      assert.equal(merged.document.elements.length, 1)
      assert.equal(merged.document.elements[0].id, 'r1')
      assert.equal(merged.serialized, serializeExcalidraw(merged.document))
    }
  })

  it('preserves base appState + files', () => {
    const result: CanvasAIResult = {
      ok: true,
      elements_json: '[{"type":"rectangle","id":"x","x":0}]',
      mode: 'edit',
    }
    const base = emptyExcalidrawDocument()
    base.appState = { viewBackgroundColor: '#1e1e1e', theme: 'dark' }
    base.files = { file1: { data: 'abc' } }
    const merged = mergeCanvasAIResult(result, base)
    assert.ok('document' in merged)
    if ('document' in merged) {
      assert.equal(merged.document.appState.viewBackgroundColor, '#1e1e1e')
      assert.equal(merged.document.appState.theme, 'dark')
      assert.deepEqual(merged.document.files.file1, { data: 'abc' })
    }
  })

  it('returns error when ok is false', () => {
    const result: CanvasAIResult = {
      ok: false,
      error: 'model failed',
    }
    const merged = mergeCanvasAIResult(result, emptyExcalidrawDocument())
    assert.ok('error' in merged)
    if ('error' in merged) assert.equal(merged.error, 'model failed')
  })

  it('returns error when elements_json is invalid', () => {
    const result: CanvasAIResult = {
      ok: true,
      elements_json: 'not json',
    }
    const merged = mergeCanvasAIResult(result, emptyExcalidrawDocument())
    assert.ok('error' in merged)
    if ('error' in merged) assert.match(merged.error, /invalid elements JSON/i)
  })

  it('returns error when elements_json is not an array', () => {
    const result: CanvasAIResult = {
      ok: true,
      elements_json: '{"not":"array"}',
    }
    const merged = mergeCanvasAIResult(result, emptyExcalidrawDocument())
    assert.ok('error' in merged)
    if ('error' in merged) assert.match(merged.error, /not an array/i)
  })
})

describe('placeholderForMode', () => {
  it('generate mode with no elements', () => {
    assert.match(
      placeholderForMode('generate', 0),
      /Draw auth flow/,
    )
  })

  it('edit mode', () => {
    assert.match(
      placeholderForMode('edit', 5),
      /Change Database box to blue/,
    )
  })

  it('generate mode with elements treats as edit', () => {
    assert.match(
      placeholderForMode('generate', 3),
      /Change Database box to blue/,
    )
  })
})

describe('submitLabel', () => {
  it('Generate when no elements', () => {
    assert.equal(submitLabel(0), 'Generate')
  })

  it('Apply when elements present', () => {
    assert.equal(submitLabel(1), 'Apply')
    assert.equal(submitLabel(10), 'Apply')
  })
})
