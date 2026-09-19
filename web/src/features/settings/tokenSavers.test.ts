import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  allSaversDefaultOff,
  applyLocalToggle,
  findSaver,
  isNotInstalled,
  isTokenSaverId,
  saverStatusLabel,
  TOKEN_SAVER_IDS,
} from './tokenSavers'
import type { TokenSaversView } from '../../lib/api'

const sample: TokenSaversView = {
  product: 'Inferenesia',
  config_home: '/tmp/inferenesia',
  savers: [
    {
      id: 'rtk',
      name: 'RTK',
      description: 'compress tool output',
      enabled: false,
      installed: false,
      status: 'not_installed',
      detail: 'not installed',
    },
    {
      id: 'headroom',
      name: 'Headroom',
      description: 'compress prompts',
      enabled: false,
      installed: false,
      status: 'not_installed',
    },
    {
      id: 'caveman',
      name: 'Caveman',
      description: 'terse output',
      enabled: false,
      installed: true,
      status: 'ready',
    },
    {
      id: 'ponytail',
      name: 'Ponytail',
      description: 'minimal code',
      enabled: false,
      installed: true,
      status: 'ready',
    },
  ],
}

describe('tokenSavers helpers (VAL-SAVER-001/007)', () => {
  it('lists four saver ids', () => {
    assert.deepEqual([...TOKEN_SAVER_IDS], ['rtk', 'headroom', 'caveman', 'ponytail'])
    assert.equal(isTokenSaverId('rtk'), true)
    assert.equal(isTokenSaverId('nope'), false)
  })

  it('shows Not installed when missing (VAL-SAVER-001)', () => {
    assert.equal(saverStatusLabel(sample.savers[0]), 'Not installed')
    assert.equal(isNotInstalled(sample.savers[0]), true)
    assert.equal(saverStatusLabel(sample.savers[2]), 'Ready')
    assert.equal(isNotInstalled(sample.savers[2]), false)
  })

  it('accepts optional labels (defaults remain EN)', () => {
    assert.equal(
      saverStatusLabel(sample.savers[0], {
        ready: 'Siap',
        notInstalled: 'Belum terpasang',
      }),
      'Belum terpasang',
    )
    assert.equal(
      saverStatusLabel(sample.savers[2], {
        ready: 'Siap',
        notInstalled: 'Belum terpasang',
      }),
      'Siap',
    )
    assert.equal(saverStatusLabel(sample.savers[0]), 'Not installed')
  })

  it('defaults all off', () => {
    assert.equal(allSaversDefaultOff(sample), true)
  })

  it('applies independent local toggle without crashing when not installed', () => {
    const next = applyLocalToggle(sample, 'rtk', true)
    assert.ok(next)
    assert.equal(findSaver(next, 'rtk')?.enabled, true)
    assert.equal(findSaver(next, 'headroom')?.enabled, false)
    // Status unchanged — missing install still Not installed
    assert.equal(saverStatusLabel(findSaver(next, 'rtk')!), 'Not installed')
  })
})
