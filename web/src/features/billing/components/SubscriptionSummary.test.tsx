import { http, HttpResponse } from "msw"
import { afterEach, beforeEach, describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { mockLocationHref } from "@/test/mock-location"
import type { Subscription } from "../types"
import { SubscriptionSummary } from "./SubscriptionSummary"

/**
 * Component-level tests for `SubscriptionSummary`, covering:
 * - the "no subscription" state (a mocked `204` on
 *   `GET /api/proxy/billing/subscription`), including its link to
 *   `/billing/plans`;
 * - an active-subscription state rendering the resolved plan name and a
 *   correctly colored status `Badge` (the fixture from `../api/handlers.ts`);
 * - clicking "Manage subscription" calls the billing-portal mutation and
 *   navigates (asserted via the mocked `window.location.href` assignment).
 */
describe("SubscriptionSummary", () => {
  let restoreLocation: () => void

  beforeEach(() => {
    restoreLocation = mockLocationHref()
  })

  afterEach(() => {
    restoreLocation()
  })

  it('renders the "no subscription" state with a link to /billing/plans', async () => {
    server.use(
      http.get("/api/proxy/billing/subscription", () => {
        return new HttpResponse(null, { status: 204 })
      }),
    )

    render(<SubscriptionSummary />)

    await waitFor(() =>
      expect(
        screen.getByText("You don't have an active subscription"),
      ).toBeInTheDocument(),
    )
    expect(screen.getByRole("link", { name: "View plans" })).toHaveAttribute(
      "href",
      "/billing/plans",
    )
  })

  it("renders an active subscription with the resolved plan name and status badge", async () => {
    render(<SubscriptionSummary />)

    await waitFor(() => expect(screen.getByText("Pro")).toBeInTheDocument())
    expect(screen.getByText("active")).toHaveAttribute("data-status", "active")
  })

  it("renders a canceling (still-active) subscription's status badge and cancellation notice", async () => {
    const canceling: Subscription = {
      status: "active",
      plan_code: "pro-monthly",
      monthly_token_allocation: 500_000,
      current_period_start: "2026-01-01T00:00:00Z",
      current_period_end: "2026-02-01T00:00:00Z",
      cancel_at_period_end: true,
      canceled_at: null,
      stripe_checkout_session_id: "cs_test_canceling",
    }
    server.use(
      http.get("/api/proxy/billing/subscription", () => {
        return HttpResponse.json<Subscription>(canceling)
      }),
    )

    render(<SubscriptionSummary />)

    await waitFor(() => expect(screen.getByText("active")).toBeInTheDocument())
    expect(
      screen.getByText(/will not renew/i),
    ).toBeInTheDocument()
    expect(screen.queryByText(/renews/i)).not.toBeInTheDocument()
  })

  it("never renders 'Renews' for an already-canceled subscription, even without cancel_at_period_end", async () => {
    const canceled: Subscription = {
      status: "canceled",
      plan_code: "pro-monthly",
      monthly_token_allocation: 500_000,
      current_period_start: "2026-01-01T00:00:00Z",
      current_period_end: "2026-02-01T00:00:00Z",
      cancel_at_period_end: false,
      canceled_at: "2026-01-15T00:00:00Z",
      stripe_checkout_session_id: "cs_test_canceled",
    }
    server.use(
      http.get("/api/proxy/billing/subscription", () => {
        return HttpResponse.json<Subscription>(canceled)
      }),
    )

    render(<SubscriptionSummary />)

    await waitFor(() => expect(screen.getByText("canceled")).toBeInTheDocument())
    expect(screen.getByText(/ended on/i)).toBeInTheDocument()
    expect(screen.queryByText(/renews/i)).not.toBeInTheDocument()
  })

  it('clicking "Manage subscription" calls the billing-portal mutation and navigates to portal_url', async () => {
    const user = userEvent.setup()
    render(<SubscriptionSummary />)

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Manage subscription" })).toBeInTheDocument(),
    )

    await user.click(screen.getByRole("button", { name: "Manage subscription" }))

    await waitFor(() =>
      expect(window.location.href).toBe("https://billing.stripe.com/p/session/fixture"),
    )
  })
})
