import { queryOptions } from "@tanstack/react-query"
import { apiRequest, ApiRequestError } from "@/lib/http-client"
import type { Session } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * Fetches the current session from `GET /api/proxy/users/me`.
 *
 * A `401` (missing or expired session cookie) is mapped to `null` instead of
 * being thrown, so callers see a plain "signed out" state rather than
 * having to catch {@link ApiRequestError} themselves. Any other error
 * (network failure, `5xx`) is rethrown and surfaces as a query error.
 */
async function getSession(fetcher: Fetcher): Promise<Session> {
  try {
    return await fetcher<Session>("/users/me")
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 401) {
      return null
    }
    throw err
  }
}

/**
 * `queryOptions()` factory for the `["auth", "session"]` query, parameterized
 * by fetcher so the same factory works client-side (default `apiRequest`)
 * or in a test with a stubbed fetcher.
 */
export function getSessionQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["auth", "session"] as const,
    queryFn: () => getSession(fetcher),
  })
}
