import { apiRequest } from "@/lib/http-client"
import type { BillingPlan, CheckoutSessionResponse } from "../types"

/** Request body for `POST /billing/checkout-session` (`CreateCheckoutSessionRequest`). */
export interface CreateCheckoutSessionInput {
  type: "subscription" | "token_purchase"
  plan_code?: string
  package_code?: string
}

/** Poster shape shared by mutation `api/` modules that only need `POST`. */
type Poster = <T>(path: string, options: RequestInit) => Promise<T>

/** Calls `POST /api/proxy/billing/checkout-session` with the given body. */
function postCheckoutSession(
  input: CreateCheckoutSessionInput,
  poster: Poster = apiRequest,
): Promise<CheckoutSessionResponse> {
  return poster<CheckoutSessionResponse>("/billing/checkout-session", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

/**
 * Derives the `CreateCheckoutSessionInput` body from a `BillingPlan` and
 * starts a Stripe Checkout Session for it: `interval === "month"` plans are
 * purchased as a `"subscription"` (keyed by `plan_code`), `interval ===
 * "one_time"` plans (token packs) as a `"token_purchase"` (keyed by
 * `package_code`, reusing the plan catalog's own `code` — Step 49's catalog
 * has no separate package-code field).
 */
export function createCheckoutSession(
  plan: BillingPlan,
  poster: Poster = apiRequest,
): Promise<CheckoutSessionResponse> {
  const input: CreateCheckoutSessionInput =
    plan.interval === "month"
      ? { type: "subscription", plan_code: plan.code }
      : { type: "token_purchase", package_code: plan.code }

  return postCheckoutSession(input, poster)
}
