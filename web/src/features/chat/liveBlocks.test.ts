import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  LIVE_BLOCK_CDN_HOSTS,
  buildLiveBlockCSP,
  buildLiveBlockSrcDoc,
  isLiveBlockLanguage,
} from './liveBlocks'

describe('liveBlocks helpers (P9-live)', () => {
  describe('isLiveBlockLanguage', () => {
    const truthy = ['live-block', 'yura-live', 'html:preview', 'LIVE-BLOCK', '  yura-live ', 'HTML:Preview']
    for (const lang of truthy) {
      it(`true for ${JSON.stringify(lang)}`, () => {
        assert.equal(isLiveBlockLanguage(lang), true)
      })
    }
    const falsy = ['', 'html', 'mermaid', 'javascript', 'liveblock', 'yuralive', null, undefined]
    for (const lang of falsy) {
      it(`false for ${JSON.stringify(lang)}`, () => {
        assert.equal(isLiveBlockLanguage(lang), false)
      })
    }
  })

  describe('buildLiveBlockSrcDoc', () => {
    it('wraps body in an HTML document', () => {
      const doc = buildLiveBlockSrcDoc('<p>hi</p>')
      assert.ok(doc.startsWith('<!DOCTYPE html>'))
      assert.ok(doc.includes('<html'))
      assert.ok(doc.includes('</html>'))
      assert.ok(doc.includes('<p>hi</p>'), 'body must be preserved verbatim')
    })

    it('injects a Content-Security-Policy meta', () => {
      const doc = buildLiveBlockSrcDoc('<p>hi</p>')
      assert.ok(/<meta http-equiv="Content-Security-Policy" content="[^"]+"/i.test(doc), 'CSP meta missing')
    })

    it('CSP restricts script-src to self + CDN allowlist', () => {
      const csp = buildLiveBlockCSP()
      assert.ok(csp.includes("script-src 'self'"), 'script-src must include self')
      for (const host of LIVE_BLOCK_CDN_HOSTS) {
        assert.ok(csp.includes(`https://${host}`), `CSP must allow ${host}`)
      }
    })

    it('CSP blocks connect (no exfiltration)', () => {
      const csp = buildLiveBlockCSP()
      assert.ok(csp.includes("connect-src 'none'"), 'connect-src must be none')
    })

    it('CSP blocks base-uri and form-action', () => {
      const csp = buildLiveBlockCSP()
      assert.ok(csp.includes("base-uri 'none'"), 'base-uri must be none')
      assert.ok(csp.includes("form-action 'none'"), 'form-action must be none')
    })

    it('CSP blocks framing by ancestors', () => {
      const csp = buildLiveBlockCSP()
      assert.ok(csp.includes("frame-ancestors 'none'"), 'frame-ancestors must be none')
    })

    it('handles empty / non-string input without throwing', () => {
      assert.doesNotThrow(() => buildLiveBlockSrcDoc(''))
      // @ts-expect-error testing runtime robustness against non-string
      const doc = buildLiveBlockSrcDoc(undefined)
      assert.ok(doc.includes('<body>'))
    })

    it('preserves script tags verbatim in body (sandbox is the boundary, not escaping)', () => {
      const doc = buildLiveBlockSrcDoc('<script>console.log(1)</script>')
      assert.ok(doc.includes('<script>console.log(1)</script>'))
    })
  })

  describe('LIVE_BLOCK_CDN_HOSTS', () => {
    it('includes core widget CDNs plus Tailwind/fonts for HTML previews', () => {
      assert.ok(LIVE_BLOCK_CDN_HOSTS.includes('cdn.jsdelivr.net'))
      assert.ok(LIVE_BLOCK_CDN_HOSTS.includes('cdn.tailwindcss.com'))
      assert.ok(LIVE_BLOCK_CDN_HOSTS.includes('fonts.googleapis.com'))
      assert.ok(LIVE_BLOCK_CDN_HOSTS.includes('fonts.gstatic.com'))
      assert.deepEqual([...LIVE_BLOCK_CDN_HOSTS].slice(0, 4), [
        'cdn.jsdelivr.net',
        'cdnjs.cloudflare.com',
        'unpkg.com',
        'esm.sh',
      ])
    })
  })
})
