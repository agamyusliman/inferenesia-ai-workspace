/**
 * Pure helpers for rendering unified diff lines in the Git panel.
 * Kept free of React for easy unit tests (VAL-GIT-002).
 */

export type DiffLineKind = 'add' | 'del' | 'hunk' | 'meta' | 'ctx'

export type DiffLine = {
  kind: DiffLineKind
  text: string
}

/** Classify a single unified-diff line. */
export function classifyDiffLine(line: string): DiffLineKind {
  if (line.startsWith('+++') || line.startsWith('---') || line.startsWith('diff ') || line.startsWith('index ') || line.startsWith('new file') || line.startsWith('deleted file') || line.startsWith('old mode') || line.startsWith('new mode') || line.startsWith('similarity ')) {
    return 'meta'
  }
  if (line.startsWith('@@')) return 'hunk'
  if (line.startsWith('+')) return 'add'
  if (line.startsWith('-')) return 'del'
  return 'ctx'
}

/** Split unified diff text into typed lines (preserves empty trailing). */
export function parseDiffLines(content: string): DiffLine[] {
  if (!content) return []
  const raw = content.replace(/\r\n/g, '\n').split('\n')
  // Drop a single trailing empty segment from split when content ends with \n
  if (raw.length > 0 && raw[raw.length - 1] === '') {
    raw.pop()
  }
  return raw.map((text) => ({ kind: classifyDiffLine(text), text }))
}

/** Count + / - lines for a compact header badge. */
export function countDiffStats(lines: DiffLine[]): { additions: number; deletions: number } {
  let additions = 0
  let deletions = 0
  for (const l of lines) {
    if (l.kind === 'add') additions++
    if (l.kind === 'del') deletions++
  }
  return { additions, deletions }
}

/** Partition panel files into staged / unstaged lists (a file may appear in both). */
export function partitionGitFiles<T extends { staged: boolean; unstaged: boolean; untracked: boolean }>(
  files: T[],
): { staged: T[]; unstaged: T[] } {
  const staged: T[] = []
  const unstaged: T[] = []
  for (const f of files) {
    if (f.staged) staged.push(f)
    if (f.unstaged || f.untracked) unstaged.push(f)
  }
  return { staged, unstaged }
}
