import "server-only"
import { headers } from "next/headers"
import { ApiRequestError } from "@/lib/http-client"

/**
 * Server-only fetcher used exclusively for RSC `prefetchQuery` calls (e.g.
 * `app/(main)/rooms/page.tsx`, `app/(main)/rooms/[roomId]/page.tsx`).
 *
 * The `server-only` import guarantees a build-time error if this module is
 * ever imported from a Client Component, since it must never end up in a
 * browser bundle.
 *
 * Deliberately re-enters the app's own `/api/proxy/*` route (Step 4's
 * `app/api/proxy/[...path]/route.ts`, rewritten by Step 30 to forward the
 * Kratos session cookie verbatim instead of minting a bearer token) rather
 * than talking to the Go API directly — the request-to-upstream forwarding
 * stays in exactly one place (the proxy route handler). The incoming
 * request's `cookie` header is forwarded so the proxy can read the same
 * `ory_kratos_session` cookie the browser would have sent.
 *
 * Throws the same `ApiRequestError` (with its `status` code intact) that
 * `@/lib/http-client`'s `apiFetch` throws on the client, so callers like
 * `app/(main)/rooms/[roomId]/page.tsx` can distinguish a `404` (render
 * `not-found.tsx`) from any other failure (let it propagate to `error.tsx`)
 * instead of treating every fetch failure as "not found".
 */

const APP_INTERNAL_URL = process.env.APP_INTERNAL_URL ?? "http://localhost:3000"

async function request<T>(path: string): Promise<T> {
  const incoming = await headers()
  const res = await fetch(`${APP_INTERNAL_URL}/api/proxy${path}`, {
    headers: { cookie: incoming.get("cookie") ?? "" },
    cache: "no-store",
  })

  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: "Unknown error" }))
    const message =
      typeof body === "object" &&
      body !== null &&
      "message" in body &&
      typeof (body as { message: unknown }).message === "string"
        ? (body as { message: string }).message
        : `Request failed: ${res.status}`

    throw new ApiRequestError(res.status, body, message)
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
