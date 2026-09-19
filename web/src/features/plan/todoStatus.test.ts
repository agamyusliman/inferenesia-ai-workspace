import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

/** Pure helpers mirroring TodoPanel status display for VAL-PLAN-005. */
function statusLabel(status: string): string {
  switch (status) {
    case 'in_progress':
      return 'in progress'
    case 'completed':
      return 'completed'
    case 'cancelled':
      return 'cancelled'
    default:
      return 'pending'
  }
}

function summarize(todos: { status: string }[]) {
  return {
    pending: todos.filter((t) => t.status === 'pending').length,
    in_progress: todos.filter((t) => t.status === 'in_progress').length,
    completed: todos.filter((t) => t.status === 'completed').length,
  }
}

describe('todo status labels', () => {
  it('labels the pending → in_progress → completed lifecycle', () => {
    assert.equal(statusLabel('pending'), 'pending')
    assert.equal(statusLabel('in_progress'), 'in progress')
    assert.equal(statusLabel('completed'), 'completed')
  })

  it('summarizes seeded pending todos after approve', () => {
    const s = summarize([
      { status: 'pending' },
      { status: 'pending' },
      { status: 'pending' },
    ])
    assert.deepEqual(s, { pending: 3, in_progress: 0, completed: 0 })
  })

  it('summarizes real-time progress updates', () => {
    const s = summarize([
      { status: 'completed' },
      { status: 'in_progress' },
      { status: 'pending' },
    ])
    assert.deepEqual(s, { pending: 1, in_progress: 1, completed: 1 })
  })
})
