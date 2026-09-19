import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  cycleMode,
  defaultViewMode,
  isRenderablePreviewPath,
  supportsExcalidrawCanvas,
  supportsMarkdownPreview,
} from './editorMode'

describe('editorMode', () => {
  it('detects markdown paths for preview', () => {
    assert.equal(supportsMarkdownPreview('README.md'), true)
    assert.equal(supportsMarkdownPreview('docs/note.mdx'), true)
    assert.equal(supportsMarkdownPreview('main.go'), false)
    assert.equal(supportsMarkdownPreview('app.tsx'), false)
  })

  it('isRenderablePreviewPath covers MD/Mermaid/image/excalidraw (VAL-IDE-026)', () => {
    assert.equal(isRenderablePreviewPath('README.md'), true)
    assert.equal(isRenderablePreviewPath('flow.mmd'), true)
    assert.equal(isRenderablePreviewPath('diagram.mermaid'), true)
    assert.equal(isRenderablePreviewPath('shot.png'), true)
    assert.equal(isRenderablePreviewPath('logo.SVG'), true)
    assert.equal(isRenderablePreviewPath('docs/flow.excalidraw'), true)
    assert.equal(isRenderablePreviewPath('main.go'), false)
  })

  it('supportsExcalidrawCanvas and defaultViewMode for .excalidraw', () => {
    assert.equal(supportsExcalidrawCanvas('a.excalidraw'), true)
    assert.equal(supportsExcalidrawCanvas('a.json'), false)
    assert.equal(defaultViewMode('docs/diagrams/auth.excalidraw'), 'preview')
    assert.equal(defaultViewMode('README.md'), 'edit')
  })

  it('cycles edit/preview only when allowed', () => {
    assert.equal(cycleMode('edit', true), 'preview')
    assert.equal(cycleMode('preview', true), 'edit')
    assert.equal(cycleMode('preview', false), 'edit')
  })
})
