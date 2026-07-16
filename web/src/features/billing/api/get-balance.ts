import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { TokenBalance } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["billing", "balance"]` query, following
 * the same fetcher-parameterized pattern as
 * `web/src/features/rooms/api/get-rooms.ts`'s `getRoomsQueryOptions`.
 */
export function getBalanceQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["billing", "balance"] as const,
    queryFn: () => fetcher<TokenBalance>("/billing/balance"),
  })
}
