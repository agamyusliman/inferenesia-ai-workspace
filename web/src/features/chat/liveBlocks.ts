/**
 * Live Blocks helpers (P9-live) — pure functions, no React, so node:test can
 * run without a DOM. Kept in sync with internal/config/live_blocks.go
 * (IsLiveBlockLanguage) and internal/core/context_discipline.go
 * (liveBlockFenceRe).
 */

export const LIVE_BLOCK_CDN_HOSTS = [
  'cdn.jsdelivr.net',
  'cdnjs.cloudflare.com',
  'unpkg.com',
  'esm.sh',
  'cdn.tailwindcss.com',
  'fonts.googleapis.com',
  'fonts.gstatic.com',
] as const

const LIVE_BLOCK_LANGS = new Set(['live-block', 'yura-live', 'html:preview'])

export function isLiveBlockLanguage(lang: string | null | undefined): boolean {
  if (!lang) return false
  return LIVE_BLOCK_LANGS.has(lang.trim().toLowerCase())
}

/**
 * Build a sandboxed srcdoc for a Live Block. The document:
 *  - injects a strict CSP <meta> restricting scripts/styles to self + the
 *    CDN allowlist (so a compromised widget cannot exfiltrate via arbitrary
 *    origins);
 *  - never grants allow-same-origin in the iframe sandbox (caller passes
 *    only `allow-scripts`), so the iframe cannot reach parent DOM/cookies/
 *    localStorage;
 *  - preserves the user's HTML body verbatim.
 *
 * The CSP is defense-in-depth on top of the iframe sandbox attribute — if a
 * future browser relaxes sandbox semantics, the CSP still blocks outbound
 * fetches to non-allowlisted hosts.
 */
export function buildLiveBlockSrcDoc(html: string): string {
  const body = typeof html === 'string' ? html : ''
  const csp = buildLiveBlockCSP()
  return (
    '<!DOCTYPE html>\n' +
    '<html lang="en">\n' +
    '<head>\n' +
    '<meta charset="utf-8">\n' +
    `<meta http-equiv="Content-Security-Policy" content="${csp}">\n` +
    '<meta name="viewport" content="width=device-width, initial-scale=1">\n' +
    '<style>html,body{margin:0;padding:0;font-family:ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;background:transparent;color:#0b1016}body{padding:8px}</style>\n' +
    '</head>\n' +
    '<body>\n' +
    body +
    '\n</body>\n</html>\n'
  )
}

export function buildLiveBlockCSP(): string {
  const scriptSrc = ["'self'", ...LIVE_BLOCK_CDN_HOSTS.map((h) => `https://${h}`)].join(' ')
  const styleSrc = ["'self'", "'unsafe-inline'", ...LIVE_BLOCK_CDN_HOSTS.map((h) => `https://${h}`)].join(' ')
  const imgSrc = ["'self'", 'data:', ...LIVE_BLOCK_CDN_HOSTS.map((h) => `https://${h}`)].join(' ')
  return [
    `default-src 'self'`,
    `script-src ${scriptSrc}`,
    `style-src ${styleSrc}`,
    `img-src ${imgSrc}`,
    `font-src https://${LIVE_BLOCK_CDN_HOSTS.join(' https://')}`,
    `connect-src 'none'`,
    `base-uri 'none'`,
    `form-action 'none'`,
  ].join('; ')
}
