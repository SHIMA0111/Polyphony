import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { MemberListResponse } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["rooms", roomId, "members"]` query,
 * sourced from `GET /api/proxy/rooms/:roomId/members`.
 *
 * Parameterized by fetcher following the Step 9 `get-room.ts` pattern so
 * this can be prefetched server-side and fetched client-side (default
 * `apiRequest`, via `useMembers`) with the same query key.
 */
export function getMembersQueryOptions(
  roomId: string,
  fetcher: Fetcher = apiRequest,
) {
  return queryOptions({
    queryKey: ["rooms", roomId, "members"] as const,
    queryFn: () =>
      fetcher<MemberListResponse>(`/rooms/${roomId}/members`),
  })
}
