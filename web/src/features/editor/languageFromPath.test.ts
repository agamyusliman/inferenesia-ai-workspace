import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { languageFromPath } from './languageFromPath'

describe('languageFromPath', () => {
  it('maps common code extensions', () => {
    assert.equal(languageFromPath('src/app.tsx'), 'typescript')
    assert.equal(languageFromPath('main.go'), 'go')
    assert.equal(languageFromPath('lib/util.py'), 'python')
    assert.equal(languageFromPath('index.js'), 'javascript')
    assert.equal(languageFromPath('styles.css'), 'css')
    assert.equal(languageFromPath('README.md'), 'markdown')
    assert.equal(languageFromPath('data.json'), 'json')
    assert.equal(languageFromPath('compose.yml'), 'yaml')
    assert.equal(languageFromPath('run.sh'), 'shell')
  })

  it('handles special filenames', () => {
    assert.equal(languageFromPath('Dockerfile'), 'dockerfile')
    assert.equal(languageFromPath('.env'), 'ini')
    assert.equal(languageFromPath('go.mod'), 'go')
  })

  it('falls back to plaintext', () => {
    assert.equal(languageFromPath('notes.xyz'), 'plaintext')
  })

  it('maps excalidraw to JSON for editor round-trip (VAL-DIAG-007)', () => {
    assert.equal(languageFromPath('docs/diagrams/bridge.excalidraw'), 'json')
  })
})
