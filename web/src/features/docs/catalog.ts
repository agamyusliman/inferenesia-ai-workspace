import { loadArticlesFromMarkdown } from './loadArticles'

export type DocCategoryId =
  | 'workspace'
  | 'chat'
  | 'safety'
  | 'plan'
  | 'git'
  | 'tools'
  | 'diagrams'
  | 'agent'
  | 'providers'
  | 'cases'

export type DocArticle = {
  slug: string
  title: string
  category: DocCategoryId
  summary: string
  keywords?: string
  body: string
  order?: number
}

export type DocCategory = {
  id: DocCategoryId
  label: string
}

export type DocsLocale = 'en' | 'id'

export const DOC_CATEGORIES: DocCategory[] = [
  { id: 'workspace', label: 'Workspaces & sessions' },
  { id: 'chat', label: 'Chat & models' },
  { id: 'safety', label: 'Undo & Revert' },
  { id: 'plan', label: 'Plan & Mission' },
  { id: 'git', label: 'Git' },
  { id: 'tools', label: 'Terminal, browser & shortcuts' },
  { id: 'diagrams', label: 'Diagrams & Playground' },
  { id: 'agent', label: 'Skills, MCP & memory' },
  { id: 'providers', label: 'Providers & context' },
  { id: 'cases', label: 'Case studies' },
]

const DOC_CATEGORIES_ID: DocCategory[] = [
  { id: 'workspace', label: 'Workspace & sesi' },
  { id: 'chat', label: 'Chat & model' },
  { id: 'safety', label: 'Undo & Revert' },
  { id: 'plan', label: 'Plan & Mission' },
  { id: 'git', label: 'Git' },
  { id: 'tools', label: 'Terminal, browser & pintasan' },
  { id: 'diagrams', label: 'Diagram & Playground' },
  { id: 'agent', label: 'Skills, MCP & memori' },
  { id: 'providers', label: 'Provider & konteks' },
  { id: 'cases', label: 'Studi kasus' },
]

export const DOC_ARTICLES: DocArticle[] = loadArticlesFromMarkdown('en')

export function getDocCategories(locale: DocsLocale = 'en'): DocCategory[] {
  return locale === 'id' ? DOC_CATEGORIES_ID : DOC_CATEGORIES
}

export function getDocArticles(locale: DocsLocale = 'en'): DocArticle[] {
  return loadArticlesFromMarkdown(locale)
}

function normalize(s: string): string {
  return s.trim().toLowerCase()
}

export function searchDocs(
  query: string,
  articles: DocArticle[] = DOC_ARTICLES,
): DocArticle[] {
  const q = normalize(query)
  if (!q) return articles
  return articles.filter((a) => {
    const hay = normalize(
      a.slug +
        ' ' +
        a.title +
        ' ' +
        a.summary +
        ' ' +
        (a.keywords || '') +
        ' ' +
        a.body,
    )
    return hay.includes(q)
  })
}

export function getDocBySlug(
  slug: string,
  articles: DocArticle[] = DOC_ARTICLES,
): DocArticle | undefined {
  const key = slug.trim()
  if (!key) return undefined
  return articles.find((a) => a.slug === key)
}

export function getDocsByCategory(
  category: DocCategoryId,
  articles: DocArticle[] = DOC_ARTICLES,
): DocArticle[] {
  return articles.filter((a) => a.category === category)
}

export function parseDocDeepLink(
  search: string | undefined | null,
): string | undefined {
  if (!search) return undefined
  const raw = search.startsWith('?') ? search.slice(1) : search
  if (!raw) return undefined
  try {
    const params = new URLSearchParams(raw)
    const doc = params.get('doc')
    if (!doc) return undefined
    const slug = doc.trim()
    return slug || undefined
  } catch {
    return undefined
  }
}
