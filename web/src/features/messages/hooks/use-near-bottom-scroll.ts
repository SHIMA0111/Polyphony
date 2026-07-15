"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import type { Message } from "@/features/messages/types"

/** Default distance, in pixels, from the bottom edge still considered "near". */
const DEFAULT_THRESHOLD_PX = 120

/**
 * Pure threshold check extracted out of the hook so it is unit-testable
 * without a DOM: is a scroll position within `thresholdPx` of the bottom of
 * its container?
 *
 * @param scrollTop - Current scroll offset from the top.
 * @param scrollHeight - Total scrollable content height.
 * @param clientHeight - Visible viewport height of the container.
 * @param thresholdPx - Maximum distance from the bottom still considered "near".
 * @returns `true` if the remaining scroll distance is within `thresholdPx`.
 */
export function computeIsNearBottom(
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
  thresholdPx: number = DEFAULT_THRESHOLD_PX,
): boolean {
  return scrollHeight - scrollTop - clientHeight <= thresholdPx
}

export interface UseNearBottomScrollResult {
  /** Attach to the scrollable container element. */
  containerRef: React.RefObject<HTMLDivElement | null>
  /** Whether the container is currently scrolled near its bottom edge. */
  isNearBottom: boolean
  /** Whether new messages arrived while the user was scrolled away from the bottom. */
  hasNewMessages: boolean
  /** Smooth-scrolls the container to the bottom and clears `hasNewMessages`. */
  scrollToBottom: () => void
}

/**
 * Tracks whether a scrollable message transcript is near its bottom edge and
 * decides, on every `messages` update, whether to auto-scroll (the user was
 * already at the bottom) or instead surface a "new messages" affordance (the
 * user has scrolled up to read history and should not be yanked back down).
 *
 * @param messages - The current message list; a change in this reference
 *   (length or content) is treated as "new content arrived".
 */
export function useNearBottomScroll(
  messages: Message[],
): UseNearBottomScrollResult {
  const containerRef = useRef<HTMLDivElement>(null)
  const [isNearBottom, setIsNearBottom] = useState(true)
  const [hasNewMessages, setHasNewMessages] = useState(false)
  const isNearBottomRef = useRef(true)
  const isFirstRenderRef = useRef(true)

  const updateIsNearBottom = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    const near = computeIsNearBottom(
      el.scrollTop,
      el.scrollHeight,
      el.clientHeight,
    )
    isNearBottomRef.current = near
    setIsNearBottom(near)
    if (near) setHasNewMessages(false)
  }, [])

  const scrollToBottom = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    el.scrollTo({ top: el.scrollHeight, behavior: "smooth" })
    isNearBottomRef.current = true
    setIsNearBottom(true)
    setHasNewMessages(false)
  }, [])

  // Keep `isNearBottom` in sync with manual scrolling.
  useEffect(() => {
    const el = containerRef.current
    if (!el) return undefined

    el.addEventListener("scroll", updateIsNearBottom, { passive: true })
    updateIsNearBottom()

    return () => el.removeEventListener("scroll", updateIsNearBottom)
  }, [updateIsNearBottom])

  // On new content: jump to bottom without animation on first mount, smooth-
  // scroll if the user was already near the bottom, otherwise flag new
  // messages instead of moving the viewport out from under the user.
  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    if (isFirstRenderRef.current) {
      isFirstRenderRef.current = false
      el.scrollTop = el.scrollHeight
      return
    }

    if (isNearBottomRef.current) {
      el.scrollTo({ top: el.scrollHeight, behavior: "smooth" })
    } else {
      setHasNewMessages(true)
    }
    // Intentionally re-run only when the message list itself changes; the
    // near-bottom check reads from a ref so it doesn't need to be a dep.
  }, [messages])

  return { containerRef, isNearBottom, hasNewMessages, scrollToBottom }
}
