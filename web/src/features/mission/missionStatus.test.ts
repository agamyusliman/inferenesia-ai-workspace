import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

/** Pure helpers mirroring MissionPanel status display (VAL-MISSION-002 message). */

function isEvidenceRequiredMessage(msg: string): boolean {
  return msg.toLowerCase().includes('evidence required')
}

function isCompleteAllowed(hasEvidence: boolean, allGatesPassed: boolean): boolean {
  return hasEvidence && allGatesPassed
}

describe('mission complete gates (UI logic)', () => {
  it('detects evidence required denial message', () => {
    assert.equal(
      isEvidenceRequiredMessage(
        'mission: evidence required — attach verification evidence before complete',
      ),
      true,
    )
    assert.equal(isEvidenceRequiredMessage('mission completed'), false)
  })

  it('complete only when evidence and gates ok', () => {
    assert.equal(isCompleteAllowed(false, true), false)
    assert.equal(isCompleteAllowed(true, false), false)
    assert.equal(isCompleteAllowed(true, true), true)
  })
})
