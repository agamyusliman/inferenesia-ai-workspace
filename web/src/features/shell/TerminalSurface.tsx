/**
 * One interactive xterm host bound to a single PTY session id.
 * Renders session.output (ANSI) and sends keystrokes via onData (VAL-IDE-031/032).
 * Safe under React StrictMode: paints from props, not setState side effects.
 */

import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { terminalResize } from '../../lib/api'
import { buildXtermOptions } from './terminalXterm'

type Props = {
  sessionId: string | null
  /** Full captured PTY output (ANSI allowed). */
  output: string
  /** Keystrokes including Enter/Backspace/Ctrl+C. */
  onData: (data: string) => void
  /** Visual focus ring for active pane in split view. */
  focused?: boolean
  onFocus?: () => void
  testId?: string
  className?: string
}

export function TerminalSurface({
  sessionId,
  output,
  onData,
  focused = false,
  onFocus,
  testId = 'terminal-surface',
  className = '',
}: Props) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const paintedIdRef = useRef<string | null>(null)
  const paintedLenRef = useRef(0)
  const suppressRef = useRef(false)
  const onDataRef = useRef(onData)
  onDataRef.current = onData
  const sessionIdRef = useRef(sessionId)
  sessionIdRef.current = sessionId
  const lastSizeRef = useRef<{ cols: number; rows: number } | null>(null)

  /** Fit xterm host and push cols×rows into the PTY (clears zsh "%" mark). */
  const pushSize = (term: Terminal, fit: FitAddon, id: string | null) => {
    try {
      fit.fit()
    } catch {
      // host may not have size yet
    }
    const cols = term.cols
    const rows = term.rows
    if (!id || cols <= 0 || rows <= 0) return
    const prev = lastSizeRef.current
    if (prev && prev.cols === cols && prev.rows === rows) return
    lastSizeRef.current = { cols, rows }
    void terminalResize(id, cols, rows).catch(() => {
      // ignore — session may have just closed
    })
  }

  // Create xterm once on mount.
  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    if (termRef.current) {
      try {
        termRef.current.dispose()
      } catch {
        // ignore
      }
      termRef.current = null
    }
    host.replaceChildren()

    const term = new Terminal(buildXtermOptions())
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    termRef.current = term
    fitRef.current = fit
    pushSize(term, fit, sessionIdRef.current)

    const disposable = term.onData((data) => {
      if (suppressRef.current) return
      if (!sessionIdRef.current) return
      onDataRef.current(data)
    })

    let roTimer: number | undefined
    const ro =
      typeof ResizeObserver !== 'undefined'
        ? new ResizeObserver(() => {
            // Debounce fit+resize during layout animations / drag.
            if (roTimer !== undefined) window.clearTimeout(roTimer)
            roTimer = window.setTimeout(() => {
              if (!termRef.current || !fitRef.current) return
              pushSize(termRef.current, fitRef.current, sessionIdRef.current)
            }, 40)
          })
        : null
    ro?.observe(host)

    return () => {
      if (roTimer !== undefined) window.clearTimeout(roTimer)
      disposable.dispose()
      ro?.disconnect()
      try {
        term.dispose()
      } catch {
        // ignore
      }
      if (termRef.current === term) {
        termRef.current = null
        fitRef.current = null
      }
      paintedIdRef.current = null
      paintedLenRef.current = 0
      lastSizeRef.current = null
    }
  }, [])

  // Paint snapshot/delta from props (outside any setState updater).
  useEffect(() => {
    const term = termRef.current
    if (!term) return
    if (!sessionId) {
      if (paintedIdRef.current !== null) {
        suppressRef.current = true
        try {
          term.reset()
        } finally {
          requestAnimationFrame(() => {
            suppressRef.current = false
          })
        }
        paintedIdRef.current = null
        paintedLenRef.current = 0
      }
      return
    }

    if (paintedIdRef.current !== sessionId) {
      // New session: force a fresh PTY size push (do not reuse previous tab size).
      lastSizeRef.current = null
      suppressRef.current = true
      try {
        term.reset()
        if (output) term.write(output)
        paintedIdRef.current = sessionId
        paintedLenRef.current = output.length
      } finally {
        requestAnimationFrame(() => {
          suppressRef.current = false
        })
      }
      if (fitRef.current) {
        pushSize(term, fitRef.current, sessionId)
      }
      return
    }

    if (output.length > paintedLenRef.current) {
      const delta = output.slice(paintedLenRef.current)
      term.write(delta)
      paintedLenRef.current = output.length
    } else if (output.length < paintedLenRef.current) {
      // Buffer rewound/replaced (rare) — full paint.
      suppressRef.current = true
      try {
        term.reset()
        if (output) term.write(output)
        paintedLenRef.current = output.length
      } finally {
        requestAnimationFrame(() => {
          suppressRef.current = false
        })
      }
    }
  }, [sessionId, output])

  // Focus when marked focused; re-fit so rows match panel height after toggle.
  useEffect(() => {
    if (!focused) return
    const t = window.setTimeout(() => {
      try {
        const term = termRef.current
        const fit = fitRef.current
        if (term && fit) {
          pushSize(term, fit, sessionIdRef.current)
        }
        termRef.current?.focus()
      } catch {
        // ignore
      }
    }, 30)
    return () => window.clearTimeout(t)
  }, [focused, sessionId])

  return (
    <div
      ref={hostRef}
      data-testid={testId}
      data-primary-input="surface"
      data-terminal-output="true"
      data-focused={focused ? 'true' : 'false'}
      data-session-id={sessionId || ''}
      role="application"
      aria-label="Integrated terminal"
      onMouseDown={() => onFocus?.()}
      onClick={() => {
        onFocus?.()
        try {
          termRef.current?.focus()
        } catch {
          // ignore
        }
      }}
      className={`terminal-xterm-host min-h-0 flex-1 overflow-hidden p-0 ${
        focused ? 'outline outline-1 outline-offset-[-1px] outline-shell-accent/50' : ''
      } ${className}`}
    />
  )
}
