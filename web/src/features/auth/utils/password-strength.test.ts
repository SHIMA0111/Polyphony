import { describe, expect, it } from "vitest"
import { getPasswordStrength } from "./password-strength"

describe("getPasswordStrength", () => {
  it("scores 0 for a short, all-lowercase password with no digit/symbol", () => {
    expect(getPasswordStrength("abc")).toBe(0)
  })

  it("scores 1 for a password that only clears the >=8 length threshold", () => {
    expect(getPasswordStrength("abcdefgh")).toBe(1)
  })

  it("scores 2 for a password that clears both length thresholds", () => {
    expect(getPasswordStrength("abcdefghijkl")).toBe(2)
  })

  it("scores an extra point for mixed-case letters", () => {
    expect(getPasswordStrength("Abcdefgh")).toBe(2)
  })

  it("scores an extra point for a digit", () => {
    expect(getPasswordStrength("abcdefg1")).toBe(2)
  })

  it("scores an extra point for a symbol", () => {
    expect(getPasswordStrength("abcdefg!")).toBe(2)
  })

  it("counts an underscore as a non-letter character, per the documented contract", () => {
    // \W (used by an earlier, buggier implementation) excludes "_" since
    // regex word-character class treats underscore as a word character;
    // the documented contract is "any non-letter character", which includes
    // "_". This must score higher than the letters-only baseline.
    expect(getPasswordStrength("abcdefg_")).toBe(2)
    expect(getPasswordStrength("abcdefg_")).toBeGreaterThan(getPasswordStrength("abcdefgh"))
  })

  it("clamps the maximum score to 4", () => {
    expect(getPasswordStrength("Abcdefghijkl1!")).toBe(4)
  })
})
