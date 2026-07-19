/**
 * The subset of Kratos's `sessions/whoami` response body this app reads,
 * reached client-side via `GET /api/kratos/sessions/whoami`
 * (`web/src/features/auth/api/get-session.ts`).
 *
 * Kratos's actual payload carries more fields (`id`, `expires_at`,
 * `authenticated_at`, ...) that this app doesn't currently need; only
 * `identity.traits` (the `email`/`username` schema fields from Step 11's
 * identity schema) are modeled here.
 */
export interface KratosSession {
  identity: {
    id: string
    traits: {
      email: string
      username: string
    }
  }
}

/**
 * Session state derived from `GET /api/kratos/sessions/whoami`.
 *
 * `null` represents a signed-out caller: a `401` response (missing or
 * expired session cookie) is mapped to `null` by `getSession` rather than
 * surfaced as a query error, since "not logged in" is an expected, common
 * state rather than a failure.
 */
export type Session = KratosSession | null
