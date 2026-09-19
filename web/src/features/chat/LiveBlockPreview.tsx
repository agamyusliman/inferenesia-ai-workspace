import { useState } from 'react'
import { buildLiveBlockSrcDoc } from './liveBlocks'

type Props = {
  html: string
  testId?: string
}

export function LiveBlockPreview({ html, testId = 'live-block-preview' }: Props) {
  const [showSource, setShowSource] = useState(false)
  const srcDoc = buildLiveBlockSrcDoc(html)
  return (
    <span
      data-testid={testId}
      data-preview-kind="live-block"
      className="live-block-preview my-1.5 block w-full overflow-hidden rounded-md border border-shell-accent/40 bg-shell-panel/60"
    >
      <span className="flex items-center justify-between gap-2 border-b border-shell-border/60 px-2 py-1 text-[10px] uppercase tracking-wide text-shell-accent">
        <span className="flex items-center gap-1.5">
          <span aria-hidden>▶</span> Live Block
        </span>
        <button
          type="button"
          data-testid="live-block-toggle-source"
          aria-expanded={showSource}
          aria-label={showSource ? 'Hide source' : 'Show source'}
          onClick={() => setShowSource((v) => !v)}
          className="rounded px-1 py-0.5 font-normal normal-case text-shell-muted transition hover:bg-shell-bg hover:text-shell-text"
        >
          {showSource ? 'Hide source' : 'Source'}
        </button>
      </span>
      <iframe
        data-testid="live-block-iframe"
        srcDoc={srcDoc}
        sandbox="allow-scripts"
        // No allow-same-origin: iframe cannot reach parent DOM/cookies/storage.
        // No allow-top-navigation: cannot redirect the app window.
        title="Live Block preview"
        className="block min-h-[120px] max-h-[480px] w-full rounded-b-md bg-white"
        loading="lazy"
      />
      {showSource && (
        <pre
          data-testid="live-block-source"
          className="m-0 max-h-48 overflow-auto rounded-b-md border-t border-shell-border/60 bg-[#0b1016] p-2.5"
        >
          <code className="font-mono text-[11px] leading-snug text-shell-text">{html}</code>
        </pre>
      )}
    </span>
  )
}
