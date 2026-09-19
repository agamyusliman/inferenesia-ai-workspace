import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  isLikelyIncompleteMermaid,
  normalizeMermaidSource,
} from './mermaidRender'

describe('normalizeMermaidSource', () => {
  it('strips mermaid fences', () => {
    assert.equal(
      normalizeMermaidSource('```mermaid\nflowchart TD\n  A --> B\n```'),
      'flowchart TD\n  A --> B',
    )
  })

  it('keeps bare source', () => {
    assert.equal(
      normalizeMermaidSource('flowchart LR\n  A --> B'),
      'flowchart LR\n  A --> B',
    )
  })
})

describe('isLikelyIncompleteMermaid', () => {
  it('flags empty and short', () => {
    assert.equal(isLikelyIncompleteMermaid(''), true)
    assert.equal(isLikelyIncompleteMermaid('flow'), true)
  })

  it('flags open subgraph', () => {
    assert.equal(
      isLikelyIncompleteMermaid('flowchart TD\nsubgraph A\n  X --> Y'),
      true,
    )
  })

  it('accepts complete flowchart', () => {
    assert.equal(
      isLikelyIncompleteMermaid('flowchart TD\n  A --> B\n  B --> C'),
      false,
    )
  })
})

describe('mermaidThemeFromAppTheme', () => {
  it('maps light → print, warm → warm, dark → dark', async () => {
    const { mermaidThemeFromAppTheme } = await import('./mermaidRender')
    assert.equal(mermaidThemeFromAppTheme('light'), 'print')
    assert.equal(mermaidThemeFromAppTheme('warm'), 'warm')
    assert.equal(mermaidThemeFromAppTheme('dark'), 'dark')
    assert.equal(mermaidThemeFromAppTheme(undefined), 'dark')
  })
})
