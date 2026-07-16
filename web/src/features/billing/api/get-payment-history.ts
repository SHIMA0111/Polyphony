import { infiniteQueryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { PaymentHistoryPage } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/** Default page size for `GET /billing/payments`. */
export const PAYMENT_HISTORY_PAGE_LIMIT = 20

/**
 * Calls `GET /billing/payments`, returning a single page exactly as the API
 * shapes it: `payments` newest-first, `next_cursor` the cursor to pass to
 * fetch the next (older) page — mirroring the cursor/limit query-string
 * construction `get-usage-history.ts`'s `getUsageHistory` (Step 48) already
 * established for this feature module.
 */
export function getPaymentHistory(
  { cursor, limit = PAYMENT_HISTORY_PAGE_LIMIT }: { cursor?: string; limit?: number } = {},
  fetcher: Fetcher = apiRequest,
): Promise<PaymentHistoryPage> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) {
    params.set("cursor", cursor)
  }
  return fetcher<PaymentHistoryPage>(`/billing/payments?${params.toString()}`)
}

/**
 * `infiniteQueryOptions()` factory for the `["billing", "payment-history"]`
 * query — cursor-based "load more" pagination via `useInfiniteQuery`,
 * mirroring `get-usage-history.ts`'s `getUsageHistoryQueryOptions`.
 */
export function getPaymentHistoryQueryOptions(
  limit = PAYMENT_HISTORY_PAGE_LIMIT,
  fetcher: Fetcher = apiRequest,
) {
  return infiniteQueryOptions({
    queryKey: ["billing", "payment-history"] as const,
    queryFn: ({ pageParam }): Promise<PaymentHistoryPage> =>
      getPaymentHistory({ cursor: pageParam, limit }, fetcher),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage: PaymentHistoryPage) => lastPage.next_cursor ?? undefined,
  })
}
