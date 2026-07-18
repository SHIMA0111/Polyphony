/**
 * Billing-domain types, mirroring the Go API's billing DTOs
 * (`server/internal/interface/handler/dto.go`'s `TokenBalanceResponse` /
 * `TokenTransactionResponse` / `TokenTransactionListResponse`), reached
 * client-side via the Step 4 data-plane proxy at `/api/proxy/billing/*`.
 *
 * `balance` / `amount` / `balance_after` are plain token counts (not
 * currency, per phases.md Phase 16) — never format them with a currency
 * formatter.
 */

/** A user's current token balance, returned by `GET /billing/balance`. */
export interface TokenBalance {
  user_id: string
  balance: number
  updated_at: string
}

/**
 * The kind of ledger entry a `TokenTransaction` represents, matching the
 * `token_transactions.type` CHECK constraint in `server/schema.sql`:
 * `"consumption"` (AI usage debit), `"charge"` (a paid top-up), or
 * `"adjustment"` (a manual/administrative correction).
 */
export type TransactionType = "consumption" | "charge" | "adjustment"

/**
 * A single `token_transactions` ledger row. `amount` is signed (negative for
 * `"consumption"`, positive for `"charge"`/`"adjustment"`); `room_id` is
 * `null` for transactions not tied to any room (e.g. top-ups).
 */
export interface TokenTransaction {
  id: string
  user_id: string
  room_id: string | null
  type: TransactionType
  amount: number
  balance_after: number
  description: string
  created_at: string
}

/** Raw paginated response from `GET /billing/transactions`. */
export interface TokenTransactionPage {
  transactions: TokenTransaction[]
  next_cursor: string | null
}

// --- Step 53: plans, Stripe Checkout, subscription management, billing
// history. Everything below is additive to Step 48's types above — no
// existing export is modified or removed. Field names mirror
// `server/internal/interface/handler/dto.go`'s billing DTOs 1:1 (see each
// type's docstring for the exact Go counterpart).

/**
 * How a `BillingPlan` recurs: `"month"` for a recurring subscription plan,
 * `"one_time"` for a single-purchase token pack.
 */
export type BillingInterval = "month" | "one_time"

/**
 * A single purchasable catalog entry, returned by `GET /billing/plans`
 * (`BillingPlanResponse`). `price_cents`/`token_allowance` are integer
 * minor-currency-unit / whole-token counts respectively — no Stripe price ID
 * is exposed to the client.
 */
export interface BillingPlan {
  code: string
  name: string
  description: string
  price_cents: number
  currency: string
  interval: BillingInterval
  token_allowance: number
}

/**
 * The current user's subscription lifecycle state. `"none"` is a
 * client-side-only value: the server signals "no subscription" via a bare
 * `204 No Content` on `GET /billing/subscription`, mapped to this status by
 * `get-subscription.ts` rather than exposed as a literal server string.
 */
export type SubscriptionStatus = "active" | "trialing" | "past_due" | "canceled" | "none"

/**
 * The current user's subscription, returned by `GET /billing/subscription`
 * and `POST /billing/subscription/cancel` (`SubscriptionResponse`). All
 * fields besides `status`/`cancel_at_period_end` are `null` only in this
 * client-side `"none"` mapping — the server's own `200` response always
 * populates `plan_code`/`monthly_token_allocation`/`current_period_start`/
 * `current_period_end` (only `canceled_at` is nullable server-side).
 */
export interface Subscription {
  status: SubscriptionStatus
  plan_code: string | null
  monthly_token_allocation: number | null
  current_period_start: string | null
  current_period_end: string | null
  cancel_at_period_end: boolean
  canceled_at: string | null
}

/** Response body for `POST /billing/checkout-session`. */
export interface CheckoutSessionResponse {
  checkout_url: string
}

/** Response body for `POST /billing/portal-session`. */
export interface BillingPortalSessionResponse {
  portal_url: string
}

/**
 * The lifecycle status of a `Payment` row, mirroring whatever string
 * `server/internal/usecase/billing`'s webhook handling actually persists
 * into `payment_history.status`. Only `"succeeded"` is written by the
 * merged usecase today; `"failed"`/`"refunded"`/`"pending"` are kept in the
 * union defensively for statuses a future webhook path may add, each with
 * its own `Badge` color in `PaymentHistoryList`.
 */
export type PaymentStatus = "succeeded" | "failed" | "refunded" | "pending"

/**
 * A single `payment_history` row, returned by `GET /billing/payments`
 * (`PaymentRecordResponse`). There is no description or invoice-URL field on
 * the server contract — `PaymentHistoryList` derives its row label from
 * `kind` + `tokens_credited`, and Stripe-hosted receipts remain reachable
 * via the billing portal instead.
 *
 * `stripe_reference_id` is the Stripe object this payment was recorded
 * against (a Checkout Session ID for a token purchase, an invoice ID for a
 * subscription renewal). The post-Checkout success page
 * (`app/(main)/billing/checkout/success/page.tsx`) matches it against its
 * own `session_id` query parameter to identify which payment resulted from
 * the Checkout Session the user just completed, rather than inferring
 * success from a balance delta.
 */
export interface Payment {
  id: string
  kind: "subscription" | "token_purchase"
  amount_cents: number
  currency: string
  tokens_credited: number
  status: PaymentStatus
  stripe_reference_id: string
  created_at: string
}

/** Raw paginated response from `GET /billing/payments`. */
export interface PaymentHistoryPage {
  payments: Payment[]
  next_cursor: string | null
}
