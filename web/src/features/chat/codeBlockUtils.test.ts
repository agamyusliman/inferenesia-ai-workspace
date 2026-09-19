import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  downloadFilename,
  inferenesiaSnippetDownloadName,
  looksLikeUnifiedDiff,
  parseFenceMeta,
  shouldRenderAsDiff,
  truncateCodeLines,
} from './codeBlockUtils'

describe('chatCodeBlock', () => {
  it('parses fence meta with path', () => {
    assert.deepEqual(parseFenceMeta('ts:web/src/a.ts'), {
      language: 'ts',
      path: 'web/src/a.ts',
    })
  })

  it('detects unified diff', () => {
    const sample = `--- a/x.ts
+++ b/x.ts
@@ -1,3 +1,3 @@
-const a = 1
+const a = 2
 context
`
    assert.equal(looksLikeUnifiedDiff(sample), true)
    assert.equal(shouldRenderAsDiff('diff', sample), true)
    assert.equal(shouldRenderAsDiff('typescript', sample), true)
  })

  it('truncates long code', () => {
    const lines = Array.from({ length: 40 }, (_, i) => `line ${i}`).join('\n')
    const t = truncateCodeLines(lines, 10)
    assert.equal(t.truncated, true)
    assert.equal(t.totalLines, 40)
    assert.ok(t.preview.split('\n').length <= 11)
  })

  it('download filename prefers path', () => {
    assert.equal(
      downloadFilename('ts', 'web/src/foo.ts', false),
      'foo.ts',
    )
    assert.equal(downloadFilename('diff', undefined, true), 'change.diff')
  })

  it('inferenesia snippet download name', () => {
    assert.equal(
      inferenesiaSnippetDownloadName({
        sessionName: 'Session · Agam portfolio',
        canvasIndex: 1,
      }),
      'Inferenesia - Agam portfolio - 1.html',
    )
    assert.equal(
      inferenesiaSnippetDownloadName({
        sessionName: 'Landing page CV',
        canvasId: 'cv_abc_xyz99',
      }),
      'Inferenesia - Landing page CV - xyz99.html',
    )
  })
})
