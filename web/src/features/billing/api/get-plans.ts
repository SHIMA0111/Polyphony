import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { BillingPlan } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/** Raw wire shape of `GET /billing/plans` (`BillingPlanListResponse`). */
interface BillingPlanListResponse {
  plans: BillingPlan[]
}

/**
 * Calls `GET /billing/plans`, unwrapping the server's
 * `{ "plans": [...] }` envelope (`BillingPlanListResponse`) down to the bare
 * `BillingPlan[]` the rest of this feature module works with.
 */
async function getPlans(fetcher: Fetcher = apiRequest): Promise<BillingPlan[]> {
  const { plans } = await fetcher<BillingPlanListResponse>("/billing/plans")
  return plans
}

/**
 * `queryOptions()` factory for the `["billing", "plans"]` query, following
 * the same fetcher-parameterized pattern as
 * `web/src/features/billing/api/get-balance.ts`'s `getBalanceQueryOptions`.
 */
export function getPlansQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["billing", "plans"] as const,
    queryFn: () => getPlans(fetcher),
  })
}
