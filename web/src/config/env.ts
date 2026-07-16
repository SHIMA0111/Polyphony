/**
 * Centralized reads of the `NEXT_PUBLIC_*` build-time environment variables
 * consumed by client-side code, so every call site agrees on the exact same
 * flag name/URL rather than repeating `process.env.NEXT_PUBLIC_*` string
 * literals across the codebase.
 *
 * This is a Next.js "public" env var: it is inlined into the client bundle
 * at build time (see the `NEXT_PUBLIC_API_URL` build arg in
 * `docker-compose.yml`), so reading `process.env` here works identically in
 * server and browser code.
 */

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
