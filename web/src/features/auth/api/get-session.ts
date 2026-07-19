import { queryOptions } from "@tanstack/react-query"
import type { KratosSession, Session } from "../types"

/**
 * Fetches the current session from `GET /api/kratos/sessions/whoami`.
 *
 * A `401` (missing or expired session cookie) resolves `null` instead of
 * throwing, so callers see a plain "signed out" state rather than having to
 * catch an error themselves. Any other non-2xx status is unexpected and
 * throws.
 */
export async function getSession(): Promise<Session> {
  const res = await fetch("/api/kratos/sessions/whoami", {
    headers: { Accept: "application/json" },
  })

  if (res.status === 401) {
    return null
  }

  if (!res.ok) {
    throw new Error(`Failed to fetch session: HTTP ${res.status}`)
  }

  return (await res.json()) as KratosSession
}

/**
 * `queryOptions()` factory for the `["auth", "session"]` query.
 *
 * Kept as its own factory (rather than inlining `useQuery` calls) so the
 * query key stays defined in exactly one place — every invalidation call
 * site elsewhere in the app uses the same `["auth", "session"]` key.
 */
export function getSessionQueryOptions() {
  return queryOptions({
    queryKey: ["auth", "session"] as const,
    queryFn: getSession,
  })
}
