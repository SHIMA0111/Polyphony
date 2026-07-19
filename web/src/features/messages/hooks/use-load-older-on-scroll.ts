"use client"

import { useCallback, useEffect, useLayoutEffect, useRef } from "react"

/** Distance, in pixels, from the top edge that triggers loading older history. */
const LOAD_THRESHOLD_PX = 100

/** Selector for the per-message DOM node `MessageBubble` renders (`data-message-id`). */
const MESSAGE_ELEMENT_SELECTOR = "[data-message-id]"

/**
 * Pure scroll-anchor math for the *element-based* anchoring strategy: given
 * the container's current `scrollTop` and an anchor element's vertical
 * offset relative to the container (captured immediately before an older
 * page of history is prepended, and again after that page's DOM has
 * landed), returns the `scrollTop` that keeps the anchor element visually in
 * the same place despite the added height above it.
 *
 * This is preferred over the scrollHeight-delta strategy
 * ({@link computeScrollAnchorAdjustment}) because it stays correct even when
 * content is *also* appended below the visible area during the same fetch
 * (e.g. a new message arriving concurrently with an older-page load): the
 * delta strategy attributes 100% of any `scrollHeight` growth to the
 * prepended content, over-adjusting when some of that growth happened at
 * the bottom instead. Anchoring to a specific element's own position is
 * unaffected by height changes elsewhere in the container.
 *
 * @param currentScrollTop - The container's `scrollTop` at the moment the
 *   new page's DOM has landed (immediately before this adjustment).
 * @param oldOffsetInContainer - The anchor element's `top` position relative
 *   to the container's `top`, captured immediately before the fetch.
 * @param newOffsetInContainer - The anchor element's `top` position relative
 *   to the container's `top`, captured immediately after the new page
 *   landed.
 * @returns The `scrollTop` to apply so the anchor element's on-screen
 *   position is restored.
 */
export function computeElementAnchorScrollTop(
  currentScrollTop: number,
  oldOffsetInContainer: number,
  newOffsetInContainer: number,
): number {
  return currentScrollTop + (newOffsetInContainer - oldOffsetInContainer)
}

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
 * This is a fallback only, used when the element-based anchor
 * ({@link computeElementAnchorScrollTop}) can't be used because the anchor
 * element captured before the fetch is no longer in the DOM. Unlike the
 * element-based strategy, this attributes *all* `scrollHeight` growth during
 * the fetch to the prepended older content, so it over-adjusts if content
 * was also added below the fold in the same window (e.g. a concurrent new
 * message).
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
 * Finds the topmost message element (`[data-message-id]`, rendered by
 * `MessageBubble`) that is at least partially visible within `container`'s
 * scrolled viewport, i.e. the first one (in DOM order) whose bottom edge is
 * below the container's top edge.
 *
 * @param container - The scrollable message-list container.
 * @returns The topmost visible message element, or `null` if the container
 *   has no `[data-message-id]` descendants yet (e.g. an empty room).
 */
function findTopmostVisibleMessageElement(container: HTMLElement): HTMLElement | null {
  const elements = container.querySelectorAll<HTMLElement>(MESSAGE_ELEMENT_SELECTOR)
  const containerTop = container.getBoundingClientRect().top

  for (const element of elements) {
    if (element.getBoundingClientRect().bottom > containerTop) {
      return element
    }
  }
  return null
}

/** Snapshot captured immediately before an older-page fetch is triggered. */
interface ScrollAnchor {
  /** Topmost visible message element at capture time, or `null` if none was found. */
  element: HTMLElement | null
  /** `element`'s `top` relative to the container's `top`, at capture time. */
  offsetInContainer: number
  /** Fallback-strategy inputs, used only when `element` is no longer in the DOM. */
  fallbackScrollTop: number
  fallbackScrollHeight: number
}

export interface UseLoadOlderOnScrollOptions {
  /** The same scroll container ref `use-near-bottom-scroll.ts` observes. */
  containerRef: React.RefObject<HTMLDivElement | null>
  hasNextPage: boolean
  isFetchingNextPage: boolean
  /**
   * Fetches the next older page of message history. May return a Promise
   * (as react-query's `fetchNextPage` does) — if it does, this hook awaits
   * it to clear its internal in-flight guard (see
   * `captureAnchorAndFetch`'s docstring) as soon as the fetch settles,
   * rather than relying solely on the `pageCount` effect, so a fetch that
   * fails (rejects) without ever landing a new page doesn't leave the guard
   * stuck and silently block every subsequent scroll-triggered load.
   */
  fetchNextPage: () => unknown
  /**
   * Number of currently loaded pages. A change signals that a new (older)
   * page's messages have actually landed in the DOM, which is when the
   * scroll-anchor adjustment must run — not when the fetch merely starts.
   */
  pageCount: number
}

export interface UseLoadOlderOnScrollResult {
  /**
   * Captures the current scroll anchor (topmost visible message element, or
   * the scrollTop/scrollHeight fallback) and calls `fetchNextPage()`,
   * regardless of the current `scrollTop`.
   *
   * Exposed so callers can trigger the exact same anchor-capture-then-fetch
   * sequence the `scroll` listener below uses from outside a scroll event —
   * e.g. `MessageList`'s "first page doesn't fill the container" effect,
   * where there is no scrollable overflow for the user to ever generate a
   * `scroll` event from in the first place.
   */
  triggerLoadOlder: () => void
}

/**
 * Upward-infinite-scroll: fetches the next (older) page of message history
 * once the transcript is scrolled near its top, and preserves the reader's
 * scroll anchor across the resulting DOM update so nothing visibly jumps.
 *
 * Mechanism: on `scroll`, if the container is within `LOAD_THRESHOLD_PX` of
 * the top and there is more history to load, this captures the topmost
 * currently-visible message element (`[data-message-id]`) and its offset
 * within the container, then calls `fetchNextPage()`. Once the fetched
 * page's messages are actually rendered (detected via a `pageCount` change,
 * in a `useLayoutEffect` so the adjustment happens before paint), `scrollTop`
 * is reset via {@link computeElementAnchorScrollTop} so that same element
 * ends up back at its original on-screen offset — this stays correct even if
 * content was also appended at the *bottom* of the container during the
 * fetch (e.g. a new message arriving concurrently), unlike a pure
 * scrollHeight-delta adjustment.
 *
 * If the captured element is no longer present in the DOM by the time the
 * new page lands (an edge case — message elements are keyed by ID and
 * shouldn't normally be removed), this falls back to the delta-based
 * {@link computeScrollAnchorAdjustment} using the `scrollTop`/`scrollHeight`
 * captured at the same time.
 *
 * @returns {@link UseLoadOlderOnScrollResult}, letting callers trigger the
 *   same anchored fetch outside of a `scroll` event (see
 *   `triggerLoadOlder`'s docstring).
 */
export function useLoadOlderOnScroll({
  containerRef,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  pageCount,
}: UseLoadOlderOnScrollOptions): UseLoadOlderOnScrollResult {
  const anchorRef = useRef<ScrollAnchor | null>(null)

  // Synchronous in-flight guard, checked/set *before* fetchNextPage() is
  // ever called. `isFetchingNextPage` alone is not enough to prevent a
  // double-fire: it is React state that only updates on the next render, so
  // two `captureAnchorAndFetch` calls in the same tick (e.g. two `scroll`
  // events firing back-to-back, or a `scroll` event racing
  // MessageList's "container doesn't fill" effect's `triggerLoadOlder`
  // call) would both still observe the stale `isFetchingNextPage === false`
  // and both call `fetchNextPage()`. This ref is updated synchronously, so
  // the second call in the same tick sees it set and bails out. It is
  // cleared either when `fetchNextPage()`'s returned Promise settles (see
  // the `fetchNextPage` field's docstring) or, as a backstop, whenever
  // `pageCount` changes (the layout effect below) -- whichever happens
  // first.
  const isFetchingRef = useRef(false)

  // A plain useCallback over the current props: its identity changes
  // whenever hasNextPage/isFetchingNextPage/fetchNextPage do, which is fine
  // for both call sites (the scroll listener effect below and MessageList's
  // "container doesn't fill" effect already list this function itself, or
  // these same values, as dependencies -- so an identity change here only
  // ever coincides with a dependency change those effects would react to
  // anyway).
  const captureAnchorAndFetch = useCallback(() => {
    const el = containerRef.current
    if (!el || !hasNextPage || isFetchingNextPage || isFetchingRef.current) return

    const topElement = findTopmostVisibleMessageElement(el)
    anchorRef.current = {
      element: topElement,
      offsetInContainer: topElement
        ? topElement.getBoundingClientRect().top - el.getBoundingClientRect().top
        : 0,
      fallbackScrollTop: el.scrollTop,
      fallbackScrollHeight: el.scrollHeight,
    }
    isFetchingRef.current = true
    const result = fetchNextPage()
    if (result instanceof Promise) {
      void result.finally(() => {
        isFetchingRef.current = false
      })
    }
  }, [containerRef, hasNextPage, isFetchingNextPage, fetchNextPage])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return undefined

    const handleScroll = () => {
      if (!hasNextPage || isFetchingNextPage) return
      if (el.scrollTop > LOAD_THRESHOLD_PX) return

      captureAnchorAndFetch()
    }

    el.addEventListener("scroll", handleScroll, { passive: true })
    return () => el.removeEventListener("scroll", handleScroll)
  }, [containerRef, hasNextPage, isFetchingNextPage, captureAnchorAndFetch])

  useLayoutEffect(() => {
    // Backstop clear for the in-flight guard: a new page landing is
    // conclusive proof the fetch this hook triggered has settled, even if
    // the Promise-based `.finally` above hasn't run yet (e.g. a caller
    // whose `fetchNextPage` doesn't return a Promise at all).
    isFetchingRef.current = false

    const el = containerRef.current
    const anchor = anchorRef.current
    if (!el || !anchor) return

    if (anchor.element && el.contains(anchor.element)) {
      const newOffsetInContainer =
        anchor.element.getBoundingClientRect().top - el.getBoundingClientRect().top
      el.scrollTop = computeElementAnchorScrollTop(
        el.scrollTop,
        anchor.offsetInContainer,
        newOffsetInContainer,
      )
    } else {
      el.scrollTop = computeScrollAnchorAdjustment(
        anchor.fallbackScrollTop,
        anchor.fallbackScrollHeight,
        el.scrollHeight,
      )
    }
    anchorRef.current = null
    // Deliberately keyed on `pageCount` (not `containerRef`/other deps) so
    // this only runs once per newly-landed older page, after its messages
    // have actually been committed to the DOM.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageCount])

  return { triggerLoadOlder: captureAnchorAndFetch }
}
