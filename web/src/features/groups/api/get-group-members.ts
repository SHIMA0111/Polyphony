import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { GroupMemberListResponse } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["groups", groupId, "members"]` query,
 * sourced from `GET /api/proxy/groups/:groupId/members`.
 *
 * Parameterized by fetcher following the Step 9/37 pattern so this can be
 * prefetched server-side and fetched client-side (default `apiRequest`, via
 * `useGroupMembers`) with the same query key.
 */
export function getGroupMembersQueryOptions(
  groupId: string,
  fetcher: Fetcher = apiRequest,
) {
  return queryOptions({
    queryKey: ["groups", groupId, "members"] as const,
    queryFn: () =>
      fetcher<GroupMemberListResponse>(`/groups/${groupId}/members`),
  })
}
