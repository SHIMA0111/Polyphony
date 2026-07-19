"use client"

import { useInfiniteQuery } from "@tanstack/react-query"
import { getPaymentHistoryQueryOptions } from "../api/get-payment-history"

/**
 * Cursor-paginated payment history for the current user, sourced from
 * `GET /api/proxy/billing/payments`.
 *
 * Returns the raw `useInfiniteQuery` result (`data.pages`, `fetchNextPage`,
 * `hasNextPage`, `isFetchingNextPage`, ...), mirroring
 * `use-usage-history.ts`'s `useUsageHistory` (Step 48): `PaymentHistoryList`
 * flattens `data.pages` itself since each page is already newest-first and
 * later pages are strictly older.
 */
export function usePaymentHistory() {
  return useInfiniteQuery(getPaymentHistoryQueryOptions())
}
