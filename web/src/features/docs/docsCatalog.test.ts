import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  DOC_ARTICLES,
  DOC_CATEGORIES,
  getDocArticles,
  getDocBySlug,
  getDocCategories,
  getDocsByCategory,
  parseDocDeepLink,
  searchDocs,
} from './catalog'
import { listLoadedMarkdownPaths } from './loadArticles'

describe('DOC_CATEGORIES', () => {
  it('is non-empty with unique ids and no start category', () => {
    assert.ok(DOC_CATEGORIES.length >= 10)
    const ids = DOC_CATEGORIES.map((c) => c.id)
    assert.equal(new Set(ids).size, ids.length)
    assert.ok(!ids.includes('start' as never))
  })
})

describe('markdown articles', () => {
  it('loads en and id md files via glob', () => {
    const paths = listLoadedMarkdownPaths()
    assert.ok(paths.some((p) => p.includes('/en/')))
    assert.ok(paths.some((p) => p.includes('/id/')))
    assert.ok(paths.length >= 16)
  })

  it('has at least 12 EN articles with rich bodies', () => {
    assert.ok(DOC_ARTICLES.length >= 8)
    for (const a of DOC_ARTICLES) {
      assert.ok(a.slug.length > 0, 'slug')
      assert.ok(a.title.length > 0, 'title')
      assert.ok(a.body.length > 200, `body too short for ${a.slug}: ${a.body.length}`)
      assert.ok(!a.body.includes('Missing body'), a.slug)
      assert.notEqual(a.slug, 'getting-started')
    }
  })

  it('includes case studies guide', () => {
    const cases = DOC_ARTICLES.filter(
      (a) => a.category === 'cases' || a.slug.includes('case'),
    )
    assert.ok(cases.length >= 1, `expected ≥1 case studies guide, got ${cases.length}`)
  })
})

describe('searchDocs', () => {
  it('returns all for empty query', () => {
    assert.equal(searchDocs('').length, DOC_ARTICLES.length)
  })

  it('finds by keyword', () => {
    const hit = searchDocs('revert agent')
    assert.ok(hit.some((a) => a.slug === 'undo-revert'))
  })

  it('finds by body substring', () => {
    const hit = searchDocs('excalidraw')
    assert.ok(hit.some((a) => a.slug === 'diagrams-canvas' || a.slug === 'case-studies'))
  })

  it('returns empty when nothing matches', () => {
    assert.equal(searchDocs('zzz-no-such-doc-xyz').length, 0)
  })
})

describe('getDocBySlug', () => {
  it('returns article for known slug', () => {
    const a = getDocBySlug('undo-revert')
    assert.ok(a)
    assert.equal(a?.title.includes('Undo'), true)
  })

  it('returns undefined for missing slug', () => {
    assert.equal(getDocBySlug('nope'), undefined)
    assert.equal(getDocBySlug(''), undefined)
  })
})

describe('getDocsByCategory', () => {
  it('filters cases category', () => {
    const cases = getDocsByCategory('cases')
    assert.ok(cases.length >= 1)
    assert.ok(cases.every((a) => a.category === 'cases'))
  })
})

describe('parseDocDeepLink', () => {
  it('reads ?doc=slug', () => {
    assert.equal(parseDocDeepLink('?doc=undo-revert'), 'undo-revert')
    assert.equal(parseDocDeepLink('doc=undo-revert'), 'undo-revert')
    assert.equal(parseDocDeepLink('?foo=1&doc=git'), 'git')
  })

  it('returns undefined when absent', () => {
    assert.equal(parseDocDeepLink(''), undefined)
    assert.equal(parseDocDeepLink(null), undefined)
    assert.equal(parseDocDeepLink('?foo=1'), undefined)
  })
})

describe('locale id docs', () => {
  it('getDocArticles(id) has full Indonesian bodies', () => {
    const idArticles = getDocArticles('id')
    assert.ok(idArticles.length >= 8)
    assert.equal(idArticles.length, DOC_ARTICLES.length)
    for (const a of idArticles) {
      assert.ok(a.body.length > 80, `id body for ${a.slug}`)
      assert.ok(!a.body.includes('Missing body'), a.slug)
    }
  })

  it('getDocCategories(id) matches EN category ids', () => {
    const idCats = getDocCategories('id')
    assert.equal(idCats.length, DOC_CATEGORIES.length)
    const enIds = DOC_CATEGORIES.map((c) => c.id).join(',')
    const idIds = idCats.map((c) => c.id).join(',')
    assert.equal(idIds, enIds)
  })

  it('searchDocs uses active locale articles', () => {
    const idArticles = getDocArticles('id')
    const hit = searchDocs('revert agent', idArticles)
    assert.ok(hit.some((a) => a.slug === 'undo-revert'))
    const idHit = searchDocs('skill', idArticles)
    assert.ok(idHit.some((a) => a.slug === 'agent-tools'))
  })
})
