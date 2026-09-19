import { useCallback, useEffect, useState } from 'react'
import { SHELL } from './shellTokens'

const STORAGE_KEY = 'inferenesia-layout-config'
const STORAGE_KEY_TYPO = 'infernesia-layout-config'
const STORAGE_KEY_LEGACY = 'yura-ai-layout-config'

/** Where the explorer/workspace list sits relative to the editor. */
export type ExplorerDock = 'left' | 'right'

/** Where the chat panel docks relative to the editor. */
export type ChatDock = 'right' | 'left' | 'bottom'

export type LayoutConfig = {
  /** Left activity/workspace-switcher sidebar (resizable; VAL-IDE-001). */
  sidebarWidth: number
  explorerWidth: number
  chatWidth: number
  /** When chat is bottom, height in px. */
  chatHeight: number
  /**
   * Integrated terminal panel height in px when open (VAL-IDE-033/034).
   * Persisted in the same session layout config.
   */
  terminalHeight: number
  timelineHeight: number
  explorerDock: ExplorerDock
  chatDock: ChatDock
}

export const LAYOUT_LIMITS = {
  /**
   * Default compact icon rail size (also sidebar minimum).
   * Kept for chrome budget math and Sidebar defaults.
   */
  activityRail: SHELL.activityRail,
  /** Icon-only floor for the activity sidebar. */
  sidebarMin: SHELL.activityRail,
  /** Expanded sidebar with nav labels. */
  sidebarMax: 220,
  explorerMin: 140,
  explorerMax: 560,
  chatMin: 200,
  chatMax: 720,
  chatHeightMin: 100,
  chatHeightMax: 520,
  /** Terminal panel height floor/ceil (VAL-IDE-033). */
  terminalHeightMin: 100,
  terminalHeightMax: 560,
  timelineHeightMin: 100,
  timelineHeightMax: 480,
  /** Minimum width kept for the flexible center (editor or empty main). */
  mainMin: 200,
  /**
   * Minimum remaining height for editor/main above the terminal so the main
   * area is never fully crushed without recovery (VAL-IDE-033).
   */
  mainHeightMin: 120,
  /** Approximate vertical splitter thickness (px). */
  splitter: 4,
} as const

const DEFAULTS: LayoutConfig = {
  sidebarWidth: SHELL.activityRail,
  explorerWidth: 256,
  chatWidth: 360,
  chatHeight: 220,
  /** ~h-52 (13rem) default used by the previous fixed terminal panel. */
  terminalHeight: 208,
  timelineHeight: 180,
  explorerDock: 'left',
  chatDock: 'right',
}

function clamp(n: number, min: number, max: number): number {
  if (max < min) return min
  return Math.round(Math.min(max, Math.max(min, n)))
}

function isExplorerDock(v: unknown): v is ExplorerDock {
  return v === 'left' || v === 'right'
}

function isChatDock(v: unknown): v is ChatDock {
  return v === 'right' || v === 'left' || v === 'bottom'
}

/**
 * Fit sidebar + explorer + optional side chat into the current window width so
 * the app never overflows horizontally (right edge cut off).
 *
 * User-preferred widths are kept when the window is large enough; when the
 * window shrinks, panels scale down proportionally while respecting mins.
 * Sidebar and explorer stay independently clampable (VAL-IDE-001/002).
 */
export function fitConfigToViewport(cfg: LayoutConfig, vw: number, vh: number): LayoutConfig {
  const split = LAYOUT_LIMITS.splitter
  const mainMin = LAYOUT_LIMITS.mainMin

  const chatSide = cfg.chatDock === 'left' || cfg.chatDock === 'right'
  // Vertical chrome: sidebar splitter always present; explorer splitter when explorer shown (assumed);
  // chat splitter when side-docked.
  const chrome = split * 2 + (chatSide ? split : 0)
  const budget = Math.max(0, vw - chrome - mainMin)

  let sidebarWidth = clamp(
    cfg.sidebarWidth,
    LAYOUT_LIMITS.sidebarMin,
    LAYOUT_LIMITS.sidebarMax,
  )
  let explorerWidth = cfg.explorerWidth
  let chatWidth = cfg.chatWidth

  if (chatSide) {
    // budget is shared by sidebar + explorer + chat
    let sum = sidebarWidth + explorerWidth + chatWidth
    if (sum > budget && sum > 0) {
      // Prefer shrinking chat, then explorer, then sidebar (keep icons usable)
      let overflow = sum - budget
      const shrinkChat = Math.min(
        overflow,
        Math.max(0, chatWidth - LAYOUT_LIMITS.chatMin),
      )
      chatWidth -= shrinkChat
      overflow -= shrinkChat
      const shrinkExp = Math.min(
        overflow,
        Math.max(0, explorerWidth - LAYOUT_LIMITS.explorerMin),
      )
      explorerWidth -= shrinkExp
      overflow -= shrinkExp
      if (overflow > 0) {
        sidebarWidth = Math.max(LAYOUT_LIMITS.sidebarMin, sidebarWidth - overflow)
      }
    }
    const expCap = Math.min(
      LAYOUT_LIMITS.explorerMax,
      Math.max(LAYOUT_LIMITS.explorerMin, Math.floor(budget * 0.45)),
    )
    const chatCap = Math.min(
      LAYOUT_LIMITS.chatMax,
      Math.max(LAYOUT_LIMITS.chatMin, Math.floor(budget * 0.45)),
    )
    explorerWidth = clamp(explorerWidth, LAYOUT_LIMITS.explorerMin, expCap)
    chatWidth = clamp(chatWidth, LAYOUT_LIMITS.chatMin, chatCap)
    sidebarWidth = clamp(sidebarWidth, LAYOUT_LIMITS.sidebarMin, LAYOUT_LIMITS.sidebarMax)

    // Final re-fit if mins pushed us over budget
    let again = sidebarWidth + explorerWidth + chatWidth
    if (again > budget && again > 0) {
      let overflow = again - budget
      const shrinkExp = Math.min(
        overflow,
        Math.max(0, explorerWidth - LAYOUT_LIMITS.explorerMin),
      )
      explorerWidth -= shrinkExp
      overflow -= shrinkExp
      if (overflow > 0) {
        chatWidth = Math.max(LAYOUT_LIMITS.chatMin, chatWidth - overflow)
      }
    }
  } else {
    // Chat is bottom: sidebar + explorer compete with main
    let sum = sidebarWidth + explorerWidth
    if (sum > budget && sum > 0) {
      let overflow = sum - budget
      const shrinkExp = Math.min(
        overflow,
        Math.max(0, explorerWidth - LAYOUT_LIMITS.explorerMin),
      )
      explorerWidth -= shrinkExp
      overflow -= shrinkExp
      if (overflow > 0) {
        sidebarWidth = Math.max(LAYOUT_LIMITS.sidebarMin, sidebarWidth - overflow)
      }
    }
    const expCap = Math.min(
      LAYOUT_LIMITS.explorerMax,
      Math.max(LAYOUT_LIMITS.explorerMin, budget - LAYOUT_LIMITS.sidebarMin),
    )
    explorerWidth = clamp(explorerWidth, LAYOUT_LIMITS.explorerMin, expCap)
    chatWidth = clamp(chatWidth, LAYOUT_LIMITS.chatMin, LAYOUT_LIMITS.chatMax)
    sidebarWidth = clamp(sidebarWidth, LAYOUT_LIMITS.sidebarMin, LAYOUT_LIMITS.sidebarMax)
  }

  const maxChatH = Math.min(
    LAYOUT_LIMITS.chatHeightMax,
    Math.max(LAYOUT_LIMITS.chatHeightMin, Math.floor(vh * 0.55)),
  )
  const chatHeight = clamp(cfg.chatHeight, LAYOUT_LIMITS.chatHeightMin, maxChatH)

  // Keep terminal height usable without crushing the editor/main (VAL-IDE-033).
  const maxTermH = Math.min(
    LAYOUT_LIMITS.terminalHeightMax,
    Math.max(
      LAYOUT_LIMITS.terminalHeightMin,
      Math.floor(vh - LAYOUT_LIMITS.mainHeightMin - 80),
    ),
  )
  const terminalHeight = clamp(
    cfg.terminalHeight,
    LAYOUT_LIMITS.terminalHeightMin,
    maxTermH,
  )

  const maxTimelineH = Math.min(
    LAYOUT_LIMITS.timelineHeightMax,
    Math.max(LAYOUT_LIMITS.timelineHeightMin, Math.floor(vh * 0.4)),
  )
  const timelineHeight = clamp(
    cfg.timelineHeight,
    LAYOUT_LIMITS.timelineHeightMin,
    maxTimelineH,
  )

  return {
    ...cfg,
    sidebarWidth,
    explorerWidth,
    chatWidth,
    chatHeight,
    terminalHeight,
    timelineHeight,
  }
}

function readStored(): LayoutConfig {
  if (typeof sessionStorage === 'undefined') return { ...DEFAULTS }
  try {
    const raw =
      sessionStorage.getItem(STORAGE_KEY) ??
      sessionStorage.getItem(STORAGE_KEY_TYPO) ??
      sessionStorage.getItem(STORAGE_KEY_LEGACY)
    const legacyWidths = sessionStorage.getItem('yura-ai-layout-widths')
    if (!raw && legacyWidths) {
      const old = JSON.parse(legacyWidths) as Partial<LayoutConfig>
      return fitConfigToViewport(
        {
          ...DEFAULTS,
          explorerWidth: Number(old.explorerWidth) || DEFAULTS.explorerWidth,
          chatWidth: Number(old.chatWidth) || DEFAULTS.chatWidth,
        },
        typeof window !== 'undefined' ? window.innerWidth : 1400,
        typeof window !== 'undefined' ? window.innerHeight : 900,
      )
    }
    if (!raw) {
      return fitConfigToViewport(
        { ...DEFAULTS },
        typeof window !== 'undefined' ? window.innerWidth : 1400,
        typeof window !== 'undefined' ? window.innerHeight : 900,
      )
    }
    const parsed = JSON.parse(raw) as Partial<LayoutConfig>
    const base: LayoutConfig = {
      sidebarWidth: Number(parsed.sidebarWidth) || DEFAULTS.sidebarWidth,
      explorerWidth: Number(parsed.explorerWidth) || DEFAULTS.explorerWidth,
      chatWidth: Number(parsed.chatWidth) || DEFAULTS.chatWidth,
      chatHeight: Number(parsed.chatHeight) || DEFAULTS.chatHeight,
      terminalHeight:
        Number(parsed.terminalHeight) || DEFAULTS.terminalHeight,
      timelineHeight:
        Number(parsed.timelineHeight) || DEFAULTS.timelineHeight,
      explorerDock: isExplorerDock(parsed.explorerDock)
        ? parsed.explorerDock
        : DEFAULTS.explorerDock,
      chatDock: isChatDock(parsed.chatDock) ? parsed.chatDock : DEFAULTS.chatDock,
    }
    return fitConfigToViewport(
      base,
      typeof window !== 'undefined' ? window.innerWidth : 1400,
      typeof window !== 'undefined' ? window.innerHeight : 900,
    )
  } catch {
    return { ...DEFAULTS }
  }
}

function writeStored(cfg: LayoutConfig) {
  if (typeof sessionStorage === 'undefined') return
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(cfg))
  } catch {
    // ignore
  }
}

/**
 * Layout sizes + dock positions with session persistence.
 * Panel pixel widths auto-fit when the browser window is resized so the UI
 * never overflows past the window edge.
 */
export function useLayoutConfig() {
  const [cfg, setCfg] = useState<LayoutConfig>(() => readStored())

  useEffect(() => {
    writeStored(cfg)
  }, [cfg])

  useEffect(() => {
    const fit = () => {
      if (typeof window === 'undefined') return
      const vw = window.innerWidth
      const vh = window.innerHeight
      setCfg((prev) => {
        const next = fitConfigToViewport(prev, vw, vh)
        if (
          next.sidebarWidth === prev.sidebarWidth &&
          next.explorerWidth === prev.explorerWidth &&
          next.chatWidth === prev.chatWidth &&
          next.chatHeight === prev.chatHeight &&
          next.terminalHeight === prev.terminalHeight &&
          next.timelineHeight === prev.timelineHeight
        ) {
          return prev
        }
        return next
      })
    }
    fit()
    window.addEventListener('resize', fit)
    return () => window.removeEventListener('resize', fit)
  }, [])

  const setSidebarWidth = useCallback((w: number) => {
    setCfg((prev) => {
      const draft = {
        ...prev,
        sidebarWidth: clamp(w, LAYOUT_LIMITS.sidebarMin, LAYOUT_LIMITS.sidebarMax),
      }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  const setExplorerWidth = useCallback((w: number) => {
    setCfg((prev) => {
      const draft = {
        ...prev,
        explorerWidth: clamp(w, LAYOUT_LIMITS.explorerMin, LAYOUT_LIMITS.explorerMax),
      }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  const setChatWidth = useCallback((w: number) => {
    setCfg((prev) => {
      const draft = {
        ...prev,
        chatWidth: clamp(w, LAYOUT_LIMITS.chatMin, LAYOUT_LIMITS.chatMax),
      }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  const setChatHeight = useCallback((h: number) => {
    setCfg((prev) => {
      const draft = {
        ...prev,
        chatHeight: clamp(h, LAYOUT_LIMITS.chatHeightMin, LAYOUT_LIMITS.chatHeightMax),
      }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  /** Integrated terminal panel height (top-edge drag; VAL-IDE-033/034). */
  const setTerminalHeight = useCallback((h: number) => {
    setCfg((prev) => {
      const draft = {
        ...prev,
        terminalHeight: clamp(
          h,
          LAYOUT_LIMITS.terminalHeightMin,
          LAYOUT_LIMITS.terminalHeightMax,
        ),
      }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  const setTimelineHeight = useCallback((h: number) => {
    setCfg((prev) => {
      const draft = {
        ...prev,
        timelineHeight: clamp(
          h,
          LAYOUT_LIMITS.timelineHeightMin,
          LAYOUT_LIMITS.timelineHeightMax,
        ),
      }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  const setExplorerDock = useCallback((dock: ExplorerDock) => {
    setCfg((prev) => {
      const draft = { ...prev, explorerDock: dock }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  const setChatDock = useCallback((dock: ChatDock) => {
    setCfg((prev) => {
      const draft = { ...prev, chatDock: dock }
      if (typeof window === 'undefined') return draft
      return fitConfigToViewport(draft, window.innerWidth, window.innerHeight)
    })
  }, [])

  const resetLayout = useCallback(() => {
    if (typeof window === 'undefined') {
      setCfg({ ...DEFAULTS })
      return
    }
    setCfg(fitConfigToViewport({ ...DEFAULTS }, window.innerWidth, window.innerHeight))
  }, [])

  return {
    ...cfg,
    setSidebarWidth,
    setExplorerWidth,
    setChatWidth,
    setChatHeight,
    setTerminalHeight,
    setTimelineHeight,
    setExplorerDock,
    setChatDock,
    resetLayout,
  }
}
