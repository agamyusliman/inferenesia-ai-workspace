/**
 * Panel header layout: single toolbar row, no h-8 + flex-col clip.
 *   npx --yes tsx --test src/features/shell/panelHeaderLayout.test.ts
 */
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  isClippedMultiRowHeaderClass,
  panelHeaderRootClass,
} from './panelHeaderLayout'
import { SHELL } from './shellTokens'

describe('panelHeaderRootClass', () => {
  it('uses single-row shell header for dense and subtitle modes', () => {
    assert.equal(panelHeaderRootClass({ dense: true }), SHELL.panelHeaderClass)
    assert.equal(
      panelHeaderRootClass({ dense: false, hasSubtitle: true }),
      SHELL.panelHeaderClass,
    )
    assert.equal(panelHeaderRootClass(), SHELL.panelHeaderClass)
  })

  it('never pairs fixed header height with flex-col (git-panel-header overflow bug)', () => {
    const root = panelHeaderRootClass({ hasSubtitle: true })
    assert.equal(isClippedMultiRowHeaderClass(root), false)
    assert.match(root, /\bh-10\b/)
    assert.match(root, /\bitems-center\b/)
    assert.doesNotMatch(root, /\bflex-col\b/)
  })

  it('detects the legacy clipped dual-row pattern', () => {
    const bad =
      'flex h-10 shrink-0 flex-col justify-center border-b border-shell-border bg-shell-panel px-2'
    assert.equal(isClippedMultiRowHeaderClass(bad), true)
    assert.equal(isClippedMultiRowHeaderClass(SHELL.panelHeaderClass), false)
  })
})
