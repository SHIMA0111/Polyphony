import { http, HttpResponse } from "msw"
import { afterEach, beforeEach, describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { mockLocationHref } from "@/test/mock-location"
import type { BillingPlan } from "../types"
import { fixturePlans } from "../api/handlers"
import { PlanList } from "./PlanList"

/**
 * Mirrors `PlanCard.tsx`'s own price formatting so assertions don't hardcode
 * locale output — reads the minor-unit exponent from the formatter's own
 * `resolvedOptions()` rather than assuming `/ 100`, since that's not true
 * for every currency (e.g. JPY has 0 decimal digits, KWD has 3), except for
 * the Stripe special cases below (ISK, UGX): ISO 4217 treats them as
 * zero-decimal, but Stripe always represents amounts in these two
 * currencies with 2 decimal digits regardless.
 */
const STRIPE_EXPONENT_OVERRIDES: Record<string, number> = {
  isk: 2,
  ugx: 2,
}

function formatPrice(priceCents: number, currency: string): string {
  const formatter = new Intl.NumberFormat(undefined, { style: "currency", currency })
  const exponent =
    STRIPE_EXPONENT_OVERRIDES[currency.toLowerCase()] ??
    formatter.resolvedOptions().maximumFractionDigits ??
    2
  // `getByText`'s whitespace-collapsing normalizer only runs on the DOM's
  // own text, not on this expected string (see `matches.js`'s
  // `getDefaultNormalizer`) — some currency formats (e.g. KWD) separate the
  // symbol from the amount with a non-breaking space (` `), which the
  // normalizer collapses to a regular space, so this must match that too.
  return formatter.format(priceCents / 10 ** exponent).replace(/\u00a0/g, " ")
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

  it("disables every card's CTA while any card's checkout session is pending", async () => {
    // A controlled promise, not a fixed timeout: the mocked POST handler
    // blocks on it until the test explicitly resolves it below, so the
    // disabled-state assertions run while the request is *provably* still
    // pending, rather than racing an arbitrary delay that could resolve
    // before (or long after) those assertions run.
    let resolveCheckoutSession = () => {}
    const checkoutSessionGate = new Promise<void>((resolve) => {
      resolveCheckoutSession = resolve
    })

    server.use(
      http.post("/api/proxy/billing/checkout-session", async () => {
        await checkoutSessionGate
        return HttpResponse.json(
          { checkout_url: "https://checkout.stripe.com/c/pay/cs_test_fixture" },
          { status: 201 },
        )
      }),
    )

    const user = userEvent.setup()
    render(<PlanList />)

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Subscribe" })).toBeInTheDocument(),
    )

    // Grab both cards' CTAs *before* clicking — the mutation is hoisted to
    // `PlanList` and shared by every card, so once pending, both buttons'
    // accessible name switches to the shared `loadingText` ("Redirecting...")
    // rather than just the clicked one's; asserting via a stable pre-click
    // reference (rather than re-querying by the now-stale "Buy tokens" name)
    // is what actually proves *every* card disables, not just the one
    // clicked.
    const [subscribeButton, buyTokensButton] = screen.getAllByRole("button")

    await user.click(subscribeButton)

    // Both cards' CTAs disable while the shared mutation is pending — not
    // just the one that was clicked. The mocked request is still blocked on
    // checkoutSessionGate here, so this cannot pass merely by having
    // outrun an already-resolved request.
    await waitFor(() => expect(subscribeButton).toBeDisabled())
    expect(buyTokensButton).toBeDisabled()

    // Only now let the mocked request resolve, and confirm the redirect
    // follows.
    resolveCheckoutSession()

    await waitFor(() =>
      expect(window.location.href).toBe("https://checkout.stripe.com/c/pay/cs_test_fixture"),
    )
  })

  it("formats zero-decimal (JPY) and three-decimal (KWD) currencies correctly", async () => {
    const jpyPlan: BillingPlan = {
      code: "jpy-pack",
      name: "JPY Pack",
      description: "A yen-denominated token pack.",
      price_cents: 500,
      currency: "jpy",
      interval: "one_time",
      token_allowance: 50_000,
    }
    const kwdPlan: BillingPlan = {
      code: "kwd-pack",
      name: "KWD Pack",
      description: "A dinar-denominated token pack.",
      price_cents: 1_500,
      currency: "kwd",
      interval: "one_time",
      token_allowance: 150_000,
    }

    server.use(
      http.get("/api/proxy/billing/plans", () => {
        return HttpResponse.json<{ plans: BillingPlan[] }>({ plans: [jpyPlan, kwdPlan] })
      }),
    )

    render(<PlanList />)

    await waitFor(() => expect(screen.getByText(jpyPlan.name)).toBeInTheDocument())

    // JPY is zero-decimal: 500 minor units is ¥500, not ¥5.00.
    expect(
      screen.getByText(formatPrice(jpyPlan.price_cents, jpyPlan.currency), { exact: false }),
    ).toBeInTheDocument()
    // KWD uses 3 decimal digits: 1500 minor units is KD 1.500, not KD 15.00.
    expect(
      screen.getByText(formatPrice(kwdPlan.price_cents, kwdPlan.currency), { exact: false }),
    ).toBeInTheDocument()
  })

  it("formats Stripe's ISK/UGX special cases as 2-decimal despite Intl treating them as zero-decimal", async () => {
    const iskPlan: BillingPlan = {
      code: "isk-pack",
      name: "ISK Pack",
      description: "A krona-denominated token pack.",
      price_cents: 500,
      currency: "isk",
      interval: "one_time",
      token_allowance: 50_000,
    }
    const ugxPlan: BillingPlan = {
      code: "ugx-pack",
      name: "UGX Pack",
      description: "A shilling-denominated token pack.",
      price_cents: 1_500,
      currency: "ugx",
      interval: "one_time",
      token_allowance: 150_000,
    }

    server.use(
      http.get("/api/proxy/billing/plans", () => {
        return HttpResponse.json<{ plans: BillingPlan[] }>({ plans: [iskPlan, ugxPlan] })
      }),
    )

    render(<PlanList />)

    await waitFor(() => expect(screen.getByText(iskPlan.name)).toBeInTheDocument())

    // Without the override, `Intl` would treat ISK as zero-decimal and
    // render 500 minor units as ISK 500 -- 100x the correct amount. Stripe's
    // own convention divides by 100 regardless, same as USD.
    expect(
      screen.getByText(formatPrice(iskPlan.price_cents, iskPlan.currency), { exact: false }),
    ).toBeInTheDocument()
    expect(
      screen.getByText(formatPrice(ugxPlan.price_cents, ugxPlan.currency), { exact: false }),
    ).toBeInTheDocument()
  })
})
