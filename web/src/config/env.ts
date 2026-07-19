/**
 * Centralized reads of the `NEXT_PUBLIC_*` build-time environment variables
 * consumed by client-side code, so every call site (the WS hook, future MSW
 * bootstrap wiring, ...) agrees on the exact same flag name/URL rather than
 * repeating `process.env.NEXT_PUBLIC_*` string literals across the codebase.
 *
 * Both variables are Next.js "public" env vars: they are inlined into the
 * client bundle at build time (see the `NEXT_PUBLIC_API_URL`/
 * `NEXT_PUBLIC_MOCK_API` build args in `docker-compose.yml`), so reading
 * `process.env` here works identically in server and browser code.
 */

/**
 * Whether the app is running in MSW mock mode. This is the same
 * `NEXT_PUBLIC_MOCK_API` flag `middleware.ts` already reads to bypass the
 * Kratos session guard for the no-login demo flow.
 *
 * `useRoomSocket` uses this to stay fully inert — no `WebSocket` is ever
 * constructed — since the MSW-mocked REST handlers have no live event
 * source for it to subscribe to.
 */
export function isMockMode(): boolean {
  return process.env.NEXT_PUBLIC_MOCK_API === "true"
}

/**
 * Base URL of the Go API's WebSocket upgrade endpoint, derived from
 * `NEXT_PUBLIC_API_URL` (e.g. `http://localhost:8080` becomes
 * `ws://localhost:8080`; `https://api.example.com` becomes
 * `wss://api.example.com`).
 *
 * The Next.js BFF proxy at `/api/proxy/*` (`web/src/app/api/proxy/[...path]/route.ts`)
 * cannot upgrade a WebSocket handshake, so — unlike every other authenticated
 * data call — the browser must open this connection directly against the Go
 * API's own origin, per Step 15's public `GET /rooms/:roomId/ws` route
 * (`server/internal/app/routes_websocket.go`).
 */
export function getWsBaseUrl(): string {
  const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"
  return apiUrl.replace(/^http/, "ws")
}
