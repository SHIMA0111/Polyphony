import { describe, expect, it, vi } from "vitest"
import { render, screen, waitFor } from "@/test/render"
import BillingCheckoutSuccessPage from "./page"

const { useSearchParamsMock } = vi.hoisted(() => ({
  useSearchParamsMock: vi.fn(),
}))

vi.mock("next/navigation", () => ({
  useSearchParams: useSearchParamsMock,
  // `Provider` (via `src/test/render.tsx`) wraps every test in
  // `EmotionRegistry`, which calls this Next.js hook to flush Emotion's SSR
  // styles; jsdom never streams, so a no-op is all component tests need
  // (mirrors `(main)/layout.test.tsx`'s identical mock).
  useServerInsertedHTML: vi.fn(),
}))

/**
 * Tests for the post-Checkout success page, covering:
 * - no `session_id` (a direct/bookmarked visit) renders the neutral,
 *   non-erroring success message;
 * - a present-but-empty `?session_id=` is normalized to the same neutral
 *   branch rather than spinning forever (the fix this test guards);
 * - a `session_id` matching `fixtureSubscription.stripe_checkout_session_id`
 *   resolves the subscription success view;
 * - a `session_id` that matches neither the subscription nor any
 *   `payment_history` row keeps polling rather than resolving to any
 *   success view — proving the "any active subscription" fallback was
 *   actually removed (see `DualPolling`'s doc comment).
 */
describe("BillingCheckoutSuccessPage", () => {
  it("renders a neutral success message when there is no session_id", async () => {
    useSearchParamsMock.mockReturnValue(new URLSearchParams())

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() =>
      expect(screen.getByText("Payment received")).toBeInTheDocument(),
    )
    expect(screen.getByRole("link", { name: "Go to billing" })).toHaveAttribute(
      "href",
      "/billing",
    )
  })

  it("renders a neutral success message for a present-but-empty session_id", async () => {
    useSearchParamsMock.mockReturnValue(new URLSearchParams("session_id="))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() =>
      expect(screen.getByText("Payment received")).toBeInTheDocument(),
    )
  })

  it("resolves the subscription success view when stripe_checkout_session_id matches session_id", async () => {
    // Matches fixtureSubscription's stripe_checkout_session_id
    // (web/src/features/billing/api/handlers.ts).
    useSearchParamsMock.mockReturnValue(new URLSearchParams("session_id=cs_test_fixture"))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() =>
      expect(screen.getByText("You're all set")).toBeInTheDocument(),
    )
    expect(screen.getByText(/Order reference:/)).toHaveTextContent("cs_test_fixture")
  })

  it("keeps polling instead of resolving when session_id matches no subscription or payment", async () => {
    useSearchParamsMock.mockReturnValue(
      new URLSearchParams("session_id=cs_does_not_match_anything"),
    )

    render(<BillingCheckoutSuccessPage />)

    // Once both queries have settled, a non-matching session_id must leave
    // the page in its "still confirming" polling state — not the
    // subscription success view, which the old "any active subscription"
    // fallback would have wrongly rendered for this same fixture data.
    await waitFor(() =>
      expect(screen.getByText("Confirming your payment...")).toBeInTheDocument(),
    )
    expect(screen.queryByText("You're all set")).not.toBeInTheDocument()
    expect(screen.queryByText("Tokens added")).not.toBeInTheDocument()
  })
})
