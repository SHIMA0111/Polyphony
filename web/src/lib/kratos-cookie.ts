/**
 * Name of the Kratos session cookie the browser holds once a self-service
 * flow (login/registration) completes successfully.
 *
 * Kratos itself sets this cookie (via `Set-Cookie`, round-tripped verbatim
 * by `app/api/kratos/[...path]/route.ts`) — this app never mints or reads
 * its value, only checks for its *presence* as a UX fast-path in
 * `middleware.ts`.
 *
 * Read from the `KRATOS_COOKIE_NAME` env var, falling back to Kratos's own
 * default ("ory_kratos_session") when unset. This module is only ever
 * imported from `middleware.ts`, which runs server-side (the Edge runtime),
 * so reading `process.env` directly here is safe — it never ends up in a
 * client bundle and needs no `NEXT_PUBLIC_` prefix. Matches
 * `KRATOS_COOKIE_NAME`'s default in
 * `server/internal/infrastructure/config/config.go` (and the
 * `KRATOS_COOKIE_NAME` env var wired through `docker-compose.yml`) — keep
 * these in lockstep so the web app's route guard and the Go API's session
 * lookup never drift onto different cookie names.
 */
export const KRATOS_SESSION_COOKIE_NAME =
  process.env.KRATOS_COOKIE_NAME ?? "ory_kratos_session"
