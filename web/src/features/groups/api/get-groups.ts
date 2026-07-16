import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { GroupListResponse } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["groups"]` query, sourced from
 * `GET /api/proxy/groups`.
 *
 * Parameterized by fetcher following the Step 9/37 `get-rooms.ts`/
 * `get-members.ts` pattern, so this can be prefetched server-side and
 * fetched client-side (default `apiRequest`, via `useGroups`) with the same
 * query key.
 */
export function getGroupsQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["groups"] as const,
    queryFn: () => fetcher<GroupListResponse>("/groups"),
  })
}
