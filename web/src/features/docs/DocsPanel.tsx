import { useEffect, useMemo, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useLocale } from '../i18n/LocaleProvider'
import type { LocaleId } from '../i18n/locale'
import type { MessageKey } from '../i18n/messages'
import { PanelHeader } from '../shell/PanelHeader'
import {
  getDocArticles,
  getDocBySlug,
  getDocCategories,
  searchDocs,
  type DocArticle,
  type DocCategoryId,
  type DocsLocale,
} from './catalog'

type Props = {
  initialSlug?: string
}

function toDocsLocale(locale: LocaleId): DocsLocale {
  return locale === 'id' ? 'id' : 'en'
}

export function DocsPanel({ initialSlug }: Props) {
  const { t, locale } = useLocale()
  const docsLocale = toDocsLocale(locale)
  const articles = useMemo(() => getDocArticles(docsLocale), [docsLocale])
  const categories = useMemo(() => getDocCategories(docsLocale), [docsLocale])

  const seed = initialSlug ? getDocBySlug(initialSlug, articles) : undefined
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState<DocCategoryId>(
    seed?.category ?? categories[0]?.id ?? 'workspace',
  )
  const [activeSlug, setActiveSlug] = useState(
    seed?.slug ?? articles.find((a) => a.category === (seed?.category ?? categories[0]?.id))?.slug ?? articles[0]?.slug ?? '',
  )

  const categoryArticles = useMemo(
    () => articles.filter((a) => a.category === category),
    [articles, category],
  )

  const searchHits = useMemo(() => {
    const q = query.trim()
    if (!q) return null
    return searchDocs(q, articles)
  }, [query, articles])

  useEffect(() => {
    if (searchHits) {
      if (searchHits.length > 0 && !searchHits.some((a) => a.slug === activeSlug)) {
        setActiveSlug(searchHits[0].slug)
        setCategory(searchHits[0].category)
      }
      return
    }
    if (categoryArticles.length === 0) return
    if (!categoryArticles.some((a) => a.slug === activeSlug)) {
      setActiveSlug(categoryArticles[0].slug)
    }
  }, [searchHits, categoryArticles, activeSlug])

  const active: DocArticle | undefined =
    getDocBySlug(activeSlug, articles) ||
    categoryArticles[0] ||
    articles[0]

  const selectCategory = (id: DocCategoryId) => {
    setQuery('')
    setCategory(id)
    const first = articles.find((a) => a.category === id)
    if (first) setActiveSlug(first.slug)
  }


  return (
    <div
      data-testid="docs-panel"
      className="flex h-full min-h-0 w-full min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader
        testId="docs-panel-header"
        title={t('docs.title')}
        subtitle={t('docs.subtitle')}
      />

      <div className="flex min-h-0 flex-1 overflow-hidden">
        <aside
          className="flex w-[220px] shrink-0 flex-col border-r border-shell-border bg-shell-panel"
          data-testid="docs-sidebar"
        >
          <div className="shrink-0 border-b border-shell-border p-2">
            <input
              data-testid="docs-search"
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('docs.search')}
              aria-label={t('docs.searchAria')}
              className="w-full rounded border border-shell-border bg-shell-bg px-2 py-1.5 text-[11px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
            />
          </div>

          {searchHits ? (
            <ul
              className="min-h-0 flex-1 list-none overflow-y-auto p-1.5"
              data-testid="docs-search-results"
            >
              {searchHits.length === 0 ? (
                <li className="px-2 py-3 text-[11px] text-shell-muted">
                  {t('docs.noMatch')}
                </li>
              ) : (
                searchHits.map((a) => {
                  const isActive = active?.slug === a.slug
                  return (
                    <li key={a.slug}>
                      <button
                        type="button"
                        data-testid={`docs-article-${a.slug}`}
                        data-active={isActive ? 'true' : 'false'}
                        onClick={() => {
                          setActiveSlug(a.slug)
                          setCategory(a.category)
                        }}
                        className={`mb-0.5 w-full rounded px-2 py-1.5 text-left text-[11px] font-medium transition ${
                          isActive
                            ? 'bg-shell-active text-shell-text'
                            : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
                        }`}
                      >
                        {a.title}
                      </button>
                    </li>
                  )
                })
              )}
            </ul>
          ) : (
            <nav
              className="min-h-0 flex-1 space-y-0.5 overflow-y-auto p-1.5"
              data-testid="docs-categories"
            >
              {categories.map((c) => {
                const activeCat = category === c.id
                return (
                  <button
                    key={c.id}
                    type="button"
                    data-testid={`docs-category-${c.id}`}
                    data-active={activeCat ? 'true' : 'false'}
                    onClick={() => selectCategory(c.id)}
                    className={`w-full rounded px-2 py-1.5 text-left text-[11px] font-medium transition ${
                      activeCat
                        ? 'bg-shell-active text-shell-text'
                        : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
                    }`}
                  >
                    {t(`docs.cat.${c.id}` as MessageKey)}
                  </button>
                )
              })}
            </nav>
          )}
        </aside>

        <main className="min-h-0 min-w-0 flex-1 overflow-y-auto bg-shell-bg">
          {active ? (
            <article className="mx-auto w-full max-w-3xl px-5 py-5 md:px-8 md:py-6">
              {!searchHits && categoryArticles.length > 1 && (
                <div
                  className="mb-4 flex flex-wrap gap-1"
                  data-testid="docs-category-articles"
                >
                  {categoryArticles.map((a) => {
                    const on = a.slug === active.slug
                    return (
                      <button
                        key={a.slug}
                        type="button"
                        data-testid={`docs-sub-${a.slug}`}
                        onClick={() => setActiveSlug(a.slug)}
                        className={`rounded border px-2 py-1 text-[11px] transition ${
                          on
                            ? 'border-shell-accent bg-shell-accent/15 text-shell-text'
                            : 'border-shell-border text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
                        }`}
                      >
                        {a.title}
                      </button>
                    )
                  })}
                </div>
              )}
              <div
                data-testid="docs-body"
                data-slug={active.slug}
                className="markdown-preview text-[13px] leading-relaxed text-shell-text"
              >
                <ReactMarkdown
                  remarkPlugins={[remarkGfm]}
                  components={{
                    a: ({ href, children, ...props }) => {
                      const external =
                        typeof href === 'string' &&
                        /^(https?:|mailto:)/i.test(href)
                      return (
                        <a
                          href={href}
                          {...props}
                          {...(external
                            ? {
                                target: '_blank',
                                rel: 'noopener noreferrer',
                              }
                            : {})}
                        >
                          {children}
                        </a>
                      )
                    },
                  }}
                >
                  {active.body}
                </ReactMarkdown>
              </div>
            </article>
          ) : (
            <div className="flex h-full items-center justify-center p-8 text-[12px] text-shell-muted">
              {t('docs.selectArticle')}
            </div>
          )}
        </main>
      </div>
    </div>
  )
}
