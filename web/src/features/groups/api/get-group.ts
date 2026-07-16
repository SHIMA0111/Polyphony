import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { Group } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["groups", groupId]` query, sourced from
 * `GET /api/proxy/groups/:groupId`.
 *
 * Parameterized by fetcher following the Step 9 `get-room.ts` pattern so
 * this can be prefetched server-side and fetched client-side (default
 * `apiRequest`, via `useGroup`) with the same query key.
 */
export function getGroupQueryOptions(
  groupId: string,
  fetcher: Fetcher = apiRequest,
) {
  return queryOptions({
    queryKey: ["groups", groupId] as const,
    queryFn: () => fetcher<Group>(`/groups/${groupId}`),
  })
}
