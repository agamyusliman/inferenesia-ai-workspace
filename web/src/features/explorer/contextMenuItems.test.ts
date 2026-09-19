import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  baseName,
  buildContextMenuItems,
  joinRel,
  parentRel,
  suggestCopyName,
  type ContextMenuItem,
} from './contextMenuItems'

function ids(items: ContextMenuItem[]): string[] {
  return items.filter((i) => i.id !== 'separator').map((i) => i.id)
}

describe('buildContextMenuItems Pack A', () => {
  it('folder menu includes New File, New Folder, Find in Folder, clipboard, Open Terminal, rename, delete', () => {
    const items = buildContextMenuItems({
      kind: 'folder',
      path: 'docs',
      revealLabel: 'Reveal in Finder',
      canPaste: true,
    })
    const list = ids(items)
    assert.ok(list.includes('new-file'))
    assert.ok(list.includes('new-folder'))
    assert.ok(list.includes('add-folder-to-workspace'))
    assert.ok(list.includes('find-in-folder'), 'Find in Folder (VAL-IDE-008)')
    const find = items.find((i) => i.id === 'find-in-folder')
    assert.equal(find?.label, 'Find in Folder')
    assert.ok(
      list.includes('open-in-terminal'),
      'Open in Integrated Terminal (VAL-IDE-012)',
    )
    const term = items.find((i) => i.id === 'open-in-terminal')
    assert.equal(term?.label, 'Open in Integrated Terminal')
    // Tree clipboard (VAL-IDE-013)
    assert.ok(list.includes('cut'))
    assert.ok(list.includes('copy'))
    assert.ok(list.includes('paste'))
    const paste = items.find((i) => i.id === 'paste')
    assert.equal(paste?.disabled, false)
    assert.ok(list.includes('copy-path'))
    assert.ok(list.includes('copy-relative-path'))
    assert.ok(list.includes('reveal-in-os'))
    assert.ok(list.includes('rename'))
    assert.ok(list.includes('delete'))
    assert.ok(!list.includes('remove-folder-from-workspace'))
    assert.ok(!list.includes('open-with-edit'))
  })

  it('workspace root menu has Find in Folder, Paste, Open Terminal, create, but not cut/rename/delete', () => {
    const items = buildContextMenuItems({ kind: 'root', path: '', canPaste: false })
    const list = ids(items)
    assert.ok(list.includes('new-file'))
    assert.ok(list.includes('new-folder'))
    assert.ok(list.includes('remove-folder-from-workspace'))
    assert.ok(list.includes('find-in-folder'))
    assert.ok(list.includes('open-in-terminal'))
    assert.ok(list.includes('paste'))
    const paste = items.find((i) => i.id === 'paste')
    assert.equal(paste?.disabled, true, 'Paste disabled when clipboard empty')
    assert.ok(!list.includes('cut'))
    assert.ok(!list.includes('copy'))
    const remove = items.find((i) => i.id === 'remove-folder-from-workspace')
    assert.ok(remove && remove.label.length <= 24, 'short remove label')
    assert.ok(!list.includes('add-folder-to-workspace'))
    assert.ok(!list.includes('rename'))
    assert.ok(!list.includes('delete'))
  })

  it('file with multiple modes includes Open With submenu', () => {
    const items = buildContextMenuItems({
      kind: 'file',
      path: 'README.md',
      hasMultipleOpenModes: true,
      canOpenPreview: true,
      openModes: [
        { id: 'open-with-edit', label: 'Text' },
        { id: 'open-with-preview', label: 'Preview' },
      ],
    })
    const list = ids(items)
    assert.ok(list.includes('open-preview'), 'Open Preview present (VAL-IDE-026)')
    const openWith = items.find((i) => i.label.startsWith('Open With'))
    assert.ok(openWith, 'Open With present')
    assert.ok(openWith?.children && openWith.children.length === 2)
    assert.deepEqual(
      openWith?.children?.map((c) => c.id),
      ['open-with-edit', 'open-with-preview'],
    )
  })

  it('file with single mode omits Open With and Open in Integrated Terminal (VAL-IDE-029)', () => {
    const items = buildContextMenuItems({
      kind: 'file',
      path: 'main.go',
      hasMultipleOpenModes: false,
      canOpenPreview: false,
    })
    const list = ids(items)
    assert.ok(!list.includes('open-with-edit'))
    assert.ok(!list.includes('open-with-preview'))
    assert.ok(!list.includes('open-preview'))
    assert.ok(!list.includes('open-in-terminal'))
    assert.ok(!list.includes('find-in-folder'))
    assert.ok(!list.includes('paste'), 'Paste only on folders/root')
    assert.ok(list.includes('cut'))
    assert.ok(list.includes('copy'))
    assert.ok(!items.some((i) => i.label.startsWith('Open With')))
    assert.ok(list.includes('copy-path'))
    assert.ok(list.includes('rename'))
    assert.ok(list.includes('delete'))
  })

  it('suggestCopyName auto-renames on clobber', () => {
    const existing = new Set(['file.txt'])
    assert.equal(suggestCopyName('file.txt', existing), 'file copy.txt')
    existing.add('file copy.txt')
    assert.equal(suggestCopyName('file.txt', existing), 'file copy 2.txt')
  })

  it('renderable image exposes Open Preview without Open With', () => {
    const items = buildContextMenuItems({
      kind: 'file',
      path: 'shot.png',
      hasMultipleOpenModes: false,
      canOpenPreview: true,
    })
    const list = ids(items)
    assert.ok(list.includes('open-preview'))
    assert.ok(!items.some((i) => i.label.startsWith('Open With')))
  })

  it('path helpers', () => {
    assert.equal(parentRel('docs/a/b.txt'), 'docs/a')
    assert.equal(parentRel('file.txt'), '')
    assert.equal(baseName('docs/a/b.txt'), 'b.txt')
    assert.equal(joinRel('docs', 'n.md'), 'docs/n.md')
    assert.equal(joinRel('', 'n.md'), 'n.md')
  })
})
