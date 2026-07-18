import { describe, expect, it } from "vitest"
import {
  computeElementAnchorScrollTop,
  computeScrollAnchorAdjustment,
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

describe("computeElementAnchorScrollTop", () => {
  it("increases scrollTop by however far the anchor element moved down", () => {
    // The anchor element was at offset 50px from the container's top; after
    // older content landed above it, it's now at 550px — 500px of content
    // was added above it, so scrollTop must increase by 500px to restore it.
    expect(computeElementAnchorScrollTop(300, 50, 550)).toBe(800)
  })

  it("returns the original scrollTop when the anchor element didn't move", () => {
    expect(computeElementAnchorScrollTop(150, 80, 80)).toBe(150)
  })

  it("stays correct even when content is also appended below the fold", () => {
    // Only the anchor element's own offset matters — unlike
    // computeScrollAnchorAdjustment, this is unaffected by scrollHeight
    // growth elsewhere in the container (e.g. a new message arriving below
    // the visible area during the same fetch).
    expect(computeElementAnchorScrollTop(0, 100, 500)).toBe(400)
  })
})
