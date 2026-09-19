import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  canvasThemeFor,
  monacoThemeFor,
  nextThemeId,
  parseThemeId,
  themeLabel,
  THEME_IDS,
  THEME_STORAGE_KEY,
  THEME_STORAGE_KEY_LEGACY,
} from './theme'

describe('parseThemeId', () => {
  it('returns dark for null/undefined/empty', () => {
    assert.equal(parseThemeId(null), 'dark')
    assert.equal(parseThemeId(undefined), 'dark')
    assert.equal(parseThemeId(''), 'dark')
    assert.equal(parseThemeId('   '), 'dark')
  })

  it('accepts warm, dark, light (case-insensitive)', () => {
    assert.equal(parseThemeId('warm'), 'warm')
    assert.equal(parseThemeId('dark'), 'dark')
    assert.equal(parseThemeId('light'), 'light')
    assert.equal(parseThemeId('WARM'), 'warm')
    assert.equal(parseThemeId(' Dark '), 'dark')
    assert.equal(parseThemeId('LIGHT'), 'light')
  })

  it('falls back to dark for invalid values', () => {
    assert.equal(parseThemeId('purple'), 'dark')
    assert.equal(parseThemeId('system'), 'dark')
    assert.equal(parseThemeId('0'), 'dark')
  })
})

describe('themeLabel', () => {
  it('returns human labels', () => {
    assert.equal(themeLabel('warm'), 'Warm')
    assert.equal(themeLabel('dark'), 'Dark')
    assert.equal(themeLabel('light'), 'Light')
  })
})

describe('THEME_IDS + storage key', () => {
  it('lists warm, dark, light', () => {
    assert.deepEqual(THEME_IDS, ['warm', 'dark', 'light'])
  })

  it('uses inferenesia-theme storage key with legacy yura-ai-theme', () => {
    assert.equal(THEME_STORAGE_KEY, 'inferenesia-theme')
    assert.equal(THEME_STORAGE_KEY_LEGACY, 'yura-ai-theme')
  })
})

describe('canvasThemeFor / monacoThemeFor', () => {
  it('maps light shell to light surfaces', () => {
    assert.equal(canvasThemeFor('light'), 'light')
    assert.equal(monacoThemeFor('light'), 'light')
  })

  it('maps warm and dark shell to dark surfaces', () => {
    assert.equal(canvasThemeFor('warm'), 'dark')
    assert.equal(canvasThemeFor('dark'), 'dark')
    assert.equal(monacoThemeFor('warm'), 'vs-dark')
    assert.equal(monacoThemeFor('dark'), 'vs-dark')
  })
})

describe('nextThemeId', () => {
  it('cycles warm → dark → light → warm', () => {
    assert.equal(nextThemeId('warm'), 'dark')
    assert.equal(nextThemeId('dark'), 'light')
    assert.equal(nextThemeId('light'), 'warm')
  })
})
