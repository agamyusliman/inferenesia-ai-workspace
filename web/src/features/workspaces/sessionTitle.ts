export function deriveSessionTitle(raw: string, maxLen = 48): string {
  let s = (raw || '').replace(/\s+/g, ' ').trim()
  if (!s) return ''

  s = s.replace(/^\/(image|plan|mission)\s+/i, '')
  s = s.replace(/\[attachment:[^\]]+\]/gi, '').trim()
  s = s.replace(/^["'“”]+|["'“”]+$/g, '').trim()
  s = s
    .replace(/^(sure[,!]?\s+|of course[,!]?\s+|here(?:'s| is)\s+)/i, '')
    .replace(/^(baik[,!]?\s+|tentu[,!]?\s+|berikut\s+)/i, '')
    .trim()
  const heading = /^#+\s+(.+)$/m.exec(raw || '')
  if (heading?.[1]) {
    s = heading[1].replace(/\s+/g, ' ').trim()
  }
  if (!s) return ''

  const cut = s.search(/[.!?\n]/)
  if (cut > 12 && cut < maxLen + 20) {
    s = s.slice(0, cut).trim()
  }

  if (s.length <= maxLen) return s
  const slice = s.slice(0, maxLen)
  const sp = slice.lastIndexOf(' ')
  const base = sp > maxLen * 0.55 ? slice.slice(0, sp) : slice
  return base.replace(/[,:;.\-–—]+$/g, '').trim() + '…'
}

export function isPlaceholderSessionName(name: string | null | undefined): boolean {
  const n = (name || '').trim()
  if (!n) return true
  if (/^new chat$/i.test(n)) return true
  if (/^session$/i.test(n)) return true
  if (/^session\s+\d{1,2}:\d{2}/i.test(n)) return true
  if (/^sesi(\s+baru)?$/i.test(n)) return true
  return false
}
