import { useEffect, useState } from 'react'
import { getMCPStatus, type MCPStatusView } from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'

export function useMCPStatus(): MCPStatusView | null {
  const [status, setStatus] = useState<MCPStatusView | null>(null)

  useEffect(() => {
    let cancelled = false
    const refresh = () => {
      getMCPStatus()
        .then((s) => {
          if (!cancelled) setStatus(s)
        })
        .catch(() => {})
    }
    refresh()
    window.addEventListener('inferenesia-mcp-changed', refresh)
    return () => {
      cancelled = true
      window.removeEventListener('inferenesia-mcp-changed', refresh)
    }
  }, [])

  return status
}

export function MCPBadge({ status }: { status: MCPStatusView | null }) {
  const { t } = useLocale()
  if (!status || status.total_servers === 0) return null

  const healthy = status.healthy_count
  const total = status.enabled_count
  const allHealthy = total > 0 && healthy === total
  const colorClass = total === 0
    ? 'border-shell-border text-shell-muted'
    : allHealthy
      ? 'border-shell-border text-shell-text'
      : 'border-shell-accent text-shell-text'

  return (
    <span
      data-testid="chat-mcp-badge"
      className={`hidden rounded-lg border bg-shell-panel px-2 py-1 text-[10px] font-medium transition sm:inline ${colorClass}`}
      title={t('settings.mcpBadgeSummary', { healthy, total, tools: status.total_tools })}
    >
      {total === 0 ? t('settings.mcpBadgeOff') : `MCP ${healthy}/${total}`}
    </span>
  )
}
