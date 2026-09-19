/** Persist git panel left column width (changes list). */
const KEY = 'inferenesia-git-panel-layout'
const KEY_TYPO = 'infernesia-git-panel-layout'
const KEY_LEGACY = 'yura-ai-git-panel-layout'

export type GitPanelLayout = {
  leftWidth: number
}

const DEFAULT: GitPanelLayout = {
  leftWidth: 340,
}

export function loadGitPanelLayout(): GitPanelLayout {
  try {
    const raw =
      sessionStorage.getItem(KEY) ??
      sessionStorage.getItem(KEY_TYPO) ??
      sessionStorage.getItem(KEY_LEGACY)
    if (!raw) return { ...DEFAULT }
    const parsed = JSON.parse(raw) as Partial<GitPanelLayout>
    return {
      leftWidth: clampLeft(parsed.leftWidth ?? DEFAULT.leftWidth),
    }
  } catch {
    return { ...DEFAULT }
  }
}

export function saveGitPanelLayout(next: GitPanelLayout): void {
  try {
    sessionStorage.setItem(
      KEY,
      JSON.stringify({ leftWidth: clampLeft(next.leftWidth) }),
    )
  } catch {
    // ignore quota
  }
}

export function clampLeft(w: number): number {
  if (!Number.isFinite(w)) return DEFAULT.leftWidth
  return Math.max(220, Math.min(720, Math.round(w)))
}

export const GIT_LEFT_MIN = 220
export const GIT_LEFT_MAX = 720
