"use client"

import { useCallback, useEffect, useLayoutEffect, useRef } from "react"

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
 * This is a fallback only (see {@link useLoadOlderOnScroll}'s doc comment):
 * it over-adjusts whenever the container's content also changes at the
 * *bottom* during the fetch (e.g. a live message arriving mid-fetch, or a
 * streaming token appending text below the fold) — that added height is
 * indistinguishable, from a bare scrollHeight delta, from height added by
 * the older page prepended above. Element-based anchoring
 * ({@link computeElementAnchoredScrollTop}) doesn't have this problem and is
 * used whenever the anchor element is still present after the update; this
 * function is only reached when it isn't (e.g. the anchor message was
 * deleted or excluded out of the list while the fetch was in flight).
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

/**
 * Computes the `scrollTop` that keeps `anchorOffset` (the anchor element's
 * top edge position relative to the container's top edge, captured via
 * `getBoundingClientRect()` diffs immediately before the fetch) unchanged,
 * given the anchor element's *current* offset (measured the same way, after
 * the older page's DOM has landed but before any scroll correction is
 * applied — so `currentScrollTop` below is still the pre-fetch value).
 *
 * Unlike {@link computeScrollAnchorAdjustment}, this is anchored to a
 * specific message element rather than the container's total height, so it
 * stays correct even if content changed at the bottom of the list (a new
 * live message, a streaming update) in the same render as the older page's
 * prepend — that content sits below the anchor and never factors into the
 * diff.
 *
 * @param currentScrollTop - The container's `scrollTop` at the moment of
 *   measurement (unchanged since the fetch was triggered; the caller has
 *   not yet applied any correction).
 * @param anchorOffset - The anchor element's `getBoundingClientRect().top -
 *   container.getBoundingClientRect().top`, captured immediately before the
 *   fetch.
 * @param currentAnchorOffset - The same measurement, taken again after the
 *   older page's DOM has landed.
 * @returns The `scrollTop` to re-apply so the anchor element's on-screen
 *   position matches `anchorOffset` again.
 */
export function computeElementAnchoredScrollTop(
  currentScrollTop: number,
  anchorOffset: number,
  currentAnchorOffset: number,
): number {
  return currentScrollTop + (currentAnchorOffset - anchorOffset)
}

/** A message element tracked by `data-message-id` inside the scroll container. */
interface AnchorElement {
  id: string
  el: HTMLElement
}

/**
 * Finds the topmost message element (by DOM order, which matches
 * chronological order) that is still at least partially visible within
 * `container`'s viewport — i.e. the first `[data-message-id]` element whose
 * bottom edge has not yet scrolled above the container's top edge.
 *
 * Returns `null` if the container has no `[data-message-id]` descendants
 * (e.g. an empty room, or a test harness that doesn't render real message
 * bubbles).
 */
function findTopmostVisibleMessageElement(
  container: HTMLElement,
): AnchorElement | null {
  const containerTop = container.getBoundingClientRect().top
  const candidates = container.querySelectorAll<HTMLElement>("[data-message-id]")

  for (const el of candidates) {
    if (el.getBoundingClientRect().bottom > containerTop) {
      const id = el.dataset.messageId
      if (id) return { id, el }
      return null
    }
  }
  return null
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
 * the top and there is more history to load, this records the topmost
 * currently-visible message element (by its `data-message-id`, see
 * `MessageBubble.tsx`) and its on-screen offset from the container's top
 * edge, then calls `fetchNextPage()`. Once the fetched page's messages are
 * actually rendered (detected via a `pageCount` change, in a
 * `useLayoutEffect` so the adjustment happens before paint), `scrollTop` is
 * corrected via `computeElementAnchoredScrollTop` so that same element ends
 * up at the same on-screen offset again.
 *
 * Element-based anchoring (rather than a bare `scrollHeight` delta) matters
 * because the older-page fetch is not the only thing that can change this
 * container's height: a live message can arrive via WebSocket, or a
 * streaming AI response can grow, while the older page is still in flight.
 * A scrollHeight-delta correction can't distinguish height added above the
 * anchor (which must be compensated) from height added below it (which
 * must not be) and over-adjusts in the latter case; anchoring to a specific
 * element sidesteps this entirely, since content below the anchor never
 * enters the calculation. The delta-based `computeScrollAnchorAdjustment`
 * is kept only as a fallback for the (rare) case where the anchor element
 * itself is no longer present after the update — e.g. it was deleted, or
 * excluded from the list, while the fetch was in flight.
 *
 * Returns `triggerLoadOlder`, the same anchor-capture-then-fetch routine the
 * `scroll` listener above uses internally, so a caller can also trigger an
 * older-page load outside of an actual scroll event — see `MessageList.tsx`,
 * which calls it when the first page(s) of history don't fill the
 * container's viewport at all (so there is no scrollable area for the user
 * to reach the top of, and this hook's `scroll` listener would otherwise
 * never fire).
 */
export function useLoadOlderOnScroll({
  containerRef,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  pageCount,
}: UseLoadOlderOnScrollOptions): { triggerLoadOlder: () => void } {
  const anchorRef = useRef<{
    scrollTop: number
    scrollHeight: number
    elementId: string | null
    elementOffset: number | null
  } | null>(null)

  const triggerLoadOlder = useCallback(() => {
    const el = containerRef.current
    if (!el || !hasNextPage || isFetchingNextPage) return

    const topmost = findTopmostVisibleMessageElement(el)
    anchorRef.current = {
      scrollTop: el.scrollTop,
      scrollHeight: el.scrollHeight,
      elementId: topmost?.id ?? null,
      elementOffset: topmost
        ? topmost.el.getBoundingClientRect().top - el.getBoundingClientRect().top
        : null,
    }
    fetchNextPage()
  }, [containerRef, hasNextPage, isFetchingNextPage, fetchNextPage])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return undefined

    const handleScroll = () => {
      if (el.scrollTop > LOAD_THRESHOLD_PX) return
      triggerLoadOlder()
    }

    el.addEventListener("scroll", handleScroll, { passive: true })
    return () => el.removeEventListener("scroll", handleScroll)
  }, [containerRef, triggerLoadOlder])

  useLayoutEffect(() => {
    const el = containerRef.current
    const anchor = anchorRef.current
    if (!el || !anchor) return

    const anchorEl =
      anchor.elementId != null
        ? el.querySelector<HTMLElement>(
            `[data-message-id="${anchor.elementId}"]`,
          )
        : null

    if (anchorEl && anchor.elementOffset != null) {
      const currentAnchorOffset =
        anchorEl.getBoundingClientRect().top - el.getBoundingClientRect().top
      el.scrollTop = computeElementAnchoredScrollTop(
        el.scrollTop,
        anchor.elementOffset,
        currentAnchorOffset,
      )
    } else {
      // Fallback: the anchor message is no longer in the DOM (e.g. deleted
      // or excluded while the fetch was in flight). This is less precise
      // when the bottom of the list also changed during the fetch, but it
      // is strictly better than leaving scrollTop uncorrected.
      el.scrollTop = computeScrollAnchorAdjustment(
        anchor.scrollTop,
        anchor.scrollHeight,
        el.scrollHeight,
      )
    }

    anchorRef.current = null
    // Deliberately keyed on `pageCount` (not `containerRef`/other deps) so
    // this only runs once per newly-landed older page, after its messages
    // have actually been committed to the DOM.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageCount])

  return { triggerLoadOlder }
}
