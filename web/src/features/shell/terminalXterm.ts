/**
 * Helpers for the interactive PTY surface (xterm.js).
 * Primary input is the emulated surface, not a chat-style command box (VAL-IDE-031/032).
 */

/** Default xterm theme matching the shell dark panel. */
export const TERMINAL_XTERM_THEME = {
  background: '#0d1117',
  foreground: '#c9d1d9',
  cursor: '#58a6ff',
  cursorAccent: '#0d1117',
  selectionBackground: '#264f78',
  black: '#484f58',
  red: '#ff7b72',
  green: '#3fb950',
  yellow: '#d29922',
  blue: '#58a6ff',
  magenta: '#bc8cff',
  cyan: '#39c5cf',
  white: '#b1bac4',
  brightBlack: '#6e7681',
  brightRed: '#ffa198',
  brightGreen: '#56d364',
  brightYellow: '#e3b341',
  brightBlue: '#79c0ff',
  brightMagenta: '#d2a8ff',
  brightCyan: '#56d4dd',
  brightWhite: '#f0f6fc',
} as const

export type TerminalSurfaceOptions = {
  /** Font size in px. */
  fontSize?: number
  /** Disable cursor blink (tests). */
  cursorBlink?: boolean
}

/**
 * Build xterm constructor options for the integrated terminal surface.
 * Kept pure so unit tests can assert interactive surface defaults.
 */
export function buildXtermOptions(opts: TerminalSurfaceOptions = {}) {
  return {
    cursorBlink: opts.cursorBlink ?? true,
    convertEol: true,
    fontSize: opts.fontSize ?? 12,
    fontFamily:
      'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
    theme: { ...TERMINAL_XTERM_THEME },
    allowProposedApi: false,
    // Scrollback keeps recent PTY history visible in the surface.
    scrollback: 5000,
    // Disable xterm's built-in Ctrl+C clipboard behavior so onData gets the
    // control char and we forward SIGINT to the PTY (VAL-IDE-032).
    disableStdin: false,
  }
}

/**
 * True when the given data is a Ctrl+C / ETX interrupt byte sequence.
 * Used by pure unit tests (runtime path goes through xterm onData → PTY write).
 */
export function isInterruptSequence(data: string): boolean {
  return data === '\u0003' || data === '\x03'
}

/**
 * Whether UI should treat primary input as the emulated surface (not a chat box).
 * The integrated panel always uses surface mode after this feature.
 */
export function usesInteractiveSurfaceInput(): boolean {
  return true
}
