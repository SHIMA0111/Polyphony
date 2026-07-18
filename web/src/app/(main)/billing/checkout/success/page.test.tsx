import { http, HttpResponse } from "msw"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import BillingCheckoutSuccessPage from "./page"

/**
 * `useSearchParams()` is mocked directly rather than routed through a real
 * Next.js router — this page only ever reads the single "session_id" query
 * parameter, so each test controls it via `mockSearchParams.mockReturnValue`.
 */
const mockSearchParams = vi.fn<() => URLSearchParams>()
vi.mock("next/navigation", async (importOriginal) => {
  const actual = await importOriginal<typeof import("next/navigation")>()
  return {
    ...actual,
    useSearchParams: () => mockSearchParams(),
  }
})

/**
 * Component tests for the redesigned `billing/checkout/success/page.tsx`,
 * covering the `session_id`-driven confirmation mechanism that replaced the
 * `sessionStorage` `checkout-marker.ts` marker:
 * - no `session_id` (or an empty/whitespace-only one) renders the neutral
 *   "purchase received" message;
 * - a `session_id` matching a `payment_history` row's `stripe_reference_id`
 *   renders the token-purchase confirmation, even when a subscription also
 *   happens to be active (matching payment takes priority);
 * - a `session_id` matching the subscription's own
 *   `stripe_checkout_session_id`, with no matching payment, renders the
 *   subscription confirmation;
 * - a `session_id` matching neither a payment record nor the subscription's
 *   `stripe_checkout_session_id` keeps polling rather than confirming an
 *   unrelated already-active subscription;
 * - a polled query erroring outright renders the distinct retryable error
 *   state.
 */
describe("BillingCheckoutSuccessPage", () => {
  beforeEach(() => {
    mockSearchParams.mockReset()
  })

  it("renders a neutral message when there is no session_id", async () => {
    mockSearchParams.mockReturnValue(new URLSearchParams())

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() => expect(screen.getByText("Checkout finished")).toBeInTheDocument())
    expect(
      screen.getByText(/if you just completed a checkout/i),
    ).toBeInTheDocument()
    expect(screen.queryByText(/Order reference/)).not.toBeInTheDocument()
  })

  it("renders a neutral message when session_id is present but empty", async () => {
    // `?session_id=` (present but valueless) must be normalized to the same
    // "nothing to confirm" branch as an absent session_id, not poll forever
    // against an empty string no payment or subscription record could ever
    // match.
    mockSearchParams.mockReturnValue(new URLSearchParams("session_id="))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() => expect(screen.getByText("Checkout finished")).toBeInTheDocument())
    expect(screen.queryByText(/Order reference/)).not.toBeInTheDocument()
  })

  it("confirms a token purchase when session_id matches a payment_history row, even with an active subscription", async () => {
    // The default /billing/subscription fixture is already "active" (see
    // `../../../../../features/billing/api/handlers.ts`'s fixtureSubscription),
    // and fixturePaymentPage1's "pay-1" row carries
    // stripe_reference_id "cs_test_pay_1" — this asserts the matching
    // payment record wins the display priority over the active subscription.
    mockSearchParams.mockReturnValue(new URLSearchParams("session_id=cs_test_pay_1"))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() => expect(screen.getByText("Tokens added")).toBeInTheDocument())
    expect(screen.getByText(/100,000 tokens/)).toBeInTheDocument()
    expect(screen.getByText(/Order reference: cs_test_pay_1/)).toBeInTheDocument()
    expect(screen.queryByText("You're all set")).not.toBeInTheDocument()
  })

  it("confirms a subscription when session_id matches the subscription's own stripe_checkout_session_id, with no matching payment", async () => {
    // fixtureSubscription's stripe_checkout_session_id is "cs_test_fixture"
    // (see ../../../../../features/billing/api/handlers.ts) and no
    // payment_history fixture row references this session_id, so only the
    // session-matched subscription branch can resolve this.
    mockSearchParams.mockReturnValue(new URLSearchParams("session_id=cs_test_fixture"))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() => expect(screen.getByText("You're all set")).toBeInTheDocument())
    expect(screen.getByText("Pro")).toBeInTheDocument()
    expect(screen.getByText(/renews on/i)).toBeInTheDocument()
    expect(screen.getByText(/Order reference: cs_test_fixture/)).toBeInTheDocument()
  })

  it("keeps polling when session_id matches neither a payment record nor the subscription's stripe_checkout_session_id", async () => {
    // The subscription fixture is active but its stripe_checkout_session_id
    // is "cs_test_fixture", not this session_id — an active subscription
    // that this checkout did not itself produce must not be mistaken for
    // this session's confirmation (e.g. a stale/abandoned Checkout link for
    // a user who already has an unrelated active subscription).
    mockSearchParams.mockReturnValue(new URLSearchParams("session_id=cs_test_unrelated"))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() => expect(screen.getByText(/Confirming your payment/)).toBeInTheDocument())
    expect(screen.queryByText("You're all set")).not.toBeInTheDocument()
  })

  it("renders a distinct retryable error state when a polled query fails", async () => {
    server.use(
      http.get("/api/proxy/billing/subscription", () => {
        return HttpResponse.json({ message: "internal error" }, { status: 500 })
      }),
    )
    mockSearchParams.mockReturnValue(new URLSearchParams("session_id=cs_test_error"))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() =>
      expect(screen.getByText("Couldn't confirm your payment")).toBeInTheDocument(),
    )
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument()
  })

  it("does not resolve on a subscription status of 'none'", async () => {
    server.use(
      http.get("/api/proxy/billing/subscription", () => {
        return new HttpResponse(null, { status: 204 })
      }),
    )
    mockSearchParams.mockReturnValue(new URLSearchParams("session_id=cs_test_pending"))

    render(<BillingCheckoutSuccessPage />)

    await waitFor(() => expect(screen.getByText(/Confirming your payment/)).toBeInTheDocument())
  })
})
