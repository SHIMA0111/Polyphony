import "server-only"
import { headers } from "next/headers"

/**
 * Server-only fetcher used exclusively for RSC `prefetchQuery` calls (e.g.
 * `app/(main)/rooms/page.tsx`, `app/(main)/rooms/[roomId]/page.tsx`).
 *
 * The `server-only` import guarantees a build-time error if this module is
 * ever imported from a Client Component, since it must never end up in a
 * browser bundle.
 *
 * Deliberately re-enters the app's own `/api/proxy/*` route (Step 4's
 * `app/api/proxy/[...path]/route.ts`) rather than talking to the Go API
 * directly or re-deriving the `Authorization: Bearer` header from the
 * session cookie itself — that cookie-to-Bearer translation stays in
 * exactly one place (the proxy route handler). The incoming request's
 * `cookie` header is forwarded so the proxy can read the same session the
 * browser would have sent.
 */

const APP_INTERNAL_URL = process.env.APP_INTERNAL_URL ?? "http://localhost:3000"

/**
 * Error thrown by `serverHttpClient.get` when the proxied request resolves
 * with a non-2xx status, carrying the upstream HTTP status code so callers
 * can distinguish "not found" (safe to map to Next's `notFound()`) from
 * every other failure (which should propagate to the nearest `error.tsx`
 * boundary instead of being misreported as a 404).
 */
export class HttpError extends Error {
  readonly status: number

  constructor(status: number) {
    super(`Request failed: ${status}`)
    this.name = "HttpError"
    this.status = status
  }
}

async function request<T>(path: string): Promise<T> {
  const incoming = await headers()
  const res = await fetch(`${APP_INTERNAL_URL}/api/proxy${path}`, {
    headers: { cookie: incoming.get("cookie") ?? "" },
    cache: "no-store",
  })

  if (!res.ok) {
    throw new HttpError(res.status)
  }

  if (res.status === 204) {
    return undefined as T
  }

  return res.json() as Promise<T>
}

/**
 * Server-side counterpart to `@/lib/http-client`'s `apiRequest`, shaped so
 * the same `queryOptions()` factories (e.g. `getRoomsQueryOptions`) can be
 * called with either fetcher: `apiRequest` on the client, `serverHttpClient.get`
 * during RSC prefetch.
 */
export const serverHttpClient = {
  get: <T>(path: string) => request<T>(path),
}
