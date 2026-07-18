"use client"

import { useQuery } from "@tanstack/react-query"
import { getSubscriptionQueryOptions } from "../api/get-subscription"

/**
 * The current user's subscription, sourced from
 * `GET /api/proxy/billing/subscription` (mapped client-side to
 * `status: "none"` on the server's documented `204 No Content`). Callers
 * that need to poll while a Checkout webhook is still being processed
 * (`app/(main)/billing/checkout/success/page.tsx`) can pass a
 * `refetchInterval` override.
 */
export function useSubscription(options?: { refetchInterval?: number | false }) {
  return useQuery({
    ...getSubscriptionQueryOptions(),
    ...options,
  })
}
