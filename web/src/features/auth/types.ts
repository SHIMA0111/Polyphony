/**
 * The authenticated caller's own identity, exactly as returned by Step 1's
 * `GET /users/me` whoami endpoint (`server/internal/interface/handler/dto.go`'s
 * `UserResponse`), reached client-side via the Step 4 data-plane proxy at
 * `/api/proxy/users/me`.
 */
export interface User {
  id: string
  email: string
  username: string
  created_at: string
}

/**
 * Session state derived from `GET /users/me`.
 *
 * `null` represents a signed-out caller: the proxy's `401` response (missing
 * or expired session cookie) is mapped to `null` by `getSessionQueryOptions`
 * rather than surfaced as a query error, since "not logged in" is an
 * expected, common state rather than a failure.
 */
export type Session = User | null
