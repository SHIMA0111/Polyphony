import { infiniteQueryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { TokenTransactionPage } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/** Default page size for `GET /billing/transactions`. */
export const USAGE_HISTORY_PAGE_LIMIT = 20

/**
 * Calls `GET /billing/transactions`, returning a single page exactly as the
 * API shapes it: `transactions` newest-first, `next_cursor` the cursor to
 * pass to fetch the next (older) page, mirroring the cursor/limit
 * query-string construction already used by
 * `web/src/features/messages/api/get-messages.ts`'s `getMessages`.
 */
export function getUsageHistory(
  { cursor, limit = USAGE_HISTORY_PAGE_LIMIT }: { cursor?: string; limit?: number } = {},
  fetcher: Fetcher = apiRequest,
): Promise<TokenTransactionPage> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) {
    params.set("cursor", cursor)
  }
  return fetcher<TokenTransactionPage>(`/billing/transactions?${params.toString()}`)
}

/**
 * `infiniteQueryOptions()` factory for the `["billing", "transactions"]`
 * query — cursor-based "load more" pagination via `useInfiniteQuery`,
 * mirroring `get-messages.ts`'s `getMessagesInfiniteQueryOptions`.
 */
export function getUsageHistoryQueryOptions(
  limit = USAGE_HISTORY_PAGE_LIMIT,
  fetcher: Fetcher = apiRequest,
) {
  return infiniteQueryOptions({
    queryKey: ["billing", "transactions", { limit }] as const,
    queryFn: ({ pageParam }): Promise<TokenTransactionPage> =>
      getUsageHistory({ cursor: pageParam, limit }, fetcher),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage: TokenTransactionPage) => lastPage.next_cursor ?? undefined,
  })
}

// Re-exported so callers that only need the element type don't have to reach
// into `../types` themselves.
export type { TokenTransactionPage }
