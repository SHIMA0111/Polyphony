import { NextResponse, type NextRequest } from "next/server"

/** Base URL of the Kratos public API, read server-side only (never inlined into the client bundle). */
const KRATOS_PUBLIC_URL = process.env.KRATOS_PUBLIC_URL ?? "http://localhost:4433"

/**
 * Upper bound, in milliseconds, on how long this proxy waits for Kratos to
 * respond before aborting the upstream request and returning a `504` to the
 * caller. Configurable via `KRATOS_PROXY_TIMEOUT_MS` for environments where
 * Kratos is known to be slower (e.g. a cold-started dev stack); defaults to
 * 30 seconds.
 */
const KRATOS_PROXY_TIMEOUT_MS = Number(process.env.KRATOS_PROXY_TIMEOUT_MS) || 30_000

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
 * The upstream `fetch` is bounded by an `AbortController` timeout
 * (`KRATOS_PROXY_TIMEOUT_MS`, default 30s): a Kratos instance that hangs
 * (rather than erroring immediately) would otherwise leave the caller's
 * request pending indefinitely. An abort is reported as a `504` JSON body;
 * any other network-level rejection (e.g. Kratos unreachable, DNS failure)
 * is reported as a `502` JSON body, so a caller always gets a timely,
 * well-formed response instead of an unhandled exception bubbling out of
 * the route handler.
 *
 * @param request - The incoming Next.js request.
 * @param context - Route context carrying the (Next 16 async) dynamic `path` segments.
 * @returns A `NextResponse` mirroring the upstream status, headers
 *   (`Location`/`Content-Type`), body, and `Set-Cookie` headers; or a
 *   `504`/`502` JSON error response if the upstream `fetch` timed out or
 *   otherwise failed.
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

  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), KRATOS_PROXY_TIMEOUT_MS)

  let upstreamRes: Response
  try {
    upstreamRes = await fetch(upstreamUrl, {
      method: request.method,
      headers,
      body: hasBody ? await request.arrayBuffer() : undefined,
      redirect: "manual",
      signal: controller.signal,
    })
  } catch (error) {
    if (error instanceof Error && error.name === "AbortError") {
      return NextResponse.json(
        { error: "Kratos request timed out" },
        { status: 504 },
      )
    }
    return NextResponse.json(
      { error: "Failed to reach Kratos" },
      { status: 502 },
    )
  } finally {
    clearTimeout(timeoutId)
  }

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
