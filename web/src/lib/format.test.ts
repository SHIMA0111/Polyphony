import { describe, expect, it } from "vitest"
import { formatCurrency } from "./format"

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
})
