"use client"

import { useInfiniteQuery } from "@tanstack/react-query"
import { getUsageHistoryQueryOptions } from "../api/get-usage-history"

/**
 * Cursor-paginated token usage/transaction history for the current user,
 * sourced from `GET /api/proxy/billing/transactions`.
 *
 * Returns the raw `useInfiniteQuery` result (`data.pages`, `fetchNextPage`,
 * `hasNextPage`, `isFetchingNextPage`, ...); `UsageHistoryList` flattens
 * `data.pages` itself since, unlike messages, transaction pages don't need
 * any oldest/newest re-ordering — each page is already newest-first and
 * later pages are strictly older.
 */
export function useUsageHistory() {
  return useInfiniteQuery(getUsageHistoryQueryOptions())
}
