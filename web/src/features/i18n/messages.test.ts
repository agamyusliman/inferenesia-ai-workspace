import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  ACTION_MESSAGE_KEYS,
  en,
  id,
  isActionKey,
  type MessageKey,
} from './messages'
import { translate } from './t'

describe('catalog parity', () => {
  it('en and id have the same keys', () => {
    const enKeys = Object.keys(en).sort()
    const idKeys = Object.keys(id).sort()
    assert.deepEqual(idKeys, enKeys)
  })
})

describe('translate', () => {
  it('localizes descriptive keys in id', () => {
    assert.equal(translate('id', 'nav.settings'), 'Pengaturan')
    assert.equal(translate('id', 'settings.title'), 'Pengaturan')
    assert.equal(translate('id', 'settings.appearance'), 'Tampilan')
    assert.equal(translate('id', 'settings.language'), 'Bahasa')
    assert.equal(translate('id', 'docs.title'), 'Dokumentasi')
    assert.equal(translate('id', 'docs.search'), 'Cari…')
    assert.equal(translate('id', 'docs.all'), 'Semua')
    assert.equal(translate('id', 'status.ready'), 'Siap')
    assert.equal(translate('id', 'status.fetch'), 'Ambil')
    assert.equal(translate('id', 'status.pull'), 'Tarik')
    assert.equal(translate('id', 'status.gitGraph'), 'Grafik Git')
    assert.equal(translate('id', 'nav.docs'), 'Dokumentasi')
    assert.equal(translate('id', 'chat.interruptSend'), 'Interrupt · Kirim')
    assert.equal(translate('id', 'undo.hint'), 'Undo = dirty pra-agent')
  })

  it('keeps English for en locale descriptive keys', () => {
    assert.equal(translate('en', 'nav.settings'), 'Settings')
    assert.equal(translate('en', 'settings.appearance'), 'Appearance')
    assert.equal(translate('en', 'docs.title'), 'Documentation')
  })

  it('narrow chrome action keys stay English even in id locale', () => {
    assert.equal(translate('id', 'action.save'), 'Save')
    assert.equal(translate('id', 'action.undo'), 'Undo')
    assert.equal(translate('id', 'action.redo'), 'Redo')
    assert.equal(translate('id', 'action.close'), 'Close')
    assert.equal(translate('id', 'action.cancel'), 'Cancel')
    assert.equal(translate('id', 'action.copy'), 'Copy')
    assert.equal(translate('id', 'action.send'), 'Send')
    assert.equal(translate('id', 'action.stop'), 'Stop')
  })

  it('ACTION_MESSAGE_KEYS is only the 8 narrow chrome keys', () => {
    assert.equal(ACTION_MESSAGE_KEYS.length, 8)
    const expected = new Set([
      'action.save',
      'action.undo',
      'action.redo',
      'action.close',
      'action.cancel',
      'action.copy',
      'action.send',
      'action.stop',
    ])
    for (const key of ACTION_MESSAGE_KEYS) {
      assert.ok(expected.has(key), `unexpected force-EN key: ${key}`)
      assert.equal(translate('id', key), en[key])
      assert.equal(translate('en', key), en[key])
      assert.equal(id[key], en[key], `id catalog must keep EN for ${key}`)
    }
  })

  it('non-chrome action keys translate in id', () => {
    assert.equal(translate('id', 'action.delete'), 'Hapus')
    assert.equal(translate('id', 'action.saveAll'), 'Simpan semua')
    assert.equal(translate('id', 'action.apply'), 'Terapkan')
    assert.equal(translate('id', 'action.generate'), 'Buat')
    assert.equal(translate('id', 'action.open'), 'Buka')
    assert.equal(translate('id', 'action.refresh'), 'Segarkan')
    assert.equal(translate('id', 'action.test'), 'Uji')
    assert.equal(translate('id', 'action.add'), 'Tambah')
    assert.equal(translate('id', 'action.edit'), 'Ubah')
    assert.equal(translate('id', 'action.canvas'), 'Playground')
    assert.equal(translate('id', 'action.source'), 'Sumber')
    assert.equal(isActionKey('action.delete'), false)
    assert.equal(isActionKey('action.canvas'), false)
  })

  it('product terms stay EN in id catalog without force list', () => {
    assert.equal(translate('id', 'action.revertAgent'), 'Revert agent')
    assert.equal(translate('id', 'action.restoreHead'), 'Restore to HEAD')
    assert.equal(isActionKey('action.revertAgent'), false)
    assert.equal(isActionKey('action.restoreHead'), false)
  })

  it('interpolates {vars}', () => {
    assert.equal(
      translate('en', 'status.themeCycle', { name: 'Dark' }),
      'Theme: Dark (click to cycle)',
    )
    assert.equal(
      translate('id', 'status.themeCycle', { name: 'Dark' }),
      'Tema: Dark (klik untuk ganti)',
    )
    assert.equal(
      translate('id', 'explorer.confirmDelete', { name: 'foo.ts' }),
      'Hapus “foo.ts”?',
    )
  })

  it('palette command labels localize in id', () => {
    assert.equal(translate('id', 'palette.closeTab'), 'Tutup Tab')
    assert.equal(translate('id', 'palette.newFile'), 'File Baru')
    assert.equal(translate('id', 'palette.openDocs'), 'Buka Dokumentasi')
  })

  it('git / workflow / layout / settings saver keys localize in id', () => {
    assert.equal(translate('id', 'git.noStaged'), 'Tidak ada file staged')
    assert.equal(translate('id', 'git.selectFileDiff'), 'Pilih file untuk melihat diff')
    assert.equal(translate('id', 'git.commitMessage'), 'Pesan commit')
    assert.equal(translate('id', 'workflow.defaultBranch'), 'Branch default')
    assert.equal(translate('id', 'workflow.editRepoOnly'), 'Edit aturan hanya untuk repo saat ini')
    assert.equal(translate('id', 'layout.reset'), 'Reset layout')
    assert.equal(translate('id', 'layout.done'), 'Selesai')
    assert.equal(translate('id', 'explorer.filterPlaceholder'), 'Filter file…')
    assert.ok(translate('id', 'workspace.folderHint').includes('Workspace'))
    assert.equal(translate('id', 'settings.ready'), 'Siap')
    assert.equal(translate('id', 'settings.notInstalled'), 'Not installed')
    assert.ok(translate('id', 'settings.saverRtkDesc').includes('Kompres'))
    assert.equal(translate('id', 'docs.cat.cases'), 'Studi kasus')
  })
})

describe('isActionKey', () => {
  it('detects only forced chrome action keys', () => {
    assert.equal(isActionKey('action.save'), true)
    assert.equal(isActionKey('action.delete'), false)
    assert.equal(isActionKey('nav.settings'), false)
    assert.equal(isActionKey('settings.title'), false)
  })

  it('ACTION_MESSAGE_KEYS are all action keys', () => {
    for (const key of ACTION_MESSAGE_KEYS) {
      assert.equal(isActionKey(key), true)
      assert.ok(key.startsWith('action.'))
    }
  })
})

describe('MessageKey coverage', () => {
  it('every en key is a MessageKey string', () => {
    const sample: MessageKey = 'action.save'
    assert.equal(en[sample], 'Save')
  })
})
