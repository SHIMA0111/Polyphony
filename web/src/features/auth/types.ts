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

/**
 * The authenticated caller's own local `users` row, sourced from
 * `GET /users/me` (`server/internal/interface/handler/user_handler.go`'s
 * `UserResponse`), reached client-side via `GET /api/proxy/users/me`
 * (`web/src/features/auth/api/get-current-user.ts`).
 *
 * `CurrentUser.id` is deliberately a *different* value from
 * `KratosSession.identity.id` under `AUTH_MODE=kratos`: `identity.id` is
 * Kratos's own identity UUID, while every other user-scoped resource in this
 * app (`Message.sender_id`, `Member.user_id`, ...) is keyed by the local
 * `users.id` UUID that `KratosAuthService.ensureLocalUser` mints and links to
 * the identity via `users.kratos_identity_id` (see `server/internal/
 * interface/auth/kratos.go`). Comparing `identity.id` against a `sender_id`/
 * `user_id` can therefore never match under Kratos auth -- any "is this the
 * viewer's own X" check must compare against `CurrentUser.id` (via
 * `useCurrentUser`) instead.
 */
export interface CurrentUser {
  id: string
  email: string
  username: string
  created_at: string
}
