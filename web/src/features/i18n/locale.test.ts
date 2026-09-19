import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  LOCALE_IDS,
  LOCALE_STORAGE_KEY,
  LOCALE_STORAGE_KEY_LEGACY,
  localeLabel,
  parseLocaleId,
} from './locale'

describe('parseLocaleId', () => {
  it('returns en for null/undefined/empty', () => {
    assert.equal(parseLocaleId(null), 'en')
    assert.equal(parseLocaleId(undefined), 'en')
    assert.equal(parseLocaleId(''), 'en')
    assert.equal(parseLocaleId('   '), 'en')
  })

  it('accepts en and id (case-insensitive)', () => {
    assert.equal(parseLocaleId('en'), 'en')
    assert.equal(parseLocaleId('id'), 'id')
    assert.equal(parseLocaleId('EN'), 'en')
    assert.equal(parseLocaleId(' Id '), 'id')
  })

  it('accepts common aliases', () => {
    assert.equal(parseLocaleId('en-US'), 'en')
    assert.equal(parseLocaleId('id-ID'), 'id')
    assert.equal(parseLocaleId('indonesian'), 'id')
    assert.equal(parseLocaleId('english'), 'en')
  })

  it('falls back to en for invalid values', () => {
    assert.equal(parseLocaleId('fr'), 'en')
    assert.equal(parseLocaleId('jp'), 'en')
    assert.equal(parseLocaleId('0'), 'en')
  })
})

describe('localeLabel', () => {
  it('returns native language names', () => {
    assert.equal(localeLabel('en'), 'English')
    assert.equal(localeLabel('id'), 'Indonesia')
  })
})

describe('LOCALE_IDS + storage key', () => {
  it('lists en and id', () => {
    assert.deepEqual(LOCALE_IDS, ['en', 'id'])
  })

  it('uses inferenesia-locale storage key with legacy yura-ai-locale', () => {
    assert.equal(LOCALE_STORAGE_KEY, 'inferenesia-locale')
    assert.equal(LOCALE_STORAGE_KEY_LEGACY, 'yura-ai-locale')
  })
})
