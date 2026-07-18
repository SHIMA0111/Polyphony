import { describe, expect, it } from "vitest"
import { formatCurrency, formatDateTimeUtc } from "./format"

/**
 * Unit tests for `formatCurrency`, focused on the minor-unit exponent: most
 * currencies use 2 decimal places (cents), but a hardcoded `/ 100` silently
 * misformats zero-decimal currencies (e.g. `"jpy"`) and three-decimal
 * currencies (e.g. `"kwd"`) alike.
 */
describe("formatCurrency", () => {
  it("divides by 100 for a standard 2-decimal currency (usd)", () => {
    expect(formatCurrency(1_050, "usd")).toBe(
      new Intl.NumberFormat(undefined, { style: "currency", currency: "usd" }).format(10.5),
    )
  })

  it("does not divide a zero-decimal currency (jpy)", () => {
    // JPY has no minor unit: 1050 minor units === 1050 yen, not 10.5 yen.
    expect(formatCurrency(1_050, "jpy")).toBe(
      new Intl.NumberFormat(undefined, { style: "currency", currency: "jpy" }).format(1_050),
    )
  })

  it("divides by 1000 for a three-decimal currency (kwd)", () => {
    // KWD's minor unit is the fils: 1000 fils === 1 dinar.
    expect(formatCurrency(1_050, "kwd")).toBe(
      new Intl.NumberFormat(undefined, { style: "currency", currency: "kwd" }).format(1.05),
    )
  })

  // --- Stripe/ISO 4217 disagreement overrides (ISK, UGX) ---

  it("divides by 100 for ISK despite Intl/ISO 4217 treating it as zero-decimal", () => {
    // Stripe bills ISK as an ordinary 2-decimal currency; without the
    // override this would fall back to Intl's own zero-decimal exponent and
    // overstate the amount 100x (1050 ISK instead of 10.50 ISK).
    expect(formatCurrency(1_050, "isk")).toBe(
      new Intl.NumberFormat(undefined, { style: "currency", currency: "isk" }).format(10.5),
    )
  })

  it("divides by 100 for UGX despite Intl/ISO 4217 treating it as zero-decimal", () => {
    expect(formatCurrency(1_050, "ugx")).toBe(
      new Intl.NumberFormat(undefined, { style: "currency", currency: "ugx" }).format(10.5),
    )
  })

  it("applies the ISK/UGX override case-insensitively, matching Stripe's own lowercase currency codes", () => {
    expect(formatCurrency(1_050, "ISK")).toBe(
      new Intl.NumberFormat(undefined, { style: "currency", currency: "ISK" }).format(10.5),
    )
  })
})

/**
 * Unit tests for `formatDateTimeUtc`: pins `timeZone: "UTC"` for
 * server/browser hydration safety (see `./format`'s own doc comment) and,
 * unlike the pre-rename `formatDateTimeLocal`, renders a trailing "UTC"
 * marker so the output itself -- not just the function name -- makes clear
 * the timestamp is not the viewer's local time.
 */
describe("formatDateTimeUtc", () => {
  it("formats an ISO timestamp as a UTC-pinned date and time with a trailing UTC marker", () => {
    expect(formatDateTimeUtc("2026-07-14T15:45:00Z")).toBe("Jul 14, 2026, 3:45 PM UTC")
  })

  it("renders the UTC calendar date/hour even when they differ from the instant's other-timezone equivalent", () => {
    // 2026-01-01T23:30:00Z is already 2026-01-02 in most timezones east of
    // UTC; a non-UTC-pinned formatter would render a different day/hour
    // depending on the runtime's own local timezone.
    expect(formatDateTimeUtc("2026-01-01T23:30:00Z")).toBe("Jan 1, 2026, 11:30 PM UTC")
  })
})
