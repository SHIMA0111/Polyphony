import { describe, expect, it } from "vitest"
import { computeIsNearBottom } from "./use-near-bottom-scroll"

describe("computeIsNearBottom", () => {
  it("returns true when within the default pixel threshold of the bottom", () => {
    // scrollHeight - scrollTop - clientHeight = 1000 - 850 - 100 = 50 <= 120
    expect(computeIsNearBottom(850, 1000, 100)).toBe(true)
  })

  it("returns true when exactly at the bottom", () => {
    expect(computeIsNearBottom(900, 1000, 100)).toBe(true)
  })

  it("returns false when scrolled further up than the threshold", () => {
    // scrollHeight - scrollTop - clientHeight = 1000 - 500 - 100 = 400 > 120
    expect(computeIsNearBottom(500, 1000, 100)).toBe(false)
  })

  it("returns true exactly at the threshold boundary", () => {
    // 1000 - 780 - 100 = 120, threshold is inclusive
    expect(computeIsNearBottom(780, 1000, 100)).toBe(true)
  })

  it("returns false just past the threshold boundary", () => {
    // 1000 - 779 - 100 = 121 > 120
    expect(computeIsNearBottom(779, 1000, 100)).toBe(false)
  })

  it("respects a custom threshold", () => {
    expect(computeIsNearBottom(400, 1000, 100, 600)).toBe(true)
    expect(computeIsNearBottom(400, 1000, 100, 400)).toBe(false)
  })

  it("returns true when content is shorter than the viewport", () => {
    expect(computeIsNearBottom(0, 50, 100)).toBe(true)
  })
})
