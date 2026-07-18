import { http, HttpResponse } from "msw"
import type {
  BillingPlan,
  BillingPortalSessionResponse,
  CheckoutSessionResponse,
  Payment,
  PaymentHistoryPage,
  Subscription,
  TokenBalance,
  TokenTransaction,
  TokenTransactionPage,
} from "../types"

/**
 * MSW request handlers for the billing feature, used by the Node
 * `setupServer` (Vitest, see `src/test/msw/server.ts`).
 *
 * Every path is pinned to the `/api/proxy/*` contract that Step 4's BFF
 * catch-all proxy (`app/api/proxy/[...path]/route.ts`) forwards 1:1 to the
 * Go API.
 */

export const fixtureBalance: TokenBalance = {
  user_id: "user-1",
  balance: 4200,
  updated_at: "2026-01-03T00:00:00Z",
}

export const fixtureZeroBalance: TokenBalance = {
  user_id: "user-1",
  balance: 0,
  updated_at: "2026-01-03T00:00:00Z",
}

export const fixtureTransactionPage1: TokenTransaction[] = [
  {
    id: "txn-2",
    user_id: "user-1",
    room_id: "room-1",
    type: "consumption",
    amount: -300,
    balance_after: 4200,
    description: "AI response using gpt-5-mini (200 prompt + 100 output tokens)",
    created_at: "2026-01-03T00:00:00Z",
  },
  {
    id: "txn-1",
    user_id: "user-1",
    room_id: null,
    type: "charge",
    amount: 5000,
    balance_after: 4500,
    description: "Token top-up",
    created_at: "2026-01-01T00:00:00Z",
  },
]

export const fixtureTransactionPage2: TokenTransaction[] = [
  {
    id: "txn-0",
    user_id: "user-1",
    room_id: null,
    type: "adjustment",
    amount: -500,
    balance_after: -500,
    description: "Manual correction",
    created_at: "2025-12-31T00:00:00Z",
  },
]

// --- Step 53: plans, Stripe Checkout, subscription management, billing
// history. Fixtures/handlers below are additive to Step 48's above — no
// existing export is modified or removed.

export const fixturePlans: BillingPlan[] = [
  {
    code: "pro-monthly",
    name: "Pro",
    description: "For teams that need more AI throughput every month.",
    price_cents: 2900,
    currency: "usd",
    interval: "month",
    token_allowance: 500_000,
  },
  {
    code: "tokens-100k",
    name: "100k Token Pack",
    description: "A one-time top-up of 100,000 tokens, no expiry.",
    price_cents: 900,
    currency: "usd",
    interval: "one_time",
    token_allowance: 100_000,
  },
]

export const fixtureSubscription: Subscription = {
  status: "active",
  plan_code: "pro-monthly",
  monthly_token_allocation: 500_000,
  current_period_start: "2026-01-01T00:00:00Z",
  current_period_end: "2026-02-01T00:00:00Z",
  cancel_at_period_end: false,
  canceled_at: null,
}

export const fixturePaymentPage1: Payment[] = [
  {
    id: "pay-2",
    kind: "subscription",
    amount_cents: 2900,
    currency: "usd",
    tokens_credited: 500_000,
    status: "succeeded",
    stripe_reference_id: "in_test_fixture",
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "pay-1",
    kind: "token_purchase",
    amount_cents: 900,
    currency: "usd",
    tokens_credited: 100_000,
    status: "succeeded",
    // Matches the checkout-session fixture's `checkout_url` session id below,
    // so tests can exercise the success page's session_id-based match.
    stripe_reference_id: "cs_test_fixture",
    created_at: "2025-12-15T00:00:00Z",
  },
]

export const fixturePaymentPage2: Payment[] = [
  {
    id: "pay-0",
    kind: "subscription",
    amount_cents: 2900,
    currency: "usd",
    tokens_credited: 500_000,
    status: "succeeded",
    stripe_reference_id: "in_test_fixture_2",
    created_at: "2025-12-01T00:00:00Z",
  },
]

export const billingHandlers = [
  http.get("/api/proxy/billing/balance", () => {
    return HttpResponse.json<TokenBalance>(fixtureBalance)
  }),

  http.get("/api/proxy/billing/transactions", ({ request }) => {
    const url = new URL(request.url)
    const cursor = url.searchParams.get("cursor")

    if (cursor === "txn-1") {
      return HttpResponse.json<TokenTransactionPage>({
        transactions: fixtureTransactionPage2,
        next_cursor: null,
      })
    }

    return HttpResponse.json<TokenTransactionPage>({
      transactions: fixtureTransactionPage1,
      next_cursor: "txn-1",
    })
  }),

  http.get("/api/proxy/billing/plans", () => {
    return HttpResponse.json<{ plans: BillingPlan[] }>({ plans: fixturePlans })
  }),

  http.post("/api/proxy/billing/checkout-session", () => {
    return HttpResponse.json<CheckoutSessionResponse>(
      { checkout_url: "https://checkout.stripe.com/c/pay/cs_test_fixture" },
      { status: 201 },
    )
  }),

  http.get("/api/proxy/billing/subscription", () => {
    return HttpResponse.json<Subscription>(fixtureSubscription)
  }),

  http.post("/api/proxy/billing/portal-session", () => {
    return HttpResponse.json<BillingPortalSessionResponse>({
      portal_url: "https://billing.stripe.com/p/session/fixture",
    })
  }),

  http.get("/api/proxy/billing/payments", ({ request }) => {
    const url = new URL(request.url)
    const cursor = url.searchParams.get("cursor")

    if (cursor === "pay-1") {
      return HttpResponse.json<PaymentHistoryPage>({
        payments: fixturePaymentPage2,
        next_cursor: null,
      })
    }

    return HttpResponse.json<PaymentHistoryPage>({
      payments: fixturePaymentPage1,
      next_cursor: "pay-1",
    })
  }),
]
