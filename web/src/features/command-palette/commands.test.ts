import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  filterCommands,
  isModKey,
  PALETTE_COMMANDS,
  resolveShortcut,
} from './commands'

describe('filterCommands', () => {
  it('returns all commands for empty query', () => {
    const all = filterCommands('')
    assert.equal(all.length, PALETTE_COMMANDS.length)
  })

  it('filters Save All by partial label', () => {
    const hit = filterCommands('save all')
    assert.ok(hit.some((c) => c.id === 'save-all'))
    assert.ok(hit.every((c) => /save/i.test(c.label) || /save/i.test(c.id)))
  })

  it('filters Toggle Terminal by keyword', () => {
    const hit = filterCommands('pty')
    assert.ok(hit.some((c) => c.id === 'toggle-terminal'))
  })

  it('returns empty when nothing matches', () => {
    assert.equal(filterCommands('zzz-no-such-command').length, 0)
  })
})

describe('resolveShortcut', () => {
  const base = {
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    key: '',
  }

  it('maps Cmd+S to save', () => {
    assert.equal(
      resolveShortcut({ ...base, metaKey: true, key: 's' }),
      'save',
    )
  })

  it('maps Ctrl+W to close-tab', () => {
    assert.equal(
      resolveShortcut({ ...base, ctrlKey: true, key: 'w' }),
      'close-tab',
    )
  })

  it('maps Cmd+Shift+P to command-palette', () => {
    assert.equal(
      resolveShortcut({
        ...base,
        metaKey: true,
        shiftKey: true,
        key: 'p',
      }),
      'command-palette',
    )
  })

  it('maps Cmd+P (no shift) to quick-open', () => {
    assert.equal(
      resolveShortcut({ ...base, metaKey: true, key: 'p' }),
      'quick-open',
    )
  })

  it('ignores non-mod keys', () => {
    assert.equal(resolveShortcut({ ...base, key: 's' }), null)
  })

  it('maps Cmd+Alt+S to save-all', () => {
    assert.equal(
      resolveShortcut({ ...base, metaKey: true, altKey: true, key: 's' }),
      'save-all',
    )
  })
})

describe('isModKey', () => {
  it('true for meta or ctrl', () => {
    assert.equal(isModKey({ metaKey: true, ctrlKey: false }), true)
    assert.equal(isModKey({ metaKey: false, ctrlKey: true }), true)
    assert.equal(isModKey({ metaKey: false, ctrlKey: false }), false)
  })
})
