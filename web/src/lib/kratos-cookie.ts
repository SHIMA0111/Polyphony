/**
 * Name of the Kratos session cookie the browser holds once a self-service
 * flow (login/registration) completes successfully.
 *
 * Kratos itself sets this cookie (via `Set-Cookie`, round-tripped verbatim
 * by `app/api/kratos/[...path]/route.ts`) — this app never mints or reads
 * its value, only checks for its *presence* as a UX fast-path in
 * `middleware.ts`.
 *
 * Read from the server-side `KRATOS_COOKIE_NAME` env var (this module is
 * only ever imported by `middleware.ts`, which runs server/edge-side, so
 * this is safe to read directly without `NEXT_PUBLIC_` exposure), falling
 * back to Kratos's own default cookie name when unset. Must match
 * `KRATOS_COOKIE_NAME`'s default in
 * `server/internal/infrastructure/config/config.go` (and the
 * `KRATOS_COOKIE_NAME` env var wired through `docker-compose.yml`) — keep
 * these in lockstep so the web app's route guard and the Go API's session
 * lookup never drift onto different cookie names.
 */
export const KRATOS_SESSION_COOKIE_NAME =
  process.env.KRATOS_COOKIE_NAME ?? "ory_kratos_session"
