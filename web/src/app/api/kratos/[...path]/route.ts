import { NextResponse, type NextRequest } from "next/server"

/** Base URL of the Kratos public API, read server-side only (never inlined into the client bundle). */
const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL ?? "http://localhost:4433"

/** Route handlers must not be statically optimized: every request carries a distinct session cookie. */
export const dynamic = "force-dynamic"

interface RouteContext {
  params: Promise<{ path: string[] }>
}

/**
 * Shared implementation for the `GET`/`POST` methods this catch-all proxy
 * forwards to Kratos's public API (self-service login/registration/logout
 * flows, `sessions/whoami`, and — since Step 44 — the OIDC method's
 * provider-redirect and callback legs).
 *
 * Forwards the incoming request's raw `Cookie` header verbatim (the Kratos
 * session cookie, and the flow's CSRF/continuity cookies set by preceding
 * calls, all round-trip through this same-origin proxy so the browser
 * attaches them automatically) and the request body verbatim on `POST`.
 *
 * The incoming request's own `Accept` header is forwarded as-is (defaulting
 * to `application/json` only when the caller sent none), rather than always
 * being forced to `application/json` — Kratos's content negotiation on flow
 * submission is genuinely bimodal:
 *   - With `Accept: application/json` (this app's JS-driven `fetch` calls in
 *     `features/auth/api/*.ts`, all of which set it explicitly), Kratos
 *     returns flow/session JSON directly, or — for the `oidc` method
 *     specifically — a `422 browser_location_change_required` body carrying
 *     a `redirect_browser_to` URL instead of a real HTTP redirect (since a
 *     JS `fetch` caller must decide for itself whether/how to navigate).
 *   - Without that header (a real, full-page HTML `<form>` POST — see
 *     `SocialLoginButtons.tsx`, which must submit via genuine browser
 *     navigation, not `fetch`, for exactly this reason), Kratos instead
 *     issues a genuine `302`/`303` redirect straight to the OIDC provider.
 * Forcing `application/json` unconditionally would turn that second case
 * into the first, leaving a real browser navigation stuck rendering a raw
 * `422` JSON body instead of continuing to the provider.
 *
 * The upstream `fetch` uses `redirect: "manual"` so any `3xx` upstream
 * response (the case above, and Kratos's own callback-completion redirect)
 * is returned to the browser as a real redirect rather than being followed
 * server-side and collapsed into a `200` — the caller (a real browser
 * navigation) must see and follow the redirect itself, since the next hop
 * is frequently a different origin entirely (the OIDC provider). Node's
 * `fetch` (unlike a browser's) still exposes the real status/`Location`
 * header in `manual` mode (no opaque-redirect filtering applies server-side).
 *
 * Every upstream `Set-Cookie` header is copied onto the outgoing
 * `NextResponse` — Kratos may set more than one (e.g. session + CSRF/
 * continuity cookies), and `Headers.get("set-cookie")` only ever returns
 * one, so `getSetCookie()` is used and each value is appended individually.
 * This applies whether the upstream response is a normal body or a
 * redirect: Kratos sets its continuity cookie on the very `302`/`303` that
 * kicks off the OIDC provider redirect (see the module doc above), so it
 * must be preserved on redirect responses too, not just `200`s.
 *
 * The upstream body is streamed back unchanged for non-redirect responses,
 * but only `Content-Type` is copied from the upstream response's other
 * headers — `Content-Encoding`/`Transfer-Encoding` are intentionally
 * dropped since the body was already read and decoded here (mirrors
 * `app/api/proxy/[...path]/route.ts`).
 *
 * @param request - The incoming Next.js request.
 * @param context - Route context carrying the (Next 16 async) dynamic `path` segments.
 * @returns A `NextResponse` mirroring the upstream status, headers
 *   (`Location`/`Content-Type`), body, and `Set-Cookie` headers.
 */
async function proxy(
  request: NextRequest,
  context: RouteContext,
): Promise<NextResponse> {
  const { path } = await context.params
  const upstreamUrl = `${KRATOS_PUBLIC_URL}/${path.join("/")}${request.nextUrl.search}`

  const headers: Record<string, string> = {
    Accept: request.headers.get("Accept") ?? "application/json",
  }
  const contentType = request.headers.get("Content-Type")
  if (contentType) {
    headers["Content-Type"] = contentType
  }
  const cookie = request.headers.get("Cookie")
  if (cookie) {
    headers["Cookie"] = cookie
  }

  const hasBody = request.method !== "GET" && request.method !== "HEAD"

  const upstreamRes = await fetch(upstreamUrl, {
    method: request.method,
    headers,
    body: hasBody ? await request.arrayBuffer() : undefined,
    redirect: "manual",
  })

  const responseHeaders = new Headers()
  for (const cookieValue of upstreamRes.headers.getSetCookie()) {
    responseHeaders.append("Set-Cookie", cookieValue)
  }

  if (upstreamRes.status >= 300 && upstreamRes.status < 400) {
    const location = upstreamRes.headers.get("Location")
    if (location) {
      responseHeaders.set("Location", location)
    }
    return new NextResponse(null, {
      status: upstreamRes.status,
      headers: responseHeaders,
    })
  }

  const responseBody = await upstreamRes.arrayBuffer()
  const upstreamContentType = upstreamRes.headers.get("Content-Type")
  if (upstreamContentType) {
    responseHeaders.set("Content-Type", upstreamContentType)
  }

  return new NextResponse(responseBody, {
    status: upstreamRes.status,
    headers: responseHeaders,
  })
}

export async function GET(request: NextRequest, context: RouteContext) {
  return proxy(request, context)
}

export async function POST(request: NextRequest, context: RouteContext) {
  return proxy(request, context)
}
