import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { Invitation } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["invitations", "by-code", code]`
 * query, sourced from `GET /api/proxy/invitations/by-code/:code` — used by
 * the `/invite/[code]` landing page to preview an invitation before it is
 * accepted or rejected. A 404 (unknown/expired code) surfaces as a query
 * error, handled by the page itself.
 */
export function getInvitationByCodeQueryOptions(
  code: string,
  fetcher: Fetcher = apiRequest,
) {
  return queryOptions({
    queryKey: ["invitations", "by-code", code] as const,
    queryFn: () => fetcher<Invitation>(`/invitations/by-code/${code}`),
  })
}
