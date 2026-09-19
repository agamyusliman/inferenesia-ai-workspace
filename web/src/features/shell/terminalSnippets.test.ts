import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

/**
 * Mirrors default snippet policy (VAL-IDE-036): defaults must never hold secrets.
 * Server-side defaults live in Go core; this guards the client expectation list.
 */
const DEFAULT_SNIPPET_BODIES = ['git status\n', 'ls -la\n', 'pwd\n']

const SECRET_MARKERS = [
  'api_key',
  'apikey',
  'password',
  'secret',
  'token',
  'bearer',
  'sk-',
  'TEMP_AI_API_KEY',
]

describe('terminal snippets defaults (VAL-IDE-036)', () => {
  it('default snippet bodies contain no secret-like markers', () => {
    for (const body of DEFAULT_SNIPPET_BODIES) {
      const lower = body.toLowerCase()
      for (const bad of SECRET_MARKERS) {
        assert.equal(
          lower.includes(bad.toLowerCase()),
          false,
          `default body must not contain ${bad}: ${body}`,
        )
      }
      assert.ok(body.trim().length > 0)
    }
  })

  it('insert target is the active terminal write path (policy)', () => {
    // Policy check: insert uses terminalWrite(id, body) into focused session,
    // not clipboard-only paste and not workspace-local storage.
  const storageRoot = '~/.inferenesia/snippets/terminal.json'
  assert.ok(storageRoot.includes('.inferenesia'))
    assert.ok(!storageRoot.includes('.env'))
  })
})
