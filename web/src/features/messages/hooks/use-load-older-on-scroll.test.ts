import { act, renderHook } from "@testing-library/react"
import { createRef } from "react"
import { describe, expect, it, vi } from "vitest"
import {
  computeElementAnchoredScrollTop,
  computeScrollAnchorAdjustment,
  useLoadOlderOnScroll,
} from "./use-load-older-on-scroll"

describe("computeScrollAnchorAdjustment", () => {
  it("shifts scrollTop down by the added height when older content is prepended", () => {
    // 500px of older content was added above the previously-visible content
    // (2000 -> 2500); the reader was 300px from the top.
    expect(computeScrollAnchorAdjustment(300, 2000, 2500)).toBe(800)
  })

  it("returns the original scrollTop when the content height didn't change", () => {
    expect(computeScrollAnchorAdjustment(150, 2000, 2000)).toBe(150)
  })

  it("handles a reader at the very top of the container", () => {
    expect(computeScrollAnchorAdjustment(0, 1000, 1400)).toBe(400)
  })
})

describe("computeElementAnchoredScrollTop", () => {
  it("shifts scrollTop down by however far the anchor element moved down", () => {
    // The anchor was 40px from the container's top edge; after the older
    // page landed it measures 540px from the top (500px of older content
    // was prepended above it). scrollTop must grow by that same 500px.
    expect(computeElementAnchoredScrollTop(300, 40, 540)).toBe(800)
  })

  it("returns the original scrollTop when the anchor element didn't move", () => {
    expect(computeElementAnchoredScrollTop(150, 40, 40)).toBe(150)
  })

  it("is unaffected by content added below the anchor element", () => {
    // This is the scenario computeScrollAnchorAdjustment gets wrong: a live
    // message (or a growing streaming response) appended *below* the
    // visible area during the same render as the older-page prepend still
    // grows scrollHeight, but never changes the anchor element's own
    // getBoundingClientRect() offset from the container's top — so the
    // element-anchored correction is unaffected by it, unlike a bare
    // scrollHeight-delta correction would be.
    const beforeOffset = 40
    const afterOffset = 540 // only the 500px prepended above the anchor moved it
    expect(computeElementAnchoredScrollTop(300, beforeOffset, afterOffset)).toBe(
      800,
    )
  })

  it("handles an anchor already at the container's top edge", () => {
    expect(computeElementAnchoredScrollTop(0, 0, 400)).toBe(400)
  })
})

describe("useLoadOlderOnScroll's triggerLoadOlder double-fire guard", () => {
  it("only calls fetchNextPage once when triggerLoadOlder is invoked twice synchronously, before isFetchingNextPage has had a chance to flip", () => {
    const containerRef = createRef<HTMLDivElement>()
    containerRef.current = document.createElement("div")

    const fetchNextPage = vi.fn()
    const { result } = renderHook(() =>
      useLoadOlderOnScroll({
        containerRef,
        hasNextPage: true,
        isFetchingNextPage: false,
        fetchNextPage,
        pageCount: 1,
      }),
    )

    // Two synchronous calls in the same tick, exactly like a `scroll` event
    // firing back-to-back with MessageList.tsx's own viewport-underfill
    // call -- both still observe the stale isFetchingNextPage=false prop,
    // so only the synchronous isLoadingRef guard can prevent a second
    // dispatch.
    act(() => {
      result.current.triggerLoadOlder()
      result.current.triggerLoadOlder()
    })

    expect(fetchNextPage).toHaveBeenCalledTimes(1)
  })

  it("releases the guard once isFetchingNextPage flips back to false, allowing a subsequent trigger even without a pageCount change (a failed fetch)", () => {
    const containerRef = createRef<HTMLDivElement>()
    containerRef.current = document.createElement("div")

    const fetchNextPage = vi.fn()
    const { result, rerender } = renderHook(
      (props: { isFetchingNextPage: boolean }) =>
        useLoadOlderOnScroll({
          containerRef,
          hasNextPage: true,
          isFetchingNextPage: props.isFetchingNextPage,
          fetchNextPage,
          pageCount: 1,
        }),
      { initialProps: { isFetchingNextPage: false } },
    )

    act(() => {
      result.current.triggerLoadOlder()
    })
    expect(fetchNextPage).toHaveBeenCalledTimes(1)

    // Fetch is now in flight: a second trigger must be a no-op, guarded by
    // the (now-current) isFetchingNextPage prop.
    rerender({ isFetchingNextPage: true })
    act(() => {
      result.current.triggerLoadOlder()
    })
    expect(fetchNextPage).toHaveBeenCalledTimes(1)

    // The fetch settles (failed, so pageCount never changed) and
    // isFetchingNextPage flips back to false: the guard must release so a
    // retry can fire.
    rerender({ isFetchingNextPage: false })
    act(() => {
      result.current.triggerLoadOlder()
    })
    expect(fetchNextPage).toHaveBeenCalledTimes(2)
  })
})
