import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  classifyDiffLine,
  countDiffStats,
  parseDiffLines,
  partitionGitFiles,
} from './gitDiffLines.ts'

describe('gitDiffLines', () => {
  it('classifies add/del/hunk/meta', () => {
    assert.equal(classifyDiffLine('+foo'), 'add')
    assert.equal(classifyDiffLine('-bar'), 'del')
    assert.equal(classifyDiffLine('@@ -1,2 +1,3 @@'), 'hunk')
    assert.equal(classifyDiffLine('diff --git a/x b/x'), 'meta')
    assert.equal(classifyDiffLine(' context'), 'ctx')
  })

  it('parses unified diff and counts stats', () => {
    const content = [
      'diff --git a/a.txt b/a.txt',
      '--- a/a.txt',
      '+++ b/a.txt',
      '@@ -1 +1,2 @@',
      '-old',
      '+new',
      '+extra',
      '',
    ].join('\n')
    const lines = parseDiffLines(content)
    assert.ok(lines.some((l) => l.kind === 'add' && l.text === '+new'))
    assert.ok(lines.some((l) => l.kind === 'del' && l.text === '-old'))
    const stats = countDiffStats(lines)
    assert.equal(stats.additions, 2)
    assert.equal(stats.deletions, 1)
  })

  it('partitions staged vs unstaged', () => {
    const { staged, unstaged } = partitionGitFiles([
      { staged: true, unstaged: false, untracked: false },
      { staged: false, unstaged: true, untracked: false },
      { staged: true, unstaged: true, untracked: false },
      { staged: false, unstaged: false, untracked: true },
    ])
    assert.equal(staged.length, 2)
    assert.equal(unstaged.length, 3)
  })
})
