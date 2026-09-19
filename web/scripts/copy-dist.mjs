import { cpSync, mkdirSync, existsSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const webRoot = join(__dirname, '..')
const dist = join(webRoot, 'dist')
const desktopDist = join(webRoot, '..', 'cmd', 'desktop', 'frontend', 'dist')

if (!existsSync(dist)) {
  console.error('web/dist missing; run vite build first')
  process.exit(1)
}

mkdirSync(dirname(desktopDist), { recursive: true })
rmSync(desktopDist, { recursive: true, force: true })
cpSync(dist, desktopDist, { recursive: true })
console.log('copied web/dist → cmd/desktop/frontend/dist')
