import { describe, expect, it } from "vitest"
import { computeScrollAnchorAdjustment } from "./use-load-older-on-scroll"

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
