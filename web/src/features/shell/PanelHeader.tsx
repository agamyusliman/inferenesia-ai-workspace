import type { ReactNode } from 'react'
import { panelHeaderRootClass } from './panelHeaderLayout'
import { SHELL } from './shellTokens'

type Props = {
  title: string
  subtitle?: string
  leading?: ReactNode
  actions?: ReactNode
  testId?: string
  dense?: boolean
}

export function PanelHeader({
  title,
  subtitle,
  leading,
  actions,
  testId,
  dense,
}: Props) {
  const showSubtitle = Boolean(subtitle) && !dense

  return (
    <div
      data-testid={testId}
      data-layout="single-row"
      className={panelHeaderRootClass({ dense, hasSubtitle: showSubtitle })}
    >
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {leading && (
          <div
            className="flex shrink-0 items-center"
            data-testid={testId ? `${testId}-leading` : undefined}
          >
            {leading}
          </div>
        )}
        <h2
          className={`min-w-0 shrink truncate ${SHELL.panelHeaderTitleClass} ${
            showSubtitle ? 'max-w-[55%]' : 'flex-1'
          }`}
        >
          {title}
        </h2>
        {showSubtitle && (
          <span
            data-testid={testId ? `${testId}-subtitle` : undefined}
            className={`min-w-0 flex-1 ${SHELL.panelSublineClass}`}
            title={subtitle}
          >
            {subtitle}
          </span>
        )}
      </div>
      {actions && (
        <div
          className="flex shrink-0 items-center gap-1"
          data-testid={testId ? `${testId}-actions` : undefined}
        >
          {actions}
        </div>
      )}
    </div>
  )
}
