/**
 * Display-safe base URL helper for the providers panel (VAL-PROV-001).
 * Strips credentials from userinfo/query when the API already sent a masked URL,
 * and falls back to host-only when only base_host is available.
 */
export function displayBaseURL(
  masked?: string,
  host?: string,
): string {
  const m = (masked || '').trim()
  if (m) {
    // Defensive strip if a raw URL with userinfo ever slips through.
    try {
      const u = new URL(m)
      u.username = ''
      u.password = ''
      u.search = ''
      u.hash = ''
      return u.toString().replace(/\/$/, '')
    } catch {
      return m.replace(/\/\/[^@/]+@/, '//')
    }
  }
  return (host || '').trim() || '—'
}

/** Mask a key for optional one-time confirm UI (never re-display full secrets). */
export function maskKeyHint(key: string): string {
  const k = key.trim()
  if (!k) return ''
  if (k.length <= 8) return '••••'
  return `${k.slice(0, 4)}…${k.slice(-2)}`
}
