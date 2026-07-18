import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { Subscription } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/** The client-side representation of "no subscription exists yet". */
const NO_SUBSCRIPTION: Subscription = {
  status: "none",
  plan_code: null,
  monthly_token_allocation: null,
  current_period_start: null,
  current_period_end: null,
  cancel_at_period_end: false,
  canceled_at: null,
  stripe_checkout_session_id: "",
}

/**
 * Calls `GET /billing/subscription`, mapping the server's documented
 * `204 No Content` (no subscription) response to {@link NO_SUBSCRIPTION}.
 *
 * `apiRequest`/`apiFetch` (`@/lib/http-client`) resolves `undefined` for any
 * `204` response body, so a `204` here surfaces as `fetcher(...)` resolving
 * `undefined` rather than throwing — this function is what turns that
 * `undefined` into the explicit `status: "none"` shape the rest of this
 * feature module (`SubscriptionSummary`, `use-subscription.ts`) works with.
 */
async function getSubscription(fetcher: Fetcher = apiRequest): Promise<Subscription> {
  const subscription = await fetcher<Subscription | undefined>("/billing/subscription")
  return subscription ?? NO_SUBSCRIPTION
}

/**
 * `queryOptions()` factory for the `["billing", "subscription"]` query,
 * following the same fetcher-parameterized pattern as
 * `web/src/features/billing/api/get-balance.ts`'s `getBalanceQueryOptions`.
 */
export function getSubscriptionQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["billing", "subscription"] as const,
    queryFn: () => getSubscription(fetcher),
  })
}
