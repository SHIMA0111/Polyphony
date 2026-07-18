import { describe, expect, it } from "vitest"
import { formatDateTimeUtc } from "./format"

describe("formatDateTimeUtc", () => {
  it("formats an ISO timestamp pinned to UTC with a trailing 'UTC' suffix", () => {
    // 23:30 UTC deliberately falls on a different calendar day in most
    // non-UTC timezones, so this also guards against a regression back to
    // the runtime's own local timezone.
    expect(formatDateTimeUtc("2026-07-14T23:30:00Z")).toBe("Jul 14, 2026, 11:30 PM UTC")
  })

  it("always ends with the literal 'UTC' suffix, distinguishing it from the viewer's local time", () => {
    expect(formatDateTimeUtc("2026-01-01T00:00:00Z")).toMatch(/ UTC$/)
  })
})
