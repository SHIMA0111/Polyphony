import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { InvitationListResponse } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["rooms", roomId, "invitations"]`
 * query, sourced from `GET /api/proxy/rooms/:roomId/invitations`.
 */
export function getRoomInvitationsQueryOptions(
  roomId: string,
  fetcher: Fetcher = apiRequest,
) {
  return queryOptions({
    queryKey: ["rooms", roomId, "invitations"] as const,
    queryFn: () =>
      fetcher<InvitationListResponse>(`/rooms/${roomId}/invitations`),
  })
}
