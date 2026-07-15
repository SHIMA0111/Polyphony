"use client"

import { useEffect, useLayoutEffect, useRef } from "react"

/** Distance, in pixels, from the top edge that triggers loading older history. */
const LOAD_THRESHOLD_PX = 100

/**
 * Pure scroll-anchor math, extracted out of the hook so it is unit-testable
 * without a DOM (mirroring `use-near-bottom-scroll.ts`'s
 * `computeIsNearBottom` pattern): given the scroll container's
 * `scrollTop`/`scrollHeight` captured immediately before an older page of
 * history is prepended above the current content, and the container's new
 * `scrollHeight` after that page's DOM has landed, returns the `scrollTop`
 * that keeps the reader's previously-visible content anchored in place
 * (no visible jump) despite the added height above it.
 *
 * @param oldScrollTop - `scrollTop` captured immediately before the fetch.
 * @param oldScrollHeight - `scrollHeight` captured immediately before the fetch.
 * @param newScrollHeight - `scrollHeight` after the older page's DOM landed.
 * @returns The `scrollTop` to re-apply so the pre-fetch content doesn't move.
 */
export function computeScrollAnchorAdjustment(
  oldScrollTop: number,
  oldScrollHeight: number,
  newScrollHeight: number,
): number {
  return newScrollHeight - oldScrollHeight + oldScrollTop
}

export interface UseLoadOlderOnScrollOptions {
  /** The same scroll container ref `use-near-bottom-scroll.ts` observes. */
  containerRef: React.RefObject<HTMLDivElement | null>
  hasNextPage: boolean
  isFetchingNextPage: boolean
  fetchNextPage: () => void
  /**
   * Number of currently loaded pages. A change signals that a new (older)
   * page's messages have actually landed in the DOM, which is when the
   * scroll-anchor adjustment must run — not when the fetch merely starts.
   */
  pageCount: number
}

/**
 * Upward-infinite-scroll: fetches the next (older) page of message history
 * once the transcript is scrolled near its top, and preserves the reader's
 * scroll anchor across the resulting DOM update so nothing visibly jumps.
 *
 * Mechanism: on `scroll`, if the container is within `LOAD_THRESHOLD_PX` of
 * the top and there is more history to load, this captures the container's
 * current `scrollTop`/`scrollHeight` and calls `fetchNextPage()`. Once the
 * fetched page's messages are actually rendered (detected via a `pageCount`
 * change, in a `useLayoutEffect` so the adjustment happens before paint),
 * `scrollTop` is reset to `computeScrollAnchorAdjustment`'s result, which
 * accounts for the height the newly-rendered older messages added above the
 * previously-visible content.
 */
export function useLoadOlderOnScroll({
  containerRef,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  pageCount,
}: UseLoadOlderOnScrollOptions): void {
  const anchorRef = useRef<{ scrollTop: number; scrollHeight: number } | null>(null)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return undefined

    const handleScroll = () => {
      if (!hasNextPage || isFetchingNextPage) return
      if (el.scrollTop > LOAD_THRESHOLD_PX) return

      anchorRef.current = { scrollTop: el.scrollTop, scrollHeight: el.scrollHeight }
      fetchNextPage()
    }

    el.addEventListener("scroll", handleScroll, { passive: true })
    return () => el.removeEventListener("scroll", handleScroll)
  }, [containerRef, hasNextPage, isFetchingNextPage, fetchNextPage])

  useLayoutEffect(() => {
    const el = containerRef.current
    const anchor = anchorRef.current
    if (!el || !anchor) return

    el.scrollTop = computeScrollAnchorAdjustment(
      anchor.scrollTop,
      anchor.scrollHeight,
      el.scrollHeight,
    )
    anchorRef.current = null
    // Deliberately keyed on `pageCount` (not `containerRef`/other deps) so
    // this only runs once per newly-landed older page, after its messages
    // have actually been committed to the DOM.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageCount])
}
