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
 * flows and `sessions/whoami`).
 *
 * Forwards the incoming request's raw `Cookie` header verbatim (the Kratos
 * session cookie, and the flow's CSRF cookie set by the preceding `GET
 * .../browser` call, both round-trip through this same-origin proxy so the
 * browser attaches them automatically) and the request body verbatim on
 * `POST`. Sets `Accept: application/json` on the upstream request — Kratos's
 * AJAX/SPA contract: without it, `self-service/{login,registration}/browser`
 * and flow submission endpoints respond with `302`/`303` HTML redirects
 * intended for full-page browser navigation instead of returning the
 * flow/session JSON this proxy's callers expect.
 *
 * Every upstream `Set-Cookie` header is copied onto the outgoing
 * `NextResponse` — Kratos may set more than one (e.g. session + CSRF
 * cookies), and `Headers.get("set-cookie")` only ever returns one, so
 * `getSetCookie()` is used and each value is appended individually.
 *
 * The upstream body is streamed back unchanged, but only `Content-Type` is
 * copied from the upstream response's other headers — `Content-Encoding`/
 * `Transfer-Encoding` are intentionally dropped since the body was already
 * read and decoded here (mirrors `app/api/proxy/[...path]/route.ts`).
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
 * @returns A `NextResponse` mirroring the upstream status, body, and
 *   `Set-Cookie` headers; or a `504`/`502` JSON error response if the
 *   upstream `fetch` timed out or otherwise failed.
 */
async function proxy(
  request: NextRequest,
  context: RouteContext,
): Promise<NextResponse> {
  const { path } = await context.params
  const upstreamUrl = `${KRATOS_PUBLIC_URL}/${path.join("/")}${request.nextUrl.search}`

  const headers: Record<string, string> = {
    Accept: "application/json",
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

  const responseBody = await upstreamRes.arrayBuffer()
  const responseHeaders = new Headers()
  const upstreamContentType = upstreamRes.headers.get("Content-Type")
  if (upstreamContentType) {
    responseHeaders.set("Content-Type", upstreamContentType)
  }
  for (const cookieValue of upstreamRes.headers.getSetCookie()) {
    responseHeaders.append("Set-Cookie", cookieValue)
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
