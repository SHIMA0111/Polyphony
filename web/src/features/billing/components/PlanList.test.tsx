import { afterEach, beforeEach, describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { mockLocationHref } from "@/test/mock-location"
import { fixturePlans } from "../api/handlers"
import { PlanList } from "./PlanList"

/** Mirrors `PlanCard.tsx`'s own price formatting so assertions don't hardcode locale output. */
function formatPrice(priceCents: number, currency: string): string {
  return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(
    priceCents / 100,
  )
}

/**
 * Component-level tests for `PlanList`, covering:
 * - plan cards rendering with correctly formatted prices from a mocked
 *   `GET /api/proxy/billing/plans` (the fixture from `../api/handlers.ts`);
 * - clicking "Subscribe" calls the checkout-session mutation and navigates
 *   (asserted via the mocked `window.location.href` assignment, since
 *   Stripe Checkout is a real cross-origin redirect that jsdom cannot and
 *   should not actually perform).
 */
describe("PlanList", () => {
  let restoreLocation: () => void

  beforeEach(() => {
    restoreLocation = mockLocationHref()
  })

  afterEach(() => {
    restoreLocation()
  })

  it("renders plan cards with correctly formatted prices", async () => {
    render(<PlanList />)

    const [monthlyPlan, oneTimePlan] = fixturePlans

    await waitFor(() => expect(screen.getByText(monthlyPlan.name)).toBeInTheDocument())
    expect(screen.getByText(oneTimePlan.name)).toBeInTheDocument()

    expect(
      screen.getByText(formatPrice(monthlyPlan.price_cents, monthlyPlan.currency), {
        exact: false,
      }),
    ).toBeInTheDocument()
    expect(
      screen.getByText(formatPrice(oneTimePlan.price_cents, oneTimePlan.currency), {
        exact: false,
      }),
    ).toBeInTheDocument()

    // Monthly plans get a "Subscribe" CTA, one-time packs a "Buy tokens" CTA.
    expect(screen.getByRole("button", { name: "Subscribe" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Buy tokens" })).toBeInTheDocument()
  })

  it('clicking "Subscribe" calls the checkout-session mutation and navigates to checkout_url', async () => {
    const user = userEvent.setup()
    render(<PlanList />)

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Subscribe" })).toBeInTheDocument(),
    )

    await user.click(screen.getByRole("button", { name: "Subscribe" }))

    await waitFor(() =>
      expect(window.location.href).toBe("https://checkout.stripe.com/c/pay/cs_test_fixture"),
    )
  })
})
