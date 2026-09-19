import assert from 'node:assert/strict'
import { describe, it, afterEach } from 'node:test'
import {
  altKeyLabel,
  fileManagerName,
  formatShortcut,
  getHostOS,
  isPrimaryModKey,
  isSaveKeyEvent,
  modKeyLabel,
  platformMessageVars,
  revealInOSLabel,
  setHostOSForTests,
} from './platform'

describe('platform', () => {
  afterEach(() => {
    setHostOSForTests(null)
  })

  it('macos labels use Finder and Cmd/⌘', () => {
    setHostOSForTests('macos')
    assert.equal(getHostOS(), 'macos')
    assert.equal(fileManagerName(), 'Finder')
    assert.equal(revealInOSLabel(), 'Reveal in Finder')
    assert.equal(modKeyLabel('text'), 'Cmd')
    assert.equal(modKeyLabel('symbol'), '⌘')
    assert.equal(altKeyLabel('symbol'), '⌥')
    assert.equal(formatShortcut('S'), '⌘S')
    assert.equal(formatShortcut('S', { alt: true }), '⌥⌘S')
    assert.equal(formatShortcut('P', { shift: true }), '⇧⌘P')
    assert.equal(formatShortcut('`', { forceCtrl: true }), 'Ctrl+`')
  })

  it('windows labels use Explorer and Ctrl', () => {
    setHostOSForTests('windows')
    assert.equal(fileManagerName(), 'Explorer')
    assert.equal(revealInOSLabel(), 'Reveal in Explorer')
    assert.equal(modKeyLabel('text'), 'Ctrl')
    assert.equal(formatShortcut('S'), 'Ctrl+S')
    assert.equal(formatShortcut('S', { alt: true }), 'Ctrl+Alt+S')
    assert.equal(formatShortcut('P', { shift: true }), 'Ctrl+Shift+P')
  })

  it('linux labels use Files and Ctrl', () => {
    setHostOSForTests('linux')
    assert.equal(fileManagerName(), 'Files')
    assert.equal(revealInOSLabel(), 'Reveal in Files')
    assert.equal(formatShortcut('W'), 'Ctrl+W')
  })

  it('platformMessageVars exposes shortcut strings', () => {
    setHostOSForTests('windows')
    const v = platformMessageVars()
    assert.equal(v.mod, 'Ctrl')
    assert.equal(v.saveShortcut, 'Ctrl+S')
    assert.equal(v.fileManager, 'Explorer')
    assert.equal(v.reveal, 'Reveal in Explorer')
  })

  it('isSaveKeyEvent matches OS primary modifier', () => {
    setHostOSForTests('macos')
    assert.equal(
      isSaveKeyEvent({
        key: 's',
        metaKey: true,
        ctrlKey: false,
        altKey: false,
        shiftKey: false,
      }),
      true,
    )
    assert.equal(
      isSaveKeyEvent({
        key: 's',
        metaKey: false,
        ctrlKey: true,
        altKey: false,
        shiftKey: false,
      }),
      false,
    )
    assert.equal(isPrimaryModKey({ metaKey: true, ctrlKey: false }), true)

    setHostOSForTests('windows')
    assert.equal(
      isSaveKeyEvent({
        key: 's',
        metaKey: false,
        ctrlKey: true,
        altKey: false,
        shiftKey: false,
      }),
      true,
    )
    assert.equal(
      isSaveKeyEvent({
        key: 's',
        metaKey: true,
        ctrlKey: false,
        altKey: false,
        shiftKey: false,
      }),
      false,
    )
    assert.equal(isPrimaryModKey({ metaKey: false, ctrlKey: true }), true)
  })
})
