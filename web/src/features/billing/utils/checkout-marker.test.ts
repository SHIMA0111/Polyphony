import { afterEach, describe, expect, it } from "vitest"
import { readAndClearCheckoutMarker, writeCheckoutMarker } from "./checkout-marker"

/**
 * Unit tests for `checkout-marker.ts`, covering:
 * - the subscription/token-purchase round trip through `sessionStorage`;
 * - the marker being cleared after one read (so a revisit doesn't replay a
 *   stale poll);
 * - `null` on a missing, malformed, or unparseable marker.
 */
describe("checkout-marker", () => {
  afterEach(() => {
    window.sessionStorage.clear()
  })

  it("round-trips a subscription marker", () => {
    writeCheckoutMarker({ kind: "subscription" })
    expect(readAndClearCheckoutMarker()).toEqual({ kind: "subscription" })
  })

  it("round-trips a token_purchase marker with priorBalance", () => {
    writeCheckoutMarker({ kind: "token_purchase", priorBalance: 4_200 })
    expect(readAndClearCheckoutMarker()).toEqual({
      kind: "token_purchase",
      priorBalance: 4_200,
    })
  })

  it("clears the marker after reading it once", () => {
    writeCheckoutMarker({ kind: "subscription" })
    expect(readAndClearCheckoutMarker()).toEqual({ kind: "subscription" })
    expect(readAndClearCheckoutMarker()).toBeNull()
  })

  it("returns null when no marker was ever written", () => {
    expect(readAndClearCheckoutMarker()).toBeNull()
  })

  it("returns null for a malformed marker", () => {
    window.sessionStorage.setItem("billing:checkout-marker", JSON.stringify({ kind: "bogus" }))
    expect(readAndClearCheckoutMarker()).toBeNull()
  })

  it("returns null for a token_purchase marker missing priorBalance", () => {
    window.sessionStorage.setItem(
      "billing:checkout-marker",
      JSON.stringify({ kind: "token_purchase" }),
    )
    expect(readAndClearCheckoutMarker()).toBeNull()
  })

  it("returns null for unparseable JSON", () => {
    window.sessionStorage.setItem("billing:checkout-marker", "not json")
    expect(readAndClearCheckoutMarker()).toBeNull()
  })
})
