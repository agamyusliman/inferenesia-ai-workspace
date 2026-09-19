import { readdirSync, readFileSync, writeFileSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const docsRoot = join(here, '..')

function loadLocale(locale) {
  const dir = join(docsRoot, 'articles', locale)
  if (!existsSync(dir)) return []
  const arts = []
  for (const name of readdirSync(dir)) {
    if (!name.endsWith('.md')) continue
    const raw = readFileSync(join(dir, name), 'utf8')
    if (!raw.startsWith('---')) continue
    const end = raw.indexOf('\n---', 3)
    if (end < 0) continue
    const fm = raw.slice(3, end).trim()
    let body = raw.slice(end + 4)
    if (body.startsWith('\n')) body = body.slice(1)
    const meta = {}
    for (const line of fm.split('\n')) {
      const m = line.match(/^([A-Za-z0-9_-]+):\s*(.*)$/)
      if (!m) continue
      let v = m[2].trim()
      if (
        (v.startsWith('"') && v.endsWith('"')) ||
        (v.startsWith("'") && v.endsWith("'"))
      ) {
        v = v.slice(1, -1)
      }
      meta[m[1]] = v
    }
    arts.push({
      slug: name.replace(/\.md$/, ''),
      title: meta.title || name,
      category: meta.category || '',
      summary: meta.summary || '',
      keywords: meta.keywords || '',
      order: Number(meta.order) || 100,
      body: body.endsWith('\n') ? body : body + '\n',
    })
  }
  arts.sort((a, b) => a.order - b.order || a.slug.localeCompare(b.slug))
  return arts
}

function esc(s) {
  return JSON.stringify(s)
}

function emit(name, arts) {
  let s = `const ${name}: RawArticle[] = [\n`
  for (const a of arts) {
    s += '  {\n'
    s += `    slug: ${esc(a.slug)},\n`
    s += `    title: ${esc(a.title)},\n`
    s += `    category: ${esc(a.category)} as DocCategoryId,\n`
    s += `    summary: ${esc(a.summary)},\n`
    if (a.keywords) s += `    keywords: ${esc(a.keywords)},\n`
    s += `    order: ${a.order},\n`
    s += `    body: ${esc(a.body)},\n`
    s += '  },\n'
  }
  s += ']\n\n'
  return s
}

const en = loadLocale('en')
const id = loadLocale('id')
let out = `import type { DocArticle, DocCategoryId, DocsLocale } from './catalog'

type RawArticle = DocArticle & { order?: number }

const CATEGORY_IDS = new Set<string>([
  'workspace',
  'chat',
  'safety',
  'plan',
  'git',
  'tools',
  'diagrams',
  'agent',
  'providers',
  'cases',
])

// Generated from articles/{en,id}/*.md — run: node src/features/docs/scripts/generate-articles.mjs
// Source of truth: Markdown files (rich tables/code). Do not hand-edit this file.

`
out += emit('ARTICLES_EN', en)
out += emit('ARTICLES_ID', id)
out += `function stripOrder(list: RawArticle[]): DocArticle[] {
  return list
    .filter((a) => CATEGORY_IDS.has(a.category))
    .map(({ order: _o, ...rest }) => {
      void _o
      return rest
    })
}

export function loadArticlesFromMarkdown(locale: DocsLocale): DocArticle[] {
  return stripOrder(locale === 'id' ? ARTICLES_ID : ARTICLES_EN)
}

export function listLoadedMarkdownPaths(): string[] {
  const paths: string[] = []
  for (const a of ARTICLES_EN) paths.push(\`./articles/en/\${a.slug}.md\`)
  for (const a of ARTICLES_ID) paths.push(\`./articles/id/\${a.slug}.md\`)
  return paths.sort()
}
`
writeFileSync(join(docsRoot, 'loadArticles.ts'), out)
console.log('generated', en.length, 'en +', id.length, 'id → loadArticles.ts')
