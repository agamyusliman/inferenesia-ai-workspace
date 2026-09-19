export type HostOS = 'macos' | 'windows' | 'linux' | 'unknown'

let osOverride: HostOS | null = null

export function setHostOSForTests(os: HostOS | null): void {
  osOverride = os
}

export function getHostOS(): HostOS {
  if (osOverride) return osOverride
  if (typeof navigator === 'undefined') return 'unknown'

  const uaData = (
    navigator as Navigator & {
      userAgentData?: { platform?: string }
    }
  ).userAgentData
  const fromUaData = (uaData?.platform || '').toLowerCase()
  if (fromUaData.includes('mac')) return 'macos'
  if (fromUaData.includes('win')) return 'windows'
  if (fromUaData.includes('linux')) return 'linux'
  if (fromUaData.includes('android')) return 'linux'
  if (fromUaData.includes('cros')) return 'linux'

  const ua = (navigator.userAgent || '').toLowerCase()
  if (ua.includes('mac os') || ua.includes('macintosh')) return 'macos'
  if (ua.includes('windows')) return 'windows'
  if (ua.includes('linux') || ua.includes('x11')) return 'linux'
  if (ua.includes('android')) return 'linux'
  if (ua.includes('cros')) return 'linux'

  const plat = (navigator.platform || '').toLowerCase()
  if (plat.startsWith('mac') || plat.includes('mac')) return 'macos'
  if (plat.startsWith('win')) return 'windows'
  if (plat.includes('linux')) return 'linux'

  return 'unknown'
}

export function isMacOS(): boolean {
  return getHostOS() === 'macos'
}

export function modKeyLabel(style: 'symbol' | 'text' = 'text'): string {
  if (isMacOS()) return style === 'symbol' ? '⌘' : 'Cmd'
  return style === 'symbol' ? 'Ctrl' : 'Ctrl'
}

export function altKeyLabel(style: 'symbol' | 'text' = 'text'): string {
  if (isMacOS()) return style === 'symbol' ? '⌥' : 'Option'
  return style === 'symbol' ? 'Alt' : 'Alt'
}

export function controlKeyLabel(style: 'symbol' | 'text' = 'text'): string {
  if (isMacOS()) return style === 'symbol' ? '⌃' : 'Ctrl'
  return style === 'symbol' ? 'Ctrl' : 'Ctrl'
}

export function isPrimaryModKey(e: {
  metaKey: boolean
  ctrlKey: boolean
}): boolean {
  return isMacOS() ? e.metaKey : e.ctrlKey
}

export function isSaveKeyEvent(e: {
  key: string
  code?: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  shiftKey: boolean
}): boolean {
  if (e.altKey || e.shiftKey) return false
  if (!isPrimaryModKey(e)) return false
  const k = (e.key || '').toLowerCase()
  return k === 's' || e.code === 'KeyS'
}

export function formatShortcut(
  key: string,
  opts?: {
    alt?: boolean
    shift?: boolean
    forceCtrl?: boolean
    style?: 'symbol' | 'text'
  },
): string {
  const style = opts?.style ?? (isMacOS() ? 'symbol' : 'text')
  const keyUp = key.length === 1 ? key.toUpperCase() : key
  const useMac = isMacOS() && !opts?.forceCtrl

  if (useMac && style === 'symbol') {
    const parts: string[] = []
    if (opts?.shift) parts.push('⇧')
    if (opts?.alt) parts.push('⌥')
    parts.push(opts?.forceCtrl ? '⌃' : '⌘')
    parts.push(keyUp)
    return parts.join('')
  }

  const parts: string[] = []
  parts.push(opts?.forceCtrl || !isMacOS() ? 'Ctrl' : 'Cmd')
  if (opts?.alt) parts.push(isMacOS() ? 'Option' : 'Alt')
  if (opts?.shift) parts.push('Shift')
  parts.push(keyUp)
  return parts.join('+')
}

export function fileManagerName(): string {
  switch (getHostOS()) {
    case 'windows':
      return 'Explorer'
    case 'linux':
      return 'Files'
    case 'macos':
      return 'Finder'
    default:
      return 'file manager'
  }
}

export function revealInOSLabel(): string {
  switch (getHostOS()) {
    case 'windows':
      return 'Reveal in Explorer'
    case 'linux':
      return 'Reveal in Files'
    case 'macos':
      return 'Reveal in Finder'
    default:
      return 'Reveal in file manager'
  }
}

export function platformMessageVars(): Record<string, string> {
  return {
    mod: modKeyLabel('text'),
    modSym: modKeyLabel('symbol'),
    alt: altKeyLabel('text'),
    altSym: altKeyLabel('symbol'),
    ctrl: controlKeyLabel('text'),
    fileManager: fileManagerName(),
    reveal: revealInOSLabel(),
    saveShortcut: formatShortcut('S'),
    saveAllShortcut: formatShortcut('S', { alt: true }),
    closeTabShortcut: formatShortcut('W'),
    paletteShortcut: formatShortcut('P', { shift: true }),
    terminalShortcut: formatShortcut('`', { forceCtrl: true }),
  }
}
