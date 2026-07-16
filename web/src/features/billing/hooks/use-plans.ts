"use client"

import { useQuery } from "@tanstack/react-query"
import { getPlansQueryOptions } from "../api/get-plans"

/**
 * The purchasable plan/token-pack catalog, sourced from
 * `GET /api/proxy/billing/plans`. Consumed by `PlanList` (rendering the
 * catalog itself) and `SubscriptionSummary` (resolving a subscription's
 * `plan_code` to a display name).
 */
export function usePlans() {
  return useQuery(getPlansQueryOptions())
}
