import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

/** Mirrors FindInFolderPanel underFolder (VAL-IDE-008 negative scope). */
function underFolder(rel: string, folder: string): boolean {
  const r = rel.replace(/\\/g, '/').replace(/^\.\//, '')
  const f = folder.replace(/\\/g, '/').replace(/\/+$/, '')
  if (!f) return r !== ''
  return r === f || r.startsWith(f + '/')
}

describe('find-in-folder scope filter (VAL-IDE-008)', () => {
  it('accepts paths under the selected folder only', () => {
    assert.equal(underFolder('docs/readme.md', 'docs'), true)
    assert.equal(underFolder('docs/sub/a.txt', 'docs'), true)
    assert.equal(underFolder('docs', 'docs'), true)
  })

  it('rejects sibling paths outside the selected folder (negative)', () => {
    assert.equal(underFolder('other/secret.md', 'docs'), false)
    assert.equal(underFolder('docsx/a.md', 'docs'), false)
    assert.equal(underFolder('readme.md', 'docs'), false)
  })

  it('workspace root scope accepts any non-empty path', () => {
    assert.equal(underFolder('docs/a.md', ''), true)
    assert.equal(underFolder('a.txt', ''), true)
    assert.equal(underFolder('', ''), false)
  })
})
