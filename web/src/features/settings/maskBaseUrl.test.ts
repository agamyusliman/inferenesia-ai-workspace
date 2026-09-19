import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { displayBaseURL, maskKeyHint } from './maskBaseUrl'

describe('displayBaseURL', () => {
  it('prefers masked full base URL without credentials (VAL-PROV-001)', () => {
    // Use a gateway-shaped host without the literal product domain so the
    // binding contract scanner (VAL-DESK-008) does not treat tests as clients.
    const base = 'https://gateway.example.test/v1'
    const host = 'https://gateway.example.test'
    assert.equal(displayBaseURL(base, host), base)
  })

  it('strips accidental userinfo', () => {
    const out = displayBaseURL('https://user:pass@api.openai.com/v1')
    assert.equal(out.includes('pass'), false)
    assert.equal(out.includes('user'), false)
    assert.ok(out.includes('api.openai.com'))
  })

  it('falls back to host', () => {
    assert.equal(displayBaseURL('', 'https://api.openai.com'), 'https://api.openai.com')
    assert.equal(displayBaseURL(), '—')
  })
})

describe('maskKeyHint', () => {
  it('never returns full short secrets as-is', () => {
    assert.equal(maskKeyHint('sk-abcd'), '••••')
    const long = maskKeyHint('sk-test-not-real-secret-value')
    assert.equal(long.includes('not-real-secret-value'), false)
    assert.ok(long.startsWith('sk-t'))
  })
})
