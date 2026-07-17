import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { CurrentUser } from "../types"

/**
 * Fetches the authenticated caller's own local user record from
 * `GET /users/me`, via `apiRequest`'s `/api/proxy/*` BFF (which forwards the
 * browser's Kratos session cookie -- see `apiRequest`'s own docstring).
 *
 * Deliberately distinct from `get-session.ts`'s `getSession`: that reads
 * Kratos's own `identity.id`, which is a different UUID from the local
 * `users.id` this resolves to -- see `CurrentUser`'s doc comment.
 */
export function getCurrentUser(): Promise<CurrentUser> {
  return apiRequest<CurrentUser>("/users/me")
}

/**
 * `queryOptions()` factory for the `["auth", "current-user"]` query.
 *
 * Kept as its own factory (rather than inlining `useQuery` calls) so the
 * query key stays defined in exactly one place, matching `get-session.ts`'s
 * `getSessionQueryOptions` convention.
 */
export function getCurrentUserQueryOptions() {
  return queryOptions({
    queryKey: ["auth", "current-user"] as const,
    queryFn: getCurrentUser,
  })
}
