/**
 * Smart scroll lock for chat message list (VAL-CHAT-029).
 * Auto-scroll while generating unless the user has scrolled up.
 */

/** Distance (px) from bottom within which we still treat as "at bottom". */
export const SCROLL_BOTTOM_THRESHOLD_PX = 80

export type ScrollMetrics = {
  scrollTop: number
  scrollHeight: number
  clientHeight: number
}

/**
 * True when the viewport is near the bottom (user is following the stream).
 */
export function isNearBottom(
  metrics: ScrollMetrics,
  thresholdPx = SCROLL_BOTTOM_THRESHOLD_PX,
): boolean {
  const { scrollTop, scrollHeight, clientHeight } = metrics
  // Empty / short content is always "near bottom".
  if (scrollHeight <= clientHeight + 1) return true
  const distanceFromBottom = scrollHeight - scrollTop - clientHeight
  return distanceFromBottom <= thresholdPx
}

/**
 * Update the sticky lock: when the user scrolls up, lock (no auto-scroll).
 * When they scroll back to the bottom, unlock so generation can follow again.
 */
export function updateScrollLock(
  metrics: ScrollMetrics,
  _prevLocked: boolean,
  thresholdPx = SCROLL_BOTTOM_THRESHOLD_PX,
): boolean {
  void _prevLocked // signature keeps call-site stability for sticky updates
  if (isNearBottom(metrics, thresholdPx)) {
    return false // unlocked — stick to latest
  }
  // User is above the bottom → lock.
  return true
}

/**
 * Whether the list should auto-scroll after a messages update.
 * Only when not user-locked.
 */
export function shouldAutoScroll(locked: boolean): boolean {
  return !locked
}

/**
 * Next scrollTop to jump to bottom (used when auto-scroll applies).
 */
export function scrollTopToBottom(metrics: Pick<ScrollMetrics, 'scrollHeight' | 'clientHeight'>): number {
  return Math.max(0, metrics.scrollHeight - metrics.clientHeight)
}
