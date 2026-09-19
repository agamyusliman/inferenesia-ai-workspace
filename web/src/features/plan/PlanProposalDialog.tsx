import { useState } from 'react'
import {
  approvePlan,
  rejectPlan,
  type PlanProposalView,
  type TodoListView,
} from '../../lib/api'

type Props = {
  proposal: PlanProposalView
  onApproved: (todos: TodoListView) => void
  onRejected: () => void
  onError?: (msg: string) => void
}

/**
 * Approve / Reject dialog for PlanProposed (VAL-PLAN-003/004).
 * Approve seeds todos with pending status; Reject leaves the todo panel unchanged.
 */
export function PlanProposalDialog({
  proposal,
  onApproved,
  onRejected,
  onError,
}: Props) {
  const [busy, setBusy] = useState(false)

  const decide = async (approve: boolean) => {
    if (busy) return
    setBusy(true)
    try {
      if (approve) {
        const res = await approvePlan()
        onApproved(res.todos)
      } else {
        await rejectPlan()
        onRejected()
      }
    } catch (e) {
      onError?.(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const steps = proposal.steps || []

  return (
    <div
      data-testid="plan-proposal-dialog"
      data-plan-id={proposal.id}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="plan-proposal-title"
    >
      <div className="flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-lg border border-shell-border bg-shell-panel shadow-xl">
        <header className="shrink-0 border-b border-shell-border px-4 py-3">
          <p className="text-[10px] font-semibold uppercase tracking-wide text-shell-accent">
            Plan proposed
          </p>
          <h2
            id="plan-proposal-title"
            data-testid="plan-proposal-title"
            className="mt-0.5 text-[14px] font-semibold text-shell-text"
          >
            {proposal.title || 'Implementation plan'}
          </h2>
          {proposal.summary && (
            <p
              data-testid="plan-proposal-summary"
              className="mt-1 text-[12px] text-shell-muted"
            >
              {proposal.summary}
            </p>
          )}
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          {proposal.body ? (
            <pre
              data-testid="plan-proposal-body"
              className="mb-3 whitespace-pre-wrap font-sans text-[12px] leading-relaxed text-shell-text/90"
            >
              {proposal.body}
            </pre>
          ) : null}
          <p className="mb-1 text-[11px] font-medium text-shell-muted">
            Steps (become todos on approve)
          </p>
          <ol
            data-testid="plan-proposal-steps"
            className="list-decimal space-y-1 pl-5 text-[12px] text-shell-text"
          >
            {steps.map((s, i) => (
              <li key={`${i}-${s}`} data-testid="plan-proposal-step">
                {s}
              </li>
            ))}
          </ol>
        </div>

        <footer className="flex shrink-0 items-center justify-end gap-2 border-t border-shell-border px-4 py-3">
          <button
            type="button"
            data-testid="plan-reject"
            disabled={busy}
            onClick={() => void decide(false)}
            className="rounded-md border border-shell-border px-3 py-1.5 text-[12px] text-shell-text hover:bg-shell-hover disabled:opacity-50"
          >
            {busy ? '…' : 'Reject'}
          </button>
          <button
            type="button"
            data-testid="plan-approve"
            disabled={busy}
            onClick={() => void decide(true)}
            className="rounded-md bg-shell-accent px-3 py-1.5 text-[12px] font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {busy ? '…' : 'Approve'}
          </button>
        </footer>
      </div>
    </div>
  )
}
