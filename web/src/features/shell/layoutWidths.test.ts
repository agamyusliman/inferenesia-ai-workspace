/**
 * Pure layout clamp + dock tests:
 *   npx --yes tsx --test src/features/shell/layoutWidths.test.ts
 */
import { fitConfigToViewport, LAYOUT_LIMITS } from './useLayoutConfig'
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n))
}

describe('layout config limits', () => {
  it('sidebar min equals default activity rail', () => {
    assert.equal(LAYOUT_LIMITS.sidebarMin, LAYOUT_LIMITS.activityRail)
    assert.equal(LAYOUT_LIMITS.activityRail, 48)
  })

  it('clamps sidebar, explorer and chat independently', () => {
    const sidebar = clamp(120, LAYOUT_LIMITS.sidebarMin, LAYOUT_LIMITS.sidebarMax)
    const explorer = clamp(400, LAYOUT_LIMITS.explorerMin, LAYOUT_LIMITS.explorerMax)
    const chat = clamp(320, LAYOUT_LIMITS.chatMin, LAYOUT_LIMITS.chatMax)
    assert.equal(sidebar, 120)
    assert.equal(explorer, 400)
    assert.equal(chat, 320)
  })

  it('fitConfigToViewport shrinks side panels so they fit the window', () => {
    const wide = {
      sidebarWidth: 180,
      explorerWidth: 500,
      chatWidth: 500,
      chatHeight: 300,
      terminalHeight: 208,
      explorerDock: 'left' as const,
      chatDock: 'right' as const,
    }
    const fitted = fitConfigToViewport(wide, 800, 600)
    const chrome =
      LAYOUT_LIMITS.splitter * 2 + LAYOUT_LIMITS.mainMin
    assert.ok(
      fitted.sidebarWidth + fitted.explorerWidth + fitted.chatWidth <=
        800 - chrome + 2,
      `sidebar+explorer+chat ${fitted.sidebarWidth}+${fitted.explorerWidth}+${fitted.chatWidth} should fit under 800`,
    )
    assert.ok(fitted.sidebarWidth >= LAYOUT_LIMITS.sidebarMin)
    assert.ok(fitted.explorerWidth >= LAYOUT_LIMITS.explorerMin)
    assert.ok(fitted.chatWidth >= LAYOUT_LIMITS.chatMin)
  })

  it('resizing explorer budget leaves sidebar clamp independent of explorer default', () => {
    const base = {
      sidebarWidth: 140,
      explorerWidth: 256,
      chatWidth: 360,
      chatHeight: 220,
      terminalHeight: 208,
      explorerDock: 'left' as const,
      chatDock: 'right' as const,
    }
    // Widen explorer only in draft; fit should keep sidebar near preference on a large viewport.
    const wide = fitConfigToViewport(
      { ...base, explorerWidth: 420 },
      1600,
      900,
    )
    assert.equal(wide.sidebarWidth, 140)
    assert.equal(wide.explorerWidth, 420)
  })

  it('clamps terminal height and leaves main room (VAL-IDE-033)', () => {
    assert.ok(LAYOUT_LIMITS.terminalHeightMin >= 80)
    assert.ok(LAYOUT_LIMITS.mainHeightMin >= 100)
    const short = fitConfigToViewport(
      {
        sidebarWidth: 48,
        explorerWidth: 256,
        chatWidth: 360,
        chatHeight: 220,
        terminalHeight: 900,
        explorerDock: 'left',
        chatDock: 'right',
      },
      1400,
      500,
    )
    assert.ok(
      short.terminalHeight <= LAYOUT_LIMITS.terminalHeightMax,
      'terminal height capped by max',
    )
    assert.ok(
      short.terminalHeight <= 500 - LAYOUT_LIMITS.mainHeightMin - 80,
      'terminal leaves mainHeightMin room on short viewports',
    )
    assert.ok(short.terminalHeight >= LAYOUT_LIMITS.terminalHeightMin)
  })

  it('preserves preferred terminal height on tall viewports (VAL-IDE-034)', () => {
    const preferred = 320
    const fitted = fitConfigToViewport(
      {
        sidebarWidth: 48,
        explorerWidth: 256,
        chatWidth: 360,
        chatHeight: 220,
        terminalHeight: preferred,
        explorerDock: 'left',
        chatDock: 'right',
      },
      1600,
      1000,
    )
    assert.equal(fitted.terminalHeight, preferred)
  })

  it('main min stays positive', () => {
    assert.ok(LAYOUT_LIMITS.mainMin > 0)
  })
})
