import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  applySearchReplacePatches,
  extractSearchReplacePatches,
  resolveCanvasAgentReply,
} from './canvasPatch'

describe('canvasPatch', () => {
  it('extracts search/replace blocks', () => {
    const text = [
      '<<<<<<< SEARCH',
      'hello',
      '=======',
      'world',
      '>>>>>>> REPLACE',
      '',
      '<<<<<<< SEARCH',
      'a',
      '=======',
      'b',
      '>>>>>>> REPLACE',
    ].join('\n')
    const patches = extractSearchReplacePatches(text)
    assert.equal(patches.length, 2)
    assert.equal(patches[0].search, 'hello')
    assert.equal(patches[0].replace, 'world')
    assert.equal(patches[1].search, 'a')
    assert.equal(patches[1].replace, 'b')
  })

  it('applies patches without rewriting untouched regions', () => {
    const base = [
      '<header>Nav A</header>',
      '<main>Hero stays</main>',
      '<footer>Foot</footer>',
    ].join('\n')
    const result = applySearchReplacePatches(base, [
      { search: '<header>Nav A</header>', replace: '<header>Nav B</header>' },
    ])
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.mode, 'patches')
    assert.equal(result.applied, 1)
    assert.equal(
      result.next,
      [
        '<header>Nav B</header>',
        '<main>Hero stays</main>',
        '<footer>Foot</footer>',
      ].join('\n'),
    )
  })

  it('fails when SEARCH is missing', () => {
    const result = applySearchReplacePatches('<div>a</div>', [
      { search: 'missing', replace: 'x' },
    ])
    assert.equal(result.ok, false)
    if (result.ok) return
    assert.match(result.error, /SEARCH not found/)
  })

  it('prefers patches over full document', () => {
    const base = 'A [Start] --> B [End]'
    const reply = [
      '<<<<<<< SEARCH',
      'A [Start]',
      '=======',
      'A [Mulai]',
      '>>>>>>> REPLACE',
      '',
      '```mermaid',
      'graph TD',
      'X --> Y',
      '```',
    ].join('\n')
    const result = resolveCanvasAgentReply(base, reply, 'graph TD\nX --> Y')
    assert.equal(result.ok, true)
    if (!result.ok) return
    assert.equal(result.mode, 'patches')
    assert.equal(result.next, 'A [Mulai] --> B [End]')
  })
})
