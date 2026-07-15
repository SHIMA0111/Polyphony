/**
 * Shared constants for the httpOnly session cookie that the Web BFF sets
 * after a successful login/register call to the Go API.
 *
 * Both the `/api/auth/*` route handlers (which set/clear the cookie) and
 * `middleware.ts` (which only checks for the cookie's presence as a UX
 * fast-path) import these constants so the cookie name and lifetime never
 * drift between the two.
 */

/** Name of the httpOnly cookie that stores the Go API's JWT access token. */
export const ACCESS_TOKEN_COOKIE = "access_token"

/**
 * Lifetime of the access token cookie, in seconds.
 *
 * Matches `tokenExpiry = 24 * time.Hour` in
 * `server/internal/interface/auth/simple_jwt.go` — keep these in lockstep so
 * the cookie never outlives (or expires before) the JWT it stores.
 */
export const ACCESS_TOKEN_MAX_AGE_SECONDS = 60 * 60 * 24
