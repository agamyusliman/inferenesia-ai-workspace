export type MermaidThemeId = 'dark' | 'warm' | 'print'

export function mermaidThemeFromAppTheme(
  app: 'warm' | 'dark' | 'light' | string | null | undefined,
): MermaidThemeId {
  const t = String(app || '').trim().toLowerCase()
  if (t === 'light') return 'print'
  if (t === 'warm') return 'warm'
  return 'dark'
}

export function normalizeMermaidSource(raw: string): string {
  let s = (raw || '').trim()
  if (!s) return ''
  s = s.replace(/^```(?:mermaid|mmd)?\s*\n?/i, '')
  s = s.replace(/\n?```\s*$/i, '')
  return s.trim()
}

export function isLikelyIncompleteMermaid(source: string): boolean {
  const s = normalizeMermaidSource(source)
  if (!s) return true
  if (s.length < 12) return true
  const open = (s.match(/subgraph\b/gi) || []).length
  const close = (s.match(/\bend\b/gi) || []).length
  if (open > close) return true
  if (/-->\s*$/.test(s) || /--\s*$/.test(s)) return true
  return false
}

export function mermaidStageBg(theme: MermaidThemeId): string {
  if (theme === 'print') return '#ffffff'
  if (theme === 'warm') return '#1a1816'
  return '#0b1016'
}

export function mermaidJpgBg(theme: MermaidThemeId): string {
  return mermaidStageBg(theme)
}

let mermaidMod: typeof import('mermaid') | null = null
let activeTheme: MermaidThemeId | null = null
let seq = 0

function themeConfig(theme: MermaidThemeId) {
  if (theme === 'print') {
    return {
      startOnLoad: false as const,
      securityLevel: 'strict' as const,
      theme: 'base' as const,
      fontFamily: 'ui-sans-serif, system-ui, sans-serif',
      themeVariables: {
        darkMode: false,
        background: '#ffffff',
        primaryColor: '#ffffff',
        primaryTextColor: '#111111',
        primaryBorderColor: '#111111',
        secondaryColor: '#f3f4f6',
        tertiaryColor: '#ffffff',
        lineColor: '#111111',
        textColor: '#111111',
        mainBkg: '#ffffff',
        nodeBorder: '#111111',
        clusterBkg: '#f9fafb',
        clusterBorder: '#111111',
        titleColor: '#111111',
        edgeLabelBackground: '#ffffff',
        actorBkg: '#ffffff',
        actorBorder: '#111111',
        actorTextColor: '#111111',
        signalColor: '#111111',
        signalTextColor: '#111111',
        labelBoxBkgColor: '#ffffff',
        labelTextColor: '#111111',
        loopTextColor: '#111111',
        noteBkgColor: '#f9fafb',
        noteTextColor: '#111111',
        noteBorderColor: '#111111',
      },
      flowchart: {
        htmlLabels: false,
        curve: 'basis' as const,
      },
    }
  }

  if (theme === 'warm') {
    return {
      startOnLoad: false as const,
      securityLevel: 'strict' as const,
      theme: 'base' as const,
      fontFamily: 'ui-sans-serif, system-ui, sans-serif',
      themeVariables: {
        darkMode: true,
        background: '#1a1816',
        primaryColor: '#3a3028',
        primaryTextColor: '#e8e4de',
        primaryBorderColor: '#d4845a',
        secondaryColor: '#2a2420',
        tertiaryColor: '#201e1b',
        lineColor: '#d4845a',
        textColor: '#e8e4de',
        mainBkg: '#2a2420',
        nodeBorder: '#d4845a',
        clusterBkg: '#201e1b',
        clusterBorder: '#3d3834',
        titleColor: '#e8e4de',
        edgeLabelBackground: '#1a1816',
        actorBkg: '#2a2420',
        actorBorder: '#d4845a',
        actorTextColor: '#e8e4de',
        signalColor: '#d4845a',
        signalTextColor: '#e8e4de',
        labelBoxBkgColor: '#2a2420',
        labelTextColor: '#e8e4de',
        loopTextColor: '#e8e4de',
        noteBkgColor: '#3a3028',
        noteTextColor: '#e8e4de',
        noteBorderColor: '#d4845a',
      },
      flowchart: {
        htmlLabels: false,
        curve: 'basis' as const,
      },
    }
  }

  return {
    startOnLoad: false as const,
    securityLevel: 'strict' as const,
    theme: 'dark' as const,
    fontFamily: 'ui-sans-serif, system-ui, sans-serif',
    flowchart: {
      htmlLabels: false,
      curve: 'basis' as const,
    },
  }
}

export async function getMermaid(theme: MermaidThemeId = 'dark') {
  if (!mermaidMod) {
    mermaidMod = await import('mermaid')
  }
  const mermaid = mermaidMod.default
  if (activeTheme !== theme) {
    mermaid.initialize(themeConfig(theme))
    activeTheme = theme
  }
  return mermaid
}

export async function renderMermaidSvg(
  source: string,
  theme: MermaidThemeId = 'dark',
): Promise<{ svg: string; error?: string }> {
  const code = normalizeMermaidSource(source)
  if (!code) return { svg: '', error: 'Empty diagram' }
  if (isLikelyIncompleteMermaid(code)) {
    return { svg: '', error: 'incomplete' }
  }
  try {
    const mermaid = await getMermaid(theme)
    try {
      await mermaid.parse(code)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      return { svg: '', error: msg || 'parse failed' }
    }
    seq += 1
    const id = `inf-mermaid-${seq}-${Date.now().toString(36)}`
    const { svg } = await mermaid.render(id, code)
    return { svg }
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    return { svg: '', error: msg || 'render failed' }
  }
}
