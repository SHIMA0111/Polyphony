import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { InvitationListResponse } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["invitations", "mine"]` query, sourced
 * from `GET /api/proxy/invitations` — the caller's own pending invitations
 * across every room.
 */
export function getMyInvitationsQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["invitations", "mine"] as const,
    queryFn: () => fetcher<InvitationListResponse>("/invitations"),
  })
}
