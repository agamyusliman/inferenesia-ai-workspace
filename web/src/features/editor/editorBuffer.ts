/**
 * Pure helpers for editor dirty/save state (VAL-IDE-016/017).
 * Buffers keep lastSavedContent so dirty is (content !== lastSavedContent).
 */

export type EditorBufferState = {
  content: string
  lastSavedContent: string
}

/** True when the buffer differs from the last saved (or loaded) snapshot. */
export function isDirty(buf: EditorBufferState): boolean {
  return buf.content !== buf.lastSavedContent
}

/** Apply a content edit; preserves lastSavedContent. */
export function withContent(
  buf: EditorBufferState,
  content: string,
): EditorBufferState {
  return { ...buf, content }
}

/** Mark current content as saved (clears dirty). */
export function markSaved(buf: EditorBufferState): EditorBufferState {
  return { ...buf, lastSavedContent: buf.content }
}

/** Initial state when opening a file from disk. */
export function fromDisk(content: string): EditorBufferState {
  return { content, lastSavedContent: content }
}
